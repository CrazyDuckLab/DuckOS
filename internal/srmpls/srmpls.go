// srmpls/srmpls.go: V1 segment-routing
// and MPLS surface. The package owns the
// typed contracts for LDP, SR-MPLS, SRv6,
// L3VPN, MPLS pseudowire / L2VPN, route
// distinguishers and targets, and the
// per-platform capability gate.
//
// The runtime is a thin mapping to
// internal/frr and the kernel MPLS
// dataplane.
package srmpls

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
)

// Capability is the platform MPLS / SR
// capability gate.
type Capability string

const (
	CapMPLSFwd Capability = "mpls-forwarding"
	CapLDP     Capability = "ldp"
	CapSRMPLS  Capability = "sr-mpls"
	CapSRv6    Capability = "srv6"
	CapL3VPN   Capability = "l3vpn"
	CapL2VPN   Capability = "l2vpn"
	CapPW      Capability = "pseudowire"
)

// Valid reports whether c is known.
func (c Capability) Valid() bool {
	switch c {
	case CapMPLSFwd, CapLDP, CapSRMPLS, CapSRv6,
		CapL3VPN, CapL2VPN, CapPW:
		return true
	}
	return false
}

// CapabilitySet is the per-platform
// capability report.
type CapabilitySet struct {
	Capabilities []Capability `json:"capabilities"`
	DetectedAt   string       `json:"detectedAt"`
}

// Supports reports whether the supplied
// capability is in the set.
func (c CapabilitySet) Supports(cap Capability) bool {
	for _, x := range c.Capabilities {
		if x == cap {
			return true
		}
	}
	return false
}

// RouteDistinguisher is the L3VPN RD.
type RouteDistinguisher struct {
	Type   string `json:"type"` // "as" | "ip"
	AS     uint32 `json:"as,omitempty"`
	IP     string `json:"ip,omitempty"`
	Number uint32 `json:"number,omitempty"`
}

// Valid reports whether r is well-formed.
func (r RouteDistinguisher) Valid() error {
	switch r.Type {
	case "as":
		if r.AS == 0 || r.Number == 0 {
			return errors.New("srmpls: as-type RD requires as+number")
		}
	case "ip":
		if r.IP == "" || r.Number == 0 {
			return errors.New("srmpls: ip-type RD requires ip+number")
		}
	default:
		return errors.New("srmpls: RD type is unknown")
	}
	return nil
}

// String returns the canonical "ASN:NN" or
// "IP:NN" form.
func (r RouteDistinguisher) String() string {
	switch r.Type {
	case "as":
		return fmt.Sprintf("%d:%d", r.AS, r.Number)
	case "ip":
		return fmt.Sprintf("%s:%d", r.IP, r.Number)
	}
	return ""
}

// RouteTarget is the RT.
type RouteTarget struct {
	Value string `json:"value"`
}

// Valid reports whether t is well-formed.
func (t RouteTarget) Valid() error {
	if !rtPattern.MatchString(t.Value) {
		return errors.New("srmpls: RT is not canonical: " + t.Value)
	}
	return nil
}

var rtPattern = regexp.MustCompile(`^(\d+:\d+|[0-9a-fA-F]{2,4}:(\d+:\d+|\d+))$`)

// L3VPN is one L3VPN instance.
type L3VPN struct {
	Name      string             `json:"name"`
	RD        RouteDistinguisher `json:"rd"`
	ImportRTs []RouteTarget      `json:"importRts,omitempty"`
	ExportRTs []RouteTarget      `json:"exportRts,omitempty"`
	// VRF is the local VRF id.
	VRF string `json:"vrf"`
	// Interfaces participating in the
	// VRF.
	Interfaces []string `json:"interfaces,omitempty"`
}

var namePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,63}$`)

// Valid reports whether l is well-formed.
func (l L3VPN) Valid() error {
	if !namePattern.MatchString(l.Name) {
		return errors.New("srmpls: name is not canonical")
	}
	if err := l.RD.Valid(); err != nil {
		return err
	}
	for _, rt := range l.ImportRTs {
		if err := rt.Valid(); err != nil {
			return err
		}
	}
	for _, rt := range l.ExportRTs {
		if err := rt.Valid(); err != nil {
			return err
		}
	}
	return nil
}

// MPLSPath is a static MPLS LSP.
type MPLSPath struct {
	Name    string `json:"name"`
	Labels  []int  `json:"labels"`
	Nexthop string `json:"nexthop"`
}

// Valid reports whether p is well-formed.
func (p MPLSPath) Valid() error {
	if !namePattern.MatchString(p.Name) {
		return errors.New("srmpls: name is not canonical")
	}
	if len(p.Labels) == 0 {
		return errors.New("srmpls: labels are required")
	}
	for _, l := range p.Labels {
		if l < 0 || l > 1048575 {
			return errors.New("srmpls: label out of range")
		}
	}
	return nil
}

// SRPolicy is a segment-routing policy.
type SRPolicy struct {
	Name     string `json:"name"`
	Endpoint string `json:"endpoint"`
	Color    int    `json:"color"`
	// SegmentList is the SID list.
	SegmentList []int `json:"segmentList"`
	// Preference is the path preference.
	Preference int `json:"preference"`
}

// Valid reports whether p is well-formed.
func (p SRPolicy) Valid() error {
	if !namePattern.MatchString(p.Name) {
		return errors.New("srmpls: name is not canonical")
	}
	if p.Endpoint == "" {
		return errors.New("srmpls: endpoint is required")
	}
	if len(p.SegmentList) == 0 {
		return errors.New("srmpls: segment list is required")
	}
	return nil
}

// Config is the SR/MPLS configuration.
type Config struct {
	L3VPNs   []L3VPN    `json:"l3vpns,omitempty"`
	Paths    []MPLSPath `json:"paths,omitempty"`
	Policies []SRPolicy `json:"policies,omitempty"`
}

// Valid reports whether c is well-formed.
func (c Config) Valid() error {
	for _, l := range c.L3VPNs {
		if err := l.Valid(); err != nil {
			return err
		}
	}
	for _, p := range c.Paths {
		if err := p.Valid(); err != nil {
			return err
		}
	}
	for _, p := range c.Policies {
		if err := p.Valid(); err != nil {
			return err
		}
	}
	return nil
}

// Render produces the frr-compatible
// fragment text. The output is
// deterministic.
func Render(c Config) (string, error) {
	if err := c.Valid(); err != nil {
		return "", err
	}
	var sb strings.Builder
	sb.WriteString("! DuckOS-generated MPLS / SR config\n")
	vpns := append([]L3VPN(nil), c.L3VPNs...)
	sort.Slice(vpns, func(i, k int) bool { return vpns[i].Name < vpns[k].Name })
	for _, v := range vpns {
		fmt.Fprintf(&sb, "vrf %s\n", v.Name)
		fmt.Fprintf(&sb, " vrf rd %s\n", v.RD.String())
		imps := append([]RouteTarget(nil), v.ImportRTs...)
		sort.Slice(imps, func(i, k int) bool { return imps[i].Value < imps[k].Value })
		for _, rt := range imps {
			fmt.Fprintf(&sb, " vrf rt import %s\n", rt.Value)
		}
		exps := append([]RouteTarget(nil), v.ExportRTs...)
		sort.Slice(exps, func(i, k int) bool { return exps[i].Value < exps[k].Value })
		for _, rt := range exps {
			fmt.Fprintf(&sb, " vrf rt export %s\n", rt.Value)
		}
		ifs := append([]string(nil), v.Interfaces...)
		sort.Strings(ifs)
		for _, i := range ifs {
			fmt.Fprintf(&sb, " vrf interface %s\n", i)
		}
		sb.WriteString("!\n")
	}
	paths := append([]MPLSPath(nil), c.Paths...)
	sort.Slice(paths, func(i, k int) bool { return paths[i].Name < paths[k].Name })
	for _, p := range paths {
		fmt.Fprintf(&sb, "mpls lsp %s nexthop %s labels", p.Name, p.Nexthop)
		for _, l := range p.Labels {
			fmt.Fprintf(&sb, " %d", l)
		}
		sb.WriteString("\n!\n")
	}
	pols := append([]SRPolicy(nil), c.Policies...)
	sort.Slice(pols, func(i, k int) bool { return pols[i].Name < pols[k].Name })
	for _, p := range pols {
		fmt.Fprintf(&sb, "segment-routing policy %s endpoint %s color %d preference %d\n", p.Name, p.Endpoint, p.Color, p.Preference)
		fmt.Fprintf(&sb, "  segment-list")
		for _, s := range p.SegmentList {
			fmt.Fprintf(&sb, " %d", s)
		}
		sb.WriteString("\n!\n")
	}
	return sb.String(), nil
}

// Manager is the V1 SR/MPLS store.
type Manager struct {
	mu     sync.Mutex
	config Config
}

// NewManager returns an empty manager.
func NewManager() *Manager { return &Manager{} }

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

// Render returns the rendered config text.
func (m *Manager) Render() (string, error) {
	m.mu.Lock()
	c := m.config
	m.mu.Unlock()
	return Render(c)
}

// Capabilities is the platform capability
// store.
type Capabilities struct {
	mu sync.Mutex
	c  CapabilitySet
}

// NewCapabilities returns an empty
// capability store.
func NewCapabilities() *Capabilities { return &Capabilities{} }

// Set replaces the capability set.
func (c *Capabilities) Set(set CapabilitySet) {
	for _, x := range set.Capabilities {
		if !x.Valid() {
			return
		}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.c = set
}

// Get returns the current set.
func (c *Capabilities) Get() CapabilitySet {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.c
}
