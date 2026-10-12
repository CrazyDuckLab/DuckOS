//go:build linux

// cmd/duck-frr-launcher/tracker.go: DuckOS FRR lifetime
// owner.
//
// Architecture (see privilege_model.go + runtime_linux.go):
//
//   The duck-frr-launcher daemon is the canonical
//   owner of every /usr/lib/frr/{zebra,bgpd} process
//   it starts. The duck-agent NEVER kills, PIDs
//   tracked zebra/bgpd directly. Instead, the agent
//   asks the launcher (over
//   /run/duckos-agent/frr-launcher.sock) to:
//
//     EXEC  <binary> <pathspace> <conf> <log> <pidfile>
//                          [passthrough ...]   - start FRR
//                          (idempotent: returns existing
//                           PID if the same (pathspace,
//                           binary) is already running and
//                           verified to be FRR)
//     STOP  <pathspace>                  - kill zebra+bgpd
//     STOP  <pathspace> <binary>         - kill one
//     LIST                               - dump tracked
//     PING                                - liveness
//
//   The launcher daemon verifies the actual identity of
//   every tracked PID by reading /proc/<pid>/cmdline and
//   matching the binary path (/usr/lib/frr/<bin>) AND the
//   -N <pathspace> flag. A recycled PID that happens to
//   reuse a stale zebra.pid content (an unrelated process)
//   is NOT killed. A zombie, exited, or wrong-binary PID
//   is NOT killed. Only an alive, identity-verified FRR PID
//   is reaped.
//
//   This eliminates the prior bug:
//
//     systemctl restart duck-agent
//     -> the OLD runtime's killPidsLocked best-effort
//        syscall.Kill raced against the launcher daemon's
//        fork+exec, sometimes targeted a stale / recycled
//        PID, or never reaped the actual FRR daemon (which
//        has setsid()'d into its own session and is
//        reparented to PID 1 once the re-exec launcher
//        exits).
//     -> the new agent then issued a second EXEC and a
//        second zebra/bgpd appeared alongside the
//        surviving old one.
//
//   With this owner design, EVERY kill is gated by a
//   /proc/<pid>/cmdline identity check; the launcher
//   daemon serialises all EXEC/STOP/LIST over the same
//   Unix-socket connection; and the FRR lifetime is
//   always 1:1 with a (pathspace, binary) tuple in the
//   launcher's tracker.
//
// Rehydration:
//
//   On launcher daemon startup, we scan every existing
//   /run/frr/*/*.pid and /var/lib/frr/*/*.pid file, and
//   for each PID we verify the actual process identity
//   via /proc/<pid>/cmdline. Surviving FRR children are
//   re-tracked; stale / recycled PIDs are left alone.
//   This handles both fresh boot (no pidfiles) and
//   launcher-restart scenarios where pidfiles outlive the
//   daemon.

package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// TrackedFRR is the canonical record the launcher
// daemon keeps for every running FRR child it has
// verified. Identity is re-checked before every kill;
// the recorded cmdline struct below is a stale-by-design
// cache, not authoritative.
type TrackedFRR struct {
	Binary    string    // "zebra" or "bgpd"
	Pathspace string    // "-N" flag value
	PID       int       // daemon PID (NOT the re-exec launcher)
	StartedAt time.Time // when the launcher forked it
	PidFile   string    // <sockDir>/<binary>.pid; cleanup on STOP
	SockDir   string    // <run/frr>/<pathspace>; cleanup is caller's job
	Cmdline   string    // last /proc/<pid>/cmdline snapshot
}

// Tracker is the launcher's authoritative map of FRR
// children, indexed by (pathspace, binary). It is
// goroutine-safe; the daemon's accept loop is
// single-threaded but mutex access costs little and makes
// the data race detector pass on tests.
type Tracker struct {
	mu      sync.Mutex
	entries map[string]map[string]*TrackedFRR // pathspace -> binary -> entry
}

// NewTracker returns an empty tracker.
func NewTracker() *Tracker {
	return &Tracker{entries: map[string]map[string]*TrackedFRR{}}
}

// Lookup returns a copy of the entry for
// (pathspace, binary). The copy is safe to read without
// the lock; mutating fields is NOT safe.
func (t *Tracker) Lookup(pathspace, binary string) *TrackedFRR {
	t.mu.Lock()
	defer t.mu.Unlock()
	m, ok := t.entries[pathspace]
	if !ok {
		return nil
	}
	e, ok := m[binary]
	if !ok {
		return nil
	}
	cp := *e
	return &cp
}

