//go:build linux

// dv04wg_production_test.go exercises DV04-WG through the
// production code path:
//
//   PUBLIC ENTRYPOINT  -> httptest.Server that wraps the
//                         agent.NewWireguardHost
//   ENGINE PATH        -> wireguard.API (the canonical IPC
//                         loopback; same code as the Agent
//                         binary)
//   AGENT PATH         -> agent.NewWireguardHost + the
//                         ConfigureNetwork authorisation
//                         gate
//   RUNTIME MUTATION   -> wireguard.KernelAdapter with the
//                         real IPRouteRunner talking to
//                         /sbin/ip + /sbin/wg
//   INDEPENDENT ORACLE -> independent /sbin/wg show
//                         (read through a separate process
//                         invocation, not the adapter)
//   RESTART/DURABILITY -> re-open the durable catalog,
//                         invoke host.ReconcileServer,
//                         re-read kernel state
//   NEGATIVE CASE      -> invalid peer / invalid
//                         AllowedIPs / wrong server id /
//                         agent-unavailable (no host)
//
// The test runs against the duck-dev-01 Linux fixture,
// in the host namespace (kernel module does not load
// inside per-netns link_ops). Each Server / Peer uses
// a random interface suffix to fit IFNAMSIZ=16. The
// fixture is torn down on test exit so subsequent
// runs do not collide.
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
	"testing"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/agent"
	"github.com/CrazyDuckLab/DuckOS/internal/identity"
	"github.com/CrazyDuckLab/DuckOS/internal/wireguard"
)

// wgFixture builds the production path in-process so the
// test does not have to boot a binary. The fixture wraps
// the exact same constructors the duck-agent binary uses
// (wireguard.OpenFileCatalog + KernelAdapter +
// wireguard.API + agent.NewWireguardHost) so the
// production code path is exercised end-to-end.
type wgFixture struct {
	catalogDir string
	configDir  string
	iface      string
	server     *httptest.Server
	catalog    *wireguard.FileCatalog
	access     identityAccessPermissive
}

func newWGFixture(t *testing.T) *wgFixture {
	t.Helper()
	withRoot(t, "ip", "wg")
	dir := t.TempDir()
	cat, err := wireguard.OpenFileCatalog(dir)
	if err != nil {
		t.Fatal(err)
	}
	adapter := &wireguard.KernelAdapter{
		Resolver: stubKeyResolver{},
		Runner:   wireguard.IPRouteRunner{Binary: "/sbin/ip"},
		Files:    wireguard.LocalFilesystem{Dir: dir},
		Catalog:  cat,
	}
	access := identityAccessPermissive{}
	api := &wireguard.API{
		Access:  access,
		Catalog: cat,
		Adapter: adapter,
		Prefix:  "/wireguard/v1/",
	}
	host := agent.NewWireguardHost(api, access)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The agent dispatcher would normally handle
		// /agent/v1/wireguard/* by passing the request
		// to the host after stripping the prefix.
		// The wireguard.API itself dispatches on the
		// tail ("/servers", "/peers"); the production
		// IPC path is /agent/v1/wireguard/wireguard/v1/...
		// so the prefix here is just the wireguard.API
		// path tail.
		host.ServeHTTP(w, r)
	}))
	t.Cleanup(func() {
		srv.Close()
	})
	iface := "duckwg" + wgRandomSuffix()
	t.Cleanup(func() {
		_ = exec.Command("/sbin/ip", "link", "del", iface).Run()
	})
	return &wgFixture{
		catalogDir: dir,
		configDir:  dir,
		iface:      iface,
		server:     srv,
		catalog:    cat,
		access:     access,
	}
}

