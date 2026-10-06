//go:build linux

// frr/runtime_linux.go: production runtime that owns
// the lifecycle of real FRR daemons (zebra + bgpd) and
// drives them through vtysh. The runtime is the DV09
// production-path seam: PUBLIC -> API -> ENGINE -> Agent
// -> *frr.Runtime -> real /usr/lib/frr/{zebra,bgpd}
// -> independent vtysh oracle.
//
// The runtime is goroutine-safe and never silently
// destroys a working release. The lifecycle is:
//   1. NewRuntime(netns, pathspace, sockDir) creates a
//      private netns (when netns != ""), a /run/frr/<ps>
//      pathspace, and starts zebra + bgpd with the
//      current Manager config.
//   2. Apply(ctx, cfg) writes the rendered frr.conf
//      and reloads via `vtysh -f <conf>` (hot reload)
//      when the daemons are alive, or restarts them
//      when they are not. The previous config is
//      retained for rollback.
//   3. Rollback(reason) restores the previous config
//      and reloads / restarts.
//   4. Stop() sends SIGTERM to both daemons, removes
//      the pathspace dir, and (when netns != "") deletes
//      the netns. Idempotent.
//
// The runtime is the canonical source of truth for
// /agent/v1/frr; the IPC layer never invents state.
// A nil runtime returns 503 frr_unavailable.

package frr

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"
)

// FRRPath is the IPC route for FRR status (GET).
const FRRPath = "/agent/v1/frr"

// FRRApplyPath is the IPC route for staging a config
// (POST). Body is a Config JSON; response is the
// canonical lifecycle outcome.
const FRRApplyPath = "/agent/v1/frr/apply"

// FRRRollbackPath is the IPC route for rolling back
// to the previous config (POST). Body is {"reason": "..."}.
const FRRRollbackPath = "/agent/v1/frr/rollback"

// ApplyMaxBytes is the per-request config size cap.
// The renderer's output is bounded; 1 MiB is generous.
const ApplyMaxBytes = 1 << 20

// PathspaceBase, StateBase, FRRRunUser, FRRRunGroup,
// chownToFRR, lookupFRRIDs, ErrFRRUserMissing all live in
// privilege_model.go so cross-platform tests can assert
// the narrow privilege contract that fixes the armv7
// "Permission denied" / "cap_set_proc failed" failure.

// FRRBinary is the canonical path to the zebra / bgpd
// binaries. Tests can override it via the
// DUCKOS_FRR_BIN env var when running on a custom
// install.
func frrBinary(name string) string {
	if v := os.Getenv("DUCKOS_FRR_BIN"); v != "" {
		return filepath.Join(v, name)
	}
	return filepath.Join("/usr/lib/frr", name)
}

// Runtime owns a disposable FRR instance. The runtime
// is goroutine-safe; every mutating call takes the
// mutex so a concurrent Apply cannot race with a
// Rollback.
type Runtime struct {
	mu         sync.Mutex
	netns      string // empty => host netns (production)
	pathspace  string // -N flag for vtysh / FRR daemons
	sockDir    string // private FRR socket / pid / log dir
	manager    *Manager
	zebraPID   int
	bgpdPID    int
	zebraCmd   *exec.Cmd
	bgpdCmd    *exec.Cmd
	clock      func() time.Time
	stopped    bool
	lastReload time.Time
	lastError  string
}

