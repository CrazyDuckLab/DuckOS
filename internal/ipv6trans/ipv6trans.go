// ipv6trans/ipv6trans.go: V1 IPv6
// transition + CGNAT surface. Package
// owns typed contracts for 464XLAT,
// DS-Lite, 6rd, MAP-E, MAP-T, NPTv6,
// and the CGNAT pool / port-block /
// subscriber-mapping surface.
//
// The runtime is capability-aware: a
// feature is reported supported only when
// the kernel / userspace backend exposes
// it.
package ipv6trans

import (
	"errors"
	"regexp"
	"sort"
	"sync"
)

// TransitionKind is the transition
// technology.
type TransitionKind string

const (
	Trans464XLAT TransitionKind = "464xlat"
	TransDSLite  TransitionKind = "ds-lite"
	Trans6rd     TransitionKind = "6rd"
	TransMAPE    TransitionKind = "map-e"
	TransMAPT    TransitionKind = "map-t"
	TransNPTv6   TransitionKind = "nptv6"
)

// Valid reports whether k is known.
func (k TransitionKind) Valid() bool {
	switch k {
	case Trans464XLAT, TransDSLite, Trans6rd, TransMAPE,
		TransMAPT, TransNPTv6:
		return true
	}
	return false
}

// Capability is the kernel/userspace
// capability gate.
type Capability struct {
	Transitions []TransitionKind `json:"transitions"`
}

// Supports reports whether k is in the
// capability set.
func (c Capability) Supports(k TransitionKind) bool {
	for _, x := range c.Transitions {
		if x == k {
			return true
		}
	}
	return false
}

// Prefix is the provider prefix for 6rd /
// MAP-E / MAP-T.
type Prefix struct {
	Kind    TransitionKind `json:"kind"`
	Prefix  string         `json:"prefix"`
	BR      string         `json:"br,omitempty"`   // 6rd BR address
	PSID    int            `json:"psid,omitempty"` // MAP-E PSID
	PSIDLen int            `json:"psidLen,omitempty"`
	EA      int            `json:"eaBits,omitempty"` // MAP-E EA bits
	Rule    string         `json:"rule,omitempty"`   // MAP-T rule
}

var prefixPattern = regexp.MustCompile(`^[0-9a-fA-F:./]+$`)

// Valid reports whether p is well-formed.
func (p Prefix) Valid() error {
	if !p.Kind.Valid() {
		return errors.New("ipv6trans: kind is unknown")
	}
	if p.Prefix == "" {
		return errors.New("ipv6trans: prefix is required")
	}
	if !prefixPattern.MatchString(p.Prefix) {
		return errors.New("ipv6trans: prefix is not canonical")
	}
	return nil
}

// CGNATPool is a CGNAT public address pool.
type CGNATPool struct {
	Name      string   `json:"name"`
	Addresses []string `json:"addresses"`
	// PortBlockSize is the per-subscriber
	// port-block size.
	PortBlockSize int `json:"portBlockSize"`
	// Deterministic toggles RFC 7422
	// deterministic NAT.
	Deterministic bool `json:"deterministic,omitempty"`
	// PerSubscriberLimit is the per-
	// subscriber session cap.
	PerSubscriberLimit int `json:"perSubscriberLimit,omitempty"`
}

// Valid reports whether p is well-formed.
func (p CGNATPool) Valid() error {
	if p.Name == "" {
		return errors.New("ipv6trans: pool name is required")
	}
	if len(p.Addresses) == 0 {
		return errors.New("ipv6trans: pool addresses are required")
	}
	for _, a := range p.Addresses {
		if a == "" {
			return errors.New("ipv6trans: empty address")
		}
	}
	if p.PortBlockSize < 0 || p.PortBlockSize > 4096 {
		return errors.New("ipv6trans: portBlockSize out of range")
	}
	return nil
}

// ExhaustionState is the CGNAT exhaustion
// state.
type ExhaustionState struct {
	AddressExhausted bool `json:"addressExhausted"`
	PortExhausted    bool `json:"portExhausted"`
	FreeAddresses    int  `json:"freeAddresses"`
	FreePortBlocks   int  `json:"freePortBlocks"`
	Subscribers      int  `json:"subscribers"`
}

// CGNATManager is the CGNAT store.
type CGNATManager struct {
	mu    sync.Mutex
	pools map[string]CGNATPool
	exh   ExhaustionState
}

// NewCGNATManager returns an empty manager.
func NewCGNATManager() *CGNATManager {
	return &CGNATManager{pools: map[string]CGNATPool{}}
}

// AddPool inserts a pool.
func (m *CGNATManager) AddPool(p CGNATPool) error {
	if err := p.Valid(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pools[p.Name] = p
	return nil
}

// GetPool returns a pool.
func (m *CGNATManager) GetPool(name string) (CGNATPool, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.pools[name]
	return p, ok
}

// SetExhaustion records the runtime
// exhaustion state.
func (m *CGNATManager) SetExhaustion(e ExhaustionState) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.exh = e
}

// Exhaustion returns the recorded
// exhaustion state.
func (m *CGNATManager) Exhaustion() ExhaustionState {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.exh
}

// TransitionManager is the IPv6 transition
// store.
type TransitionManager struct {
	mu       sync.Mutex
	prefixes map[string]Prefix
	cap      Capability
}

// NewTransitionManager returns an empty
// manager.
func NewTransitionManager() *TransitionManager {
	return &TransitionManager{prefixes: map[string]Prefix{}}
}

// SetCapability records the platform
// capability.
func (m *TransitionManager) SetCapability(c Capability) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cap = c
}

// AddPrefix inserts a prefix.
func (m *TransitionManager) AddPrefix(p Prefix) error {
	if err := p.Valid(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.prefixes[p.Prefix] = p
	return nil
}

// Prefixes returns a sorted copy.
func (m *TransitionManager) Prefixes() []Prefix {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Prefix, 0, len(m.prefixes))
	for _, p := range m.prefixes {
		out = append(out, p)
	}
	sort.Slice(out, func(i, k int) bool { return out[i].Prefix < out[k].Prefix })
	return out
}
