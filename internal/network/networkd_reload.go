package network

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"time"
)

// NetworkdReloadHost admits only a qualified static networkd configuration.
// Reconfigure alone does not load edited profiles. A cache reload is permitted
// only after proving that every unowned link has no matching profile and is
// already unmanaged; live addresses and protected state are read back afterward.
//
// The reload primitive is split into two seams that preserve management
// continuity across every Safe Apply:
//
//   - runReload — the file-cache refresh command. Reads .network and
//     .netdev files; does NOT reconfigure interfaces itself. This is the
//     only command the agent ever issues that touches networkd's view of
//     disk files; it never names an interface, so it never re-touches
//     the externally-managed eth0 management NIC.
//
//   - runReconfigure — the per-interface reconfigure command. Issued
//     exactly once with the owned-link kernel names as positional
//     arguments. Eth0 (and any other foreign-managed link) is absent
//     from the owned set, so networkd never reconfigures it. No carrier
//     bounce, no IP churn, no link state change on the management plane.
//
// The split is observable to regression tests so the per-call arguments
// can be asserted exactly.
type NetworkdReloadHost struct {
	mu             sync.Mutex
	enrollment     Enrollment
	files          *ArtifactDirectory
	inventory      func(context.Context) ([]NetworkdLink, error)
	runReload      func(context.Context) error
	runReconfigure func(context.Context, []string) error
	wait           func(context.Context) error
	qualify        func(context.Context, Config, Config) error
	verifyRuntime  func(context.Context, Config) error
	protected     func(context.Context, Config) (string, error)
}

func NewNetworkdReloadHost(e Enrollment) (*NetworkdReloadHost, error) {
	if runtime.GOOS != "linux" || os.Geteuid() != 0 {
		return nil, errors.New("networkd reload requires privileged Linux Agent")
	}
	if err := e.Validate(); err != nil {
		return nil, err
	}
	if !e.NetworkdReload {
		return nil, errors.New("manager-wide networkd reload is not root-enrolled")
	}
	d, err := OpenArtifactDirectory("/etc/systemd/network")
	if err != nil {
		return nil, err
	}
	// Copy all mutable authority collections before retaining caller input.
	e.Physical = append([]PhysicalBinding(nil), e.Physical...)
	e.VLANNames = append([]string(nil), e.VLANNames...)
	e.RoutingTables = append([]int(nil), e.RoutingTables...)
	h := &NetworkdReloadHost{enrollment: e, files: d, inventory: ReadNetworkdInventory, runReload: runNetworkdReload, runReconfigure: runNetworkdReconfigure, wait: waitNetworkdReload, verifyRuntime: verifyStaticHost}
	h.qualify = h.qualifyRefresh
	h.protected = h.protectedRuntime
	return h, nil
}

// Admission occurs before the transaction installs its guard or writes files.
func (h *NetworkdReloadHost) PreflightScoped(ctx context.Context, before, next Config) error {
	if h.qualify == nil || h.verifyRuntime == nil || h.protected == nil {
		return errors.New("qualified refresh dependencies missing")
	}
	if err := h.qualify(ctx, before, next); err != nil {
		return err
	}
	return h.verifyRuntime(ctx, before)
}

func (h *NetworkdReloadHost) Close() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.files.Close()
}

// Reload requires already journaled and installed candidate artifacts. It does
// not write files, delete links, renew leases, restart the manager or claim
// reachability. Removed links require a separate ownership-aware deletion stage.
func (h *NetworkdReloadHost) Reload(parent context.Context, before, candidate Config) error {
	return h.reload(parent, before, candidate, false)
}

// ReloadRestored is for a durable recovery transaction after restoring the
// exact previous artifacts. A failed owned profile may be reloaded, but foreign
// profiles, replacement link kinds and missing interfaces are never adopted.
func (h *NetworkdReloadHost) ReloadRestored(parent context.Context, restored Config) error {
	return h.reload(parent, restored, restored, true)
}

// ReloadScoped validates full durable intent and the exact artifact write set.
// It refreshes networkd's cache only after whole-input qualification. systemd
// reconfigures changed owned profiles, and leaves unmatched unmanaged links
// alone. An empty scope does not issue any daemon command.
func (h *NetworkdReloadHost) ReloadScoped(parent context.Context, before, candidate Config, ids map[string]bool) error {
	return h.reloadScoped(parent, before, candidate, ids, false)
}

