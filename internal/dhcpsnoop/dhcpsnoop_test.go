// dhcpsnoop/dhcpsnoop_test.go pins the V1
// DHCP-snooping + DAI + RA-Guard model.
package dhcpsnoop

import "testing"

func TestPortClassValid(t *testing.T) {
	good := PortClass{Bridge: "br-1", Link: "eth0", Class: TrustTrusted}
	if err := good.Valid(); err != nil {
		t.Fatal(err)
	}
	bad := PortClass{Bridge: "br-1", Link: "eth0", Class: "garbage"}
	if err := bad.Valid(); err == nil {
		t.Fatal("unknown class must fail")
	}
}

func TestManagerSetPortAndClass(t *testing.T) {
	m := NewManager()
	if err := m.SetPort(PortClass{Bridge: "br-1", Link: "eth0", Class: TrustTrusted}); err != nil {
		t.Fatal(err)
	}
	if err := m.SetPort(PortClass{Bridge: "br-1", Link: "eth1", Class: TrustDHCPClient}); err != nil {
		t.Fatal(err)
	}
	if cls, ok := m.ClassOf("br-1", "eth0"); !ok || cls != TrustTrusted {
		t.Fatalf("ClassOf eth0: %q %v", cls, ok)
	}
	if cls, ok := m.ClassOf("br-1", "eth1"); !ok || cls != TrustDHCPClient {
		t.Fatalf("ClassOf eth1: %q %v", cls, ok)
	}
}

func TestManagerLeaseAndBinding(t *testing.T) {
	m := NewManager()
	if err := m.SetLease(Lease{MAC: "aa:bb:cc:dd:ee:01", IP: "192.168.1.10", Bridge: "br-1", Link: "eth1", VLAN: 10, Expiry: 1700000000, Trusted: true}); err != nil {
		t.Fatal(err)
	}
	if l, ok := m.LeaseForMAC("aa:bb:cc:dd:ee:01"); !ok || l.IP != "192.168.1.10" {
		t.Fatalf("LeaseForMAC: %+v %v", l, ok)
	}
	if b, ok := m.BindingForIP("192.168.1.10"); !ok || b.MAC != "aa:bb:cc:dd:ee:01" {
		t.Fatalf("BindingForIP: %+v %v", b, ok)
	}
}

func TestManagerRogueRAs(t *testing.T) {
	m := NewManager()
	if err := m.SetPort(PortClass{Bridge: "br-1", Link: "eth0", Class: TrustTrusted}); err != nil {
		t.Fatal(err)
	}
	if err := m.SetPort(PortClass{Bridge: "br-1", Link: "eth1", Class: TrustUntrusted}); err != nil {
		t.Fatal(err)
	}
	m.RecordRA("br-1", "eth0", RAObservation{Source: "fe80::1", Prefix: "2001:db8::/64", At: 1, Flags: "M"})
	m.RecordRA("br-1", "eth1", RAObservation{Source: "fe80::bad", Prefix: "fd00::/64", At: 2, Flags: "M"})
	rogues := m.RogueRAs()
	if len(rogues) != 1 {
		t.Fatalf("expected 1 rogue, got %d", len(rogues))
	}
	if rogues[0].Source != "fe80::bad" {
		t.Fatalf("rogue: %+v", rogues[0])
	}
}
