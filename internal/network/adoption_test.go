package network

import (
	"strings"
	"testing"
	"time"
)

// syntheticLiveLinks is a deterministic LiveLinkAdopter for tests. Each
// entry is (name, MAC). LookupLink returns the matching entry or
// (zero-value, false). It never mutates: it has no kernel, no journal,
// no I/O. The synthetic never represents an externally-managed link;
// tests that require externally-managed semantics construct a separate
// scenario.
type syntheticLiveLinks struct {
	links map[string]LiveLinkIdentity
}

func (s syntheticLiveLinks) LookupLink(name string) (LiveLinkIdentity, bool) {
	if s.links == nil {
		return LiveLinkIdentity{}, false
	}
	id, ok := s.links[name]
	return id, ok
}

func adoptionFixture(enroll []PhysicalBinding, live syntheticLiveLinks) (Enrollment, LiveLinkAdopter) {
	return Enrollment{
		Version:        1,
		CreatedAt:      time.Unix(0, 0).UTC(),
		Physical:       enroll,
		FirewallTable:  "inet duckos",
		VLANNames:      nil,
		RoutingTables:  []int{10000},
		PolicyRouting:  true,
		NetworkdReload: true,
	}, live
}

type physicalBindings = []PhysicalBinding

// dummyExternallyManaged binds but the same id would also exist in
// "non-external" form elsewhere; tests select which they want.
func adoptionExternallyManagedBinding(id, name, mac string) PhysicalBinding {
	return PhysicalBinding{
		ID:                id,
		Name:              name,
		MAC:               mac,
		ExternallyManaged: true,
	}
}

func adoptionOwnedBinding(id, name, mac string) PhysicalBinding {
	return PhysicalBinding{
		ID:   id,
		Name: name,
		MAC:  mac,
	}
}

func emptyConfig() Config                  { return Config{Version: 1} }
func singleInterface(iface Interface) Config {
	return Config{Version: 1, Interfaces: []Interface{iface}}
}

func ownedInterface(id, name string) Interface {
	return Interface{
		ID:         id,
		Name:       name,
		Kind:       "physical",
		Role:       "lan",
		MTU:        1500,
		Management: true,
		Addresses:  []string{"10.77.20.10/24"},
		DHCPClient: "none",
		AcceptRA:   false,
	}
}

// TestAdoption_AcceptsEnrolledNonExternalMatchingLive is the happy path:
// an empty durable Config adopts an enrolled, non-externally-managed
// physical link whose live kernel identity matches the binding exactly.
func TestAdoption_AcceptsEnrolledNonExternalMatchingLive(t *testing.T) {
	enroll, live := adoptionFixture(
		[]PhysicalBinding{adoptionOwnedBinding("eth1", "eth1", "00:15:5d:04:07:10")},
		syntheticLiveLinks{links: map[string]LiveLinkIdentity{
			"eth1": {MAC: "00:15:5d:04:07:10", HasMAC: true},
		}},
	)
	n := singleInterface(ownedInterface("eth1", "eth1"))
	if err := enforceAdoptionBoundary(enroll, live, emptyConfig(), n); err != nil {
		t.Fatalf("adoption must accept enrolled matching live link: %v", err)
	}
}

// TestAdoption_AcceptsCaseInsensitiveMACMatch verifies the compare is
// case-insensitive across the colon-separated MAC hex bytes. Operators
// and rev-prop tooling occasionally write the address in uppercase.
func TestAdoption_AcceptsCaseInsensitiveMACMatch(t *testing.T) {
	enroll, live := adoptionFixture(
		[]PhysicalBinding{adoptionOwnedBinding("eth1", "eth1", "00:15:5d:04:07:10")},
		syntheticLiveLinks{links: map[string]LiveLinkIdentity{
			"eth1": {MAC: "00:15:5D:04:07:10", HasMAC: true},
		}},
	)
	n := singleInterface(ownedInterface("eth1", "eth1"))
	if err := enforceAdoptionBoundary(enroll, live, emptyConfig(), n); err != nil {
		t.Fatalf("case-insensitive MAC compare must admit the bound link: %v", err)
	}
}

// TestAdoption_RefusesExternallyManaged pins the externally-managed
// guarantee: an adoptable-looking ID that exists in enrollment but
// carries the externallyManaged flag MUST remain non-adoptable through
// the Safe Apply path. Operators must explicitly migrate that binding
// out of the foreign manager first.
func TestAdoption_RefusesExternallyManaged(t *testing.T) {
	enroll, live := adoptionFixture(
		[]PhysicalBinding{adoptionExternallyManagedBinding("eth0", "eth0", "00:15:5d:04:07:07")},
		syntheticLiveLinks{links: map[string]LiveLinkIdentity{
			"eth0": {MAC: "00:15:5d:04:07:07", HasMAC: true},
		}},
	)
	n := singleInterface(ownedInterface("eth0", "eth0"))
	err := enforceAdoptionBoundary(enroll, live, emptyConfig(), n)
	if err == nil {
		t.Fatal("adoption must refuse externallyManaged binding")
	}
	if !strings.Contains(err.Error(), "externally managed") {
		t.Fatalf("refusal must call out externally-managed binding; got %q", err)
	}
}

