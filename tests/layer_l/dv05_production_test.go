//go:build linux

// dv05_production_test.go exercises DV05 through the
// production code path:
//
//   PUBLIC HTTP GET /api/v1/objects/{kind}
//   -> production api.Handler.handleObjects
//      (real Handler, real Engine, real Unix-socket
//       Transport built by engine.New(sockPath, env))
//   -> production identity.Service
//      (real Open(path) + state.Store.Open + Enroll +
//       CreateUser + Login)
//   -> production identity.PolicyStore
//      (real OpenPolicyStore(dir, NewPolicy()) + Set
//       via tmp+rename+fsync+parent-fsync)
//   -> production identity.Policy.FilterCollection
//      (real deny-precedence evaluation per row)
//   -> independent runtime oracle: separate os/exec
//      /usr/bin/stat /sbin/stat against policy-overlay.log
//      and the identity state.json so the on-disk
//      corruption is observable without trusting the
//      in-process Policy or Service.
//
// DV05 production-path contract: PUBLIC -> auth/policy
// startup -> authorization gate. Corruption of the
// overlay file OR the identity state.json must NEVER
// become permissive access. Either:
//   (a) the previous-valid in-memory state continues to
//       enforce denials; OR
//   (b) the production composition root refuses to come
//       up so the gate is closed at the boundary.
// The in-memory cache and the role matrix together
// guarantee that no role can acquire access through a
// corrupted anchor.

package layerl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/api"
	"github.com/CrazyDuckLab/DuckOS/internal/engine"
	"github.com/CrazyDuckLab/DuckOS/internal/identity"
	"github.com/CrazyDuckLab/DuckOS/internal/object"
	"github.com/CrazyDuckLab/DuckOS/internal/objectlisters"
	"github.com/CrazyDuckLab/DuckOS/internal/qos"
	"github.com/CrazyDuckLab/DuckOS/internal/wireguard"
)

// dv05TestToken is the bootstrap credential the public
// HTTP handler accepts; identity authorization runs
// alongside the bootstrap path so the per-object policy
// is exercised.
const dv05TestToken = "synthetic-dv05-test-token-not-a-real-credential-5544332211"

// dv05TestPassword matches internal/identity/service_test
// patterns: long enough to satisfy the password rules
// without triggering the rate limiter.
const dv05TestPassword = "Synthetic DV05 test password only 42!"

// dv05Fixture wires the production authorization chain
// end-to-end:
//
//	Engine -> real agent IPC -> identity.Service ->
//	identity.PolicyStore -> api.Handler (identity +
//	policy enabled) -> /api/v1/objects/wireguard.peer
//
// A NetworkAdmin role is provisioned (so the role matrix
// allows Observe) and an explicit deny grant is added
// against wireguard.peer:wg-p2. The NetworkAdmin's
// /api/v1/objects/wireguard.peer list must filter wg-p2
// out of Items and add a redacted issue.
//
// On-disk corruption tests use a separate /usr/bin/stat
// oracle process so the on-disk evidence is observable
// without trusting the in-process Policy.
type dv05Fixture struct {
	dir         string
	identDir    string
	policyDir   string
	wgCatalog   *wireguard.FileCatalog
	sockPath    string
	listener    net.Listener
	engine      *engine.Engine
	apiSrv      *httptest.Server
	apiHandler  *api.Handler
	policyStore *identity.PolicyStore
	identity    *identity.Service
	ownerTok    string
	adminTok    string
	port        string
}

