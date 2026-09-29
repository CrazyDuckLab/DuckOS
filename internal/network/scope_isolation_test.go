// internal/network/scope_isolation_test.go: prove that a config/apply
// transaction which does not actually mutate a given link leaves the
// link's networkd artifact untouched, rejects any explicit mutation of
// an externally-managed binding, and treats unrelated interfaces as
// out-of-scope. These tests pin the management-scope invariant the
// managed-routing adapter must preserve across every Safe Apply cycle.
package network

import (
	"sort"
	"testing"
	"time"
)

func ethLink(id, name string, addrs ...string) Interface {
	return Interface{
		ID:         id,
		Name:       name,
		Kind:       "physical",
		Role:       "lan",
		MTU:        1500,
		DHCPClient: "none",
		AcceptRA:   false,
		Management: false,
		Addresses:  addrs,
	}
}

func baselineConfig() Config {
	return Config{
		Version: 1,
		Interfaces: []Interface{
			ethLink("eth0", "eth0", "172.18.4.149/16"),
			ethLink("eth1", "eth1", "10.77.10.2/24"),
			ethLink("eth2", "eth2", "10.77.20.2/24"),
		},
		Routes:   []Route{},
		Firewall: Firewall{Forwards: []Forward{}, Masquerade: []string{}, Rules: []Rule{}},
		DNS:      DNS{Upstreams: []string{}, Records: []Record{}},
		Management: &ManagementPolicy{Rules: []ManagementRule{
			{ID: "witness-bootstrap", Interface: "eth0", Service: "ssh", Port: 22, Sources: []string{"10.77.10.0/24"}},
		}},
	}
}

func enrollmentFor(t *testing.T, externals ...string) Enrollment {
	t.Helper()
	externalsMap := map[string]bool{}
	for _, e := range externals {
		externalsMap[e] = true
	}
	return Enrollment{
		Version:        1,
		CreatedAt:      time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		FirewallTable:  "inet duckos",
		RoutingTables:  []int{10000},
		PolicyRouting:  true,
		NetworkdReload: true,
		Physical: []PhysicalBinding{
			{ID: "eth0", Name: "eth0", MAC: "00:15:5d:04:07:07", ExternallyManaged: externalsMap["eth0"]},
			{ID: "eth1", Name: "eth1", MAC: "00:15:5d:04:07:0a", ExternallyManaged: externalsMap["eth1"]},
			{ID: "eth2", Name: "eth2", MAC: "00:15:5d:04:07:0b", ExternallyManaged: externalsMap["eth2"]},
		},
	}
}

// TestScopeIsolation_Eth1OnlyLeavesEth0Untouched is the core regression:
// the candidate moves only eth1's address. The adapter must NOT include
// any 10-duckos-eth0.network file in either the before or next plans.
func TestScopeIsolation_Eth1OnlyLeavesEth0Untouched(t *testing.T) {
	b := baselineConfig()
	n := baselineConfig()
	for i := range n.Interfaces {
		if n.Interfaces[i].ID == "eth1" {
			n.Interfaces[i].Addresses = []string{"10.77.10.3/24"}
		}
	}
	changed := changedInterfaceIDs(b, n)
	if changed["eth0"] || changed["eth2"] {
		t.Fatalf("eth0/eth2 must not appear in the changed set for an eth1-only transaction, got %v", changed)
	}
	if !changed["eth1"] {
		t.Fatalf("eth1 must be in the changed set, got %v", changed)
	}
	pb, err := Render(b)
	if err != nil {
		t.Fatal(err)
	}
	pn, err := Render(n)
	if err != nil {
		t.Fatal(err)
	}
	pbNetd := filterNetworkdByIDs(pb.Networkd, changed)
	pnNetd := filterNetworkdByIDs(pn.Networkd, changed)
	if _, ok := pbNetd["10-duckos-eth0.network"]; ok {
		t.Fatalf("eth0 artifact must not appear in the filtered before plan; keys=%v", keys(pbNetd))
	}
	if _, ok := pnNetd["10-duckos-eth0.network"]; ok {
		t.Fatalf("eth0 artifact must not appear in the filtered next plan; keys=%v", keys(pnNetd))
	}
	if _, ok := pnNetd["10-duckos-eth1.network"]; !ok {
		t.Fatalf("eth1 artifact must be present in the filtered next plan; keys=%v", keys(pnNetd))
	}
	if _, ok := pnNetd["10-duckos-eth2.network"]; ok {
		t.Fatalf("eth2 artifact must not be in the filtered next plan; keys=%v", keys(pnNetd))
	}
}

