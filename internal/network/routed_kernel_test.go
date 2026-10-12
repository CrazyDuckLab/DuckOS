package network

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

type syntheticGuardedFirewall struct {
	syntheticFirewallKernel
	failGuard, failCandidate bool
	releases                 int
}

func (f *syntheticGuardedFirewall) InstallForwardGuard(ctx context.Context, before NFTSnapshot) (NFTSnapshot, error) {
	if err := f.Verify(ctx, before); err != nil {
		return NFTSnapshot{}, err
	}
	guard, err := guardSnapshot(before)
	if err != nil {
		return NFTSnapshot{}, err
	}
	f.live = guard
	if f.failGuard {
		f.failGuard = false
		return NFTSnapshot{}, errors.New("lost guard acknowledgement")
	}
	return guard, nil
}
func (f *syntheticGuardedFirewall) ApplyGuarded(ctx context.Context, _ Config, before NFTSnapshot) (NFTSnapshot, error) {
	if err := f.Verify(ctx, before); err != nil {
		return NFTSnapshot{}, err
	}
	if err := verifyForwardGuard(before); err != nil {
		return NFTSnapshot{}, err
	}
	s, _ := DecodeNFTSnapshot([]byte(syntheticNFTTable))
	f.live, _ = guardSnapshot(s)
	if f.failCandidate {
		f.failCandidate = false
		return NFTSnapshot{}, errors.New("lost candidate acknowledgement")
	}
	return f.live, nil
}
func (f *syntheticGuardedFirewall) ReleaseForwardGuard(context.Context) error {
	f.releases++
	// Production behavior (see FirewallHost.ReleaseForwardGuard in
	// forward_guard.go): only the reserved forward-guard chain is
	// removed; every other chain / rule / table object that
	// happened to be live at the moment of release stays intact.
	// We model that by filtering out exactly the guard chain from
	// the snapshot rather than zeroing the whole snapshot, so the
	// post-release state matches what a real kernel would have
	// after the delete.
	keep := NFTSnapshot{Objects: make([]json.RawMessage, 0, len(f.live.Objects))}
	for _, raw := range f.live.Objects {
		var object map[string]json.RawMessage
		if json.Unmarshal(raw, &object) != nil {
			continue
		}
		body, ok := object["chain"]
		if !ok {
			keep.Objects = append(keep.Objects, raw)
			continue
		}
		var chain struct{ Name string }
		if json.Unmarshal(body, &chain) != nil {
			keep.Objects = append(keep.Objects, raw)
			continue
		}
		if chain.Name == "duckos_apply_guard" {
			// Drop only the guard chain.
			continue
		}
		keep.Objects = append(keep.Objects, raw)
	}
	f.live = keep
	return nil
}

func routedFixture(t *testing.T) (*RoutedKernel, Config, *syntheticGuardedFirewall, *syntheticRoutes, *syntheticPolicy) {
	t.Helper()
	c, _ := staticTrafficFixture()
	c.Routes = append(c.Routes, Route{ID: "private-default", Interface: "uplink", Destination: "0.0.0.0/0", Gateway: "192.0.2.1", Table: 10000, Metric: 100})
	c.PolicyRules = []PolicyRule{{ID: "private-rule", Family: "ipv4", Priority: 20000, From: "198.51.100.0/24", Table: 10000}}
	r, err := PrivateRoutesForConfig(c, []int{10000})
	if err != nil {
		t.Fatal(err)
	}
	p, err := PolicySnapshotForConfig(c)
	if err != nil {
		t.Fatal(err)
	}
	f := &syntheticGuardedFirewall{syntheticFirewallKernel: syntheticFirewallKernel{live: NFTSnapshot{Objects: []json.RawMessage{}}}}
	route := &syntheticRoutes{known: r.Routes}
	policy := &syntheticPolicy{known: p.Rules}
	k := &RoutedKernel{firewall: f, tables: []int{10000},
		routes: &PrivateRouteHost{tables: []int{10000}, names: map[string]bool{"testwan0": true, "testlan0": true}, run: route.run},
		rules:  &PolicyRuleHost{tables: map[int]bool{10000: true}, run: policy.run}}
	return k, c, f, route, policy
}