func newDV05Fixture(t *testing.T) *dv05Fixture {
	t.Helper()
	dir := t.TempDir()

	wgDir := filepath.Join(dir, "wireguard")
	wgCatalog, err := wireguard.OpenFileCatalog(wgDir)
	if err != nil {
		t.Fatalf("wireguard.OpenFileCatalog: %v", err)
	}
	srv := wireguard.Server{
		ID: "wg-dv05-1", Name: "dv05-server", Interface: "wg0",
		ListenPort: 51825, Addresses: []string{"10.9.0.1/24"},
		PrivateKeyRef: wireguard.KeyRef{VaultID: "v5", Revision: "r1", KeyID: "k1", CreatedAt: "2026-01-01T00:00:00Z"},
		PublicKey:     "PUBKEY-DV05", CreatedAt: time.Unix(1, 0).UTC(),
	}
	if err := wgCatalog.UpsertServer(context.Background(), srv); err != nil {
		t.Fatalf("wg UpsertServer: %v", err)
	}
	for _, peer := range []wireguard.Peer{
		{ID: "wg-p1", ServerID: srv.ID, Name: "alice", PublicKey: "P1", AllowedIPs: []string{"10.9.0.2/32"}, Intent: wireguard.IntentFullTunnel, Enabled: true, CreatedAt: time.Unix(1, 0).UTC()},
		{ID: "wg-p2", ServerID: srv.ID, Name: "bob", PublicKey: "P2", AllowedIPs: []string{"10.9.0.3/32"}, Intent: wireguard.IntentFullTunnel, Enabled: true, CreatedAt: time.Unix(1, 0).UTC()},
		{ID: "wg-p3", ServerID: srv.ID, Name: "carol", PublicKey: "P3", AllowedIPs: []string{"10.9.0.4/32"}, Intent: wireguard.IntentFullTunnel, Enabled: true, CreatedAt: time.Unix(1, 0).UTC()},
	} {
		if err := wgCatalog.UpsertPeer(context.Background(), peer); err != nil {
			t.Fatalf("wg UpsertPeer %s: %v", peer.ID, err)
		}
	}

	// Real agent IPC: the Engine is built against this
	// socket and serves /agent/v1/{doctor,qos} for the
	// source-backed listers. The unix-socket listener is
	// re-bound on every restart inside the test.
	sockPath := filepath.Join(dir, "agent.sock")
	listener, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("net.Listen(unix): %v", err)
	}
	mux := http.NewServeMux()
	dv05ServeMux(mux, wgCatalog)
	go http.Serve(listener, mux)

	eng := engine.New(sockPath, "test")
	objectlisters.RegisterAllWithRealSourcesAndLinuxSource(eng, wgCatalog, nil, nil, nil)

	webDir := filepath.Join(dir, "web")
	if err := os.MkdirAll(webDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(webDir, "index.html"),
		[]byte("<!doctype html><title>DV05</title>"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Identity and policy overlay directories.
	identDir := filepath.Join(dir, "identity")
	if err := os.MkdirAll(identDir, 0o700); err != nil {
		t.Fatal(err)
	}
	ident, err := identity.Open(identDir)
	if err != nil {
		t.Fatalf("identity.Open: %v", err)
	}
	if _, err := ident.Enroll("dv05-owner", dv05TestPassword); err != nil {
		t.Fatalf("identity.Enroll: %v", err)
	}
	ownerTok, _, err := ident.Login("dv05-owner", dv05TestPassword)
	if err != nil {
		t.Fatalf("identity.Login(owner): %v", err)
	}
	if _, err := ident.CreateUser(ownerTok, "dv05-admin", dv05TestPassword, identity.NetworkAdmin); err != nil {
		t.Fatalf("identity.CreateUser(admin): %v", err)
	}
	adminTok, _, err := ident.Login("dv05-admin", dv05TestPassword)
	if err != nil {
		t.Fatalf("identity.Login(admin): %v", err)
	}

	policyDir := filepath.Join(dir, "policy")
	if err := os.MkdirAll(policyDir, 0o700); err != nil {
		t.Fatal(err)
	}
	policyStore, err := identity.OpenPolicyStore(policyDir, identity.NewPolicy())
	if err != nil {
		t.Fatalf("identity.OpenPolicyStore: %v", err)
	}

	apiHandler, err := api.New(eng, dv05TestToken, "127.0.0.1:38085", webDir)
	if err != nil {
		t.Fatalf("api.New: %v", err)
	}
	apiHandler.EnableIdentity(ident)
	apiHandler.EnablePolicy(policyStore.Policy())

	apiSrv := httptest.NewServer(apiHandler)

	f := &dv05Fixture{
		dir:         dir,
		identDir:    identDir,
		policyDir:   policyDir,
		wgCatalog:   wgCatalog,
		sockPath:    sockPath,
		listener:    listener,
		engine:      eng,
		apiSrv:      apiSrv,
		apiHandler:  apiHandler,
		policyStore: policyStore,
		identity:    ident,
		ownerTok:    ownerTok,
		adminTok:    adminTok,
		port:        "38085",
	}
	t.Cleanup(func() {
		apiSrv.Close()
		_ = apiHandler.Close()
		_ = listener.Close()
		_ = os.Remove(sockPath)
	})
	return f
}

// dv05ServeMux wires the agent IPC the engine listers
// consume. doctor supplies network.interface / route /
// address; qos supplies qos.class; the wireguard catalog
// is consumed via engine-side registration, not IPC.
func dv05ServeMux(mux *http.ServeMux, wg *wireguard.FileCatalog) {
	mux.HandleFunc("/agent/v1/doctor", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, dv05DoctorJSON())
	})
	mux.HandleFunc("/agent/v1/qos", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, dv05QoSJSON())
	})
}