// TestScopeIsolation_NoInterfaceChangesSkipsAllWrites pins that an apply
// whose candidate equals the baseline produces zero rendered networkd
// artifacts. The adapter must skip the entire replace/reload sequence.
func TestScopeIsolation_NoInterfaceChangesSkipsAllWrites(t *testing.T) {
	b := baselineConfig()
	n := baselineConfig()
	changed := changedInterfaceIDs(b, n)
	if len(changed) != 0 {
		t.Fatalf("identical baseline/candidate must produce an empty change set, got %v", changed)
	}
	pn, err := Render(n)
	if err != nil {
		t.Fatal(err)
	}
	pnNetd := filterNetworkdByIDs(pn.Networkd, changed)
	if len(pnNetd) != 0 {
		t.Fatalf("no networkd artifact should be queued for an empty change set, got %v", keys(pnNetd))
	}
}

// TestScopeIsolation_ExternallyManagedMutationRejected ensures the
// adapter refuses to render or reload for an interface the enrollment
// flagged as externally managed. The candidate must leave eth0's state
// untouched; any explicit mutation must fail closed.
func TestScopeIsolation_ExternallyManagedMutationRejected(t *testing.T) {
	b := baselineConfig()
	n := baselineConfig()
	for i := range n.Interfaces {
		if n.Interfaces[i].ID == "eth0" {
			n.Interfaces[i].Addresses = []string{"172.18.4.150/16"}
		}
	}
	e := enrollmentFor(t, "eth0")
	if err := rejectExternallyManagedMutation(e, b, n); err == nil {
		t.Fatalf("external-management mutation must be rejected, got nil")
	}
	// An identical-state proposal must NOT be rejected — the adapter must
	// still be able to record the existing state in a fresh durable intent.
	if err := rejectExternallyManagedMutation(e, b, baselineConfig()); err != nil {
		t.Fatalf("identical external interface must not be rejected: %v", err)
	}
}

func TestScopeIsolationExternalRouteOnlyMutationRejected(t *testing.T) {
	b, n := baselineConfig(), baselineConfig()
	n.Routes = []Route{{ID: "private", Destination: "198.51.100.0/24", Interface: "eth0", Table: 10000}}
	if _, err := Render(n); err != nil {
		t.Fatal(err)
	}
	if err := rejectExternallyManagedMutation(enrollmentFor(t, "eth0"), b, n); err == nil {
		t.Fatal("route-only external takeover admitted")
	}
	n.Routes[0].Interface = "eth1"
	if err := rejectExternallyManagedMutation(enrollmentFor(t, "eth0"), b, n); err != nil {
		t.Fatal("owned route rejected", err)
	}
}

// TestScopeIsolation_ScopedConfigsContainOnlyChangedInterfaces proves
// the scoped configs the networkd daemon receives are bounded to the
// changed IDs. Unchanged interfaces are absent, so the daemon's
// ownership and convergence checks can never raise a foreign claim
// against them.
func TestScopeIsolation_ScopedConfigsContainOnlyChangedInterfaces(t *testing.T) {
	b := baselineConfig()
	n := baselineConfig()
	for i := range n.Interfaces {
		if n.Interfaces[i].ID == "eth1" {
			n.Interfaces[i].Addresses = []string{"10.77.10.3/24"}
		}
	}
	changed := changedInterfaceIDs(b, n)
	sb, sn := scopeConfigsToInterfaceIDs(b, n, changed)
	for _, c := range []Config{sb, sn} {
		for _, i := range c.Interfaces {
			if i.ID != "eth1" {
				t.Fatalf("scoped config must only contain eth1, got %v", linkNames(c.Interfaces))
			}
		}
	}
}

// TestScopeIsolation_MultipleUnrelatedInterfacesStayOutOfScope confirms
// that a candidate with no per-interface field changes produces no
// .network artifact for any interface, even when baseline lists many.
func TestScopeIsolation_MultipleUnrelatedInterfacesStayOutOfScope(t *testing.T) {
	b := Config{
		Version: 1,
		Interfaces: []Interface{
			ethLink("eth0", "eth0", "172.18.4.149/16"),
			ethLink("eth1", "eth1", "10.77.10.2/24"),
			ethLink("eth2", "eth2", "10.77.20.2/24"),
			ethLink("eth3", "eth3", "10.77.30.2/24"),
			ethLink("eth4", "eth4", "10.77.40.2/24"),
		},
		Routes: []Route{},
	}
	n := b // identical
	changed := changedInterfaceIDs(b, n)
	pn, _ := Render(n)
	pnNetd := filterNetworkdByIDs(pn.Networkd, changed)
	if len(pnNetd) != 0 {
		t.Fatalf("unchanged interfaces must not produce .network artifacts, got %v", keys(pnNetd))
	}
}

func keys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func linkNames(in []Interface) []string {
	out := make([]string, 0, len(in))
	for _, i := range in {
		out = append(out, i.Name)
	}
	return out
}
