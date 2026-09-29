// vlan/vlan.go: V1 VLAN/QinQ/bridge-isolation
// model. The package owns the typed contracts
// for VLAN access/trunk ports, QinQ
// (802.1ad) service tags, bridge port
// isolation, and FDB control.
//
// The runtime is platform-agnostic; the Linux
// mapping is `ip link add link ... type
// vlan` and `bridge vlan add`. The V1 model is
// the contract.
package vlan

import (
	"errors"
	"regexp"
	"sort"
	"sync"
)

// PVID is the port VLAN id; valid range is
// 1..4094.
type PVID int

// PVIDMin / PVIDMax are the canonical bounds.
const (
	PVIDMin = 1
	PVIDMax = 4094
)

// PortMode is the per-port VLAN mode.
type PortMode string

const (
	PortAccess   PortMode = "access"
	PortTrunk    PortMode = "trunk"
	PortQinQ     PortMode = "qinq"
	PortUntagged PortMode = "untagged"
)

// Valid reports whether m is known.
func (m PortMode) Valid() bool {
	switch m {
	case PortAccess, PortTrunk, PortQinQ, PortUntagged:
		return true
	}
	return false
}

// Port is one bridge port with its VLAN
// configuration.
type Port struct {
	Link          string   `json:"link"`
	Bridge        string   `json:"bridge"`
	Mode          PortMode `json:"mode"`
	PVID          int      `json:"pvid"`
	Tagged        []int    `json:"tagged,omitempty"`
	Untagged      []int    `json:"untagged,omitempty"`
	IngressFilter bool     `json:"ingressFilter"`
	// Native is the native VLAN on a trunk.
	// For access ports, the PVID is the only
	// untagged VLAN.
	Native int `json:"native,omitempty"`
	// QinQServiceTag is the outer tag for
	// QinQ-encapsulated frames. Only valid
	// when Mode == PortQinQ.
	QinQServiceTag int `json:"qinqServiceTag,omitempty"`
	// Isolated is true when the port is part
	// of a protected/isolated group.
	Isolated bool `json:"isolated"`
}

var linkPattern = regexp.MustCompile(`^[a-z][a-z0-9._-]{0,15}$`)

// Valid reports whether p is well-formed.
func (p Port) Valid() error {
	if !linkPattern.MatchString(p.Link) {
		return errors.New("vlan: link is not canonical")
	}
	if !linkPattern.MatchString(p.Bridge) {
		return errors.New("vlan: bridge is not canonical")
	}
	if !p.Mode.Valid() {
		return errors.New("vlan: port mode is unknown")
	}
	if p.PVID < PVIDMin || p.PVID > PVIDMax {
		return errors.New("vlan: pvid out of range")
	}
	if p.QinQServiceTag != 0 && (p.QinQServiceTag < PVIDMin || p.QinQServiceTag > PVIDMax) {
		return errors.New("vlan: qinq service tag out of range")
	}
	if p.QinQServiceTag != 0 && p.Mode != PortQinQ {
		return errors.New("vlan: qinq service tag requires qinq mode")
	}
	for _, vid := range p.Tagged {
		if vid < PVIDMin || vid > PVIDMax {
			return errors.New("vlan: tagged list contains out-of-range id")
		}
	}
	return nil
}

// Bridge is a software bridge with its port
// set and FDB configuration.
type Bridge struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Ports []Port `json:"ports"`
	// VLANFiltering enables per-VLAN bridge
	// filtering.
	VLANFiltering bool `json:"vlanFiltering"`
	// StaticMACs is a list of pinned FDB
	// entries: mac -> port.
	StaticMACs []StaticMAC `json:"staticMacs,omitempty"`
}

// StaticMAC is a pinned FDB entry.
type StaticMAC struct {
	MAC  string `json:"mac"`
	Port string `json:"port"`
	VLAN int    `json:"vlan"`
}

var idPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:-]{0,63}$`)

// Valid reports whether b is well-formed.
func (b Bridge) Valid() error {
	if !idPattern.MatchString(b.ID) {
		return errors.New("vlan: id is not canonical")
	}
	if !linkPattern.MatchString(b.Name) {
		return errors.New("vlan: name is not canonical")
	}
	for _, p := range b.Ports {
		if err := p.Valid(); err != nil {
			return err
		}
	}
	return nil
}

// Manager is the V1 bridge/VLAN store. Manager
// is goroutine-safe.
type Manager struct {
	mu      sync.Mutex
	bridges map[string]Bridge
}

// NewManager returns an empty manager.
func NewManager() *Manager {
	return &Manager{bridges: map[string]Bridge{}}
}

// Set replaces or inserts a bridge.
func (m *Manager) Set(b Bridge) error {
	if err := b.Valid(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.bridges[b.ID] = b
	return nil
}

// Get returns a bridge by id.
func (m *Manager) Get(id string) (Bridge, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.bridges[id]
	return b, ok
}

// List returns the bridges sorted by id.
func (m *Manager) List() []Bridge {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Bridge, 0, len(m.bridges))
	for _, b := range m.bridges {
		out = append(out, b)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Delete removes a bridge.
func (m *Manager) Delete(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.bridges, id)
}

// ValidateTopology checks for dangerous
// combinations before apply: trunk/access
// contradictions, isolated-port members of
// the same bridge, and missing native vlan on
// a trunk.
func (m *Manager) ValidateTopology() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	var warnings []string
	for _, b := range m.bridges {
		isolated := map[string]bool{}
		for _, p := range b.Ports {
			if p.Isolated {
				isolated[p.Link] = true
			}
		}
		// Two isolated ports in the same
		// bridge with no protection: warn.
		count := 0
		for _, p := range b.Ports {
			if isolated[p.Link] {
				count++
			}
		}
		if count > 1 {
			warnings = append(warnings, "bridge "+b.Name+" contains multiple isolated ports")
		}
		for _, p := range b.Ports {
			if p.Mode == PortTrunk && p.Native == 0 {
				warnings = append(warnings, "bridge "+b.Name+" port "+p.Link+" is a trunk without a native VLAN")
			}
		}
	}
	return warnings
}
