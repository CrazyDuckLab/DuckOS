package network

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func fixture() Config {
	return Config{Version: 1, Interfaces: []Interface{
		{ID: "uplink", Name: "testwan0", Kind: "physical", Role: "wan", MTU: 1500, DHCPClient: "ipv4", Addresses: []string{"192.0.2.2/24"}},
		{ID: "inside", Name: "testlan0", Kind: "physical", Role: "lan", MTU: 1500, DHCPClient: "none", Management: true, Addresses: []string{"198.51.100.1/24", "2001:db8:1::1/64"}, DHCPServer: &DHCPServer{Start: "198.51.100.100", End: "198.51.100.200", LeaseSeconds: 3600}},
	}, Routes: []Route{{ID: "default", Interface: "uplink", Destination: "0.0.0.0/0", Gateway: "192.0.2.1", Table: 254}}, DNS: DNS{Upstreams: []string{"192.0.2.53"}, Records: []Record{{Name: "router.test", Address: "198.51.100.1"}}}, Firewall: Firewall{Masquerade: []string{"uplink"}, Rules: []Rule{{ID: "internet", From: "inside", To: "uplink", Action: "accept", Protocol: "any"}}, Forwards: []Forward{{ID: "website", WAN: "uplink", Protocol: "tcp", Port: 8443, Target: "198.51.100.10", TargetPort: 443}}}}
}
func TestRenderManagedArtifacts(t *testing.T) {
	c := fixture()
	p, err := Render(c)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"policy drop;", "fib daddr type local tcp dport 8443 dnat ip to 198.51.100.10:443", `iifname "testlan0" oifname "testwan0" counter accept`, `oifname "testwan0" ip saddr 198.51.100.0/24 masquerade`} {
		if !strings.Contains(p.NFTables, required) {
			t.Errorf("missing %s", required)
		}
	}
	if strings.Contains(p.NFTables, "flush ruleset") || strings.Contains(p.NFTables, `iifname "testwan0" tcp dport { 22, 443 }`) {
		t.Fatal("unsafe firewall output")
	}
	if !strings.Contains(p.DNSMasq, "dhcp-range=set:inside,198.51.100.100,198.51.100.200,255.255.255.0,3600s") || strings.Contains(p.DNSMasq, "interface=testwan0") {
		t.Fatal(p.DNSMasq)
	}
	if !strings.Contains(p.Networkd["10-duckos-inside.network"], "Address=2001:db8:1::1/64") {
		t.Fatal("lost IPv6")
	}
	encoded, _ := json.Marshal(c)
	decoded, err := Decode(encoded)
	if err != nil {
		t.Fatal(err)
	}
	q, err := Render(decoded)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := json.Marshal(p)
	b, _ := json.Marshal(q)
	if string(a) != string(b) {
		t.Fatal("nondeterministic rendering")
	}
}
func TestEmptyConfigurationHasNoActiveServicesOrFirewallHooks(t *testing.T) {
	p, err := Render(Config{Version: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Networkd) != 0 || p.DNSMasq != "" || p.NFTables != "destroy table inet duckos\n" {
		t.Fatal("empty baseline would affect existing host networking", p)
	}
	if _, err = Render(Config{Version: 1, DNS: DNS{Upstreams: []string{"192.0.2.53"}}}); err == nil {
		t.Fatal("silently ignored DNS without a managed LAN")
	}
}

func TestRejectNetworkConflictsAndInjection(t *testing.T) {
	cases := map[string]func(*Config){
		"name injection":           func(c *Config) { c.Interfaces[0].Name = "eth0\nDNS=x" },
		"shell name":               func(c *Config) { c.Interfaces[0].Name = "$(id)" },
		"duplicate id":             func(c *Config) { c.Interfaces[0].ID = "inside" },
		"overlap":                  func(c *Config) { c.Interfaces[0].Addresses = []string{"198.51.100.2/24"} },
		"management WAN":           func(c *Config) { c.Interfaces[0].Management = true },
		"no management":            func(c *Config) { c.Interfaces[1].Management = false },
		"bad pool":                 func(c *Config) { c.Interfaces[1].DHCPServer.Start = "198.51.100.0" },
		"router in pool":           func(c *Config) { c.Interfaces[1].DHCPServer.Start = "198.51.100.1" },
		"broadcast in pool":        func(c *Config) { c.Interfaces[1].DHCPServer.End = "198.51.100.255" },
		"invalid protocol":         func(c *Config) { c.Firewall.Forwards[0].Protocol = "tcp; accept" },
		"unmanaged forward":        func(c *Config) { c.Firewall.Forwards[0].Target = "203.0.113.2" },
		"router forward":           func(c *Config) { c.Firewall.Forwards[0].Target = "198.51.100.1" },
		"broadcast forward":        func(c *Config) { c.Firewall.Forwards[0].Target = "198.51.100.255" },
		"network address forward":  func(c *Config) { c.Firewall.Forwards[0].Target = "198.51.100.0" },
		"invalid route":            func(c *Config) { c.Routes[0].Gateway = "203.0.113.1" },
		"noncanonical route":       func(c *Config) { c.Routes[0].Destination = "192.0.2.1/24" },
		"unsupported policy table": func(c *Config) { c.Routes[0].Table = 100 },
		"dns injection":            func(c *Config) { c.DNS.Records[0].Name = "x\ninterface=eth0" },
		"ipv4 mapped IPv6":         func(c *Config) { c.DNS.Upstreams = []string{"::ffff:192.0.2.1"} },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			c := fixture()
			change(&c)
			if _, err := Render(c); err == nil {
				t.Fatal("invalid configuration rendered")
			}
		})
	}
}