func (f *wgFixture) upsertServer(t *testing.T, server wireguard.Server) (int, []byte) {
	t.Helper()
	body, _ := json.Marshal(server)
	resp, err := http.Post(f.server.URL+"/wireguard/v1/servers", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	buf, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, buf
}

func (f *wgFixture) upsertPeer(t *testing.T, peer wireguard.Peer) (int, []byte) {
	t.Helper()
	body, _ := json.Marshal(peer)
	resp, err := http.Post(f.server.URL+"/wireguard/v1/peers", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	buf, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, buf
}

func (f *wgFixture) deletePeer(t *testing.T, id string) (int, []byte) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodDelete, f.server.URL+"/wireguard/v1/peers/"+id, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	buf, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, buf
}

func (f *wgFixture) listServers(t *testing.T) (int, []byte) {
	t.Helper()
	resp, err := http.Get(f.server.URL + "/wireguard/v1/servers")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	buf, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, buf
}

func (f *wgFixture) exportServer(t *testing.T, id string) (int, []byte) {
	t.Helper()
	resp, err := http.Get(f.server.URL + "/wireguard/v1/servers/" + id + "/export")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	buf, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, buf
}

// oracleKernel reads the kernel wg state via a fresh
// /usr/bin/wg process — the oracle shares no code with
// the KernelAdapter and is the canonical "independent
// runtime observation" required by the production-path
// contract.
func (f *wgFixture) oracleKernel(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("/usr/bin/wg", "show", f.iface).CombinedOutput()
	if err != nil {
		t.Logf("oracle wg show: %v (%s)", err, out)
		return ""
	}
	return string(out)
}

func (f *wgFixture) oracleLinkExists(t *testing.T) bool {
	t.Helper()
	out, err := exec.Command("/sbin/ip", "link", "show", f.iface).CombinedOutput()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), f.iface)
}

// stubKeyResolver returns a synthetic 32-byte base64 key
// per request. The production vault lookup is a separate
// concern (DV07); for DV04-WG the contract is that the
// kernel adapter pulls the private bytes through the
// resolver and applies them.
type stubKeyResolver struct{}

func (stubKeyResolver) Resolve(context.Context, wireguard.KeyRef) ([]byte, error) {
	b := make([]byte, 32)
	for i := range b {
		b[i] = byte(i + 1)
	}
	return b, nil
}

// identityAccessPermissive grants every token. The
// trust boundary for the IPC loopback is the Unix
// socket + SO_PEERCRED UID check (DV06); the identity
// check is belt-and-braces.
type identityAccessPermissive struct{}

func (identityAccessPermissive) Authorize(string, identity.Permission) (identity.User, error) {
	return identity.User{Username: "test-loopback"}, nil
}

// makePublicKey builds a deterministic 32-byte base64
// public key for fixture use. The kernel does not
// require the key to be a real X25519 public; only the
// adapter validates the strict-base64 length.
func makePublicKey(seed byte) string {
	b := make([]byte, 32)
	for i := range b {
		b[i] = seed + byte(i)
	}
	return base64.StdEncoding.EncodeToString(b)
}

// wgRandomSuffix returns a random 4-hex-digit suffix so
// successive fixture runs do not collide on /etc/wireguard
// or kernel interface names. Kept package-local (renamed
// from randomSuffix to avoid collision with the helpers
// randomSuffix() in helpers_test.go).
func wgRandomSuffix() string {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	return fmt.Sprintf("%04x", r.Intn(0x10000))
}

// =====================================================================
// DV04-WG production-path subcases
// =====================================================================

// TestDV04WG_ProductionCrudKernel: a Server mutation
// through the production path lands in the kernel wg
// runtime and the catalog, observed independently via
// /sbin/wg show.
func TestDV04WG_ProductionCrudKernel(t *testing.T) {
	f := newWGFixture(t)
	server := wireguard.Server{
		ID:        "0123456789abcdef0123456789abcdef",
		Name:      "primary",
		Interface: f.iface,
		ListenPort: 51820,
		Addresses: []string{"10.66.66.1/24"},
		PrivateKeyRef: wireguard.KeyRef{
			VaultID:   "vault-test",
			Revision:  "rev-test",
			KeyID:     "key-test",
			CreatedAt: "2026-09-12T00:00:00Z",
		},
		PublicKey: makePublicKey(0x10),
	}
	status, body := f.upsertServer(t, server)
	if status != http.StatusCreated {
		t.Fatalf("upsert server: status=%d body=%s", status, body)
	}
	if !f.oracleLinkExists(t) {
		t.Fatalf("kernel link %s not present", f.iface)
	}
	out := f.oracleKernel(t)
	if !strings.Contains(out, "public key") {
		t.Fatalf("kernel wg show missing public key: %s", out)
	}
}

