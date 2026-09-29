// explain/expand.go: V1 Explain Why
// expansion. The R11 Chain covered the
// happy path; the R12 expansion adds VRF,
// dynamic-routing filter reasons, L2/L3
// loop evidence, BGP/OSPF session state,
// and a typed service endpoint.
//
// The expansion is additive: every new
// hop kind and decision is a constant
// extension. Existing callers continue to
// work because Chain.Encoded JSON is
// forward-compatible.
package explain

import (
	"errors"
	"regexp"
	"sort"
	"sync"
	"time"
)

// HopKind extensions for R12.AM.
const (
	HopVRF         HopKind = "vrf"
	HopBridge      HopKind = "bridge"
	HopSTP         HopKind = "stp"
	HopMACFlap     HopKind = "mac-flap"
	HopBGPSession  HopKind = "bgp-session"
	HopOSPFSession HopKind = "ospf-session"
	HopRouteFilter HopKind = "route-filter"
	HopLoop        HopKind = "loop"
	HopService     HopKind = "service"
)

// Decision extensions for R12.AM.
const (
	DecisionFiltered Decision = "filtered"
	DecisionLoop     Decision = "loop"
)

// EvidenceKind extensions for R12.AM.
const (
	EvidenceConfiguredNotObserved EvidenceKind = "configured-not-observed"
	EvidenceObservedNotConfigured EvidenceKind = "observed-not-configured"
)

// ServiceEndpoint describes a service the
// user is trying to reach, including the
// port and protocol. It composes with the
// R11 Endpoint by adding Protocol/Port
// semantics in a backward-compatible way.
type ServiceEndpoint struct {
	Endpoint
	ServiceName string `json:"serviceName,omitempty"`
	VRF         string `json:"vrf,omitempty"`
	Resolved    bool   `json:"resolved,omitempty"`
}

// VRFHop is the per-VRF hop evidence.
type VRFHop struct {
	VRF      string   `json:"vrf"`
	Table    int      `json:"table"`
	Decision Decision `json:"decision"`
	Reason   string   `json:"reason,omitempty"`
}

// BGPSessionHop is the per-BGP-session
// evidence.
type BGPSessionHop struct {
	Neighbor     string `json:"neighbor"`
	State        string `json:"state"`
	Received     int    `json:"received"`
	Advertised   int    `json:"advertised"`
	FilterHit    string `json:"filterHit,omitempty"`
	RejectReason string `json:"rejectReason,omitempty"`
}

// OSPFSessionHop is the per-OSPF-session
// evidence.
type OSPFSessionHop struct {
	Neighbor string `json:"neighbor"`
	State    string `json:"state"`
	Area     string `json:"area,omitempty"`
}

// RouteFilterHop is the per-filter-trace
// evidence.
type RouteFilterHop struct {
	Prefix string `json:"prefix"`
	Filter string `json:"filter"`
	Action string `json:"action"`
	Reason string `json:"reason,omitempty"`
}

// LoopHop is the per-loop evidence.
type LoopHop struct {
	Level   string   `json:"level"` // "l2" | "l3"
	Members []string `json:"members,omitempty"`
	Detail  string   `json:"detail,omitempty"`
}

// Expand attaches a list of typed
// expansion hops to a Chain. The expansion
// hops are added after the main Hops so the
// wire format remains forward-compatible.
type Expand struct {
	VRF         []VRFHop         `json:"vrf,omitempty"`
	BGP         []BGPSessionHop  `json:"bgp,omitempty"`
	OSPF        []OSPFSessionHop `json:"ospf,omitempty"`
	RouteFilter []RouteFilterHop `json:"routeFilter,omitempty"`
	Loop        []LoopHop        `json:"loop,omitempty"`
}

// ChainExpand is the public output of
// Explain Why: the original R11 Chain
// plus the typed expansion evidence.
type ChainExpand struct {
	Chain
	Expand Expand `json:"expand"`
}

// ExplainBuilder accumulates expansion
// evidence before the final Chain is
// rendered. The builder is goroutine-safe.
type ExplainBuilder struct {
	mu          sync.Mutex
	chain       Chain
	vrf         []VRFHop
	bgp         []BGPSessionHop
	ospf        []OSPFSessionHop
	routeFilter []RouteFilterHop
	loop        []LoopHop
	clock       func() time.Time
}

// NewExplainBuilder returns a builder for
// the supplied endpoints.
func NewExplainBuilder(src, dst Endpoint, clock func() time.Time) *ExplainBuilder {
	if clock == nil {
		clock = time.Now
	}
	return &ExplainBuilder{chain: Chain{SchemaVersion: SchemaVersion, At: clock().UTC(), Source: src, Destination: dst}}
}

// AddVRF records a VRF decision.
func (b *ExplainBuilder) AddVRF(v VRFHop) { b.mu.Lock(); defer b.mu.Unlock(); b.vrf = append(b.vrf, v) }

// AddBGP records a BGP session hop.
func (b *ExplainBuilder) AddBGP(s BGPSessionHop) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.bgp = append(b.bgp, s)
}

