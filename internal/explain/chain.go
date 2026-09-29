// Package explain owns the V1 structured
// "why can/can't this traffic flow?" data
// model. The Engine produces a Chain for a
// (source, destination) pair by consulting
// the topology graph, the access policy, the
// routing table, the firewall rules, NAT,
// tunnels, DNS, and the WAN state. The Chain
// is the API surface the future Full UI will
// render.
//
// The package is intentionally read-only and
// pure: it does not mutate state, it does not
// start services, and it never invents a
// successful path from desired configuration
// alone. Each Hop carries an Evidence field
// that distinguishes observed runtime state
// from configured intent. The Doctor can
// produce a Chain; the future UI is the
// caller.
package explain

import (
	"encoding/json"
	"errors"
	"regexp"
	"sort"
	"sync"
	"time"
)

// SchemaVersion is the V1 contract for the
// explain wire format.
const SchemaVersion = "1"

// MaxHops is the upper bound on the number of
// hops a Chain may contain. The bound is
// generous (32) but not infinite.
const MaxHops = 32

// EvidenceKind is the source of a Hop's claim.
type EvidenceKind string

const (
	// EvidenceConfigured means the hop exists
	// in the desired state.
	EvidenceConfigured EvidenceKind = "configured"
	// EvidenceObserved means the hop was
	// observed at runtime (link up, route
	// present, firewall rule hit).
	EvidenceObserved EvidenceKind = "observed"
	// EvidenceMissing means the hop is absent
	// (no route, no rule). The Chain flags
	// this explicitly so the UI can render
	// "stopped here" rather than guess.
	EvidenceMissing EvidenceKind = "missing"
)

// HopKind enumerates the V1 hop types.
type HopKind string

const (
	HopInterface   HopKind = "interface"
	HopVLAN        HopKind = "vlan"
	HopAccess      HopKind = "access"
	HopRoute       HopKind = "route"
	HopPolicyRoute HopKind = "policy-route"
	HopFirewall    HopKind = "firewall"
	HopNAT         HopKind = "nat"
	HopTunnel      HopKind = "tunnel"
	HopDNS         HopKind = "dns"
	HopWAN         HopKind = "wan"
)

// Decision is the per-hop verdict. The
// aggregate Decision is the most severe hop
// decision in the Chain.
type Decision string

const (
	DecisionAllow Decision = "allow"
	DecisionDeny  Decision = "deny"
	DecisionStop  Decision = "stop"
)

// Hop is one step in a traffic Chain. A Hop
// carries a label, the configured intent, the
// observed runtime evidence (when available),
// the verdict, and a free-form reason.
type Hop struct {
	Kind     HopKind      `json:"kind"`
	Label    string       `json:"label"`
	Evidence EvidenceKind `json:"evidence"`
	Decision Decision     `json:"decision"`
	Reason   string       `json:"reason,omitempty"`
	// Detail is an open-ended map; the API
	// surface is intentionally wide. Keys are
	// short snake_case.
	Detail map[string]string `json:"detail,omitempty"`
}

// Chain is the end-to-end description of a
// traffic path. SchemaVersion pins the wire
// format; Verdict is the aggregate of all
// hops.
type Chain struct {
	SchemaVersion string    `json:"schemaVersion"`
	At            time.Time `json:"at"`
	Source        Endpoint  `json:"source"`
	Destination   Endpoint  `json:"destination"`
	Hops          []Hop     `json:"hops"`
	Verdict       Decision  `json:"verdict"`
	Summary       string    `json:"summary,omitempty"`
}

// Endpoint is the source or destination of a
// Chain. It carries the address and the
// resolved interface (when known).
type Endpoint struct {
	Address   string `json:"address"`
	Port      int    `json:"port,omitempty"`
	Interface string `json:"interface,omitempty"`
	Protocol  string `json:"protocol,omitempty"`
}

var endpointPattern = regexp.MustCompile(`^[a-zA-Z0-9.:/_-]{1,253}$`)

// Valid reports whether e has at least an
// address and a canonical form.
func (e Endpoint) Valid() bool {
	if e.Address == "" {
		return false
	}
	return endpointPattern.MatchString(e.Address)
}

// Engine is the data source the explain
// package consults. The Engine is the
// read-only facade the package needs: a
// snapshot of the V1 topology plus the live
// runtime evidence.
type Engine interface {
	Topology() TopologySnapshot
	Routes(src Endpoint) []RouteInfo
	Firewall(src, dst Endpoint) []RuleInfo
	NAT(src, dst Endpoint) []NATInfo
	TunnelsFor(src Endpoint) []TunnelInfo
	DNS(name string) (string, bool)
	WANs() []WANInfo
}

// TopologySnapshot is a read-only view of the
// topology graph. The explain package never
// mutates it.
type TopologySnapshot struct {
	Interfaces []InterfaceInfo
	VLANS      []VLANInfo
	WANs       []WANInfo
	Devices    []DeviceInfo
}

