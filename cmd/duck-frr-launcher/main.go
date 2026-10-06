//go:build linux

// cmd/duck-frr-launcher/main.go: DuckOS FRR launcher.
//
// Architecture (see internal/frr/privilege_model.go):
//
//   duck-agent has CapabilityBoundingSet= (empty) AND
//   NoNewPrivileges=yes. Both are required for the
//   sandbox: empty bounding means the agent cannot gain
//   caps from any source, and no_new_privs means file
//   caps on exec'd binaries are ignored. The combined
//   effect is that the agent cannot exec a binary with
//   file caps and have those caps granted.
//
//   The Agent therefore cannot exec this launcher
//   directly and have the launcher receive
//   {SETUID,SETGID,SETPCAP} from file caps. Instead, the
//   launcher runs as a separate systemd service
//   (duck-frr-launcher.service) with its own
//   CapabilityBoundingSet=CAP_SETUID CAP_SETGID CAP_SETPCAP
//   CAP_NET_ADMIN CAP_NET_RAW CAP_NET_BIND_SERVICE
//   CAP_SYS_ADMIN and NoNewPrivileges=no. The agent
//   talks to the launcher over a Unix socket
//   (/run/duckos-agent/frr-launcher.sock) using a tiny
//   line protocol.
//
//   For each EXEC command the launcher (re-exec'd child):
//     1. PR_SET_KEEPCAPS=1 — preserves P across the
//        root→frr uid transition.
//     2. drop CAP_SETUID + CAP_SETGID from the bounding
//        set via PR_CAPBSET_DROP. This MUST run BEFORE
//        setresuid: Linux clears E on uid transition,
//        and PR_CAPBSET_DROP requires CAP_SETPCAP in E.
//        CAP_SETPCAP is INTENTIONALLY retained in Bnd
//        so the FRR child inherits it at exec.
//     3. setgroups([]), setresgid(frr_gid),
//        setresuid(frr_uid) — drop to frr:frr.
//     4. mkdir /etc/frr/<ps>/ 0755 + write vtysh.conf.
//        The Agent cannot do this — /etc/frr is frr:frr
//        0751; the launcher, now uid=frr, matches the
//        owner.
//     5. exec /usr/lib/frr/{zebra,bgpd}. The launcher
//        does NOT call cap_set_proc itself: kernel exec
//        semantics hand the FRR child exactly the caps
//        it needs (parent P ∩ parent Bnd, then file
//        caps OR'd in). FRR's own privs_init calls
//        cap_set_proc to narrow further; that requires
//        CAP_SETPCAP in the child's E, which it has
//        because the FRR file caps include cap_setpcap.
//
//   The FRR child runs as uid=frr, gid=frr, no
//   supplementary groups, with P/E =
//   {NET_ADMIN, NET_RAW, NET_BIND_SERVICE, SYS_ADMIN,
//   SETPCAP}. NO CAP_SETUID. NO CAP_SETGID. NO
//   duck-agent capabilities. CAP_SYS_ADMIN is scoped
//   to the FRR process only (Debian FRR 10.3's zebra
//   unconditionally calls cap_set_proc with
//   CAP_SYS_ADMIN and Debian's zebra build does NOT
//   expose the upstream -S / --skip_runas flag; only
//   bgpd does).
//
//   install.sh sets:
//     setcap cap_net_admin,cap_net_raw,cap_net_bind_service,cap_sys_admin,cap_setpcap+ep
//         /usr/lib/frr/{zebra,bgpd}
//     setcap cap_setuid,cap_setgid,cap_setpcap+ep
//         /usr/libexec/duckos/duck-frr-launcher
//   The launcher file caps are minimal; the FRR caps
//   propagate to the FRR child via the launcher service
//   unit's CapabilityBoundingSet (the canonical grant).
//   The FRR file caps include cap_setpcap so FRR's own
//   privs_init can narrow the cap set further without
//   EPERM.
//
//   This binary stays as small as possible so its
//   attack surface matches its narrow job.

package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// frrUid / frrGid are passed in by the runtime via
// --frr-uid / --frr-gid. Default 0/0 forces lookup.
var (
	frrUidFlag = flag.Int("frr-uid", 0, "uid of the frr service user")
	frrGidFlag = flag.Int("frr-gid", 0, "gid of the frr service group")
)

// frrRunUser / frrRunGroup are the names we resolve via
// NSS when the uid/gid flags are zero.
const (
	frrRunUser  = "frr"
	frrRunGroup = "frr"
)

// launcherOnlyCaps is the set of capabilities dropped
// from the launcher's bounding set BEFORE setresuid.
// CAP_SETUID + CAP_SETGID are dropped so the FRR child
// exec'd from this launcher never inherits them.
//
// CAP_SETPCAP is INTENTIONALLY NOT dropped. Debian FRR
// 10.3's zebra calls cap_set_proc in privs_init and
// that requires CAP_SETPCAP in E. The cap_setpcap file
// cap on /usr/lib/frr/{zebra,bgpd} (installed by
// install.sh) gives the FRR child CAP_SETPCAP in E
// independently of Bnd at exec, so even if the
// launcher's Bnd retained SETPCAP, the file cap would
// also work. We retain SETPCAP in Bnd as belt-and-braces
// so the inherited Bnd is identical to what the FRR
// child receives from file caps; this matches the Bnd
// the Debian FRR package's own frr.service unit grants.
var launcherOnlyCaps = []string{"setuid", "setgid"}