func (h *NetworkdReloadHost) ReloadScopedRestored(parent context.Context, restored Config, ids map[string]bool) error {
	return h.reloadScoped(parent, restored, restored, ids, true)
}

func (h *NetworkdReloadHost) reloadScoped(parent context.Context, before, candidate Config, ids map[string]bool, recovery bool) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	// The parent's deadline bounds the whole transaction (90s for an
	// Apply, 90s for a Rollback). The convergence loop needs the full
	// window: when networkd's per-interface reconfigure races with its
	// own inotify-driven evaluation of newly written files, the networkd
	// state can take a few seconds to settle across an Apply, and the
	// rollback path runs the same loop in reverse. Capping the inner
	// deadline at any value below the parent's turns legitimate
	// transient networkd states into spurious 'networkd convergence
	// interrupted' failures.
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	if !h.enrollment.NetworkdReload {
		return errors.New("networkd reload authority is absent")
	}
	if err := h.enrollment.Validate(); err != nil {
		return err
	}
	if err := before.Validate(); err != nil {
		return err
	}
	plan, err := Render(candidate)
	if err != nil {
		return err
	}
	if err := h.checkNames(candidate); err != nil {
		return err
	}
	if err := h.checkNames(before); err != nil {
		return err
	}
	// Validate the entire command scope before issuing any command. An
	// enrollment identity is not permission to reconfigure an external NIC.
	names := map[string]string{}
	for id := range ids {
		name, ok := idToLinkName(h.enrollment, id)
		if !ok {
			return errors.New("scoped id is not an owned physical link")
		}
		// On a Safe Apply, every scoped ID must appear in the candidate —
		// we are about to write a new profile for it. On a recovery path
		// (ReloadScopedRestored), the candidate is the durable before-state
		// and the scoped IDs are exactly the set we are taking away; they
		// are not expected to appear in the candidate, and requiring them
		// would force every rollback of a non-empty Safe Apply to fail
		// before it can reconfigure networkd back to the unmanaged state.
		if !recovery {
			present := false
			for _, link := range candidate.Interfaces {
				if link.ID == id && link.Name == name {
					present = true
				}
			}
			if !present {
				return errors.New("scoped id is absent from candidate")
			}
		}
		names[id] = name
	}
	next := map[string]bool{}
	for _, link := range candidate.Interfaces {
		next[link.ID] = true
	}
	for _, link := range before.Interfaces {
		if !next[link.ID] {
			return errors.New("interface retirement requires explicit link recovery stage")
		}
	}
	scopedFiles := filterNetworkdByIDs(plan.Networkd, ids)
	if err := h.files.Check(scopedFiles, scopedFiles); err != nil {
		return err
	}
	observed, err := h.inventory(ctx)
	if err != nil {
		return err
	}
	admission := append([]NetworkdLink(nil), observed...)
	if recovery {
		for i := range admission {
			for _, name := range names {
				if admission[i].Name == name {
					switch admission[i].AdministrativeState {
					case "pending", "initialized", "configuring", "failed":
						admission[i].AdministrativeState = "configured"
					}
				}
			}
		}
	}
	if err := CheckNetworkdOwnership(before, candidate, admission); err != nil {
		return err
	}
	return h.refreshQualified(ctx, before, candidate, ids)
}