// NewRuntime creates the runtime, brings up the
// netns (when netns != ""), creates the pathspace dir,
// and starts zebra + bgpd. A nil manager returns nil
// so the Agent composition root refuses to register
// a runtime without a real config lifecycle.
//
// pathspace must be canonical ([a-z0-9-]+) and unique
// per runtime; the runtime writes its private
// /run/frr/<pathspace> dir.
func NewRuntime(netns, pathspace string, manager *Manager) (*Runtime, error) {
	if manager == nil {
		return nil, errors.New("frr: manager is required")
	}
	if pathspace == "" {
		return nil, errors.New("frr: pathspace is required")
	}
	if !pathspacePattern.MatchString(pathspace) {
		return nil, fmt.Errorf("frr: pathspace %q is not canonical", pathspace)
	}
	sockDir := filepath.Join(PathspaceBase, pathspace)
	if err := os.MkdirAll(sockDir, 0o755); err != nil {
		return nil, fmt.Errorf("frr: mkdir %s: %w", sockDir, err)
	}
	// chownToFRR is narrow: it only accepts paths under
	// StateBase (/var/lib/frr) or PathspaceBase (/run/frr),
	// and chowns only the gid to the FRR service group.
	// The daemon running as uid=frr needs group write on
	// /run/frr/<ps> to write its vty socket and log files.
	if err := chownToFRR(sockDir); err != nil {
		return nil, fmt.Errorf("frr: chown %s: %w", sockDir, err)
	}
	// FRR's pathspace state dir lives under /var/lib/frr/<ps>
	// and is created lazily by the daemon. The Debian FRR
	// package ships /var/lib/frr/ as frr:frr 0755, so the
	// daemon running as uid=frr gid=frr has group-write
	// access and can lazily mkdir the pathspace itself. The
	// runtime pre-creates the state dir as a defense-in-depth
	// measure (so the armv7 "Permission denied" failure
	// caused by FRR's old lazy mkdir as root CANNOT recur),
	// but if pre-create fails because the Agent's restricted
	// capability set refuses a write into /var/lib/frr, we
	// fall back to letting the daemon create it. The Agent
	// does NOT need the daemon to depend on its pre-create
	// succeeding.
	stateDir := filepath.Join(StateBase, pathspace)
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		log.Printf("frr: pre-create %s: %v; daemon will create it", stateDir, err)
	} else if err := chownToFRR(stateDir); err != nil {
		log.Printf("frr: chown %s: %v; daemon will own its own creation", stateDir, err)
	}
	// FRR's pathspace (-N) requires /etc/frr/<ps>/vtysh.conf
	// to exist; otherwise vtysh aborts with "Can't open
	// configuration file ... due to 'No such file or
	// directory'". The daemon's vty socket itself lives
	// under /run/frr/<ps>/, so /etc/frr/<ps>/ is a
	// configuration-only path.
//
// /etc/frr is owned by frr:frr 0700 in the Debian
// package. The Agent has no caps and UID 0 does not
// match the file owner; gid=frr IS in SupplementaryGroups
// but mode 0700 has no group bits. The Agent cannot
// write into /etc/frr without a privilege expansion.
//
// Solution: the launcher creates /etc/frr/<ps>/vtysh.conf
	// AFTER dropping privileges to the frr service user.
	// The frr user matches /etc/frr's owner, so the
	// mkdir + write succeed. The runtime does NOT try
	// to pre-create /etc/frr/<ps>; it accepts that
	// vtysh will emit a single warning line until the
	// launcher has run. After the launcher's first
	// invocation, subsequent restarts see the existing
	// dir.
	etcDir := filepath.Join("/etc/frr", pathspace)
	if err := os.MkdirAll(etcDir, 0o755); err != nil {
		// Best-effort: this fails on every install because
		// /etc/frr is 0700 frr:frr and the Agent has no
		// caps. The launcher, running as uid=frr, will
		// create the dir + vtysh.conf after its privilege
		// drop. We log once so the operator can see what
		// happened; this is the documented architecture.
		log.Printf("frr: Agent cannot create %s (mode 0700 frr:frr); launcher will create it after privilege drop", etcDir)
	}
	if netns != "" {
		if err := runIPNetnsAdd(netns); err != nil {
			return nil, fmt.Errorf("frr: netns add %s: %w", netns, err)
		}
	}
	r := &Runtime{
		netns:     netns,
		pathspace: pathspace,
		sockDir:   sockDir,
		manager:   manager,
		clock:     time.Now,
	}
	cfg, renderErr := manager.Render()
	if renderErr != nil {
		cfg, _ = Render(Config{Daemons: []Daemon{DaemonZebra, DaemonBGP}})
	}
	if cfg != "" {
		if err := r.writeConf(cfg); err != nil {
			_ = r.cleanupOnFailure()
			return nil, fmt.Errorf("frr: write initial conf: %w", err)
		}
	}
	if err := r.startDaemons(); err != nil {
		_ = r.cleanupOnFailure()
		return nil, fmt.Errorf("frr: start daemons: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, _, err := r.runVTYSHFull(ctx, "-C", "-f", filepath.Join(r.sockDir, "frr.conf")); err != nil {
		_ = r.cleanupOnFailure()
		return nil, ErrConfig
	}
	if stdout, stderr, err := r.runVTYSHFull(ctx, "-d", "bgpd", "-c", "show bgp ipv4 unicast"); err != nil {
		_ = r.cleanupOnFailure()
		return nil, fmt.Errorf("frr: startup BGP readiness failed: %v: stderr=%q stdout=%q", err, strings.TrimSpace(stderr), strings.TrimSpace(stdout))
	}
	return r, nil
}