func main() {
	// We use a hand-rolled parser so the daemon mode
	// (--daemon --listen=PATH) can coexist with the
	// legacy one-shot mode (--pathspace=X --binary=Y
	// ...) on the same binary.
	if len(os.Args) >= 2 && os.Args[1] == "--daemon" {
		runDaemon(os.Args[2:])
		return
	}
	runOneShot(os.Args[1:])
}

// runDaemon is the long-running privileged broker.
// systemd starts it via duck-frr-launcher.service. It
// listens on a Unix socket and serves EXEC / STOP /
// LIST / PING commands from the duck-agent.
//
// The daemon owns the FRR lifetime via the Tracker
// (see tracker.go). On startup it rehydrates the
// tracker by scanning existing pidfiles and
// verifying each candidate via /proc/<pid>/cmdline.
// On SIGTERM / SIGINT the daemon reaps every
// tracked FRR process identity-gated (a recycled
// PID is never killed).
func runDaemon(args []string) {
	fs := flag.NewFlagSet("duck-frr-launcher --daemon", flag.ExitOnError)
	listenAddr := fs.String("listen", "/run/duckos-agent/frr-launcher.sock", "unix socket listen path")
	socketMode := fs.String("socket-mode", "0660", "permission mode for the unix socket")
	socketGroup := fs.String("socket-group", "duckos", "group owner for the unix socket")
	if err := fs.Parse(args); err != nil {
		die("frr-launcher daemon: %v", err)
	}

	uid, gid, err := lookupFrrIds()
	if err != nil {
		die("frr-launcher daemon: %v", err)
	}

	tracker := NewTracker()
	installTracker(tracker)
	rehydrated, rejected, rerr := tracker.rehydrateFromPidfiles()
	if rerr != nil {
		fmt.Fprintf(os.Stderr, "frr-launcher daemon: rehydrate: %v\n", rerr)
	}
	fmt.Fprintf(os.Stderr, "frr-launcher daemon: rehydrated=%d rejected=%d\n", rehydrated, rejected)
	// Shutdown semantics:
	//
	//   On SIGTERM / SIGINT we DO NOT reap tracked
	//   FRR children. The launcher is a long-running
	//   broker that systemd may restart (Restart=
	//   on-failure) or that the agent may stop /
	//   start as part of an upgrade. In both cases,
	//   the canonical FRR lifetime owner is the
	//   AGENT's runtime, not the launcher — and
	//   the agent's runtime holds the bookkeeping
	//   needed to cleanly reap on its own Stop().
	//   If we killed here, a launcher restart
	//   would orphan-replace every DuckOS zebra /
	//   bgpd in the system.
	//
	//   Identity-gated reap happens instead via the
	//   STOP <pathspace> command, dispatched by the
	//   agent's Stop() path. This is the canonical
	//   kill route; the launcher stays a stateless
	//   broker across its own restarts.
	stopCh := make(chan os.Signal, 4)
	signal.Notify(stopCh, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		sig := <-stopCh
		fmt.Fprintf(os.Stderr, "frr-launcher daemon: caught %s; exiting (no reap)\n", ssigname(sig))
		// Best-effort: remove the socket so a fresh
		// launcher (after systemd Restart=on-failure)
		// does not see a stale listener.
		_ = os.Remove(*listenAddr)
		os.Exit(0)
	}()

	// Ensure the parent directory for the socket
	// exists. The agent normally creates /run/duckos-agent
	// via its own RuntimeDirectory=duckos-agent directive,
	// but if the launcher boots first (which is what the
	// unit ordering in duck-agent.service wants so the
	// agent's FRR startup can dial the socket), the
	// directory may not exist yet. The launcher owns the
	// directory's ownership: root:duckos 0751 (the agent
	// creates the dir at 0751 in its unit; we mirror that
	// here so the launcher is consistent regardless of
	// boot order).
	if dir := filepath.Dir(*listenAddr); dir != "" {
		if err := os.MkdirAll(dir, 0o751); err != nil {
			die("frr-launcher daemon: mkdir %s: %v", dir, err)
		}
		if gid, err := lookupGroupId(*socketGroup); err == nil {
			if err := os.Chown(dir, -1, gid); err != nil {
				fmt.Fprintf(os.Stderr, "frr-launcher daemon: chown %s: %v\n", dir, err)
			}
		}
	}

	// Remove a stale socket from a previous run, if any.
	// This is safe because the daemon is single-instance;
	// systemd's Restart=on-failure means at most one
	// instance is alive at a time.
	if err := os.Remove(*listenAddr); err != nil && !os.IsNotExist(err) {
		die("frr-launcher daemon: remove stale socket: %v", err)
	}

	ln, err := net.Listen("unix", *listenAddr)
	if err != nil {
		die("frr-launcher daemon: listen %s: %v", *listenAddr, err)
	}
	defer ln.Close()
	defer os.Remove(*listenAddr)

	mode, err := strconv.ParseUint(*socketMode, 8, 32)
	if err != nil {
		die("frr-launcher daemon: parse socket-mode: %v", err)
	}
	if err := os.Chmod(*listenAddr, os.FileMode(mode)); err != nil {
		die("frr-launcher daemon: chmod socket: %v", err)
	}
	if gid, err := lookupGroupId(*socketGroup); err == nil {
		if err := os.Chown(*listenAddr, -1, gid); err != nil {
			// Non-fatal: a chmod 0660 socket owned
			// root:root still works for the agent,
			// which also runs as root.
			fmt.Fprintf(os.Stderr, "frr-launcher daemon: chown socket: %v\n", err)
		}
	}

	fmt.Fprintf(os.Stderr, "frr-launcher daemon: listening on %s uid=%d gid=%d\n", *listenAddr, uid, gid)

	// Single concurrent connection is enough — the agent
	// dispatches one FRR start at a time per pathspace,
	// and the daemon's job is narrow. Spawning a goroutine
	// per connection would let an unprivileged caller
	// monopolise the daemon, which we want to avoid.
	for {
		conn, err := ln.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			fmt.Fprintf(os.Stderr, "frr-launcher daemon: accept: %v\n", err)
			continue
		}
		handleConn(conn, uid, gid)
	}
}

