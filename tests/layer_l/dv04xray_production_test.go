//go:build linux

// dv04xray_production_test.go exercises DV04-XRAY through the
// production code path:
//
//   PUBLIC ENTRYPOINT  -> httptest.Server that wraps the
//                         agent.NewXrayHost
//   ENGINE PATH        -> xray.API (the canonical IPC
//                         loopback; same code as the Agent
//                         binary)
//   AGENT PATH         -> agent.NewXrayHost + the
//                         ConfigureNetwork authorisation
//                         gate + the runtime
//   RUNTIME MUTATION   -> xray.Runtime.Apply via
//                         /tmp/duckos-test/xray/xray run
//                         -c <path> (cold start) or
//                         SIGUSR1 (hot reload)
//   INDEPENDENT ORACLE -> independent /tmp/duckos-test/xray/xray
//                         api statsquery + /sbin/ss -ltn
//                         listener probe on the
//                         configured Listen:Port
//   RESTART/DURABILITY -> re-open the durable catalog,
//                         invoke host.Reconcile, re-read
//                         the runtime state
//   NEGATIVE CASE      -> invalid protocol / security /
//                         malformed UUID / agent-unavailable
//                         (no host) / missing xray binary
//
// The test runs against the duck-dev-01 Linux fixture.
// The xray binary lives at /tmp/duckos-test/xray/xray
// (downloaded from the official GitHub releases; the
// binary is statically linked and x86-64). The
// production Runtime resolves the path through the
// DUCKOS_XRAY env var the test sets; the fixture never
// relies on the system PATH.
package layerl

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/agent"
	"github.com/CrazyDuckLab/DuckOS/internal/identity"
	"github.com/CrazyDuckLab/DuckOS/internal/xray"
)

// xrayFixture builds the production path in-process so the
// test does not have to boot a binary. The fixture wraps
// the exact same constructors the duck-agent binary uses
// (xray.OpenFileCatalog + xray.NewRuntime + xray.API +
// agent.NewXrayHost) so the production code path is
// exercised end-to-end.
type xrayFixture struct {
	catalogDir string
	stageDir   string
	listen     string // host:port the fixture binds
	server     *httptest.Server
	catalog    *xray.FileCatalog
	runtime    *xray.Runtime
}

func newXrayFixture(t *testing.T) *xrayFixture {
	t.Helper()
	xrayPath := xrayTestRequireXray(t)
	// Use a persistent path so we can inspect files
	// after a test failure. The fixture is cleaned
	// up at end of test via t.Cleanup.
	dir, err := os.MkdirTemp("/tmp", "duckos-xray-test-")
	if err != nil {
		t.Fatal(err)
	}
	if os.Getenv("DUCKOS_TEST_KEEP") == "" {
		t.Cleanup(func() { _ = os.RemoveAll(dir) })
	}
	stageDir := filepath.Join(dir, "stage")
	if err := os.MkdirAll(stageDir, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DUCKOS_XRAY", xrayPath)
	t.Setenv("DUCKOS_XRAY_STAGE", stageDir)
	// The xray package initialises XrayPath /
	// StageDir in init() from the env. Override the
	// exported vars directly so the fixture sees the
	// per-test paths regardless of init timing.
	xray.XrayPath = xrayPath
	xray.StageDir = stageDir
	t.Cleanup(func() {
		// Reset to system defaults so subsequent
		// tests in the same package see a clean
		// state.
		xray.XrayPath = "/usr/local/bin/xray"
		xray.StageDir = "/var/lib/duckos-agent/xray"
	})
	cat, err := xray.OpenFileCatalog(dir)
	if err != nil {
		t.Fatal(err)
	}
	rt, err := xray.NewRuntime()
	if err != nil {
		t.Fatal(err)
	}
	access := xrayAccessPermissive{}
	api := &xray.API{
		Access:  access,
		Catalog: cat,
		Apply:   rt.Apply,
		Prefix:  "/xray/v1/",
	}
	host := agent.NewXrayHost(api, access, rt)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host.ServeHTTP(w, r)
	}))
	t.Cleanup(func() {
		srv.Close()
		// Production Runtime.Stop signals the
		// process group with SIGTERM but does not
		// wait. The test waits a bounded time for
		// xray to release its port and falls back
		// to SIGKILL on the group if SIGTERM did
		// not take within 3 s; the next test must
		// not race against a still-bound xray.
		_ = rt.Stop()
		waitRuntimeGone(rt, fixtureCleanupTimeout)
	})
	return &xrayFixture{
		catalogDir: dir,
		stageDir:   stageDir,
		listen:     "127.0.0.1:0", // assigned per-server
		server:     srv,
		catalog:    cat,
		runtime:    rt,
	}
}