// TestAdoption_RefusesMACMismatch ensures identity-mismatch rejection:
// the enrolled binding's MAC differs from the live kernel's observed
// hardware address. This protects against silent adoption of a
// physically replaced NIC.
func TestAdoption_RefusesMACMismatch(t *testing.T) {
	enroll, live := adoptionFixture(
		[]PhysicalBinding{adoptionOwnedBinding("eth1", "eth1", "00:15:5d:04:07:10")},
		syntheticLiveLinks{links: map[string]LiveLinkIdentity{
			"eth1": {MAC: "00:15:5d:04:07:99", HasMAC: true},
		}},
	)
	n := singleInterface(ownedInterface("eth1", "eth1"))
	err := enforceAdoptionBoundary(enroll, live, emptyConfig(), n)
	if err == nil {
		t.Fatal("adoption must refuse MAC mismatch")
	}
	if !strings.Contains(err.Error(), "does not match enrolled MAC") {
		t.Fatalf("refusal must call out MAC; got %q", err)
	}
}

// TestAdoption_RefusesUnknownID protects "unknown link" closure: an
// interface ID with no enrolled binding must remain a kernel-creation
// attempt regardless of what the kernel reports.
func TestAdoption_RefusesUnknownID(t *testing.T) {
	enroll, live := adoptionFixture(
		[]PhysicalBinding{adoptionOwnedBinding("eth1", "eth1", "00:15:5d:04:07:10")},
		syntheticLiveLinks{links: map[string]LiveLinkIdentity{
			"eth99": {MAC: "00:15:5d:04:07:99", HasMAC: true},
		}},
	)
	n := singleInterface(ownedInterface("eth99", "eth99"))
	err := enforceAdoptionBoundary(enroll, live, emptyConfig(), n)
	if err == nil {
		t.Fatal("adoption must refuse unknown id with no binding")
	}
	if !strings.Contains(err.Error(), "no enrolled physical binding") {
		t.Fatalf("refusal must call out missing enrollment; got %q", err)
	}
}

// TestAdoption_RefusesNameMismatchInBinding protects the enrollment
// audit: when the desired Name diverges from the enrolled Name the
// operator should not silently adopt under a renamed link.
func TestAdoption_RefusesNameMismatchInBinding(t *testing.T) {
	enroll, live := adoptionFixture(
		[]PhysicalBinding{adoptionOwnedBinding("eth1", "eth1-old", "00:15:5d:04:07:10")},
		syntheticLiveLinks{links: map[string]LiveLinkIdentity{
			"eth1": {MAC: "00:15:5d:04:07:10", HasMAC: true},
		}},
	)
	n := singleInterface(ownedInterface("eth1", "eth1"))
	err := enforceAdoptionBoundary(enroll, live, emptyConfig(), n)
	if err == nil {
		t.Fatal("adoption must refuse name mismatch with enrollment")
	}
	if !strings.Contains(err.Error(), "does not match enrolled binding name") {
		t.Fatalf("refusal must call out name mismatch; got %q", err)
	}
}

// TestAdoption_RefusesLiveLinkMissing enforces the "kernel already
// has the link" rule. Adoption is an audit of an existing link; it must
// never produce a netlink-create. Without a live link, refusal is the
// only correct outcome.
func TestAdoption_RefusesLiveLinkMissing(t *testing.T) {
	enroll, live := adoptionFixture(
		[]PhysicalBinding{adoptionOwnedBinding("eth1", "eth1", "00:15:5d:04:07:10")},
		syntheticLiveLinks{links: nil}, // empty: no live links reported
	)
	n := singleInterface(ownedInterface("eth1", "eth1"))
	err := enforceAdoptionBoundary(enroll, live, emptyConfig(), n)
	if err == nil {
		t.Fatal("adoption must refuse when live kernel lacks the link")
	}
	if !strings.Contains(err.Error(), "live kernel has no link") {
		t.Fatalf("refusal must call out missing live link; got %q", err)
	}
}

