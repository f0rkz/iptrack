package ipam

// Repository is the persistence contract used by the API and discovery engine.
type Repository interface {
	Health() error
	Snapshot() (Snapshot, error)
	Networks() ([]Network, error)
	Network(string) (Network, error)
	CreateNetwork(NetworkInput) (Network, error)
	UpdateNetwork(string, NetworkInput) (Network, error)
	DeleteNetwork(string) error
	Addresses(string) ([]Address, error)
	Address(string) (Address, error)
	CreateAddress(AddressInput) (Address, error)
	UpdateAddress(string, AddressInput) (Address, error)
	DeleteAddress(string) error
	Allocate(AllocationInput) (Address, error)
	RecordDiscovery(networkID, ip, hostname, mac string, metadata map[string]string) (Address, error)
}

var _ Repository = (*Store)(nil)
