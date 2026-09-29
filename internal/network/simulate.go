package network

import (
	"errors"
	"fmt"
	"net/netip"
)

var ErrTrafficQuestion = errors.New("invalid traffic simulation question")

// TrafficQuestion describes a new routed unicast flow, not an observed packet.
// Ingress is explicit: an IP alone cannot prove client or network membership.
// A zero port means unspecified, never an invented default service port.
type TrafficQuestion struct {
	Ingress         string `json:"ingress"`
	Source          string `json:"source"`
	Destination     string `json:"destination"`
	Protocol        string `json:"protocol"`
	Port            int    `json:"port,omitempty"`
	ConnectionState string `json:"connectionState"`
}

type TrafficStep struct {
	Stage    string `json:"stage"`
	Code     string `json:"code"`
	ObjectID string `json:"objectId,omitempty"`
	Detail   string `json:"detail"`
	Table    int    `json:"table,omitempty"`
	Priority int    `json:"priority,omitempty"`
}

type TrafficExplanation struct {
	ConfigDigest    string        `json:"configDigest"`
	Basis           string        `json:"basis"`
	Status          string        `json:"status"`
	ManagedDecision string        `json:"managedDecision"`
	Reachability    string        `json:"reachability"`
	Summary         string        `json:"summary"`
	Egress          string        `json:"egress,omitempty"`
	Gateway         string        `json:"gateway,omitempty"`
	Destination     string        `json:"destination"`
	Port            int           `json:"port,omitempty"`
	Steps           []TrafficStep `json:"steps"`
	Assumptions     []string      `json:"assumptions"`
}

func (e *TrafficExplanation) step(stage, code, id, detail string) {
	e.Steps = append(e.Steps, TrafficStep{Stage: stage, Code: code, ObjectID: id, Detail: detail})
}
func (e *TrafficExplanation) unknown(stage, code, detail string) (TrafficExplanation, error) {
	e.Status, e.ManagedDecision, e.Summary = "unknown", "unknown", detail
	e.step(stage, code, "", detail)
	return *e, nil
}
func (e *TrafficExplanation) finish(decision, summary string) (TrafficExplanation, error) {
	// Even a definite decision of this candidate is conditional on unobserved
	// kernel state. It must not become a live reachability green checkmark.
	e.Status, e.ManagedDecision, e.Summary = "conditional", decision, summary
	return *e, nil
}

