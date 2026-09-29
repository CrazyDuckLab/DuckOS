//go:build linux

package network

import (
	"context"
	"testing"
)

// This validates actual iprule numeric JSON and exact replay in a disposable
// namespace. It is not a test of management reachability or networkd lifecycle.
func TestPolicyRuleNativeApplyRestore(t *testing.T) {
	requireIsolatedNetworkNamespace(t)
	enrollment := syntheticEnrollment()
	enrollment.PolicyRouting = true
	enrollment.RoutingTables = []int{10000, 10001}
	h, err := NewPolicyRuleHost(enrollment)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	before, err := h.Capture(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range before.Rules {
		if r.Priority == 21800 || r.Priority == 21801 {
			t.Fatal("test priority already exists")
		}
	}
	lookup := KernelPolicyRule{Family: "ipv4", Priority: 21800, From: "203.0.113.123/32", Table: 10001, SuppressPrefixLength: -1, Protocol: 4}
	guard := lookup
	guard.Priority++
	guard.Table = 0
	guard.Unreachable = true
	candidate := PolicySnapshot{Version: 1, Rules: append(append([]KernelPolicyRule(nil), before.Rules...), lookup, guard)}
	// Cleanup uses only the captured/candidate union, even if a command loses
	// acknowledgement. It never flushes rules or recreates a host namespace.
	defer func() {
		if err := h.Restore(ctx, before, candidate); err != nil {
			t.Error("native rule cleanup failed", err)
		}
	}()
	if err := h.Apply(ctx, before, candidate); err != nil {
		t.Fatal(err)
	}
	if err := h.Verify(ctx, candidate); err != nil {
		t.Fatal(err)
	}
	if err := h.Restore(ctx, before, candidate); err != nil {
		t.Fatal(err)
	}
	if err := h.Verify(ctx, before); err != nil {
		t.Fatal("before-state not restored", err)
	}
}
