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
	if persisted.Hostname != "persistent-node" || persisted.NetworkID != network.ID {
		t.Fatalf("unexpected persisted address: %#v", persisted)
	}
	if err := reopened.DeleteAddress(address.ID); err != nil {
		t.Fatal(err)
	}
	if err := reopened.DeleteNetwork(network.ID); err != nil {
		t.Fatal(err)
	}
}
