package network

import (
	"errors"
	"fmt"
	"net/netip"
	"sort"
	"strings"
)

// ManagementPolicy grants access independently of interface role. It does not
// configure listeners or prove that an administrator can reach them. A non-nil
// policy replaces legacy interface-wide grants rather than augmenting them.
type ManagementPolicy struct {
	Rules []ManagementRule `json:"rules"`
}

type ManagementRule struct {
	ID        string   `json:"id"`
	Interface string   `json:"interface"`
	Service   string   `json:"service"`
	Port      int      `json:"port"`
	Sources   []string `json:"sources"`
}

func (c Config) validateManagement(links map[string]Interface, addID func(string) error) error {
	if len(c.Management.Rules) > 64 || (len(c.Interfaces) > 0 && len(c.Management.Rules) == 0) {
		return errors.New("managed configuration requires a bounded explicit management grant")
	}
	grants := map[string]bool{}
	for _, rule := range c.Management.Rules {
		if err := addID(rule.ID); err != nil {
			return err
		}
		link, ok := links[rule.Interface]
		if !ok {
			return errors.New("management interface is not managed")
		}
		if len(link.Addresses) == 0 && link.DHCPClient == "none" && !link.AcceptRA {
			return errors.New("management interface requires an address acquisition method")
		}
		if (rule.Service != "ssh" && rule.Service != "web" && rule.Service != "api") || rule.Port < 1 || rule.Port > 65535 || len(rule.Sources) == 0 || len(rule.Sources) > 16 {
			return errors.New("invalid management service, port or source limit")
		}
		for _, raw := range rule.Sources {
			p, err := netip.ParsePrefix(raw)
			if err != nil || p != p.Masked() || p.String() != raw || p.Bits() == 0 || p.Addr().Is4In6() || p.Addr().IsMulticast() || p.Addr().IsLoopback() || p.Addr().IsUnspecified() {
				return errors.New("management source must be an explicit canonical trusted CIDR")
			}
			key := fmt.Sprintf("%s/%d/%s", rule.Interface, rule.Port, raw)
			if grants[key] {
				return errors.New("duplicate management transport grant")
			}
			grants[key] = true
		}
	}
	return nil
}

func (c Config) renderManagement(b *strings.Builder, links map[string]Interface) {
	if c.Management == nil {
		return
	}
	rules := append([]ManagementRule(nil), c.Management.Rules...)
	sort.Slice(rules, func(i, j int) bool { return rules[i].ID < rules[j].ID })
	for _, rule := range rules {
		sources := append([]string(nil), rule.Sources...)
		sort.Strings(sources)
		for _, source := range sources {
			prefix, _ := netip.ParsePrefix(source)
			family := "ip6"
			if prefix.Addr().Is4() {
				family = "ip"
			}
			fmt.Fprintf(b, "  iifname %q %s saddr %s tcp dport %d counter accept comment %q\n", links[rule.Interface].Name, family, source, rule.Port, "management:"+rule.ID+":"+rule.Service)
		}
	}
}
