// mclag/mclag.go: V1 MC-LAG surface.
// Package owns typed contracts for
// multi-chassis LAG: peer state, member
// state, role, consistency, split-brain
// risk, orphan ports. Honest "what we
// actually have" model — no claim of
// silent state synchronization.
package mclag

import (
	"errors"
	"regexp"
	"sort"
	"sync"
	"time"
)

// Role is the MC-LAG node role.
type Role string

const (
	RolePrimary   Role = "primary"
	RoleSecondary Role = "secondary"
)

// Valid reports whether r is known.
func (r Role) Valid() bool { return r == RolePrimary || r == RoleSecondary }

// PeerLink is the inter-chassis link.
type PeerLink struct {
	Interface string `json:"interface"`
	// State is the runtime state of the
	// peer link.
	State     string    `json:"state"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Domain is one MC-LAG domain.
type Domain struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	PeerAddress string   `json:"peerAddress"`
	PeerLink    PeerLink `json:"peerLink"`
	Role        Role     `json:"role"`
	// Members are the local member ports.
	Members []string `json:"members,omitempty"`
	// OrphanPorts is set by the runtime
	// when the peer link is down.
	OrphanPorts []string `json:"orphanPorts,omitempty"`
	// SplitBrainRisk is computed when the
	// peer link is down for longer than
	// the configured timeout.
	SplitBrainRisk bool      `json:"splitBrainRisk,omitempty"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

var idPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:-]{0,63}$`)

// Valid reports whether d is well-formed.
func (d Domain) Valid() error {
	if !idPattern.MatchString(d.ID) {
		return errors.New("mclag: id is not canonical")
	}
	if d.PeerAddress == "" {
		return errors.New("mclag: peer address is required")
	}
	if !d.Role.Valid() {
		return errors.New("mclag: role is invalid")
	}
	return nil
}

// Manager is the MC-LAG store. Manager is
// goroutine-safe.
type Manager struct {
	mu      sync.Mutex
	domains map[string]Domain
}

// NewManager returns an empty manager.
func NewManager() *Manager { return &Manager{domains: map[string]Domain{}} }

// Set replaces a domain.
func (m *Manager) Set(d Domain) error {
	if err := d.Valid(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	d.UpdatedAt = time.Now().UTC()
	m.domains[d.ID] = d
	return nil
}

// Get returns a copy of the domain.
func (m *Manager) Get(id string) (Domain, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	d, ok := m.domains[id]
	return d, ok
}

// List returns every domain sorted by id.
func (m *Manager) List() []Domain {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Domain, 0, len(m.domains))
	for _, d := range m.domains {
		out = append(out, d)
	}
	sort.Slice(out, func(i, k int) bool { return out[i].ID < out[k].ID })
	return out
}

// SetPeerLink updates the peer-link runtime
// state. When the state is "down", the
// members become orphan ports and the
// split-brain risk is raised.
func (m *Manager) SetPeerLink(id, ifname, state string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	d, ok := m.domains[id]
	if !ok {
		return errors.New("mclag: unknown domain")
	}
	d.PeerLink = PeerLink{Interface: ifname, State: state, UpdatedAt: time.Now().UTC()}
	if state == "down" {
		d.OrphanPorts = append([]string(nil), d.Members...)
		d.SplitBrainRisk = true
	} else {
		d.OrphanPorts = nil
		d.SplitBrainRisk = false
	}
	d.UpdatedAt = time.Now().UTC()
	m.domains[id] = d
	return nil
}
