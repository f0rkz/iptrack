package discovery

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"sort"
	"sync"
	"time"

	"github.com/f0rkz/iptrack/internal/ipam"
)

type JobStatus string

const (
	JobQueued   JobStatus = "queued"
	JobRunning  JobStatus = "running"
	JobComplete JobStatus = "complete"
	JobFailed   JobStatus = "failed"
)

type Job struct {
	ID          string     `json:"id"`
	NetworkID   string     `json:"network_id"`
	Status      JobStatus  `json:"status"`
	Scanned     int        `json:"scanned"`
	Reachable   int        `json:"reachable"`
	Error       string     `json:"error,omitempty"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

type Request struct {
	NetworkID string `json:"network_id"`
	Ports     []int  `json:"ports,omitempty"`
	TimeoutMS int    `json:"timeout_ms,omitempty"`
}

type Manager struct {
	mu       sync.RWMutex
	jobs     map[string]Job
	store    ipam.Repository
	scanner  Scanner
	maxHosts int
}

func NewManager(store ipam.Repository, workers, maxHosts int) *Manager {
	return &Manager{jobs: map[string]Job{}, store: store, scanner: Scanner{Workers: workers}, maxHosts: maxHosts}
}

func (m *Manager) Start(req Request) (Job, error) {
	network, err := m.store.Network(req.NetworkID)
	if err != nil {
		return Job{}, err
	}
	prefix, _ := netip.ParsePrefix(network.CIDR)
	if _, err := Hosts(prefix, m.maxHosts); err != nil {
		return Job{}, fmt.Errorf("%w: %v", ipam.ErrInvalid, err)
	}
	if len(req.Ports) == 0 {
		req.Ports = []int{22, 80, 443, 445, 3389}
	}
	if len(req.Ports) > 32 {
		return Job{}, fmt.Errorf("%w: at most 32 ports may be probed", ipam.ErrInvalid)
	}
	for _, p := range req.Ports {
		if p < 1 || p > 65535 {
			return Job{}, fmt.Errorf("%w: invalid TCP port %d", ipam.ErrInvalid, p)
		}
	}
	if req.TimeoutMS == 0 {
		req.TimeoutMS = 300
	}
	if req.TimeoutMS < 50 || req.TimeoutMS > 5000 {
		return Job{}, fmt.Errorf("%w: timeout_ms must be between 50 and 5000", ipam.ErrInvalid)
	}
	now := time.Now().UTC()
	job := Job{ID: fmt.Sprintf("scan_%d", now.UnixNano()), NetworkID: req.NetworkID, Status: JobQueued, CreatedAt: now}
	m.mu.Lock()
	m.jobs[job.ID] = job
	m.mu.Unlock()
	go m.run(job.ID, prefix, req.Ports, time.Duration(req.TimeoutMS)*time.Millisecond)
	return job, nil
}

func (m *Manager) Get(id string) (Job, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	j, ok := m.jobs[id]
	if !ok {
		return Job{}, ipam.ErrNotFound
	}
	return j, nil
}
func (m *Manager) List() []Job {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Job, 0, len(m.jobs))
	for _, j := range m.jobs {
		out = append(out, j)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

func (m *Manager) run(id string, prefix netip.Prefix, ports []int, timeout time.Duration) {
	now := time.Now().UTC()
	m.change(id, func(j *Job) { j.Status = JobRunning; j.StartedAt = &now })
	err := m.scanner.Scan(context.Background(), prefix, ports, timeout, m.maxHosts, func(result Result) {
		m.change(id, func(j *Job) {
			j.Scanned++
			if result.Reachable {
				j.Reachable++
			}
		})
		if result.Reachable {
			_, _ = m.store.RecordDiscovery(m.networkID(id), result.IP, result.Hostname, result.MAC, result.Metadata)
		}
	})
	done := time.Now().UTC()
	m.change(id, func(j *Job) {
		j.CompletedAt = &done
		j.Status = JobComplete
		if err != nil && !errors.Is(err, context.Canceled) {
			j.Status = JobFailed
			j.Error = err.Error()
		}
	})
}

func (m *Manager) networkID(id string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.jobs[id].NetworkID
}
func (m *Manager) change(id string, fn func(*Job)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	j := m.jobs[id]
	fn(&j)
	m.jobs[id] = j
}
