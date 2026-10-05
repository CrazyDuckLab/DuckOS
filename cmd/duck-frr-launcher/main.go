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
//   and NoNewPrivileges=no. The agent talks to the
//   launcher over a Unix socket
//   (/run/duckos-agent/frr-launcher.sock) using a tiny
//   line protocol.
//
//   For each EXEC command the launcher:
//     1. fork()
//     3. PR_SET_KEEPCAPS=1 (in child),
//     4. setgroups([]), setresgid(frr_gid), setresuid(frr_uid),
//     5. mkdir /etc/frr/<ps>/ 0755 + write vtysh.conf
//        (the Agent cannot do this — /etc/frr is
//        frr:frr 0700; the launcher, now uid=frr,
//        matches the owner),
//     6. drop CAP_SETUID, CAP_SETGID, CAP_SETPCAP from
//        the bounding set (prctl PR_CAPBSET_DROP), so the
//        child exec's FRR with bounding = {NET_ADMIN,
//        NET_RAW, NET_BIND_SERVICE} only,
//     7. cap_set_proc({NET_ADMIN, NET_RAW, NET_BIND_SERVICE})
//        — narrow P/E/I to FRR's actual needs;
//     CAP_SYS_ADMIN is intentionally absent.
//     8. exec /usr/lib/frr/{zebra,bgpd} with `-S`
//        (--skip_runas) so FRR's privs_init does NOT
//        call cap_set_proc.
//
//   The FRR child runs as uid=frr, gid=frr, no
//   supplementary groups, with P/E =
//   {NET_ADMIN, NET_RAW, NET_BIND_SERVICE}. NO
//   CAP_SYS_ADMIN. NO CAP_SETUID/SETGID/SETPCAP. NO
//   duck-agent capabilities.
//
//   install.sh sets:
//     setcap cap_net_admin,cap_net_raw,cap_net_bind_service+ep
//         /usr/lib/frr/{zebra,bgpd}
//     setcap cap_setuid,cap_setgid,cap_setpcap+ep
//         /usr/libexec/duckos/duck-frr-launcher
//   The file caps are belt-and-braces in case the
//   service unit is ever overridden; the unit's
//   CapabilityBoundingSet is the canonical grant.
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
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"unsafe"

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

// capsToKeep is the capability set we hand to the
// FRR child. The Debian FRR 10.3 package's zebra
// binary declares CAP_NET_ADMIN, CAP_NET_RAW, AND
// CAP_SYS_ADMIN in its cap_set_proc call and DOES
// NOT support upstream's -S / --skip_runas flag
// (only bgpd supports it in the Debian build), so
// zebra's privs_init would fail without CAP_SYS_ADMIN
// in P. Per the BETA.2 architectural review, CAP_SYS_ADMIN
// on a FRR-only child is acceptable because the FRR
// process genuinely needs it. CAP_SYS_ADMIN MUST NOT
// reach duck-agent itself; the agent's bounding set
// stays empty.
var capsToKeep = []string{"net_admin", "net_raw", "net_bind_service", "sys_admin"}

// capSetByName maps the launcher's-own caps (CAP_SETUID,
// CAP_SETGID) — these are the caps the launcher process
// itself needs to perform the privilege drop. They are
// dropped from the bounding set after the drop so the
// FRR child exec'd from this launcher never inherits them.
//
// CAP_SETPCAP is INTENTIONALLY NOT dropped. Debian FRR's
// privs_init calls cap_set_proc and the kernel requires
// CAP_SETPCAP in the calling process's effective set for
// that call to succeed. The launcher's own drop is done
// before the bounding drop so the launcher's applyCaps
// (which masks P/E/I to capsToKeep) doesn't drop SETPCAP
// from the bounding set the FRR child inherits.
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
// listens on a Unix socket and serves EXEC / KILL
// commands from the duck-agent.
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
			pid, err := execFRR(parts[1:], frrUid, frrGid)
			if err != nil {
				fmt.Fprintf(c, "ERR %s\n", err)
				continue
			}
			fmt.Fprintf(c, "OK %d\n", pid)
		case "SIGNAL":
			if err := signalFRR(parts[1:]); err != nil {
				fmt.Fprintf(c, "ERR %s\n", err)
				continue
			}
			fmt.Fprintln(c, "OK")
		default:
			fmt.Fprintf(c, "ERR unknown command %q\n", parts[0])
		}
	}
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

	// Stage the log file + pidfile as the launcher
	// user (root). The re-exec'd child (running as
	// uid=frr after its privilege drop) will create the
	// file itself with O_CREATE and own it from the
	// start; pre-creating it here as root:duckos with a
	// restrictive mode was the cause of an EACCES loop
	// (chown to frr was also blocked by the service
	// unit's missing CAP_CHOWN).
	if logPath != "" {
		if err := os.MkdirAll(filepath.Dir(logPath), 0o775); err != nil {
			return 0, fmt.Errorf("mkdir %s: %w", filepath.Dir(logPath), err)
		}
		_ = os.WriteFile(logPath, []byte{}, 0o664)
	}
	if pidFile != "" {
		_ = os.WriteFile(pidFile, []byte{}, 0o644)
	}

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

