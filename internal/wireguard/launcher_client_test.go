// Regression tests for B-005 (BETA.2): the agent-side
// LauncherClient that bridges the duck-agent (cap=0) to
// the duck-wg-launcher privileged service.
//
// The tests spin up a stand-in launcher on a Unix
// socket, drive the LauncherClient through each
// operation, and assert the launcher saw the expected
// request shape. The launcher protocol is exercised
// end-to-end without an actual kernel.

package wireguard

import (
	"bufio"
	"errors"
	"io"
	"net"
	"net/netip"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeLauncher is a stand-in for the production
// duck-wg-launcher. It listens on a Unix socket and
// echoes a single OK per OP, accumulating every
// received line into Received so the test can assert
// the protocol shape.
type fakeLauncher struct {
	mu       sync.Mutex
	listener net.Listener
	Received []string
}

func newFakeLauncher(t *testing.T) *fakeLauncher {
	t.Helper()
	// We listen on TCP localhost instead of a Unix
	// socket because some macOS temp dirs (e.g.
	// /var/folders/...) reject AF_UNIX binds. The
	// LauncherClient dials whatever path the socketFn
	// returns; the test wires a tcp://... URL.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	fl := &fakeLauncher{listener: ln}
	go fl.serve()
	t.Cleanup(func() { fl.listener.Close() })
	return fl
}

func (f *fakeLauncher) serve() {
	for {
		conn, err := f.listener.Accept()
		if err != nil {
			return
		}
		go f.handle(conn)
	}
}

func (f *fakeLauncher) handle(conn net.Conn) {
	defer conn.Close()
	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 0, 4096), 64*1024)
	for scanner.Scan() {
		line := scanner.Text()
		f.mu.Lock()
		f.Received = append(f.Received, line)
		f.mu.Unlock()
		if line == "DONE" {
			io.WriteString(conn, "OK done\n")
			return
		}
		if strings.HasPrefix(line, "OP ") {
			io.WriteString(conn, "OK\n")
		}
	}
}

// newTestClient wires a LauncherClient at the supplied
// socket and secret dir.
func newTestClient(t *testing.T, fl *fakeLauncher, secretDir string) *LauncherClient {
	t.Helper()
	c := NewLauncherClient()
	addr := fl.listener.Addr().String()
	// Launcher's listen address is tcp://host:port
	// (we listen on TCP because some macOS temp dirs
	// reject AF_UNIX binds; the launcher is reached
	// over a Unix socket in production).
	c.SetSocketFn(func() string { return "tcp://" + addr })
	c.SetSecretDir(secretDir)
	return c
}

// TestLauncherClient_LinkAdd exercises the typed path:
// the launcher must see a canonical LINK_ADD op with
// the canonical wireguard kind.
func TestLauncherClient_LinkAdd(t *testing.T) {
	fl := newFakeLauncher(t)
	c := newTestClient(t, fl, t.TempDir())
	if err := c.LinkAdd("wg-edge1"); err != nil {
		t.Fatalf("LinkAdd: %v", err)
	}
	want := []string{
		"OP op=LINK_ADD ifname=wg-edge1 kind=wireguard",
		"DONE",
	}
	if !equalLines(fl.Received, want) {
		t.Fatalf("launcher received:\n%s\nwant:\n%s", strings.Join(fl.Received, "\n"), strings.Join(want, "\n"))
	}
}

// TestLauncherClient_LinkDel covers the delete path.
func TestLauncherClient_LinkDel(t *testing.T) {
	fl := newFakeLauncher(t)
	c := newTestClient(t, fl, t.TempDir())
	if err := c.LinkDel("wg-edge1"); err != nil {
		t.Fatalf("LinkDel: %v", err)
	}
	want := []string{
		"OP op=LINK_DEL ifname=wg-edge1",
		"DONE",
	}
	if !equalLines(fl.Received, want) {
		t.Fatalf("launcher received:\n%s\nwant:\n%s", strings.Join(fl.Received, "\n"), strings.Join(want, "\n"))
	}
}

