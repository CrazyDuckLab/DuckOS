// macsec/macsec.go: V1 MACsec surface.
// Package owns typed contracts for MACsec
// associations, keys (vault-only), SA
// state, capability gate, and rotation.
package macsec

import (
	"errors"
	"regexp"
	"sort"
	"sync"
	"time"
)

// Capability is the platform MACsec gate.
type Capability string

const (
	CapSoftware    Capability = "software"
	CapHardware    Capability = "hardware"
	CapUnsupported Capability = "unsupported"
)

// Valid reports whether c is known.
func (c Capability) Valid() bool {
	switch c {
	case CapSoftware, CapHardware, CapUnsupported:
		return true
	}
	return false
}

// SA is a Secure Association.
type SA struct {
	Interface string `json:"interface"`
	KeyName   string `json:"keyName"` // vault ref
	SCI       string `json:"sci"`
	// KeyServer is the MKA key server id.
	KeyServer uint8     `json:"keyServer"`
	CreatedAt time.Time `json:"createdAt"`
	ExpiresAt time.Time `json:"expiresAt"`
}

var ifnamePattern = regexp.MustCompile(`^[a-z][a-z0-9._-]{0,15}$`)

// Valid reports whether s is well-formed.
func (s SA) Valid() error {
	if !ifnamePattern.MatchString(s.Interface) {
		return errors.New("macsec: interface is not canonical")
	}
	if s.KeyName == "" {
		return errors.New("macsec: keyName (vault ref) is required")
	}
	if !s.ExpiresAt.IsZero() && !s.ExpiresAt.After(s.CreatedAt) {
		return errors.New("macsec: expiresAt must be after createdAt")
	}
	return nil
}

// Manager is the MACsec store.
type Manager struct {
	mu  sync.Mutex
	sas map[string]SA
	cap Capability
}

// NewManager returns an empty manager.
func NewManager() *Manager { return &Manager{sas: map[string]SA{}} }

// SetCapability records the platform
// capability.
func (m *Manager) SetCapability(c Capability) {
	if !c.Valid() {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cap = c
}

// Capability returns the recorded
// capability.
func (m *Manager) Capability() Capability {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cap
}

// AddSA inserts an SA.
func (m *Manager) AddSA(s SA) error {
	if err := s.Valid(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sas[s.Interface] = s
	return nil
}

// RemoveSA drops an SA.
func (m *Manager) RemoveSA(ifname string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sas, ifname)
}

// SAs returns every SA sorted.
func (m *Manager) SAs() []SA {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]SA, 0, len(m.sas))
	for _, s := range m.sas {
		out = append(out, s)
	}
	sort.Slice(out, func(i, k int) bool { return out[i].Interface < out[k].Interface })
	return out
}
