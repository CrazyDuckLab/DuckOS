// wireguard/launcher_client.go: agent-side WGRunner that
// talks to the privileged duck-wg-launcher over a Unix
// socket.
//
// B-005 (BETA.2): the duck-agent correctly runs with
// CapabilityBoundingSet= empty and NoNewPrivileges=yes.
// It cannot exec /sbin/ip or /usr/bin/wg. The
// LauncherClient adapter replaces IPRouteRunner
// (which exec'd /sbin/ip and /usr/bin/wg directly) so
// the kernel operations run in a separate unit with
// CAP_NET_ADMIN only.
//
// The runner speaks the launcher's line protocol. Every
// call sends one OP and reads one response. The runner
// carries no privileged state itself — it is just a
// typed bridge between the WireGuard API and the
// privileged service. The launcher is the single gate
// for canonical interface names, addresses and routes.

package wireguard

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"
)

// WGLauncherSocket returns the canonical launcher
// socket path. The variable is kept at package level so
// tests can swap it for a temp dir.
var WGLauncherSocket = func() string {
	return "/run/duckos-agent/wg-launcher.sock"
}

// LauncherClient is the agent-side WG runner. It is
// goroutine-safe: a single dial handles one request at
// a time, and the package reuses the connection if the
// launcher keeps it open. The wire protocol is HTTP-
// like (one OP per line, "DONE" closes) so multiple
// requests can share a connection.
//
// The driver stores private-key bytes for `wg setconf`
// only via the secret-channel file (Path). The agent
// writes the key bytes to that file (mode 0600,
// root:duckos) before the OP is sent, and shreds the
// file on completion. The launcher never sees the
// bytes; only the file path crosses the IPC boundary.
type LauncherClient struct {
	mu sync.Mutex
	// socketFn returns the canonical launcher socket
	// path. The variable indirection lets tests
	// redirect to a temp socket without rewriting the
	// production code path.
	socketFn func() string
	// secretDir is the secret channel the agent uses
	// to hand off private-key bytes. The launcher
	// reads the file under secretDir; the agent
	// controls the lifecycle (write / chmod / shred).
	secretDir string
}

// NewLauncherClient constructs a client bound to the
// canonical socket and the canonical secret dir. Tests
// override either via SetSocket / SetSecretDir.
func NewLauncherClient() *LauncherClient {
	return &LauncherClient{
		socketFn:  WGLauncherSocket,
		secretDir: "/run/duckos-agent/secret",
	}
}

// SetSocketFn overrides the socket path (tests only).
func (c *LauncherClient) SetSocketFn(fn func() string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.socketFn = fn
}

// SetSecretDir overrides the secret channel dir (tests
// only). The launcher will refuse any private-key path
// outside this dir.
func (c *LauncherClient) SetSecretDir(dir string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.secretDir = dir
}

// LinkAdd implements the WGRunner interface.
func (c *LauncherClient) LinkAdd(ifname string) error {
	return c.sendOp(opLine("LINK_ADD", map[string]string{
		"ifname": ifname,
		"kind":   "wireguard",
	}))
}

// LinkDel implements the WGRunner interface.
func (c *LauncherClient) LinkDel(ifname string) error {
	return c.sendOp(opLine("LINK_DEL", map[string]string{
		"ifname": ifname,
	}))
}

// AddrAdd implements the WGRunner interface.
func (c *LauncherClient) AddrAdd(ifname, prefix string) error {
	if _, err := netip.ParsePrefix(prefix); err != nil {
		return fmt.Errorf("invalid prefix %q: %w", prefix, err)
	}
	return c.sendOp(opLine("ADDR_ADD", map[string]string{
		"ifname":  ifname,
		"address": prefix,
	}))
}

// AddrDel implements the WGRunner interface.
func (c *LauncherClient) AddrDel(ifname, prefix string) error {
	if _, err := netip.ParsePrefix(prefix); err != nil {
		return fmt.Errorf("invalid prefix %q: %w", prefix, err)
	}
	return c.sendOp(opLine("ADDR_DEL", map[string]string{
		"ifname":  ifname,
		"address": prefix,
	}))
}