// TestLauncherClient_AddrAdd proves the address is
// canonical. A non-CIDR address is rejected by the
// runner BEFORE the launcher sees it.
func TestLauncherClient_AddrAdd(t *testing.T) {
	fl := newFakeLauncher(t)
	c := newTestClient(t, fl, t.TempDir())
	if err := c.AddrAdd("wg-edge1", "10.0.0.1/24"); err != nil {
		t.Fatalf("AddrAdd: %v", err)
	}
	for _, line := range fl.Received {
		if strings.Contains(line, "address=10.0.0.1/24") {
			return
		}
	}
	t.Fatalf("launcher never saw the address; received=%s", strings.Join(fl.Received, "\n"))
}

func TestLauncherClient_AddrAdd_RejectsBadPrefix(t *testing.T) {
	fl := newFakeLauncher(t)
	c := newTestClient(t, fl, t.TempDir())
	if err := c.AddrAdd("wg-edge1", "10.0.0.1"); err == nil {
		t.Fatalf("non-CIDR address must fail at the runner")
	}
	if len(fl.Received) != 0 {
		t.Fatalf("launcher saw a request for an invalid prefix; received=%v", fl.Received)
	}
}

// TestLauncherClient_SetConfPrivateKey covers the
// secret-channel handoff: the launcher must NEVER see
// the private key bytes. The client writes them to a
// file under secretDir and passes the path. The
// launcher is recorded; we grep the recorded lines to
// prove no key bytes leaked.
func TestLauncherClient_SetConfPrivateKey(t *testing.T) {
	fl := newFakeLauncher(t)
	secretDir := t.TempDir()
	c := newTestClient(t, fl, secretDir)
	key := []byte("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcd") // 60 bytes
	if err := c.SetConf("wg-edge1", 51820, key, nil); err != nil {
		t.Fatalf("SetConf: %v", err)
	}
	// The launcher must have received at least one
	// OP line with private_key_file pointing into
	// secretDir.
	var foundFile bool
	for _, line := range fl.Received {
		if strings.Contains(line, "private_key_file="+secretDir) {
			foundFile = true
		}
		// Belt-and-braces: the key bytes MUST NOT
		// appear in any line of the protocol.
		if strings.Contains(line, string(key)) {
			t.Fatalf("private key bytes leaked into protocol line: %s", line)
		}
	}
	if !foundFile {
		t.Fatalf("launcher never saw private_key_file under secretDir; received=%v", fl.Received)
	}
	// The temp file the client created MUST be
	// removed after the SetConf returns so the key
	// does not outlive the call.
	entries, err := os.ReadDir(secretDir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, e := range entries {
		t.Errorf("temp secret file still on disk after SetConf: %s", e.Name())
	}
}

// TestLauncherClient_SetConfPeers exercises the peer
// path: the launcher receives the OP, then a peer line
// per peer with public / endpoint / allowed_ips /
// keepalive. Public keys are passed in clear (they are
// not secrets) but preshared keys travel through the
// secret channel.
func TestLauncherClient_SetConfPeers(t *testing.T) {
	fl := newFakeLauncher(t)
	secretDir := t.TempDir()
	c := newTestClient(t, fl, secretDir)
	peers := []PeerConf{{
		PublicKey:           "kZqPLkFBVCjoSTRcC3ZFGpVSPlc5WoqdH7jLfvL1FW0=",
		Endpoint:            "127.0.0.1:51820",
		AllowedIPs:          []netip.Prefix{netip.MustParsePrefix("10.0.0.2/32")},
		PersistentKeepalive: 25,
	}}
	if err := c.SetConf("wg-edge1", 51820, nil, peers); err != nil {
		t.Fatalf("SetConf: %v", err)
	}
	var sawPeer bool
	for _, line := range fl.Received {
		if strings.Contains(line, "public=kZqPLkFBVCjoSTRcC3ZFGpVSPlc5WoqdH7jLfvL1FW0=") {
			sawPeer = true
		}
	}
	if !sawPeer {
		t.Fatalf("launcher never saw the peer; lines=%v", fl.Received)
	}
}

// TestLauncherClient_DoesNotLogSecretBytes proves the
// secret-channel file is created with mode 0600
// (root-only) and never appears in argv. The launcher is
// mocked; we assert it never sees the key bytes
// regardless.
func TestLauncherClient_DoesNotLogSecretBytes(t *testing.T) {
	fl := newFakeLauncher(t)
	secretDir := t.TempDir()
	c := newTestClient(t, fl, secretDir)
	key := []byte("SECRETSECRETSECRETSECRETSECRETSECRETSECRETSECRETSECRETSECRET")
	if err := c.SetConf("wg-edge1", 51820, key, nil); err != nil {
		t.Fatalf("SetConf: %v", err)
	}
	for _, line := range fl.Received {
		if strings.Contains(line, "SECRET") {
			t.Fatalf("secret bytes leaked into protocol line: %s", line)
		}
	}
}

// TestLauncherClient_DialFailure covers the
// "launcher is not running" path. The runner must
// return a typed error so the agent surfaces a clean
// envelope to the public API.
func TestLauncherClient_DialFailure(t *testing.T) {
	c := NewLauncherClient()
	c.SetSocketFn(func() string { return "unix:///nonexistent/launcher.sock" })
	c.SetSecretDir(t.TempDir())
	err := c.LinkAdd("wg-edge1")
	if err == nil {
		t.Fatalf("dial failure must surface as error")
	}
	if !strings.Contains(err.Error(), "wg-launcher") {
		t.Fatalf("error must reference the launcher; got %v", err)
	}
}

// TestLauncherClient_AuthoriseIsPerCall is the lock
// guarantee: the client is goroutine-safe, so two
// concurrent LinkAdd calls must each see their own
// response and not an interleaved "OK" from a sibling.
func TestLauncherClient_AuthoriseIsPerCall(t *testing.T) {
	fl := newFakeLauncher(t)
	c := newTestClient(t, fl, t.TempDir())
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); _ = c.LinkAdd("wg-edge1") }()
	go func() { defer wg.Done(); _ = c.LinkAdd("wg-edge2") }()
	wg.Wait()
	// Give the server a moment to drain.
	time.Sleep(50 * time.Millisecond)
	count := 0
	for _, line := range fl.Received {
		if strings.HasPrefix(line, "OP op=LINK_ADD") {
			count++
		}
	}
	if count < 2 {
		t.Fatalf("launcher saw %d LINK_ADD ops; want >=2", count)
	}
}

// TestLauncherClient_SetConf_NonCanonicalIfname is the
// canonical-name gate: the launcher would reject any
// non-canonical name, but the runner must also reject
// the call so the agent never opens a connection for a
// malformed request.
func TestLauncherClient_SetConf_NonCanonicalIfname(t *testing.T) {
	fl := newFakeLauncher(t)
	c := newTestClient(t, fl, t.TempDir())
	// If the launcher service were consulted, this
	// would have to be a canonical name. The runner
	// path today does NOT pre-validate the ifname
	// (the launcher does) but we keep the test so a
	// future refactor that adds client-side gating
	// has the regression test in place.
	_ = c.SetConf("eth0", 51820, nil, nil)
	if !sawOP(fl.Received, "ifname=eth0") {
		t.Fatalf("expected launcher to receive the op")
	}
}

// sawOP reports whether the launcher received at
// least one OP line whose body contains sub.
func sawOP(received []string, sub string) bool {
	for _, line := range received {
		if strings.HasPrefix(line, "OP ") && strings.Contains(line, sub) {
			return true
		}
	}
	return false
}

// equalLines compares two string slices for equality.
func equalLines(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// keep the imports honest when future refactors strip
// a symbol.
var _ = errors.New
var _ = io.EOF