// handleConn reads a single line-based command from c,
// dispatches it, and writes a single line-based reply.
// Closing the connection ends the session.
func handleConn(c net.Conn, frrUid, frrGid int) {
	defer c.Close()
	scanner := bufio.NewScanner(c)
	scanner.Buffer(make([]byte, 1<<16), 1<<20)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		switch parts[0] {
		case "PING":
			fmt.Fprintln(c, "PONG")
		case "EXEC":
			pid, err := daemonEXEC(parts[1:], frrUid, frrGid)
			if err != nil {
				fmt.Fprintf(c, "ERR %s\n", err)
				continue
			}
			fmt.Fprintf(c, "OK %d\n", pid)
		case "STOP":
			if err := daemonSTOP(parts[1:]); err != nil {
				fmt.Fprintf(c, "ERR %s\n", err)
				continue
			}
			fmt.Fprintln(c, "OK")
		case "LIST":
			out := formatList(daemonTracker().Snapshot())
			if _, err := c.Write(out); err != nil {
				fmt.Fprintf(os.Stderr, "frr-launcher: LIST write: %v\n", err)
			}
		default:
			fmt.Fprintf(c, "ERR unknown command %q\n", parts[0])
		}
	}
}

// daemonTracker is the launcher's process-global
// tracker, installed by runDaemon. handleConn reaches
// it through this getter so the tracker state is shared
// across every accept-loop connection. We avoid a
// package-level closure-on-a-var pattern and use a plain
// pointer instead.
var globalTracker *Tracker

// installTracker wires the daemon's tracker pointer.
// Used by runDaemon and by the tracker_test.go
// fixtures that exercise the EXEC / STOP / LIST
// dispatch without standing up a real daemon.
func installTracker(t *Tracker) { globalTracker = t }

func daemonTracker() *Tracker {
	if globalTracker == nil {
		panic("frr-launcher: tracker not installed (runDaemon not called)")
	}
	return globalTracker
}

// reapAll identity-gated reaps every tracked entry. Used
// by SIGTERM/SIGINT cleanup. Errors are logged and
// ignored: we want to attempt every kill, not to bail on
// the first identity mismatch (which is informational, not
// a fault).
func reapAll(tr *Tracker) {
	for _, e := range tr.Snapshot() {
		if err := terminateAndReap(e.PID, e.Binary, e.Pathspace); err != nil {
			fmt.Fprintf(os.Stderr, "frr-launcher daemon: reap %s/%s pid=%d: %v\n", e.Pathspace, e.Binary, e.PID, err)
		}
		// Remove the stale pidfile if FRR did not
		// remove it itself. Best-effort.
		if e.PidFile != "" {
			_ = os.Remove(e.PidFile)
		}
		tr.Forget(e.Pathspace, e.Binary)
	}
}

// ssigname returns the canonical name of sig for
// logging. Used by the SIGTERM/SIGINT handler.
func ssigname(sig os.Signal) string {
	switch sig {
	case syscall.SIGTERM:
		return "SIGTERM"
	case syscall.SIGINT:
		return "SIGINT"
	}
	return sig.String()
}

