// upnp/upnp.go: V1 UPnP/PCP (NAT-PMP) model.
// The V1 contract is strict: dynamic port
// mapping is OFF by default, enabled only on
// selected trusted LAN/VLAN. The package owns
// the typed contracts and the audit
// surface; the runtime is a thin mapping to
// miniupnpd or libpcp.
package upnp

import (
	"errors"
	"regexp"
	"sort"
	"sync"
	"time"
)

// Protocol is the mapping protocol.
type Protocol string

const (
	ProtoPCP    Protocol = "pcp"
	ProtoNATPMP Protocol = "nat-pmp"
	ProtoUPnP   Protocol = "upnp"
)

// Valid reports whether p is known.
func (p Protocol) Valid() bool {
	switch p {
	case ProtoPCP, ProtoNATPMP, ProtoUPnP:
		return true
	}
	return false
}

// Network is a network on which UPnP/PCP is
// enabled. The NetworkRef is a stable pointer
// to the firewall/bridge zone.
type Network struct {
	NetworkRef string     `json:"networkRef"`
	Protocols  []Protocol `json:"protocols"`
	// TTL is the maximum lifetime an external
	// mapping may live.
	TTLSec int `json:"ttlSec"`
	// AllowedClients is the optional
	// allow-list of source addresses.
	AllowedClients []string `json:"allowedClients,omitempty"`
}

var idPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:-]{0,63}$`)

// Valid reports whether n is well-formed.
func (n Network) Valid() error {
	if !idPattern.MatchString(n.NetworkRef) {
		return errors.New("upnp: networkRef is not canonical")
	}
	if len(n.Protocols) == 0 {
		return errors.New("upnp: at least one protocol is required")
	}
	for _, p := range n.Protocols {
		if !p.Valid() {
			return errors.New("upnp: protocol is unknown")
		}
	}
	if n.TTLSec < 0 || n.TTLSec > 86400 {
		return errors.New("upnp: TTL out of range")
	}
	return nil
}

// Mapping is one dynamic port mapping. The
// runtime is the source of truth; the model
// owns the surface and the audit.
type Mapping struct {
	ID           string    `json:"id"`
	NetworkRef   string    `json:"networkRef"`
	Protocol     Protocol  `json:"protocol"`
	ClientIP     string    `json:"clientIp"`
	ExternalPort int       `json:"externalPort"`
	InternalPort int       `json:"internalPort"`
	InternalHost string    `json:"internalHost"`
	ExpiresAt    time.Time `json:"expiresAt"`
	CreatedAt    time.Time `json:"createdAt"`
	Source       string    `json:"source"`
}

// Valid reports whether m is well-formed.
func (m Mapping) Valid() error {
	if m.NetworkRef == "" {
		return errors.New("upnp: networkRef is required")
	}
	if !m.Protocol.Valid() {
		return errors.New("upnp: protocol is unknown")
	}
	if m.ExternalPort < 1 || m.ExternalPort > 65535 {
		return errors.New("upnp: externalPort out of range")
	}
	if m.InternalPort < 1 || m.InternalPort > 65535 {
		return errors.New("upnp: internalPort out of range")
	}
	if m.ClientIP == "" {
		return errors.New("upnp: clientIp is required")
	}
	return nil
}

// Manager is the V1 UPnP/PCP store. Manager is
// goroutine-safe.
type Manager struct {
	mu       sync.Mutex
	networks map[string]Network
	mappings map[string]Mapping
}

// NewManager returns an empty manager.
func NewManager() *Manager {
	return &Manager{networks: map[string]Network{}, mappings: map[string]Mapping{}}
}

// EnableNetwork registers a network for
// UPnP/PCP.
func (m *Manager) EnableNetwork(n Network) error {
	if err := n.Valid(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.networks[n.NetworkRef] = n
	return nil
}

// DisableNetwork removes a network.
func (m *Manager) DisableNetwork(ref string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.networks, ref)
}

// IsEnabled reports whether the network has
// at least one protocol enabled.
func (m *Manager) IsEnabled(ref string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.networks[ref]
	return ok
}

// AddMapping records a dynamic mapping.
func (m *Manager) AddMapping(mp Mapping) error {
	if err := mp.Valid(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.mappings[mp.ID] = mp
	return nil
}

// RevokeMapping removes a mapping.
func (m *Manager) RevokeMapping(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.mappings, id)
}

// Mappings returns the current mappings sorted
// by id.
func (m *Manager) Mappings() []Mapping {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Mapping, 0, len(m.mappings))
	for _, m := range m.mappings {
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Networks returns the enabled networks sorted
// by reference.
func (m *Manager) Networks() []Network {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Network, 0, len(m.networks))
	for _, n := range m.networks {
		out = append(out, n)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].NetworkRef < out[j].NetworkRef })
	return out
}