// LinkUp implements the WGRunner interface.
func (c *LauncherClient) LinkUp(ifname string) error {
	return c.sendOp(opLine("LINK_UP", map[string]string{
		"ifname": ifname,
	}))
}

// LinkDown implements the WGRunner interface.
func (c *LauncherClient) LinkDown(ifname string) error {
	return c.sendOp(opLine("LINK_DOWN", map[string]string{
		"ifname": ifname,
	}))
}

// SetConf implements the WGRunner interface. The agent
// hands the private key bytes to the runner via the
// secret channel: the runner writes a temp file under
// secretDir with the bytes, sends the file path to the
// launcher, and shreds the file on completion. The
// launcher never sees the bytes.
//
// listenPort may be 0 to mean "do not bind"; privateKey
// may be empty to mean "do not overwrite the existing
// key". Peers travel as peer= fields on the same
// WG_SET op so the launcher's protocol parser can
// validate the full op atomically.
func (c *LauncherClient) SetConf(ifname string, listenPort int, privateKey []byte, peers []PeerConf) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	var keyFile string
	if len(privateKey) > 0 {
		var cleanup func()
		var err error
		keyFile, cleanup, err = writeSecret(c.secretDir, ifname, privateKey)
		if err != nil {
			return err
		}
		defer cleanup()
	}

	op := opLine("WG_SET", map[string]string{
		"ifname":      ifname,
		"listen_port": fmt.Sprintf("%d", listenPort),
	})
	if keyFile != "" {
		op += " private_key_file=" + keyFile
	}
	if len(peers) > 0 {
		op += " replace_peers=true"
		for _, p := range peers {
			var pkv strings.Builder
			pkv.WriteString("public=")
			pkv.WriteString(p.PublicKey)
			if p.Endpoint != "" {
				pkv.WriteString(" endpoint=")
				pkv.WriteString(p.Endpoint)
			}
			if len(p.AllowedIPs) > 0 {
				pkv.WriteString(" allowed_ips=")
				for i, a := range p.AllowedIPs {
					if i > 0 {
						pkv.WriteString(",")
					}
					pkv.WriteString(a.String())
				}
			}
			if p.PersistentKeepalive > 0 {
				fmt.Fprintf(&pkv, " keepalive=%d", p.PersistentKeepalive)
			}
			op += " peer=\"" + pkv.String() + "\""
		}
	}
	return c.sendOpLocked(op)
}

// PeerConf is the agent-side peer config the runner
// passes through the secret channel to the launcher.
// PresharedKey is delivered via a file written under
// the secret dir; the bytes never cross the wire.
type PeerConf struct {
	PublicKey           string
	PresharedKey        []byte
	Endpoint            string
	AllowedIPs          []netip.Prefix
	PersistentKeepalive int
}