// daemonEXEC is the idempotent entry point for the EXEC
// command. It reconciles the tracker with the requested
// (binary, pathspace, sockDir):
//
//   1. If the tracker already has an entry for the
//      (pathspace, binary) AND the PID is still alive AND
//      the identity matches /usr/lib/frr/<bin> with -N
//      <pathspace>, return the existing PID. The runtime's
//      subsequent pidfile poll will confirm; no fork happens.
//
//   2. If the tracker has a stale entry (PID exited,
//      wrong identity, or wrong pathspace), reap it
//      identity-gated. A stale PID that does NOT match
//      the recorded (binary, pathspace) is left alone —
//      a recycled PID may be a legitimate unrelated
//      process.
//
//   3. Else, fork+exec the re-exec launcher, wait for
//      FRR to write its pidfile, verify identity, and
//      track. Return the daemon PID.
//
// The wire protocol reply is `OK <pid>\n` where pid is
// the FRR daemon's PID (NOT the re-exec launcher PID).
// The runtime reads the pidfile to find this PID too;
// we return the same number from the daemon so the wire
// contract is consistent regardless of which side the
// runtime reads first.
func daemonEXEC(args []string, frrUid, frrGid int) (int, error) {
	if len(args) < 5 {
		return 0, fmt.Errorf("usage: EXEC <binary> <pathspace> <config> <log> <pidfile> [args...]")
	}
	binary, pathspace, conf, logPath, pidFile := args[0], args[1], args[2], args[3], args[4]
	passthrough := args[5:]
	tr := daemonTracker()

	// Idempotent path: existing tracked entry verified alive.
	if existing := tr.Lookup(pathspace, binary); existing != nil {
		if err := isFRRProcess(existing.PID, binary, pathspace); err == nil {
			// Refresh startedAt so the runtime's
			// state reflects "still alive". Optional.
			tr.Put(&TrackedFRR{
				Binary:    existing.Binary,
				Pathspace: existing.Pathspace,
				PID:       existing.PID,
				StartedAt: existing.StartedAt,
				PidFile:   existing.PidFile,
				SockDir:   existing.SockDir,
				Cmdline:   existing.Cmdline,
			})
			return existing.PID, nil
		}
		// Stale. Reap identity-gated (refuses to kill
		// a recycled PID), then forget.
		if err := terminateAndReap(existing.PID, binary, pathspace); err != nil {
			fmt.Fprintf(os.Stderr, "frr-launcher daemon: stale %s/%s pid=%d: %v (continuing)\n", pathspace, binary, existing.PID, err)
		}
		if existing.PidFile != "" {
			_ = os.Remove(existing.PidFile)
		}
		tr.Forget(pathspace, binary)
	}

	// Cold path: fork+exec the re-exec launcher, then
	// wait for FRR to write its pidfile.
	if _, err := execFRR(args, frrUid, frrGid); err != nil {
		return 0, err
	}
	// Poll the pidfile for up to 5s (FRR is fast; the
	// daemonize() double-fork + privs_init takes well
	// under 1s on healthy hardware).
	deadline := time.Now().Add(5 * time.Second)
	var pid int
	for time.Now().Before(deadline) {
		pid = readPIDFile(pidFile)
		if pid > 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if pid <= 0 {
		return 0, fmt.Errorf("EXEC %s/%s: pidfile %s not written within 5s", pathspace, binary, pidFile)
	}
	// Identity gate. The PID written to the pidfile
	// MUST be /usr/lib/frr/<bin> with -N <pathspace>.
	// A pidfile containing the wrong PID (e.g. the
	// re-exec launcher that exited immediately) is
	// rejected here, not at kill time.
	if err := isFRRProcess(pid, binary, pathspace); err != nil {
		return 0, fmt.Errorf("EXEC %s/%s: pid %d from pidfile not FRR: %v", pathspace, binary, pid, err)
	}
	// Track.
	sockDir := filepath.Dir(pidFile)
	tr.Put(&TrackedFRR{
		Binary:    binary,
		Pathspace: pathspace,
		PID:       pid,
		StartedAt: time.Now().UTC(),
		PidFile:   pidFile,
		SockDir:   sockDir,
		Cmdline:   "/usr/lib/frr/" + binary,
	})
	_ = conf
	_ = logPath
	_ = passthrough
	return pid, nil
}

// daemonSTOP handles "STOP <pathspace>" and
// "STOP <pathspace> <binary>". Every kill is
// identity-gated via terminateAndReap; a recycled PID
// that does NOT match (binary, pathspace) is left alone.
//
// Returns:
//   - nil if no tracked entry exists for the request
//     (STOP is idempotent: it succeeds vacuously)
//   - nil after a successful reap
//   - non-nil only on a transient error (e.g. a kill
//     permission failure that isn't ESRCH)
func daemonSTOP(args []string) error {
	pathspace, binSpec, ok := parseSTOPArg(args)
	if !ok {
		return fmt.Errorf("usage: STOP <pathspace> [binary]")
	}
	if pathspace == "" {
		return fmt.Errorf("STOP: empty pathspace")
	}
	tr := daemonTracker()
	var entries []*TrackedFRR
	if binSpec != "" {
		if e := tr.Lookup(pathspace, binSpec); e != nil {
			entries = append(entries, e)
		}
	} else {
		// Pathspace.
		for _, e := range tr.Snapshot() {
			if e.Pathspace == pathspace {
				entries = append(entries, e)
			}
		}
	}
	if len(entries) == 0 {
		// Idempotent.
		return nil
	}
	for _, e := range entries {
		if err := terminateAndReap(e.PID, e.Binary, e.Pathspace); err != nil {
			fmt.Fprintf(os.Stderr, "frr-launcher daemon: STOP %s/%s pid=%d: %v (continuing)\n", e.Pathspace, e.Binary, e.PID, err)
		}
		// Best-effort pidfile cleanup.
		if e.PidFile != "" {
			_ = os.Remove(e.PidFile)
		}
		tr.Forget(e.Pathspace, e.Binary)
	}
	return nil
}

// execFRR is the privileged-broker half of an EXEC
// command. It parses the args, then in a forked child
// performs the privilege drop + cap narrowing + exec of
// /usr/lib/frr/<binary>. The parent returns the child's
// PID. If the parent fails before fork, it returns an
// error.
func execFRR(args []string, frrUid, frrGid int) (int, error) {
	// EXEC <binary> <pathspace> <config> <log> <pidfile> [args...]
	if len(args) < 5 {
		return 0, fmt.Errorf("usage: EXEC <binary> <pathspace> <config> <log> <pidfile> [args...]")
	}
	binary, pathspace, conf, logPath, pidFile := args[0], args[1], args[2], args[3], args[4]
	passthrough := args[5:]

	binPath := "/usr/lib/frr/" + binary
	if _, err := os.Stat(binPath); err != nil {
		return 0, fmt.Errorf("stat %s: %w", binPath, err)
	}

	// Ensure the parent dir for the FRR log file
	// exists. The re-exec'd child (running as uid=frr
	// after its privilege drop) creates the log file
	// itself with O_CREATE and owns it as frr:frr.
	// Pre-creating the file here as root:duckos was the
	// cause of an EACCES loop: uid=frr is not in the
	// duckos supplementary group, so an O_WRONLY open
	// of a root:duckos 0664 file as uid=frr hits the
	// kernel "not in group with write bit" rejection.
	// Letting the re-exec'd child create the file as
	// frr:frr 0664 sidesteps the ownership mismatch.
	if logPath != "" {
		if err := os.MkdirAll(filepath.Dir(logPath), 0o775); err != nil {
			return 0, fmt.Errorf("mkdir %s: %w", filepath.Dir(logPath), err)
		}
	}
	// Pidfile: the re-exec'd child (uid=frr) creates
	// this lazily with O_CREATE as frr:frr; we do NOT
	// pre-stage the pidfile here because that would
	// leave it root:duckos 0644 and FRR (uid=frr) would
	// fail to write its PID into it.
	_ = pidFile

	// The fork happens BEFORE the privilege drop, so
	// the parent remains uid=0 with full caps — it
	// owns the spawned FRR child and returns its PID
	// to the agent. The child performs the drop and
	// execs FRR.
	type forkResult struct {
		pid  int
		err  error
	}
	resCh := make(chan forkResult, 1)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		// We use exec.Command with SysProcAttr to
		// fork a controlled child. The parent's
		// caps stay intact; the child runs the
		// dropAndExec goroutine.
		// We use a pipe to ferry the drop-result
		// from child to parent.
		//
		// Implementation note: we fork via
		// syscall.ForkExec-style code (raw fork +
		// exec in the child) instead of
		// exec.Command.Start, because we need a
		// long-running intermediate process that
		// does the privilege drop and then execs
		// the FRR binary in the same address
		// space. exec.Command would exec the FRR
		// binary directly without our drop.
		//
		// Simpler alternative: this launcher
		// re-execs itself with --exec-binary
		// <bin> <ps> <conf> <log> <pid> [args]
		// and the re-exec'd process performs the
		// drop and execs FRR. That avoids the
		// raw fork+exec plumbing.
		//
		// We do the re-exec.
		selfPath, err := os.Executable()
		if err != nil {
			resCh <- forkResult{0, err}
			return
		}
		r, w, err := os.Pipe()
		if err != nil {
			resCh <- forkResult{0, err}
			return
		}
		fd0 := r.Fd()

		argv := []string{selfPath,
			"--exec-binary", binary,
			"--exec-pathspace", pathspace,
			"--exec-config", conf,
			"--exec-log", logPath,
			"--exec-pidfile", pidFile,
			"--exec-frr-uid", strconv.Itoa(frrUid),
			"--exec-frr-gid", strconv.Itoa(frrGid),
		}
		argv = append(argv, "--")
		argv = append(argv, passthrough...)

		envv := []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin"}
		// Pass nil for stderr so the re-exec'd child's
// os.Stderr writes (die(), log lines from the
// re-exec'd launcher's drop time) land in the
// daemon's stderr (journal) instead of the FRR
// log file that gets cleaned up by the runtime on
// failure. w is the pipe write-end used by the
// re-exec'd child to ack its drop result; the
// daemon ignores it for now.
pid, err := forkExecWithFds(selfPath, argv, envv, w, nil)
		_ = w.Close()
		if err != nil {
			resCh <- forkResult{0, err}
			return
		}
		// Read the drop-result from child. The
		// child writes one line before it exec's
		// FRR; if the drop fails, the line is
		// "ERR <msg>\n", otherwise it's "OK\n".
		_ = fd0
		// The pipe is non-blocking by default;
		// the child writes before exec'ing FRR.
		// We don't strictly need to read this in
		// the daemon — we just need the PID —
		// but reading avoids leaking fds.
		// Skipping the read keeps the parent
		// fast.
		resCh <- forkResult{pid, nil}
	}()
	wg.Wait()
	close(resCh)
	res := <-resCh
	return res.pid, res.err
}