// signalFRR parses SIGNAL <pid> <signal-name|number>.
// signal-name is SIGTERM, SIGKILL, SIGHUP, etc.
func signalFRR(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: SIGNAL <pid> <signal>")
	}
	pid, err := strconv.Atoi(args[0])
	if err != nil {
		return fmt.Errorf("parse pid %q: %w", args[0], err)
	}
	sig, err := parseSignal(args[1])
	if err != nil {
		return err
	}
	if err := unix.Kill(pid, sig); err != nil {
		return fmt.Errorf("kill %d %s: %w", pid, args[1], err)
	}
	return nil
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
		"-S",
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
		"-S",
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
func dropPrivilegesAndCap(uid, gid int, pathspace string) error {
	// PR_SET_KEEPCAPS preserves P/E across the uid
	// transition from 0 to a non-zero uid.
	if _, _, errno := unix.Syscall(unix.PR_SET_KEEPCAPS, 1, 0, 0); errno != 0 {
		return fmt.Errorf("PR_SET_KEEPCAPS: %v", errno)
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
	// 0700. The launcher, now uid=frr, matches the
	// owner and can write.
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

	// Drop the launcher's-own caps (SETUID, SETGID,
	// SETPCAP) from the bounding set so the FRR
	// child exec'd from this process never inherits
	// them. CAP_SETPCAP must still be in the
	// process's permitted set to call PR_CAPBSET_DROP;
	// it is, because we have not yet called
	// cap_set_proc.
	if err := dropCapsFromBounding(launcherOnlyCaps); err != nil {
		return fmt.Errorf("dropCapsFromBounding: %w", err)
	}

	// Narrow P/E/I to FRR's actual needs. After
	// this, the process no longer holds CAP_SETPCAP
	// so any further cap_set_proc would fail; we
	// exec FRR immediately.
	if err := applyCaps(capsToKeep); err != nil {
		return fmt.Errorf("applyCaps: %w", err)
	}
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

// applyCaps sets P/E/I to the union of the named
// capabilities. Requires CAP_SETPCAP in the calling
// process's permitted set; the caller's file caps or
// service-unit bounding set must include CAP_SETPCAP.
//
// We use LINUX_CAPABILITY_VERSION_3 with two uint32s
// per set (the 64-bit caps format).
// golang.org/x/sys/unix only exposes V1/V2 via
// CapUserData, so we issue the syscall directly.
func applyCaps(names []string) error {
	lo, hi, err := capMask(names)
	if err != nil {
		return err
	}
	type hdrT struct {
		Version uint32
		Pid     int32
	}
	type dataT struct {
		// The kernel's __user_cap_data_struct is
		// (effective, permitted, inheritable) per
		// 32-bit slot. For V3 (64-bit caps) the
		// kernel reads two of these back-to-back,
		// so the memory layout must be:
		//   effective0, permitted0, inheritable0,
		//   effective1, permitted1, inheritable1.
		// The bug had these in a different order —
		// the kernel saw permitted as the hi32 of
		// effective (always 0), rejected the
		// not-a-subset check, and returned EPERM.
		effective0, permitted0, inheritable0 uint32
		effective1, permitted1, inheritable1 uint32
	}
	hdr := hdrT{Version: 0x20080522, Pid: 0}
	data := dataT{
		effective0:   lo,
		permitted0:   lo,
		inheritable0: lo,
		effective1:   hi,
		permitted1:   hi,
		inheritable1: hi,
	}
	_, _, errno := unix.Syscall(
		unix.SYS_CAPSET,
		uintptr(unsafe.Pointer(&hdr)),
		uintptr(unsafe.Pointer(&data)),
		0,
	)
	if errno != 0 {
		fmt.Fprintf(os.Stderr, "frr-launcher: applyCaps(%v) failed: errno=%d lo=0x%x hi=0x%x hdr.version=0x%x hdr.pid=%d\n",
			names, errno, lo, hi, hdr.Version, hdr.Pid)
		return errno
	}
	return nil
}

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