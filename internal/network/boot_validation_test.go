package network

import (
	"context"
	"errors"
	"testing"
)

// unboundedBootKernel returns whatever the test seeded in live, without the
// table filter that the real iproute2 decoder applies. The boot gate must
// still refuse revoked or missing entries because the gate is a defensive
// check independent of the host decoder's filter.
type unboundedBootKernel struct {
	routes []KernelRoute
	rules  []KernelPolicyRule
	tables []int
}

func (k *unboundedBootKernel) EnrolledTables() []int { return append([]int(nil), k.tables...) }
func (k *unboundedBootKernel) CaptureRoutes(_ context.Context) (RouteSnapshot, error) {
	// Return a snapshot that is structurally valid but may contain entries
	// in non-enrolled tables. The boot gate is the layer that must reject
	// those entries; the host decoder only filters by table when it is the
	// one enumerating routes.
	raw, err := normalizeRouteSnapshot(k.tables, k.routes)
	if err != nil {
		// Fall back to a permissive snapshot so the gate, not the decoder,
		// is the only thing that can reject.
		return RouteSnapshot{Version: 1, Tables: append([]int{}, k.tables...), Routes: k.routes}, nil
	}
	_ = raw
	return RouteSnapshot{Version: 1, Tables: append([]int{}, k.tables...), Routes: k.routes}, nil
}
func (k *unboundedBootKernel) CaptureRules(_ context.Context) (PolicySnapshot, error) {
	return PolicySnapshot{Version: 1, Rules: k.rules}, nil
}

func TestValidateBootOwnershipRejectsRevokedTableRoute(t *testing.T) {
	enrolled := syntheticEnrollment()
	enrolled.RoutingTables = []int{10000}
	enrolled.PolicyRouting = true
	before, _ := staticTrafficFixture()
	before.Routes = append(before.Routes, Route{ID: "private", Interface: "uplink", Destination: "0.0.0.0/0", Gateway: "192.0.2.1", Table: 10000, Metric: 100})
	expected, err := PrivateRoutesForConfig(before, []int{10000})
	if err != nil {
		t.Fatal(err)
	}
	rules, err := PolicySnapshotForConfig(before)
	if err != nil {
		t.Fatal(err)
	}
	kernel := &unboundedBootKernel{
		routes: append(expected.Routes, KernelRoute{Family: "ipv4", Table: 10001, Destination: "0.0.0.0/0", Interface: "testwan0", Gateway: "192.0.2.9", Metric: 100}),
		rules:  rules.Rules,
		tables: []int{10000},
	}
	err = validateBootOwnership(context.Background(), kernel, before, enrolled)
	if err == nil || !contains(err.Error(), "revoked private table") {
		t.Fatal("expected revoked-table rejection, got", err)
	}
}

func TestValidateBootOwnershipRejectsRevokedTableRule(t *testing.T) {
	enrolled := syntheticEnrollment()
	enrolled.RoutingTables = []int{10000}
	enrolled.PolicyRouting = true
	before, _ := staticTrafficFixture()
	before.Routes = append(before.Routes, Route{ID: "private", Interface: "uplink", Destination: "0.0.0.0/0", Gateway: "192.0.2.1", Table: 10000, Metric: 100})
	before.PolicyRules = []PolicyRule{{ID: "private-rule", Family: "ipv4", Priority: 20000, From: "198.51.100.0/24", Table: 10000}}
	routes, _ := PrivateRoutesForConfig(before, []int{10000})
	expectedRules, _ := PolicySnapshotForConfig(before)
	revoked := expectedRules.Rules[0]
	revoked.Table = 10002
	kernel := &unboundedBootKernel{
		routes: routes.Routes,
		rules:  []KernelPolicyRule{revoked},
		tables: []int{10000},
	}
	err := validateBootOwnership(context.Background(), kernel, before, enrolled)
	if err == nil || !contains(err.Error(), "revoked private table") {
		t.Fatal("expected revoked-rule rejection, got", err)
	}
}

