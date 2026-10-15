// Package xray / policy.go: routing policy studio model.
//
// Policies are the operator-facing split-tunneling rules; Compile
// deterministically maps them onto Xray RoutingRules. The model
// never bypasses Safe Apply: compiled rules flow through the same
// rendered-config path as everything else, and a policy set that
// fails conflict detection is refused before any render.
//
// DNS-leak posture: domain selectors rely on destination sniffing,
// which renderInbound enables on every inbound; policy
// documentation and diagnostics must state that domain rules are
// only as good as the sniffed SNI/Host. IPv6: CIDR selectors are
// address-family specific; a v4-only CIDR never matches v6 traffic
// and vice versa — Conflicts reports mixed-family duplicates as a
// warning-level conflict, not an error.
package xray

import (
	"errors"
	"fmt"
	"net/netip"
	"sort"
	"strings"
)

// PolicyAction is the routing decision for matching traffic.
type PolicyAction string

const (
	PolicyDirect   PolicyAction = "direct"
	PolicyProxy    PolicyAction = "proxy"
	PolicyBlock    PolicyAction = "block"
	PolicyOutbound PolicyAction = "outbound"
)

// Policy is one split-tunneling rule.
type Policy struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Priority int    `json:"priority"` // lower wins; deterministic tiebreak by ID
	// Selectors: a rule matches when ANY of its selectors matches.
	Domains     []string     `json:"domains,omitempty"`     // suffix or full match patterns
	CIDRs       []string     `json:"cidrs,omitempty"`       // ip or cidr, any family
	Ports       []string     `json:"ports,omitempty"`       // "443" or "1000-2000"
	Protocols   []string     `json:"protocols,omitempty"`   // tcp|udp
	InboundTags []string     `json:"inboundTags,omitempty"` // LAN/device scope via inbound tags
	Action      PolicyAction `json:"action"`
	// OutboundTag is required for PolicyOutbound.
	OutboundTag string `json:"outboundTag,omitempty"`
	Enabled     bool   `json:"enabled"`
}

// Validate enforces the policy contract.
func (p Policy) Validate() error {
	if p.ID == "" || p.Name == "" {
		return errors.New("policy: id and name are required")
	}
	switch p.Action {
	case PolicyDirect, PolicyProxy, PolicyBlock, PolicyOutbound:
	default:
		return fmt.Errorf("policy: action %q is not canonical", p.Action)
	}
	if (p.Action == PolicyOutbound || p.Action == PolicyProxy) && p.OutboundTag == "" {
		return errors.New("policy: proxy/outbound action requires an explicit outbound tag (the renderer never invents remote endpoints)")
	}
	if len(p.Domains) == 0 && len(p.CIDRs) == 0 && len(p.Ports) == 0 &&
		len(p.Protocols) == 0 && len(p.InboundTags) == 0 {
		return errors.New("policy: at least one selector is required")
	}
	for _, c := range p.CIDRs {
		if !strings.Contains(c, "/") {
			if _, err := netip.ParseAddr(c); err != nil {
				return fmt.Errorf("policy: selector %q is not an address or CIDR", c)
			}
			continue
		}
		if _, err := netip.ParsePrefix(c); err != nil {
			return fmt.Errorf("policy: selector %q is not a CIDR", c)
		}
	}
	for _, d := range p.Domains {
		if d == "" || len(d) > 253 || strings.ContainsAny(d, " \t/\\") {
			return fmt.Errorf("policy: domain selector %q is not canonical", d)
		}
	}
	for _, port := range p.Ports {
		if err := validatePortSelector(port); err != nil {
			return err
		}
	}
	for _, pr := range p.Protocols {
		if pr != "tcp" && pr != "udp" {
			return fmt.Errorf("policy: protocol %q is not tcp/udp", pr)
		}
	}
	return nil
}

func validatePortSelector(port string) error {
	if a, b, ok := strings.Cut(port, "-"); ok {
		if !isPortStr(a) || !isPortStr(b) {
			return fmt.Errorf("policy: port range %q is not numeric", port)
		}
		return nil
	}
	if !isPortStr(port) {
		return fmt.Errorf("policy: port %q is not numeric", port)
	}
	return nil
}

