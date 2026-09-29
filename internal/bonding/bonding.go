// bonding/bonding.go: V1 link aggregation
// (bonding) model. A Bond aggregates multiple
// member links into one logical link. The
// model covers the V1 modes: active-backup
// and 802.3ad (LACP). The runtime is a thin
// mapping to `ip link add type bond` and
// `ip link set ... master bond0`.
package bonding

import (
	"errors"
	"regexp"
	"sort"
	"sync"
)

// Mode is the bonding mode.
type Mode string

const (
	ModeActiveBackup Mode = "active-backup"
	ModeLACP         Mode = "802.3ad"
)

// Valid reports whether m is a known mode.
func (m Mode) Valid() bool {
	switch m {
	case ModeActiveBackup, ModeLACP:
		return true
	}
	return false
}

// HashMode is the LACP hash policy.
type HashMode string

const (
	HashLayer2  HashMode = "layer2"
	HashLayer23 HashMode = "layer2+3"
	HashLayer34 HashMode = "layer3+4"
	HashEncap23 HashMode = "encap2+3"
	HashEncap34 HashMode = "encap3+4"
)

// Valid reports whether h is known.
func (h HashMode) Valid() bool {
	switch h {
	case HashLayer2, HashLayer23, HashLayer34, HashEncap23, HashEncap34:
		return true
	}
	return false
}

// Member is a single bond member.
type Member struct {
	Link     string `json:"link"`
	Priority int    `json:"priority"`
	Active   bool   `json:"active"`
	// LacpPort is the LACP port number; only
	// meaningful for ModeLACP.
	LacpPort int `json:"lacpPort,omitempty"`
	// SpeedMbps is the observed speed; the
	// runtime reports it.
	SpeedMbps int `json:"speedMbps,omitempty"`
}

// Bond is the aggregation.
type Bond struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Mode       Mode     `json:"mode"`
	HashMode   HashMode `json:"hashMode,omitempty"`
	MIIMonitor int      `json:"miiMonitorMs"`
	Members    []Member `json:"members"`
	// MTU is the per-bond MTU. Zero means
	// "kernel default".
	MTU int `json:"mtu,omitempty"`
	// XmitHashPolicy mirrors the LACP hashing
	// policy; for active-backup it is unused.
	XmitHashPolicy string `json:"xmitHashPolicy,omitempty"`
	// ActiveMember is the link currently
	// active for active-backup.
	ActiveMember string `json:"activeMember,omitempty"`
}

var idPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:-]{0,63}$`)
var namePattern = regexp.MustCompile(`^[a-z][a-z0-9._-]{0,15}$`)
var linkPattern = regexp.MustCompile(`^[a-z][a-z0-9._-]{0,15}$`)

// Valid reports whether b is well-formed.
func (b Bond) Valid() error {
	if !idPattern.MatchString(b.ID) {
		return errors.New("bonding: id is not canonical")
	}
	if !namePattern.MatchString(b.Name) {
		return errors.New("bonding: name is not canonical")
	}
	if !b.Mode.Valid() {
		return errors.New("bonding: mode is unknown")
	}
	if b.MIIMonitor < 0 {
		return errors.New("bonding: MII monitor must be non-negative")
	}
	if b.Mode == ModeLACP && !b.HashMode.Valid() {
		return errors.New("bonding: LACP requires a hash mode")
	}
	if len(b.Members) < 2 {
		return errors.New("bonding: a bond needs at least two members")
	}
	for _, m := range b.Members {
		if !linkPattern.MatchString(m.Link) {
			return errors.New("bonding: member link is not canonical")
		}
		if m.Priority < 0 {
			return errors.New("bonding: priority must be non-negative")
		}
	}
	return nil
}

// Manager is the V1 bond store. Manager is
// goroutine-safe.
type Manager struct {
	mu    sync.Mutex
	bonds map[string]Bond
}

// NewManager returns an empty manager.
func NewManager() *Manager {
	return &Manager{bonds: map[string]Bond{}}
}

// Set replaces or inserts a bond.
func (m *Manager) Set(b Bond) error {
	if err := b.Valid(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.bonds[b.ID] = b
	return nil
}

// Get returns a bond by id.
func (m *Manager) Get(id string) (Bond, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.bonds[id]
	return b, ok
}

// List returns the bonds sorted by id.
func (m *Manager) List() []Bond {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Bond, 0, len(m.bonds))
	for _, b := range m.bonds {
		out = append(out, b)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Delete removes a bond.
func (m *Manager) Delete(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.bonds, id)
}
