package objectlisters

import (
	"strings"
	"testing"

	"github.com/CrazyDuckLab/DuckOS/internal/identity"
	"github.com/CrazyDuckLab/DuckOS/internal/object"
)

// TestAddressToViewIDInsideCIDRIDPattern_RegressionRBACLive
// pins the network.address object-id format inside the
// cidrIDPattern character class. '|' is rejected by the
// regex and causes the per-object RBAC filter to redact
// every real address from Owner-visible
// /api/v1/objects/network.address. This regression test
// pins the B-009 RBAC LIVE recovery landed by the BETA.2
// corrective round.
func TestAddressToViewIDInsideCIDRIDPattern_RegressionRBACLive(t *testing.T) {
	v := addressToView("eth0", "fe80::1/64")
	id := v.Ref.ID
	if strings.Contains(id, "|") {
		t.Fatalf("network.address id %q must not contain '|' (rejected by cidrIDPattern)", id)
	}
	obj := identity.ObjectKey{Kind: "network.address", ID: id}
	if !obj.Valid() {
		t.Fatalf("network.address id %q must be accepted by cidrIDPattern (Owner-visible RBAC contract)", id)
	}
	// Sanity: the iface part must remain the first
	// segment so existing UI consumers keep working.
	if !strings.HasPrefix(id, "eth0") {
		t.Fatalf("network.address id %q must keep the iface prefix", id)
	}
}

// TestAddressToViewAcceptsIPv4AndIPv6 pins the canonical
// formatting for both address families.
func TestAddressToViewAcceptsIPv4AndIPv6(t *testing.T) {
	for _, c := range []struct{ iface, addr string }{
		{"eth0", "192.168.1.1/24"},
		{"eth1", "10.0.0.5/8"},
		{"eth0", "fe80::1/64"},
	} {
		v := addressToView(c.iface, c.addr)
		obj := identity.ObjectKey{Kind: "network.address", ID: v.Ref.ID}
		if !obj.Valid() {
			t.Errorf("network.address id %q (iface=%q addr=%q) must be valid",
				v.Ref.ID, c.iface, c.addr)
		}
		_ = object.ViewObject{}
	}
}