// Simulate evaluates only the supplied candidate. It never executes commands,
// opens sockets, resolves DNS, changes configuration or confirms Safe Apply.
func Simulate(c Config, q TrafficQuestion) (TrafficExplanation, error) {
	if err := c.Validate(); err != nil {
		return TrafficExplanation{}, err
	}
	src, se := netip.ParseAddr(q.Source)
	dst, de := netip.ParseAddr(q.Destination)
	if se != nil || de != nil || !usable(src) || !usable(dst) || src.Is4() != dst.Is4() || q.Port < 0 || q.Port > 65535 ||
		(q.Protocol != "tcp" && q.Protocol != "udp" && q.Protocol != "icmp" && q.Protocol != "ipv6-icmp") ||
		(q.Protocol == "icmp" && !src.Is4()) || (q.Protocol == "ipv6-icmp" && src.Is4()) ||
		((q.Protocol == "icmp" || q.Protocol == "ipv6-icmp") && q.Port != 0) ||
		(q.ConnectionState != "new" && q.ConnectionState != "established" && q.ConnectionState != "related" && q.ConnectionState != "invalid") {
		return TrafficExplanation{}, ErrTrafficQuestion
	}
	links := make(map[string]Interface, len(c.Interfaces))
	for _, link := range c.Interfaces {
		links[link.ID] = link
	}
	if _, ok := links[q.Ingress]; !ok {
		return TrafficExplanation{}, ErrTrafficQuestion
	}
	e := TrafficExplanation{ConfigDigest: configFingerprint(c), Basis: "candidate-only", Reachability: "not-tested", Destination: dst.String(), Port: q.Port,
		Assumptions: []string{
			"The supplied candidate is installed unchanged; no external nftables chains, policy rules, routes or local addresses override it.",
			"The flow enters the stated interface and is routed through DuckOS, not switched directly at layer 2 or terminated by an application proxy.",
			"Forwarding, interfaces, next-hop resolution and return paths work; reverse-path filtering, MTU, TTL and upstream filtering are not evaluated.",
			"Source IP is a question input, not authenticated client identity. DNS/category policy and tunnel health are not evaluated.",
		}}
	e.step("identity", "explicit-ingress", q.Ingress, "Evaluate the supplied source IP on this interface; client ownership is not inferred.")
	e.step("dns", "literal-address", "", "Destination is an IP literal. No DNS query or category lookup was performed.")
	if src.IsLinkLocalUnicast() || dst.IsLinkLocalUnicast() || src.String() == "255.255.255.255" || dst.String() == "255.255.255.255" {
		return e.unknown("routing", "scoped-address", "Link-local and broadcast traffic requires a scope-specific packet model.")
	}
	if q.ConnectionState != "new" {
		e.Status, e.ManagedDecision, e.Summary = "unsupported", "unknown", "Existing or invalid connections require actual conntrack and packet-state observations."
		e.step("conntrack", "observation-required", "", e.Summary)
		return e, nil
	}
	for _, link := range c.Interfaces {
		for _, raw := range link.Addresses {
			p := netip.MustParsePrefix(raw)
			if p.Addr() == src {
				return e.unknown("identity", "local-source", "A router-originated flow uses a different packet path.")
			}
			if p.Contains(src) && !usableSubnetHost(p, src) || p.Contains(dst) && !usableSubnetHost(p, dst) {
				return e.unknown("identity", "non-unicast-subnet-address", "Network and broadcast destinations require a different packet model.")
			}
		}
	}
	program := compileForward(c)
	local := configuredLocal(c, dst)
	dnatted := false
	for _, r := range program.dnat {
		if r.ingress != q.Ingress || r.protocol != q.Protocol || !dst.Is4() {
			continue
		}
		if !local {
			if dynamicFamily(c, true) && (q.Port == 0 || q.Port == r.port) {
				return e.unknown("dnat", "local-address-unknown", "DHCP may supply a local address matching this port forward; its lease is not part of the candidate.")
			}
			continue
		}
		if q.Port == 0 {
			return e.unknown("dnat", "port-required", "Specify a destination port to decide whether port forwarding applies.")
		}
		if r.port != q.Port {
			continue
		}
		dst, q.Port, dnatted = r.target, r.targetPort, true
		e.Destination, e.Port = dst.String(), q.Port
		e.step("dnat", "destination-translated", r.id, fmt.Sprintf("Translate destination to %s:%d before route lookup and forward filtering.", dst, q.Port))
		break
	}
	if !dnatted {
		e.step("dnat", "not-matched", "", "No managed destination translation matched.")
	}
	if configuredLocal(c, dst) {
		e.Status, e.ManagedDecision, e.Summary = "unsupported", "unknown", "This destination belongs to DuckOS and uses the input chain, not forwarding."
		e.step("routing", "local-input", "", e.Summary)
		return e, nil
	}
	if dynamicFamily(c, dst.Is4()) {
		return e.unknown("routing", "dynamic-addresses-and-routes", "DHCP or router advertisements may change local addresses and route selection; their observed state is required.")
	}
	route, known, selected := selectTrafficRoute(c, src, dst, &e)
	if !known {
		return e.unknown("routing", "route-observation-required", "The route cannot be determined from this candidate alone; dynamic or ambiguous routing needs observed state.")
	}
	if !selected {
		return e.finish("drop", "The candidate has no permitted route for this destination. No ordinary-WAN fallback is assumed.")
	}
	e.Egress, e.Gateway = route.link, route.gateway
	e.step("route", "selected", route.id, fmt.Sprintf("Use interface %s for %s in table %d.", route.link, route.prefix, route.table))
	e.Steps[len(e.Steps)-1].Table = route.table
	for _, r := range program.filters {
		// A new valid flow cannot match the leading conntrack rules.
		if r.kind != "rule" && r.kind != "dnat" {
			continue
		}
		if r.from != q.Ingress || (r.kind == "rule" && r.to != route.link) || (r.protocol != "any" && r.protocol != q.Protocol) {
			continue
		}
		if r.kind == "dnat" && (!dnatted || r.target != dst) {
			continue
		}
		if r.port != 0 && q.Port == 0 {
			return e.unknown("firewall", "port-required", "A matching interface/protocol rule is port-specific. Specify the port before deciding access.")
		}
		if r.port != 0 && r.port != q.Port {
			continue
		}
		e.step("firewall", r.action, r.id, "First matching managed forward rule: "+r.action+".")
		if r.action == "drop" {
			return e.finish("drop", "A managed firewall rule blocks this new flow.")
		}
		for _, nat := range program.snat {
			if nat.egress == route.link && nat.source.Contains(src) {
				e.step("snat", "masquerade", nat.egress, "Masquerade the source on this egress; the exact translated address/port requires runtime state.")
				return e.finish("allow", "Managed forwarding permits this new flow with source NAT, subject to the listed runtime assumptions.")
			}
		}
		e.step("snat", "not-matched", "", "No managed source translation matched; a working return route is required.")
		return e.finish("allow", "Managed forwarding permits this new flow without source NAT, subject to the listed runtime assumptions.")
	}
	e.step("firewall", "default-drop", "", "No forward rule matched; the managed forward chain defaults to drop.")
	return e.finish("drop", "The managed firewall blocks this flow by default.")
}