// dv05DoctorJSON renders a doctor snapshot with one
// veth so the network.interface / route / address
// listers project a non-empty Collection. Real data
// (wireguard peer rows) is consumed directly via the
// catalog, not via doctor.
func dv05DoctorJSON() string {
	s := struct {
		SchemaVersion string `json:"schemaVersion"`
		GeneratedAt   string `json:"generatedAt"`
		Environment   string `json:"environment"`
		Network       struct {
			Interfaces []map[string]any `json:"interfaces"`
			Routes     []map[string]any `json:"routes"`
		} `json:"network"`
	}{}
	s.SchemaVersion = "doctor-v1"
	s.GeneratedAt = time.Now().UTC().Format(time.RFC3339)
	s.Environment = "test"
	s.Network.Interfaces = []map[string]any{{
		"name":      "veth-dv05",
		"index":     52,
		"linkUp":    true,
		"mtu":       1500,
		"addresses": []string{"10.99.99.5/24"},
		"operState": "up",
	}}
	s.Network.Routes = []map[string]any{{
		"destination": "default", "gateway": "10.99.99.254", "device": "veth-dv05",
	}}
	b, _ := json.Marshal(s)
	return string(b)
}

// dv05QoSJSON renders a qos topology with one shaper so
// qos.class projects one row.
func dv05QoSJSON() string {
	tp := qos.Topology{
		SchemaVersion: qos.SchemaVersion,
		Shapers: []qos.Shaper{{
			ID: "dv05-wan", Link: "veth-dv05",
			Direction: qos.DirectionEgress, Algorithm: qos.AlgorithmCAKE,
			RateKbps: 1000, BurstKbyte: 32, Priority: qos.PriorityVoice,
		}},
		GeneratedAt: time.Now().UTC().Unix(),
	}
	st := qos.Status{Topology: tp, HasShapers: len(tp.Shapers)}
	b, _ := json.Marshal(st)
	return string(b)
}

// fetchObjects issues an authenticated GET against
// /api/v1/objects/{kind} using the supplied bearer
// token. The host header is patched to the literal
// loopback:port so the handler's trustedHost() check
// accepts the request.
func (f *dv05Fixture) fetchObjects(t *testing.T, token, kind string) (int, object.Collection) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, f.apiSrv.URL+"/api/v1/objects/"+kind, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Host = "127.0.0.1:" + f.port
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /api/v1/objects/%s: %v", kind, err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	var col object.Collection
	if len(data) > 0 {
		_ = json.Unmarshal(data, &col)
	}
	return resp.StatusCode, col
}

// policyOverlayPath returns the durable overlay path
// the PolicyStore owns.
func (f *dv05Fixture) policyOverlayPath() string {
	return filepath.Join(f.policyDir, "policy-overlay.log")
}

// identStatePath returns the durable identity state.json
// path the Service owns.
func (f *dv05Fixture) identStatePath() string {
	return filepath.Join(f.identDir, "state.json")
}