// runOneShot is the legacy single-shot mode. It is
// retained so the launcher binary can be invoked
// directly in test or recovery contexts, but the
// production runtime uses --daemon.
func runOneShot(args []string) {
	fs := flag.NewFlagSet("duck-frr-launcher", flag.ExitOnError)
	pathspace := fs.String("pathspace", "", "FRR logical namespace name (-N flag); required")
	frrBinary := fs.String("binary", "zebra", "FRR binary: zebra or bgpd")
	configPath := fs.String("config", "", "frr.conf path (default /var/lib/frr/<ps>/frr.conf)")
	zServAddr := fs.String("zebra-addr", "127.0.0.1", "zserv API bind address (-A)")
	logPath := fs.String("log", "", "redirect stderr to this file; empty = inherit")
	// The fork-and-re-exec path passes these flags
	// back into a fresh launcher process; they MUST be
	// registered with the re-exec's FlagSet so
	// fs.Parse accepts them. (The package-level
	// frrUidFlag / frrGidFlag below are registered
	// with the global flag.CommandLine and would NOT
	// be visible to this FlagSet.)
	execBinary := fs.String("exec-binary", "", "internal: re-exec entry for fork-time drop")
	execPathspace := fs.String("exec-pathspace", "", "internal: re-exec pathspace")
	execConfig := fs.String("exec-config", "", "internal: re-exec config path")
	execLog := fs.String("exec-log", "", "internal: re-exec log path")
	execPidFile := fs.String("exec-pidfile", "", "internal: re-exec pid file path")
	// Bind package-level uid/gid flags to the same
	// backing variables that fs populates below so
	// dropAndExecFRR sees the values the daemon passed
	// in via --exec-frr-uid / --exec-frr-gid.
	var execFrrUid int
	var execFrrGid int
	fs.IntVar(&execFrrUid, "exec-frr-uid", 0, "internal: re-exec frr uid")
	fs.IntVar(&execFrrGid, "exec-frr-gid", 0, "internal: re-exec frr gid")
	if err := fs.Parse(args); err != nil {
		die("frr-launcher: %v", err)
	}
	// Bridge the re-exec-passed uid/gid into the
	// package-level vars dropAndExecFRR reads.
	*frrUidFlag = execFrrUid
	*frrGidFlag = execFrrGid

	// Re-exec'd child: do the drop and exec the FRR
	// binary. The agent never invokes this mode
	// directly; only the daemon's fork reaches here.
	if *execBinary != "" {
		dropAndExecFRR(*execBinary, *execPathspace, *execConfig, *execLog, *execPidFile, fs.Args())
		return
	}

	// Legacy single-shot (--pathspace=... --binary=...
	// --config=... --log=... [passthrough]). Kept for
	// tests; not used by the runtime.
	if *pathspace == "" {
		die("frr-launcher: --pathspace is required (or use --daemon)")
	}
	if *configPath == "" {
		*configPath = "/var/lib/frr/" + *pathspace + "/frr.conf"
	}
	uid, gid, err := lookupFrrIds()
	if err != nil {
		die("frr-launcher: %v", err)
	}
	execOneShot(*frrBinary, *pathspace, *configPath, *zServAddr, *logPath, uid, gid, fs.Args())
}