func configuredLocal(c Config, addr netip.Addr) bool {
	for _, link := range c.Interfaces {
		for _, raw := range link.Addresses {
			if netip.MustParsePrefix(raw).Addr() == addr {
				return true
			}
		}
	}
	return false
}
func dynamicFamily(c Config, ipv4 bool) bool {
	for _, link := range c.Interfaces {
		if link.DHCPClient == "dual" || ipv4 && link.DHCPClient == "ipv4" || !ipv4 && (link.DHCPClient == "ipv6" || link.AcceptRA) {
			return true
		}
	}
	return false
}

type trafficRoute struct {
	id, link, gateway string
	prefix            netip.Prefix
	table, metric     int
	connected         bool
}

func lookupTrafficRoute(c Config, dst netip.Addr, table, suppress int) (trafficRoute, bool, bool) {
	// Dynamic leases and RA can add more-specific routes, not just defaults.
	if table == 254 && dynamicFamily(c, dst.Is4()) {
		return trafficRoute{}, false, false
	}
	var candidates []trafficRoute
	for _, r := range c.Routes {
		prefix := netip.MustParsePrefix(r.Destination)
		if r.Table == table && prefix.Contains(dst) && prefix.Bits() > suppress {
			candidates = append(candidates, trafficRoute{id: r.ID, link: r.Interface, gateway: r.Gateway, prefix: prefix, table: table, metric: effectiveRouteMetric(r)})
		}
	}
	if table == 254 {
		for _, link := range c.Interfaces {
			for _, raw := range link.Addresses {
				prefix := netip.MustParsePrefix(raw).Masked()
				if prefix.Contains(dst) && prefix.Bits() > suppress {
					candidates = append(candidates, trafficRoute{id: "connected:" + link.ID + ":" + prefix.String(), link: link.ID, prefix: prefix, table: 254, connected: true})
				}
			}
		}
	}
	var best trafficRoute
	found, ambiguous := false, false
	bestBits := -1
	for _, r := range candidates {
		if r.prefix.Bits() > bestBits {
			bestBits = r.prefix.Bits()
		}
	}
	connected, explicit := false, false
	for _, r := range candidates {
		if r.prefix.Bits() == bestBits {
			if r.connected {
				connected = true
			} else {
				explicit = true
			}
		}
	}
	if connected && explicit {
		return trafficRoute{}, false, false
	}
	for _, r := range candidates {
		if !found || r.prefix.Bits() > best.prefix.Bits() {
			best, found, ambiguous = r, true, false
			continue
		}
		if r.prefix.Bits() != best.prefix.Bits() {
			continue
		}
		if r.connected || best.connected {
			// Identical aliases on one interface are the same connected route.
			// Explicit-vs-connected metrics are not guessed across IP families.
			if !(r.connected && best.connected && r.link == best.link) {
				ambiguous = true
			}
			continue
		}
		if r.metric < best.metric {
			best, ambiguous = r, false
		} else if r.metric == best.metric {
			ambiguous = true
		}
	}
	return best, !ambiguous, found
}

func selectTrafficRoute(c Config, src, dst netip.Addr, e *TrafficExplanation) (trafficRoute, bool, bool) {
	family := "ipv6"
	if dst.Is4() {
		family = "ipv4"
	}
	for _, rule := range c.compilePolicy() {
		if rule.family != family || rule.from != "" && !netip.MustParsePrefix(rule.from).Contains(src) || rule.to != "" && !netip.MustParsePrefix(rule.to).Contains(dst) {
			continue
		}
		if rule.unreachable {
			e.step("policy-routing", "unreachable-guard", rule.id, "The selected lookup found no route. Its paired unreachable guard prevents fallback to main.")
			e.Steps[len(e.Steps)-1].Priority = rule.priority
			return trafficRoute{}, true, false
		}
		e.step("policy-routing", "table-lookup", rule.id, fmt.Sprintf("Matching selector looks up table %d at priority %d.", rule.table, rule.priority))
		e.Steps[len(e.Steps)-1].Table, e.Steps[len(e.Steps)-1].Priority = rule.table, rule.priority
		route, known, found := lookupTrafficRoute(c, dst, rule.table, rule.suppress)
		if !known || found {
			return route, known, found
		}
	}
	e.step("policy-routing", "main-lookup", "", "No earlier managed policy selected a route; evaluate main table 254.")
	e.Steps[len(e.Steps)-1].Table = 254
	return lookupTrafficRoute(c, dst, 254, -1)
}
