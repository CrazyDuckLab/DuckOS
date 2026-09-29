// evpn/evpn.go: V1 VXLAN/EVPN deep
// surface. The package owns the typed
// contracts for VTEPs, VNIs, EVPN
// Ethernet Segments, ESI, multihoming
// all-active / single-active, and
// ARP/ND suppression.
//
// The runtime is a thin mapping to the
// existing internal/tunnels package +
// internal/frr (bgp evpn address-family).
package evpn

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
)

// MultihomingMode is the EVPN multihoming
// mode.
type MultihomingMode string

const (
	ModeAllActive    MultihomingMode = "all-active"
	ModeSingleActive MultihomingMode = "single-active"
)

// Valid reports whether m is known.
func (m MultihomingMode) Valid() bool { return m == ModeAllActive || m == ModeSingleActive }

// VTEP is a VXLAN Tunnel Endpoint.
type VTEP struct {
	Name    string `json:"name"`
	Address string `json:"address"` // local source IP
}

// Valid reports whether v is well-formed.
func (v VTEP) Valid() error {
	if v.Name == "" {
		return errors.New("evpn: vtep name is required")
	}
	if v.Address == "" {
		return errors.New("evpn: vtep address is required")
	}
	return nil
}

// EthernetSegment is the EVPN Ethernet
// Segment Identifier.
type EthernetSegment struct {
	ESI     string          `json:"esi"`
	Name    string          `json:"name,omitempty"`
	Mode    MultihomingMode `json:"mode"`
	Members []string        `json:"members,omitempty"`
}

var esiPattern = regexp.MustCompile(`^([0-9a-fA-F]{2}:){9}[0-9a-fA-F]{2}$`)

// Valid reports whether e is well-formed.
func (e EthernetSegment) Valid() error {
	if !esiPattern.MatchString(e.ESI) {
		return errors.New("evpn: ESI is not canonical: " + e.ESI)
	}
	if !e.Mode.Valid() {
		return errors.New("evpn: multihoming mode is invalid")
	}
	return nil
}

// VNI is one VXLAN network identifier.
type VNI struct {
	ID          int    `json:"id"`
	Description string `json:"description,omitempty"`
	VTEP        string `json:"vtep"`
	Bridge      string `json:"bridge"`
	// VLANs maps the inner VLAN to the
	// VNI.
	VLANs []int `json:"vlans,omitempty"`
	// ARPSuppression enables ARP/ND
	// suppression.
	ARPSuppression bool `json:"arpSuppression,omitempty"`
	// VRF optionally binds the VNI to a
	// L3VNI.
	VRF string `json:"vrf,omitempty"`
}

// Valid reports whether v is well-formed.
func (v VNI) Valid() error {
	if v.ID < 1 || v.ID > 16777215 {
		return errors.New("evpn: VNI out of range")
	}
	if v.VTEP == "" {
		return errors.New("evpn: vtep is required")
	}
	if v.Bridge == "" {
		return errors.New("evpn: bridge is required")
	}
	return nil
}

// Config is the EVPN configuration.
type Config struct {
	VTEPs    []VTEP            `json:"vteps"`
	Segments []EthernetSegment `json:"segments,omitempty"`
	VNIs     []VNI             `json:"vnis,omitempty"`
}

// Valid reports whether c is well-formed.
func (c Config) Valid() error {
	if len(c.VTEPs) == 0 {
		return errors.New("evpn: at least one VTEP is required")
	}
	for _, v := range c.VTEPs {
		if err := v.Valid(); err != nil {
			return err
		}
	}
	for _, s := range c.Segments {
		if err := s.Valid(); err != nil {
			return err
		}
	}
	for _, n := range c.VNIs {
		if err := n.Valid(); err != nil {
			return err
		}
	}
	return nil
}

// Render produces the FRR EVPN
// configuration text. The output is
// deterministic.
func Render(c Config) (string, error) {
	if err := c.Valid(); err != nil {
		return "", err
	}
	var sb strings.Builder
	sb.WriteString("! DuckOS-generated EVPN config\n")
	vteps := append([]VTEP(nil), c.VTEPs...)
	sort.Slice(vteps, func(i, k int) bool { return vteps[i].Name < vteps[k].Name })
	for _, v := range vteps {
		fmt.Fprintf(&sb, "interface lo\n ip address %s/32\n", v.Address)
	}
	vnis := append([]VNI(nil), c.VNIs...)
	sort.Slice(vnis, func(i, k int) bool { return vnis[i].ID < vnis[k].ID })
	for _, n := range vnis {
		fmt.Fprintf(&sb, "vni %d\n", n.ID)
		fmt.Fprintf(&sb, " vxlan id %d\n", n.ID)
		if n.VRF != "" {
			fmt.Fprintf(&sb, " vrf %s\n", n.VRF)
		}
		if n.ARPSuppression {
			sb.WriteString(" arp-nd-suppress\n")
		}
	}
	segs := append([]EthernetSegment(nil), c.Segments...)
	sort.Slice(segs, func(i, k int) bool { return segs[i].ESI < segs[k].ESI })
	for _, s := range segs {
		fmt.Fprintf(&sb, "evpn mh es-id %s mode %s\n", s.ESI, s.Mode)
	}
	return sb.String(), nil
}

// DFState is the Designated Forwarder
// state for an Ethernet Segment.
type DFState string

const (
	DFPrimary DFState = "primary"
	DFBackup  DFState = "backup"
	DFNonDF   DFState = "non-df"
)

// Valid reports whether s is known.
func (s DFState) Valid() bool {
	return s == DFPrimary || s == DFBackup || s == DFNonDF
}

// DFResult is the per-ES DF election
// result.
type DFResult struct {
	ESI    string  `json:"esi"`
	State  DFState `json:"state"`
	VTEP   string  `json:"vtep"`
	Reason string  `json:"reason,omitempty"`
}

// Manager is the EVPN store.
type Manager struct {
	mu     sync.Mutex
	config Config
	df     map[string]DFResult
}

// NewManager returns an empty manager.
func NewManager() *Manager { return &Manager{df: map[string]DFResult{}} }

// Set replaces the config.
func (m *Manager) Set(c Config) error {
	if err := c.Valid(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.config = c
	return nil
}

// Render returns the rendered config.
func (m *Manager) Render() (string, error) {
	m.mu.Lock()
	c := m.config
	m.mu.Unlock()
	return Render(c)
}

// SetDF records the DF election result.
func (m *Manager) SetDF(r DFResult) {
	if !r.State.Valid() {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.df[r.ESI] = r
}

// DF returns the DF result for an ESI.
func (m *Manager) DF(esi string) (DFResult, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.df[esi]
	return r, ok
}

// DFAll returns every DF result.
func (m *Manager) DFAll() []DFResult {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]DFResult, 0, len(m.df))
	for _, r := range m.df {
		out = append(out, r)
	}
	sort.Slice(out, func(i, k int) bool { return out[i].ESI < out[k].ESI })
	return out
}