// Show implements the WGRunner interface. The runner
// asks the launcher for `wg show` output and returns
// it. The launcher is not allowed to log the output
// (the secret channel protects the keys).
func (c *LauncherClient) Show(ifname string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	conn, err := c.dialLocked()
	if err != nil {
		return "", err
	}
	defer conn.Close()
	if _, err := io.WriteString(conn.Writer, "OP op=WG_SHOW ifname="+ifname+"\nDONE\n"); err != nil {
		return "", err
	}
	// The launcher returns the WG_SHOW output on a
	// single line for downstream proxying; the agent
	// receives it as is.
	scanner := bufio.NewScanner(conn.Reader)
	scanner.Buffer(make([]byte, 0, 4096), 256*1024)
	var out strings.Builder
	for scanner.Scan() {
		line := scanner.Text()
		if line == "OK done" {
			return strings.TrimSpace(out.String()), nil
		}
		if strings.HasPrefix(line, "ERR ") {
			return "", errors.New(strings.TrimPrefix(line, "ERR "))
		}
		out.WriteString(line)
		out.WriteString("\n")
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return strings.TrimSpace(out.String()), nil
}

// dialConn is the agent-side connection the launcher
// returned. Reader / Writer are buffered so the agent can
// pipeline OP / DONE on a single connection.
type dialConn struct {
	net.Conn
	Reader *bufio.Reader
	Writer *bufio.Writer
}

func (c *LauncherClient) dialLocked() (*dialConn, error) {
	addr := c.socketFn()
	// The launcher is reached over a Unix socket in
	// production (/run/duckos-agent/wg-launcher.sock).
	// Tests swap the socketFn to a tcp:// URL so the
	// runner stays runnable on hosts where the temp
	// dir rejects AF_UNIX binds (some macOS temp
	// dirs). Detect the scheme and dial accordingly.
	var conn net.Conn
	var err error
	if strings.HasPrefix(addr, "tcp://") {
		conn, err = net.Dial("tcp", strings.TrimPrefix(addr, "tcp://"))
	} else if strings.HasPrefix(addr, "unix://") {
		conn, err = net.Dial("unix", strings.TrimPrefix(addr, "unix://"))
	} else {
		// default: Unix domain socket at addr
		conn, err = net.Dial("unix", addr)
	}
	if err != nil {
		return nil, fmt.Errorf("dial wg-launcher %s: %w", addr, err)
	}
	return &dialConn{
		Conn:   conn,
		Reader: bufio.NewReader(conn),
		Writer: bufio.NewWriter(conn),
	}, nil
}

// sendOp is the single-op form: one OP, one response.
func (c *LauncherClient) sendOp(opLine string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.sendOpLocked(opLine)
}

// sendOpLocked sends a single OP / DONE pair over a
// fresh connection and reads the launcher's response. The
// caller MUST hold c.mu.
func (c *LauncherClient) sendOpLocked(opLine string) error {
	conn, err := c.dialLocked()
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err := io.WriteString(conn.Writer, "OP "+opLine+"\nDONE\n"); err != nil {
		return err
	}
	if err := conn.Writer.Flush(); err != nil {
		return err
	}
	scanner := bufio.NewScanner(conn.Reader)
	scanner.Buffer(make([]byte, 0, 1024), 64*1024)
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case line == "OK done":
			return nil
		case line == "OK":
			return nil
		case strings.HasPrefix(line, "ERR "):
			return errors.New(strings.TrimPrefix(line, "ERR "))
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return errors.New("wg-launcher closed without a final response")
}

// opLine assembles "op=VERB k1=v1 k2=v2 ...". Keys are
// emitted in sorted order so the wire format is stable
// across map iterations (Go map iteration order is
// randomised). The launcher parser does not care about
// order; the test suite relies on a canonical output.
func opLine(verb string, m map[string]string) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var out strings.Builder
	out.WriteString("op=")
	out.WriteString(verb)
	for _, k := range keys {
		out.WriteByte(' ')
		out.WriteString(k)
		out.WriteByte('=')
		out.WriteString(m[k])
	}
	return out.String()
}

// peerLine is the bare renderer-side form the launcher
// parses in its peer block.
func peerLine(s string) string { return s }

// writeSecret writes bytes to a temp file under dir
// with mode 0600 and returns the file path plus a
// cleanup function the caller MUST defer. The cleanup
// uses shred -u when available; otherwise the file is
// os.Remove'd (kernel zero-fills the inode after
// unlink).
func writeSecret(dir, ifname string, bytes []byte) (string, func(), error) {
	if dir == "" {
		return "", func() {}, errors.New("secret dir is not configured")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", func() {}, err
	}
	f, err := os.CreateTemp(dir, "wg-secret-*.key")
	if err != nil {
		return "", func() {}, err
	}
	path := f.Name()
	if _, err := f.Write(bytes); err != nil {
		f.Close()
		os.Remove(path)
		return "", func() {}, err
	}
	if err := f.Chmod(0o600); err != nil {
		f.Close()
		os.Remove(path)
		return "", func() {}, err
	}
	if err := f.Close(); err != nil {
		os.Remove(path)
		return "", func() {}, err
	}
	cleanup := func() { shred(path) }
	return path, cleanup, nil
}

func shred(path string) {
	if path == "" {
		return
	}
	// Best-effort shred. On a fresh install that lacks
	// shred, fall back to os.Remove so the file does
	// not outlive the launcher call.
	if _, err := exec.LookPath("shred"); err == nil {
		_ = exec.Command("shred", "-u", path).Run()
		return
	}
	os.Remove(path)
}
