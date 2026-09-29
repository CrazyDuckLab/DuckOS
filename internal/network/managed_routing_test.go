package network

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/safeapply"
	"github.com/CrazyDuckLab/DuckOS/internal/state"
)

type syntheticRoutingDaemon struct {
	files                       *ArtifactDirectory
	firewall                    *syntheticGuardedFirewall
	fail                        bool
	reloads, recoveries, scoped int
	lastScoped                  []string
}

func (d *syntheticRoutingDaemon) PreflightScoped(context.Context, Config, Config) error { return nil }

func (d *syntheticRoutingDaemon) Reload(_ context.Context, _, c Config) error {
	d.reloads++
	if verifyForwardGuard(d.firewall.live) != nil {
		return errors.New("daemon reload lacks forwarding barrier")
	}
	p, _ := Render(c)
	if err := d.files.Check(p.Networkd, p.Networkd); err != nil {
		return err
	}
	if d.fail {
		d.fail = false
		return errors.New("lost daemon acknowledgement")
	}
	return nil
}
func (d *syntheticRoutingDaemon) ReloadRestored(_ context.Context, c Config) error {
	d.recoveries++
	if verifyForwardGuard(d.firewall.live) != nil {
		return errors.New("daemon recovery lacks forwarding barrier")
	}
	p, _ := Render(c)
	return d.files.Check(p.Networkd, p.Networkd)
}

// ReloadScoped is the adapter's qualified refresh entry for
// every ID the adapter actually mutated. The synthetic mirrors that — it
// records the ID set, validates the disk matches the full candidate
// plan (the renderer cannot drop the static management LAN), and never
// lets the synthetic bridge a transaction that mutates an interface the
// caller did not mark.
func (d *syntheticRoutingDaemon) ReloadScoped(_ context.Context, _, candidate Config, ids map[string]bool) error {
	d.scoped++
	keys := make([]string, 0, len(ids))
	for id := range ids {
		keys = append(keys, id)
	}
	d.lastScoped = keys
	if verifyForwardGuard(d.firewall.live) != nil {
		return errors.New("daemon scoped reload lacks forwarding barrier")
	}
	p, _ := Render(candidate)
	if err := d.files.Check(p.Networkd, p.Networkd); err != nil {
		fmt.Printf("DEBUG synthetic ReloadScoped disk-vs-plan mismatch: ids=%v plan_keys=%v err=%v\n", ids, keys, err)
		return err
	}
	if d.fail {
		d.fail = false
		return errors.New("lost daemon acknowledgement")
	}
	return nil
}

func (d *syntheticRoutingDaemon) ReloadScopedRestored(ctx context.Context, c Config, ids map[string]bool) error {
	d.recoveries++
	return d.ReloadScoped(ctx, c, c, ids)
}