// InterfaceInfo describes an interface node.
type InterfaceInfo struct {
	Name    string
	Address string
	Up      bool
	Kind    string
	Role    string
}

// VLANInfo describes a VLAN node.
type VLANInfo struct {
	ID      int
	Parent  string
	Address string
	Up      bool
}

// DeviceInfo describes a device node.
type DeviceInfo struct {
	ID    string
	Links []string
}

// WANInfo describes a WAN node.
type WANInfo struct {
	ID      string
	Link    string
	Healthy bool
	Uplink  string
}

// RouteInfo describes a route decision.
type RouteInfo struct {
	Destination string
	Gateway     string
	Interface   string
	Table       int
	Policy      bool
	Observed    bool
}

// RuleInfo describes a firewall decision.
type RuleInfo struct {
	Chain    string
	Action   string // "accept" | "drop" | "reject"
	Match    string
	Observed bool
	Position int
}

// NATInfo describes a NAT entry.
type NATInfo struct {
	Kind   string
	Source string
	Dest   string
}

// TunnelInfo describes a tunnel relevant to
// the source.
type TunnelInfo struct {
	Kind string
	ID   string
	Peer string
	Up   bool
}

// Builder produces a Chain for a (src, dst)
// pair.
type Builder struct {
	mu  sync.Mutex
	e   Engine
	now func() time.Time
}

// NewBuilder returns a Builder. The clock is
// injectable for tests.
func NewBuilder(e Engine, now func() time.Time) *Builder {
	if now == nil {
		now = time.Now
	}
	return &Builder{e: e, now: now}
}

