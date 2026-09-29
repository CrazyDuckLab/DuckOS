package network

import (
	"errors"
	"fmt"
	"net/netip"
	"sort"
	"strings"
)

// PolicyRule is the bounded IP-level execution intent. Higher-level traffic
// policies and route groups will resolve into these owned table references;
// domain/SNI classification is not inferred by this low-level compiler.
type PolicyRule struct {
	ID       string `json:"id"`
	Family   string `json:"family"`
	Priority int    `json:"priority"`
	Table    int    `json:"table"`
	From     string `json:"from,omitempty"`
	To       string `json:"to,omitempty"`
}

func privateRouteTable(table int) bool { return table >= 10000 && table <= 10999 }

func (c Config) validatePolicy(addID func(string) error) error {
	if len(c.PolicyRules) > 512 {
		return errors.New("policy rule count exceeds limit")
	}
	tables := map[string]bool{}
	for _, r := range c.Routes {
		prefix, _ := netip.ParsePrefix(r.Destination)
		tables[fmt.Sprintf("%d/%t", r.Table, prefix.Addr().Is4())] = true
	}
	priorities := map[int]bool{}
	for _, rule := range c.PolicyRules {
		if err := addID(rule.ID); err != nil {
			return err
		}
		if (rule.Family != "ipv4" && rule.Family != "ipv6") || !privateRouteTable(rule.Table) || rule.Priority < 20000 || rule.Priority > 21998 || rule.Priority%2 != 0 || priorities[rule.Priority] || (rule.From == "" && rule.To == "") {
			return errors.New("invalid policy identity, selector or reserved priority pair")
		}
		priorities[rule.Priority] = true
		if !tables[fmt.Sprintf("%d/%t", rule.Table, rule.Family == "ipv4")] {
			return errors.New("policy requires a configured route in its selected table and family")
		}
		for _, value := range []string{rule.From, rule.To} {
			if value == "" {
				continue
			}
			p, err := netip.ParsePrefix(value)
			if err != nil || p != p.Masked() || p.Addr().Is4In6() || p.Addr().Is4() != (rule.Family == "ipv4") || p.Addr().IsMulticast() {
				return errors.New("invalid policy address prefix or family")
			}
		}
	}
	if len(c.PolicyRules) > 0 {
		for _, link := range c.Interfaces {
			for _, value := range link.Addresses {
				p, _ := netip.ParsePrefix(value)
				if p.Bits() == 0 {
					return errors.New("policy routing cannot protect a zero-width connected subnet")
				}
			}
		}
	}
	return nil
}

func (c Config) policyAnchor() string {
	var anchor string
	if c.Management != nil {
		for _, rule := range c.Management.Rules {
			if anchor == "" || rule.Interface < anchor {
				anchor = rule.Interface
			}
		}
		return anchor
	}
	for _, i := range c.Interfaces {
		if i.Management && (anchor == "" || i.ID < anchor) {
			anchor = i.ID
		}
	}
	return anchor
}

// renderPolicy emits paired rules: a lookup followed by an unreachable guard
// for the same selector. Empty/failed private tables must not fall through to
// main. Connected networks are protected first, also with no default fallback.
// The host adapter must install/verify these under a traffic guard; networkd
// reload itself is not atomic and this compiler does not claim otherwise.
type policyInstruction struct {
	id, family, from, to      string
	priority, table, suppress int
	unreachable               bool
}

func (c Config) compilePolicy() []policyInstruction {
	if len(c.PolicyRules) == 0 {
		return nil
	}
	var program []policyInstruction
	connected := map[string]bool{}
	for _, link := range c.Interfaces {
		for _, value := range link.Addresses {
			p, _ := netip.ParsePrefix(value)
			connected[p.Masked().String()] = true
		}
	}
	prefixes := make([]string, 0, len(connected))
	for prefix := range connected {
		prefixes = append(prefixes, prefix)
	}
	sort.Strings(prefixes)
	for index, prefix := range prefixes {
		p, _ := netip.ParsePrefix(prefix)
		family := "ipv6"
		if p.Addr().Is4() {
			family = "ipv4"
		}
		priority := 10000 + index*2
		program = append(program,
			policyInstruction{id: "connected:" + prefix, family: family, to: prefix, priority: priority, table: 254, suppress: p.Bits() - 1},
			policyInstruction{id: "connected:" + prefix, family: family, to: prefix, priority: priority + 1, suppress: -1, unreachable: true})
	}
	rules := append([]PolicyRule(nil), c.PolicyRules...)
	sort.Slice(rules, func(i, j int) bool { return rules[i].Priority < rules[j].Priority })
	for _, rule := range rules {
		for offset := 0; offset < 2; offset++ {
			program = append(program, policyInstruction{id: rule.ID, family: rule.Family, from: rule.From, to: rule.To, priority: rule.Priority + offset, table: rule.Table, suppress: -1, unreachable: offset == 1})
		}
	}
	return program
}

func (c Config) renderPolicy(b *strings.Builder) {
	for _, r := range c.compilePolicy() {
		fmt.Fprintf(b, "\n[RoutingPolicyRule]\nFamily=%s\n", r.family)
		// Preserve deterministic artifact bytes for existing connected guards.
		if r.suppress >= 0 || strings.HasPrefix(r.id, "connected:") {
			fmt.Fprintf(b, "To=%s\nPriority=%d\n", r.to, r.priority)
		} else {
			fmt.Fprintf(b, "Priority=%d\n", r.priority)
			if r.from != "" {
				fmt.Fprintf(b, "From=%s\n", r.from)
			}
			if r.to != "" {
				fmt.Fprintf(b, "To=%s\n", r.to)
			}
		}
		if r.unreachable {
			b.WriteString("Type=unreachable\n")
		} else {
			fmt.Fprintf(b, "Table=%d\n", r.table)
			if r.suppress >= 0 {
				fmt.Fprintf(b, "SuppressPrefixLength=%d\n", r.suppress)
			}
		}
	}
}