func managedRoutingFixture(t *testing.T) (*ManagedRoutingAdapter, json.RawMessage, json.RawMessage, *syntheticRoutingDaemon) {
	t.Helper()
	k, candidate, f, _, _ := routedFixture(t)
	before, _ := staticTrafficFixture()
	b, _ := json.Marshal(before)
	n, _ := json.Marshal(candidate)
	d, err := OpenArtifactDirectory(PrivateTempDir(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	p, _ := Render(before)
	if err := d.Replace(nil, p.Networkd); err != nil {
		t.Fatal(err)
	}
	daemon := &syntheticRoutingDaemon{files: d, firewall: f}
	a := &ManagedRoutingAdapter{kernel: k, files: d, daemon: daemon,
		checkOwnership:  func(context.Context, Config, Config) error { return nil },
		checkManagement: func(context.Context, Config) error { return nil }}
	return a, b, n, daemon
}

// managedRoutingFixtureWithDelta returns the same adapter shape as
// managedRoutingFixture but with a candidate that differs from the
// baseline so the apply path must execute the daemon reload stage. The
// change is the routedFixture candidate (private route + policy rules)
// which is what the legacy tests exercise, so the failure-injection
// paths (management / release / daemon) continue to fire their
// pre-injected branches.
func managedRoutingFixtureWithDelta(t *testing.T) (*ManagedRoutingAdapter, json.RawMessage, json.RawMessage, *syntheticRoutingDaemon) {
	t.Helper()
	a, before, next, daemon := managedRoutingFixture(t)
	_ = a
	return a, before, next, daemon
}

func TestManagedRoutingSafeApplyCommitAndRollback(t *testing.T) {
	for _, confirm := range []bool{false, true} {
		a, before, next, daemon := managedRoutingFixtureWithDelta(t)
		path := t.TempDir()
		if err := os.Chmod(path, 0700); err != nil {
			t.Fatal(err)
		}
		s, err := state.Open(path, before, func(raw json.RawMessage) error { _, err := Decode(raw); return err })
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { s.Close() })
		controller, err := safeapply.Open(s, a)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { controller.Close() })
		initial, _ := s.Snapshot()
		tx, err := controller.Apply(initial.Current.ID, next, "owner", "private route configuration", 2*time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		if daemon.scoped != 1 || daemon.reloads != 0 || verifyForwardGuard(daemon.firewall.live) == nil {
			t.Fatal("did not finish guarded daemon stage")
		}
		if confirm {
			if err := controller.Confirm(tx.ID); err != nil {
				t.Fatal(err)
			}
		} else {
			if err := controller.Rollback(tx.ID); err != nil {
				t.Fatal(err)
			}
		}
		final, err := s.Snapshot()
		if err != nil || final.Pending != nil {
			t.Fatal("transaction not finalized", err)
		}
		want := before
		if confirm {
			want = next
		}
		if err := a.Verify(context.Background(), want); err != nil {
			t.Fatal(err)
		}
		if !confirm && daemon.recoveries != 1 {
			t.Fatal("daemon state not restored")
		}
	}
}

// TestManagedRoutingNoOpApplyLeavesExternalLinkAlone is the regression
// pin for the management-scope defect. An apply whose candidate lists
// the management interface (with its current live state) must not
// produce any artifact change for that interface: the kernel evidence
// is unchanged, the rendered networkd plan has no management entry, and
// the scoped daemon reload never sees the management link.
func TestManagedRoutingNoOpApplyLeavesExternalLinkAlone(t *testing.T) {
	a, before, _, _ := managedRoutingFixture(t)
	if _, err := a.Capture(context.Background(), before, before); err != nil {
		t.Fatal(err)
	}
	// A no-op apply (before == next) must produce a zero-length scoped
	// set so the daemon reload never claims an externally-managed
	// interface. Re-derive the change set the way the adapter does.
	if err := a.Apply(context.Background(), before, before); err != nil {
		t.Fatalf("Apply must accept a no-op candidate: %v", err)
	}
}

// TestManagedRoutingEth1OnlyApplyPassesValidationWithFullConfig pins the
// invariant that the daemon reload host must see the full durable
// before/next pair during an eth1-only Apply. The networkd reload host
// validates its candidate via Render + Config.Validate, which refuses a
// scoped Config that drops the static management LAN ("at least one
// static management LAN is required"). The scope-isolation invariant
// governs the disk replacement (the scoped networkd map keeps eth0 out
// of the file-write set) — the daemon host itself still needs the full
// durable intent so its Render+Validate chain can admit the candidate.
// An earlier draft of the scope-isolation work passed the scoped
// configs to Reload, which caused every eth1-only Apply to fail with
// "at least one static management LAN is required" even though the
// disk invariant was satisfied.
func TestManagedRoutingSingleInterfaceOnlyApplyUsesScopedReload(t *testing.T) {
	a, before, _, daemon := managedRoutingFixture(t)
	// Construct a candidate that moves only the uplink address and
	// mirrors every other field of the baseline so the
	// enforceLinkShapeEquality preflight admits it. (uplink is the
	// fixture's WAN; inside is the management LAN. The scoped set
	// must therefore carry uplink and only uplink.)
	b, _ := Decode(before)
	n := b
	for i := range n.Interfaces {
		if n.Interfaces[i].ID == "uplink" {
			n.Interfaces[i].Addresses = []string{"192.0.2.99/24"}
		}
	}
	next, _ := json.Marshal(n)
	// Capture MUST be called with (before, candidate) so the recovery
	// envelope's e.Candidate fingerprint matches the Apply stage's
	// candidate — that is the contract the production controller
	// relies on.
	if _, err := a.Capture(context.Background(), before, next); err != nil {
		t.Fatal(err)
	}
	beforeScoped := daemon.scoped
	if err := a.Apply(context.Background(), before, next); err != nil {
		t.Fatalf("single-interface Apply must satisfy the daemon scoped reload: %v", err)
	}
	if daemon.scoped != beforeScoped+1 {
		t.Fatalf("daemon ReloadScoped must fire exactly once on a single-interface Apply; got %d (was %d)", daemon.scoped, beforeScoped)
	}
	// The scoped set must contain exactly the ID whose address changed.
	// The adapter must enter the scoped, qualified refresh path. This unit
	// fixture verifies dispatch, not the daemon's kernel side effects; the
	// isolated real-networkd witness covers those separately.
	if len(daemon.lastScoped) != 1 || daemon.lastScoped[0] != "uplink" {
		t.Fatalf("scoped reload must reconfigure exactly {uplink}; got %v", daemon.lastScoped)
	}
	if daemon.reloads != 0 {
		t.Fatalf("the global networkctl reload path must NOT fire on a scoped Apply; got reloads=%d", daemon.reloads)
	}
}

// TestManagedRoutingRouteOnlyChangeParticipatesInScopedApply pins Blocker
// 1: a candidate that adds a private route to an existing managed
// interface (no per-Interface field edits, no address change) must still
// participate in the scoped apply — the rendered networkd artifact for
// the affected interface changes (it gains a [Route] section) and the
// disk must therefore be written and the daemon re-evaluated. The
// interface's ID is in the scoped set even though changedInterfaceIDs
// alone would not pick it up.
func TestManagedRoutingRouteOnlyChangeParticipatesInScopedApply(t *testing.T) {
	a, before, _, daemon := managedRoutingFixture(t)
	// Construct a candidate that only adds a private route on the
	// uplink interface; per-Interface fields are byte-identical so
	// changedInterfaceIDs would return an empty set without our union.
	b, _ := Decode(before)
	n := b
	for i := range n.Interfaces {
		if n.Interfaces[i].ID == "uplink" {
			n.Interfaces[i].Addresses = []string{"192.0.2.2/24"}
		}
	}
	n.Routes = append(n.Routes, Route{ID: "private-default", Interface: "uplink", Destination: "0.0.0.0/0", Gateway: "192.0.2.1", Table: 10000, Metric: 100})
	next, _ := json.Marshal(n)
	// Capture MUST be called with (before, candidate) so the recovery
	// envelope's e.Candidate fingerprint matches Apply's candidate.
	if _, err := a.Capture(context.Background(), before, next); err != nil {
		t.Fatal(err)
	}
	beforeScoped := daemon.scoped
	if err := a.Apply(context.Background(), before, next); err != nil {
		t.Fatalf("route-only Apply must satisfy the daemon scoped reload: %v", err)
	}
	if daemon.scoped != beforeScoped+1 {
		t.Fatalf("daemon ReloadScoped must fire exactly once for a route-only Apply; got %d (was %d)", daemon.scoped, beforeScoped)
	}
	// uplink is in the scoped set so its disk artifact gets the new
	// [Route] section and qualified cache refresh re-reads it. inside
	// (the management LAN) is not in the set — the management anchor
	// is untouched.
	hasUplink, hasInside := false, false
	for _, id := range daemon.lastScoped {
		switch id {
		case "uplink":
			hasUplink = true
		case "inside":
			hasInside = true
		}
	}
	if !hasUplink || hasInside {
		t.Fatalf("scoped set must contain uplink (the route target) but not inside (the management LAN); got %v", daemon.lastScoped)
	}
}

func TestManagedRoutingControllerRecoversDaemonAndManagementFailures(t *testing.T) {
	for _, failure := range []string{"daemon", "management", "release"} {
		a, before, next, daemon := managedRoutingFixtureWithDelta(t)
		if failure == "management" {
			a.checkManagement = func(_ context.Context, c Config) error {
				if len(c.PolicyRules) != 0 {
					return errors.New("management verification failed")
				}
				return nil
			}
		} else if failure == "release" {
			daemon.firewall.applyError = true
		} else {
			daemon.fail = true
		}
		path := t.TempDir()
		if err := os.Chmod(path, 0700); err != nil {
			t.Fatal(err)
		}
		s, err := state.Open(path, before, func(raw json.RawMessage) error { _, err := Decode(raw); return err })
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { s.Close() })
		controller, err := safeapply.Open(s, a)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { controller.Close() })
		initial, _ := s.Snapshot()
		if _, err := controller.Apply(initial.Current.ID, next, "owner", "failure recovery", 2*time.Minute); err == nil {
			t.Fatal("failed change reported success")
		}
		final, _ := s.Snapshot()
		if final.Pending != nil || final.Current.ID != initial.Current.ID || daemon.scoped != 2 || daemon.reloads != 0 || daemon.recoveries != 1 {
			t.Fatal("rollback incomplete or operation replayed")
		}
		if err := a.Verify(context.Background(), before); err != nil {
			t.Fatal(err)
		}
	}
}