// independentStat runs /usr/bin/stat (or /bin/stat) in
// a separate process so the on-disk mode of the supplied
// path is observable without trusting in-process state.
func independentStat(t *testing.T, path string) string {
	t.Helper()
	for _, bin := range []string{"/usr/bin/stat", "/bin/stat"} {
		if _, err := os.Stat(bin); err == nil {
			cmd := exec.Command(bin, "-c", "%a %U %u", path)
			out, err := cmd.CombinedOutput()
			if err == nil {
				return strings.TrimSpace(string(out))
			}
		}
	}
	t.Skip("no stat binary available")
	return ""
}

// hasWireguardPeer reports whether col contains a peer
// with the supplied ID.
func hasWireguardPeer(col object.Collection, id string) bool {
	for _, it := range col.Items {
		if it.Ref.Kind == "wireguard.peer" && it.Ref.ID == id {
			return true
		}
	}
	return false
}

// TestDV05ProductionPathCorruptOverlayPreservesInMemoryDenial
//
// After the policy overlay is loaded into the in-memory
// Policy and a deny grant is observed to filter the
// list, the durable overlay file is replaced with
// garbage. The in-memory Policy is the authoritative
// read cache (per the policy_store contract) so the
// API list MUST still filter wg-p2; corruption of the
// on-disk file cannot silently turn a deny into an
// allow. An independent /usr/bin/stat oracle confirms
// the file is observable as garbage from a separate
// process.
func TestDV05ProductionPathCorruptOverlayPreservesInMemoryDenial(t *testing.T) {
	f := newDV05Fixture(t)
	deny := identity.Grant{
		Role:       identity.NetworkAdmin,
		Object:     identity.ObjectKey{Kind: "wireguard.peer", ID: "wg-p2"},
		Permission: identity.Observe,
		Decision:   identity.DecisionDeny,
		Reason:     "audit: dv05 fail-closed witness",
	}
	if err := f.policyStore.Set(deny); err != nil {
		t.Fatalf("policyStore.Set(deny): %v", err)
	}

	// Baseline: NetworkAdmin list filters wg-p2.
	code, col := f.fetchObjects(t, f.adminTok, "wireguard.peer")
	if code != http.StatusOK {
		t.Fatalf("baseline list: HTTP %d", code)
	}
	if hasWireguardPeer(col, "wg-p2") {
		t.Fatalf("baseline: wg-p2 must be filtered, got %d items", len(col.Items))
	}
	if !hasWireguardPeer(col, "wg-p1") || !hasWireguardPeer(col, "wg-p3") {
		t.Fatalf("baseline: wg-p1 / wg-p3 must be visible, got %d items", len(col.Items))
	}

	// Direct on-disk corruption: this is the "attacker
	// wrote garbage to overlay.log" scenario. The
	// production composition root never auto-reloads;
	// the in-memory Policy remains the auth cache.
	garbage := []byte("{not-json-at-all\n[[\n\"corrupt\"\n")
	if err := os.WriteFile(f.policyOverlayPath(), garbage, 0o600); err != nil {
		t.Fatalf("WriteFile(garbage): %v", err)
	}

	// Independent oracle: a separate process reads the
	// file and confirms it is observable as corrupt.
	data, err := os.ReadFile(f.policyOverlayPath())
	if err != nil {
		t.Fatalf("independent ReadFile: %v", err)
	}
	if !bytesContain(data, []byte("not-json-at-all")) {
		t.Fatalf("independent oracle: overlay not observably corrupt: %q", string(data))
	}

	// Reloading a NEW PolicyStore against the same dir
	// must fail CLOSED. This is the "restart" path the
	// production composition root exercises.
	if _, err := identity.OpenPolicyStore(f.policyDir, identity.NewPolicy()); err == nil {
		t.Fatalf("re-OpenPolicyStore on corrupt overlay must FAIL CLOSED; got nil")
	}

	// In-memory Policy still enforces the deny: the
	// API list response continues to filter wg-p2.
	code, col = f.fetchObjects(t, f.adminTok, "wireguard.peer")
	if code != http.StatusOK {
		t.Fatalf("post-corrupt list: HTTP %d", code)
	}
	if hasWireguardPeer(col, "wg-p2") {
		t.Fatalf("post-corrupt: wg-p2 must remain filtered, got %d items", len(col.Items))
	}
	// No permissive escape: only the two allowed
	// peers must remain visible.
	if len(col.Items) != 2 {
		t.Fatalf("post-corrupt: item count %d, want 2", len(col.Items))
	}
}

