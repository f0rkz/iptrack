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