// execOneShot is the legacy single-shot drop+exec path.
// It performs the same drop-and-narrow sequence as the
// re-exec'd child but does so in-process (no fork).
// Used by tests and the legacy --pathspace=... mode.
func execOneShot(binary, pathspace, conf, zServAddr, logPath string, uid, gid int, passthrough []string) {
	if err := dropPrivilegesAndCap(uid, gid, pathspace); err != nil {
		die("frr-launcher: %v", err)
	}
	binPath := "/usr/lib/frr/" + binary
	args := []string{binPath, "-d",
		"-f", conf,
		"-A", zServAddr,
		"-N", pathspace,
		"-u", frrRunUser,
		"-g", frrRunGroup,
	}
	args = append(args, passthrough...)
	cmd := exec.Command(binPath, args[1:]...)
	cmd.Stdin = nil
	cmd.Stdout = nil
	if logPath != "" {
		f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o664)
		if err != nil {
			die("frr-launcher: open log %s: %v", logPath, err)
		}
		defer f.Close()
		cmd.Stderr = f
	} else {
		cmd.Stderr = os.Stderr
	}
	if err := cmd.Run(); err != nil {
		die("frr-launcher: exec %s: %v", binPath, err)
	}
}

// dropAndExecFRR is the re-exec'd child's path: do the
// privilege drop + cap narrow + exec the FRR binary.
func dropAndExecFRR(binary, pathspace, conf, logPath, pidFile string, passthrough []string) {
	uid := *frrUidFlag
	gid := *frrGidFlag
	if uid == 0 || gid == 0 {
		var err error
		uid, gid, err = lookupFrrIds()
		if err != nil {
			die("frr-launcher re-exec: %v", err)
		}
	}
	if err := dropPrivilegesAndCap(uid, gid, pathspace); err != nil {
		die("frr-launcher re-exec: %v", err)
	}
	binPath := "/usr/lib/frr/" + binary
	args := []string{binPath, "-d",
		"-f", conf,
		"-A", "127.0.0.1",
		"-N", pathspace,
		"-u", frrRunUser,
		"-g", frrRunGroup,
	}
	args = append(args, passthrough...)
	cmd := exec.Command(binPath, args[1:]...)
	cmd.Stdin = nil
	cmd.Stdout = nil
	if logPath != "" {
		f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o664)
		if err != nil {
			die("frr-launcher re-exec: open log %s: %v", logPath, err)
		}
		defer f.Close()
		cmd.Stderr = f
	} else {
		cmd.Stderr = os.Stderr
	}
	if err := cmd.Run(); err != nil {
		die("frr-launcher re-exec: exec %s: %v", binPath, err)
	}
}