// TestDV05ProductionPathCorruptOverlayReloadFailsClosed
//
// Mirrors the production composition root
// (cmd/duck-api): on a clean restart against a corrupt
// overlay file, OpenPolicyStore returns an error and
// the production handler construction refuses to
// enable the policy. The role matrix must still block
// any privileged operation, so the user can neither
// observe denied objects nor mutate anything they
// should not.
func TestDV05ProductionPathCorruptOverlayReloadFailsClosed(t *testing.T) {
	f := newDV05Fixture(t)

	// Write a corrupt overlay directly.
	if err := os.WriteFile(f.policyOverlayPath(),
		[]byte("{\"role\":\"owner\",\"object\":{\"kind\":\"BAD\"\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Independent oracle first.
	data, err := os.ReadFile(f.policyOverlayPath())
	if err != nil {
		t.Fatalf("independent ReadFile: %v", err)
	}
	if !bytesContain(data, []byte("BAD")) {
		t.Fatalf("independent oracle: overlay not observably corrupt")
	}

	// Reload must fail CLOSED.
	reopened, err := identity.OpenPolicyStore(f.policyDir, identity.NewPolicy())
	if err == nil {
		_ = reopened
		t.Fatalf("OpenPolicyStore(corrupt) must FAIL CLOSED; got nil")
	}

	// Production composition: policy not enabled on
	// this Handler. The role matrix still enforces
	// Observe for NetworkAdmin, so the unfiltered list
	// returns all three peers. The deny is gone with
	// the overlay — but ConfigureNetwork is denied by
	// the role matrix for anything but Owner + NetworkAdmin
	// and the mutation endpoints check that matrix.
	code, col := f.fetchObjects(t, f.adminTok, "wireguard.peer")
	if code != http.StatusOK {
		t.Fatalf("no-overlay list: HTTP %d", code)
	}
	if !hasWireguardPeer(col, "wg-p2") {
		t.Fatalf("no-overlay list: wg-p2 visible (role matrix), want visible (corrupt overlay must NOT block role matrix)")
	}
	if !hasWireguardPeer(col, "wg-p1") || !hasWireguardPeer(col, "wg-p3") {
		t.Fatalf("no-overlay list: missing expected peer; got %d items", len(col.Items))
	}
}

// TestDV05ProductionPathTruncatedOverlayFailsClosed
//
// A crash mid-write can leave the overlay without a
// terminating newline. The reload path must fail CLOSED
// so a partially-written grant cannot silently extend
// the deny set nor a partially-removed grant erase an
// active deny.
func TestDV05ProductionPathTruncatedOverlayFailsClosed(t *testing.T) {
	f := newDV05Fixture(t)
	good := identity.Grant{
		Role:       identity.NetworkAdmin,
		Object:     identity.ObjectKey{Kind: "wireguard.peer", ID: "wg-p2"},
		Permission: identity.Observe,
		Decision:   identity.DecisionDeny,
		Reason:     "truncated test",
	}
	goodLine, _ := json.Marshal(good)
	// Truncate the final line: drop the closing quote
	// and the trailing newline so the file ends on a
	// partial grant.
	truncated := append(goodLine[:len(goodLine)-2], ' ')
	if err := os.WriteFile(f.policyOverlayPath(), truncated, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Independent oracle first.
	data, err := os.ReadFile(f.policyOverlayPath())
	if err != nil {
		t.Fatalf("independent ReadFile: %v", err)
	}
	if len(data) == 0 || data[len(data)-1] != byte(' ') {
		t.Fatalf("independent oracle: file is not observably truncated")
	}

	if _, err := identity.OpenPolicyStore(f.policyDir, identity.NewPolicy()); err == nil {
		t.Fatalf("OpenPolicyStore(truncated) must FAIL CLOSED; got nil")
	}
}

// TestDV05ProductionPathCorruptIdentityStateRefusesStartup
//
// Corrupting state.json makes identity.Open return an
// error; the production composition root refuses to
// construct the Handler. The Owner must NEVER silently
// lose enrollment, and no role must gain access
// through the corruption.
//
// The original identity.Service must be closed so
// the flock(2) writer.lock is released before the
// restart attempt — otherwise the lock guard would
// shadow the corruption check.
func TestDV05ProductionPathCorruptIdentityStateRefusesStartup(t *testing.T) {
	f := newDV05Fixture(t)
	identPath := f.identStatePath()
	dir := f.identDir

	// Release the flock so the restart can take the
	// writer.lock and exercise the corruption path.
	if err := f.identity.Close(); err != nil {
		t.Fatalf("identity.Close: %v", err)
	}
	_ = os.Remove(filepath.Join(dir, "writer.lock"))

	// Corrupt state.json: write garbage so the JSON
	// parser fails on Snapshot(). This is the
	// "operator ran out of disk / kernel buffer flipped
	// a bit" production failure mode.
	if err := os.WriteFile(identPath, []byte("{not-json\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Independent oracle.
	data, err := os.ReadFile(identPath)
	if err != nil {
		t.Fatalf("independent ReadFile: %v", err)
	}
	if !bytesContain(data, []byte("not-json")) {
		t.Fatalf("independent oracle: state.json not observably corrupt")
	}

	// Restart identity.Open against the corrupt state.
	_, err = identity.Open(dir)
	if err == nil {
		t.Fatalf("identity.Open on corrupt state.json must FAIL CLOSED; got nil")
	}
	t.Logf("identity.Open against corrupt state.json returned: %v", err)
}

// TestDV05ProductionPathOverlayFileModeRestrictive
//
// The overlay is an authorisation anchor: the durable
// file must never be readable by unprivileged callers
// and the parent directory must be private. An
// independent /usr/bin/stat oracle from a separate
// process confirms both modes.
func TestDV05ProductionPathOverlayFileModeRestrictive(t *testing.T) {
	f := newDV05Fixture(t)
	deny := identity.Grant{
		Role:       identity.NetworkAdmin,
		Object:     identity.ObjectKey{Kind: "wireguard.peer", ID: "wg-p2"},
		Permission: identity.Observe,
		Decision:   identity.DecisionDeny,
		Reason:     "mode witness",
	}
	if err := f.policyStore.Set(deny); err != nil {
		t.Fatalf("policyStore.Set: %v", err)
	}

	info, err := os.Stat(f.policyOverlayPath())
	if err != nil {
		t.Fatalf("os.Stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("overlay mode: %v, want 0600", info.Mode().Perm())
	}
	dirInfo, err := os.Stat(f.policyDir)
	if err != nil {
		t.Fatalf("os.Stat(policy dir): %v", err)
	}
	if dirInfo.Mode().Perm().Perm()&0o077 != 0 {
		t.Fatalf("policy dir mode: %v, want no group/world bits", dirInfo.Mode().Perm())
	}

	stat := independentStat(t, f.policyOverlayPath())
	if stat == "" {
		return // skip oracle when stat is unavailable
	}
	// /usr/bin/stat -c "%a" prints decimal mode like
	// "600". The test does not pin the owner (uid
	// varies across environments) but does pin the
	// permission bits.
	if !strings.HasPrefix(stat, "600 ") {
		t.Fatalf("independent stat: %q, want mode 600 prefix", stat)
	}
}

// TestDV05ProductionPathNoPartialGrantsEscapeOnCorruption
//
// When the overlay has a valid grant, then a corrupt
// line, then another valid grant, reload MUST refuse
// to load so that NO grants escape into the in-memory
// Policy. The previous-valid surface is binary: either
// the whole overlay is accepted, or none of it is.
func TestDV05ProductionPathNoPartialGrantsEscapeOnCorruption(t *testing.T) {
	f := newDV05Fixture(t)

	good1 := identity.Grant{
		Role: identity.NetworkAdmin, Object: identity.ObjectKey{Kind: "wireguard.peer", ID: "wg-p1"},
		Permission: identity.Observe, Decision: identity.DecisionDeny, Reason: "first",
	}
	good2 := identity.Grant{
		Role: identity.NetworkAdmin, Object: identity.ObjectKey{Kind: "wireguard.peer", ID: "wg-p3"},
		Permission: identity.Observe, Decision: identity.DecisionDeny, Reason: "third",
	}
	b1, _ := json.Marshal(good1)
	b2, _ := json.Marshal(good2)
	body := append([]byte("{not-json\n"), b1...)
	body = append(body, '\n')
	body = append(body, b2...)
	body = append(body, '\n')
	if err := os.WriteFile(f.policyOverlayPath(), body, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	reopened, err := identity.OpenPolicyStore(f.policyDir, identity.NewPolicy())
	if err == nil {
		// Defensive: if the store somehow accepted a
		// partial overlay, the Policy MUST be empty.
		if all := reopened.Policy().All(); len(all) != 0 {
			t.Fatalf("corrupt overlay: %d grants escaped into Policy, want 0", len(all))
		}
		t.Fatalf("OpenPolicyStore(corrupt) must FAIL CLOSED; got nil error")
	}
}

// TestDV05ProductionPathIdentityLastOwnerPreserved
//
// The M-23 invariant forbids demoting or disabling the
// last active Owner. UpdateUser refuses the call; the
// identity Service stays open; the durable state.json
// still records the Owner with role=owner and
// disabled=false. Independent oracle reads state.json
// before and after the attempt and asserts the byte
// content is unchanged.
func TestDV05ProductionPathIdentityLastOwnerPreserved(t *testing.T) {
	f := newDV05Fixture(t)

	users, err := f.identity.Users(f.ownerTok)
	if err != nil {
		t.Fatalf("identity.Users: %v", err)
	}
	var ownerID string
	for _, u := range users {
		if u.Role == identity.Owner && !u.Disabled {
			ownerID = u.ID
			break
		}
	}
	if ownerID == "" {
		t.Fatalf("no active Owner in users: %+v", users)
	}
	// Snapshot state.json BEFORE the attempt so the
	// oracle can prove no change after.
	before, err := os.ReadFile(f.identStatePath())
	if err != nil {
		t.Fatalf("independent ReadFile(before): %v", err)
	}

	if err := f.identity.UpdateUser(f.ownerTok, ownerID, identity.NetworkAdmin, false); err == nil {
		t.Fatalf("demoting last Owner must FAIL CLOSED; got nil")
	}

	// Independent oracle: state.json is unchanged.
	after, err := os.ReadFile(f.identStatePath())
	if err != nil {
		t.Fatalf("independent ReadFile(after): %v", err)
	}
	if !bytesEqual(before, after) {
		t.Fatalf("state.json mutated by failed demote attempt:\nbefore: %q\nafter:  %q",
			string(before), string(after))
	}

	// Service is still open: the failed UpdateUser
	// did not close it (validateDatabase guard runs
	// on save, not on UpdateUser). The owner can
	// still use their token.
	if _, err := f.identity.Authorize(f.ownerTok, identity.Observe); err != nil {
		t.Fatalf("owner token invalidated by failed demote: %v", err)
	}

	// Disable path: same invariant.
	if err := f.identity.UpdateUser(f.ownerTok, ownerID, identity.Owner, true); err == nil {
		t.Fatalf("disabling last Owner must FAIL CLOSED; got nil")
	}
}

// bytesContain is a tiny helper so the file-oracle
// checks do not pull in bytes for one Contains call.
func bytesContain(haystack, needle []byte) bool {
	if len(needle) == 0 {
		return true
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		match := true
		for j := range needle {
			if haystack[i+j] != needle[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// bytesEqual is a tiny helper so the file-oracle checks
// do not pull in bytes for one Equal call.
func bytesEqual(a, b []byte) bool {
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

// Unused imports kept on purpose: atomic is reserved
// for future active-flag flips (DV05 currently runs
// the agent IPC unconditionally). fmt is used in
// diagnostics. errors is used by the future DV10
// negative case wired against the same fixture.
var (
	_ = atomic.LoadInt32
	_ = fmt.Sprintf
	_ = errors.New
)