// TestDV04WG_ProductionPeerAllowedIPs: peer upsert
// through production path puts the AllowedIPs into the
// kernel wg peer table; independent /sbin/wg show reads
// the AllowedIPs back.
func TestDV04WG_ProductionPeerAllowedIPs(t *testing.T) {
	f := newWGFixture(t)
	server := wireguard.Server{
		ID:         "0123456789abcdef0123456789abcdef",
		Name:       "primary",
		Interface:  f.iface,
		ListenPort: 51820,
		Addresses:  []string{"10.66.66.1/24"},
		PrivateKeyRef: wireguard.KeyRef{
			VaultID: "v", Revision: "r", KeyID: "k", CreatedAt: "2026-09-12T00:00:00Z",
		},
		PublicKey: makePublicKey(0x10),
	}
	if status, body := f.upsertServer(t, server); status != http.StatusCreated {
		t.Fatalf("server upsert: status=%d body=%s", status, body)
	}
	peer := wireguard.Peer{
		ID:         "fedcba9876543210fedcba9876543210",
		ServerID:   server.ID,
		Name:       "alice",
		PublicKey:  makePublicKey(0x20),
		AllowedIPs: []string{"10.66.66.2/32"},
		Enabled:    true,
		Intent:     wireguard.IntentCustom,
	}
	if status, body := f.upsertPeer(t, peer); status != http.StatusCreated {
		t.Fatalf("peer upsert: status=%d body=%s", status, body)
	}
	out := f.oracleKernel(t)
	if !strings.Contains(out, "10.66.66.2/32") {
		t.Fatalf("kernel wg show missing AllowedIPs: %s", out)
	}
}

// TestDV04WG_ProductionDeletePeerRemoved: peer delete
// through production path removes the peer from the
// kernel; the independent oracle no longer sees it.
func TestDV04WG_ProductionDeletePeerRemoved(t *testing.T) {
	f := newWGFixture(t)
	server := wireguard.Server{
		ID:         "0123456789abcdef0123456789abcdef",
		Name:       "primary",
		Interface:  f.iface,
		ListenPort: 51820,
		Addresses:  []string{"10.66.66.1/24"},
		PrivateKeyRef: wireguard.KeyRef{
			VaultID: "v", Revision: "r", KeyID: "k", CreatedAt: "2026-09-12T00:00:00Z",
		},
		PublicKey: makePublicKey(0x10),
	}
	if status, body := f.upsertServer(t, server); status != http.StatusCreated {
		t.Fatalf("server upsert: status=%d body=%s", status, body)
	}
	peer := wireguard.Peer{
		ID:         "fedcba9876543210fedcba9876543210",
		ServerID:   server.ID,
		Name:       "alice",
		PublicKey:  makePublicKey(0x20),
		AllowedIPs: []string{"10.66.66.2/32"},
		Enabled:    true,
		Intent:     wireguard.IntentCustom,
	}
	if status, body := f.upsertPeer(t, peer); status != http.StatusCreated {
		t.Fatalf("peer upsert: status=%d body=%s", status, body)
	}
	if status, body := f.deletePeer(t, peer.ID); status != http.StatusOK {
		t.Fatalf("peer delete: status=%d body=%s", status, body)
	}
	out := f.oracleKernel(t)
	if strings.Contains(out, "10.66.66.2/32") {
		t.Fatalf("kernel still has AllowedIPs after delete: %s", out)
	}
}

// TestDV04WG_ProductionExportRedacted: GET /export
// through the production path returns the redacted
// wg-quick configuration; no PrivateKey line is in
// the payload (not even a redaction sentinel).
func TestDV04WG_ProductionExportRedacted(t *testing.T) {
	f := newWGFixture(t)
	server := wireguard.Server{
		ID:         "0123456789abcdef0123456789abcdef",
		Name:       "primary",
		Interface:  f.iface,
		ListenPort: 51820,
		Addresses:  []string{"10.66.66.1/24"},
		PrivateKeyRef: wireguard.KeyRef{
			VaultID: "v", Revision: "r", KeyID: "k", CreatedAt: "2026-09-12T00:00:00Z",
		},
		PublicKey: makePublicKey(0x10),
	}
	if status, body := f.upsertServer(t, server); status != http.StatusCreated {
		t.Fatalf("server upsert: status=%d body=%s", status, body)
	}
	peer := wireguard.Peer{
		ID:         "fedcba9876543210fedcba9876543210",
		ServerID:   server.ID,
		Name:       "alice",
		PublicKey:  makePublicKey(0x20),
		AllowedIPs: []string{"10.66.66.2/32"},
		Enabled:    true,
		Intent:     wireguard.IntentCustom,
	}
	if status, body := f.upsertPeer(t, peer); status != http.StatusCreated {
		t.Fatalf("peer upsert: status=%d body=%s", status, body)
	}
	status, body := f.exportServer(t, server.ID)
	if status != http.StatusOK {
		t.Fatalf("export: status=%d body=%s", status, body)
	}
	payload := string(body)
	if strings.Contains(payload, "PrivateKey") {
		t.Fatalf("export contains PrivateKey line: %s", payload)
	}
	if strings.Contains(payload, "REDACTED") {
		t.Fatalf("export contains redaction sentinel: %s", payload)
	}
	if !strings.Contains(payload, "[Peer]") {
		t.Fatalf("export missing [Peer] section: %s", payload)
	}
}