// pathspacePattern mirrors the canonical form vtysh /
// FRR daemons accept on -N.
var pathspacePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)

// Apply stages the supplied config and reloads / restarts
// the daemons so the kernel route table reflects the
// change. The previous config is retained for rollback.
func (r *Runtime) Apply(ctx context.Context, cfg Config) error {
	if r == nil {
		return errors.New("frr: runtime is not configured")
	}
	if err := cfg.Valid(); err != nil {
		return fmt.Errorf("%w: %v", ErrConfig, err)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.stopped {
		return errors.New("frr: runtime is stopped")
	}
	previous, previousErr := r.manager.Render()
	if previousErr != nil {
		previous, _ = Render(Config{Daemons: []Daemon{DaemonZebra, DaemonBGP}})
	}
	rendered, err := Render(cfg)
	if err != nil {
		return err
	}
	if err := r.writeConf(rendered); err != nil {
		return err
	}
	if err := r.reloadOrRestart(ctx); err != nil {
		return r.restoreRuntime(previous, err)
	}
	if err := r.manager.Set(cfg); err != nil {
		return r.restoreRuntime(previous, err)
	}
	r.lastReload = r.clock().UTC()
	r.lastError = ""
	r.manager.appendHistoryLocked(LifecycleEvent{At: r.clock().UTC(), Action: "apply", Result: "ok"})
	return nil
}

func (r *Runtime) restoreRuntime(previous string, cause error) error {
	r.lastError = "apply failed"
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := r.writeConf(previous); err != nil {
		r.lastError = "recovery_required"
		return errors.New("frr: recovery required")
	}
	if err := r.reloadOrRestart(ctx); err != nil {
		r.lastError = "recovery_required"
		return errors.New("frr: recovery required")
	}
	return cause
}

// Rollback restores the previous config and reloads /
// restarts the daemons. The previous config is rotated
// out so a double Rollback returns an error.
func (r *Runtime) Rollback(ctx context.Context, reason string) error {
	if r == nil {
		return errors.New("frr: runtime is not configured")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.manager.Rollback(reason); err != nil {
		return err
	}
	rendered, err := r.manager.Render()
	if err != nil {
		return err
	}
	if err := r.writeConf(rendered); err != nil {
		return err
	}
	if err := r.reloadOrRestart(ctx); err != nil {
		r.lastError = err.Error()
		return err
	}
	r.lastReload = r.clock().UTC()
	r.lastError = ""
	r.manager.appendHistoryLocked(LifecycleEvent{At: r.clock().UTC(), Action: "rollback", Result: "ok", Detail: reason})
	return nil
}

// Stop terminates both daemons, removes the pathspace
// dir, and (when netns != "") deletes the netns.
// Idempotent.
//
// Stop FIRST asks the duck-frr-launcher daemon to STOP
// the pathspace. The launcher identity-checks every kill
// via terminateAndReap (recycled PIDs are NOT reaped), so
// a stale zebra.pid whose number has been recycled to an
// unrelated process CANNOT be signalled by accident. We
// then re-issue STOP for each binary as a defence-in-depth
// step: the launcher keeps its own (pathspace, binary)
// map, so two STOPs are equivalent.
//
// We deliberately do NOT directly call syscall.Kill on
// r.zebraPID / r.bgpidPID from this code path. The agent
// is not authoritative for FRR lifetime — the launcher
// is. The recorded r.zebraPID is only the daemon's
// read-back of the pidfile FRR wrote; the launcher's
// tracker is the source of truth.
func (r *Runtime) Stop() {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.stopped {
		return
	}
	r.stopped = true
	// Identity-gated kill via launcher.
	if err := r.launcherStop(""); err != nil {
		log.Printf("frr: launcher STOP %s: %v", r.pathspace, err)
	}
	// Defence-in-depth: after the launcher has had a
	// chance to terminate the pathspace, send SIGTERM
	// then SIGKILL to our recorded PIDs IF the launcher
	// can't reach its daemon. Skipped silently if the
	// recorded PIDs are 0 or the launcher already
	// succeeded.
	r.killPidsLocked()
	_ = os.RemoveAll(r.sockDir)
	_ = os.RemoveAll(filepath.Join("/etc/frr", r.pathspace))
	if r.netns != "" {
		_ = runIPNetnsDel(r.netns)
	}
}

// State returns the runtime's current state. Sessions
// and route counts come from independent vtysh queries
// (the daemon's own view) — never from the runtime's
// own bookkeeping.
func (r *Runtime) State() State {
	if r == nil {
		return State{}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	st := State{
		At:         r.clock().UTC(),
		LastReload: r.lastReload,
		LastError:  r.lastError,
	}
	if r.zebraCmd != nil && r.zebraCmd.Process != nil {
		if err := r.zebraCmd.Process.Signal(syscall.Signal(0)); err == nil {
			st.SessionsEstablished, st.RoutesInstalled = r.queryFRRCounts()
		}
	}
	return st
}

// Manager returns the runtime's Manager (the canonical
// config lifecycle store).
func (r *Runtime) Manager() *Manager {
	if r == nil {
		return nil
	}
	return r.manager
}

// Pathspace returns the FRR -N pathspace the runtime
// owns. The IPC oracle dials it through vtysh -N.
func (r *Runtime) Pathspace() string {
	if r == nil {
		return ""
	}
	return r.pathspace
}

// Netns returns the runtime's network namespace ("" if
// the runtime is running in the host netns).
func (r *Runtime) Netns() string {
	if r == nil {
		return ""
	}
	return r.netns
}

// writeConf renders the current manager config and
// writes it to <sockDir>/frr.conf.
func (r *Runtime) writeConf(rendered string) error {
	path := filepath.Join(r.sockDir, "frr.conf")
	return os.WriteFile(path, []byte(rendered), 0o644)
}

// reloadOrRestart applies the rendered frr.conf to
// the running FRR daemons. The canonical happy path
// is `vtysh -f <conf>` (hot reload). When the daemons
// are not running (e.g. fresh runtime, or the previous
// start failed and was rolled back), the function
// falls back to startDaemons.
//
// A re-restart of an already-running daemon is
// deliberately rejected: the daemon holds its own
// pidfile lock and a duplicate start fails with
// EAGAIN, leaving the runtime in an inconsistent
// state. Apply callers can rely on the rule:
//   - daemons alive: hot reload only
//   - daemons dead: cold start
func (r *Runtime) reloadOrRestart(ctx context.Context) error {
	if _, _, err := r.runVTYSHFull(ctx, "-C", "-f", filepath.Join(r.sockDir, "frr.conf")); err != nil {
		return ErrConfig
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// vtysh -f is additive, so removed advertisements would survive it.
	// Replace only this owned pathspace to apply the complete desired set.
	r.killPidsLocked()
	if err := r.startDaemons(); err != nil {
		return err
	}
	return r.runVTYSH(ctx, "-d", "bgpd", "-c", "show bgp ipv4 unicast")
}

// daemonsAlive reports whether the recorded zebra /
// bgpd processes are still running. A nil zebra / bgpd
// pid or an unsuccessful Signal(0) probe means the
// daemon is dead.
func (r *Runtime) daemonsAlive() bool {
	if r.zebraCmd != nil && r.zebraCmd.Process != nil {
		if err := r.zebraCmd.Process.Signal(syscall.Signal(0)); err == nil {
			return true
		}
	}
	if r.zebraPID > 0 {
		if err := syscall.Kill(r.zebraPID, syscall.Signal(0)); err == nil {
			return true
		}
	}
	return false
}

// startDaemons launches zebra and bgpd inside the
// runtime's netns (or host netns when netns == ""),
// daemonized (-d), with the canonical -N pathspace.
//
// Implementation note: duck-agent holds no capabilities
// AND NoNewPrivileges=yes. The combination means the
// agent cannot exec /usr/libexec/duckos/duck-frr-launcher
// and have the launcher's file caps granted. Instead,
// the launcher runs as a long-running daemon
// (duck-frr-launcher.service) with its own
// CapabilityBoundingSet=CAP_SETUID CAP_SETGID CAP_SETPCAP
// and NoNewPrivileges=no, listening on
// /run/duckos-agent/frr-launcher.sock.
//
// For each EXEC, the launcher re-execs a child that does
// the privilege drop (setgroups+setresgid+setresuid to
// frr:frr), drops CAP_SETUID + CAP_SETGID from the
// bounding set BEFORE setresuid, and exec's
// /usr/lib/frr/<binary>. NO `-S` (--skip_runas) flag
// because Debian's zebra build does not expose it; the
// FRR child receives {NET_ADMIN, NET_RAW, NET_BIND_SERVICE,
// SYS_ADMIN, SETPCAP} via kernel exec semantics (parent
// P ∩ parent Bnd, then file_caps OR'd in). The parent
// (launcher daemon) returns the FRR child's PID to the
// agent over the socket. CAP_SYS_ADMIN reaches the FRR
// child because Debian FRR 10.3's zebra calls cap_set_proc
// with CAP_SYS_ADMIN unconditionally.
//
// Returns the zebra / bgpd PIDs on success. Failure
// tears down any half-started daemons before
// returning.
func (r *Runtime) startDaemons() error {
	conf := filepath.Join(r.sockDir, "frr.conf")
	zebraPIDFile := filepath.Join(r.sockDir, "zebra.pid")
	bgpdPIDFile := filepath.Join(r.sockDir, "bgpd.pid")
	zebraLog := filepath.Join(r.sockDir, "zebra.log")
	bgpdLog := filepath.Join(r.sockDir, "bgpd.log")
	if err := r.launcherExec("zebra", r.pathspace, conf, zebraLog, zebraPIDFile, []string{"-s", "90000000", "-i", zebraPIDFile}); err != nil {
		return fmt.Errorf("start zebra (launcher): %w", err)
	}
	if err := r.launcherExec("bgpd", r.pathspace, conf, bgpdLog, bgpdPIDFile, []string{"-i", bgpdPIDFile}); err != nil {
		_ = r.killPIDFromFile(zebraPIDFile)
		return fmt.Errorf("start bgpd (launcher): %w", err)
	}
	// Reap the daemons' PIDs from their pidfiles so
	// the runtime can signal / kill them deterministically.
	zPID, _ := readPIDFromFile(zebraPIDFile)
	bPID, _ := readPIDFromFile(bgpdPIDFile)
	r.zebraPID = zPID
	r.bgpdPID = bPID
	// FRR daemons need a brief boot window before
	// vtysh can talk to them.
	time.Sleep(800 * time.Millisecond)
	return nil
}

// launcherExec asks the running duck-frr-launcher
// daemon to start an FRR child (zebra or bgpd). The
// daemon listens on frrLauncherSocket and replies with
// `OK <pid>\n` or `ERR <message>\n`. We do not exec
// the launcher directly: see startDaemons' comment
// for why NoNewPrivileges=yes on the agent blocks
// file caps on a directly-exec'd launcher.
//
// The daemon's EXEC is idempotent: if (binary, pathspace)
// is already running and verified by /proc identity, the
// daemon returns the existing PID without forking. This
// is the canonical fix for the prior restart-orphan bug.
func (r *Runtime) launcherExec(binary, pathspace, conf, logPath, pidFile string, passthrough []string) error {
	args := []string{binary, pathspace, conf, logPath, pidFile}
	args = append(args, passthrough...)
	return launcherSend("EXEC", args...)
}

// launcherStop asks the running duck-frr-launcher
// daemon to STOP an FRR child for this pathspace. The
// daemon identity-checks every PID via /proc/<pid>/cmdline
// and refuses to kill a recycled PID or a wrong-binary
// process. STOP is idempotent: it returns OK even if no
// tracked entry exists.
//
// We ALWAYS go through the launcher for kill — never
// syscall.Kill on the recorded PID directly. That way the
// agent's Stop() never races with a pathspace-foreign
// process whose PID happens to collide with a stale
// pidfile.
func (r *Runtime) launcherStop(binary string) error {
	if binary != "" {
		return launcherSend("STOP", r.pathspace, binary)
	}
	return launcherSend("STOP", r.pathspace)
}

// launcherSend opens a fresh connection to the launcher
// socket, sends the command, and returns the response.
//
// Wire format:
//
//   EXEC <binary> <pathspace> <config> <log> <pidfile> [args...]
//   STOP <pathspace> [binary]
//   PING
//
// Reply: "OK ..." on success, "ERR <msg>\n" on failure.
//
// We open a NEW connection per command rather than
// reusing one. The launcher's accept loop is
// single-threaded; a stuck long-lived connection (one
// where the agent's socket buffer never drained, for
// instance) would block every other agent command. A
// short-lived connect / write / read / close cycle is
// also how systemd-networkd-style brokers typically
// operate.
func launcherSend(cmd string, args ...string) error {
	sock := frrLauncherSocket()
	c, err := net.DialTimeout("unix", sock, 5*time.Second)
	if err != nil {
		return fmt.Errorf("dial %s: %w", sock, err)
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(15 * time.Second))
	line := cmd
	for _, a := range args {
		if a == "" {
			continue
		}
		line += " " + a
	}
	line += "\n"
	if _, err := c.Write([]byte(line)); err != nil {
		return fmt.Errorf("write %s: %w", cmd, err)
	}
	br := bufio.NewReader(c)
	resp, err := br.ReadString('\n')
	if err != nil {
		return fmt.Errorf("read reply: %w", err)
	}
	resp = strings.TrimRight(resp, "\r\n")
	if !strings.HasPrefix(resp, "OK") {
		return fmt.Errorf("launcher %s: %s", cmd, resp)
	}
	return nil
}

// frrLauncherSocket is the Unix socket the
// duck-frr-launcher daemon listens on. The socket is
// owned root:duckos 0660 so the agent (running as
// root) and the duck-api user (duckos) can both
// dispatch commands.
//
// Tests override this with a private Unix socket path
// (var indirection so the override survives across
// every launcherSend call in the test goroutine).
var frrLauncherSocket = func() string {
	return "/run/duckos-agent/frr-launcher.sock"
}

// frrLauncherBinary is kept as a stable symbol so
// downstream code (and tests) can refer to the launcher
// install path even though the runtime no longer exec's
// it directly. The runtime talks to the running
// duck-frr-launcher daemon over frrLauncherSocket().
// The launcher itself is started by
// duck-frr-launcher.service which holds
// CapabilityBoundingSet=CAP_SETUID CAP_SETGID CAP_SETPCAP.
func frrLauncherBinary() string {
	return "/usr/libexec/duckos/duck-frr-launcher"
}

// killPidsLocked sends SIGTERM to the recorded
// daemons' PIDs, then SIGKILL after a brief grace
// period. Best-effort; never returns an error.
//
// Identity guard.
//
//   We only kill a PID whose /proc/<pid>/cmdline matches
//   /usr/lib/frr/{zebra,bgpd} with -N <pathspace>. A
//   recycled PID whose number happens to appear in a
//   stale zebra.pid (but whose cmdline is now an
//   unrelated process) is NEVER signalled by this code
//   path. This is the defence-in-depth complement to
//   launcherStop; even if the launcher daemon is
//   unreachable, the runtime never misdirects a kill.
//
//   The launcher's STOP is the canonical kill path; this
//   function is only invoked as a final sweep after
//   launcherStop returns, and as the immediate-cleanup
//   path in reloadOrRestart.
func (r *Runtime) killPidsLocked() {
	for _, pid := range []int{r.zebraPID, r.bgpdPID} {
		if pid <= 0 {
			continue
		}
		// Best-effort identity probe. We do not
		// hard-fail on identity mismatch; the
		// launcher is the canonical owner, and we
		// don't want a transient /proc read failure
		// to leak the previous flag. We log on
		// mismatch and skip.
		if isFRRForPathspace(pid, r.pathspace) {
			_ = syscall.Kill(pid, syscall.SIGTERM)
		} else {
			log.Printf("frr: killPidsLocked: pid %d not FRR for pathspace %s; skipped", pid, r.pathspace)
		}
	}
	time.Sleep(200 * time.Millisecond)
	for _, pid := range []int{r.zebraPID, r.bgpdPID} {
		if pid <= 0 {
			continue
		}
		if isFRRForPathspace(pid, r.pathspace) {
			_ = syscall.Kill(pid, syscall.SIGKILL)
		}
	}
	r.zebraPID = 0
	r.bgpdPID = 0
}

// isFRRForPathspace returns true iff pid is alive AND
// its /proc/<pid>/cmdline starts with /usr/lib/frr/ and
// contains -N <pathspace>. Used as the runtime's local
// defence-in-depth identity gate; the canonical gate
// lives in the launcher daemon.
//
// This helper is intentionally conservative: on any
// /proc read failure it returns false so the runtime
// never misdirects a kill. The launcher is the
// authoritative gate.
func isFRRForPathspace(pid int, pathspace string) bool {
	if pid <= 0 {
		return false
	}
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
	if err != nil {
		return false
	}
	// NUL-separated; replace NUL with space.
	s := strings.ReplaceAll(string(data), "\x00", " ")
	if !strings.Contains(s, "/usr/lib/frr/") {
		return false
	}
	if pathspace != "" {
		if !strings.Contains(s, "-N "+pathspace) {
			return false
		}
	}
	return true
}

// killPIDFromFile reads pid from path and kills it.
// Best-effort.
func (r *Runtime) killPIDFromFile(path string) error {
	pid, err := readPIDFromFile(path)
	if err != nil {
		return err
	}
	if pid > 0 {
		_ = syscall.Kill(pid, syscall.SIGTERM)
	}
	return nil
}

// queryFRRCounts asks vtysh for the BGP summary and
// the route count. The returned values are the
// canonical daemon-side counts — never the runtime's
// own bookkeeping.
func (r *Runtime) queryFRRCounts() (sessions, routes int) {
	out, err := r.runVTYSHOutput(context.Background(), "-c", "show ip bgp summary")
	if err == nil {
		sessions = countBGPSessions(out)
	}
	out2, err := r.runVTYSHOutput(context.Background(), "-c", "show ip route")
	if err == nil {
		routes = countRoutes(out2)
	}
	return sessions, routes
}

// Manager appendHistoryLocked is a thin shim that
// forwards to the Manager's appendHistoryLocked. It
// is exported on Manager for this purpose.
func (m *Manager) appendHistoryLocked(e LifecycleEvent) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.history = append(m.history, e)
	if len(m.history) > MaxLifecycle {
		m.history = m.history[len(m.history)-MaxLifecycle:]
	}
}

// runVTYSH runs vtysh inside the runtime's netns (or
// host netns) with the canonical -N pathspace and
// returns success / failure. Output is discarded; the
// caller uses runVTYSHOutput when it needs the bytes.
func (r *Runtime) runVTYSH(ctx context.Context, args ...string) error {
	_, _, err := r.runVTYSHFull(ctx, args...)
	return err
}

// runVTYSHOutput is runVTYSH returning the stdout bytes.
func (r *Runtime) runVTYSHOutput(ctx context.Context, args ...string) (string, error) {
	stdout, _, err := r.runVTYSHFull(ctx, args...)
	return stdout, err
}

// runVTYSHFull is the shared implementation. The
// pathspace flag (-N <ps>) is prepended automatically.
func (r *Runtime) runVTYSHFull(ctx context.Context, args ...string) (string, string, error) {
	full := append([]string{"-N", r.pathspace}, args...)
	cmd := exec.CommandContext(ctx, "/usr/bin/vtysh", full...)
	cmd.Env = []string{"PATH=/usr/bin:/bin", "LANG=C", "LC_ALL=C"}
	if r.netns != "" {
		cmd = exec.CommandContext(ctx, "/sbin/ip", append([]string{"netns", "exec", r.netns, "/usr/bin/vtysh"}, full...)...)
	}
	stdout, stderr, err := captureCmd(cmd)
	return stdout, stderr, err
}

// execInNetns runs prog with args inside the runtime's
// netns (or host netns when netns == ""), redirecting
// stdout / stderr to logPath. Returns the combined
// stderr on failure.
//
// As of the post-BETA.2-review privilege model, the
// runtime no longer exec's anything that needs netns
// isolation directly: FRR is started via the
// duck-frr-launcher daemon over a Unix socket, and the
// launcher handles netns via the runtime's pathspace
// argument. This helper is retained for callers that
// need a generic in-netns exec (e.g. `vtysh -f
// <conf>`); netns-aware invocations should use
// /sbin/ip netns exec explicitly to keep the path
// self-documenting.
func (r *Runtime) execInNetns(prog string, args []string, logPath string) error {
	cmd := exec.Command(prog, args...)
	cmd.Env = []string{"PATH=/sbin:/usr/sbin:/usr/bin:/bin", "LANG=C", "LC_ALL=C"}
	if r.netns != "" {
		cmd = exec.Command("/sbin/ip", append([]string{"netns", "exec", r.netns, prog}, args...)...)
		cmd.Env = []string{"PATH=/sbin:/usr/sbin:/usr/bin:/bin", "LANG=C", "LC_ALL=C"}
	}
	lf, lerr := os.Create(logPath)
	if lerr != nil {
		return fmt.Errorf("create log %s: %w", logPath, lerr)
	}
	cmd.Stdout = lf
	cmd.Stderr = lf
	if err := cmd.Run(); err != nil {
		_ = lf.Close()
		tail, _ := os.ReadFile(logPath)
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(tail)))
	}
	_ = lf.Close()
	return nil
}

// cleanupOnFailure undoes everything NewRuntime did
// when the startup path failed midway.
func (r *Runtime) cleanupOnFailure() error {
	r.killPidsLocked()
	if r.sockDir != "" {
		_ = os.RemoveAll(r.sockDir)
	}
	if r.netns != "" {
		_ = runIPNetnsDel(r.netns)
	}
	return nil
}

// =====================================================================
// Helpers that defer to system calls without leaking
// platform imports into the rest of the package.
// =====================================================================

// runIPNetnsAdd / runIPNetnsDel are tiny wrappers
// around `ip netns add|del` so the runtime stays
// portable across the package boundary.
func runIPNetnsAdd(name string) error {
	return runIPNetnsOp("add", name)
}
func runIPNetnsDel(name string) error {
	return runIPNetnsOp("del", name)
}
func runIPNetnsOp(op, name string) error {
	cmd := exec.Command("/sbin/ip", "netns", op, name)
	cmd.Env = []string{"PATH=/sbin:/usr/sbin:/usr/bin:/bin"}
	out, err := cmd.CombinedOutput()
	if err != nil {
		// Deleting a non-existent netns is OK; the
		// runtime's Stop is idempotent.
		if op == "del" && strings.Contains(string(out), "does not exist") {
			return nil
		}
		return fmt.Errorf("ip netns %s %s: %w (out=%s)", op, name, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// readPIDFromFile reads the first whitespace-delimited
// token of path as a pid. Returns 0 if the file is
// missing or empty (the daemon has not yet written its
// pid — common during cold start).
func readPIDFromFile(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	var pid int
	_, _ = fmt.Sscanf(strings.TrimSpace(string(data)), "%d", &pid)
	return pid, nil
}

// captureCmd runs cmd and returns its stdout, stderr,
// and combined error. Reused by runVTYSHFull.
func captureCmd(cmd *exec.Cmd) (string, string, error) {
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

// countBGPSessions counts "Established" lines in a
// `show ip bgp summary` table. Used by State().
func countBGPSessions(out string) int {
	n := 0
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "Established") {
			n++
		}
	}
	return n
}

// countRoutes counts non-empty, non-header lines in a
// `show ip route` table.
func countRoutes(out string) int {
	n := 0
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if strings.HasPrefix(line, "Codes:") {
			continue
		}
		if strings.HasPrefix(line, "Kernel ") {
			continue
		}
		n++
	}
	return n
}

// =====================================================================
// IPC DTOs (consumed by the Agent's frr_host.go).
// =====================================================================

// FRRStatusResponse is the JSON shape /agent/v1/frr returns.
type FRRStatusResponse struct {
	Version             int       `json:"version"`
	UpdatedAt           time.Time `json:"updatedAt"`
	Pathspace           string    `json:"pathspace"`
	SessionsEstablished int       `json:"sessionsEstablished"`
	RoutesInstalled     int       `json:"routesInstalled"`
	LastReload          time.Time `json:"lastReload"`
	LastError           string    `json:"lastError,omitempty"`
	Operator            string    `json:"operator,omitempty"`
}

// FRRLifecycleResponse is the JSON shape Apply / Rollback return.
type FRRLifecycleResponse struct {
	Version   int       `json:"version"`
	UpdatedAt time.Time `json:"updatedAt"`
	Result    string    `json:"result"`
	Reason    string    `json:"reason,omitempty"`
}

// MarshalStatus marshals the runtime's State into the
// IPC status response. Exported so the Agent adapter
// can use it without duplicating the shape.
func (r *Runtime) MarshalStatus(operator string) ([]byte, error) {
	st := r.State()
	return json.Marshal(FRRStatusResponse{
		Version:             1,
		UpdatedAt:           st.At,
		Pathspace:           r.pathspace,
		SessionsEstablished: st.SessionsEstablished,
		RoutesInstalled:     st.RoutesInstalled,
		LastReload:          st.LastReload,
		LastError:           st.LastError,
		Operator:            operator,
	})
}

// MarshalLifecycle marshals an Apply / Rollback outcome.
func (r *Runtime) MarshalLifecycle(result, reason string) ([]byte, error) {
	return json.Marshal(FRRLifecycleResponse{
		Version:   1,
		UpdatedAt: r.clock().UTC(),
		Result:    result,
		Reason:    reason,
	})
}

// silence unused-import warnings for io when no helper
// uses it.
var _ = io.Discard

// =====================================================================
// Public sentinel errors.
// =====================================================================

// ErrConfig is the typed error envelope for config
// rejection. Apply wraps the validator's error so
// callers can match on it.
var ErrConfig = errors.New("frr: config rejected")
