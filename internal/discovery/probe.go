package discovery

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Result struct {
	IP        string            `json:"ip"`
	Reachable bool              `json:"reachable"`
	Hostname  string            `json:"hostname,omitempty"`
	MAC       string            `json:"mac,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

type Scanner struct {
	Workers int
}

func (s Scanner) Scan(ctx context.Context, prefix netip.Prefix, ports []int, timeout time.Duration, maxHosts int, progress func(Result)) error {
	hosts, err := Hosts(prefix, maxHosts)
	if err != nil {
		return err
	}
	neighbors := readARP()
	workers := s.Workers
	if workers < 1 {
		workers = 32
	}
	jobs := make(chan netip.Addr)
	results := make(chan Result)
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ip := range jobs {
				results <- probe(ctx, ip, ports, timeout, neighbors[ip.String()])
			}
		}()
	}
	go func() {
		defer close(jobs)
		for _, ip := range hosts {
			select {
			case jobs <- ip:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() { wg.Wait(); close(results) }()
	for result := range results {
		progress(result)
	}
	return ctx.Err()
}

// Probe checks one address using the same mechanisms as a network scan.
func (s Scanner) Probe(ctx context.Context, ip netip.Addr, ports []int, timeout time.Duration) Result {
	return probe(ctx, ip, ports, timeout, readARP()[ip.String()])
}

func Hosts(prefix netip.Prefix, maximum int) ([]netip.Addr, error) {
	prefix = prefix.Masked()
	if maximum < 1 {
		return nil, fmt.Errorf("maximum host count must be positive")
	}
	hosts := make([]netip.Addr, 0)
	for ip := prefix.Addr(); ip.IsValid() && prefix.Contains(ip); ip = ip.Next() {
		if ip == prefix.Addr() || (ip.Is4() && !prefix.Contains(ip.Next())) {
			continue
		}
		if len(hosts) >= maximum {
			return nil, fmt.Errorf("network %s exceeds discovery limit of %d hosts", prefix, maximum)
		}
		hosts = append(hosts, ip)
	}
	return hosts, nil
}

func probe(ctx context.Context, ip netip.Addr, ports []int, timeout time.Duration, mac string) Result {
	result := Result{IP: ip.String(), MAC: mac, Metadata: map[string]string{}}
	var open []int
	for _, port := range ports {
		dialer := net.Dialer{Timeout: timeout}
		conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(ip.String(), strconv.Itoa(port)))
		if err == nil {
			open = append(open, port)
			result.Reachable = true
			conn.Close()
		}
	}
	if len(open) > 0 {
		sort.Ints(open)
		values := make([]string, len(open))
		for i, p := range open {
			values[i] = strconv.Itoa(p)
		}
		result.Metadata["open_tcp_ports"] = strings.Join(values, ",")
		result.Metadata["reachable_via"] = "tcp"
	}
	if ping(ctx, ip.String(), timeout) {
		result.Reachable = true
		if result.Metadata["reachable_via"] != "" {
			result.Metadata["reachable_via"] += ",icmp"
		} else {
			result.Metadata["reachable_via"] = "icmp"
		}
	}
	neighborSource := ""
	if mac != "" {
		neighborSource = "arp"
	} else {
		mac, neighborSource = lookupNeighbor(ctx, ip, timeout, result.Reachable)
		result.MAC = mac
	}
	if result.MAC != "" {
		result.Reachable = true
		result.Metadata["mac_source"] = neighborSource
	}
	if result.Reachable {
		lookupCtx, cancel := context.WithTimeout(ctx, timeout)
		names, _ := net.DefaultResolver.LookupAddr(lookupCtx, ip.String())
		cancel()
		if len(names) > 0 {
			result.Hostname = strings.TrimSuffix(names[0], ".")
			result.Metadata["hostname_source"] = "reverse_dns"
		}
	}
	if len(result.Metadata) == 0 {
		result.Metadata = nil
	}
	return result
}

func ping(ctx context.Context, ip string, timeout time.Duration) bool {
	path, err := exec.LookPath("ping")
	if err != nil {
		return false
	}
	seconds := max(1, int(timeout.Round(time.Second)/time.Second))
	cmd := exec.CommandContext(ctx, path, "-n", "-c", "1", "-W", strconv.Itoa(seconds), ip)
	return cmd.Run() == nil
}

func readARP() map[string]string {
	f, err := os.Open("/proc/net/arp")
	if err != nil {
		return map[string]string{}
	}
	defer f.Close()
	return parseARP(f)
}

func parseARP(r io.Reader) map[string]string {
	result := map[string]string{}
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 4 || net.ParseIP(fields[0]) == nil {
			continue
		}
		flags, err := strconv.ParseUint(fields[2], 0, 32)
		if err != nil || flags&0x2 == 0 {
			continue
		}
		if mac, ok := parseHardwareAddress(fields[3]); ok {
			result[fields[0]] = mac
		}
	}
	return result
}

func lookupNeighbor(ctx context.Context, ip netip.Addr, timeout time.Duration, queryIPCommand bool) (string, string) {
	if mac := readARP()[ip.String()]; mac != "" {
		return mac, "arp"
	}
	// Avoid starting one process for every unresponsive host in a large scan.
	// IPv4 ARP remains a reachability signal; `ip neigh` enriches hosts already
	// reached by TCP/ICMP and adds IPv6 NDP support.
	if !queryIPCommand {
		return "", ""
	}
	path, err := exec.LookPath("ip")
	if err != nil {
		return "", ""
	}
	lookupCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	out, err := exec.CommandContext(lookupCtx, path, "neigh", "show", "to", ip.String()).Output()
	if err != nil {
		return "", ""
	}
	if mac := parseIPNeighbor(string(out)); mac != "" {
		return mac, "neighbor_table"
	}
	return "", ""
}

func parseIPNeighbor(output string) string {
	fields := strings.Fields(output)
	for i := 0; i+1 < len(fields); i++ {
		if fields[i] == "lladdr" {
			if mac, ok := parseHardwareAddress(fields[i+1]); ok {
				return mac
			}
		}
	}
	return ""
}

func parseHardwareAddress(raw string) (string, bool) {
	mac, err := net.ParseMAC(raw)
	if err != nil || len(mac) != 6 {
		return "", false
	}
	return mac.String(), true
}
