package wan

import "testing"

func staticProvider() Provider {
	return Provider{
		ID: "uplink-1", Name: "Primary", Link: "eth-wan", Mode: ModeStatic, Enabled: true,
		MAC:    MACConfig{Source: MACSourceHardware},
		Static: &StaticConfig{Address: "203.0.113.2/32", Gateway: "203.0.113.1", DNSServers: []string{"9.9.9.9"}},
	}
}

func TestValidProviderComplete(t *testing.T) {
	if err := ValidProviderComplete(staticProvider()); err != nil {
		t.Fatalf("static provider rejected: %v", err)
	}
	dhcp := Provider{ID: "uplink-dhcp", Name: "DHCP", Link: "eth-wan", Mode: ModeDHCP, Enabled: true, MAC: MACConfig{Source: MACSourceHardware}}
	if err := ValidProviderComplete(dhcp); err != nil {
		t.Fatalf("dhcp provider rejected: %v", err)
	}
	// DHCP must not carry a static sub-config.
	bad := dhcp
	bad.Static = &StaticConfig{Address: "203.0.113.2/24"}
	if err := ValidProviderComplete(bad); err == nil {
		t.Fatal("dhcp with static accepted")
	}
}

func TestStaticValidation(t *testing.T) {
	cases := []func(*StaticConfig) string{
		func(s *StaticConfig) string { s.Address = ""; return "missing address" },
		func(s *StaticConfig) string { s.Address = "203.0.113.2"; return "address without prefix" },
		func(s *StaticConfig) string { s.Address = "203.0.113.2/24"; return "unmasked" }, // host bits set
		func(s *StaticConfig) string { s.Address = "0.0.0.0/0"; return "unspecified" },
		func(s *StaticConfig) string { s.Gateway = "not-an-ip"; return "bad gateway" },
		func(s *StaticConfig) string { s.Gateway = "::1"; return "family mismatch" },
		func(s *StaticConfig) string { s.DNSServers = make([]string, MaxDNSServers+1); return "dns overflow" },
		func(s *StaticConfig) string { s.DNSServers = []string{"nope"}; return "bad dns" },
		func(s *StaticConfig) string { s.DNSServers = []string{"224.0.0.1"}; return "multicast dns" },
	}
	for _, mutate := range cases {
		s := StaticConfig{Address: "203.0.113.2/32", Gateway: "203.0.113.1"}
		want := mutate(&s)
		if err := s.Valid(); err == nil {
			t.Fatalf("case %q accepted", want)
		}
	}
}
