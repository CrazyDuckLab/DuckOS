package network

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func policyFixture() Config {
	c := fixture()
	c.Routes = append(c.Routes, Route{ID: "selected-uplink", Interface: "uplink", Destination: "0.0.0.0/0", Gateway: "192.0.2.1", Table: 10000, Metric: 100})
	c.PolicyRules = []PolicyRule{{ID: "staff-egress", Family: "ipv4", Priority: 20000, Table: 10000, From: "198.51.100.0/24"}}
	return c
}

func TestPolicyRenderingPairsLookupWithUnreachable(t *testing.T) {
	c := policyFixture()
	p, err := Render(c)
	if err != nil {
		t.Fatal(err)
	}
	lan := p.Networkd["10-duckos-inside.network"]
	for _, want := range []string{
		"Family=ipv4\nPriority=20000\nFrom=198.51.100.0/24\nTable=10000\n",
		"Family=ipv4\nPriority=20001\nFrom=198.51.100.0/24\nType=unreachable\n",
		"To=198.51.100.0/24\nPriority=10002\nTable=254\nSuppressPrefixLength=23\n",
		"To=198.51.100.0/24\nPriority=10003\nType=unreachable\n",
		"To=2001:db8:1::/64\nPriority=10004\nTable=254\nSuppressPrefixLength=63\n",
	} {
		if !strings.Contains(lan, want) {
			t.Fatal("missing fail-closed policy section", want)
		}
	}
	wan := p.Networkd["10-duckos-uplink.network"]
	if strings.Contains(wan, "[RoutingPolicyRule]") || !strings.Contains(wan, "Table=10000\nGateway=192.0.2.1\nGatewayOnLink=yes\n") {
		t.Fatal("policy tied to transient WAN or private gateway not explicit")
	}
	if strings.Count(lan, "Type=unreachable\n") != 4 {
		t.Fatal("missing per-lookup fallback guards")
	}
	before, _ := json.Marshal(c)
	c.Interfaces[0], c.Interfaces[1] = c.Interfaces[1], c.Interfaces[0]
	other, err := Render(c)
	if err != nil || other.Networkd["10-duckos-inside.network"] != lan {
		t.Fatal("input interface order changed policy priority allocation", err)
	}
	var roundtrip Config
	if err := json.Unmarshal(before, &roundtrip); err != nil {
		t.Fatal(err)
	}
	if err := roundtrip.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestPolicyIPv6AndValidation(t *testing.T) {
	c := policyFixture()
	c.Routes = append(c.Routes, Route{ID: "selected-v6", Interface: "uplink", Destination: "::/0", Gateway: "fe80::1", Table: 10001, Metric: 100})
	c.PolicyRules = append(c.PolicyRules, PolicyRule{ID: "staff-v6", Family: "ipv6", Priority: 20002, Table: 10001, From: "2001:db8:1::/64", To: "2001:db8:2::/64"})
	p, err := Render(c)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(p.Networkd["10-duckos-inside.network"], "Family=ipv6\nPriority=20003\nFrom=2001:db8:1::/64\nTo=2001:db8:2::/64\nType=unreachable\n") {
		t.Fatal("IPv6 fallback guard differs from selector")
	}
	for name, mutate := range map[string]func(*Config){
		"foreign table":            func(c *Config) { c.Routes[1].Table = 100 },
		"kernel priority":          func(c *Config) { c.PolicyRules[0].Priority = 32766 },
		"reserved paired priority": func(c *Config) { c.PolicyRules[0].Priority = 20001 },
		"family mismatch":          func(c *Config) { c.PolicyRules[0].Family = "ipv6" },
		"noncanonical prefix":      func(c *Config) { c.PolicyRules[0].From = "198.51.100.1/24" },
		"missing route":            func(c *Config) { c.PolicyRules[0].Table = 10001 },
		"no selector":              func(c *Config) { c.PolicyRules[0].From = "" },
		"injection":                func(c *Config) { c.PolicyRules[0].From = "0.0.0.0/0\nType=unicast" },
		"duplicate route":          func(c *Config) { r := c.Routes[1]; r.ID = "duplicate"; c.Routes = append(c.Routes, r) },
		"duplicate priority":       func(c *Config) { r := c.PolicyRules[0]; r.ID = "duplicate"; c.PolicyRules = append(c.PolicyRules, r) },
	} {
		t.Run(name, func(t *testing.T) {
			c := policyFixture()
			mutate(&c)
			if c.Validate() == nil {
				t.Fatal("unsafe policy accepted")
			}
		})
	}
}

func TestPolicyRequiresIndependentRootEnrollment(t *testing.T) {
	e := syntheticEnrollment()
	c := policyFixture()
	live := HostSnapshot{Version: 1, CapturedAt: time.Now().UTC(), Links: []HostLink{
		{Name: "testwan0", Index: 2, MAC: e.Physical[0].MAC, MTU: 1500},
		{Name: "testlan0", Index: 3, MAC: e.Physical[1].MAC, MTU: 1500},
	}}
	if e.CheckBindings(c, c, live) == nil {
		t.Fatal("API intent granted private table ownership")
	}
	e.RoutingTables = []int{10000}
	if e.CheckBindings(c, c, live) == nil {
		t.Fatal("table grant implicitly granted rule priority ranges")
	}
	e.PolicyRouting = true
	if err := e.CheckBindings(c, c, live); err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(fixture())
	next, _ := json.Marshal(c)
	if _, _, err := firewallPair(before, next); err == nil {
		t.Fatal("firewall-only adapter silently accepted policy mutation")
	}
}

func TestExpandedPolicyPlanHasHardArtifactBound(t *testing.T) {
	c := policyFixture()
	for i := 0; i < 62; i++ {
		link := Interface{ID: fmt.Sprintf("extra-%d", i), Name: fmt.Sprintf("testextra%d", i), Kind: "physical", Role: "lan", MTU: 1500, DHCPClient: "none"}
		for j := 0; j < 16; j++ {
			link.Addresses = append(link.Addresses, fmt.Sprintf("2001:db8:%x:%x:ffff:ffff:ffff:ffff/128", 4096+i, 4096+j))
		}
		c.Interfaces = append(c.Interfaces, link)
	}
	c.PolicyRules = nil
	for i := 0; i < 512; i++ {
		c.PolicyRules = append(c.PolicyRules, PolicyRule{ID: fmt.Sprintf("policy-%d", i), Family: "ipv4", Priority: 20000 + i*2, Table: 10000, From: "198.51.100.0/24"})
	}
	if err := c.Validate(); err != nil {
		t.Fatal("synthetic limit fixture invalid", err)
	}
	if _, err := Render(c); !errors.Is(err, ErrPlanLimit) {
		t.Fatal("expanded artifact limit was not enforced", err)
	}
}