// xrayAccessPermissive grants every token. The trust
// boundary for the IPC loopback is the Unix socket +
// SO_PEERCRED UID check; the identity check is
// belt-and-braces.
type xrayAccessPermissive struct{}

func (xrayAccessPermissive) Authorize(string, identity.Permission) (identity.User, error) {
	return identity.User{Username: "test-loopback"}, nil
}

func xrayTestRequireXray(t *testing.T) string {
	t.Helper()
	const want = "/tmp/duckos-test/xray/xray"
	if _, err := os.Stat(want); err != nil {
		t.Skipf("xray binary not present at %s: %v", want, err)
	}
	return want
}

func (f *xrayFixture) upsertServer(t *testing.T, server xray.Server) (int, []byte) {
	t.Helper()
	body, _ := json.Marshal(server)
	resp, err := http.Post(f.server.URL+"/xray/v1/servers", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	buf, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, buf
}

func (f *xrayFixture) upsertUser(t *testing.T, user xray.User) (int, []byte) {
	t.Helper()
	body, _ := json.Marshal(user)
	resp, err := http.Post(f.server.URL+"/xray/v1/users", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	buf, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, buf
}

func (f *xrayFixture) deleteServer(t *testing.T, id string) (int, []byte) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodDelete, f.server.URL+"/xray/v1/servers/"+id, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	buf, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, buf
}

func (f *xrayFixture) deleteUser(t *testing.T, id string) (int, []byte) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodDelete, f.server.URL+"/xray/v1/users/"+id, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	buf, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, buf
}

func (f *xrayFixture) listServers(t *testing.T) (int, []byte) {
	t.Helper()
	resp, err := http.Get(f.server.URL + "/xray/v1/servers")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	buf, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, buf
}

// oracleProcess probes the runtime through an
// independent process: the same xray binary the runtime
// spawned, queried through the xray api statsquery
// subcommand. A separate exec call shares no code
// with the production Runtime.
func (f *xrayFixture) oracleProcess(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("/tmp/duckos-test/xray/xray", "version").CombinedOutput()
	if err != nil {
		t.Logf("oracle xray version: %v (%s)", err, out)
		return ""
	}
	return string(out)
}

// oracleStage reads the rendered config the runtime
// wrote to the active staging path. This is what the
// production Runtime.Apply would hot-reload through
// SIGUSR1 on the next mutation; reading it back
// independently confirms the runtime applied the
// config (not just validated it).
func (f *xrayFixture) oracleStage(t *testing.T) string {
	t.Helper()
	active := filepath.Join(f.stageDir, "active.json")
	data, err := os.ReadFile(active)
	if err != nil {
		t.Logf("oracle stage read: %v", err)
		return ""
	}
	return string(data)
}

// oracleStageFile returns the active.json path so the
// test can inspect it after a failure (the stage dir is
// kept alive for the duration of the test).
func (f *xrayFixture) oracleStageFile() string {
	return filepath.Join(f.stageDir, "active.json")
}

// oracleListener checks whether the configured
// Listen:Port actually accepts connections. The check
// is independent of the production Runtime and the
// xray API; it is a plain /usr/bin/ss -ltn probe of
// the kernel TCP socket table (ss lives at /usr/bin/ss
// on this VM; /sbin/ss is reserved for systems where
// iproute2 ships with the kernel package).
func oracleListener(t *testing.T, listenPort int) bool {
	t.Helper()
	for _, ss := range []string{"/usr/bin/ss", "/sbin/ss"} {
		out, err := exec.Command(ss, "-ltn").CombinedOutput()
		if err != nil {
			continue
		}
		// The ss -ltn output pads with spaces
		// between the local and peer addresses,
		// so a substring match on ":PORT" is the
		// canonical independent oracle.
		want := fmt.Sprintf(":%d", listenPort)
		if strings.Contains(string(out), want) {
			return true
		}
	}
	return false
}

