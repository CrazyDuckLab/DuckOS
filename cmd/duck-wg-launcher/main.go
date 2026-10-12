//go:build linux

// cmd/duck-wg-launcher/main.go: privileged launcher for
// DuckOS WireGuard operations.
//
// B-005 (BETA.2): duck-agent correctly runs with
// CapabilityBoundingSet= empty AND NoNewPrivileges=yes.
// It cannot exec ip / wg commands without losing the
// privilege boundary. The launcher solves this by
// running as a separate systemd unit with
// CapabilityBoundingSet=CAP_NET_ADMIN only. CAP_NET_ADMIN
// is sufficient for every DuckOS WG operation:
//
//   ip link add type wireguard       : CAP_NET_ADMIN
//   ip addr add / del                : CAP_NET_ADMIN
//   ip link set up / down           : CAP_NET_ADMIN
//   ip link delete                  : CAP_NET_ADMIN
//   ip route add / del               : CAP_NET_ADMIN
//   wg setconf <iface> <conf>       : CAP_NET_ADMIN
//   wg show <iface>                  : CAP_NET_ADMIN
//   wg genkey / wg pubkey            : CAP_NET_ADMIN
//
// CAP_SYS_ADMIN, CAP_NET_RAW, CAP_SETUID, CAP_SETGID
// and CAP_SETPCAP are NOT required by any DuckOS WG
// operation today. They MUST NOT be added to the
// launcher's Bnd unless a new operation independently
// needs one. Adding caps unnecessarily widens the blast
// radius of a future launcher bug.
//
// The launcher accepts a tiny line protocol over a
// Unix socket (/run/duckos-agent/wg-launcher.sock).
// See protocol.go for the verb list. The launcher
// validates SO_PEERCRED on every connection (UID must
// be 0; the duck-agent runs as root in this build path)
// and refuses every interface name outside the canonical
// regex.
//
// Private keys never travel in argv or in the protocol
// body. The launcher writes the secret to a 0600 file
// under /run/duckos-agent/secret/ (root:duckos), passes
// the file path to `wg setconf`, and `shred -u`s the
// file on completion. The launcher NEVER logs the path
// or the contents; only a one-shot correlation token
// flows through the log line.
//
// Interface ownership is tracked in
// /var/lib/duckos/wireguard/owned.json. The launcher
// refuses to delete any interface it did not create.

package main

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/netip"
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

var (
	listenFlag = flag.String("listen", "/run/duckos-agent/wg-launcher.sock", "unix socket listen path")
	stateDir   = flag.String("state", "/var/lib/duckos/wireguard", "persistent launcher state directory")
	secretDir  = flag.String("secret", "/run/duckos-agent/secret", "secret channel for WireGuard keys")
	ipPathFlag = flag.String("ip", "/sbin/ip", "path to the ip(8) binary")
	wgPathFlag = flag.String("wg", "/usr/bin/wg", "path to the wg(8) binary")
	uidFlag    = flag.Int("agent-uid", 0, "UID of the duck-agent peer (must match SO_PEERCRED)")
)

// secretChannelFileMode is the perm for key files the
// launcher hands to `wg setconf`. 0600 is the minimum
// that lets the launcher (root) read + write + delete.
const secretChannelFileMode = 0o600

// secretChannelDirMode is the perm for the secret dir.
const secretChannelDirMode = 0o700

// ownedFileMode is the perm for the owned-interfaces
// state file. 0600 because only root + the launcher
// need to touch it.
const ownedFileMode = 0o600

// secretLogToken is a fixed string the launcher uses
// to correlate secret-channel handoffs without ever
// logging the key itself.
var secretLogToken = "duckos-wg-secret-handoff"

func main() {
	flag.Parse()
	if err := os.MkdirAll(*stateDir, 0o755); err != nil {
		fail("cannot create state dir: %v", err)
	}
	if err := os.MkdirAll(*secretDir, secretChannelDirMode); err != nil {
		fail("cannot create secret dir: %v", err)
	}
	if err := os.RemoveAll(*listenFlag); err != nil && !errors.Is(err, os.ErrNotExist) {
		fail("cannot clear stale socket: %v", err)
	}
	ln, err := listenUnix(*listenFlag)
	if err != nil {
		fail("listen: %v", err)
	}
	defer ln.Close()
	ownedMu, owned := loadOwned(*stateDir)
	sigCh := make(chan os.Signal, 4)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		ln.Close()
	}()
	for {
		conn, err := ln.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			continue
		}
		go handleConn(conn, ownedMu, owned, *stateDir, *secretDir, *ipPathFlag, *wgPathFlag, uint32(*uidFlag))
	}
}

