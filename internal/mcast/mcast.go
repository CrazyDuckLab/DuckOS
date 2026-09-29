// mcast/mcast.go: V1 multicast / service-
// discovery model. The package owns the typed
// contracts for IGMP/MLD snooping, IGMP proxy,
// mDNS/Bonjour reflector, and the
// per-network reflector policy.
//
// The runtime is a thin mapping to
// `bridge mdb` and the avahi reflector; the
// model is the policy the future UI binds to
// and the diagnostics consult.
package mcast

import (
	"errors"
	"regexp"
	"sort"
	"sync"
)

// Mode is the multicast mode for a network.
type Mode string

const (
	ModeSnooping Mode = "snooping"
	ModeProxy    Mode = "proxy"
	ModeOff      Mode = "off"
)

// Valid reports whether m is known.
func (m Mode) Valid() bool {
	switch m {
	case ModeSnooping, ModeProxy, ModeOff:
		return true
	}
	return false
}

// Network is a multicast-enabled network.
type Network struct {
	ID     string `json:"id"`
	Bridge string `json:"bridge"`
	Mode   Mode   `json:"mode"`
	// UpstreamIf is the upstream interface
	// for IGMP proxy. Required when
	// Mode == ModeProxy.
	UpstreamIf string `json:"upstreamIf,omitempty"`
	// Querier is the canonical querier
	// address (often the bridge's link-local
	// IPv6 address).
	Querier string `json:"querier,omitempty"`
}

var idPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:-]{0,63}$`)

// Valid reports whether n is well-formed.
func (n Network) Valid() error {
	if !idPattern.MatchString(n.ID) {
		return errors.New("mcast: id is not canonical")
	}
	if !n.Mode.Valid() {
		return errors.New("mcast: mode is unknown")
	}
	if n.Mode == ModeProxy && n.UpstreamIf == "" {
		return errors.New("mcast: proxy mode requires upstreamIf")
	}
	return nil
}

// MDNSReflectorPolicy is the mDNS/Bonjour
// reflector policy. The V1 contract is
// strict: the reflector is off by default and
// only between explicitly selected networks.
type MDNSReflectorPolicy struct {
	Enabled     bool     `json:"enabled"`
	FromNetwork string   `json:"fromNetwork"`
	ToNetworks  []string `json:"toNetworks"`
	// AllowedServiceTypes is the allow-list of
	// service types (e.g. "_airplay._tcp",
	// "_ipp._tcp", "_googlecast._tcp"). An
	// empty allow-list means all services
	// pass; the V1 default is the explicit
	// list of common service types.
	AllowedServiceTypes []string `json:"allowedServiceTypes,omitempty"`
}

// Valid reports whether p is well-formed.
func (p MDNSReflectorPolicy) Valid() error {
	if !p.Enabled {
		return nil
	}
	if !idPattern.MatchString(p.FromNetwork) {
		return errors.New("mcast: fromNetwork is not canonical")
	}
	for _, t := range p.ToNetworks {
		if !idPattern.MatchString(t) {
			return errors.New("mcast: toNetwork is not canonical")
		}
	}
	return nil
}

// Manager is the V1 multicast store. Manager
// is goroutine-safe.
type Manager struct {
	mu       sync.Mutex
	networks map[string]Network
	mdns     map[string]MDNSReflectorPolicy // keyed by FromNetwork
}

// NewManager returns an empty manager.
func NewManager() *Manager {
	return &Manager{networks: map[string]Network{}, mdns: map[string]MDNSReflectorPolicy{}}
}

// SetNetwork replaces or inserts a network.
func (m *Manager) SetNetwork(n Network) error {
	if err := n.Valid(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.networks[n.ID] = n
	return nil
}

// SetMDNS replaces or inserts an mDNS
// reflector policy.
func (m *Manager) SetMDNS(p MDNSReflectorPolicy) error {
	if err := p.Valid(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.mdns[p.FromNetwork] = p
	return nil
}

// Networks returns the networks sorted by id.
func (m *Manager) Networks() []Network {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Network, 0, len(m.networks))
	for _, n := range m.networks {
		out = append(out, n)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// MDNS returns the mDNS reflector policies
// sorted by fromNetwork.
func (m *Manager) MDNS() []MDNSReflectorPolicy {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]MDNSReflectorPolicy, 0, len(m.mdns))
	for _, p := range m.mdns {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].FromNetwork < out[j].FromNetwork })
	return out
}

// CanReflect reports whether the reflector
// may forward a packet from the given source
// to the given destination network. The
// default is "no global reflection".
func (m *Manager) CanReflect(from, to string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.mdns[from]
	if !ok || !p.Enabled {
		return false
	}
	for _, t := range p.ToNetworks {
		if t == to {
			return true
		}
	}
	return false
}
