package discovery

import (
	"bufio"
	"context"
	"fmt"
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
	if mac != "" {
		result.Reachable = true
		result.Metadata["neighbor_table"] = "arp"
	}
	if result.Reachable {
		lookupCtx, cancel := context.WithTimeout(ctx, timeout)
		names, _ := net.DefaultResolver.LookupAddr(lookupCtx, ip.String())
		cancel()
		if len(names) > 0 {
			result.Hostname = strings.TrimSuffix(names[0], ".")
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
	result := map[string]string{}
	f, err := os.Open("/proc/net/arp")
	if err != nil {
		return result
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 4 && net.ParseIP(fields[0]) != nil && fields[3] != "00:00:00:00:00:00" {
			result[fields[0]] = fields[3]
		}
	}
	return result
}
