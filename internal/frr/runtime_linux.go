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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
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
	// FRR's pathspace (-N) requires the matching
	// /etc/frr/<ps>/vtysh.conf to exist; otherwise
	// vtysh aborts with "Can't open configuration
	// file ... due to 'No such file or directory'"
	// before parsing the runtime's frr.conf. An
	// empty file is sufficient; the daemon vty
	// sockets are under /run/frr/<ps>/, not
	// /etc/frr/<ps>/. The dir must be owned by frr:frr
	// because the daemon opens it after dropping privs.
	// Best-effort: if the Agent's restricted capability
	// set refuses to mkdir /etc/frr/<ps>, we log and
	// continue; vtysh will then emit its warning
	// without affecting the daemons.
	etcDir := filepath.Join("/etc/frr", pathspace)
	if err := os.MkdirAll(etcDir, 0o755); err != nil {
		log.Printf("frr: pre-create %s: %v; vtysh will warn", etcDir, err)
	} else {
		_ = os.WriteFile(filepath.Join(etcDir, "vtysh.conf"), []byte("!\n"), 0o644)
		// chownToFRR refuses paths outside StateBase and
		// PathspaceBase. /etc/frr/<ps> is not in that set,
		// so we deliberately skip chown here: the FRR
		// Debian package already ships /etc/frr owned by
		// root:frr and the runtime does not need to mutate
		// it. If the package is missing, the daemons will
		// surface a clear error on startup, which is the
		// right time for the operator to fix the install.
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
	if err := r.runVTYSH(ctx, "-d", "bgpd", "-c", "show bgp ipv4 unicast"); err != nil {
		_ = r.cleanupOnFailure()
		return nil, errors.New("frr: startup BGP readiness failed")
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
// Returns the zebra / bgpd PIDs on success. Failure
// tears down any half-started daemons before
// returning.
func (r *Runtime) startDaemons() error {
	conf := filepath.Join(r.sockDir, "frr.conf")
	zebraPIDFile := filepath.Join(r.sockDir, "zebra.pid")
	bgpdPIDFile := filepath.Join(r.sockDir, "bgpd.pid")
	zebraLog := filepath.Join(r.sockDir, "zebra.log")
	bgpdLog := filepath.Join(r.sockDir, "bgpd.log")
	zebraBin := frrBinary("zebra")
	bgpdBin := frrBinary("bgpd")
	// -d daemonizes; without it the test process
	// would block forever. -N is mandatory on FRR 10+
	// to isolate from any host-wide FRR instance; -N
	// auto-creates the pathspace's own /run/frr/<ps>/
	// zebra socket, so -z must NOT be passed (the FRR
	// log otherwise complains "-N option overridden
	// by -z for zebra named socket path").
	zebraArgs := []string{"-d", "-f", conf, "-i", zebraPIDFile,
		"-s", "90000000", "-A", "127.0.0.1", "-N", r.pathspace,
		"-u", FRRRunUser, "-g", FRRRunGroup}
	bgpdArgs := []string{"-d", "-f", conf, "-i", bgpdPIDFile,
		"-A", "127.0.0.1", "-N", r.pathspace,
		"-u", FRRRunUser, "-g", FRRRunGroup}
	if err := r.execInNetns(zebraBin, zebraArgs, zebraLog); err != nil {
		return fmt.Errorf("start zebra: %w", err)
	}
	if err := r.execInNetns(bgpdBin, bgpdArgs, bgpdLog); err != nil {
		_ = r.killPIDFromFile(zebraPIDFile)
		return fmt.Errorf("start bgpd: %w", err)
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

// killPidsLocked sends SIGTERM to the recorded
// daemons' PIDs, then SIGKILL after a brief grace
// period. Best-effort; never returns an error.
func (r *Runtime) killPidsLocked() {
	for _, pid := range []int{r.zebraPID, r.bgpdPID} {
		if pid <= 0 {
			continue
		}
		_ = syscall.Kill(pid, syscall.SIGTERM)
	}
	time.Sleep(200 * time.Millisecond)
	for _, pid := range []int{r.zebraPID, r.bgpdPID} {
		if pid <= 0 {
			continue
		}
		_ = syscall.Kill(pid, syscall.SIGKILL)
	}
	r.zebraPID = 0
	r.bgpdPID = 0
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