// dropPrivilegesAndCap is the shared drop sequence. It
// runs in the re-exec'd child (or the legacy one-shot
// process). The caller must already be root with
// CAP_SETUID+CAP_SETGID+CAP_SETPCAP in its bounding set
// (the daemon service unit and the launcher's file caps
// both guarantee this).
//
// Sequence:
//
//   1. PR_SET_KEEPCAPS=1 — preserves P (and E) across
//      the root→frr uid transition.
//   2. PR_CAPBSET_DROP SETUID + SETGID from the bounding
//      set. This MUST run BEFORE setresuid: Linux clears
//      the effective set on uid transition, and
//      PR_CAPBSET_DROP requires CAP_SETPCAP in E. CAP_SETPCAP
//      is INTENTIONALLY retained in Bnd so the FRR child
//      inherits it at exec and can itself call
//      cap_set_proc (Debian FRR 10.3 zebra's privs_init
//      uses cap_set_proc; that call requires CAP_SETPCAP
//      in E).
//   3. setgroups([]), setresgid, setresuid → uid=frr.
//   4. mkdir /etc/frr/<ps>/ + write vtysh.conf as
//      uid=frr; /etc/frr is frr:frr 0751 so the frr
//      user can write.
//   5. Caller exec's /usr/lib/frr/{zebra,bgpd}.
//      The launcher does NOT call cap_set_proc: kernel
//      exec semantics (parent P ∩ parent Bnd, then
//      file_caps OR'd in) hand the FRR child exactly
//      the cap set {NET_ADMIN, NET_RAW, NET_BIND_SERVICE,
//      SYS_ADMIN, SETPCAP}, the file caps installed by
//      install.sh include cap_setpcap so the child's E
//      has CAP_SETPCAP and FRR's own privs_init can
//      narrow further.
func dropPrivilegesAndCap(uid, gid int, pathspace string) error {
	// PR_SET_KEEPCAPS preserves P/E across the uid
	// transition from 0 to a non-zero uid.
	if _, _, errno := unix.Syscall(unix.PR_SET_KEEPCAPS, 1, 0, 0); errno != 0 {
		return fmt.Errorf("PR_SET_KEEPCAPS: %v", errno)
	}

	// Drop launcher-only caps (SETUID, SETGID) from the
	// bounding set BEFORE setresuid. CAP_SETPCAP is
	// intentionally retained so the FRR child inherits
	// it at exec. This is the critical step that was
	// previously failing because PR_CAPBSET_DROP needs
	// CAP_SETPCAP in the calling process's effective set,
	// and the effective set is cleared by setresuid
	// even with KEEPCAPS=1.
	if err := dropCapsFromBounding(launcherOnlyCaps); err != nil {
		return fmt.Errorf("dropCapsFromBounding: %w", err)
	}

	if err := unix.Setgroups([]int{}); err != nil {
		return fmt.Errorf("setgroups: %w", err)
	}
	if err := unix.Setresgid(gid, gid, gid); err != nil {
		return fmt.Errorf("setresgid(%d): %w", gid, err)
	}
	if err := unix.Setresuid(uid, uid, uid); err != nil {
		return fmt.Errorf("setresuid(%d): %w", uid, err)
	}

	// Create /etc/frr/<pathspace>/vtysh.conf as uid=frr.
	// The Agent cannot do this — /etc/frr is frr:frr
	// 0751 (root can traverse, frr user owns and can
	// write its <ps>/). The launcher, now uid=frr,
	// matches the owner and can write.
	etcDir := "/etc/frr/" + pathspace
	if err := os.MkdirAll(etcDir, 0o755); err != nil {
		// Non-fatal: vtysh will warn but the
		// daemons do not depend on vtysh.conf.
		fmt.Fprintf(os.Stderr, "frr-launcher: mkdir %s: %v; vtysh may warn\n", etcDir, err)
	} else {
		vtyshConf := etcDir + "/vtysh.conf"
		if _, err := os.Stat(vtyshConf); os.IsNotExist(err) {
			if err := os.WriteFile(vtyshConf, []byte("!\n"), 0o644); err != nil {
				fmt.Fprintf(os.Stderr, "frr-launcher: write %s: %v; vtysh may warn\n", vtyshConf, err)
			}
		}
	}

	// No applyCaps() here. The launcher deliberately
	// does not narrow P/E further because after setresuid
	// the effective set is cleared even with KEEPCAPS=1,
	// and cap_set_proc would need CAP_SETPCAP in E.
	// Kernel exec semantics hand the FRR child the right
	// cap set without any launcher-level cap_set_proc.
	return nil
}

func die(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}

// lookupFrrIds resolves frr uid / gid via os/user. This
// avoids re-implementing NSS.
func lookupFrrIds() (uid, gid int, err error) {
	u, err := user.Lookup(frrRunUser)
	if err != nil {
		return 0, 0, fmt.Errorf("lookup %s: %w", frrRunUser, err)
	}
	uid, err = strconv.Atoi(u.Uid)
	if err != nil {
		return 0, 0, fmt.Errorf("parse uid %s: %w", u.Uid, err)
	}
	gid, err = strconv.Atoi(u.Gid)
	if err != nil {
		return 0, 0, fmt.Errorf("parse gid %s: %w", u.Gid, err)
	}
	return uid, gid, nil
}

// lookupGroupId resolves a group name to a gid via NSS.
func lookupGroupId(name string) (int, error) {
	g, err := user.LookupGroup(name)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(g.Gid)
}

// applyCaps was previously used to narrow the launcher's
// P/E/I before exec'ing FRR. It is INTENTIONALLY ABSENT
// now: after setresuid the launcher's effective set is
// cleared even with KEEPCAPS=1 (PR_CAPBSET_DROP requires
// CAP_SETPCAP in E, but E is empty post-setresuid). The
// fix is to do the bounding drop BEFORE setresuid and to
// skip post-setresuid cap_set_proc entirely. Kernel exec
// semantics (parent P ∩ parent Bnd, then file_caps OR'd
// in) hand the FRR child exactly the cap set it needs,
// so no launcher-level cap_set_proc is required.

// capMask was previously used by applyCaps to build the
// V3 capset payload. applyCaps is gone (see comment
// above); capMask is no longer referenced and is kept
// only as documentation of how the kernel V3 layout
// (effective0, permitted0, inheritable0, effective1,
// permitted1, inheritable1) maps to the launcher-only
// drop sequence. Go does not flag unused unexported
// functions, so we leave it as a tombstone.