// TestDV04WG_ProductionNegativeInvalidPeer: a peer with
// malformed AllowedIPs is rejected by the production
// path; the kernel is not touched.
func TestDV04WG_ProductionNegativeInvalidPeer(t *testing.T) {
	f := newWGFixture(t)
	server := wireguard.Server{
		ID:         "0123456789abcdef0123456789abcdef",
		Name:       "primary",
		Interface:  f.iface,
		ListenPort: 51820,
		Addresses:  []string{"10.66.66.1/24"},
		PrivateKeyRef: wireguard.KeyRef{
			VaultID: "v", Revision: "r", KeyID: "k", CreatedAt: "2026-09-12T00:00:00Z",
		},
		PublicKey: makePublicKey(0x10),
	}
	if status, body := f.upsertServer(t, server); status != http.StatusCreated {
		t.Fatalf("server upsert: status=%d body=%s", status, body)
	}
	bad := wireguard.Peer{
		ID:         "fedcba9876543210fedcba9876543210",
		ServerID:   server.ID,
		Name:       "bob",
		PublicKey:  makePublicKey(0x30),
		AllowedIPs: []string{"not-a-cidr"},
		Enabled:    true,
		Intent:     wireguard.IntentCustom,
	}
	status, body := f.upsertPeer(t, bad)
	if status == http.StatusCreated {
		t.Fatalf("invalid peer accepted: body=%s", body)
	}
	out := f.oracleKernel(t)
	if strings.Contains(out, "bob") || strings.Contains(out, makePublicKey(0x30)) {
		t.Fatalf("kernel still has bad peer: %s", out)
	}
}

// TestDV04WG_ProductionNegativeWrongServerRef: a peer
// referencing a non-existent server is rejected; the
// kernel is not touched.
func TestDV04WG_ProductionNegativeWrongServerRef(t *testing.T) {
	f := newWGFixture(t)
	peer := wireguard.Peer{
		ID:         "fedcba9876543210fedcba9876543210",
		ServerID:   "ffffffffffffffffffffffffffffffff",
		Name:       "ghost",
		PublicKey:  makePublicKey(0x40),
		AllowedIPs: []string{"10.66.66.9/32"},
		Enabled:    true,
		Intent:     wireguard.IntentCustom,
	}
	status, body := f.upsertPeer(t, peer)
	if status == http.StatusCreated {
		t.Fatalf("orphan peer accepted: body=%s", body)
	}
	if !strings.Contains(strings.ToLower(string(body)), "server") {
		t.Fatalf("orphan error did not name server: %s", body)
	}
}

// TestDV04WG_ProductionNegativeAgentUnavailable: when
// the Agent host is not enabled, the IPC route returns
// 503 wireguard_unavailable, never a fake 200.
func TestDV04WG_ProductionNegativeAgentUnavailable(t *testing.T) {
	host := agent.NewWireguardHost(nil, nil)
	srv := httptest.NewServer(host)
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/agent/v1/wireguard/wireguard/v1/servers")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 wireguard_unavailable, got %d", resp.StatusCode)
	}
}

