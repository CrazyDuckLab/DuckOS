package doctor

import "testing"

// TestHexToIPByteSwap_RegressionRBACLive guards the
// /proc/net/route hex-to-dotted-quad conversion. The
// kernel writes the destination hex in little-endian
// byte order; if the conversion skips the byte-swap the
// resulting id is the byte-reversed CIDR form (e.g.
// "0.0.18.172/0.0.255.255" instead of "172.18.0.0/
// 255.255.0.0"). The post-regex ValidateCIDRBody helper
// refuses '.' after '/' and the per-object RBAC filter
// redacts every real route, so Owner-visible
// /api/v1/objects/network.route returns rbac_redacted
// issues instead of the real objects. This regression
// test pins the byte-swap that the B-009 RBAC LIVE
// recovery landed.
func TestHexToIPByteSwap_RegressionRBACLive(t *testing.T) {
	cases := []struct {
		hex   string
		want  string
		note  string
	}{
		{"000012AC", "172.18.0.0", "IPv4 172.18.0.0 written LE by the kernel"},
		{"00000000", "0.0.0.0", "IPv4 0.0.0.0 default route"},
		{"0A000001", "1.0.0.10", "IPv4 10.0.0.1 LE -> dotted-quad"},
		{"FFFFFFFF", "255.255.255.255", "IPv4 broadcast"},
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