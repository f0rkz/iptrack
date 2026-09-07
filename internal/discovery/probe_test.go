package discovery

import (
	"net/netip"
	"strings"
	"testing"
)

func TestHostsIPv4Boundaries(t *testing.T) {
	hosts, err := Hosts(netip.MustParsePrefix("192.0.2.0/30"), 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(hosts) != 2 || hosts[0].String() != "192.0.2.1" || hosts[1].String() != "192.0.2.2" {
		t.Fatalf("unexpected hosts: %v", hosts)
	}
}

func TestHostsEnforcesLimit(t *testing.T) {
	_, err := Hosts(netip.MustParsePrefix("10.0.0.0/24"), 10)
	if err == nil || !strings.Contains(err.Error(), "exceeds discovery limit") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseARP(t *testing.T) {
	table := `IP address       HW type     Flags       HW address            Mask     Device
192.0.2.10       0x1         0x2         AA:BB:CC:DD:EE:FF     *        eth0
192.0.2.11       0x1         0x0         00:00:00:00:00:00     *        eth0
`
	neighbors := parseARP(strings.NewReader(table))
	if got := neighbors["192.0.2.10"]; got != "aa:bb:cc:dd:ee:ff" {
		t.Fatalf("got %q, want normalized MAC", got)
	}
	if _, ok := neighbors["192.0.2.11"]; ok {
		t.Fatal("incomplete neighbor should be ignored")
	}
}

func TestParseIPNeighbor(t *testing.T) {
	output := "2001:db8::10 dev eth0 lladdr AA:BB:CC:DD:EE:FF REACHABLE\n"
	if got := parseIPNeighbor(output); got != "aa:bb:cc:dd:ee:ff" {
		t.Fatalf("got %q, want normalized MAC", got)
	}
	if got := parseIPNeighbor("192.0.2.1 dev eth0 FAILED\n"); got != "" {
		t.Fatalf("got unexpected MAC %q", got)
	}
}