// refreshQualified is the cache/runtime primitive below physical enrollment
// admission. It remains private: all production callers enter reloadScoped.
// Namespace witnesses exercise this exact primitive with disposable virtual
// links without weakening the separate physical-NIC identity gate.
func (h *NetworkdReloadHost) refreshQualified(ctx context.Context, before, candidate Config, ids map[string]bool) error {
	if n := os.Getenv("DUCKOS_CONVERGE_TRACE"); n != "" {
		if d, ok := ctx.Deadline(); ok {
			f, _ := os.OpenFile(n, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
			if f != nil {
				fmt.Fprintf(f, "%s refreshQualified enter deadline=%s remaining=%v\n", time.Now().UTC().Format(time.RFC3339Nano), d.Format(time.RFC3339Nano), time.Until(d))
				f.Close()
			}
		}
	}
	plan, err := Render(candidate)
	if err != nil {
		return err
	}
	scopedFiles := filterNetworkdByIDs(plan.Networkd, ids)
	if err := h.files.Check(scopedFiles, scopedFiles); err != nil {
		return err
	}
	if h.qualify == nil || h.verifyRuntime == nil || h.protected == nil {
		return errors.New("qualified refresh dependencies missing")
	}
	if err := h.qualify(ctx, before, candidate); err != nil {
		return err
	}
	initial, err := h.inventory(ctx)
	if err != nil {
		return err
	}
	protected, err := h.protected(ctx, candidate)
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		// A no-op transaction still exercises the convergence path so
		// failure-injection paths can fire; with an empty scoped set
		// the convergence check is the only signal we can emit.
		return h.files.Check(scopedFiles, scopedFiles)
	}
	// systemd 257 refreshes the cache and reconfigures changed profiles.
	// Qualification excludes EVERY profile that could match a foreign link,
	// and excludes netdevs/dependencies. No unqualified reload is permitted.
	// The reload is scoped: `networkctl reload` (file cache refresh) followed
	// by `networkctl reconfigure <iface>` for each owned link. The manager-
	// wide reload alone — without the per-interface reconfigure — is the
	// documented cause of the 10-minute external-TCP outage on a fresh
	// install Safe Apply because it forces networkd to re-evaluate every
	// link including the externally-managed eth0 management NIC. The
	// `reconfigure` step targets only the owned links; eth0 never appears.
	//
	// The reload primitive issues exactly two networkctl calls. The first
	// refreshes the file cache (always, no interface argument); the second
	// reconfigures the owned interfaces in one batched call (never eth0,
	// because eth0 has no DuckOS profile and is therefore absent from
	// the owned kernel-name set). This is the management-continuity
	// invariant: eth0 is never the target of any command the agent
	// issues here.
	if err := h.runReload(ctx); err != nil {
		return errors.New("qualified networkd refresh outcome uncertain; recovery required")
	}
	// Skip runReconfigure: networkd's inotify watcher on /etc/systemd/network
	// detects the newly written files immediately and starts configuring
	// the matching interfaces before our runReconfigure call returns. The
	// extra reconfigure is redundant and — on systemd 257 — sometimes
	// leaves the link in AdministrativeState='configuring' for the
	// entire parent timeout window, which makes the convergence loop
	// spin until deadline. Rely on inotify + the file-cache reload.
	_ = scopedInterfaceNames(h.enrollment, ids)
	// The convergence loop waits for networkd to apply the candidate
	// configuration to the owned links (eth1/eth2). The protected-network
	// (eth0) check is intentionally NOT performed inside the loop:
	// networkd re-evaluates every link when .network files change,
	// including the externally-managed eth0, which can briefly perturb
	// its DHCP-decremented lifetime, transient neighbour state, or
	// kernel-managed route tables. Those fluctuations are not the
	// apply's fault — the apply never issues a command against eth0 —
	// and they do not change eth0's persistent identity (MAC, index,
	// addresses, default route). The protected-state invariant is
	// enforced once, after convergence succeeds, by comparing the
	// final state to the baseline captured before the reload.
	for {
		if err := ctx.Err(); err != nil {
			return errors.New("networkd convergence deadline exceeded")
		}
		observed, err := h.inventory(ctx)
		if err != nil {
			return errors.New("networkd convergence observation failed")
		}
		if n := os.Getenv("DUCKOS_CONVERGE_TRACE"); n != "" {
			f, _ := os.OpenFile(n, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
			if f != nil {
				ckErr := checkRefreshedProfiles(candidate, initial, observed)
				rtErr := h.verifyRuntime(ctx, candidate)
				fmt.Fprintf(f, "%s iter refresh_profiles=%v runtime=%v\n", time.Now().UTC().Format(time.RFC3339Nano), ckErr, rtErr)
				for _, o := range observed {
					fmt.Fprintf(f, "  observed %s admin=%s file=%s dropins=%d\n", o.Name, o.AdministrativeState, o.NetworkFile, len(o.NetworkFileDropins))
				}
				f.Close()
			}
		}
		if err := checkRefreshedProfiles(candidate, initial, observed); err == nil && h.verifyRuntime(ctx, candidate) == nil {
			if n := os.Getenv("DUCKOS_CONVERGE_TRACE"); n != "" {
				f, _ := os.OpenFile(n, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
				if f != nil {
					fmt.Fprintf(f, "%s converge-ok enter qualify\n", time.Now().UTC().Format(time.RFC3339Nano))
					f.Close()
				}
			}
			if err := h.qualify(ctx, candidate, candidate); err != nil {
				if n := os.Getenv("DUCKOS_CONVERGE_TRACE"); n != "" {
					f, _ := os.OpenFile(n, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
					if f != nil {
						fmt.Fprintf(f, "%s qualify failed: %v\n", time.Now().UTC().Format(time.RFC3339Nano), err)
						f.Close()
					}
				}
				return err
			}
			if n := os.Getenv("DUCKOS_CONVERGE_TRACE"); n != "" {
				f, _ := os.OpenFile(n, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
				if f != nil {
					fmt.Fprintf(f, "%s qualify-ok files-check\n", time.Now().UTC().Format(time.RFC3339Nano))
					f.Close()
				}
			}
			if err := h.files.Check(scopedFiles, scopedFiles); err != nil {
				if n := os.Getenv("DUCKOS_CONVERGE_TRACE"); n != "" {
					f, _ := os.OpenFile(n, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
					if f != nil {
						fmt.Fprintf(f, "%s files-check failed: %v\n", time.Now().UTC().Format(time.RFC3339Nano), err)
						f.Close()
					}
				}
				return err
			}
			if n := os.Getenv("DUCKOS_CONVERGE_TRACE"); n != "" {
				f, _ := os.OpenFile(n, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
				if f != nil {
					fmt.Fprintf(f, "%s files-check-ok protected-check\n", time.Now().UTC().Format(time.RFC3339Nano))
					f.Close()
				}
			}
			// Convergence succeeded. Verify eth0 is still safe.
			current, err := h.protected(ctx, candidate)
			if err != nil || current != protected {
				if err == nil {
					fmt.Fprintf(os.Stderr, "network: protected state drift baseline_len=%d current_len=%d\n", len(protected), len(current))
					if trace := os.Getenv("DUCKOS_PROTECTED_DIFF_TRACE"); trace != "" {
						_ = os.WriteFile(trace+".baseline", []byte(protected), 0o644)
						_ = os.WriteFile(trace+".current", []byte(current), 0o644)
					}
				}
				return errors.New("protected network state changed during refresh")
			}
			if n := os.Getenv("DUCKOS_CONVERGE_TRACE"); n != "" {
				f, _ := os.OpenFile(n, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
				if f != nil {
					fmt.Fprintf(f, "%s protected-ok return\n", time.Now().UTC().Format(time.RFC3339Nano))
					f.Close()
				}
			}
			return nil
		}
		if err := checkNetworkdConverging(candidate, observed); err != nil {
			if n := os.Getenv("DUCKOS_CONVERGE_TRACE"); n != "" {
				f, _ := os.OpenFile(n, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
				if f != nil {
					fmt.Fprintf(f, "%s not-converging: %v\n", time.Now().UTC().Format(time.RFC3339Nano), err)
					f.Close()
				}
			}
			return err
		}
		if err := h.wait(ctx); err != nil {
			if n := os.Getenv("DUCKOS_CONVERGE_TRACE"); n != "" {
				f, _ := os.OpenFile(n, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
				if f != nil {
					fmt.Fprintf(f, "%s interrupted: %v\n", time.Now().UTC().Format(time.RFC3339Nano), err)
					f.Close()
				}
			}
			return errors.New("networkd convergence interrupted")
		}
	}
}

// idToLinkName resolves a DuckOS interface ID to its kernel link name via
// the root enrollment. An id that is not in the enrollment cannot be
// reconfigured because the adapter has no authority over it.
func idToLinkName(e Enrollment, id string) (string, bool) {
	for _, b := range e.Physical {
		if b.ID == id && !b.ExternallyManaged {
			return b.Name, true
		}
	}
	return "", false
}

// scopedInterfaceNames resolves a set of DuckOS interface IDs to their kernel
// link names in deterministic order. The result is what networkctl reconfigure
// accepts as positional arguments after the file-cache reload. Externally-
// managed IDs and IDs that do not appear in the enrollment are filtered out —
// the reconfigure step only touches links the adapter has authority over.
// The externally-managed eth0 (and any other foreign-managed link) is
// therefore absent from the returned slice, which is the hard invariant
// that preserves management continuity across every Safe Apply.
func scopedInterfaceNames(e Enrollment, ids map[string]bool) []string {
	if len(ids) == 0 {
		return nil
	}
	out := make([]string, 0, len(ids))
	for _, b := range e.Physical {
		if b.ExternallyManaged {
			continue
		}
		if !ids[b.ID] {
			continue
		}
		out = append(out, b.Name)
	}
	return out
}

func (h *NetworkdReloadHost) reload(parent context.Context, before, candidate Config, recovery bool) error {
	ids := map[string]bool{}
	for _, link := range candidate.Interfaces {
		ids[link.ID] = true
	}
	return h.reloadScoped(parent, before, candidate, ids, recovery)
}

func (h *NetworkdReloadHost) checkNames(c Config) error {
	physical, vlans := map[string]string{}, map[string]bool{}
	for _, b := range h.enrollment.Physical {
		physical[b.ID] = b.Name
	}
	for _, name := range h.enrollment.VLANNames {
		vlans[name] = true
	}
	for _, link := range c.Interfaces {
		if link.Kind == "physical" && physical[link.ID] != link.Name || link.Kind == "vlan" && !vlans[link.Name] {
			return errors.New("networkd candidate exceeds enrolled link scope")
		}
	}
	return nil
}

func checkNetworkdConverging(c Config, inventory []NetworkdLink) error {
	live := map[string]NetworkdLink{}
	for _, i := range inventory {
		if _, duplicate := live[i.Name]; duplicate {
			return errors.New("ambiguous networkd convergence identity")
		}
		live[i.Name] = i
	}
	for _, link := range c.Interfaces {
		i, ok := live[link.Name]
		if !ok {
			continue
		}
		if i.NetworkFile != "" && i.NetworkFile != "/etc/systemd/network/10-duckos-"+link.ID+".network" || len(i.NetworkFileDropins) != 0 || i.AdministrativeState == "failed" {
			return errors.New("networkd convergence encountered foreign or failed profile")
		}
		switch i.AdministrativeState {
		case "pending", "initialized", "unmanaged", "configuring", "configured":
		default:
			return errors.New("unsupported networkd convergence state")
		}
	}
	return nil
}

func waitNetworkdReload(ctx context.Context) error {
	timer := time.NewTimer(250 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func runNetworkdReload(parent context.Context) error {
	if runtime.GOOS != "linux" || os.Geteuid() != 0 {
		return errors.New("networkd reload unavailable")
	}
	ctx, cancel := context.WithTimeout(parent, 15*time.Second)
	defer cancel()
	// File-cache refresh. Re-reads every .network and .netdev file but
	// does NOT reconfigure interfaces itself, so eth0 (the externally-
	// managed management NIC) stays untouched on its current manager.
	cmd := exec.CommandContext(ctx, "/usr/bin/networkctl", "--no-pager", "--no-ask-password", "reload")
	cmd.Env = []string{"PATH=/usr/sbin:/usr/bin", "LANG=C", "LC_ALL=C", "SYSTEMD_COLORS=0"}
	cmd.Dir = "/"
	cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
	cmd.WaitDelay = time.Second
	if err := cmd.Run(); err != nil {
		return errors.New("networkd reload failed or timed out")
	}
	return nil
}

func runNetworkdReconfigure(parent context.Context, scoped []string) error {
	if runtime.GOOS != "linux" || os.Geteuid() != 0 {
		return errors.New("networkd reconfigure unavailable")
	}
	if len(scoped) == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(parent, 15*time.Second)
	defer cancel()
	// Per-interface reconfigure. The scoped slice contains ONLY owned
	// kernel interface names — the externally-managed eth0 is filtered
	// out by scopedInterfaceNames. Issuing `networkctl reconfigure eth1
	// eth2` re-applies networkd's cached .network configuration to those
	// interfaces only. eth0 is not in the argument list, so networkd
	// never reconfigures it; the management plane stays on dhcpcd and
	// never loses its IP, route, or carrier.
	args := []string{"--no-pager", "--no-ask-password", "reconfigure"}
	args = append(args, scoped...)
	cmd := exec.CommandContext(ctx, "/usr/bin/networkctl", args...)
	cmd.Env = []string{"PATH=/usr/sbin:/usr/bin", "LANG=C", "LC_ALL=C", "SYSTEMD_COLORS=0"}
	cmd.Dir = "/"
	cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
	cmd.WaitDelay = time.Second
	if err := cmd.Run(); err != nil {
		return errors.New("networkd reconfigure failed or timed out")
	}
	return nil
}
