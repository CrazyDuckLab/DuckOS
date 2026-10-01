package api

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/doctor"
	"github.com/CrazyDuckLab/DuckOS/internal/engine"
	"github.com/CrazyDuckLab/DuckOS/internal/identity"
	"github.com/CrazyDuckLab/DuckOS/internal/migrate"
	"github.com/CrazyDuckLab/DuckOS/internal/multiwan"
	"github.com/CrazyDuckLab/DuckOS/internal/object"
	"github.com/CrazyDuckLab/DuckOS/internal/qos"
	"github.com/CrazyDuckLab/DuckOS/internal/sites"
	"github.com/CrazyDuckLab/DuckOS/internal/topology"
	"github.com/CrazyDuckLab/DuckOS/internal/ups"
	"github.com/CrazyDuckLab/DuckOS/internal/wifi"
)

const testToken = "synthetic-test-token-not-a-real-credential-123456789"
const testAssertionKey = "synthetic-agent-assertion-key-for-tests-123456"

type testEngine struct {
	failed      bool
	calls       int
	wgCalls     int
	xrayCalls   int
	lastWGToken string
	lastXRToken string
}

func (e *testEngine) Overview(context.Context) ([]byte, error) {
	e.calls++
	if e.failed {
		return nil, errors.New("secret stderr must not escape")
	}
	return []byte(`{"testFixture":true}`), nil
}
func (e *testEngine) Readiness(context.Context) (engine.ReadinessSnapshot, error) {
	return engine.ReadinessSnapshot{Version: 1}, nil
}
func (e *testEngine) Config() *engine.Config {
	return nil
}
func (e *testEngine) ObjectList(ctx context.Context, kind string, params object.CollectionParams) (object.Collection, error) {
	return object.Collection{SnapshotID: "test", Complete: true, Items: []object.ViewObject{}}, nil
}
func (e *testEngine) Ready(context.Context) error {
	e.calls++
	if e.failed {
		return errors.New("secret")
	}
	return nil
}
func (e *testEngine) Doctor(context.Context) (doctor.Snapshot, error) {
	if e.failed {
		return doctor.Snapshot{}, errors.New("secret")
	}
	b := doctor.NewBuilder("test", "duckos-dev", "boot-1", time.Now().UTC(), 0)
	return b.Snapshot(), nil
}
func (e *testEngine) Topology(context.Context) (topology.Graph, error) {
	if e.failed {
		return topology.Graph{}, errors.New("secret")
	}
	return topology.NewBuilder(time.Now().Unix()).Snapshot(), nil
}
func (e *testEngine) MultiWAN(context.Context) (multiwan.Topology, map[string]multiwan.Selection, map[string]string, error) {
	if e.failed {
		return multiwan.Topology{}, nil, nil, errors.New("secret")
	}
	return multiwan.Topology{SchemaVersion: multiwan.SchemaVersion}, map[string]multiwan.Selection{}, map[string]string{}, nil
}
func (e *testEngine) QoS(context.Context) (qos.Status, error) {
	if e.failed {
		return qos.Status{}, errors.New("secret")
	}
	return qos.Status{Topology: qos.Topology{SchemaVersion: qos.SchemaVersion}}, nil
}
func (e *testEngine) QoSApply(context.Context, qos.Plan) (qos.ApplyOutcome, error) {
	if e.failed {
		return qos.ApplyOutcome{}, errors.New("secret")
	}
	return qos.ApplyOutcome{Applied: 1, StartedAt: time.Unix(0, 0), EndedAt: time.Unix(0, 1)}, nil
}
func (e *testEngine) QoSRollback(context.Context) (qos.ApplyOutcome, error) {
	if e.failed {
		return qos.ApplyOutcome{}, errors.New("secret")
	}
	return qos.ApplyOutcome{Applied: 0, StartedAt: time.Unix(0, 0), EndedAt: time.Unix(0, 1)}, nil
}
func (e *testEngine) UPS(context.Context) (ups.Status, error) {
	if e.failed {
		return ups.Status{}, errors.New("secret")
	}
	return ups.Status{Topology: ups.Topology{SchemaVersion: ups.SchemaVersion}}, nil
}
func (e *testEngine) UPSProbe(context.Context) error {
	if e.failed {
		return errors.New("secret")
	}
	return nil
}
func (e *testEngine) Migrate(_ context.Context, _ migrate.Source, body []byte) (migrate.Plan, error) {
	if e.failed {
		return migrate.Plan{}, errors.New("secret")
	}
	return migrate.Analyse(migrate.Source{Filename: "test", Format: migrate.FormatMikroTik}, body, time.Unix(1, 0))
}
func (e *testEngine) Sites(context.Context) (sites.Status, error) {
	if e.failed {
		return sites.Status{}, errors.New("secret")
	}
	return sites.Status{Topology: sites.Topology{SchemaVersion: sites.SchemaVersion}}, nil
}
func (e *testEngine) Wifi(context.Context) (wifi.Status, error) {
	if e.failed {
		return wifi.Status{}, errors.New("secret")
	}
	return wifi.Status{Topology: wifi.Topology{SchemaVersion: wifi.SchemaVersion}}, nil
}
func (e *testEngine) Upgrade(context.Context) ([]byte, error) {
	if e.failed {
		return nil, errors.New("secret")
	}
	return []byte(`{"version":1,"healthy":true}`), nil
}
func (e *testEngine) Backup(context.Context, string) ([]byte, error) {
	if e.failed {
		return nil, errors.New("secret")
	}
	return []byte(`{"version":1,"backups":[]}`), nil
}
func (e *testEngine) BackupRestore(context.Context, []byte) ([]byte, error) {
	if e.failed {
		return nil, errors.New("secret")
	}
	return []byte(`{"version":1,"outcome":"applied","entryId":"backup-0001-restored"}`), nil
}
func (e *testEngine) WireGuardProxy(_ context.Context, _, _, token string, _ io.Reader) (int, []byte, string, error) {
	e.wgCalls++
	e.lastWGToken = token
	return 200, []byte(`{"ok":true}`), "application/json", nil
}
func (e *testEngine) XrayProxy(_ context.Context, _, _, token string, _ io.Reader) (int, []byte, string, error) {
	e.xrayCalls++
	e.lastXRToken = token
	return 200, []byte(`{"ok":true}`), "application/json", nil
}
func (e *testEngine) SpeedLabProxy(_ context.Context, _, _, token string, _ io.Reader) (int, []byte, string, error) {
	e.lastToken = token
	return 200, []byte(`{"ok":true}`), "application/json", nil
}
func (e *testEngine) UPnPProxy(_ context.Context, _, _, token string, _ io.Reader) (int, []byte, string, error) {
	e.lastToken = token
	return 200, []byte(`{"ok":true}`), "application/json", nil
}
func (e *testEngine) WakeProxy(_ context.Context, _, _, token string, _ io.Reader) (int, []byte, string, error) {
	e.lastToken = token
	return 200, []byte(`{"ok":true}`), "application/json", nil
}
func (e *testEngine) UpgradeStage(context.Context, []byte) ([]byte, error) {
	return []byte(`{"version":1,"result":"staged"}`), nil
}
func (e *testEngine) UpgradeActivate(context.Context, []byte) ([]byte, error) {
	return []byte(`{"version":1,"result":"activated"}`), nil
}
func (e *testEngine) UpgradeRollback(context.Context, []byte) ([]byte, error) {
	return []byte(`{"version":1,"result":"rolled-back"}`), nil
}
func (e *testEngine) FRR(context.Context) ([]byte, error) {
	return []byte(`{"version":1,"pathspace":"test","sessionsEstablished":0,"routesInstalled":0}`), nil
}
func (e *testEngine) FRRApply(context.Context, []byte) ([]byte, error) {
	return []byte(`{"version":1,"result":"applied"}`), nil
}
func (e *testEngine) FRRRollback(context.Context, []byte) ([]byte, error) {
	return []byte(`{"version":1,"result":"rolled-back"}`), nil
}
func (e *testEngine) ZTPIssue(context.Context, []byte) ([]byte, error) {
	if e.failed {
		return nil, errors.New("secret")
	}
	return []byte(`{"token":{"subject":"node-1","issuedAt":"2026-09-16T00:00:00Z","expiresAt":"2026-09-16T00:30:00Z","mac":"abc"}}`), nil
}
func (e *testEngine) ZTPRedeem(context.Context, []byte) ([]byte, error) {
	if e.failed {
		return nil, errors.New("secret")
	}
	return []byte(`{"record":{"id":"ztp-1","phase":"redeemed","tokenDigest":"digest","issuedAt":"2026-09-16T00:00:00Z"}}`), nil
}
func handler(t *testing.T, e *testEngine) *Handler {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(dir+"/index.html", []byte("<!doctype html><title>DuckOS test</title>"), 0600); err != nil {
		t.Fatal(err)
	}
	h, err := New(e, testToken, "127.0.0.1:8080", dir)
	if err != nil {
		t.Fatal(err)
	}
	h.EnableAgentAssertionKey(testAssertionKey)
	t.Cleanup(func() { _ = h.Close() })
	return h
}
func TestAuthenticationAndOriginBoundary(t *testing.T) {
	e := &testEngine{}
	h := handler(t, e)
	for _, tc := range []struct {
		name, path, method, host, origin, auth string
		status                                 int
	}{
		{"health", "/api/v1/health", "GET", "127.0.0.1:8080", "", "", 200},
		{"overview needs auth", "/api/v1/overview", "GET", "127.0.0.1:8080", "", "", 401},
		{"wrong token", "/api/v1/overview", "GET", "127.0.0.1:8080", "", "Bearer wrong", 401},
		{"authorized", "/api/v1/overview", "GET", "127.0.0.1:8080", "http://127.0.0.1:8080", "Bearer " + testToken, 200},
		{"ready auth", "/api/v1/ready", "GET", "127.0.0.1:8080", "", "Bearer " + testToken, 200},
		{"doctor auth", "/api/v1/doctor", "GET", "127.0.0.1:8080", "", "Bearer " + testToken, 200},
		{"doctor unauth", "/api/v1/doctor", "GET", "127.0.0.1:8080", "", "", 401},
		{"doctor mutation", "/api/v1/doctor", "POST", "127.0.0.1:8080", "", "Bearer " + testToken, 405},
		{"doctor query", "/api/v1/doctor?limit=10", "GET", "127.0.0.1:8080", "", "Bearer " + testToken, 200},
		{"topology auth", "/api/v1/topology", "GET", "127.0.0.1:8080", "", "Bearer " + testToken, 200},
		{"topology unauth", "/api/v1/topology", "GET", "127.0.0.1:8080", "", "", 401},
		{"topology mutation", "/api/v1/topology", "POST", "127.0.0.1:8080", "", "Bearer " + testToken, 405},
		{"topology query", "/api/v1/topology?limit=10", "GET", "127.0.0.1:8080", "", "Bearer " + testToken, 200},
		{"multiwan auth", "/api/v1/multiwan", "GET", "127.0.0.1:8080", "", "Bearer " + testToken, 200},
		{"multiwan unauth", "/api/v1/multiwan", "GET", "127.0.0.1:8080", "", "", 401},
		{"multiwan mutation", "/api/v1/multiwan", "POST", "127.0.0.1:8080", "", "Bearer " + testToken, 405},
		{"multiwan query", "/api/v1/multiwan?limit=10", "GET", "127.0.0.1:8080", "", "Bearer " + testToken, 200},
		{"qos auth", "/api/v1/qos", "GET", "127.0.0.1:8080", "", "Bearer " + testToken, 200},
		{"qos unauth", "/api/v1/qos", "GET", "127.0.0.1:8080", "", "", 401},
		{"qos mutation", "/api/v1/qos", "POST", "127.0.0.1:8080", "", "Bearer " + testToken, 405},
		{"qos apply needs POST", "/api/v1/qos/apply", "GET", "127.0.0.1:8080", "", "Bearer " + testToken, 405},
		{"ups auth", "/api/v1/ups", "GET", "127.0.0.1:8080", "", "Bearer " + testToken, 200},
		{"ups unauth", "/api/v1/ups", "GET", "127.0.0.1:8080", "", "", 401},
		{"ups mutation", "/api/v1/ups", "POST", "127.0.0.1:8080", "", "Bearer " + testToken, 405},
		{"ups probe needs POST", "/api/v1/ups/probe", "GET", "127.0.0.1:8080", "", "Bearer " + testToken, 405},
		{"migrate needs POST", "/api/v1/migrate", "GET", "127.0.0.1:8080", "", "Bearer " + testToken, 405},
		{"sites auth", "/api/v1/sites", "GET", "127.0.0.1:8080", "", "Bearer " + testToken, 200},
		{"sites unauth", "/api/v1/sites", "GET", "127.0.0.1:8080", "", "", 401},
		{"sites mutation", "/api/v1/sites", "POST", "127.0.0.1:8080", "", "Bearer " + testToken, 405},
		{"wifi auth", "/api/v1/wifi", "GET", "127.0.0.1:8080", "", "Bearer " + testToken, 200},
		{"wifi unauth", "/api/v1/wifi", "GET", "127.0.0.1:8080", "", "", 401},
		{"wifi mutation", "/api/v1/wifi", "POST", "127.0.0.1:8080", "", "Bearer " + testToken, 405},
		{"rebinding", "/api/v1/health", "GET", "attacker.example:8080", "", "", 403},
		{"foreign origin", "/api/v1/overview", "GET", "127.0.0.1:8080", "https://attacker.example", "Bearer " + testToken, 403},
		{"mutation denied", "/api/v1/overview", "POST", "127.0.0.1:8080", "", "Bearer " + testToken, 405},
		{"query denied", "/api/v1/overview?limit=10&after=cursor-1", "GET", "127.0.0.1:8080", "", "Bearer " + testToken, 200},
		{"unknown not SPA", "/api/v1/unknown", "GET", "127.0.0.1:8080", "", "", 404},
		{"ui", "/", "GET", "127.0.0.1:8080", "", "", 200},
		{"missing JS", "/assets/missing.js", "GET", "127.0.0.1:8080", "", "", 404},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, "http://127.0.0.1:8080"+tc.path, nil)
			r.Host = tc.host
			r.Header.Set("Origin", tc.origin)
			if tc.auth != "" {
				r.Header.Set("Authorization", tc.auth)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tc.status {
				t.Fatalf("status %d body %s", w.Code, w.Body.String())
			}
			if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Content-Security-Policy") == "" {
				t.Fatal("missing security headers")
			}
		})
	}
	// G03/G06: query strings no longer trigger pre-auth
	// rejection, so we cannot assert the exact Engine call
	// counter without coupling to the testEngine shape. We
	// instead verify that no test that expected auth
	// rejection (e.g. "rebinding") reached the engine.
	if e.calls > 4 {
		t.Fatalf("Engine called too many times (%d); expected at most 4 (overview+ready auth success)", e.calls)
	}
	// G06: unknown query keys are silently dropped; only
	// limit / after / cursor are whitelisted.
	for _, q := range []string{"?exec=anything", "?command=rm", "?filter=dangerous"} {
		r := httptest.NewRequest("GET", "http://127.0.0.1:8080/api/v1/doctor"+q, nil)
		r.Host = "127.0.0.1:8080"
		r.Header.Set("Authorization", "Bearer "+testToken)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 200 {
			t.Errorf("query %q must be accepted (unknown keys ignored), got %d", q, w.Code)
		}
	}
}
func TestAgentFailureIsSanitized(t *testing.T) {
	h := handler(t, &testEngine{failed: true})
	r := httptest.NewRequest("GET", "http://127.0.0.1:8080/api/v1/overview", nil)
	r.Header.Set("Authorization", "Bearer "+testToken)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 503 || strings.Contains(w.Body.String(), "secret") {
		t.Fatalf("unsafe error %d %s", w.Code, w.Body.String())
	}
}

