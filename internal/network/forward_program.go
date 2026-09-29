package network

import (
	"fmt"
	"net/netip"
	"sort"
	"strings"
)

// forwardProgram is shared by nftables generation and offline evaluation.
// It does not describe input-chain, bridge or application-proxy decisions.
type forwardProgram struct {
	filters []forwardFilter
	dnat    []destinationNAT
	snat    []sourceNAT
}
type forwardFilter struct {
	id, kind, from, to, protocol, action string
	port                                 int
	target                               netip.Addr
}
type destinationNAT struct {
	id, ingress, protocol string
	port, targetPort      int
	target                netip.Addr
}
type sourceNAT struct {
	egress, sourceInterface string
	source                  netip.Prefix
}

// Config.Validate must precede compilation. IDs retain provenance; Linux names
// are resolved only at the renderer boundary.
func compileForward(c Config) forwardProgram {
	p := forwardProgram{filters: []forwardFilter{{kind: "invalid", action: "drop"}, {kind: "established-related", action: "accept"}}}
	for _, r := range c.Firewall.Rules {
		p.filters = append(p.filters, forwardFilter{id: r.ID, kind: "rule", from: r.From, to: r.To, protocol: r.Protocol, port: r.DestinationPort, action: r.Action})
	}
	for _, f := range c.Firewall.Forwards {
		target := netip.MustParseAddr(f.Target)
		p.filters = append(p.filters, forwardFilter{id: f.ID, kind: "dnat", from: f.WAN, protocol: f.Protocol, port: f.TargetPort, target: target, action: "accept"})
		p.dnat = append(p.dnat, destinationNAT{id: f.ID, ingress: f.WAN, protocol: f.Protocol, port: f.Port, targetPort: f.TargetPort, target: target})
	}
	ordered := append([]Interface(nil), c.Interfaces...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].ID < ordered[j].ID })
	for _, egress := range c.Firewall.Masquerade {
		for _, link := range ordered {
			if link.Role != "lan" {
				continue
			}
			for _, raw := range link.Addresses {
				prefix := netip.MustParsePrefix(raw)
				if prefix.Addr().Is4() {
					p.snat = append(p.snat, sourceNAT{egress: egress, sourceInterface: link.ID, source: prefix.Masked()})
				}
			}
		}
	}
	return p
}

func (p forwardProgram) render(b *strings.Builder, links map[string]Interface) {
	b.WriteString(" }\n chain forward {\n  type filter hook forward priority filter; policy drop;\n")
	for _, r := range p.filters {
		switch r.kind {
		case "invalid":
			b.WriteString("  ct state invalid drop\n")
		case "established-related":
			b.WriteString("  ct state established,related accept\n")
		case "rule":
			fmt.Fprintf(b, "  iifname %q oifname %q", links[r.from].Name, links[r.to].Name)
			if r.protocol != "any" {
				fmt.Fprintf(b, " meta l4proto %s", r.protocol)
			}
			if r.port != 0 {
				fmt.Fprintf(b, " %s dport %d", r.protocol, r.port)
			}
			fmt.Fprintf(b, " counter %s\n", r.action)
		case "dnat":
			fmt.Fprintf(b, "  iifname %q ct status dnat ip daddr %s %s dport %d accept\n", links[r.from].Name, r.target, r.protocol, r.port)
		}
	}
	b.WriteString(" }\n chain prerouting {\n  type nat hook prerouting priority dstnat; policy accept;\n")
	for _, r := range p.dnat {
		fmt.Fprintf(b, "  iifname %q fib daddr type local %s dport %d dnat ip to %s:%d\n", links[r.ingress].Name, r.protocol, r.port, r.target, r.targetPort)
	}
	b.WriteString(" }\n chain postrouting {\n  type nat hook postrouting priority srcnat; policy accept;\n")
	for _, r := range p.snat {
		fmt.Fprintf(b, "  oifname %q ip saddr %s masquerade\n", links[r.egress].Name, r.source)
	}
	b.WriteString(" }\n}\n")
}
