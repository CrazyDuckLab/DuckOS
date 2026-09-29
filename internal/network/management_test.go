package network

import (
	"encoding/json"
	"strings"
	"testing"
)

func edgeFixture() Config {
	return Config{Version: 1,
		Interfaces: []Interface{{ID: "uplink", Name: "testedge0", Role: "wan", Kind: "physical", MTU: 1500, DHCPClient: "ipv4"}},
		Management: &ManagementPolicy{Rules: []ManagementRule{
			{ID: "admin-ssh", Interface: "uplink", Service: "ssh", Port: 2222, Sources: []string{"203.0.113.5/32", "2001:db8:10::/64"}},
			{ID: "admin-web", Interface: "uplink", Service: "web", Port: 8443, Sources: []string{"203.0.113.5/32"}},
		}},
	}
}

func TestEdgeManagementIsIndependentOfRole(t *testing.T) {
	c := edgeFixture()
	p, err := Render(c)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`iifname "testedge0" ip saddr 203.0.113.5/32 tcp dport 2222`,
		`iifname "testedge0" ip6 saddr 2001:db8:10::/64 tcp dport 2222`,
		`iifname "testedge0" ip saddr 203.0.113.5/32 tcp dport 8443`,
		`comment "management:admin-ssh:ssh"`,
	} {
		if !strings.Contains(p.NFTables, want) {
			t.Fatal("missing source-scoped management rule", want)
		}
	}
	if p.DNSMasq != "" || strings.Contains(p.NFTables, "dport { 22, 443 }") || strings.Contains(p.NFTables, "dport 53") {
		t.Fatal("edge node acquired implicit listeners or management access")
	}
	if c.policyAnchor() != "uplink" {
		t.Fatal("policy compilation still requires physical LAN")
	}
	// A topology role change does not itself change management authorization.
	c.Interfaces[0].Role = "lan"
	c.Interfaces[0].DHCPClient = "none"
	c.Interfaces[0].Addresses = []string{"192.0.2.2/24"}
	q, err := Render(c)
	if err != nil || !strings.Contains(q.NFTables, `ip saddr 203.0.113.5/32 tcp dport 2222`) {
		t.Fatal("management authorization depended on role", err)
	}
}

func TestManagementRejectsAmbiguousOrPublicGrants(t *testing.T) {
	for name, change := range map[string]func(*Config){
		"no grants":            func(c *Config) { c.Management.Rules = nil },
		"legacy mixed":         func(c *Config) { c.Interfaces[0].Management = true },
		"unknown link":         func(c *Config) { c.Management.Rules[0].Interface = "missing" },
		"no address":           func(c *Config) { c.Interfaces[0].DHCPClient = "none" },
		"public v4":            func(c *Config) { c.Management.Rules[0].Sources = []string{"0.0.0.0/0"} },
		"public v6":            func(c *Config) { c.Management.Rules[0].Sources = []string{"::/0"} },
		"missing source":       func(c *Config) { c.Management.Rules[0].Sources = nil },
		"noncanonical":         func(c *Config) { c.Management.Rules[0].Sources = []string{"203.0.113.5/24"} },
		"mapped":               func(c *Config) { c.Management.Rules[0].Sources = []string{"::ffff:203.0.113.5/128"} },
		"injection":            func(c *Config) { c.Management.Rules[0].Sources = []string{"203.0.113.5/32 accept"} },
		"duplicate":            func(c *Config) { c.Management.Rules[0].Sources = []string{"203.0.113.5/32", "203.0.113.5/32"} },
		"service":              func(c *Config) { c.Management.Rules[0].Service = "subscription" },
		"port":                 func(c *Config) { c.Management.Rules[0].Port = 65536 },
		"DNS without listener": func(c *Config) { c.DNS.Upstreams = []string{"192.0.2.53"} },
	} {
		t.Run(name, func(t *testing.T) {
			c := edgeFixture()
			change(&c)
			if c.Validate() == nil {
				t.Fatal("unsafe or unsupported intent accepted")
			}
		})
	}
}

func TestManagementRoundTripAndFirewallOnlyRefusal(t *testing.T) {
	c := edgeFixture()
	raw, _ := json.Marshal(c)
	d, err := Decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := Render(c)
	b, _ := Render(d)
	if a.NFTables != b.NFTables {
		t.Fatal("management policy lost in canonical round trip")
	}
	d.Management.Rules[0].Port = 2223
	next, _ := json.Marshal(d)
	if _, _, err := firewallPair(raw, next); err == nil {
		t.Fatal("management change bypassed composite recovery gate")
	}
	legacy := fixture()
	l, err := Render(legacy)
	if err != nil || !strings.Contains(l.NFTables, `iifname "testlan0" tcp dport { 22, 443 } accept`) {
		t.Fatal("legacy configuration changed without migration", err)
	}
}
