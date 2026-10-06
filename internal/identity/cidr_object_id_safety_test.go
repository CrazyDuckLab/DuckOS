// B-009 safety review (BETA.2 pre-live gate).
//
// The fix in commit 14a761e introduces a per-kind
// idPattern override. The BETA.2 review asked us to
// prove, not assume, that:
//
//   1. The global idPattern stays strict. Other
//      object kinds (vlan, wireguard.peer, xray.server,
//      ...) MUST still reject '/' in their ids.
//   2. CIDR-bearing kinds (network.route,
//      network.address, ...) accept '/' only because
//      they are explicitly listed in cidrKinds.
//   3. Even CIDR-bearing kinds reject path-traversal
//      payloads (../../etc/passwd) and shell
//      metacharacters — the wider cidrIDPattern is
//      tight, not "anything goes".
//
// These tests are the safety net. They run in the
// standard `go test` invocation and any future
// refactor that weakens the contract must break one
// of them first.

package identity

import (
	"testing"

	"github.com/CrazyDuckLab/DuckOS/internal/object"
)

// TestB009_GlobalSlashStillRejected_ProvesKindBounded is
// the canonical safety claim. For each non-CIDR kind
// the global idPattern is applied, and that pattern
// does NOT include '/'. A '/' in the id MUST fail
// for any kind that is not in cidrKinds.
func TestB009_GlobalSlashStillRejected_ProvesKindBounded(t *testing.T) {
	cases := []struct {
		kind string
		id   string
		note string
	}{
		{"vlan", "vlan-1/edge", "kernel-side vlan ref"},
		{"wireguard.peer", "peer-a/edge", "peer opaque id"},
		{"xray.server", "xr-edge1/edge", "xray server opaque id"},
		{"xray.user", "u1/payment", "xray user opaque id"},
		{"frr.bgp", "65001-vrf/2", "ASN opaque id"},
		{"qos.class", "gold/vip", "qos class opaque id"},
		{"interface", "eth0/alt", "interface name"},
		{"dns.zone", "example.com/dns", "dns zone"},
		{"vault.secret", "secret/ref", "vault ref"},
		{"upnp.network", "eth1/lan", "upnp network"},
		{"site", "site/main", "site id"},
	}
	for _, c := range cases {
		k := ObjectKey{Kind: c.kind, ID: c.id}
		if k.Valid() {
			t.Errorf("%s/%s must be rejected (kind not in cidrKinds); note=%s", k.Kind, k.ID, c.note)
		}
	}
}

// TestB009_PathTraversalRejectedEverywhere is the
// path-traversal invariant. "../../etc/passwd" must
// fail for every kind, including the CIDR-bearing
// ones. The wider pattern allows '/'.
func TestB009_PathTraversalRejectedEverywhere(t *testing.T) {
	payloads := []string{
		"../../etc/passwd",
		"../../../etc/shadow",
		"/etc/passwd",
		"foo/../../etc/passwd",
		"foo/../../../etc/shadow",
		"..%2F..%2Fetc%2Fpasswd",
	}
	// Every kind — including the cidrKinds ones —
	// MUST refuse these payloads.
	for _, kind := range []string{
		"network.route",
		"network.address",
		"network.firewall.rule",
		"network.firewall.zone",
		"network.nat.rule",
		"network.dhcp.reservation",
		"vlan",
		"wireguard.peer",
		"xray.server",
		"frr.bgp",
	} {
		for _, p := range payloads {
			k := ObjectKey{Kind: kind, ID: p}
			if k.Valid() {
				t.Errorf("%s/%q must be rejected (path traversal)", kind, p)
			}
		}
	}
}

// TestB009_GlobalPatternContainsNoSlash is the
// grammar proof: the global regex literally does
// not contain '/' as an allowed byte.
func TestB009_GlobalPatternContainsNoSlash(t *testing.T) {
	if p := idPattern.String(); containsSlash(p) {
		t.Fatalf("global idPattern contains '/': %q", p)
	}
}

// TestB009_CidrPatternAcceptsSlash is the inverse
// proof: the wider cidr pattern IS supposed to
// contain '/' and the safety net is the cidrKinds
// gate, not the regex itself.
func TestB009_CidrPatternAcceptsSlash(t *testing.T) {
	if p := cidrIDPattern.String(); !containsSlash(p) {
		t.Fatalf("cidrIDPattern must contain '/'; got %q", p)
	}
}

// TestB009_CidrPatternStillRejectsShellMetas is the
// "wide, not wide-open" guarantee. The wider pattern
// admits only bytes CIDR / zone-id forms need; every
// shell meta / whitespace / control byte must still
// be refused.
func TestB009_CidrPatternStillRejectsShellMetas(t *testing.T) {
	bad := []string{
		"10.0.0.0; rm -rf /",
		"10.0.0.0 && curl evil",
		"10.0.0.0|nc cat",
		"10.0.0.0\n",
		"10.0.0.0\r",
		"10.0.0.0\t",
		"10.0.0.0`id`",
		"10.0.0.0$VAR",
		"10.0.0.0*",
		"10.0.0.0?",  // shell wildcard
		"10.0.0.0[",  // shell glob
		"10.0.0.0]",  // shell glob
		"10.0.0.0{",  // brace expansion
		"10.0.0.0}",  // brace expansion
		"10.0.0.0\\", // escape
		"10.0.0.0\"", // quote
		"10.0.0.0'",  // single quote
		"10.0.0.0<>", // redirect
		"",           // empty
		" ",          // space
		"\x00",       // NUL
	}
	for _, id := range bad {
		k := ObjectKey{Kind: "network.route", ID: id}
		if k.Valid() {
			t.Errorf("network.route/%q must be rejected by cidrIDPattern", id)
		}
	}
}