// Put inserts or replaces the tracker entry. The caller
// transfers ownership of the pointer; do not mutate the
// pointed-to fields after Put.
func (t *Tracker) Put(e *TrackedFRR) {
	t.mu.Lock()
	defer t.mu.Unlock()
	m, ok := t.entries[e.Pathspace]
	if !ok {
		m = map[string]*TrackedFRR{}
		t.entries[e.Pathspace] = m
	}
	m[e.Binary] = e
}

// Forget removes any entry for (pathspace, binary).
// Returns the entry that was there, or nil.
func (t *Tracker) Forget(pathspace, binary string) *TrackedFRR {
	t.mu.Lock()
	defer t.mu.Unlock()
	m, ok := t.entries[pathspace]
	if !ok {
		return nil
	}
	e, ok := m[binary]
	if !ok {
		return nil
	}
	delete(m, binary)
	if len(m) == 0 {
		delete(t.entries, pathspace)
	}
	return e
}

// ForgetPathspace removes every entry for pathspace.
// Returns the removed entries.
func (t *Tracker) ForgetPathspace(pathspace string) []*TrackedFRR {
	t.mu.Lock()
	defer t.mu.Unlock()
	m, ok := t.entries[pathspace]
	if !ok {
		return nil
	}
	out := make([]*TrackedFRR, 0, len(m))
	for _, e := range m {
		out = append(out, e)
	}
	delete(t.entries, pathspace)
	return out
}

// Snapshot returns a defensive deep-copy snapshot of every
// tracked entry. Used by LIST and shutdown cleanup.
func (t *Tracker) Snapshot() []*TrackedFRR {
	t.mu.Lock()
	defer t.mu.Unlock()
	var out []*TrackedFRR
	for _, m := range t.entries {
		for _, e := range m {
			cp := *e
			out = append(out, &cp)
		}
	}
	return out
}

// =====================================================================
// Process identity verification.
//
// isFRRProcess(pid, binary, pathspace) returns true iff:
//
//   1. /proc/<pid> exists (process is alive OR a zombie;
//      zombies still expose cmdline),
//   2. /proc/<pid>/comm matches the expected binary name,
//   3. /proc/<pid>/cmdline contains the literal argv0
//      "/usr/lib/frr/<binary>",
//   4. /proc/<pid>/cmdline contains the literal
//      "<pathspace>" adjacent to a "-N" token (the FRR
//      -N flag is the canonical pathspace selector).
//
// Identity is required BEFORE kill; a recycled PID pointing
// at an unrelated process is NEVER reaped, even if a
// stale zebra.pid happens to contain its number.
// =====================================================================

// readPIDCmdline reads /proc/<pid>/cmdline as a
// NUL-separated byte slice and joins with spaces for
// human inspection. The first field is argv0.
//
// The kernel writes argv entries as NUL-terminated
// chunks with no separator between them; the LAST
// chunk ends with a NUL. bufio.ReadBytes(0) returns the
// chunk up to AND INCLUDING the NUL delimiter. We
// strip the trailing NUL, append the chunk, and read
// the next one until the underlying read returns io.EOF.
func readPIDCmdline(pid int) (string, error) {
	raw, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
	if err != nil {
		return "", err
	}
	// argv entries separated by NUL. The final
	// character is a NUL too (kernel invariant). A
	// trailing empty string (the post-final NUL) is
	// dropped.
	parts := strings.Split(string(raw), "\x00")
	// Drop the trailing empty entry from the
	// trailing NUL.
	if len(parts) > 0 && parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	return strings.Join(parts, " "), nil
}

// readPIDComm returns /proc/<pid>/comm (the truncated
// process name). Used as a fast sanity check before
// pulling cmdline.
func readPIDComm(pid int) (string, error) {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", pid))
	if err != nil {
		return "", err
	}
	return strings.TrimRight(string(data), "\n"), nil
}

// pidAlive returns nil if /proc/<pid> is reachable AND
// the process is in a "running" state (R/S/D/T/I). Zombies
// (state=Z) and dead (state=X) are reported as errors so
// we never try to signal them.
func pidAlive(pid int) error {
	if pid <= 0 {
		return fmt.Errorf("pid %d invalid", pid)
	}
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return fmt.Errorf("pid %d: %w", pid, err)
	}
	// /proc/<pid>/stat: "pid (comm) state ..."
	// state is the third whitespace-separated field
	// after the comm. comm may contain spaces; the
	// kernel wraps it in parens.
	// Format:
	//   PID (COMM) STATE PPID ...
	// Use the LAST ')' as the comm terminator to be
	// robust against ')' in the process name (rare).
	i := bytes.LastIndexByte(data, ')')
	if i < 0 || i+2 >= len(data) {
		return fmt.Errorf("pid %d: malformed /proc stat", pid)
	}
	state := data[i+2]
	// 'Z' = zombie, 'X' = dead. Anything else (R, S, D,
	// T, I) is alive in the kill sense.
	if state == 'Z' || state == 'X' {
		return fmt.Errorf("pid %d: not alive (state=%c)", pid, state)
	}
	return nil
}

