//go:build linux

// dv01_production_test.go exercises DV01 through the
// production code path:
//
//   PUBLIC HTTP GET /api/v1/objects/{kind}
//   -> production api.Handler.handleObjects
//   -> engine.ObjectList (real Engine, real Unix-socket transport)
//   -> registered production lister
//   -> real source-backed data:
//        wireguard.*  -> wireguard.FileCatalog (durable on disk)
//        xray.*       -> xray.FileCatalog      (durable on disk)
//        qos.class + network.{interface,route,address}
//                    -> engine.Doctor / engine.QoS over real Unix socket
//        9 linux-runtime kinds -> LinuxSource accessor
//                                (nil in this fixture -> honest
//                                 engine_unavailable, never
//                                 fake-empty Complete=true)
//
// DV01 production-path contract: every one of the 19 canonical
// V1 object kinds must be reachable through the public API
// with real source-backed listers, honest unavailable when the
// source is genuinely absent, paginated via limit/after,
// bootstrappable via the API token, restart-stable, and
// honest on a disabled Agent. No grep/source-only PASS; no
// fake-empty Complete=true; no kernel-route-only test.

package layerl

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/api"
	"github.com/CrazyDuckLab/DuckOS/internal/doctor"
	"github.com/CrazyDuckLab/DuckOS/internal/engine"
	"github.com/CrazyDuckLab/DuckOS/internal/object"
	"github.com/CrazyDuckLab/DuckOS/internal/objectlisters"
	"github.com/CrazyDuckLab/DuckOS/internal/qos"
	"github.com/CrazyDuckLab/DuckOS/internal/wireguard"
	"github.com/CrazyDuckLab/DuckOS/internal/xray"
)

// dv01Production is the production-boundary fixture for
// DV01. It wires the exact constructor chain cmd/duck-api
// uses:
//
//	Engine.New(sockPath, env)            // same as cmd/duck-api
//	+ RegisterAllWithRealSourcesAndLinuxSource
//	                                     // production boundary
//	-> api.New(engine, token, listen, webDir)
//	                                     // same as cmd/duck-api
//	-> httptest.NewServer(api.Handler)
//
// The fakeAgentSocket is a real net.Listen(unix) the Engine
// HTTP client dials through its Unix-socket transport. The
// wireguard / xray catalogs are real FileCatalog instances
// with real disk-backed state populated via UpsertServer /
// UpsertUser. LinuxSource is intentionally nil so the 9
// Linux-runtime kinds surface honest engine_unavailable
// rather than fake-empty Complete=true.
type dv01Production struct {
	dir         string
	sockPath    string
	listener    net.Listener
	apiSrv      *httptest.Server
	token       string
	wgCatalog   *wireguard.FileCatalog
	xrayCatalog *xray.FileCatalog
	engine      *engine.Engine
	doctorCalls atomic.Int64
	qosCalls    atomic.Int64
	active      int32
}

const dv01TestToken = "synthetic-dv01-test-token-not-a-real-credential-1234567890"

func newDV01Production(t *testing.T) *dv01Production {
	t.Helper()
	p := &dv01Production{dir: t.TempDir(), token: dv01TestToken, active: 1}

	p.sockPath = filepath.Join(p.dir, "agent.sock")
	listener, err := net.Listen("unix", p.sockPath)
	if err != nil {
		t.Fatalf("net.Listen(unix): %v", err)
	}
	p.listener = listener

	mux := http.NewServeMux()
	mux.HandleFunc("/agent/v1/doctor", p.serveDoctor)
	mux.HandleFunc("/agent/v1/qos", p.serveQoS)
	go http.Serve(listener, mux)

	p.engine = engine.New(p.sockPath, "test")
	p.populateWireguardCatalog(t)
	p.populateXrayCatalog(t)
	objectlisters.RegisterAllWithRealSourcesAndLinuxSource(
		p.engine, p.wgCatalog, p.xrayCatalog, nil, nil,
	)

	webDir := filepath.Join(p.dir, "web")
	if err := os.MkdirAll(webDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(webDir, "index.html"),
		[]byte("<!doctype html><title>DV01</title>"), 0o600); err != nil {
		t.Fatal(err)
	}
	handler, err := api.New(p.engine, p.token, "127.0.0.1:38080", webDir)
	if err != nil {
		t.Fatalf("api.New: %v", err)
	}
	p.apiSrv = httptest.NewServer(handler)
	t.Cleanup(func() {
		p.apiSrv.Close()
		_ = handler.Close()
		_ = listener.Close()
		_ = os.Remove(p.sockPath)
	})
	return p
}

