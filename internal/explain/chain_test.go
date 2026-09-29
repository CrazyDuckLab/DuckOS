// explain/chain_test.go pins the V1 explain
// data model. The test exercises the Chain
// builder through a fake Engine that
// implements the success path, the missing-
// interface path, the no-route path, the
// firewall-deny path, and the tun/DNS/WAN
// observed-runtime path.
package explain

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

type fakeEngine struct {
	snap TopologySnapshot
	rts  []RouteInfo
	rls  []RuleInfo
	nat  []NATInfo
	tun  []TunnelInfo
	dns  map[string]string
	wans []WANInfo
}

func (f *fakeEngine) Topology() TopologySnapshot { return f.snap }
func (f *fakeEngine) Routes(src Endpoint) []RouteInfo {
	return f.rts
}
func (f *fakeEngine) Firewall(src, dst Endpoint) []RuleInfo {
	return f.rls
}
func (f *fakeEngine) NAT(src, dst Endpoint) []NATInfo      { return f.nat }
func (f *fakeEngine) TunnelsFor(src Endpoint) []TunnelInfo { return f.tun }
func (f *fakeEngine) DNS(name string) (string, bool) {
	v, ok := f.dns[name]
	return v, ok
}
func (f *fakeEngine) WANs() []WANInfo { return f.wans }

func TestChainValidEndpoint(t *testing.T) {
	good := Endpoint{Address: "192.0.2.1"}
	if !good.Valid() {
		t.Fatal("Valid endpoint rejected")
	}
	var empty Endpoint
	if empty.Valid() {
		t.Fatal("empty endpoint must be invalid")
	}
}

func TestChainMissingInterface(t *testing.T) {
	e := &fakeEngine{}
	b := NewBuilder(e, time.Now)
	c, err := b.Build(Endpoint{Address: "192.0.2.1"}, Endpoint{Address: "198.51.100.1"})
	if err != nil {
		t.Fatal(err)
	}
	if c.Verdict != DecisionStop {
		t.Fatalf("verdict: %q", c.Verdict)
	}
	if len(c.Hops) == 0 || c.Hops[0].Kind != HopInterface {
		t.Fatal("first hop must be the interface")
	}
}