func TestDHCPReservationRefusesRouterAliasesAndNonUnicastMACs(t *testing.T) {
	for _, mac := range []string{"00:00:00:00:00:00", "ff:ff:ff:ff:ff:ff", "01:00:5e:00:00:01"} {
		c := fixture()
		c.Interfaces[1].DHCPServer.Reservations = []Reservation{{MAC: mac, Address: "198.51.100.50", Hostname: "client"}}
		if c.Validate() == nil {
			t.Fatal("invalid client MAC accepted", mac)
		}
	}
	c := fixture()
	c.Interfaces[1].Addresses = append(c.Interfaces[1].Addresses, "198.51.100.2/24")
	c.Interfaces[1].DHCPServer.Reservations = []Reservation{{MAC: "02:00:00:00:00:01", Address: "198.51.100.2", Hostname: "client"}}
	if c.Validate() == nil {
		t.Fatal("secondary router address leased to client")
	}
	c.Interfaces[1].DHCPServer.Reservations[0].Address = "198.51.100.50"
	if err := c.Validate(); err != nil {
		t.Fatal("valid locally administered unicast MAC rejected", err)
	}
}
func TestVLANRender(t *testing.T) {
	c := fixture()
	c.Interfaces = append(c.Interfaces, Interface{ID: "guest", Name: "testguest0", Kind: "vlan", Parent: "inside", VLAN: 20, Role: "lan", MTU: 1500, DHCPClient: "none", Addresses: []string{"203.0.113.1/24"}})
	p, err := Render(c)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(p.Networkd["10-duckos-inside.network"], "VLAN=testguest0") || !strings.Contains(p.Networkd["10-duckos-guest.netdev"], "Id=20") {
		t.Fatal("missing VLAN association")
	}
	c.Interfaces[2].Parent = "guest"
	if c.Validate() == nil {
		t.Fatal("accepted VLAN cycle")
	}
}
func FuzzDecodeRender(f *testing.F) {
	data, _ := json.Marshal(fixture())
	f.Add(data)
	edge, _ := json.Marshal(edgeFixture())
	f.Add(edge)
	f.Add([]byte(`{}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		c, err := Decode(data)
		if err != nil {
			return
		}
		p, err := Render(c)
		if errors.Is(err, ErrPlanLimit) {
			return
		} // Valid intent may exceed bounded expanded output.
		if err != nil {
			t.Fatal(err)
		}
		for name := range p.Networkd {
			if strings.ContainsAny(name, "/\\\n") {
				t.Fatal("unsafe generated path")
			}
		}
	})
}