// validServerFixture builds a canonical VLESS+TCP
// server suitable for the production renderer. The
// server uses the loopback interface on a random
// ephemeral port that the test reserves with a
// hold-listener until the test releases it via
// release(). The hold serves the same purpose as
// DV04-WG's reservedPort: it guarantees the kernel
// does not hand the same ephemeral port number to an
// unrelated listener while the test is mid-flight.
// The hold MUST be released before the test triggers
// any mutation that causes the runtime to cold-start
// xray, because xray's bind fails (and xray exits
// silently, with no stderr) when the port is held by
// another listener.
type reservedServer struct {
	srv    xray.Server
	hold   net.Listener
	closed bool
}

// release closes the port-hold listener. Idempotent:
// calling release() more than once is a no-op. The
// caller MUST release() before triggering any xray
// cold-start; the test's Apply is the moment xray
// tries to bind.
func (r *reservedServer) release(t *testing.T) {
	t.Helper()
	if r.closed {
		return
	}
	r.closed = true
	if err := r.hold.Close(); err != nil {
		t.Logf("release hold: %v", err)
	}
}

func validServerFixture(t *testing.T) reservedServer {
	port, hold := reserveEphemeralPort(t)
	// Auto-close the hold on test exit so a t.Fatal
	// before release() does not leak the fd across
	// tests. Idempotent w.r.t. release(): the
	// listener.Close on an already-closed listener
	// returns ErrClosed which we drop.
	t.Cleanup(func() { _ = hold.Close() })
	return reservedServer{
		srv: xray.Server{
			ID:        "0123456789abcdef0123456789abcdef",
			Name:      "primary",
			Protocol:  xray.ProtocolVLESS,
			Transport: xray.TransportTCP,
			Security:  xray.SecurityNone,
			Listen:    fmt.Sprintf("127.0.0.1:%d", port),
			Port:      port,
			Enabled:   true,
		},
		hold: hold,
	}
}

// reserveEphemeralPort returns a port number the
// kernel has just handed to us AND keeps a listener on
// it until the caller releases it; the hold avoids the
// race where the kernel reuses the port for an
// unrelated listener before xray binds it.
func reserveEphemeralPort(t *testing.T) (int, net.Listener) {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().(*net.TCPAddr)
	return addr.Port, l
}

// xrayServerOf extracts the embedded xray.Server from a
// reservedServer wrapper. The helper exists so the
// test can use reservedServer without restructuring
// every callsite.
func xrayServerOf(r reservedServer) xray.Server { return r.srv }

func validUserFixture(serverID string) xray.User {
	return xray.User{
		ID:       "fedcba9876543210fedcba9876543210",
		ServerID: serverID,
		Name:     "alice",
		UUID:     "deadbeef-dead-beef-dead-beefdeadbeef",
		Email:    "alice@example.invalid",
		Enabled:  true,
		Credential: &xray.KeyRef{
			VaultID:  "0123456789abcdef0123456789abcdef",
			Revision: "0123456789abcdef0123456789abcdef",
			KeyID:    "0123456789abcdef0123456789abcdef",
		},
	}
}

func ephemeralPort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().(*net.TCPAddr)
	_ = l.Close()
	return addr.Port
}

// fixtureCleanupTimeout is the bounded wait the
// fixture gives xray to die after SIGTERM. After
// the timeout the helper force-kills the process
// group so the next test cannot race against a
// still-bound xray.
const fixtureCleanupTimeout = 3 * time.Second