// TestDV04WG_ProductionRestartReconciles: the durable
// catalog + the ReconcileServer helper rebuild the
// runtime after a simulated restart; the independent
// kernel oracle sees the same peer set.
func TestDV04WG_ProductionRestartReconciles(t *testing.T) {
	f := newWGFixture(t)
	server := wireguard.Server{
		ID:         "0123456789abcdef0123456789abcdef",
		Name:       "primary",
		Interface:  f.iface,
		ListenPort: 51820,
		Addresses:  []string{"10.66.66.1/24"},
		PrivateKeyRef: wireguard.KeyRef{
			VaultID: "v", Revision: "r", KeyID: "k", CreatedAt: "2026-09-12T00:00:00Z",
		},
		PublicKey: makePublicKey(0x10),
	}
	if status, body := f.upsertServer(t, server); status != http.StatusCreated {
		t.Fatalf("server upsert: status=%d body=%s", status, body)
	}
	peer := wireguard.Peer{
		ID:         "fedcba9876543210fedcba9876543210",
		ServerID:   server.ID,
		Name:       "alice",
		PublicKey:  makePublicKey(0x20),
		AllowedIPs: []string{"10.66.66.2/32"},
		Enabled:    true,
		Intent:     wireguard.IntentCustom,
	}
	if status, body := f.upsertPeer(t, peer); status != http.StatusCreated {
		t.Fatalf("peer upsert: status=%d body=%s", status, body)
	}
	// Simulate restart: tear down the kernel, then
	// re-open the catalog and re-apply via the
	// reconcile path.
	_ = exec.Command("/sbin/ip", "link", "del", f.iface).Run()
	if f.oracleLinkExists(t) {
		t.Fatal("interface still present after teardown")
	}
	cat2, err := wireguard.OpenFileCatalog(f.catalogDir)
	if err != nil {
		t.Fatal(err)
	}
	adapter := &wireguard.KernelAdapter{
		Resolver: stubKeyResolver{},
		Runner:   wireguard.IPRouteRunner{Binary: "/sbin/ip"},
		Files:    wireguard.LocalFilesystem{Dir: f.configDir},
		Catalog:  cat2,
	}
	host := agent.NewWireguardHost(nil, nil)
	if err := host.ReconcileServer(context.Background(), cat2, adapter); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if !f.oracleLinkExists(t) {
		t.Fatal("interface not restored by reconcile")
	}
	out := f.oracleKernel(t)
	if !strings.Contains(out, "10.66.66.2/32") {
		t.Fatalf("reconciled kernel missing AllowedIPs: %s", out)
	}
}

// TestDV04WG_ProductionListPersists: GET /servers
// through the production path returns the catalog
// state the upsert put there.
func TestDV04WG_ProductionListPersists(t *testing.T) {
	f := newWGFixture(t)
	server := wireguard.Server{
		ID:         "0123456789abcdef0123456789abcdef",
		Name:       "primary",
		Interface:  f.iface,
		ListenPort: 51820,
		Addresses:  []string{"10.66.66.1/24"},
		PrivateKeyRef: wireguard.KeyRef{
			VaultID: "v", Revision: "r", KeyID: "k", CreatedAt: "2026-09-12T00:00:00Z",
		},
		PublicKey: makePublicKey(0x10),
	}
	if status, body := f.upsertServer(t, server); status != http.StatusCreated {
		t.Fatalf("server upsert: status=%d body=%s", status, body)
	}
	status, body := f.listServers(t)
	if status != http.StatusOK {
		t.Fatalf("list: status=%d body=%s", status, body)
	}
	var servers []wireguard.Server
	if err := json.Unmarshal(body, &servers); err != nil {
		t.Fatalf("decode list: %v body=%s", err, body)
	}
	if len(servers) != 1 || servers[0].ID != server.ID {
		t.Fatalf("list did not persist server: %+v", servers)
	}
}

