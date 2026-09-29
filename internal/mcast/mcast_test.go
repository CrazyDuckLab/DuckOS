// mcast/mcast_test.go pins the V1 multicast
// model.
package mcast

import "testing"

func TestNetworkValid(t *testing.T) {
	good := Network{ID: "lan", Bridge: "br-lan", Mode: ModeSnooping}
	if err := good.Valid(); err != nil {
		t.Fatal(err)
	}
	proxy := Network{ID: "lan", Bridge: "br-lan", Mode: ModeProxy, UpstreamIf: "wan0"}
	if err := proxy.Valid(); err != nil {
		t.Fatal(err)
	}
	bad := Network{ID: "lan", Bridge: "br-lan", Mode: ModeProxy}
	if err := bad.Valid(); err == nil {
		t.Fatal("proxy without upstream must fail")
	}
}

func TestMDNSValid(t *testing.T) {
	good := MDNSReflectorPolicy{Enabled: true, FromNetwork: "lan", ToNetworks: []string{"guest"}}
	if err := good.Valid(); err != nil {
		t.Fatal(err)
	}
	off := MDNSReflectorPolicy{Enabled: false}
	if err := off.Valid(); err != nil {
		t.Fatal(err)
	}
	bad := MDNSReflectorPolicy{Enabled: true, FromNetwork: "../bad"}
	if err := bad.Valid(); err == nil {
		t.Fatal("non-canonical from must fail")
	}
}

func TestManagerCanReflectDefaultNo(t *testing.T) {
	m := NewManager()
	if m.CanReflect("lan", "guest") {
		t.Fatal("default must be no global reflection")
	}
}

func TestManagerCanReflectExplicit(t *testing.T) {
	m := NewManager()
	if err := m.SetMDNS(MDNSReflectorPolicy{Enabled: true, FromNetwork: "lan", ToNetworks: []string{"guest"}}); err != nil {
		t.Fatal(err)
	}
	if !m.CanReflect("lan", "guest") {
		t.Fatal("explicit policy must allow")
	}
	if m.CanReflect("guest", "lan") {
		t.Fatal("reverse direction must not be allowed without a policy")
	}
}

func TestManagerNetworks(t *testing.T) {
	m := NewManager()
	if err := m.SetNetwork(Network{ID: "lan", Bridge: "br-lan", Mode: ModeSnooping}); err != nil {
		t.Fatal(err)
	}
	if err := m.SetNetwork(Network{ID: "guest", Bridge: "br-guest", Mode: ModeOff}); err != nil {
		t.Fatal(err)
	}
	if got := len(m.Networks()); got != 2 {
		t.Fatalf("Networks count: %d", got)
	}
}