func TestValidateBootOwnershipRejectsPolicyWithoutEnrollment(t *testing.T) {
	enrolled := syntheticEnrollment()
	enrolled.RoutingTables = []int{10000}
	enrolled.PolicyRouting = false
	before, _ := staticTrafficFixture()
	before.Routes = append(before.Routes, Route{ID: "private", Interface: "uplink", Destination: "0.0.0.0/0", Gateway: "192.0.2.1", Table: 10000, Metric: 100})
	before.PolicyRules = []PolicyRule{{ID: "private-rule", Family: "ipv4", Priority: 20000, From: "198.51.100.0/24", Table: 10000}}
	routes, _ := PrivateRoutesForConfig(before, []int{10000})
	rules, _ := PolicySnapshotForConfig(before)
	kernel := &unboundedBootKernel{routes: routes.Routes, rules: rules.Rules, tables: []int{10000}}
	if err := validateBootOwnership(context.Background(), kernel, before, enrolled); err == nil {
		t.Fatal("expected policy-routing enrollment refusal")
	}
}

func TestValidateBootOwnershipRejectsDriftAndMissingIntent(t *testing.T) {
	enrolled := syntheticEnrollment()
	enrolled.RoutingTables = []int{10000}
	enrolled.PolicyRouting = true
	before, _ := staticTrafficFixture()
	before.Routes = append(before.Routes, Route{ID: "private", Interface: "uplink", Destination: "0.0.0.0/0", Gateway: "192.0.2.1", Table: 10000, Metric: 100})
	before.PolicyRules = []PolicyRule{{ID: "private-rule", Family: "ipv4", Priority: 20000, From: "198.51.100.0/24", Table: 10000}}
	expectedRoutes, _ := PrivateRoutesForConfig(before, []int{10000})
	expectedRules, _ := PolicySnapshotForConfig(before)
	// Drift: gateway changed.
	drift := expectedRoutes.Routes[0]
	drift.Gateway = "192.0.2.99"
	kernel := &unboundedBootKernel{routes: []KernelRoute{drift}, rules: expectedRules.Rules, tables: []int{10000}}
	if err := validateBootOwnership(context.Background(), kernel, before, enrolled); err == nil {
		t.Fatal("expected route drift rejection")
	}
	// Missing required route.
	kernel.routes = nil
	if err := validateBootOwnership(context.Background(), kernel, before, enrolled); err == nil {
		t.Fatal("expected missing-route rejection")
	}
	// Happy path.
	kernel.routes = expectedRoutes.Routes
	if err := validateBootOwnership(context.Background(), kernel, before, enrolled); err != nil {
		t.Fatal(err)
	}
}

func TestValidateBootOwnershipRejectsInvalidInputs(t *testing.T) {
	enrolled := syntheticEnrollment()
	enrolled.RoutingTables = []int{10000}
	enrolled.PolicyRouting = true
	before, _ := staticTrafficFixture()
	kernel := &unboundedBootKernel{tables: []int{10000}}
	if err := validateBootOwnership(context.Background(), nil, before, enrolled); err == nil {
		t.Fatal("expected nil-kernel rejection")
	}
	bad := enrolled
	bad.RoutingTables = []int{}
	if err := validateBootOwnership(context.Background(), kernel, before, bad); err == nil {
		t.Fatal("expected enrollment rejection")
	}
	// Kernel and enrollment disagree on the table set.
	enrolled.RoutingTables = []int{10000}
	kernel.tables = []int{10001}
	if err := validateBootOwnership(context.Background(), kernel, before, enrolled); err == nil {
		t.Fatal("expected kernel/enrollment divergence rejection")
	}
}

func TestValidateBootScopeRejectsMismatch(t *testing.T) {
	enrolled := syntheticEnrollment()
	enrolled.RoutingTables = []int{10000}
	if err := validateBootScope(&unboundedBootKernel{tables: []int{10000}}, enrolled); err != nil {
		t.Fatal(err)
	}
	if err := validateBootScope(&unboundedBootKernel{tables: []int{10001}}, enrolled); err == nil {
		t.Fatal("expected scope mismatch")
	}
	if err := validateBootScope(&unboundedBootKernel{tables: []int{10000, 10001}}, enrolled); err == nil {
		t.Fatal("expected extra table rejection")
	}
	if err := validateBootScope(nil, enrolled); err == nil {
		t.Fatal("expected nil-kernel rejection")
	}
}

func contains(haystack, needle string) bool {
	if len(needle) == 0 {
		return true
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}

var _ = errors.New