// pidGone returns nil if /proc/<pid> is absent OR the
// process is a zombie / dead. Used by terminateAndReap
// to confirm that a SIGTERM/SIGKILL was effective.
func pidGone(pid int) bool {
	if pid <= 0 {
		return true
	}
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		// /proc/<pid> missing means the process has
		// been reaped (parent waited) OR the PID has
		// been recycled and the new owner is not
		// us. Either way, the original PID is gone.
		return true
	}
	i := bytes.LastIndexByte(data, ')')
	if i < 0 || i+2 >= len(data) {
		return true // malformed; treat as gone
	}
	state := data[i+2]
	// Z (zombie) and X (dead) count as gone for the
	// purpose of "did the kill succeed". The PID may
	// still occupy a slot in the kernel's PID hash
	// table, waiting for its parent to wait(); from
	// our standpoint it has exited.
	return state == 'Z' || state == 'X'
}

// isFRRProcess reports whether pid is /usr/lib/frr/<binary>
// with -N <pathspace> in its argv. Returns a precise
// error so the launcher can log WHY a stale PID was
// rejected.
func isFRRProcess(pid int, binary, pathspace string) error {
	if err := pidAlive(pid); err != nil {
		return fmt.Errorf("pid %d: %w", pid, err)
	}
	comm, err := readPIDComm(pid)
	if err != nil {
		return fmt.Errorf("pid %d: comm: %w", pid, err)
	}
	// comm is truncated to 15 chars. zebra and bgpd fit.
	if comm != binary {
		return fmt.Errorf("pid %d: comm=%q expected %q", pid, comm, binary)
	}
	cmdline, err := readPIDCmdline(pid)
	if err != nil {
		return fmt.Errorf("pid %d: cmdline: %w", pid, err)
	}
	if !strings.Contains(cmdline, "/usr/lib/frr/"+binary) {
		return fmt.Errorf("pid %d: argv0 not /usr/lib/frr/%s (cmdline=%q)", pid, binary, cmdline)
	}
	if pathspace != "" {
		// Match "-N <pathspace>" as adjacent words. Avoid
		// false positives where the path space name is a
		// substring of some other token.
		if !strings.Contains(cmdline, " -N "+pathspace+" ") &&
			!strings.HasSuffix(cmdline, " -N "+pathspace) &&
			!strings.Contains(cmdline, " -N "+pathspace+"\x00") {
			return fmt.Errorf("pid %d: -N %q not in cmdline (cmdline=%q)", pid, pathspace, cmdline)
		}
	}
	return nil
}

// =====================================================================
// Lifecycle primitives.
// =====================================================================

// terminateAndReap sends SIGTERM, polls for 5s, then
// SIGKILL. It is identity-checked: if the PID is not the
// FRR process we tracked (a recycled PID), we ABORT.
// Best-effort; never panics.
//
// Returns:
//   - nil on success (process gone / zombie reaped by us)
//   - errIdentityMismatch if the PID's identity changed
//     (recycled PID); the caller MUST NOT log "killed"
//   - errProcessGone if the PID is already exited/zombie
//     before we get to signal it
//   - other errors for transient conditions
var (
	errIdentityMismatch = errors.New("frr-launcher: pid identity mismatch")
	errProcessGone      = errors.New("frr-launcher: pid is not an exit target")
)