func TestRoutedKernelRetainsBarrierAndConsumesAdmission(t *testing.T) {
	k, c, f, route, policy := routedFixture(t)
	ctx := context.Background()
	raw, err := k.Capture(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	guard, err := k.Apply(ctx, c, raw)
	if err != nil || verifyForwardGuard(guard) != nil {
		t.Fatalf("apply: %v", err)
	}
	if len(route.live) == 0 || len(policy.live) == 0 {
		t.Fatal("missing route/policy application")
	}
	if _, err := k.Apply(ctx, c, raw); err == nil {
		t.Fatal("replayed consumed admission")
	}
	if verifyForwardGuard(f.live) != nil {
		t.Fatal("barrier released before outer verification")
	}
	// Recovery constructs a new coordinator, so no in-memory capture survives.
	k = &RoutedKernel{firewall: f, routes: k.routes, rules: k.rules, tables: k.tables}
	if _, err := k.Restore(ctx, c, raw); err != nil {
		t.Fatal(err)
	}
	if len(route.live) != 0 || len(policy.live) != 0 || verifyForwardGuard(f.live) != nil {
		t.Fatal("incomplete guarded recovery")
	}
	if _, err := k.Restore(ctx, c, raw); err != nil {
		t.Fatal("recovery not idempotent", err)
	}
}

func TestRoutedKernelLostAcknowledgementsRecoverWithoutReplay(t *testing.T) {
	for _, point := range []string{"guard", "routes", "rules", "firewall"} {
		t.Run(point, func(t *testing.T) {
			k, c, f, route, policy := routedFixture(t)
			ctx := context.Background()
			raw, err := k.Capture(ctx, c)
			if err != nil {
				t.Fatal(err)
			}
			switch point {
			case "guard":
				f.failGuard = true
			case "routes":
				route.failAfter = true
			case "rules":
				policy.failAfter = 1
			case "firewall":
				f.failCandidate = true
			}
			if _, err := k.Apply(ctx, c, raw); err == nil {
				t.Fatal("uncertain mutation reported success")
			}
			if verifyForwardGuard(f.live) != nil {
				t.Fatal("failure removed guard")
			}
			if _, err := k.Apply(ctx, c, raw); err == nil {
				t.Fatal("retried uncertain mutation")
			}
			if _, err := k.Restore(ctx, c, raw); err != nil {
				t.Fatal(err)
			}
			if len(route.live) != 0 || len(policy.live) != 0 || verifyForwardGuard(f.live) != nil {
				t.Fatal("failed restoration")
			}
		})
	}
}

func TestRoutedKernelRejectsInvalidRecoveryBeforeMutation(t *testing.T) {
	k, c, f, route, policy := routedFixture(t)
	ctx := context.Background()
	raw, err := k.Capture(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	var e routedRecovery
	if err := json.Unmarshal(raw, &e); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*routedRecovery){
		func(e *routedRecovery) { e.Candidate = "wrong" },
		func(e *routedRecovery) { e.Firewall = nil },
		func(e *routedRecovery) { e.Firewall = &NFTSnapshot{} },
		func(e *routedRecovery) { e.Rules = &PolicySnapshot{Version: 1} },
		func(e *routedRecovery) {
			e.Routes = &RouteSnapshot{Version: 1, Tables: []int{10001}, Routes: []KernelRoute{}}
		},
	} {
		copy := e
		mutate(&copy)
		bad, _ := json.Marshal(copy)
		if _, err := k.Restore(ctx, c, bad); err == nil {
			t.Fatal("accepted invalid evidence")
		}
	}
	if len(f.live.Objects) != 0 || route.mutations != 0 || len(policy.commands) != 0 {
		t.Fatal("mutated before validating evidence")
	}
}

func TestRoutedKernelUnknownRouteDriftKeepsBarrier(t *testing.T) {
	k, c, f, route, _ := routedFixture(t)
	ctx := context.Background()
	raw, err := k.Capture(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := k.Apply(ctx, c, raw); err != nil {
		t.Fatal(err)
	}
	drift := route.live[0]
	drift.Metric++
	route.live = append(route.live, drift)
	if _, err := k.Restore(ctx, c, raw); err == nil {
		t.Fatal("overwrote unknown drift")
	}
	if len(route.live) != 2 || verifyForwardGuard(f.live) != nil {
		t.Fatal("lost unknown route or forwarding barrier")
	}
}