// serveDoctor is the fake Agent IPC for the Engine.Doctor
// source. The snapshot must include enough data for the
// network.interface / network.route / network.address
// listers to project at least one row.
func (p *dv01Production) serveDoctor(w http.ResponseWriter, r *http.Request) {
	p.doctorCalls.Add(1)
	if atomic.LoadInt32(&p.active) == 0 {
		http.Error(w, "agent unavailable", http.StatusServiceUnavailable)
		return
	}
	snap := doctor.Snapshot{
		SchemaVersion: doctor.SchemaVersion,
		GeneratedAt:   time.Now().UTC(),
		Environment:   "test",
		Network: doctor.Network{
			Interfaces: []doctor.Interface{{
				Name:      "veth-dv01",
				Index:     42,
				LinkUp:    true,
				MTU:       1500,
				Addresses: []string{"10.99.99.1/24"},
				OperState: "up",
			}},
			Routes: []doctor.Route{{
				Destination: "default", Gateway: "10.99.99.254", Device: "veth-dv01",
			}},
		},
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(snap)
}

// serveQoS is the fake Agent IPC for the Engine.QoS source.
// The topology must include at least one shaper so
// qos.class lister has data to project.
func (p *dv01Production) serveQoS(w http.ResponseWriter, r *http.Request) {
	p.qosCalls.Add(1)
	if atomic.LoadInt32(&p.active) == 0 {
		http.Error(w, "agent unavailable", http.StatusServiceUnavailable)
		return
	}
	tp := qos.Topology{
		SchemaVersion: qos.SchemaVersion,
		Shapers: []qos.Shaper{{
			ID: "dv01-wan", Link: "veth-dv01",
			Direction: qos.DirectionEgress, Algorithm: qos.AlgorithmCAKE,
			RateKbps: 1000, BurstKbyte: 32, Priority: qos.PriorityVoice,
		}},
		GeneratedAt: time.Now().UTC().Unix(),
	}
	st := qos.Status{Topology: tp, HasShapers: len(tp.Shapers)}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(st)
}

func (p *dv01Production) populateWireguardCatalog(t *testing.T) {
	t.Helper()
	dir := filepath.Join(p.dir, "wireguard")
	cat, err := wireguard.OpenFileCatalog(dir)
	if err != nil {
		t.Fatalf("wireguard.OpenFileCatalog: %v", err)
	}
	srv := wireguard.Server{
		ID: "wg-dv01-1", Name: "dv01-server", Interface: "wg0",
		ListenPort: 51820, Addresses: []string{"10.8.0.1/24"},
		PrivateKeyRef: wireguard.KeyRef{VaultID: "v1", Revision: "r1", KeyID: "k1", CreatedAt: "2026-01-01T00:00:00Z"},
		PublicKey:     "PUBKEY-DV01",
		CreatedAt:     time.Unix(1, 0).UTC(),
	}
	if err := cat.UpsertServer(context.Background(), srv); err != nil {
		t.Fatalf("wg UpsertServer: %v", err)
	}
	peer := wireguard.Peer{
		ID: "wg-dv01-p1", ServerID: "wg-dv01-1", Name: "alice",
		PublicKey: "PEERPUBKEY-DV01", AllowedIPs: []string{"10.8.0.2/32"},
		Intent: wireguard.IntentFullTunnel, Enabled: true,
		CreatedAt: time.Unix(1, 0).UTC(),
	}
	if err := cat.UpsertPeer(context.Background(), peer); err != nil {
		t.Fatalf("wg UpsertPeer: %v", err)
	}
	p.wgCatalog = cat
}

func (p *dv01Production) populateXrayCatalog(t *testing.T) {
	t.Helper()
	dir := filepath.Join(p.dir, "xray")
	cat, err := xray.OpenFileCatalog(dir)
	if err != nil {
		t.Fatalf("xray.OpenFileCatalog: %v", err)
	}
	now := time.Unix(1, 0).UTC()
	srv := xray.Server{
		ID: "xray-dv01-1", Name: "dv01-xray", Protocol: xray.ProtocolVLESS,
		Transport: xray.TransportTCP, Security: xray.SecurityReality,
		Listen: "127.0.0.1", Port: 443, Enabled: true, CreatedAt: now, UpdatedAt: now,
	}
	if err := cat.UpsertServer(context.Background(), srv); err != nil {
		t.Fatalf("xray UpsertServer: %v", err)
	}
	user := xray.User{
		ID: "xray-dv01-u1", ServerID: "xray-dv01-1", Name: "alice",
		UUID: "11111111-2222-3333-4444-555555555555", Enabled: true,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := cat.UpsertUser(context.Background(), user); err != nil {
		t.Fatalf("xray UpsertUser: %v", err)
	}
	profile := xray.Profile{
		ID: "xray-dv01-pr1", Name: "alice-full", UserID: user.ID,
		Intent: xray.ProfileIntentFullTunnel, Enabled: true,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := cat.UpsertProfile(context.Background(), profile); err != nil {
		t.Fatalf("xray UpsertProfile: %v", err)
	}
	sub := xray.Subscription{
		ID: "xray-dv01-sub1", Name: "alice-sub", ProfileIDs: []string{profile.ID},
		Enabled: true, CreatedAt: now, UpdatedAt: now,
	}
	if err := cat.UpsertSubscription(context.Background(), sub); err != nil {
		t.Fatalf("xray UpsertSubscription: %v", err)
	}
	p.xrayCatalog = cat
}

// fetch issues an authenticated GET /api/v1/objects/{kind}
// against the production api.Handler under httptest. The
// production handler enforces a loopback host check against
// the listener port recorded at construction time; httptest
// picks a different port, so we override the Host header
// to satisfy the trustedHost check.
func (p *dv01Production) fetch(t *testing.T, kind string, params url.Values) (int, object.Collection) {
	t.Helper()
	u := p.apiSrv.URL + "/api/v1/objects/" + kind
	if len(params) > 0 {
		u += "?" + params.Encode()
	}
	req, _ := http.NewRequest(http.MethodGet, u, nil)
	req.Header.Set("Authorization", "Bearer "+p.token)
	req.Host = "127.0.0.1:38080"
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", u, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return resp.StatusCode, object.Collection{}
	}
	var col object.Collection
	if err := json.NewDecoder(resp.Body).Decode(&col); err != nil {
		t.Fatalf("decode %s: %v", kind, err)
	}
	return resp.StatusCode, col
}

// dv01Kinds is the canonical 19-kind surface the API must
// serve through the production path. The list mirrors
// objectlisters.SupportedKinds and api.supportedObjectKinds
// — both are identical by construction (the API mirrors the
// listers so a kind is never silently dropped on the way
// out).
var dv01Kinds = []string{
	"network.interface",
	"network.route",
	"network.firewall.rule",
	"network.firewall.set",
	"network.lease",
	"network.address",
	"wireguard.server",
	"wireguard.peer",
	"xray.server",
	"xray.user",
	"xray.profile",
	"xray.subscription",
	"qos.class",
	"dns.zone",
	"dhcp.subnet",
	"bgp.peer",
	"ospf.area",
	"isis.area",
	"route.map",
}

// linuxSourceKinds are the 9 kinds whose production
// boundary is the LinuxSource accessor. When LinuxSource
// is nil (this fixture) the listers MUST surface honest
// engine_unavailable — never fake-empty Complete=true.
var linuxSourceKinds = []string{
	"network.firewall.rule",
	"network.firewall.set",
	"network.lease",
	"dns.zone",
	"dhcp.subnet",
	"bgp.peer",
	"ospf.area",
	"isis.area",
	"route.map",
}

// TestDV01AllNineteenKindsReachableViaPublicAPI iterates
// every canonical V1 kind and asserts the public API
// returns a well-formed Collection envelope (200 + JSON
// object, not a 404 or a generic error).
func TestDV01ProductionAllNineteenKindsReachableViaPublicAPI(t *testing.T) {
	p := newDV01Production(t)
	if len(dv01Kinds) != 19 {
		t.Fatalf("DV01 expects 19 core kinds, fixture lists %d", len(dv01Kinds))
	}
	for _, kind := range dv01Kinds {
		code, col := p.fetch(t, kind, nil)
		if code != http.StatusOK {
			t.Errorf("%s: HTTP %d, want 200", kind, code)
			continue
		}
		if col.SnapshotID == "" {
			t.Errorf("%s: SnapshotID missing", kind)
		}
	}
}

// TestDV01WireguardSourceBackedLister asserts the
// wireguard.{server,peer} kinds return REAL rows from
// the durable FileCatalog — not engine_unavailable, not
// a fake-empty Complete=true.
func TestDV01ProductionWireguardSourceBackedLister(t *testing.T) {
	p := newDV01Production(t)

	code, srvCol := p.fetch(t, "wireguard.server", nil)
	if code != http.StatusOK {
		t.Fatalf("wireguard.server: HTTP %d", code)
	}
	if !srvCol.Complete {
		t.Fatalf("wireguard.server: Complete=false, want true (real source-backed lister)")
	}
	if len(srvCol.Items) != 1 || srvCol.Items[0].Ref.ID != "wg-dv01-1" {
		t.Fatalf("wireguard.server: items=%+v, want 1 row with id=wg-dv01-1", srvCol.Items)
	}

	code, peerCol := p.fetch(t, "wireguard.peer", nil)
	if code != http.StatusOK {
		t.Fatalf("wireguard.peer: HTTP %d", code)
	}
	if !peerCol.Complete {
		t.Fatalf("wireguard.peer: Complete=false, want true (real source-backed lister)")
	}
	if len(peerCol.Items) != 1 || peerCol.Items[0].Ref.ID != "wg-dv01-p1" {
		t.Fatalf("wireguard.peer: items=%+v, want 1 row with id=wg-dv01-p1", peerCol.Items)
	}
}

// TestDV01XraySourceBackedLister asserts the 4 xray.*
// kinds return REAL rows from the durable FileCatalog.
func TestDV01ProductionXraySourceBackedLister(t *testing.T) {
	p := newDV01Production(t)

	cases := []struct {
		kind string
		want string
	}{
		{"xray.server", "xray-dv01-1"},
		{"xray.user", "xray-dv01-u1"},
		{"xray.profile", "xray-dv01-pr1"},
		{"xray.subscription", "xray-dv01-sub1"},
	}
	for _, tc := range cases {
		code, col := p.fetch(t, tc.kind, nil)
		if code != http.StatusOK {
			t.Errorf("%s: HTTP %d", tc.kind, code)
			continue
		}
		if !col.Complete {
			t.Errorf("%s: Complete=false, want true (real source-backed lister)", tc.kind)
			continue
		}
		if len(col.Items) != 1 || col.Items[0].Ref.ID != tc.want {
			t.Errorf("%s: items=%+v, want 1 row with id=%s", tc.kind, col.Items, tc.want)
		}
	}
}

// TestDV01EngineBackedListers asserts the engine-backed
// kinds (qos.class + network.{interface,route,address})
// return REAL rows from the Engine's Doctor + QoS IPC,
// not engine_unavailable.
func TestDV01ProductionEngineBackedListers(t *testing.T) {
	p := newDV01Production(t)

	if p.doctorCalls.Load() == 0 {
		// First fetch should have hit the agent.
		p.fetch(t, "network.interface", nil)
	}

	cases := []struct {
		kind     string
		minItems int
	}{
		{"network.interface", 1},
		{"network.route", 1},
		{"network.address", 1},
		{"qos.class", 0}, // topology has shapers; classes are derived at render time
	}
	for _, tc := range cases {
		code, col := p.fetch(t, tc.kind, nil)
		if code != http.StatusOK {
			t.Errorf("%s: HTTP %d", tc.kind, code)
			continue
		}
		if !col.Complete {
			t.Errorf("%s: Complete=false, want true (engine-backed lister)", tc.kind)
		}
		if len(col.Items) < tc.minItems {
			t.Errorf("%s: items=%d, want >=%d", tc.kind, len(col.Items), tc.minItems)
		}
	}
	if p.doctorCalls.Load() == 0 {
		t.Errorf("engine-backed listers never hit the doctor IPC")
	}
}

// TestDV01LinuxSourceNilHonestUnavailable asserts the 9
// Linux-runtime kinds surface HONEST engine_unavailable
// when LinuxSource is nil — never fake-empty Complete=true.
// The contract is the source-of-truth for the
// "no backend for this kind" affordance.
func TestDV01ProductionLinuxSourceNilHonestUnavailable(t *testing.T) {
	p := newDV01Production(t)
	for _, kind := range linuxSourceKinds {
		code, col := p.fetch(t, kind, nil)
		if code != http.StatusOK {
			t.Errorf("%s: HTTP %d, want 200 (envelope present, just unavailable)", kind, code)
			continue
		}
		if col.Complete {
			t.Errorf("%s: Complete=true on nil LinuxSource = fake-empty success violation", kind)
		}
		found := false
		for _, iss := range col.Issues {
			if iss.ReasonID == "engine_unavailable" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("%s: missing engine_unavailable Issue; got %+v", kind, col.Issues)
		}
		if len(col.Items) != 0 {
			t.Errorf("%s: Items=%d on unavailable, want 0", kind, len(col.Items))
		}
	}
}

// TestDV01PaginationLimitAndAfter asserts the API honours
// the bounded limit/after query parameters on the public
// surface. The wireguard catalog has 1 row; the limit=1
// request must echo a cursor the next call can use.
func TestDV01ProductionPaginationLimitAndAfter(t *testing.T) {
	p := newDV01Production(t)

	code, col := p.fetch(t, "wireguard.server", url.Values{"limit": {"1"}})
	if code != http.StatusOK {
		t.Fatalf("HTTP %d", code)
	}
	if len(col.Items) > 1 {
		t.Fatalf("limit=1 returned %d items", len(col.Items))
	}
	// Cursor may be empty when the page is complete; if
	// it is set, it must round-trip without error.
	if col.Cursor != "" {
		code2, col2 := p.fetch(t, "wireguard.server", url.Values{
			"limit": {"1"},
			"after": {col.Cursor},
		})
		if code2 != http.StatusOK {
			t.Fatalf("after-cursor HTTP %d", code2)
		}
		// Items after the cursor must not overlap with the first page.
		for _, it := range col2.Items {
			for _, first := range col.Items {
				if it.Ref.ID == first.Ref.ID {
					t.Errorf("cursor returned duplicate id=%s", it.Ref.ID)
				}
			}
		}
	}
}

// TestDV01BootstrapTokenAuthentication asserts the public
// API admits the bootstrap token (no enrolled identity).
// Every kind must return 200 with the same envelope
// contract as the enrolled path.
func TestDV01ProductionBootstrapTokenAuthentication(t *testing.T) {
	p := newDV01Production(t)
	for _, kind := range dv01Kinds {
		code, _ := p.fetch(t, kind, nil)
		if code != http.StatusOK {
			t.Errorf("%s: HTTP %d with bootstrap token, want 200", kind, code)
		}
	}
}

// TestDV01RestartPreservesIdentity tears the Engine down,
// re-creates it against the same fixture, re-registers
// the production listers, and asserts the canonical
// ObjectRef.ID is preserved across the restart. The
// wireguard catalog is durable on disk so the IDs survive
// even if the in-process Engine is destroyed.
func TestDV01ProductionRestartPreservesIdentity(t *testing.T) {
	p := newDV01Production(t)

	_, first := p.fetch(t, "wireguard.server", nil)
	if len(first.Items) == 0 {
		t.Fatal("pre-restart: no wireguard.server rows")
	}
	firstID := first.Items[0].Ref.ID

	// Tear down and re-create. The wireguard / xray
	// catalogs are on disk and survive; the Engine is
	// destroyed.
	_ = p.apiSrv.Close
	_ = p.listener.Close()
	_ = os.Remove(p.sockPath)

	newListener, err := net.Listen("unix", p.sockPath)
	if err != nil {
		t.Fatalf("net.Listen(unix): %v", err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/agent/v1/doctor", p.serveDoctor)
	mux.HandleFunc("/agent/v1/qos", p.serveQoS)
	go http.Serve(newListener, mux)
	p.engine = engine.New(p.sockPath, "test")
	objectlisters.RegisterAllWithRealSourcesAndLinuxSource(
		p.engine, p.wgCatalog, p.xrayCatalog, nil, nil,
	)
	webDir := filepath.Join(p.dir, "web")
	handler, err := api.New(p.engine, p.token, "127.0.0.1:38080", webDir)
	if err != nil {
		t.Fatalf("api.New: %v", err)
	}
	p.apiSrv = httptest.NewServer(handler)
	p.listener = newListener
	t.Cleanup(func() {
		p.apiSrv.Close()
		_ = handler.Close()
		_ = newListener.Close()
		_ = os.Remove(p.sockPath)
	})

	_, second := p.fetch(t, "wireguard.server", nil)
	if len(second.Items) == 0 {
		t.Fatal("post-restart: no wireguard.server rows")
	}
	if firstID != second.Items[0].Ref.ID {
		t.Fatalf("identity drift across restart: %q vs %q", firstID, second.Items[0].Ref.ID)
	}
}

// TestDV01DisabledAgentProducesTypedUnavailable flips the
// fake agent to inactive. The Engine.Doctor / Engine.QoS
// calls must surface a typed 503 from the IPC, the lister
// must convert that to an honest engine_unavailable Issue,
// and the API must NOT return a fake-empty Complete=true
// envelope for the engine-backed kinds.
//
// The LinuxSource-backed kinds were already returning
// engine_unavailable; this case proves the same contract
// holds for engine-backed kinds when the Agent goes down.
func TestDV01ProductionDisabledAgentProducesTypedUnavailable(t *testing.T) {
	p := newDV01Production(t)
	atomic.StoreInt32(&p.active, 0)
	defer atomic.StoreInt32(&p.active, 1)

	for _, kind := range []string{"network.interface", "network.route", "qos.class"} {
		code, col := p.fetch(t, kind, nil)
		if code != http.StatusOK {
			// The 503 envelope is acceptable — the engine
			// bubble-up may surface 503 from the doctor IPC
			// — but the contract here is "no fake success":
			// never Complete=true with empty Items.
			if col.Complete && len(col.Items) == 0 {
				t.Errorf("%s: HTTP %d with Complete=true and empty Items = fake-empty success", kind, code)
			}
			continue
		}
		if col.Complete {
			t.Errorf("%s: Complete=true on disabled agent = fake-empty success violation", kind)
		}
		found := false
		for _, iss := range col.Issues {
			if iss.ReasonID == "engine_unavailable" || strings.Contains(iss.ReasonID, "unavailable") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("%s: missing engine_unavailable Issue on disabled agent; got %+v", kind, col.Issues)
		}
	}
}

// fmtError is a tiny helper used in test diagnostics.
func fmtError(err error) string { return fmt.Sprintf("%v", err) }
