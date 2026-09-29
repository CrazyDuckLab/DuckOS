// wan/provider_test.go pins the V1 WAN
// provider contract.
package wan

import "testing"

func TestParseMACValid(t *testing.T) {
	for _, good := range []string{"02:42:ac:11:00:01", "de:ad:be:ef:00:01", "AA:BB:CC:DD:EE:FF"} {
		if _, err := ParseMAC(good); err != nil {
			t.Errorf("ParseMAC(%q): %v", good, err)
		}
	}
}

func TestParseMACRejectsBad(t *testing.T) {
	for _, bad := range []string{
		"", "00:00:00:00:00:00", "ff:ff:ff:ff:ff:ff",
		"01:00:00:00:00:00", // multicast
		"00-00-00-00-00-00", // wrong separator
		"not a mac",
		"zz:zz:zz:zz:zz:zz",
	} {
		if _, err := ParseMAC(bad); err == nil {
			t.Errorf("ParseMAC(%q) must fail", bad)
		}
	}
}

func TestMACSourceValid(t *testing.T) {
	for _, s := range []Source{MACSourceHardware, MACSourceOverride, MACSourceCloneCurrent, MACSourceCloneRecent, "garbage"} {
		want := s == MACSourceHardware || s == MACSourceOverride || s == MACSourceCloneCurrent || s == MACSourceCloneRecent
		if got := s.Valid(); got != want {
			t.Errorf("%s.Valid=%v want %v", s, got, want)
		}
	}
}

func TestMACConfigOverrideRequiresAddress(t *testing.T) {
	if err := (MACConfig{Source: MACSourceOverride}).Valid(); err == nil {
		t.Fatal("override without address must fail")
	}
	if err := (MACConfig{Source: MACSourceOverride, Address: "02:42:ac:11:00:01"}).Valid(); err != nil {
		t.Fatal(err)
	}
}

func TestMTUConfigValid(t *testing.T) {
	for _, m := range []MTUConfig{
		{MTU: 1500, MSSClamp: true},
		{MTU: 1500, MSSClamp: true, MSSClampPMTU: true},
		{MTU: 9000},
		{MTU: -1},
		{MTU: 1500, MSSClampPMTU: true},
	} {
		want := m.MTU >= 0 && m.MTU <= 9000 && !(m.MSSClampPMTU && !m.MSSClamp)
		if err := m.Valid(); (err == nil) != want {
			t.Errorf("MTUConfig%+v: err=%v want-valid=%v", m, err, want)
		}
	}
}

func TestProviderValid(t *testing.T) {
	hwMAC := MACConfig{Source: MACSourceHardware}
	mtu := MTUConfig{MTU: 1500, MSSClamp: true}
	good := Provider{ID: "wan-1", Link: "eth0", Mode: ModeDHCP, MAC: hwMAC, MTU: mtu, Enabled: true}
	if err := good.Valid(); err != nil {
		t.Fatal(err)
	}
	pppoe := Provider{ID: "wan-1", Link: "eth0", Mode: ModePPPoE, MAC: hwMAC, MTU: mtu, PPPoE: &PPPoEConfig{Username: "u", PasswordRef: "vault-ref"}, Enabled: true}
	if err := pppoe.Valid(); err != nil {
		t.Fatal(err)
	}
	// PPPoE without credentials must fail.
	bad := Provider{ID: "wan-1", Link: "eth0", Mode: ModePPPoE, MAC: hwMAC, MTU: mtu, PPPoE: &PPPoEConfig{}, Enabled: true}
	if err := bad.Valid(); err == nil {
		t.Fatal("PPPoE without credentials must fail")
	}
}

func TestProviderJSONRoundTrip(t *testing.T) {
	p := Provider{ID: "wan-1", Link: "eth0", Mode: ModeStatic, MAC: MACConfig{Source: MACSourceOverride, Address: "02:42:ac:11:00:01"}, MTU: MTUConfig{MTU: 1500}, Static: &StaticConfig{Address: "203.0.113.1/24", Gateway: "203.0.113.254"}, Enabled: true}
	raw, err := p.JSON()
	if err != nil {
		t.Fatal(err)
	}
	back, err := DecodeProvider(raw)
	if err != nil {
		t.Fatal(err)
	}
	if back.ID != p.ID || back.Mode != p.Mode || back.MAC.Source != p.MAC.Source {
		t.Fatalf("round-trip mismatch: %+v vs %+v", back, p)
	}
}
