package doctor

import "testing"

// TestHexToIPByteSwap_RegressionRBACLive guards the
// /proc/net/route hex-to-dotted-quad conversion. Every
// column of /proc/net/route (Destination / Gateway /
// Mask) is written by the kernel in host byte order
// (little-endian on x86_64); we byte-swap so the
// resulting string is the canonical CIDR form
// ("172.18.0.0" not "0.0.18.172"). Without the swap the
// per-object RBAC filter rejects every real route (the
// post-regex ValidateCIDRBody helper refuses '.' after
// '/') and Owner-visible /api/v1/objects/network.route
// returns rbac_redacted issues instead of the real
// objects. This regression test pins the byte-swap that
// the B-009 RBAC LIVE recovery landed.
func TestHexToIPByteSwap_RegressionRBACLive(t *testing.T) {
	cases := []struct {
		hex  string
		want string
		note string
	}{
		{"000012AC", "172.18.0.0", "Destination / Gateway LE: 172.18.0.0"},
		{"00000000", "0.0.0.0", "Destination LE: 0.0.0.0 default route"},
		{"0A000001", "1.0.0.10", "Destination LE: 10.0.0.1 -> dotted-quad"},
		{"FE0112AC", "172.18.1.254", "Gateway LE: 172.18.1.254"},
	}
	for _, c := range cases {
		got, err := hexToIP(c.hex)
		if err != nil {
			t.Errorf("%s: unexpected error: %v", c.note, err)
			continue
		}
		if got != c.want {
			t.Errorf("%s: hexToIP(%s) = %q, want %q", c.note, c.hex, got, c.want)
		}
	}
}

// TestHexToPrefixLength_RegressionRBACLive pins the
// Mask -> "/N" conversion that the B-009 RBAC LIVE
// recovery landed. Without it, the engine produces
// non-canonical IDs like "172.18.0.0/255.255.0.0" that
// ValidateCIDRBody refuses ('.' after '/') and the
// per-object RBAC filter redacts every real route.
func TestHexToPrefixLength_RegressionRBACLive(t *testing.T) {
	cases := []struct {
		hex  string
		want string
		note string
	}{
		// Hex values are what the kernel prints in
		// /proc/net/route on little-endian: the u32
		// host-byte-order value in %08X. For a /N netmask the
		// printed value byte-swaps to the MSB-contiguous mask.
		{"0000FFFF", "16", "/16 mask in LE (255.255.0.0)"},
		{"00000000", "0", "/0 default mask"},
		{"FFFFFFFF", "32", "/32 host mask"},
		{"80FFFFFF", "25", "/25 mask (255.255.255.128)"},
		{"C0FFFFFF", "26", "/26 mask (255.255.255.192)"},
		{"00000080", "1", "/1 mask (128.0.0.0)"},
	}
	// Non-contiguous: must error ("7F000000" byte-swaps to
	// 0x0000007F, a mask no kernel ever writes).
	if _, err := hexToPrefixLength("7F000000"); err == nil {
		t.Errorf("non-contiguous mask must error")
	}
	// Wrong byte length: must error, not silently succeed.
	if _, err := hexToPrefixLength("0000"); err == nil {
		t.Errorf("wrong-length mask must error")
	}
	for _, c := range cases {
		got, err := hexToPrefixLength(c.hex)
		if err != nil {
			t.Errorf("%s: unexpected error: %v", c.note, err)
			continue
		}
		if got != c.want {
			t.Errorf("%s: hexToPrefixLength(%s) = %q, want %q", c.note, c.hex, got, c.want)
		}
	}
}

// TestHexToIPRejectsBadInput_RegressionRBACLive pins
// the input validation so future refactors do not
// silently widen the parser to invalid hex or wrong
// byte length.
func TestHexToIPRejectsBadInput_RegressionRBACLive(t *testing.T) {
	for _, hex := range []string{"", "zz", "00", "0000", "000000000000"} {
		if _, err := hexToIP(hex); err == nil {
			t.Errorf("hexToIP(%q) must reject", hex)
		}
	}
}