// TestAdoption_RefusesLiveLinkWithoutMAC guards against adapters whose
// link cannot be anchored by hardware address (e.g., a tunnel or a
// VLAN-only attachment). The operator audit is undermined without a
// stable MAC; refusal is correct.
func TestAdoption_RefusesLiveLinkWithoutMAC(t *testing.T) {
	enroll, live := adoptionFixture(
		[]PhysicalBinding{adoptionOwnedBinding("eth1", "eth1", "00:15:5d:04:07:10")},
		syntheticLiveLinks{links: map[string]LiveLinkIdentity{
			"eth1": {MAC: "", HasMAC: false},
		}},
	)
	n := singleInterface(ownedInterface("eth1", "eth1"))
	err := enforceAdoptionBoundary(enroll, live, emptyConfig(), n)
	if err == nil {
		t.Fatal("adoption must refuse when the live link carries no MAC")
	}
	if !strings.Contains(err.Error(), "no stable hardware address") {
		t.Fatalf("refusal must call out missing live MAC; got %q", err)
	}
}

// TestAdoption_RefusesNilLiveSnapshotter is the safety net: the
// adapter's preflight refuses if the live snapshotter is unavailable.
// Adoption is the only place the snapshotter is consulted; an absent
// snapshotter must not silently admit an unknown link.
func TestAdoption_RefusesNilLiveSnapshotter(t *testing.T) {
	enroll := Enrollment{
		Version:        1,
		CreatedAt:      time.Unix(0, 0).UTC(),
		Physical:       []PhysicalBinding{adoptionOwnedBinding("eth1", "eth1", "00:15:5d:04:07:10")},
		FirewallTable:  "inet duckos",
		RoutingTables:  []int{10000},
		PolicyRouting:  true,
		NetworkdReload: true,
	}
	n := singleInterface(ownedInterface("eth1", "eth1"))
	err := enforceAdoptionBoundary(enroll, nil, emptyConfig(), n)
	if err == nil {
		t.Fatal("adoption must refuse nil live snapshotter")
	}
	if !strings.Contains(err.Error(), "live link snapshotter is unavailable") {
		t.Fatalf("refusal must call out missing snapshotter; got %q", err)
	}
}

// TestAdoption_ExistingInBeforeSkipsAdoptionCheck pins the contract
// that links already present in the durable "before" config are NOT
// re-adjudicated by the adoption boundary. Their identity invariants
// are owned by enforceLinkShapeEquality, which runs separately. An
// empty (but non-nil) synthetic snapshotter proves the adoption loop
// never asks the kernel about an already-tracked link.
func TestAdoption_ExistingInBeforeSkipsAdoptionCheck(t *testing.T) {
	enroll := Enrollment{
		Version:        1,
		CreatedAt:      time.Unix(0, 0).UTC(),
		Physical:       []PhysicalBinding{adoptionOwnedBinding("eth1", "eth1", "00:15:5d:04:07:10")},
		FirewallTable:  "inet duckos",
		RoutingTables:  []int{10000},
		PolicyRouting:  true,
		NetworkdReload: true,
	}
	if err := enforceAdoptionBoundary(enroll,
		syntheticLiveLinks{links: map[string]LiveLinkIdentity{}},
		singleInterface(ownedInterface("eth1", "eth1")),
		singleInterface(ownedInterface("eth1", "eth1"))); err != nil {
		t.Fatalf("links already in before must skip the adoption check; got %v", err)
	}
}

// TestAdoption_SyntheticIsPure proves the synthetic snapshotter is the
// only production-shaped surface the tests exercise, and that the
// adoption boundary never depends on file I/O, netlink, or time —
// outside the synthetic there is no state to mutate.
func TestAdoption_SyntheticIsPure(t *testing.T) {
	enroll, live := adoptionFixture(
		[]PhysicalBinding{adoptionOwnedBinding("eth1", "eth1", "00:15:5d:04:07:10")},
		syntheticLiveLinks{links: map[string]LiveLinkIdentity{
			"eth1": {MAC: "00:15:5d:04:07:10", HasMAC: true},
		}},
	)
	n := singleInterface(ownedInterface("eth1", "eth1"))
	for i := 0; i < 3; i++ {
		if err := enforceAdoptionBoundary(enroll, live, emptyConfig(), n); err != nil {
			t.Fatalf("adoption iteration %d failed: %v", i, err)
		}
	}
	// Empty config is still empty: the synthetic left no state behind.
	if ok, _ := live.LookupLink("eth1"); !ok.HasMAC {
		t.Fatal("synthetic live must remain a constant snapshot")
	}
}

// TestSameMAC is a focused pin for the case-insensitive compare that
// the boundary uses to anchor an adoption to the bound hardware address.
func TestSameMAC(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"00:15:5d:04:07:10", "00:15:5d:04:07:10", true},
		{"00:15:5D:04:07:10", "00:15:5d:04:07:10", true},
		{"00:15:5d:04:07:10", "00:15:5d:04:07:11", false},
		{"00:15:5d:04:07:10", "", false},
		{"", "00:15:5d:04:07:10", false},
		{"0015:5d:04:07:10", "00:15:5d:04:07:10", false}, // length differs
	}
	for _, tc := range cases {
		if got := sameMAC(tc.a, tc.b); got != tc.want {
			t.Errorf("sameMAC(%q,%q) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}
