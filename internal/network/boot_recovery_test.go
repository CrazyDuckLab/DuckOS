package network

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/state"
)

// syntheticBootKernel satisfies the BootKernel contract with the same
// synthetic run functions used by managed_routing tests, so the no-pending
// gate can be exercised without touching a real kernel.
type syntheticBootKernel struct {
	routes *syntheticRoutes
	rules  *syntheticPolicy
	tables []int
}

func (k *syntheticBootKernel) EnrolledTables() []int { return append([]int(nil), k.tables...) }
func (k *syntheticBootKernel) CaptureRoutes(ctx context.Context) (RouteSnapshot, error) {
	raw, err := k.routes.run(ctx, []string{"-j", "-N", "-4", "route", "show", "table", "all"})
	if err != nil {
		return RouteSnapshot{}, err
	}
	v4, err := decodePrivateRoutes(raw, "ipv4", map[int]bool{10000: true})
	if err != nil {
		return RouteSnapshot{}, err
	}
	raw, err = k.routes.run(ctx, []string{"-j", "-N", "-6", "route", "show", "table", "all"})
	if err != nil {
		return RouteSnapshot{}, err
	}
	v6, err := decodePrivateRoutes(raw, "ipv6", map[int]bool{10000: true})
	if err != nil {
		return RouteSnapshot{}, err
	}
	all := append(v4, v6...)
	return normalizeRouteSnapshot(k.tables, all)
}
func (k *syntheticBootKernel) CaptureRules(ctx context.Context) (PolicySnapshot, error) {
	s := PolicySnapshot{Version: 1}
	for _, family := range []string{"ipv4", "ipv6"} {
		flag := "-4"
		if family == "ipv6" {
			flag = "-6"
		}
		raw, err := k.rules.run(ctx, []string{"-j", "-N", "-details", flag, "rule", "show"})
		if err != nil {
			return PolicySnapshot{}, err
		}
		rules, err := decodePolicyInventory(raw, family)
		if err != nil {
			return PolicySnapshot{}, err
		}
		s.Rules = append(s.Rules, rules...)
	}
	return s, nil
}

// pendingRecoveryFixture preserves the historical relationship between the
// Safe Apply fixture and the boot recovery fixture. The journaled "before"
// still anchors on the static fixture so file inventory remains valid; the
// synthetic kernel is provided only so the no-pending gate can be exercised
// elsewhere.
func pendingRecoveryFixture(t *testing.T) (*ManagedRoutingAdapter, BootKernel, json.RawMessage, json.RawMessage) {
	t.Helper()
	a, before, next, _ := managedRoutingFixture(t)
	_, _, _, route, policy := routedFixture(t)
	kernel := &syntheticBootKernel{routes: route, rules: policy, tables: []int{10000}}
	return a, BootKernel(kernel), before, next
}

