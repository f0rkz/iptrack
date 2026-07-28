package ipam

import "time"

type Network struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	CIDR        string            `json:"cidr"`
	Description string            `json:"description,omitempty"`
	Tags        map[string]string `json:"tags,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

type Address struct {
	ID         string            `json:"id"`
	NetworkID  string            `json:"network_id"`
	IP         string            `json:"ip"`
	Hostname   string            `json:"hostname,omitempty"`
	Status     AddressStatus     `json:"status"`
	MAC        string            `json:"mac,omitempty"`
	Vendor     string            `json:"vendor,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty"`
	LastSeenAt *time.Time        `json:"last_seen_at,omitempty"`
	CreatedAt  time.Time         `json:"created_at"`
	UpdatedAt  time.Time         `json:"updated_at"`
}

type AddressStatus string

const (
	StatusReserved   AddressStatus = "reserved"
	StatusAssigned   AddressStatus = "assigned"
	StatusDiscovered AddressStatus = "discovered"
)

type NetworkInput struct {
	Name        string            `json:"name"`
	CIDR        string            `json:"cidr"`
	Description string            `json:"description"`
	Tags        map[string]string `json:"tags"`
}

type AddressInput struct {
	NetworkID string            `json:"network_id"`
	IP        string            `json:"ip"`
	Hostname  string            `json:"hostname"`
	Status    AddressStatus     `json:"status"`
	MAC       string            `json:"mac"`
	Vendor    string            `json:"vendor"`
	Metadata  map[string]string `json:"metadata"`
}

type AllocationInput struct {
	NetworkID string            `json:"network_id"`
	Hostname  string            `json:"hostname"`
	Status    AddressStatus     `json:"status"`
	Metadata  map[string]string `json:"metadata"`
}

type Snapshot struct {
	Networks  []Network `json:"networks"`
	Addresses []Address `json:"addresses"`
}
