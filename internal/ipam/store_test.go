package ipam

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestStoreLifecycleAndAllocation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	network, err := store.CreateNetwork(NetworkInput{Name: "lab", CIDR: "10.42.0.19/29", Tags: map[string]string{"site": "home"}})
	if err != nil {
		t.Fatal(err)
	}
	if network.CIDR != "10.42.0.16/29" {
		t.Fatalf("CIDR was not canonicalized: %s", network.CIDR)
	}
	first, err := store.Allocate(AllocationInput{NetworkID: network.ID, Hostname: "gateway", Status: StatusReserved})
	if err != nil {
		t.Fatal(err)
	}
	if first.IP != "10.42.0.17" {
		t.Fatalf("got %s, want first usable address", first.IP)
	}
	second, err := store.Allocate(AllocationInput{NetworkID: network.ID})
	if err != nil {
		t.Fatal(err)
	}
	if second.IP != "10.42.0.18" || second.Status != StatusAssigned {
		t.Fatalf("unexpected second allocation: %#v", second)
	}
	if _, err := store.CreateAddress(AddressInput{NetworkID: network.ID, IP: "10.42.0.23"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("expected broadcast address rejection, got %v", err)
	}
	if err := store.DeleteNetwork(network.ID); !errors.Is(err, ErrConflict) {
		t.Fatalf("expected non-empty network conflict, got %v", err)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	addresses, _ := reopened.Addresses(network.ID)
	if got := len(addresses); got != 2 {
		t.Fatalf("persisted %d addresses, want 2", got)
	}
}

func TestRejectsOverlappingNetworks(t *testing.T) {
	store, _ := Open(filepath.Join(t.TempDir(), "state.json"))
	if _, err := store.CreateNetwork(NetworkInput{Name: "parent", CIDR: "192.168.0.0/16"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateNetwork(NetworkInput{Name: "child", CIDR: "192.168.20.0/24"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("expected overlap conflict, got %v", err)
	}
	if _, err := store.CreateNetwork(NetworkInput{Name: "other", CIDR: "192.169.20.0/24"}); err != nil {
		t.Fatalf("non-overlapping network failed: %v", err)
	}
}

func TestDiscoveryRefreshesExistingAddress(t *testing.T) {
	store, _ := Open(filepath.Join(t.TempDir(), "state.json"))
	n, _ := store.CreateNetwork(NetworkInput{Name: "lan", CIDR: "10.0.0.0/24"})
	a, err := store.CreateAddress(AddressInput{NetworkID: n.ID, IP: "10.0.0.8", Status: StatusAssigned})
	if err != nil {
		t.Fatal(err)
	}
	updated, err := store.RecordDiscovery(n.ID, "10.0.0.8", "printer.local", "AA:BB:CC:DD:EE:FF", map[string]string{"reachable_via": "icmp"})
	if err != nil {
		t.Fatal(err)
	}
	if updated.ID != a.ID || updated.Hostname != "printer.local" || updated.MAC != "aa:bb:cc:dd:ee:ff" || updated.LastSeenAt == nil {
		t.Fatalf("unexpected discovery update: %#v", updated)
	}
	addresses, _ := store.Addresses(n.ID)
	if len(addresses) != 1 {
		t.Fatal("discovery duplicated an existing address")
	}
}
