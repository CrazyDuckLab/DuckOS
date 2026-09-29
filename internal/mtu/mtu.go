// mtu/mtu.go: V1 MTU/PMTUD model. The package
// owns the typed contracts for per-link MTU,
// per-tunnel MTU, L2MTU where exposed by
// hardware, MSS clamping, and PMTUD
// diagnostics. The runtime is a thin mapping
// to `ip link set mtu`, `iptables -A FORWARD
// -p tcp --tcp-flags SYN,RST SYN -j TCPMSS
// --clamp-mss-to-pmtu`, and the per-route
// metrics that drive PMTUD failure
// detection.
package mtu

import (
	"errors"
	"regexp"
	"sort"
	"sync"
)

// MaxLinkMTU is the V1 hard cap for any link
// MTU. Jumbo frames are accepted up to this
// value.
const MaxLinkMTU = 9000

// LinkMTU is the per-link MTU configuration.
type LinkMTU struct {
	Link string `json:"link"`
	MTU  int    `json:"mtu"`
	// L2MTU is the hardware-exposed L2 MTU
	// (informational; the runtime reads it
	// from sysfs).
	L2MTU int `json:"l2mtu,omitempty"`
	// Overhead is the per-overlay overhead
	// (PPPoE=8, VLAN=4, QinQ=8, VXLAN=50,
	// GRE=4, IPsec=50..100, WireGuard=32).
	Overhead int `json:"overhead,omitempty"`
}

var linkPattern = regexp.MustCompile(`^[a-z][a-z0-9._-]{0,15}$`)

// Valid reports whether m is well-formed.
func (m LinkMTU) Valid() error {
	if !linkPattern.MatchString(m.Link) {
		return errors.New("mtu: link is not canonical")
	}
	if m.MTU < 1280 || m.MTU > MaxLinkMTU {
		return errors.New("mtu: MTU out of range")
	}
	if m.L2MTU < 0 || m.L2MTU > MaxLinkMTU {
		return errors.New("mtu: L2MTU out of range")
	}
	return nil
}

// ClampMSS returns the MSS for a given
// outer-MTU. The formula follows the V1
// contract: MSS = outerMTU - 40 (IPv4) or
// outerMTU - 60 (IPv6).
func ClampMSS(outerMTU int, v6 bool) int {
	if v6 {
		return outerMTU - 60
	}
	return outerMTU - 40
}

// PMTUReport is the per-target PMTUD result.
// The runtime reports the measured outer-MTU
// for a destination; the model surfaces the
// result.
type PMTUReport struct {
	Destination string `json:"destination"`
	MTU         int    `json:"mtu"`
	Reachable   bool   `json:"reachable"`
	Blackhole   bool   `json:"blackhole,omitempty"`
	// Reason is the operator-facing
	// explanation when reachable=false.
	Reason string `json:"reason,omitempty"`
}

// Valid reports whether r is well-formed.
func (r PMTUReport) Valid() error {
	if r.Destination == "" {
		return errors.New("mtu: destination is required")
	}
	if r.MTU < 0 {
		return errors.New("mtu: MTU must be non-negative")
	}
	return nil
}

// Probe is a per-target PMTUD probe. The
// runtime runs `ping -M do -s <n>` to detect
// black-holes.
type Probe struct {
	Destination string `json:"destination"`
	// StartMTU is the lower bound for the
	// search.
	StartMTU int `json:"startMtu"`
	// MinMTU is the floor; the probe stops
	// when MTU drops to MinMTU.
	MinMTU int `json:"minMtu"`
}

// Manager is the V1 MTU/PMTUD store.
type Manager struct {
	mu      sync.Mutex
	links   map[string]LinkMTU
	reports map[string]PMTUReport
}

// NewManager returns an empty manager.
func NewManager() *Manager {
	return &Manager{links: map[string]LinkMTU{}, reports: map[string]PMTUReport{}}
}

// SetLink replaces or inserts a LinkMTU.
func (m *Manager) SetLink(l LinkMTU) error {
	if err := l.Valid(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.links[l.Link] = l
	return nil
}

// SetReport stores a PMTU report.
func (m *Manager) SetReport(r PMTUReport) error {
	if err := r.Valid(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.reports[r.Destination] = r
	return nil
}

// Links returns all LinkMTUs sorted by link.
func (m *Manager) Links() []LinkMTU {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]LinkMTU, 0, len(m.links))
	for _, l := range m.links {
		out = append(out, l)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Link < out[j].Link })
	return out
}

// Reports returns all PMTU reports sorted by
// destination.
func (m *Manager) Reports() []PMTUReport {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]PMTUReport, 0, len(m.reports))
	for _, r := range m.reports {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Destination < out[j].Destination })
	return out
}

// ClampMSSForLink returns the MSS for a given
// link, accounting for the link's overhead
// (PPPoE/VLAN/QinQ/VXLAN/GRE/IPsec/WireGuard).
func (m *Manager) ClampMSSForLink(link string, v6 bool) (int, error) {
	m.mu.Lock()
	l, ok := m.links[link]
	m.mu.Unlock()
	if !ok {
		return 0, errors.New("mtu: link not found")
	}
	outer := l.MTU - l.Overhead
	if outer < 1280 {
		return 0, errors.New("mtu: effective MTU is below IPv6 minimum")
	}
	return ClampMSS(outer, v6), nil
}