func isPortStr(s string) bool {
	if s == "" || len(s) > 5 {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// Conflict describes a policy-pair contradiction.
type Conflict struct {
	A, B   string `json:"a"`
	Reason string `json:"reason"`
	// Severity: error blocks compilation; warning does not.
	Severity string `json:"severity"`
}

// DetectConflicts checks every enabled policy pair for selector
// overlap with differing actions. Identical overlap with identical
// action is benign (priority decides). Domain/CIDR overlaps across
// differing actions are errors; mixed address-family duplicates are
// warnings (they never actually collide).
func DetectConflicts(policies []Policy) []Conflict {
	var out []Conflict
	add := func(a, b Policy, severity, reason string) {
		out = append(out, Conflict{A: a.ID, B: b.ID, Reason: reason, Severity: severity})
	}
	for i := range policies {
		a := policies[i]
		if !a.Enabled {
			continue
		}
		for j := i + 1; j < len(policies); j++ {
			b := policies[j]
			if !b.Enabled {
				continue
			}
			if a.Action == b.Action && a.OutboundTag == b.OutboundTag {
				continue
			}
			if overlapAny(a.Domains, b.Domains) {
				add(a, b, "error", "overlapping domain selectors with different actions")
			}
			for _, ca := range a.CIDRs {
				for _, cb := range b.CIDRs {
					fa, fb := family(ca), family(cb)
					if fa != fb {
						add(a, b, "warning", fmt.Sprintf("selectors %s and %s are different address families", ca, cb))
						continue
					}
					if ca == cb {
						add(a, b, "error", fmt.Sprintf("identical CIDR selector %s with different actions", ca))
					}
				}
			}
			if overlapAny(a.Ports, b.Ports) && (len(a.Domains) == 0) == (len(b.Domains) == 0) && (len(a.CIDRs) == 0) == (len(b.CIDRs) == 0) &&
				len(a.Domains) == 0 && len(b.Domains) == 0 && len(a.CIDRs) == 0 && len(b.CIDRs) == 0 {
				// Port-only rules that overlap and differ in action
				// are ambiguous for shared selector space.
				add(a, b, "error", "overlapping port-only selectors with different actions")
			}
		}
	}
	return out
}

func family(cidr string) string {
	if strings.Contains(cidr, ":") {
		return "6"
	}
	return "4"
}

func overlapAny(a, b []string) bool {
	set := map[string]bool{}
	for _, v := range a {
		set[v] = true
	}
	for _, v := range b {
		if set[v] {
			return true
		}
	}
	return false
}

// Compile converts enabled policies into deterministic routing
// rules. Errors (error-severity conflicts) abort compilation.
// Rule order: priority ascending, then id — stable across runs so
// diffs stay meaningful.
func Compile(policies []Policy) ([]RoutingRule, error) {
	if conflicts := DetectConflicts(policies); conflicts != nil {
		for _, c := range conflicts {
			if c.Severity == "error" {
				return nil, fmt.Errorf("policy: conflict %s vs %s: %s", c.A, c.B, c.Reason)
			}
		}
	}
	enabled := make([]Policy, 0, len(policies))
	for _, p := range policies {
		if p.Enabled {
			if err := p.Validate(); err != nil {
				return nil, fmt.Errorf("policy %s: %w", p.ID, err)
			}
			enabled = append(enabled, p)
		}
	}
	sort.Slice(enabled, func(i, j int) bool {
		if enabled[i].Priority != enabled[j].Priority {
			return enabled[i].Priority < enabled[j].Priority
		}
		return enabled[i].ID < enabled[j].ID
	})
	rules := make([]RoutingRule, 0, len(enabled))
	for _, p := range enabled {
		tag := outboundTagFor(p)
		rule := RoutingRule{Type: "field", OutboundTag: tag}
		rule.Domains = append([]string(nil), p.Domains...)
		rule.IPs = append([]string(nil), p.CIDRs...)
		if len(p.Ports) > 0 {
			rule.Port = strings.Join(p.Ports, ",")
		}
		if len(p.InboundTags) > 0 {
			rule.InboundTag = append([]string(nil), p.InboundTags...)
		}
		rules = append(rules, rule)
	}
	return rules, nil
}

func outboundTagFor(p Policy) string {
	switch p.Action {
	case PolicyDirect:
		return "direct"
	case PolicyBlock:
		return "block"
	case PolicyProxy, PolicyOutbound:
		// Validate() guarantees a tag for both actions.
		return p.OutboundTag
	}
	return "direct"
}
