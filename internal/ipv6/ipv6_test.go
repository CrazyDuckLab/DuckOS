// ipv6/ipv6_test.go pins the V1 IPv6 model.
package ipv6

import "testing"

func TestValidatePrefix(t *testing.T) {
	for _, good := range []string{"2001:db8::/64", "::/0", "fe80::/10", "2001:db8:1::/48"} {
		if err := ValidatePrefix(good); err != nil {
			t.Errorf("ValidatePrefix(%q): %v", good, err)
		}
	}
	for _, bad := range []string{"", "no-slash", "2001:db8::/129", ":::/64", "2001:db8::xyz/64"} {
		if err := ValidatePrefix(bad); err == nil {
			t.Errorf("ValidatePrefix(%q) must fail", bad)
		}
	}
}

func TestPDConfigValid(t *testing.T) {
	good := PDConfig{Interface: "lan0", UpstreamIf: "wan0", IAID: 1, State: PDBound, Assignments: []Assignment{{Interface: "lan0", Prefix: "2001:db8:1::", PrefixLen: 64, State: PDBound}}}
	if err := good.Valid(); err != nil {
		t.Fatal(err)
	}
	bad := PDConfig{Interface: "lan0", State: "garbage"}
	if err := bad.Valid(); err == nil {
		t.Fatal("unknown state must fail")
	}
}

func TestNAT64ConfigValid(t *testing.T) {
	off := NAT64Config{Enabled: false}
	if err := off.Valid(); err != nil {
		t.Fatal(err)
	}
	on := NAT64Config{Enabled: true, Prefix: "2001:db8:64::/96"}
	if err := on.Valid(); err != nil {
		t.Fatal(err)
	}
	onNoPrefix := NAT64Config{Enabled: true}
	if err := onNoPrefix.Valid(); err == nil {
		t.Fatal("enabled without prefix must fail")
	}
}

func TestDNS64ConfigValid(t *testing.T) {
	on := DNS64Config{Enabled: true, Prefix: "2001:db8:64::/96"}
	if err := on.Valid(); err != nil {
		t.Fatal(err)
	}
}

func TestManagerPDCRUD(t *testing.T) {
	m := NewManager()
	if err := m.SetPD(PDConfig{Interface: "lan0", UpstreamIf: "wan0", State: PDBound, Assignments: []Assignment{{Interface: "lan0", Prefix: "2001:db8:1::", PrefixLen: 64, State: PDBound}}}); err != nil {
		t.Fatal(err)
	}
	if _, ok := m.PD("lan0"); !ok {
		t.Fatal("PD must be retrievable")
	}
	if err := m.SetNAT64(NAT64Config{Enabled: true, Prefix: "2001:db8:64::/96"}); err != nil {
		t.Fatal(err)
	}
	if !m.NAT64().Enabled {
		t.Fatal("NAT64 must be enabled")
	}
	if err := m.SetDNS64(DNS64Config{Enabled: true, Prefix: "2001:db8:64::/96"}); err != nil {
		t.Fatal(err)
	}
	if !m.DNS64().Enabled {
		t.Fatal("DNS64 must be enabled")
	}
}