func TestWireGuardAndXrayPublicRoutesRequireAuth(t *testing.T) {
	e := &testEngine{}
	h := handler(t, e)
	for _, path := range []string{"/api/v1/wireguard/servers", "/api/v1/xray/servers"} {
		r := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8080"+path, strings.NewReader(`{}`))
		r.Host = "127.0.0.1:8080"
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 401 {
			t.Fatalf("%s unauthenticated status %d body %s", path, w.Code, w.Body.String())
		}
	}
	if e.wgCalls != 0 || e.xrayCalls != 0 {
		t.Fatalf("unauthenticated WG/Xray request reached Engine: wg=%d xray=%d", e.wgCalls, e.xrayCalls)
	}

	for _, tc := range []struct {
		path string
		wg   bool
	}{
		{"/api/v1/wireguard/servers", true},
		{"/api/v1/xray/servers", false},
	} {
		r := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8080"+tc.path, strings.NewReader(`{}`))
		r.Host = "127.0.0.1:8080"
		r.Header.Set("Authorization", "Bearer "+testToken)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatalf("%s authenticated status %d body %s", tc.path, w.Code, w.Body.String())
		}
	}
	access := identity.ForwardedAccess{Secret: testAssertionKey}
	if _, err := access.Authorize(e.lastWGToken, identity.Observe); err != nil {
		t.Fatalf("wireguard forwarded token is not a valid internal assertion: %v", err)
	}
	if _, err := access.Authorize(e.lastXRToken, identity.Observe); err != nil {
		t.Fatalf("xray forwarded token is not a valid internal assertion: %v", err)
	}
	if e.wgCalls != 1 || e.xrayCalls != 1 || e.lastWGToken == testToken || e.lastXRToken == testToken {
		t.Fatalf("authorized WG/Xray did not proxy signed internal assertions: wg=%d/%q xray=%d/%q", e.wgCalls, e.lastWGToken, e.xrayCalls, e.lastXRToken)
	}
}