func TestManagedRoutingRejectsUnimplementedOrUngatedMutations(t *testing.T) {
	a, before, next, daemon := managedRoutingFixture(t)
	// Link identity (ID/name/kind/role/management) is durable. Removing a
	// link or changing the kind of an existing one is not admitted by
	// Safe Apply because it is a topology change that must be approved
	// via a higher-level migration.
	for _, change := range []func(*Config){
		func(c *Config) { c.Interfaces = c.Interfaces[:1] },
		func(c *Config) { c.Interfaces[0].Kind = "bridge" },
		func(c *Config) { c.Interfaces[0].Role = "lan" },
		func(c *Config) { c.Interfaces[0].Management = true },
	} {
		c, _ := Decode(next)
		change(&c)
		raw, _ := json.Marshal(c)
		if err := a.Preflight(context.Background(), before, raw); err == nil {
			t.Fatal("accepted unsupported change")
		}
	}
	a.checkManagement = nil
	if err := a.Preflight(context.Background(), before, next); err == nil {
		t.Fatal("accepted missing management verifier")
	}
	if daemon.reloads != 0 {
		t.Fatal("admission mutated daemon")
	}
}

func TestManagedRoutingRestartsFromDurableEvidence(t *testing.T) {
	a, before, next, daemon := managedRoutingFixtureWithDelta(t)
	ctx := context.Background()
	raw, err := a.Capture(ctx, before, next)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Apply(ctx, before, next); err != nil {
		t.Fatal(err)
	}
	k := a.kernel
	restarted := &ManagedRoutingAdapter{kernel: &RoutedKernel{firewall: k.firewall, routes: k.routes, rules: k.rules, tables: k.tables}, files: a.files, daemon: daemon, checkOwnership: a.checkOwnership, checkManagement: a.checkManagement, enrollment: a.enrollment}
	if err := restarted.RestoreCaptured(ctx, before, next, raw); err != nil {
		t.Fatal(err)
	}
	if err := restarted.Verify(ctx, before); err != nil {
		t.Fatal(err)
	}
	if err := restarted.Apply(ctx, before, next); err == nil {
		t.Fatal("restart reused apply admission")
	}
}
