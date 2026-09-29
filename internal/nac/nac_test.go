package nac

import "testing"

func TestDeviceLifecycle(t *testing.T) {
	m := NewManager()
	if err := m.Observe(Device{MAC: "aa:bb:cc:dd:ee:01", IP: "10.0.0.5"}); err != nil {
		t.Fatal(err)
	}
	if err := m.Observe(Device{MAC: "aa:bb:cc:dd:ee:01", IP: "10.0.0.6"}); err != nil {
		t.Fatal(err)
	}
	if err := m.Approve("aa:bb:cc:dd:ee:01"); err != nil {
		t.Fatal(err)
	}
	if err := m.Quarantine("aa:bb:cc:dd:ee:01"); err != nil {
		t.Fatal(err)
	}
	if err := m.Approve("missing"); err == nil {
		t.Fatal("approve missing must fail")
	}
}

func TestIPAM(t *testing.T) {
	s := NewIPAMStore()
	s.AddPrefix(Prefix{CIDR: "10.0.0.0/24", VLAN: 10, VRF: "main"})
	s.AddPool(Pool{Name: "lan-pool", CIDR: "10.0.0.0/24", StartIP: "10.0.0.100", EndIP: "10.0.0.200"})
	if len(s.Prefixes()) != 1 || len(s.Pools()) != 1 {
		t.Fatal("sizes")
	}
}

func TestPortSec(t *testing.T) {
	s := NewPortSecStore()
	s.Set(PortSecurity{Port: "eth0", MaxMACs: 4, Sticky: true})
	s.ObserveLLDP(LLDPNeighbor{Port: "eth0", ChassisID: "00:11:22:33:44:55", PortID: "1", SystemName: "switch1"})
	if len(s.Neighbors()) != 1 {
		t.Fatal("neighbors")
	}
	if err := s.Set(PortSecurity{}); err == nil {
		t.Fatal("empty port must fail")
	}
}

func TestSPAN(t *testing.T) {
	s := NewSPANStore()
	s.Add(SPANSession{ID: "sp1", Source: "eth0", Dest: "eth1"})
	s.Remove("sp1")
	if len(s.All()) != 0 {
		t.Fatal("all")
	}
}

func TestAccel(t *testing.T) {
	s := NewAccelStore()
	s.Set("eth0", AccelXDP)
	if b, ok := s.Get("eth0"); !ok || b != AccelXDP {
		t.Fatal("accel")
	}
}

func TestSRIOV(t *testing.T) {
	s := NewSRIOVStore()
	s.Set(SRIOVPort{PF: "ens3", VF: 0})
	s.Set(SRIOVPort{PF: "ens3", VF: 1})
	if len(s.All()) != 2 {
		t.Fatal("sriov")
	}
}

func TestDHCPHA(t *testing.T) {
	s := NewDHCPHAStore()
	s.Set(DHCPHAPeer{Name: "primary", Address: "10.0.0.1", State: "primary"})
	if _, ok := s.Get("primary"); !ok {
		t.Fatal("ha")
	}
	if err := s.Set(DHCPHAPeer{}); err == nil {
		t.Fatal("empty must fail")
	}
}

func TestAnycast(t *testing.T) {
	s := NewAnycastStore()
	s.Set(AnycastService{Name: "dns", VIP: "10.0.0.53", LocalNode: "node-1", Healthy: true})
	if len(s.All()) != 1 {
		t.Fatal("anycast")
	}
}