func TestChainHappyPath(t *testing.T) {
	e := &fakeEngine{
		snap: TopologySnapshot{
			Interfaces: []InterfaceInfo{{Name: "lan0", Address: "192.0.2.1", Up: true, Kind: "lan"}},
			VLANS:      []VLANInfo{{ID: 10, Parent: "lan0", Up: true}},
			WANs:       []WANInfo{{ID: "wan1", Link: "wan0", Healthy: true, Uplink: "203.0.113.1"}},
			Devices:    []DeviceInfo{{ID: "dev-1", Links: []string{"lan0"}}},
		},
		rts:  []RouteInfo{{Destination: "0.0.0.0/0", Gateway: "203.0.113.254", Interface: "wan0", Table: 200, Policy: true, Observed: true}},
		rls:  []RuleInfo{{Chain: "forward", Action: "accept", Match: "all"}},
		nat:  []NATInfo{{Kind: "snat", Source: "192.0.2.0/24", Dest: "203.0.113.1"}},
		tun:  []TunnelInfo{{Kind: "wireguard", ID: "wg-1", Peer: "10.0.0.1", Up: true}},
		dns:  map[string]string{"example.com": "198.51.100.1"},
		wans: []WANInfo{{ID: "wan1", Link: "wan0", Healthy: true, Uplink: "203.0.113.1"}},
	}
	b := NewBuilder(e, time.Now)
	c, err := b.Build(Endpoint{Address: "192.0.2.1"}, Endpoint{Address: "example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if c.Verdict != DecisionAllow {
		t.Fatalf("verdict: %q", c.Verdict)
	}
	// Hops must include interface, vlan, access,
	// route, policy-route, firewall, nat, tunnel,
	// dns, wan.
	seen := map[HopKind]bool{}
	for _, h := range c.Hops {
		seen[h.Kind] = true
	}
	for _, k := range []HopKind{HopInterface, HopVLAN, HopAccess, HopRoute, HopPolicyRoute, HopFirewall, HopNAT, HopTunnel, HopDNS, HopWAN} {
		if !seen[k] {
			t.Fatalf("hop %s missing from %+v", k, c.Hops)
		}
	}
}

func TestChainFirewallDeny(t *testing.T) {
	e := &fakeEngine{
		snap: TopologySnapshot{Interfaces: []InterfaceInfo{{Name: "lan0", Address: "192.0.2.1", Up: true}}},
		rts:  []RouteInfo{{Destination: "0.0.0.0/0", Gateway: "203.0.113.254", Interface: "wan0"}},
		rls:  []RuleInfo{{Chain: "forward", Action: "drop"}},
	}
	b := NewBuilder(e, time.Now)
	c, err := b.Build(Endpoint{Address: "192.0.2.1"}, Endpoint{Address: "198.51.100.1"})
	if err != nil {
		t.Fatal(err)
	}
	if c.Verdict != DecisionDeny {
		t.Fatalf("verdict: %q", c.Verdict)
	}
}

func TestChainNoRoute(t *testing.T) {
	e := &fakeEngine{
		snap: TopologySnapshot{Interfaces: []InterfaceInfo{{Name: "lan0", Address: "192.0.2.1", Up: true}}},
	}
	b := NewBuilder(e, time.Now)
	c, err := b.Build(Endpoint{Address: "192.0.2.1"}, Endpoint{Address: "198.51.100.1"})
	if err != nil {
		t.Fatal(err)
	}
	if c.Verdict != DecisionStop {
		t.Fatalf("verdict: %q", c.Verdict)
	}
}

func TestChainInterfaceDown(t *testing.T) {
	e := &fakeEngine{
		snap: TopologySnapshot{Interfaces: []InterfaceInfo{{Name: "lan0", Address: "192.0.2.1", Up: false}}},
	}
	b := NewBuilder(e, time.Now)
	c, err := b.Build(Endpoint{Address: "192.0.2.1"}, Endpoint{Address: "198.51.100.1"})
	if err != nil {
		t.Fatal(err)
	}
	if c.Verdict != DecisionDeny {
		t.Fatalf("verdict: %q", c.Verdict)
	}
}

func TestChainJSONRoundTrip(t *testing.T) {
	e := &fakeEngine{
		snap: TopologySnapshot{Interfaces: []InterfaceInfo{{Name: "lan0", Address: "192.0.2.1", Up: true}}},
		rts:  []RouteInfo{{Destination: "0.0.0.0/0", Gateway: "203.0.113.254", Interface: "wan0"}},
		rls:  []RuleInfo{{Chain: "forward", Action: "accept"}},
		wans: []WANInfo{{ID: "wan1", Link: "wan0", Healthy: true}},
	}
	b := NewBuilder(e, time.Now)
	c, err := b.Build(Endpoint{Address: "192.0.2.1"}, Endpoint{Address: "198.51.100.1"})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := c.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"schemaVersion":"1"`) {
		t.Fatalf("schemaVersion must be 1: %s", raw)
	}
	var back Chain
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	if back.Verdict != c.Verdict {
		t.Fatalf("Verdict: %q vs %q", back.Verdict, c.Verdict)
	}
}

func TestChainInvalidEndpointIsError(t *testing.T) {
	b := NewBuilder(&fakeEngine{}, time.Now)
	if _, err := b.Build(Endpoint{Address: "192.0.2.1"}, Endpoint{}); err == nil {
		t.Fatal("Build with empty destination must fail")
	}
	if _, err := b.Build(Endpoint{}, Endpoint{Address: "198.51.100.1"}); err == nil {
		t.Fatal("Build with empty source must fail")
	}
}