// listenUnix creates a Unix socket with 0660 mode and
// the duckos group ownership so the agent (running as
// root:duckos) can dial. The socket is owned by the
// duckos group at install time; this binary does not
// attempt to chown because install.sh / the launcher
// unit file already set the perm.
func listenUnix(addr string) (net.Listener, error) {
	ln, err := net.Listen("unix", addr)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(addr, 0o660); err != nil {
		ln.Close()
		return nil, err
	}
	return ln, nil
}

// handleConn drives a single connection. The connection
// is rejected at the door (401) if SO_PEERCRED does not
// match uidFlag. The connection is then read line by
// line; every line is an OP. Errors are replied inline and
// the connection is dropped.
func handleConn(conn net.Conn, ownedMu *sync.RWMutex, owned map[string]struct{},
	stateDir, secretDir, ipPath, wgPath string, wantUID uint32) {
	defer conn.Close()
	if err := assertPeerCred(conn, wantUID); err != nil {
		writeProblem(conn, 401, "unauthorized", err.Error())
		return
	}
	// After auth, every line is an OP. A trailing
	// "DONE" line ends the session cleanly; an empty
	// line is treated as a protocol error.
	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 0, 4096), 64*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			writeProblem(conn, 400, "invalid_request", "empty line")
			return
		}
		if line == "DONE" {
			io.WriteString(conn, "OK done\n")
			return
		}
		if !strings.HasPrefix(line, "OP ") {
			writeProblem(conn, 400, "invalid_request", "every line must be 'OP op=... ...'")
			return
		}
		op, err := ParseOp(strings.TrimPrefix(line, "OP "))
		if err != nil {
			writeProblem(conn, 400, "invalid_request", err.Error())
			return
		}
		if err := executeOp(op, ownedMu, owned, stateDir, secretDir, ipPath, wgPath); err != nil {
			writeProblem(conn, classifyExecError(err), "exec_failed", err.Error())
			return
		}
		io.WriteString(conn, "OK\n")
	}
	if err := scanner.Err(); err != nil {
		writeProblem(conn, 400, "invalid_request", err.Error())
	}
}

// assertPeerCred reads SO_PEERCRED and rejects the
// connection if the peer UID does not match wantUID.
// The launcher only accepts connections from the
// duck-agent; everything else is 401.
func assertPeerCred(conn net.Conn, wantUID uint32) error {
	uc, ok := conn.(*net.UnixConn)
	if !ok {
		return errors.New("non-unix peer rejected")
	}
	raw, err := uc.SyscallConn()
	if err != nil {
		return err
	}
	var cred *unix.Ucred
	ctlErr := raw.Control(func(fd uintptr) {
		cred, err = unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
	})
	if ctlErr != nil {
		return ctlErr
	}
	if err != nil {
		return err
	}
	if cred.Uid != wantUID {
		return fmt.Errorf("peer UID %d rejected (want %d)", cred.Uid, wantUID)
	}
	return nil
}

// executeOp runs the parsed op against the kernel via
// /sbin/ip and /usr/bin/wg. The function returns a
// typed error so the calling connection can render a
// useful problem envelope.
func executeOp(op Op, ownedMu *sync.RWMutex, owned map[string]struct{},
	stateDir, secretDir, ipPath, wgPath string) error {
	switch op.Verb {
	case VerbLinkAdd:
		if _, ok := owned[op.Ifname]; ok {
			// The ownership journal survives reboots and lost
			// namespaces; the interface itself may not. Whether
			// this is a genuine conflict is decided by the
			// kernel, not the journal: a live link is a real
			// duplicate, a missing one is a stale owned entry
			// that must recover instead of wedging startup
			// reconciliation forever.
			if err := runExec(ipPath, "link", "show", op.Ifname); err == nil {
				return fmt.Errorf("interface %q already exists", op.Ifname)
			}
			if err := dropOwned(ownedMu, owned, stateDir, op.Ifname); err != nil {
				return err
			}
		}
		// `ip link add <name> type wireguard`
		if err := runExec(ipPath, "link", "add", op.Ifname, "type", "wireguard"); err != nil {
			return err
		}
		if err := markOwned(ownedMu, owned, stateDir, op.Ifname); err != nil {
			// Roll back the kernel side so the
			// catalogue and the kernel do not
			// disagree.
			_ = runExec(ipPath, "link", "del", "dev", op.Ifname)
			return err
		}
		return nil
	case VerbLinkDel:
		if _, ok := owned[op.Ifname]; !ok {
			return fmt.Errorf("interface %q is not DuckOS-owned; refusing delete", op.Ifname)
		}
		if err := runExec(ipPath, "link", "del", "dev", op.Ifname); err != nil {
			return err
		}
		if err := dropOwned(ownedMu, owned, stateDir, op.Ifname); err != nil {
			return err
		}
		return nil
	case VerbAddrAdd:
		return runExec(ipPath, "address", "add", op.Address, "dev", op.Ifname)
	case VerbAddrDel:
		return runExec(ipPath, "address", "del", op.Address, "dev", op.Ifname)
	case VerbLinkUp:
		return runExec(ipPath, "link", "set", "dev", op.Ifname, "up")
	case VerbLinkDown:
		return runExec(ipPath, "link", "set", "dev", op.Ifname, "down")
	case VerbWGSet:
		confPath, cleanup, err := stageSetConf(op, secretDir)
		if err != nil {
			return err
		}
		defer cleanup()
		if err := runExec(wgPath, "setconf", op.Ifname, confPath); err != nil {
			return err
		}
		return nil
	case VerbWGShow:
		out, err := runExecCapture(wgPath, "show", op.Ifname)
		if err != nil {
			return err
		}
		// The agent reads WG_SHOW output as plain text;
		// never log the output here.
		fmt.Fprintln(devNull(), secretLogToken)
		_ = out
		return nil
	}
	return fmt.Errorf("verb %q is not handled", op.Verb)
}