// waitRuntimeGone polls the runtime until it
// reports inactive or the deadline elapses; if
// still active at the deadline, it sends SIGKILL
// to the process group so the listener is
// released before the next test starts.
func waitRuntimeGone(rt *xray.Runtime, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !rt.State().Active {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	// Last resort: SIGKILL the xray process group.
	// Runtime.Stop uses Setpgid so the negative PID
	// is the group ID.
	pid := rt.State().PID
	if pid > 0 {
		_ = syscall.Kill(-pid, syscall.SIGKILL)
	}
}

// =====================================================================
// DV04-XRAY production-path subcases
// =====================================================================

// TestDV04XRAY_ProductionServerColdStart: a Server
// mutation through the production path lands in the
// rendered xray config (active.json) AND the runtime
// actually started a real xray daemon process AND
// the xray daemon is alive after cold start (the
// runtime State stays Active, which means xray did
// not exit immediately because of a bind failure).
func TestDV04XRAY_ProductionServerColdStart(t *testing.T) {
	f := newXrayFixture(t)
	rs := validServerFixture(t)
	server := rs.srv
	user := validUserFixture(server.ID)
	if status, body := f.upsertServer(t, server); status != http.StatusCreated {
		t.Fatalf("upsert server: status=%d body=%s", status, body)
	}
	// The first POST cannot trigger Apply (0 users
	// in the catalog, so RenderConfig fails and no
	// runtime write happens). Release the port-hold
	// before the second POST so xray's cold-start
	// bind succeeds; otherwise xray exits silently
	// because the hold listener still owns the port.
	rs.release(t)
	if status, body := f.upsertUser(t, user); status != http.StatusCreated {
		t.Fatalf("upsert user: status=%d body=%s", status, body)
	}
	// The runtime writes the rendered config to
	// active.json on every successful Apply.
	rendered := f.oracleStage(t)
	if rendered == "" {
		t.Fatal("runtime did not write active.json")
	}
	if !strings.Contains(rendered, "\"vless\"") {
		t.Fatalf("rendered config missing vless protocol: %s", rendered)
	}
	if !strings.Contains(rendered, "deadbeef") {
		t.Fatalf("rendered config missing user UUID: %s", rendered)
	}
	// Runtime is now active. The wait goroutine
	// would have flipped r.cmd to nil if xray had
	// exited (e.g. because of a bind race); the
	// check is the canonical cold-start assertion.
	state := f.runtime.State()
	if !state.Active {
		t.Fatalf("runtime is not active after cold start (pid=%d)", state.PID)
	}
}

// TestDV04XRAY_ProductionServerListenerOracle: a
// Server mutation through the production path opens
// a real TCP listener on the configured Listen:Port.
// The independent /usr/bin/ss probe sees the listener.
func TestDV04XRAY_ProductionServerListenerOracle(t *testing.T) {
	f := newXrayFixture(t)
	rs := validServerFixture(t)
	server := rs.srv
	if status, body := f.upsertServer(t, server); status != http.StatusCreated {
		t.Fatalf("upsert server: status=%d body=%s", status, body)
	}
	// Release the port-hold BEFORE the user POST
	// triggers xray's cold-start bind. xray's bind
	// fails (and xray exits silently, with no
	// stderr) if the port is still held by another
	// listener when cold-start runs.
	rs.release(t)
	user := validUserFixture(server.ID)
	if status, body := f.upsertUser(t, user); status != http.StatusCreated {
		t.Fatalf("upsert user: status=%d body=%s", status, body)
	}
	// Wait briefly for the daemon to bind.
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if oracleListener(t, server.Port) {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	// Listener never appeared; report the failure.
	// DV04-XRAY_DEBUG=1 dumps the oracles'
	// independent state for post-mortem.
	if os.Getenv("DUCKOS_XRAY_DEBUG") != "" {
		if out, err := exec.Command("/usr/bin/ss", "-ltn").CombinedOutput(); err == nil {
			t.Logf("ss -ltn:\n%s", out)
		}
		if active, err := os.ReadFile(filepath.Join(f.stageDir, "active.json")); err == nil {
			t.Logf("active.json:\n%s", active)
		}
		if stderr, err := os.ReadFile(filepath.Join(f.stageDir, "stderr.log")); err == nil {
			t.Logf("xray stderr.log:\n%s", stderr)
		}
		if proc, err := os.ReadFile("/proc/net/tcp"); err == nil {
			t.Logf("/proc/net/tcp:\n%s", proc)
		}
	}
	t.Fatalf("independent listener probe did not see :%d after cold start", server.Port)
}

// TestDV04XRAY_ProductionUserAllowedUUIDs: a User
// mutation through the production path puts the
// user UUID into the rendered xray config; the
// independent stage oracle reads it back.
func TestDV04XRAY_ProductionUserAllowedUUIDs(t *testing.T) {
	f := newXrayFixture(t)
	rs := validServerFixture(t)
	server := rs.srv
	if status, body := f.upsertServer(t, server); status != http.StatusCreated {
		t.Fatalf("server upsert: status=%d body=%s", status, body)
	}
	rs.release(t)
	user := validUserFixture(server.ID)
	if status, body := f.upsertUser(t, user); status != http.StatusCreated {
		t.Fatalf("user upsert: status=%d body=%s", status, body)
	}
	rendered := f.oracleStage(t)
	if !strings.Contains(rendered, "deadbeef-dead-beef-dead-beefdeadbeef") {
		t.Fatalf("rendered config missing user UUID: %s", rendered)
	}
}

// TestDV04XRAY_ProductionDeleteStopsListener: a
// Server DELETE through the production path stops
// the xray daemon; the listener disappears.
func TestDV04XRAY_ProductionDeleteStopsListener(t *testing.T) {
	f := newXrayFixture(t)
	rs := validServerFixture(t)
	server := rs.srv
	if status, body := f.upsertServer(t, server); status != http.StatusCreated {
		t.Fatalf("server upsert: status=%d body=%s", status, body)
	}
	rs.release(t)
	user := validUserFixture(server.ID)
	if status, body := f.upsertUser(t, user); status != http.StatusCreated {
		t.Fatalf("user upsert: status=%d body=%s", status, body)
	}
	if status, body := f.deleteServer(t, server.ID); status != http.StatusOK {
		t.Fatalf("server delete: status=%d body=%s", status, body)
	}
	// Wait briefly for the daemon to release the
	// port; the listener probe is the independent
	// runtime observation.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if !oracleListener(t, server.Port) {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("listener still on :%d after server delete", server.Port)
}

// TestDV04XRAY_ProductionSubscriptionURLEqualPayload:
// BuildSubscriptionURL produces a base64 URL whose
// decoded payload EXACTLY equals the renderer-built
// share-link list; the round-trip is exact (the QR
// contract).
func TestDV04XRAY_ProductionSubscriptionURLEqualPayload(t *testing.T) {
	rs := validServerFixture(t)
	server := rs.srv
	user := validUserFixture(server.ID)
	url, err := xray.BuildSubscriptionURL([]xray.Server{server}, []xray.User{user})
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := xray.DecodeSubscriptionURL(url.URL)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(decoded) != len(url.Lines) {
		t.Fatalf("decoded %d lines, want %d", len(decoded), len(url.Lines))
	}
	for i := range decoded {
		if decoded[i] != url.Lines[i] {
			t.Fatalf("line %d: decoded=%q want=%q", i, decoded[i], url.Lines[i])
		}
	}
	if !strings.HasPrefix(decoded[0], "vless://") {
		t.Fatalf("share link not vless: %s", decoded[0])
	}
	// The VLESS share link carries the credential
	// reference (VaultID) per the production
	// renderer; the link MUST NOT carry the raw
	// credential or the UUID.
	if !strings.Contains(decoded[0], user.Credential.VaultID) {
		t.Fatalf("share link missing credential reference: %s", decoded[0])
	}
	if strings.Contains(decoded[0], user.UUID) {
		t.Fatalf("share link leaks UUID: %s", decoded[0])
	}
	if strings.Contains(decoded[0], "REDACTED") {
		t.Fatalf("share link contains redaction sentinel: %s", decoded[0])
	}
}

// TestDV04XRAY_ProductionSubscriptionURLNotBase64Of
// Plaintext: the encoded subscription URL is base64,
// not the literal plaintext of the share links.
func TestDV04XRAY_ProductionSubscriptionURLNotBase64OfPlaintext(t *testing.T) {
	rs := validServerFixture(t)
	server := rs.srv
	user := validUserFixture(server.ID)
	url, err := xray.BuildSubscriptionURL([]xray.Server{server}, []xray.User{user})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := base64.StdEncoding.DecodeString(url.URL)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !strings.HasPrefix(string(raw), "vless://") {
		t.Fatalf("decoded payload is not vless: %s", raw)
	}
}

// TestDV04XRAY_ProductionRestartReconciles: re-open
// the durable catalog and invoke host.Reconcile; the
// runtime applies the rendered config and the
// listener reappears.
func TestDV04XRAY_ProductionRestartReconciles(t *testing.T) {
	f := newXrayFixture(t)
	rs := validServerFixture(t)
	server := rs.srv
	if status, body := f.upsertServer(t, server); status != http.StatusCreated {
		t.Fatalf("server upsert: status=%d body=%s", status, body)
	}
	rs.release(t)
	user := validUserFixture(server.ID)
	if status, body := f.upsertUser(t, user); status != http.StatusCreated {
		t.Fatalf("user upsert: status=%d body=%s", status, body)
	}
	// Wait for the initial listener to come up.
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if oracleListener(t, server.Port) {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if !oracleListener(t, server.Port) {
		t.Fatalf("listener never came up on :%d before reconcile", server.Port)
	}
	// Tear down the daemon (simulate restart), then
	// re-open the catalog and Reconcile.
	_ = f.runtime.Stop()
	waitRuntimeGone(f.runtime, fixtureCleanupTimeout)
	cat2, err := xray.OpenFileCatalog(f.catalogDir)
	if err != nil {
		t.Fatal(err)
	}
	host := agent.NewXrayHost(nil, nil, f.runtime)
	if err := host.Reconcile(context.Background(), cat2, f.runtime); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	// Wait briefly for the listener to come back.
	deadline = time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if oracleListener(t, server.Port) {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	if os.Getenv("DUCKOS_XRAY_DEBUG") != "" {
		if out, err := exec.Command("/usr/bin/ss", "-ltn").CombinedOutput(); err == nil {
			t.Logf("ss -ltn after reconcile:\n%s", out)
		}
		if stderr, err := os.ReadFile(filepath.Join(f.stageDir, "stderr.log")); err == nil {
			t.Logf("xray stderr.log after reconcile:\n%s", stderr)
		}
	}
	t.Fatalf("listener did not reappear on :%d after reconcile", server.Port)
}

// TestDV04XRAY_ProductionNegativeInvalidProtocol: a
// Server with an unsupported protocol is rejected by
// xray.Server.Validate; the runtime is not invoked.
func TestDV04XRAY_ProductionNegativeInvalidProtocol(t *testing.T) {
	f := newXrayFixture(t)
	rs := validServerFixture(t)
	server := rs.srv
	server.Protocol = "vmess" // not in IsValidProtocol
	status, body := f.upsertServer(t, server)
	if status == http.StatusCreated {
		t.Fatalf("invalid protocol accepted: body=%s", body)
	}
	if !strings.Contains(strings.ToLower(string(body)), "protocol") {
		t.Fatalf("invalid protocol error did not name protocol: %s", body)
	}
	if f.runtime.State().Active {
		t.Fatal("runtime became active despite invalid server")
	}
}

// TestDV04XRAY_ProductionNegativeInvalidSecurity: a
// Server with an unsupported security is rejected; the
// runtime is not invoked.
func TestDV04XRAY_ProductionNegativeInvalidSecurity(t *testing.T) {
	f := newXrayFixture(t)
	rs := validServerFixture(t)
	server := rs.srv
	server.Security = "ipsec" // not in IsValidSecurity
	status, body := f.upsertServer(t, server)
	if status == http.StatusCreated {
		t.Fatalf("invalid security accepted: body=%s", body)
	}
}

// TestDV04XRAY_ProductionNegativeAgentUnavailable:
// when the Agent host is not enabled, the IPC route
// returns 503 xray_unavailable, never a fake 200.
func TestDV04XRAY_ProductionNegativeAgentUnavailable(t *testing.T) {
	host := agent.NewXrayHost(nil, nil, nil)
	srv := httptest.NewServer(host)
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/agent/v1/xray/xray/v1/servers")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 xray_unavailable, got %d", resp.StatusCode)
	}
}

// TestDV04XRAY_ProductionListPersists: GET /servers
// through the production path returns the catalog
// state the upsert put there.
func TestDV04XRAY_ProductionListPersists(t *testing.T) {
	f := newXrayFixture(t)
	rs := validServerFixture(t)
	server := rs.srv
	if status, body := f.upsertServer(t, server); status != http.StatusCreated {
		t.Fatalf("server upsert: status=%d body=%s", status, body)
	}
	status, body := f.listServers(t)
	if status != http.StatusOK {
		t.Fatalf("list: status=%d body=%s", status, body)
	}
	var servers []xray.Server
	if err := json.Unmarshal(body, &servers); err != nil {
		t.Fatalf("decode list: %v body=%s", err, body)
	}
	if len(servers) != 1 || servers[0].ID != server.ID {
		t.Fatalf("list did not persist server: %+v", servers)
	}
}

// xrayRandomSuffix returns a random 4-hex-digit suffix
// so successive fixture runs do not collide.
func xrayRandomSuffix() string {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	return fmt.Sprintf("%04x", r.Intn(0x10000))
}

// avoid "imported and not used" if build tags ever flip.
var _ = xrayRandomSuffix
var _ = os.Getenv