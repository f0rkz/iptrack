package ipam

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestPostgresPersistenceAndAllocation(t *testing.T) {
	databaseURL := os.Getenv("IPTRACK_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("IPTRACK_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	store, err := OpenPostgres(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	network, err := store.CreateNetwork(NetworkInput{Name: "integration-" + newID("test"), CIDR: "198.18.250.0/29", Tags: map[string]string{"test": "postgres"}})
	if err != nil {
		store.Close()
		t.Fatal(err)
	}
	address, err := store.Allocate(AllocationInput{NetworkID: network.ID, Hostname: "persistent-node"})
	if err != nil {
		store.Close()
		t.Fatal(err)
	}
	if address.IP != "198.18.250.1" {
		store.Close()
		t.Fatalf("allocated %s, want 198.18.250.1", address.IP)
	}
	address, err = store.UpdateAddress(address.ID, AddressInput{NetworkID: network.ID, IP: address.IP, Hostname: "manual-node", MAC: "AA-BB-CC-DD-EE-FF", Status: StatusAssigned})
	if err != nil {
		store.Close()
		t.Fatal(err)
	}
	address, err = store.RecordDiscovery(network.ID, address.IP, "discovered-node", "11:22:33:44:55:66", map[string]string{"reachable_via": "icmp"})
	if err != nil {
		store.Close()
		t.Fatal(err)
	}
	if address.Hostname != "manual-node" || address.MAC != "aa:bb:cc:dd:ee:ff" {
		store.Close()
		t.Fatalf("discovery overwrote manual identity: %#v", address)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := OpenPostgres(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	persisted, err := reopened.Address(address.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Hostname != "manual-node" || persisted.MAC != "aa:bb:cc:dd:ee:ff" || persisted.NetworkID != network.ID || persisted.LastSeenAt == nil {
		t.Fatalf("unexpected persisted address: %#v", persisted)
	}
	if err := reopened.DeleteAddress(address.ID); err != nil {
		t.Fatal(err)
	}
	if err := reopened.DeleteNetwork(network.ID); err != nil {
		t.Fatal(err)
	}
}