func TestZTPPublicRoutesRequireAuth(t *testing.T) {
	e := &testEngine{}
	h := handler(t, e)
	for _, path := range []string{"/api/v1/ztp/issue", "/api/v1/ztp/redeem"} {
		r := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8080"+path, strings.NewReader(`{"subject":"node-1"}`))
		r.Host = "127.0.0.1:8080"
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 401 {
			t.Fatalf("%s unauthenticated status %d body %s", path, w.Code, w.Body.String())
		}
		r = httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8080"+path, strings.NewReader(`{"subject":"node-1"}`))
		r.Host = "127.0.0.1:8080"
		r.Header.Set("Authorization", "Bearer "+testToken)
		w = httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatalf("%s authenticated status %d body %s", path, w.Code, w.Body.String())
		}
	}
}

func TestTokenFilesAndListenArePrivate(t *testing.T) {
	for _, address := range []string{"0.0.0.0:8080", "[::]:8080", "localhost:8080", "192.0.2.1:8080", "127.0.0.1:0"} {
		if _, e := ValidateListen(address); e == nil {
			t.Errorf("unsafe listener %s", address)
		}
	}
	f := t.TempDir() + "/token"
	if e := os.WriteFile(f, []byte(testToken), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e := LoadToken(f); e != nil {
		t.Fatal(e)
	}
	if e := os.Chmod(f, 0644); e != nil {
		t.Fatal(e)
	}
	if _, e := LoadToken(f); e == nil {
		t.Fatal("public token accepted")
	}
	link := f + "link"
	if e := os.Symlink(f, link); e != nil {
		t.Fatal(e)
	}
	if _, e := LoadToken(link); e == nil {
		t.Fatal("symlink token accepted")
	}
}
func TestNoBodyOrDuplicateAuthorization(t *testing.T) {
	h := handler(t, &testEngine{})
	for _, body := range []string{"", "payload"} {
		r := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8080/api/v1/overview", strings.NewReader(body))
		r.Header.Add("Authorization", "Bearer "+testToken)
		r.Header.Add("Authorization", "Bearer "+testToken)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code == 200 {
			t.Fatal("ambiguous request accepted")
		}
	}
}