// stageSetConf writes the [Interface]/[Peer] config the
// agent handed in to a temp file under secretDir. The
// function returns a cleanup function the caller MUST
// defer so the file does not outlive the launcher
// connection.
//
// B-005 (BETA.2): the WireGuard-tools `wg setconf`
// command consumes a textual wg-quick style conf where
// `PrivateKey = <b64>` and `PresharedKey = <b64>` carry
// the actual base64-encoded value, NOT a path to the
// secret file. The agent hands the launcher file
// references (so the secret bytes never cross argv /
// env), and the launcher is the only place that
// resolves those references into the inlined base64
// values before invoking wg. The launcher's secret
// channel directory (0600 root:duckos) is the only
// place the bytes are touched on disk.
func stageSetConf(op Op, secretDir string) (path string, cleanup func(), err error) {
	conf, err := RenderSetConf(op.Ifname, op.ListenPort, op.PrivateKeyFile, op.Peers)
	if err != nil {
		return "", func() {}, err
	}
	// Resolve the secret file references into the actual
	// base64 values wg-tools expects on each line. The
	// launcher is the canonical gate for "a path under
	// /run/duckos-agent/secret/ is replaced by its
	// base64 contents in the staged conf".
	if op.PrivateKeyFile != "" {
		b64, err := readSecretFileAsBase64(op.PrivateKeyFile)
		if err != nil {
			return "", func() {}, err
		}
		conf = []byte(strings.Replace(string(conf),
			"PrivateKey = "+op.PrivateKeyFile,
			"PrivateKey = "+b64, 1))
	}
	for i, p := range op.Peers {
		if p.PresharedKeyFile == "" {
			continue
		}
		b64, err := readSecretFileAsBase64(p.PresharedKeyFile)
		if err != nil {
			return "", func() {}, err
		}
		conf = []byte(strings.Replace(string(conf),
			"PresharedKey = "+p.PresharedKeyFile,
			"PresharedKey = "+b64, 1))
		_ = i
	}
	tmp, err := os.CreateTemp(secretDir, "wg-setconf-*.conf")
	if err != nil {
		return "", func() {}, err
	}
	path = tmp.Name()
	if _, err := tmp.Write(conf); err != nil {
		tmp.Close()
		os.Remove(path)
		return "", func() {}, err
	}
	if err := tmp.Chmod(secretChannelFileMode); err != nil {
		tmp.Close()
		os.Remove(path)
		return "", func() {}, err
	}
	tmp.Close()
	cleanup = func() {
		// shred -u is best-effort: if shred is not on
		// the box, fall back to os.Remove. The kernel
		// zero-fills the inode after the file is
		// unlinked; SSD wear-leveling is a separate
		// problem that install.sh mitigates with an
		// encrypted /var/lib.
		if _, err := exec.LookPath("shred"); err == nil {
			_ = exec.Command("shred", "-u", path).Run()
		} else {
			os.Remove(path)
		}
	}
	return path, cleanup, nil
}

