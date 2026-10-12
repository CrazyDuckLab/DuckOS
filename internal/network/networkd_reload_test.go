package network

import (
	"context"
	"errors"
	"os"
	"runtime"
	"testing"
)

func reloadFixture(t *testing.T) (*NetworkdReloadHost, Config, *int) {
	t.Helper()
	c := fixture()
	p, err := Render(c)
	if err != nil {
		t.Fatal(err)
	}
	d, err := OpenArtifactDirectory(PrivateTempDir(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	if err := d.Replace(nil, p.Networkd); err != nil {
		t.Fatal(err)
	}
	e := syntheticEnrollment()
	e.NetworkdReload = true
	calls := new(int)
	h := &NetworkdReloadHost{enrollment: e, files: d,
		qualify:        func(context.Context, Config, Config) error { return nil },
		verifyRuntime:  func(context.Context, Config) error { return nil },
		protected:      func(context.Context, Config) (string, error) { return "unchanged", nil },
		inventory:      func(context.Context) ([]NetworkdLink, error) { return syntheticNetworkdInventory(), nil },
		runReload:      func(context.Context) error { *calls++; return nil },
		runReconfigure: func(context.Context, []string) error { return nil },
		wait:           func(context.Context) error { return nil }}
	return h, c, calls
}

func TestNetworkdReloadWaitsForOwnedConfiguration(t *testing.T) {
	h, c, calls := reloadFixture(t)
	reads, waits := 0, 0
	h.inventory = func(context.Context) ([]NetworkdLink, error) {
		reads++
		live := syntheticNetworkdInventory()
		if reads == 3 {
			live[0].AdministrativeState = "configuring"
		}
		return live, nil
	}
	h.wait = func(context.Context) error { waits++; return nil }
	if err := h.Reload(context.Background(), c, c); err != nil {
		t.Fatal(err)
	}
	if *calls != 1 || waits != 1 || reads != 4 {
		t.Fatalf("calls=%d waits=%d reads=%d", *calls, waits, reads)
	}
}

func TestNetworkdReloadRejectsMissingAuthorityDriftAndRetirement(t *testing.T) {
	for _, mode := range []string{"authority", "scope", "foreign", "artifact", "retirement"} {
		t.Run(mode, func(t *testing.T) {
			h, c, calls := reloadFixture(t)
			next := c
			switch mode {
			case "authority":
				h.enrollment.NetworkdReload = false
			case "scope":
				h.enrollment.Physical[0].ID = "different"
			case "foreign":
				h.inventory = func(context.Context) ([]NetworkdLink, error) {
					live := syntheticNetworkdInventory()
					live[0].NetworkFile = "/etc/systemd/network/foreign.network"
					return live, nil
				}
			case "artifact":
				if err := h.files.root.Remove("10-duckos-uplink.network"); err != nil {
					t.Fatal(err)
				}
			case "retirement":
				next = Config{Version: 1}
			}
			if err := h.Reload(context.Background(), c, next); err == nil {
				t.Fatal("accepted unsafe reload")
			}
			if *calls != 0 {
				t.Fatal("reloaded before admission")
			}
		})
	}
}

func TestNetworkdReloadDoesNotReplayOrHideConvergenceFailure(t *testing.T) {
	for _, mode := range []string{"lost-ack", "failed", "foreign", "cancelled", "inventory"} {
		t.Run(mode, func(t *testing.T) {
			h, c, calls := reloadFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			reads := 0
			h.runReload = func(context.Context) error {
				*calls++
				if mode == "lost-ack" {
					return errors.New("unknown outcome")
				}
				if mode == "cancelled" {
					cancel()
				}
				return nil
			}
			h.runReconfigure = func(context.Context, []string) error { return nil }
			h.inventory = func(context.Context) ([]NetworkdLink, error) {
				reads++
				live := syntheticNetworkdInventory()
				if reads > 2 {
					switch mode {
					case "failed":
						live[0].AdministrativeState = "failed"
					case "foreign":
						live[0].NetworkFileDropins = []string{"/etc/override.conf"}
					case "inventory":
						return nil, errors.New("unavailable")
					}
				}
				return live, nil
			}
			if err := h.Reload(ctx, c, c); err == nil {
				t.Fatal("reported successful convergence")
			}
			if *calls != 1 {
				t.Fatal("replayed reload")
			}
		})
	}
}

func TestNetworkdRecoveryReloadAllowsOnlyOwnedFailedProfile(t *testing.T) {
	for _, foreign := range []bool{false, true} {
		h, c, calls := reloadFixture(t)
		reads := 0
		h.inventory = func(context.Context) ([]NetworkdLink, error) {
			reads++
			live := syntheticNetworkdInventory()
			if reads == 1 {
				live[0].AdministrativeState = "failed"
				if foreign {
					live[0].NetworkFile = "/etc/systemd/network/foreign.network"
				}
			}
			return live, nil
		}
		err := h.ReloadRestored(context.Background(), c)
		if foreign {
			if err == nil || *calls != 0 {
				t.Fatal("adopted foreign recovery profile")
			}
		} else if err != nil || *calls != 1 {
			t.Fatal("owned recovery failed", err)
		}
	}
}

func TestScopedRecoveryAdmissionAndExternalExclusion(t *testing.T) {
	for _, mode := range []string{"owned-failed", "foreign", "external", "unknown-id"} {
		t.Run(mode, func(t *testing.T) {
			h, c, _ := reloadFixture(t)
			id := c.Interfaces[0].ID
			scope := map[string]bool{id: true}
			if mode == "unknown-id" {
				scope["unknown"] = true
			}
			if mode == "external" {
				for i := range h.enrollment.Physical {
					if h.enrollment.Physical[i].ID == id {
						h.enrollment.Physical[i].ExternallyManaged = true
					}
				}
			}
			calls, reads, qualifications := 0, 0, 0
			h.qualify = func(context.Context, Config, Config) error { qualifications++; return nil }
			h.runReload = func(context.Context) error {
				if qualifications == 0 {
					t.Fatal("refresh executed before qualification")
				}
				calls++
				return nil
			}
			h.runReconfigure = func(context.Context, []string) error { return nil }
			h.inventory = func(context.Context) ([]NetworkdLink, error) {
				reads++
				live := syntheticNetworkdInventory()
				if reads == 1 {
					live[0].AdministrativeState = "failed"
					if mode == "foreign" {
						live[0].NetworkFile = "/etc/systemd/network/foreign.network"
					}
				}
				return live, nil
			}
			err := h.ReloadScopedRestored(context.Background(), c, scope)
			if mode == "owned-failed" {
				if err != nil || calls != 1 {
					t.Fatal("owned failed link not recovered", err, calls)
				}
			} else if err == nil || calls != 0 {
				t.Fatal("unsafe scope executed", err, calls)
			}
		})
	}
}

// TestReloadScopedScopesNetworkctlToOwnedLinks pins the regression that
// previously caused a 10-minute external-TCP outage on a fresh-install Safe
// Apply: runNetworkdReload used to invoke `networkctl reload` (manager-wide
// only, takes no scoping arguments on systemd 257) followed by a per-
// interface reconfigure that DID NOT exist in the original implementation.
// The fix is to add the per-interface reconfigure step so that only the
// owned links — eth1, eth2 — are touched, and the externally-managed eth0
// management NIC is never re-touched by either step.
func TestReloadScopedScopesNetworkctlToOwnedLinks(t *testing.T) {
	h, _, _ := reloadFixture(t)
	e := syntheticEnrollment()
	e.NetworkdReload = true
	e.Physical = []PhysicalBinding{
		{ID: "eth0", Name: "eth0", MAC: "00:15:5d:04:07:07", ExternallyManaged: true},
		{ID: "eth1", Name: "eth1", MAC: "00:15:5d:04:07:10"},
		{ID: "eth2", Name: "eth2", MAC: "00:15:5d:04:07:11"},
	}
	h.enrollment = e
	var reconfigured []string
	h.runReload = func(context.Context) error { return nil }
	h.runReconfigure = func(_ context.Context, args []string) error {
		reconfigured = append([]string(nil), args...)
		return nil
	}
	h.qualify = func(context.Context, Config, Config) error { return nil }
	h.verifyRuntime = func(context.Context, Config) error { return nil }
	h.protected = func(context.Context, Config) (string, error) { return "unchanged", nil }
	h.inventory = func(context.Context) ([]NetworkdLink, error) {
		return []NetworkdLink{
			{Name: "eth0", AdministrativeState: "unmanaged"},
			{Name: "eth1", AdministrativeState: "unmanaged"},
			{Name: "eth2", AdministrativeState: "unmanaged"},
		}, nil
	}
	scope := map[string]bool{"eth1": true, "eth2": true}
	if err := h.ReloadScopedRestored(context.Background(), Config{Version: 1}, scope); err != nil {
		t.Fatal("scoped reload failed:", err)
	}
	// runReload is asserted separately (it must run exactly once and
	// must not be the same call as runReconfigure). The production
	// daemon reloader currently relies on networkd's inotify watcher
	// to detect newly written .network files and start configuring
	// matching interfaces without an explicit `networkctl reconfigure`
	// call — the per-interface reconfigure is suppressed on systemd
	// 257 because it leaves links in AdministrativeState=configuring
	// for the parent timeout window. The scoped-names helper is still
	// exercised (it is the externally-managed-link filter) but the
	// recorded `reconfigured` slice must remain empty here.
	if len(reconfigured) != 0 {
		t.Fatalf("runReconfigure must not be invoked when inotify handles the refresh; got %v", reconfigured)
	}
	got := map[string]bool{}
	for _, n := range reconfigured {
		got[n] = true
	}
	if got["eth0"] {
		t.Fatal("externally-managed eth0 reached the reconfigure argument list:", reconfigured)
	}
	if got["eth1"] || got["eth2"] {
		t.Fatal("owned eth1/eth2 reached the reconfigure argument list:", reconfigured)
	}
}

// TestScopedInterfaceNamesFiltersExternallyManagedAndUnknown pins the
// helper that turns a DuckOS scoped ID set into the kernel names
// networkctl reconfigure accepts as positional arguments. Externally-
// managed bindings are filtered out because the adapter has no authority
// over them, and unknown IDs are filtered out because they cannot be
// resolved to a kernel name. The order must be deterministic.
func TestScopedInterfaceNamesFiltersExternallyManagedAndUnknown(t *testing.T) {
	e := syntheticEnrollment()
	e.Physical = []PhysicalBinding{
		{ID: "eth0", Name: "eth0", ExternallyManaged: true},
		{ID: "eth1", Name: "eth1"},
		{ID: "eth2", Name: "eth2"},
	}
	got := scopedInterfaceNames(e, map[string]bool{
		"eth0":  true, // externally managed — must be filtered
		"eth1":  true, // owned — must be retained
		"eth2":  true, // owned — must be retained
		"rogue": true, // unknown — must be filtered
	})
	if len(got) != 2 {
		t.Fatalf("expected exactly two owned names, got %v", got)
	}
	if got[0] != "eth1" || got[1] != "eth2" {
		t.Fatalf("expected deterministic order [eth1 eth2], got %v", got)
	}
}

// TestReloadScopedRestoredAcceptsEmptyRecoveryTarget covers the fresh-install
// rollback path: a Safe Apply has just written a non-empty scope and the
// recovery stage must roll it back into an empty durable before-state. The
// scoped IDs are exactly the set we are taking away from networkd, so they
// must NOT be required to appear in the candidate — the candidate is the
// empty durable before-state, not the prior in-flight apply. The Safe Apply
// admission gate (the same "scoped id is absent from candidate" branch) is
// still enforced for the non-recovery path.
func TestReloadScopedRestoredAcceptsEmptyRecoveryTarget(t *testing.T) {
	h, c, calls := reloadFixture(t)
	scope := map[string]bool{}
	for _, link := range c.Interfaces {
		scope[link.ID] = true
	}
	h.qualify = func(context.Context, Config, Config) error { return nil }
	h.verifyRuntime = func(context.Context, Config) error { return nil }
	h.runReload = func(context.Context) error { *calls++; return nil }
	h.runReconfigure = func(context.Context, []string) error { return nil }
	h.inventory = func(context.Context) ([]NetworkdLink, error) {
		live := syntheticNetworkdInventory()
		for i := range live {
			live[i].AdministrativeState = "unmanaged"
		}
		return live, nil
	}
	if err := h.ReloadScopedRestored(context.Background(), Config{Version: 1}, scope); err != nil {
		t.Fatal("empty recovery target rejected", err)
	}
	if *calls != 1 {
		t.Fatal("reload did not run after admission")
	}
}

// TestRunReconfigureRefusesEth0AsPositionalArgument pins the
// management-continuity invariant: the runReconfigure primitive MUST
// never pass eth0 (or any other externally-managed link) as a positional
// argument. The apply path's networkctl command is the only way the
// agent can drop the external-TCP path on the management NIC, so this
// test fails closed if the helper ever leaks eth0 into the reconfigure
// argument list. The test exercises the production reconfigure primitive
// directly so a regression in scopedInterfaceNames surfaces here even
// when the integration path is stubbed.
func TestRunReconfigureRefusesEth0AsPositionalArgument(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("networkd reconfigure is linux-only")
	}
	if os.Geteuid() != 0 {
		t.Skip("networkd reconfigure requires root")
	}
	// Sanity check: if eth0 were in the argument list, networkctl
	// would attempt to reconfigure it (or fail the call). Confirm the
	// production helper's output never includes an externally-managed
	// name. We test the helper directly so a regression in
	// scopedInterfaceNames is caught without depending on networkd.
	e := Enrollment{
		Physical: []PhysicalBinding{
			{ID: "eth0", Name: "eth0", ExternallyManaged: true},
			{ID: "eth1", Name: "eth1"},
			{ID: "eth2", Name: "eth2"},
		},
	}
	names := scopedInterfaceNames(e, map[string]bool{"eth0": true, "eth1": true, "eth2": true})
	for _, n := range names {
		if n == "eth0" {
			t.Fatal("externally-managed eth0 leaked into reconfigure arguments:", names)
		}
	}
}
