package dhcpsnoop

import "testing"

func TestParseDnsmasqLeases(t *testing.T) {
	data := []byte(`# duck-dnsmasq lease file
1718200000 aa:bb:cc:dd:ee:ff 192.168.10.50 client-1 01:aa:bb:cc:dd:ee:ff
1718200500 0A-0B-0C-0D-0E-0F 192.168.10.51 *
1719999999 aa:bb:cc:dd:ee:00 192.168.10.52 nas
garbage line here
999999 not-a-mac 192.168.10.53 x y
1718200000 aa:bb:cc:dd:ee:10 999.999.999.999 x y
1718200000 aa:bb:cc:dd:ee:11 fd00::10 ipv6-client *
`)
	leases := ParseDnsmasqLeases(data)
	if len(leases) != 4 {
		t.Fatalf("leases: %+v", leases)
	}
	if leases[0].MAC != "aa:bb:cc:dd:ee:ff" || leases[0].IP != "192.168.10.50" {
		t.Fatalf("lease 0: %+v", leases[0])
	}
	if leases[0].Hostname != "client-1" {
		t.Fatalf("hostname: %q", leases[0].Hostname)
	}
	// Normalized dash MAC.
	if leases[1].MAC != "0a:0b:0c:0d:0e:0f" {
		t.Fatalf("normalized mac: %q", leases[1].MAC)
	}
	if leases[1].Hostname != "" {
		t.Fatalf("wildcard hostname leaked: %q", leases[1].Hostname)
	}
	// IPv6 lease passes address validation.
	if leases[3].IP != "fd00::10" {
		t.Fatalf("ipv6 lease: %+v", leases[3])
	}
	// Malformed lines were skipped, not fatal.
}

func TestNormalizeMAC(t *testing.T) {
	cases := map[string]string{
		"aa:bb:cc:dd:ee:ff": "aa:bb:cc:dd:ee:ff",
		"AA-BB-CC-DD-EE-FF": "aa:bb:cc:dd:ee:ff",
		"aabbccddeeff":      "",
		"aa:bb:cc:dd:ee":    "",
		"zz:bb:cc:dd:ee:ff": "",
	}
	for in, want := range cases {
		if got := normalizeMAC(in); got != want {
			t.Fatalf("normalizeMAC(%q) = %q, want %q", in, got, want)
		}
	}
}