// AddOSPF records an OSPF session hop.
func (b *ExplainBuilder) AddOSPF(s OSPFSessionHop) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.ospf = append(b.ospf, s)
}

// AddRouteFilter records a route filter
// trace step.
func (b *ExplainBuilder) AddRouteFilter(r RouteFilterHop) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.routeFilter = append(b.routeFilter, r)
}

// AddLoop records a loop-detection
// evidence.
func (b *ExplainBuilder) AddLoop(l LoopHop) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.loop = append(b.loop, l)
	if b.chain.Verdict != DecisionStop && b.chain.Verdict != DecisionDeny {
		b.chain.Verdict = DecisionLoop
	}
}

// AddHop appends a Hop to the chain and
// recomputes the aggregate verdict.
func (b *ExplainBuilder) AddHop(h Hop) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.chain.Hops) >= MaxHops {
		return
	}
	b.chain.Hops = append(b.chain.Hops, h)
	if h.Decision == DecisionDeny || h.Decision == DecisionStop {
		b.chain.Verdict = DecisionStop
	}
	if h.Decision == DecisionLoop && b.chain.Verdict != DecisionStop {
		b.chain.Verdict = DecisionLoop
	}
	if b.chain.Verdict == "" {
		b.chain.Verdict = DecisionAllow
	}
}

// Build returns the assembled ChainExpand.
func (b *ExplainBuilder) Build() ChainExpand {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := ChainExpand{Chain: b.chain, Expand: Expand{}}
	out.Expand.VRF = append([]VRFHop(nil), b.vrf...)
	out.Expand.BGP = append([]BGPSessionHop(nil), b.bgp...)
	out.Expand.OSPF = append([]OSPFSessionHop(nil), b.ospf...)
	out.Expand.RouteFilter = append([]RouteFilterHop(nil), b.routeFilter...)
	out.Expand.Loop = append([]LoopHop(nil), b.loop...)
	sort.SliceStable(out.Expand.VRF, func(i, k int) bool { return out.Expand.VRF[i].VRF < out.Expand.VRF[k].VRF })
	sort.SliceStable(out.Expand.BGP, func(i, k int) bool { return out.Expand.BGP[i].Neighbor < out.Expand.BGP[k].Neighbor })
	sort.SliceStable(out.Expand.RouteFilter, func(i, k int) bool { return out.Expand.RouteFilter[i].Prefix < out.Expand.RouteFilter[k].Prefix })
	return out
}

// FilterRule is the input to the filter
// trace classifier.
type FilterRule struct {
	Name   string `json:"name"`
	Action string `json:"action"` // "accept" | "reject" | "modify"
	Reason string `json:"reason,omitempty"`
}

// ClassifyRouteFilter is the pure function
// the runtime uses to build a RouteFilterHop
// list from the configured rules and an
// observed prefix. The function is
// deterministic: a prefix is either
// accepted, rejected, or modified by a
// rule, with the rule's reason surfaced.
func ClassifyRouteFilter(prefix string, rules []FilterRule) []RouteFilterHop {
	out := []RouteFilterHop{}
	for _, r := range rules {
		out = append(out, RouteFilterHop{Prefix: prefix, Filter: r.Name, Action: r.Action, Reason: r.Reason})
		if r.Action == "accept" || r.Action == "reject" {
			return out
		}
	}
	return out
}

// DetectRouteLoop walks the supplied
// next-hop chain and returns a LoopHop
// when the same next-hop is observed
// twice.
func DetectRouteLoop(nextHops []string) *LoopHop {
	seen := map[string]int{}
	for _, nh := range nextHops {
		seen[nh]++
		if seen[nh] >= 2 {
			return &LoopHop{Level: "l3", Members: append([]string(nil), nextHops...), Detail: "next-hop repeated: " + nh}
		}
	}
	return nil
}

// DetectL2Loop walks the supplied
// port/MAC list and returns a LoopHop
// when the same MAC is observed on more
// than one port within the window.
func DetectL2Loop(observations []string) *LoopHop {
	counts := map[string]int{}
	for _, m := range observations {
		counts[m]++
	}
	looped := []string{}
	for m, c := range counts {
		if c >= 2 {
			looped = append(looped, m)
		}
	}
	if len(looped) == 0 {
		return nil
	}
	sort.Strings(looped)
	return &LoopHop{Level: "l2", Members: looped, Detail: "mac observed multiple times"}
}

var macPattern = regexp.MustCompile(`^[a-fA-F0-9:]{17}$`)

// ValidServiceEndpoint reports whether a
// ServiceEndpoint is well-formed.
func ValidServiceEndpoint(s ServiceEndpoint) error {
	if s.Address == "" {
		return errors.New("explain: service address is required")
	}
	if s.Port < 0 || s.Port > 65535 {
		return errors.New("explain: service port out of range")
	}
	if s.VRF != "" && !vrfPattern.MatchString(s.VRF) {
		return errors.New("explain: vrf name is not canonical")
	}
	return nil
}

var vrfPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,31}$`)