// noPendingFixture anchors the kernel synthetic live state and the durable
// intent on the same routed configuration. This is the fixture the new
// no-pending root-enrollment gate requires to exercise intent matching.
func noPendingFixture(t *testing.T) (*ManagedRoutingAdapter, BootKernel, *syntheticRoutes, *syntheticPolicy, json.RawMessage) {
	t.Helper()
	k, c, _, route, policy := routedFixture(t)
	// Seed live state to match the routed config.
	route.live = append([]KernelRoute(nil), route.known...)
	policy.live = append([]KernelPolicyRule(nil), policy.known...)
	// Build a minimal adapter whose artifact directory holds the rendered
	// routed config so the no-pending file inventory still passes.
	d, err := OpenArtifactDirectory(PrivateTempDir(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	p, err := Render(c)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Replace(nil, p.Networkd); err != nil {
		t.Fatal(err)
	}
	a := &ManagedRoutingAdapter{
		kernel:          k,
		files:           d,
		daemon:          &syntheticRoutingDaemon{files: d, firewall: &syntheticGuardedFirewall{syntheticFirewallKernel: syntheticFirewallKernel{live: NFTSnapshot{Objects: []json.RawMessage{}}}}},
		checkOwnership:  func(context.Context, Config, Config) error { return nil },
		checkManagement: func(context.Context, Config) error { return nil },
	}
	kernelIface := &syntheticBootKernel{routes: route, rules: policy, tables: []int{10000}}
	before, _ := json.Marshal(c)
	return a, BootKernel(kernelIface), route, policy, before
}

func TestBootArtifactRecoveryPreservesPendingUntilRuntimeVerification(t *testing.T) {
	for _, phase := range []state.Phase{state.Prepared, state.Applying, state.AwaitingConfirmation, state.RollingBack, state.RecoveryRequired} {
		t.Run(string(phase), func(t *testing.T) {
			a, kernel, before, next := pendingRecoveryFixture(t)
			dir := t.TempDir()
			if err := os.Chmod(dir, 0700); err != nil {
				t.Fatal(err)
			}
			validate := func(raw json.RawMessage) error { _, err := Decode(raw); return err }
			s, err := state.Open(dir, before, validate)
			if err != nil {
				t.Fatal(err)
			}
			initial, _ := s.Snapshot()
			recovery, err := a.Capture(context.Background(), before, next)
			if err != nil {
				t.Fatal(err)
			}
			tx, err := s.PrepareWithRecovery(initial.Current.ID, next, "owner", "boot recovery", recovery)
			if err != nil {
				t.Fatal(err)
			}
			if phase != state.Prepared {
				if err := s.Transition(tx.ID, state.Applying, time.Time{}); err != nil {
					t.Fatal(err)
				}
				if phase == state.AwaitingConfirmation {
					if err := s.Transition(tx.ID, phase, time.Now().Add(2*time.Minute)); err != nil {
						t.Fatal(err)
					}
				} else if phase == state.RollingBack || phase == state.RecoveryRequired {
					if err := s.Transition(tx.ID, state.RollingBack, time.Time{}); err != nil {
						t.Fatal(err)
					}
					if phase == state.RecoveryRequired {
						if err := s.Transition(tx.ID, phase, time.Time{}); err != nil {
							t.Fatal(err)
						}
					}
				}
				b, _ := Decode(before)
				n, _ := Decode(next)
				pb, _ := Render(b)
				pn, _ := Render(n)
				if err := a.files.Replace(pb.Networkd, pn.Networkd); err != nil {
					t.Fatal(err)
				}
			}
			s.Close()
			s, err = state.OpenExisting(dir, validate)
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			e := syntheticEnrollment()
			e.NetworkdReload, e.PolicyRouting, e.RoutingTables = true, true, []int{10000}
			for i := 0; i < 2; i++ {
				if err := RecoverRoutingArtifacts(context.Background(), s, a.files, e, kernel); err != nil {
					t.Fatal(err)
				}
			}
			final, _ := s.Snapshot()
			if final.Pending == nil || final.Pending.ID != tx.ID || final.Pending.Phase != phase || final.Current.ID != initial.Current.ID {
				t.Fatal("boot phase confirmed or cleared pending transaction")
			}
		})
	}
}

func TestBootRecoveryDoesNotRewriteUnchangedExternalArtifact(t *testing.T) {
	a, kernel, before, _ := pendingRecoveryFixture(t)
	n, err := Decode(before)
	if err != nil {
		t.Fatal(err)
	}
	n.Routes = append(n.Routes, Route{ID: "scope-route", Interface: "uplink", Destination: "198.18.0.0/24", Table: 10000})
	next, err := json.Marshal(n)
	if err != nil {
		t.Fatal(err)
	}
	e := syntheticEnrollment()
	e.NetworkdReload = true
	e.PolicyRouting = true
	e.RoutingTables = []int{10000}
	e.Physical[1].ExternallyManaged = true
	a.enrollment = e
	raw, err := a.Capture(context.Background(), before, next)
	if err != nil {
		t.Fatal(err)
	}
	s, err := state.Open(PrivateTempDir(t), before, func(raw json.RawMessage) error { _, err := Decode(raw); return err })
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	snap, _ := s.Snapshot()
	if _, err := s.PrepareWithRecovery(snap.Current.ID, next, "owner", "scope", raw); err != nil {
		t.Fatal(err)
	}
	old, err := a.files.root.Stat("10-duckos-inside.network")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := RecoverRoutingArtifacts(context.Background(), s, a.files, e, kernel); err != nil {
			t.Fatal(err)
		}
	}
	now, err := a.files.root.Stat("10-duckos-inside.network")
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(old, now) || !old.ModTime().Equal(now.ModTime()) {
		t.Fatal("boot recovery rewrote unchanged external artifact")
	}
}

func TestBootArtifactRecoveryRefusesOrphansAndPreservesForeignBytes(t *testing.T) {
	a, kernel, before, _ := pendingRecoveryFixture(t)
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	s, err := state.Open(dir, before, func(raw json.RawMessage) error { _, err := Decode(raw); return err })
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	e := syntheticEnrollment()
	e.NetworkdReload, e.PolicyRouting, e.RoutingTables = true, true, []int{10000}
	if err := RecoverRoutingArtifacts(context.Background(), s, a.files, e, kernel); err != nil {
		t.Fatal(err)
	}
	if err := a.files.write("10-duckos-orphan.network", "[Match]\nName=foreign0\n"); err != nil {
		t.Fatal(err)
	}
	if err := RecoverRoutingArtifacts(context.Background(), s, a.files, e, kernel); err == nil {
		t.Fatal("ignored orphan managed artifact")
	}
	if _, err := a.files.read("10-duckos-orphan.network"); err != nil {
		t.Fatal("deleted orphan evidence")
	}
	// A missing durable journal must block startup rather than generating one.
	s.Close()
	if err := os.Rename(filepath.Join(dir, "state.json"), filepath.Join(dir, "saved-state.json")); err != nil {
		t.Fatal(err)
	}
	if opened, err := state.OpenExisting(dir, func(raw json.RawMessage) error { _, err := Decode(raw); return err }); err == nil {
		opened.Close()
		t.Fatal("lost journal replaced")
	}
}

func TestBootNoPendingGateRejectsRouteDriftBeforeNetworkd(t *testing.T) {
	a, kernel, route, _, before := noPendingFixture(t)
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	s, err := state.Open(dir, before, func(raw json.RawMessage) error { _, err := Decode(raw); return err })
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	e := syntheticEnrollment()
	e.NetworkdReload, e.PolicyRouting, e.RoutingTables = true, true, []int{10000}
	// Baseline: live state matches durable intent.
	if err := RecoverRoutingArtifacts(context.Background(), s, a.files, e, kernel); err != nil {
		t.Fatal(err)
	}
	// Foreign route appears in an enrolled table.
	drift := route.live[0]
	drift.Metric++
	route.live = append(route.live, drift)
	if err := RecoverRoutingArtifacts(context.Background(), s, a.files, e, kernel); err == nil {
		t.Fatal("drift in enrolled private table was not rejected")
	}
	route.live = route.live[:len(route.live)-1]
	// Foreign route in a revoked table scope requires a kernel whose capture
	// does not silently drop it. That defensive case is covered in
	// TestValidateBootOwnershipDirect.
	// Required route from intent is missing.
	original := route.live[0]
	route.live = nil
	if err := RecoverRoutingArtifacts(context.Background(), s, a.files, e, kernel); err == nil {
		t.Fatal("missing required private route was not rejected")
	}
	route.live = []KernelRoute{original}
}

func TestBootNoPendingGateRejectsPolicyDriftBeforeNetworkd(t *testing.T) {
	a, kernel, _, policy, before := noPendingFixture(t)
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	s, err := state.Open(dir, before, func(raw json.RawMessage) error { _, err := Decode(raw); return err })
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	e := syntheticEnrollment()
	e.NetworkdReload, e.PolicyRouting, e.RoutingTables = true, true, []int{10000}
	if err := RecoverRoutingArtifacts(context.Background(), s, a.files, e, kernel); err != nil {
		t.Fatal(err)
	}
	// Foreign policy rule appears in the kernel.
	foreign := policy.live[0]
	foreign.Priority = 21000
	policy.live = append(policy.live, foreign)
	if err := RecoverRoutingArtifacts(context.Background(), s, a.files, e, kernel); err == nil {
		t.Fatal("foreign policy rule was not rejected before networkd")
	}
	policy.live = policy.live[:len(policy.live)-1]
	// Required policy rule from intent is missing.
	policy.live = nil
	if err := RecoverRoutingArtifacts(context.Background(), s, a.files, e, kernel); err == nil {
		t.Fatal("missing required policy rule was not rejected")
	}
}

func TestBootNoPendingGateRejectsNilKernelAndRevokedScope(t *testing.T) {
	_, kernel, _, _, before := noPendingFixture(t)
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	s, err := state.Open(dir, before, func(raw json.RawMessage) error { _, err := Decode(raw); return err })
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	e := syntheticEnrollment()
	e.NetworkdReload, e.PolicyRouting, e.RoutingTables = true, true, []int{10000}
	// Nil kernel is an explicit refusal, not a fallback to a synthetic path.
	if err := RecoverRoutingArtifacts(context.Background(), s, mustArtifactDir(t), e, nil); err == nil {
		t.Fatal("nil kernel was silently accepted")
	}
	// Kernel scope diverges from root enrollment.
	mismatched := &syntheticBootKernel{routes: kernel.(*syntheticBootKernel).routes, rules: kernel.(*syntheticBootKernel).rules, tables: []int{10001}}
	if err := RecoverRoutingArtifacts(context.Background(), s, mustArtifactDir(t), e, mismatched); err == nil {
		t.Fatal("kernel/root enrollment divergence was silently accepted")
	}
}

func mustArtifactDir(t *testing.T) *ArtifactDirectory {
	t.Helper()
	d, err := OpenArtifactDirectory(PrivateTempDir(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	return d
}

// TestBootNoPendingGateAlignsWithPendingBranch checks that the same BootKernel
// contract is honored on both paths. The pending branch must also reject a
// kernel whose scope does not match the durable intent.
func TestBootNoPendingGateAlignsWithPendingBranch(t *testing.T) {
	a, kernel, before, next := pendingRecoveryFixture(t)
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	validate := func(raw json.RawMessage) error { _, err := Decode(raw); return err }
	s, err := state.Open(dir, before, validate)
	if err != nil {
		t.Fatal(err)
	}
	initial, _ := s.Snapshot()
	recovery, err := a.Capture(context.Background(), before, next)
	if err != nil {
		s.Close()
		t.Fatal(err)
	}
	if _, err := s.PrepareWithRecovery(initial.Current.ID, next, "owner", "test", recovery); err != nil {
		s.Close()
		t.Fatal(err)
	}
	s.Close()
	s, err = state.OpenExisting(dir, validate)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	e := syntheticEnrollment()
	e.NetworkdReload, e.PolicyRouting, e.RoutingTables = true, true, []int{10000}
	// A pending transaction with revoked enrollment must still be refused.
	revoked := &syntheticBootKernel{routes: kernel.(*syntheticBootKernel).routes, rules: kernel.(*syntheticBootKernel).rules, tables: []int{10001}}
	if err := RecoverRoutingArtifacts(context.Background(), s, a.files, e, revoked); err == nil {
		t.Fatal("pending recovery accepted revoked kernel scope")
	}
}