// TestB009_KnownCIDRFormsAllAccepted is the positive
// catalogue: every documented CIDR / IPv6 / zone-id
// form the runtime can emit MUST be accepted.
func TestB009_KnownCIDRFormsAllAccepted(t *testing.T) {
	cases := []struct {
		kind string
		id   string
	}{
		{"network.route", "10.0.0.0/24"},
		{"network.route", "0.0.0.0/0"},
		{"network.route", "192.168.1.0/24"},
		{"network.route", "default"},
		{"network.route", "2001:db8::/64"},
		{"network.route", "fe80::/64"},
		{"network.route", "2001:db8::ffff:ffff:ffff:ffff/128"},
		{"network.address", "192.168.1.1/24"},
		{"network.address", "fe80::1%eth0/64"},
		{"network.address", "::1/128"},
		{"network.address", "fe80::1/64"},
		{"network.firewall.rule", "10.0.0.0/24"},
		{"network.firewall.zone", "10.0.0.0/8"},
		{"network.nat.rule", "192.168.0.0/16"},
		{"network.dhcp.reservation", "00:11:22:33:44:55/24"},
	}
	for _, c := range cases {
		k := ObjectKey{Kind: c.kind, ID: c.id}
		if !k.Valid() {
			t.Errorf("%s/%s must be accepted", c.kind, c.id)
		}
	}
}

// TestB009_OwnerSeesCIDRItems proves the B-009 fix
// reaches the public surface: an Owner session
// looking at /api/v1/objects?collection=network.route
// sees the CIDR-bearing items instead of seeing them
// dropped as rbac_redacted.
func TestB009_OwnerSeesCIDRItems(t *testing.T) {
	p := NewPolicy()
	col := collectionWithItems(
		"network.route",
		[]object.ObjectRef{
			{Kind: "network.route", ID: "10.0.0.0/24"},
			{Kind: "network.route", ID: "0.0.0.0/0"},
			{Kind: "network.route", ID: "2001:db8::/64"},
			{Kind: "network.route", ID: "default"},
		},
	)
	out := p.FilterCollection(Owner, "network.route", Observe, col)
	if len(out.Items) != 4 {
		t.Fatalf("Owner must see all 4 CIDR routes; got %d (issues=%+v)", len(out.Items), out.Issues)
	}
}

// TestB009_OwnerSeesCIDRAddresses mirrors
// TestB009_OwnerSeesCIDRItems for the address kind.
func TestB009_OwnerSeesCIDRAddresses(t *testing.T) {
	p := NewPolicy()
	col := collectionWithItems(
		"network.address",
		[]object.ObjectRef{
			{Kind: "network.address", ID: "192.168.1.1/24"},
			{Kind: "network.address", ID: "fe80::1%eth0/64"},
		},
	)
	out := p.FilterCollection(Owner, "network.address", Observe, col)
	if len(out.Items) != 2 {
		t.Fatalf("Owner must see both CIDR addresses; got %d (issues=%+v)", len(out.Items), out.Issues)
	}
}

// TestB009_NonCIDRKindStillStrict is the inverse of
// TestB009_OwnerSeesCIDRItems. A non-CIDR kind with
// an opaque id that incidentally contains '/' (a
// hand-written wireguard.peer ref) MUST be redacted
// by Owner (the id is invalid). This is the
// "vlan/edge stays rejected" case at the RBAC level.
func TestB009_NonCIDRKindStillStrict(t *testing.T) {
	p := NewPolicy()
	col := collectionWithItems(
		"wireguard.peer",
		[]object.ObjectRef{
			{Kind: "wireguard.peer", ID: "peer-a/edge"}, // invalid id; 'wireguard.peer' is not in cidrKinds
		},
	)
	out := p.FilterCollection(Owner, "wireguard.peer", Observe, col)
	if len(out.Items) != 0 {
		t.Fatalf("wireguard.peer/peer-a/edge must be rejected; got %d items", len(out.Items))
	}
	if len(out.Issues) == 0 {
		t.Fatalf("rejection must surface as an rbac_redacted issue")
	}
}

// containsSlash is a tiny string helper kept here so
// the test file does not import strings just for one
// substring check.
func containsSlash(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == '/' {
			return true
		}
	}
	return false
}

// collectionWithItems builds an object.Collection from
// a slice of refs for the FilterCollection test path.
func collectionWithItems(kind string, refs []object.ObjectRef) object.Collection {
	items := make([]object.ViewObject, 0, len(refs))
	for _, r := range refs {
		items = append(items, object.ViewObject{Ref: r})
	}
	return object.Collection{SnapshotID: "snap-b009", Items: items}
}