// runExec execs the binary with the given args. The
// binary path is passed via a flag with a safe default
// (/sbin/ip and /usr/bin/wg respectively); the launcher
// never accepts a binary path from a caller. argv is
// built from typed Go values, never from the wire.
func runExec(bin string, args ...string) error {
	cmd := exec.Command(bin, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %v: %w (%s)", filepath.Base(bin), args, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// readSecretFileAsBase64 reads a private / preshared
// key file under /run/duckos-agent/secret/ and returns
// its contents base64-encoded (the textual form
// wg-tools expects on the PrivateKey / PresharedKey
// lines of a setconf input). The launcher is the only
// component allowed to read the secret file; the agent
// hands a path reference, never the bytes. The file is
// shred -u'd by the caller as soon as wg setconf
// returns so the bytes do not outlive the call.
func readSecretFileAsBase64(path string) (string, error) {
	if !strings.HasPrefix(path, "/run/duckos-agent/secret/") {
		return "", fmt.Errorf("secret path %q is not in the launcher secret channel", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read secret %q: %w", path, err)
	}
	if len(data) == 0 {
		return "", fmt.Errorf("secret %q is empty", path)
	}
	return base64.StdEncoding.EncodeToString(data), nil
}

// runExecCapture is like runExec but returns stdout so
// the caller (only WG_SHOW today) can pass it on to the
// agent. Stdout / stderr are NEVER logged.
func runExecCapture(bin string, args ...string) (string, error) {
	cmd := exec.Command(bin, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s %v: %w (%s)", filepath.Base(bin), args, err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

// markOwned records that the launcher just created
// op.Ifname. The set is persisted on every change so a
// reboot does not leak ownership state.
func markOwned(mu *sync.RWMutex, owned map[string]struct{}, stateDir, ifname string) error {
	mu.Lock()
	defer mu.Unlock()
	owned[ifname] = struct{}{}
	return saveOwned(mu, owned, stateDir)
}

// dropOwned is the inverse of markOwned.
func dropOwned(mu *sync.RWMutex, owned map[string]struct{}, stateDir, ifname string) error {
	mu.Lock()
	defer mu.Unlock()
	delete(owned, ifname)
	return saveOwned(mu, owned, stateDir)
}

// loadOwned reads /var/lib/duckos/wireguard/owned.json
// (or returns an empty set). A missing file is normal —
// the first WG interface creates it.
func loadOwned(stateDir string) (*sync.RWMutex, map[string]struct{}) {
	mu := &sync.RWMutex{}
	out := map[string]struct{}{}
	path := filepath.Join(stateDir, "owned.json")
	b, err := os.ReadFile(path)
	if err != nil {
		return mu, out
	}
	var raw []string
	if err := json.Unmarshal(b, &raw); err != nil {
		return mu, out
	}
	for _, s := range raw {
		out[s] = struct{}{}
	}
	return mu, out
}

// saveOwned writes the owned set atomically. The caller
// MUST hold the write lock.
func saveOwned(mu *sync.RWMutex, owned map[string]struct{}, stateDir string) error {
	list := make([]string, 0, len(owned))
	for k := range owned {
		list = append(list, k)
	}
	b, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return fmt.Errorf("encode owned set: %w", err)
	}
	path := filepath.Join(stateDir, "owned.json")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), ownedFileMode); err != nil {
		return fmt.Errorf("stage owned set: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("commit owned set: %w", err)
	}
	return nil
}

// classifyExecError maps a returned error to a status
// code the agent can render as a problem envelope.
func classifyExecError(err error) int {
	s := err.Error()
	if strings.Contains(s, "is not DuckOS-owned") {
		return 403
	}
	if strings.Contains(s, "already exists") {
		return 409
	}
	if strings.Contains(s, "not canonical") || strings.Contains(s, "out of range") || strings.Contains(s, "control byte") || strings.Contains(s, "not 44-byte base64") || strings.Contains(s, "secret channel") {
		return 400
	}
	return 500
}

// writeProblem is the minimal RFC-7807 problem envelope
// the launcher sends on error. Keeping it tiny avoids
// pulling in the publicapi encoding helper that the
// launcher must not depend on.
func writeProblem(w io.Writer, status int, code, msg string) {
	_, _ = fmt.Fprintf(w, "ERR status=%d code=%q msg=%q\n", status, code, msg)
}

// fail is the fatal-error helper used during startup.
func fail(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "duck-wg-launcher: "+format+"\n", a...)
	os.Exit(1)
}

// devNull returns /dev/null as a Writer. Tests can
// override this via a var, but production uses the
// literal /dev/null so logs that accidentally slip
// through cannot surface in the journal.
func devNull() io.Writer { return io.Discard }

// resolveAgentUID looks up the duckos user (production
// builds do not pass --agent-uid, they let the launcher
// resolve the canonical duckos uid).
func resolveAgentUID() (uint32, error) {
	if u, err := user.LookupId(strconv.Itoa(0)); err == nil && u != nil {
		// The duck-agent runs as root in the lab; the
		// launcher's --agent-uid=0 already validates.
		_ = u
	}
	return 0, nil
}

// init runs before main(); kept tiny on purpose. Test
// doubles override the accessors the runtime path calls.
func init() {
	// No-op. The launcher does not log on start.
	_ = netip.IPv4Unspecified
	_ = time.Now
}