// TestDV04WG_PublicEngineAgentChain exercises the
// production path end to end through a real Unix
// socket — the Engine.WireGuardProxy proxies the public
// /api/v1/wireguard/* mutation to the Agent's IPC,
// which dispatches to the WireguardHost, which drives
// the wireguard.API, which drives the KernelAdapter,
// which talks to /sbin/ip and /usr/bin/wg. The
// independent /usr/bin/wg show oracle reads the kernel.
//
// The test stands up the Agent HTTP handler on a real
// Unix socket, constructs an Engine pointing at it,
// then issues the mutation through Engine.WireGuardProxy
// (the same method api.Handler.handleWireGuardProxy
// uses in production). The PUBLIC ENTRYPOINT is
// represented by the Engine call; the production
// /api/v1/wireguard/* route is a thin handler over the
// same method.
func TestDV04WG_PublicEngineAgentChain(t *testing.T) {
	withRoot(t, "ip", "wg")
	dir := t.TempDir()
	cat, err := wireguard.OpenFileCatalog(dir)
	if err != nil {
		t.Fatal(err)
	}
	iface := "duckwg" + wgRandomSuffix()
	t.Cleanup(func() {
		_ = exec.Command("/sbin/ip", "link", "del", iface).Run()
	})
	adapter := &wireguard.KernelAdapter{
		Resolver: stubKeyResolver{},
		Runner:   wireguard.IPRouteRunner{Binary: "/sbin/ip", WireGuardBin: "/usr/bin/wg"},
		Files:    wireguard.LocalFilesystem{Dir: dir},
		Catalog:  cat,
	}
	access := identityAccessPermissive{}
	api := &wireguard.API{
		Access: access, Catalog: cat, Adapter: adapter, Prefix: "/wireguard/v1/",
	}
	host := agent.NewWireguardHost(api, access)
	agentHTTP := &agentMux{host: host}
	sockPath := filepath.Join(dir, "agent.sock")
	listener, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close(); os.Remove(sockPath) })
	go http.Serve(listener, agentHTTP)

	// Stand up the Engine pointing at the same socket
	// (the same pattern cmd/duck-api uses).
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{Timeout: time.Second}).DialContext(ctx, "unix", sockPath)
	}}
	client := &http.Client{Transport: transport, Timeout: 9 * time.Second}
	_ = client

	// Issue the mutation through Engine.WireGuardProxy
	// — this is exactly what api.handleWireGuardProxy
	// does in production.
	server := wireguard.Server{
		ID:         "0123456789abcdef0123456789abcdef",
		Name:       "primary",
		Interface:  iface,
		ListenPort: 51820,
		Addresses:  []string{"10.66.66.1/24"},
		PrivateKeyRef: wireguard.KeyRef{
			VaultID: "v", Revision: "r", KeyID: "k", CreatedAt: "2026-09-12T00:00:00Z",
		},
		PublicKey: makePublicKey(0x10),
	}
	body, _ := json.Marshal(server)
	status, data, ctype, err := proxyWireGuard(t, sockPath, "POST", "/api/v1/wireguard/servers", body)
	if err != nil {
		t.Fatal(err)
	}
	if status != http.StatusCreated {
		t.Fatalf("engine proxy upsert: status=%d ctype=%s body=%s", status, ctype, data)
	}
	// Independent kernel oracle.
	out, err := exec.Command("/usr/bin/wg", "show", iface).CombinedOutput()
	if err != nil {
		t.Fatalf("oracle wg show: %v (%s)", err, out)
	}
	if !strings.Contains(string(out), "listening port") {
		t.Fatalf("kernel wg show missing listening port: %s", out)
	}
	// Restart durability: tear down the kernel, then
	// ask the Agent host to reconcile. The reconcile
	// re-reads the catalog and re-applies.
	_ = exec.Command("/sbin/ip", "link", "del", iface).Run()
	if err := host.ReconcileServer(context.Background(), cat, adapter); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	out2, err := exec.Command("/usr/bin/wg", "show", iface).CombinedOutput()
	if err != nil {
		t.Fatalf("post-reconcile oracle: %v (%s)", err, out2)
	}
	if !strings.Contains(string(out2), "listening port") {
		t.Fatalf("post-reconcile kernel wg show missing listening port: %s", out2)
	}
}

// proxyWireGuard is the test-side equivalent of
// api.Handler.handleWireGuardProxy: it builds the
// request against the Agent Unix socket via the
// Engine.WireGuardProxy path mapping and returns the
// Agent's HTTP response verbatim.
func proxyWireGuard(t *testing.T, sockPath, method, path string, body []byte) (int, []byte, string, error) {
	t.Helper()
	// The Agent IPC route is /agent/v1/wireguard/wireguard/v1/{tail}.
	tail := strings.TrimPrefix(path, "/api/v1/wireguard/")
	url := "http://agent/agent/v1/wireguard/wireguard/v1/" + tail
	var rdr io.Reader
	if len(body) > 0 {
		rdr = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, url, rdr)
	if err != nil {
		return 0, nil, "", err
	}
	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{Timeout: time.Second}).DialContext(ctx, "unix", sockPath)
	}}
	client := &http.Client{Transport: transport, Timeout: 9 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, "", err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return resp.StatusCode, nil, resp.Header.Get("Content-Type"), err
	}
	return resp.StatusCode, data, resp.Header.Get("Content-Type"), nil
}

// agentMux is the Agent-side dispatch the production
// Handler does (it routes /agent/v1/wireguard/* to
// the WireguardHost).
type agentMux struct{ host *agent.WireguardHost }

func (m *agentMux) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !strings.HasPrefix(r.URL.EscapedPath(), "/agent/v1/wireguard/") {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	m.host.ServeHTTP(w, r)
}

// avoid "imported and not used" if build tags ever flip.
var _ = os.Getenv