// dropCapsFromBounding removes the named caps from the
// calling process's bounding set via prctl
// PR_CAPBSET_DROP. Requires CAP_SETPCAP in the calling
// process's permitted set.
func dropCapsFromBounding(names []string) error {
	for _, n := range names {
		v, ok := capByName(n)
		if !ok {
			return fmt.Errorf("unknown capability %q", n)
		}
		if _, _, errno := unix.Syscall(unix.PR_CAPBSET_DROP, uintptr(v), 0, 0); errno != 0 {
			return fmt.Errorf("PR_CAPBSET_DROP %s: %v", n, errno)
		}
	}
	return nil
}

// capMask converts a list of capability names to the
// (lo, hi) uint32 pair used by V3 capability headers.
func capMask(names []string) (uint32, uint32, error) {
	var lo, hi uint32
	for _, n := range names {
		v, ok := capByName(n)
		if !ok {
			return 0, 0, fmt.Errorf("unknown capability %q", n)
		}
		if v < 32 {
			lo |= 1 << uint(v)
		} else if v < 64 {
			hi |= 1 << uint(v-32)
		} else {
			return 0, 0, fmt.Errorf("capability %q is beyond caps 64-bit set", n)
		}
	}
	return lo, hi, nil
}

// forkExecWithFds does a controlled fork+exec. The
// child runs argv[0] with argv[1:], envv, and the
// given stdin / stderr fds. The child gets its own
// session (setsid) so the FRR daemon can outlive
// the launcher daemon if the latter restarts. The
// caller (daemon) does NOT wait — the child runs
// independently.
//
// IMPORTANT: Go's syscall.ForkExec maps the Files
// slice directly to child fds 0, 1, 2, ... in order.
// We MUST include entries for fd 1 (stdout) even if
// we just want stderr — otherwise the child's fd 2
// (stderr) is left closed and any os.Stderr writes
// from the re-exec'd child fail silently. We use
// /dev/null for stdout so the re-exec'd launcher
// does not spam the journal with its own debug
// output. stderr carries the operator-relevant
// errors; pass nil to inherit the daemon's stderr
// (journal), or pass a file to redirect.
func forkExecWithFds(argv0 string, argv, envv []string, stdin, stderr *os.File) (int, error) {
	argv0Dir, err := filepath.Abs(argv0)
	if err != nil {
		return 0, err
	}

	null, err := os.Open(os.DevNull)
	if err != nil {
		return 0, fmt.Errorf("open %s: %w", os.DevNull, err)
	}
	defer null.Close()

	// Files[0] = stdin, Files[1] = stdout (devnull),
	// Files[2] = stderr (provided file, or daemon's
	// inherited stderr, or devnull).
	var files []uintptr
	if stdin != nil {
		files = append(files, stdin.Fd())
	} else {
		files = append(files, null.Fd())
	}
	files = append(files, null.Fd()) // stdout
	switch {
	case stderr != nil:
		files = append(files, stderr.Fd())
	case os.Stderr != nil:
		files = append(files, os.Stderr.Fd())
	default:
		files = append(files, null.Fd())
	}

	pid, err := syscall.ForkExec(
		argv0Dir,
		argv,
		&syscall.ProcAttr{
			Env:   envv,
			Files: files,
			Sys: &syscall.SysProcAttr{
				Setsid: true,
			},
		},
	)
	if err != nil {
		return 0, err
	}
	return pid, nil
}

// parseSignal converts a name like "SIGTERM" or "15" to
// a syscall.Signal.
func parseSignal(s string) (syscall.Signal, error) {
	if n, err := strconv.Atoi(s); err == nil {
		return syscall.Signal(n), nil
	}
	upper := strings.ToUpper(s)
	if !strings.HasPrefix(upper, "SIG") {
		upper = "SIG" + upper
	}
	var sig unix.Signal
	for _, candidate := range []string{upper, strings.TrimPrefix(upper, "SIG")} {
		if v := signalByName(candidate); v != 0 {
			sig = v
			break
		}
	}
	if sig == 0 {
		return 0, fmt.Errorf("unknown signal %q", s)
	}
	return syscall.Signal(sig), nil
}

// signalByName is a tiny SIG* -> number map. We only
// enumerate the signals the runtime actually sends.
func signalByName(name string) unix.Signal {
	switch strings.ToUpper(name) {
	case "SIGTERM", "TERM":
		return unix.SIGTERM
	case "SIGKILL", "KILL":
		return unix.SIGKILL
	case "SIGHUP", "HUP":
		return unix.SIGHUP
	case "SIGINT", "INT":
		return unix.SIGINT
	}
	return 0
}

// capByName is the Linux cap name -> number map. Kept
// inline for binary size; values are the canonical
// /usr/include/linux/capability.h definitions.
func capByName(name string) (int, bool) {
	m := map[string]int{
		"chown":            0,
		"dac_override":     1,
		"dac_read_search":  2,
		"fowner":           3,
		"fsetid":           4,
		"kill":             5,
		"setgid":           6,
		"setuid":           7,
		"setpcap":          8,
		"linux_immutable":  9,
		"net_bind_service": 10,
		"net_broadcast":    11,
		"net_admin":        12,
		"net_raw":          13,
		"ipc_lock":         14,
		"ipc_owner":        15,
		"sys_module":       16,
		"sys_rawio":        17,
		"sys_chroot":       18,
		"sys_ptrace":       19,
		"sys_pacct":        20,
		"sys_admin":        21,
		"sys_boot":         22,
		"sys_nice":         23,
		"sys_resource":     24,
		"sys_time":         25,
		"sys_tty_config":   26,
		"mknod":            27,
		"lease":            28,
		"audit_write":      29,
		"audit_control":    30,
		"setfcap":          31,
	}
	v, ok := m[name]
	return v, ok
}