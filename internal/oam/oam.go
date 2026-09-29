// oam/oam.go: V1 carrier-Ethernet OAM
// and ring-protection surface. Package
// owns typed contracts for IEEE 802.1ag
// CFM, Y.1731-style OAM primitives, and
// ERPS / MRP ring state. The runtime is a
// thin mapping to the platform OAM agent.
package oam

import (
	"errors"
	"regexp"
	"sort"
	"sync"
	"time"
)

// CFMMD is the CFM Maintenance Domain.
type CFMMD struct {
	Name    string `json:"name"`
	Level   int    `json:"level"` // 0..7
	MDIndex int    `json:"mdIndex"`
}

// Valid reports whether m is well-formed.
func (m CFMMD) Valid() error {
	if m.Name == "" {
		return errors.New("oam: name is required")
	}
	if m.Level < 0 || m.Level > 7 {
		return errors.New("oam: level out of range")
	}
	return nil
}

// CFMMA is the CFM Maintenance Association.
type CFMMA struct {
	Name    string `json:"name"`
	MDName  string `json:"mdName"`
	MAIndex int    `json:"maIndex"`
	// CCMInterval is the continuity-check
	// message interval.
	CCMInterval time.Duration `json:"ccmInterval"`
}

// Valid reports whether a is well-formed.
func (a CFMMA) Valid() error {
	if a.Name == "" {
		return errors.New("oam: MA name is required")
	}
	if a.CCMInterval < 0 {
		return errors.New("oam: CCM interval must be non-negative")
	}
	return nil
}

// CFMState is the CFM MEP / MIC state.
type CFMState struct {
	MEP       string    `json:"mep"`
	MDName    string    `json:"mdName"`
	MAName    string    `json:"maName"`
	State     string    `json:"state"` // up / down / unknown
	LastRDI   bool      `json:"lastRdi"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// OAMState is the global OAM state.
type OAMState struct {
	CFMMEs []CFMState `json:"cfmMeps,omitempty"`
}

// Manager is the OAM state store.
type Manager struct {
	mu    sync.Mutex
	mds   map[string]CFMMD
	mas   map[string]CFMMA
	cfm   []CFMState
	clock func() time.Time
}

// NewManager returns an empty manager.
func NewManager(clock func() time.Time) *Manager {
	if clock == nil {
		clock = time.Now
	}
	return &Manager{mds: map[string]CFMMD{}, mas: map[string]CFMMA{}, clock: clock}
}

// SetMD replaces a maintenance domain.
func (m *Manager) SetMD(d CFMMD) error {
	if err := d.Valid(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.mds[d.Name] = d
	return nil
}

// SetMA replaces a maintenance association.
func (m *Manager) SetMA(a CFMMA) error {
	if err := a.Valid(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.mas[a.Name] = a
	return nil
}

// RecordCFM appends a CFM state.
func (m *Manager) RecordCFM(c CFMState) {
	if c.MEP == "" {
		return
	}
	if c.UpdatedAt.IsZero() {
		c.UpdatedAt = m.clock().UTC()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	// Replace by MEP id.
	for i, x := range m.cfm {
		if x.MEP == c.MEP {
			m.cfm[i] = c
			return
		}
	}
	m.cfm = append(m.cfm, c)
}

// CFMAll returns every CFM state, sorted
// by MEP id.
func (m *Manager) CFMAll() []CFMState {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := append([]CFMState(nil), m.cfm...)
	sort.Slice(out, func(i, k int) bool { return out[i].MEP < out[k].MEP })
	return out
}

// RingState is the ERPS / MRP ring state.
type RingState struct {
	RingID      string    `json:"ringId"`
	State       string    `json:"state"` // "idle" | "protected" | "force-switch" | "manual-switch"
	RPLOwner    bool      `json:"rplOwner"`
	RPLPort     string    `json:"rplPort,omitempty"`
	BlockedPort string    `json:"blockedPort,omitempty"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

var idPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:-]{0,63}$`)

// Valid reports whether r is well-formed.
func (r RingState) Valid() error {
	if !idPattern.MatchString(r.RingID) {
		return errors.New("oam: ringId is not canonical")
	}
	switch r.State {
	case "idle", "protected", "force-switch", "manual-switch":
	default:
		return errors.New("oam: ring state is invalid")
	}
	return nil
}

// RingManager is the ring state store.
type RingManager struct {
	mu    sync.Mutex
	rings map[string]RingState
	clock func() time.Time
}

// NewRingManager returns an empty manager.
func NewRingManager(clock func() time.Time) *RingManager {
	if clock == nil {
		clock = time.Now
	}
	return &RingManager{rings: map[string]RingState{}, clock: clock}
}

// Set replaces a ring state.
func (m *RingManager) Set(r RingState) error {
	if err := r.Valid(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if r.UpdatedAt.IsZero() {
		r.UpdatedAt = m.clock().UTC()
	}
	m.rings[r.RingID] = r
	return nil
}

// Get returns a ring state.
func (m *RingManager) Get(id string) (RingState, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.rings[id]
	return r, ok
}

// All returns every ring state sorted.
func (m *RingManager) All() []RingState {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]RingState, 0, len(m.rings))
	for _, r := range m.rings {
		out = append(out, r)
	}
	sort.Slice(out, func(i, k int) bool { return out[i].RingID < out[k].RingID })
	return out
}
