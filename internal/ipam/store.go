package ipam

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

var (
	ErrNotFound = errors.New("not found")
	ErrConflict = errors.New("conflict")
	ErrInvalid  = errors.New("invalid input")
)

type Store struct {
	mu   sync.RWMutex
	path string
	data Snapshot
}

func (s *Store) Health() error { return nil }

func Open(path string) (*Store, error) {
	s := &Store{path: path, data: Snapshot{Networks: []Network{}, Addresses: []Address{}}}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &s.data); err != nil {
		return nil, fmt.Errorf("decode %s: %w", path, err)
	}
	return s, nil
}

func (s *Store) Snapshot() (Snapshot, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneSnapshot(s.data), nil
}

func (s *Store) Networks() ([]Network, error) {
	snapshot, _ := s.Snapshot()
	data := snapshot.Networks
	sort.Slice(data, func(i, j int) bool { return data[i].Name < data[j].Name })
	return data, nil
}

func (s *Store) Network(id string) (Network, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, n := range s.data.Networks {
		if n.ID == id {
			return cloneNetwork(n), nil
		}
	}
	return Network{}, ErrNotFound
}

func (s *Store) CreateNetwork(in NetworkInput) (Network, error) {
	in.Name = strings.TrimSpace(in.Name)
	prefix, err := parsePrefix(in.CIDR)
	if err != nil || in.Name == "" {
		return Network{}, fmt.Errorf("%w: name and a valid CIDR are required", ErrInvalid)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, n := range s.data.Networks {
		existing, _ := netip.ParsePrefix(n.CIDR)
		if n.Name == in.Name || prefixesOverlap(existing, prefix) {
			return Network{}, fmt.Errorf("%w: network name or CIDR overlaps an existing network", ErrConflict)
		}
	}
	now := time.Now().UTC()
	n := Network{ID: newID("net"), Name: in.Name, CIDR: prefix.String(), Description: strings.TrimSpace(in.Description), Tags: cloneMap(in.Tags), CreatedAt: now, UpdatedAt: now}
	s.data.Networks = append(s.data.Networks, n)
	if err := s.saveLocked(); err != nil {
		s.data.Networks = s.data.Networks[:len(s.data.Networks)-1]
		return Network{}, err
	}
	return cloneNetwork(n), nil
}

func (s *Store) UpdateNetwork(id string, in NetworkInput) (Network, error) {
	in.Name = strings.TrimSpace(in.Name)
	prefix, err := parsePrefix(in.CIDR)
	if err != nil || in.Name == "" {
		return Network{}, fmt.Errorf("%w: name and a valid CIDR are required", ErrInvalid)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	idx := -1
	for i, n := range s.data.Networks {
		if n.ID == id {
			idx = i
			continue
		}
		existing, _ := netip.ParsePrefix(n.CIDR)
		if n.Name == in.Name || prefixesOverlap(existing, prefix) {
			return Network{}, fmt.Errorf("%w: network name or CIDR overlaps an existing network", ErrConflict)
		}
	}
	if idx < 0 {
		return Network{}, ErrNotFound
	}
	for _, a := range s.data.Addresses {
		ip, _ := netip.ParseAddr(a.IP)
		if a.NetworkID == id && !prefix.Contains(ip) {
			return Network{}, fmt.Errorf("%w: new CIDR excludes existing address %s", ErrConflict, a.IP)
		}
	}
	n := s.data.Networks[idx]
	n.Name, n.CIDR, n.Description, n.Tags, n.UpdatedAt = in.Name, prefix.String(), strings.TrimSpace(in.Description), cloneMap(in.Tags), time.Now().UTC()
	s.data.Networks[idx] = n
	if err := s.saveLocked(); err != nil {
		return Network{}, err
	}
	return cloneNetwork(n), nil
}

func (s *Store) DeleteNetwork(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, a := range s.data.Addresses {
		if a.NetworkID == id {
			return fmt.Errorf("%w: network still contains addresses", ErrConflict)
		}
	}
	for i := range s.data.Networks {
		if s.data.Networks[i].ID == id {
			s.data.Networks = append(s.data.Networks[:i], s.data.Networks[i+1:]...)
			return s.saveLocked()
		}
	}
	return ErrNotFound
}

func (s *Store) Addresses(networkID string) ([]Address, error) {
	snapshot, _ := s.Snapshot()
	all := snapshot.Addresses
	out := all[:0]
	for _, a := range all {
		if networkID == "" || a.NetworkID == networkID {
			out = append(out, a)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		a, _ := netip.ParseAddr(out[i].IP)
		b, _ := netip.ParseAddr(out[j].IP)
		return a.Less(b)
	})
	return out, nil
}

func (s *Store) Address(id string) (Address, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, a := range s.data.Addresses {
		if a.ID == id {
			return cloneAddress(a), nil
		}
	}
	return Address{}, ErrNotFound
}

func (s *Store) CreateAddress(in AddressInput) (Address, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.createAddressLocked(in)
}

func (s *Store) createAddressLocked(in AddressInput) (Address, error) {
	network, prefix, err := s.networkPrefixLocked(in.NetworkID)
	if err != nil {
		return Address{}, err
	}
	ip, err := netip.ParseAddr(strings.TrimSpace(in.IP))
	if err != nil || !prefix.Contains(ip) || ip.IsUnspecified() || ip.IsMulticast() {
		return Address{}, fmt.Errorf("%w: IP must be a usable address in %s", ErrInvalid, network.CIDR)
	}
	if isBoundary(prefix, ip) {
		return Address{}, fmt.Errorf("%w: network and broadcast addresses cannot be allocated", ErrInvalid)
	}
	for _, a := range s.data.Addresses {
		if a.IP == ip.String() {
			return Address{}, fmt.Errorf("%w: address is already tracked", ErrConflict)
		}
	}
	if in.Status == "" {
		in.Status = StatusAssigned
	}
	if !validStatus(in.Status) {
		return Address{}, fmt.Errorf("%w: invalid address status", ErrInvalid)
	}
	now := time.Now().UTC()
	a := Address{ID: newID("ip"), NetworkID: in.NetworkID, IP: ip.String(), Hostname: strings.TrimSpace(in.Hostname), Status: in.Status, MAC: strings.ToLower(strings.TrimSpace(in.MAC)), Vendor: strings.TrimSpace(in.Vendor), Metadata: cloneMap(in.Metadata), CreatedAt: now, UpdatedAt: now}
	if in.Status == StatusDiscovered {
		a.LastSeenAt = &now
	}
	s.data.Addresses = append(s.data.Addresses, a)
	if err := s.saveLocked(); err != nil {
		s.data.Addresses = s.data.Addresses[:len(s.data.Addresses)-1]
		return Address{}, err
	}
	return cloneAddress(a), nil
}

func (s *Store) UpdateAddress(id string, in AddressInput) (Address, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx := -1
	for i, a := range s.data.Addresses {
		if a.ID == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		return Address{}, ErrNotFound
	}
	if in.NetworkID == "" {
		in.NetworkID = s.data.Addresses[idx].NetworkID
	}
	_, prefix, err := s.networkPrefixLocked(in.NetworkID)
	if err != nil {
		return Address{}, err
	}
	ip, err := netip.ParseAddr(strings.TrimSpace(in.IP))
	if err != nil || !prefix.Contains(ip) || ip.IsUnspecified() || ip.IsMulticast() || isBoundary(prefix, ip) {
		return Address{}, fmt.Errorf("%w: IP is outside the network or reserved", ErrInvalid)
	}
	for i, a := range s.data.Addresses {
		if i != idx && a.IP == ip.String() {
			return Address{}, fmt.Errorf("%w: address is already tracked", ErrConflict)
		}
	}
	if in.Status == "" {
		in.Status = StatusAssigned
	}
	if !validStatus(in.Status) {
		return Address{}, fmt.Errorf("%w: invalid address status", ErrInvalid)
	}
	a := s.data.Addresses[idx]
	a.NetworkID, a.IP, a.Hostname, a.Status, a.MAC, a.Vendor, a.Metadata, a.UpdatedAt = in.NetworkID, ip.String(), strings.TrimSpace(in.Hostname), in.Status, strings.ToLower(strings.TrimSpace(in.MAC)), strings.TrimSpace(in.Vendor), cloneMap(in.Metadata), time.Now().UTC()
	s.data.Addresses[idx] = a
	if err := s.saveLocked(); err != nil {
		return Address{}, err
	}
	return cloneAddress(a), nil
}

func (s *Store) DeleteAddress(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.data.Addresses {
		if s.data.Addresses[i].ID == id {
			s.data.Addresses = append(s.data.Addresses[:i], s.data.Addresses[i+1:]...)
			return s.saveLocked()
		}
	}
	return ErrNotFound
}

func (s *Store) Allocate(in AllocationInput) (Address, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, prefix, err := s.networkPrefixLocked(in.NetworkID)
	if err != nil {
		return Address{}, err
	}
	used := map[netip.Addr]bool{}
	for _, a := range s.data.Addresses {
		ip, _ := netip.ParseAddr(a.IP)
		used[ip] = true
	}
	for ip := prefix.Addr().Next(); ip.IsValid() && prefix.Contains(ip); ip = ip.Next() {
		if isBoundary(prefix, ip) {
			continue
		}
		if !used[ip] {
			return s.createAddressLocked(AddressInput{NetworkID: in.NetworkID, IP: ip.String(), Hostname: in.Hostname, Status: in.Status, Metadata: in.Metadata})
		}
	}
	return Address{}, fmt.Errorf("%w: network has no available addresses", ErrConflict)
}

// RecordDiscovery creates an address or refreshes metadata for an existing one.
func (s *Store) RecordDiscovery(networkID, ip, hostname, mac string, metadata map[string]string) (Address, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	for i, a := range s.data.Addresses {
		if a.NetworkID == networkID && a.IP == ip {
			if hostname != "" {
				a.Hostname = hostname
			}
			if mac != "" {
				a.MAC = strings.ToLower(mac)
			}
			if a.Metadata == nil {
				a.Metadata = map[string]string{}
			}
			for k, v := range metadata {
				a.Metadata[k] = v
			}
			a.LastSeenAt, a.UpdatedAt = &now, now
			s.data.Addresses[i] = a
			if err := s.saveLocked(); err != nil {
				return Address{}, err
			}
			return cloneAddress(a), nil
		}
	}
	return s.createAddressLocked(AddressInput{NetworkID: networkID, IP: ip, Hostname: hostname, Status: StatusDiscovered, MAC: mac, Metadata: metadata})
}

func (s *Store) networkPrefixLocked(id string) (Network, netip.Prefix, error) {
	for _, n := range s.data.Networks {
		if n.ID == id {
			p, _ := netip.ParsePrefix(n.CIDR)
			return n, p, nil
		}
	}
	return Network{}, netip.Prefix{}, ErrNotFound
}

func (s *Store) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".iptrack-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, err = tmp.Write(b); err == nil {
		err = tmp.Sync()
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(name, s.path)
}

func parsePrefix(raw string) (netip.Prefix, error) {
	p, err := netip.ParsePrefix(strings.TrimSpace(raw))
	if err != nil {
		return p, err
	}
	return p.Masked(), nil
}
func prefixesOverlap(a, b netip.Prefix) bool { return a.Contains(b.Addr()) || b.Contains(a.Addr()) }
func validStatus(s AddressStatus) bool {
	return s == StatusReserved || s == StatusAssigned || s == StatusDiscovered
}
func isBoundary(p netip.Prefix, ip netip.Addr) bool {
	if ip == p.Addr() {
		return true
	}
	// IPv4's final address is the broadcast address. IPv6 has no broadcast.
	if ip.Is4() {
		next := ip.Next()
		return !next.IsValid() || !p.Contains(next)
	}
	return false
}
func newID(prefix string) string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return prefix + "_" + hex.EncodeToString(b)
}
func cloneMap(m map[string]string) map[string]string {
	if m == nil {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
func cloneNetwork(n Network) Network { n.Tags = cloneMap(n.Tags); return n }
func cloneAddress(a Address) Address { a.Metadata = cloneMap(a.Metadata); return a }
func cloneSnapshot(s Snapshot) Snapshot {
	out := Snapshot{Networks: make([]Network, len(s.Networks)), Addresses: make([]Address, len(s.Addresses))}
	for i, n := range s.Networks {
		out.Networks[i] = cloneNetwork(n)
	}
	for i, a := range s.Addresses {
		out.Addresses[i] = cloneAddress(a)
	}
	return out
}