func terminateAndReap(pid int, binary, pathspace string) error {
	// Identity gate. Refuse to kill anything that does
	// not match the recorded (binary, pathspace).
	if err := isFRRProcess(pid, binary, pathspace); err != nil {
		// Distinguish "gone" (no need to retry) from
		// "wrong identity" (do NOT kill).
		if os.IsNotExist(err) || strings.Contains(err.Error(), "not alive") {
			return fmt.Errorf("%w: %v", errProcessGone, err)
		}
		return fmt.Errorf("%w: %v", errIdentityMismatch, err)
	}
	// SIGTERM, then poll for up to 5s.
	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil && err != syscall.ESRCH {
		return fmt.Errorf("kill %d SIGTERM: %w", pid, err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if pidGone(pid) {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	// Force kill.
	if err := syscall.Kill(pid, syscall.SIGKILL); err != nil && err != syscall.ESRCH {
		return fmt.Errorf("kill %d SIGKILL: %w", pid, err)
	}
	// Wait for the process to be reaped (gone from
	// /proc OR state Z). SIGKILL is unblockable.
	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if pidGone(pid) {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("pid %d did not exit within 5+2s after SIGKILL", pid)
}

// readPIDFile reads the first whitespace-delimited
// token of path as a pid. Returns 0 if the file is
// missing or empty.
func readPIDFile(path string) int {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	var pid int
	_, _ = fmt.Sscanf(strings.TrimSpace(string(data)), "%d", &pid)
	return pid
}

// =====================================================================
// Rehydration: scan existing pidfiles and re-track
// surviving children on launcher startup.
//
// rehydrateFromPidfiles walks /run/frr and /var/lib/frr
// looking for <dir>/<binary>.pid files. For each PID it
// verifies the identity via /proc. Only identity-verified
// FRR children are added to the tracker. Stale PIDs
// (recycled to non-FRRR, dead, zombie) are NOT touched.
// =====================================================================

// pidfileGlob walks root looking for files named
// <pathspace>/<binary>.pid and returns (pathspace,
// binary, full path). It deliberately avoids filepath.Glob
// because the pathspace name has bounded chars but we
// want to enumerate independently of shell globs.
func pidfileGlob(root, binary string) ([]string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		pf := filepath.Join(root, e.Name(), binary+".pid")
		if _, err := os.Stat(pf); err == nil {
			out = append(out, pf)
		}
	}
	return out, nil
}

// rehydrateFromPidfiles rebuilds the tracker's state by
// reading every /run/frr/*/<bin>.pid and
// /var/lib/frr/*/<bin>.pid and verifying each PID via
// /proc/<pid>/cmdline. Identity-verified FRR children
// are tracked; everything else is left as-is (no kill).
func (t *Tracker) rehydrateFromPidfiles() (rehydrated, rejected int, ferr error) {
	roots := []string{"/run/frr", "/var/lib/frr"}
	binaries := []string{"zebra", "bgpd"}
	for _, root := range roots {
		for _, binary := range binaries {
			pidfiles, err := pidfileGlob(root, binary)
			if err != nil {
				ferr = fmt.Errorf("scan %s: %w", root, err)
				continue
			}
			for _, pf := range pidfiles {
				pid := readPIDFile(pf)
				if pid <= 0 {
					continue
				}
				pathspace := filepath.Base(filepath.Dir(pf))
				if err := isFRRProcess(pid, binary, pathspace); err != nil {
					// Stale or recycled PID; do NOT
					// touch (no pkill, no kill). Leave
					// the pidfile for the operator to
					// clean if needed.
					rejected++
					continue
				}
				// Already tracked?
				if existing := t.Lookup(pathspace, binary); existing != nil && existing.PID == pid {
					continue
				}
				t.Put(&TrackedFRR{
					Binary:    binary,
					Pathspace: pathspace,
					PID:       pid,
					StartedAt: time.Now().UTC(), // approximate
					PidFile:   pf,
					SockDir:   filepath.Dir(pf),
					Cmdline:   "/usr/lib/frr/" + binary,
				})
				rehydrated++
			}
		}
	}
	return rehydrated, rejected, ferr
}

// =====================================================================
// Wire-format helpers.
// =====================================================================

// formatList formats one line per tracked entry:
//
//	"OK <binary> <pathspace> <pid> <startedAt RFC3339>\n"
func formatList(entries []*TrackedFRR) []byte {
	var out bytes.Buffer
	out.WriteString("OK\n")
	for _, e := range entries {
		fmt.Fprintf(&out, "%s %s %d %s\n", e.Binary, e.Pathspace, e.PID, e.StartedAt.Format(time.RFC3339Nano))
	}
	return out.Bytes()
}

// parseSTOPArg accepts either "STOP <ps>" or
// "STOP <ps> <binary>" and returns (ps, binary, ok).
func parseSTOPArg(input []string) (string, string, bool) {
	if len(input) == 0 {
		return "", "", false
	}
	if len(input) == 1 {
		return input[0], "", true
	}
	return input[0], input[1], true
}

// parseIntDefault returns the int or fallback. Used
// when an arg is optional.
func parseIntDefault(s string, fallback int) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return fallback
	}
	return n
}