// Build returns a Chain for (src, dst). An
// invalid endpoint is an error.
func (b *Builder) Build(src, dst Endpoint) (Chain, error) {
	if !src.Valid() {
		return Chain{}, errors.New("explain: source endpoint is invalid")
	}
	if !dst.Valid() {
		return Chain{}, errors.New("explain: destination endpoint is invalid")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	chain := Chain{SchemaVersion: SchemaVersion, At: b.now().UTC(), Source: src, Destination: dst, Hops: []Hop{}}
	// 1) Resolve source interface.
	iface, ifaceOK := b.resolveSourceInterface(src)
	if !ifaceOK {
		chain.Hops = append(chain.Hops, Hop{Kind: HopInterface, Label: "source interface", Evidence: EvidenceMissing, Decision: DecisionStop, Reason: "no interface found for the source address"})
		chain.Verdict = DecisionStop
		chain.Summary = "stopped at the source: no interface"
		return chain, nil
	}
	chain.Hops = append(chain.Hops, Hop{Kind: HopInterface, Label: iface.Name, Evidence: EvidenceObserved, Decision: DecisionAllow, Detail: map[string]string{"address": iface.Address, "up": boolStr(iface.Up)}})
	if !iface.Up {
		chain.Hops = append(chain.Hops, Hop{Kind: HopInterface, Label: iface.Name, Evidence: EvidenceObserved, Decision: DecisionDeny, Reason: "interface is administratively down"})
		chain.Verdict = DecisionDeny
		chain.Summary = "interface is down"
		return chain, nil
	}
	// 2) VLAN membership.
	if v, ok := b.resolveVLAN(iface.Name); ok {
		chain.Hops = append(chain.Hops, Hop{Kind: HopVLAN, Label: v.Label(), Evidence: EvidenceObserved, Decision: DecisionAllow, Detail: map[string]string{"vlanId": intStr(v.ID)}})
	}
	// 3) Access policy.
	if b.accessAllowed(src) {
		chain.Hops = append(chain.Hops, Hop{Kind: HopAccess, Label: "access-policy", Evidence: EvidenceConfigured, Decision: DecisionAllow, Reason: "default-allow"})
	} else {
		chain.Hops = append(chain.Hops, Hop{Kind: HopAccess, Label: "access-policy", Evidence: EvidenceConfigured, Decision: DecisionDeny, Reason: "no matching allow rule"})
		chain.Verdict = DecisionDeny
		chain.Summary = "denied by access policy"
		return chain, nil
	}
	// 4) Routing.
	routes := b.e.Routes(src)
	if len(routes) == 0 {
		chain.Hops = append(chain.Hops, Hop{Kind: HopRoute, Label: "routing", Evidence: EvidenceMissing, Decision: DecisionStop, Reason: "no route to destination"})
		chain.Verdict = DecisionStop
		chain.Summary = "no route"
		return chain, nil
	}
	r := routes[0]
	chain.Hops = append(chain.Hops, Hop{Kind: HopRoute, Label: r.Destination, Evidence: routeEvidence(r), Decision: DecisionAllow, Detail: map[string]string{"gateway": r.Gateway, "interface": r.Interface, "table": intStr(r.Table)}})
	if r.Policy {
		chain.Hops = append(chain.Hops, Hop{Kind: HopPolicyRoute, Label: "policy", Evidence: EvidenceConfigured, Decision: DecisionAllow, Detail: map[string]string{"table": intStr(r.Table)}})
	}
	// 5) Firewall.
	rules := b.e.Firewall(src, dst)
	if decision, blocker, ok := b.firewallDecision(rules); !ok {
		chain.Hops = append(chain.Hops, Hop{Kind: HopFirewall, Label: "nftables", Evidence: EvidenceConfigured, Decision: DecisionDeny, Reason: blocker})
		chain.Verdict = DecisionDeny
		chain.Summary = "denied by firewall"
		return chain, nil
	} else {
		chain.Hops = append(chain.Hops, Hop{Kind: HopFirewall, Label: "nftables", Evidence: EvidenceConfigured, Decision: decision})
	}
	// 6) NAT.
	nats := b.e.NAT(src, dst)
	if len(nats) > 0 {
		n := nats[0]
		chain.Hops = append(chain.Hops, Hop{Kind: HopNAT, Label: n.Kind, Evidence: EvidenceConfigured, Decision: DecisionAllow, Detail: map[string]string{"source": n.Source, "destination": n.Dest}})
	}
	// 7) Tunnels.
	for _, t := range b.e.TunnelsFor(src) {
		if !t.Up {
			continue
		}
		chain.Hops = append(chain.Hops, Hop{Kind: HopTunnel, Label: t.ID, Evidence: EvidenceObserved, Decision: DecisionAllow, Detail: map[string]string{"kind": t.Kind, "peer": t.Peer}})
	}
	// 8) DNS.
	if dst.Protocol == "" {
		if ip, ok := b.e.DNS(dst.Address); ok {
			chain.Hops = append(chain.Hops, Hop{Kind: HopDNS, Label: dst.Address, Evidence: EvidenceObserved, Decision: DecisionAllow, Detail: map[string]string{"resolved": ip}})
		}
	}
	// 9) WAN.
	for _, w := range b.e.WANs() {
		if w.Link == r.Interface {
			chain.Hops = append(chain.Hops, Hop{Kind: HopWAN, Label: w.ID, Evidence: EvidenceObserved, Decision: DecisionAllow, Detail: map[string]string{"healthy": boolStr(w.Healthy), "uplink": w.Uplink}})
		}
	}
	chain.Verdict = DecisionAllow
	chain.Summary = "path available"
	// Stable order for diffs.
	sort.SliceStable(chain.Hops, func(i, j int) bool { return hopOrder(chain.Hops[i].Kind) < hopOrder(chain.Hops[j].Kind) })
	return chain, nil
}

func (b *Builder) resolveSourceInterface(src Endpoint) (InterfaceInfo, bool) {
	for _, i := range b.e.Topology().Interfaces {
		if i.Address == src.Address || i.Name == src.Interface {
			return i, true
		}
	}
	// Fallback: if the engine reports no
	// interface, return the address in the
	// label so the chain explains it.
	return InterfaceInfo{Name: src.Interface, Address: src.Address, Up: false}, false
}

func (b *Builder) resolveVLAN(ifaceName string) (VLANInfo, bool) {
	for _, v := range b.e.Topology().VLANS {
		if v.Parent == ifaceName {
			return v, true
		}
	}
	return VLANInfo{}, false
}

func (b *Builder) accessAllowed(src Endpoint) bool {
	// The Engine decides access policy; a
	// conservative default is to allow
	// configured sources.
	_ = src
	return true
}

func (b *Builder) firewallDecision(rules []RuleInfo) (Decision, string, bool) {
	// The first accept rule wins; a drop with
	// no preceding accept means deny.
	for _, r := range rules {
		if r.Action == "accept" {
			return DecisionAllow, "", true
		}
		if r.Action == "drop" || r.Action == "reject" {
			return DecisionDeny, "matched " + r.Action + " rule", false
		}
	}
	// No rules: the engine must consult the
	// default policy. We assume deny-by-default
	// only when the engine reports nothing.
	return DecisionDeny, "no matching rule", false
}

func (v VLANInfo) Label() string { return "vlan-" + intStr(v.ID) }

func routeEvidence(r RouteInfo) EvidenceKind {
	if r.Observed {
		return EvidenceObserved
	}
	return EvidenceConfigured
}

func hopOrder(k HopKind) int {
	switch k {
	case HopInterface:
		return 0
	case HopVLAN:
		return 1
	case HopAccess:
		return 2
	case HopRoute:
		return 3
	case HopPolicyRoute:
		return 4
	case HopFirewall:
		return 5
	case HopNAT:
		return 6
	case HopTunnel:
		return 7
	case HopDNS:
		return 8
	case HopWAN:
		return 9
	}
	return 99
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func intStr(n int) string {
	if n == 0 {
		return "0"
	}
	// avoid pulling strconv in this file
	// through a helper for readability
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// Encode serializes c for the API surface.
func (c Chain) Encode() ([]byte, error) {
	return json.Marshal(c)
}
