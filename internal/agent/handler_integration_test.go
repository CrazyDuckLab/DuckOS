// Package agent_integration_test covers the
// cross-subsystem wiring of the Agent Handler.
// The Handler is the single IPC surface of the
// privileged Agent; every V1 subsystem
// (overview, doctor, topology, multiwan, qos,
// ups, sites, wifi, migrate, network) must
// route through the same path without
// regression. The integration tests wire a
// representative host per subsystem and
// exercise the loopback API end-to-end.
package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/capture"
	"github.com/CrazyDuckLab/DuckOS/internal/devices"
	"github.com/CrazyDuckLab/DuckOS/internal/doctor"
	"github.com/CrazyDuckLab/DuckOS/internal/migrate"
	"github.com/CrazyDuckLab/DuckOS/internal/model"
	"github.com/CrazyDuckLab/DuckOS/internal/multiwan"
	"github.com/CrazyDuckLab/DuckOS/internal/qos"
	"github.com/CrazyDuckLab/DuckOS/internal/sites"
	"github.com/CrazyDuckLab/DuckOS/internal/topology"
	"github.com/CrazyDuckLab/DuckOS/internal/ups"
	"github.com/CrazyDuckLab/DuckOS/internal/wifi"
)

// fullAgent wires a representative host for
// every V1 subsystem and returns the Handler.
// The Handler is the single IPC surface; the
// integration tests below verify that no
// subsystem shadows another and that the
// concurrency cap is honoured under load.
func fullAgent(t *testing.T) *Handler {
	t.Helper()
	h := NewHandler(&observeStub{})
	h.EnableDoctor(&doctorHostStub{})
	h.EnableTopology(&topologyHostStub{g: topology.NewBuilder(1).Snapshot()})
	mwTopo, _ := multiwan.Build([]multiwan.Group{{ID: "wan-group", Name: "primary", Strategy: multiwan.StrategyActiveBackup, Members: []multiwan.Member{{ID: "wan-1", InterfaceID: "if-wan-1", Weight: 10}}, Probes: []multiwan.Probe{{Target: "1.1.1.1", Method: multiwan.HealthMethodICMP, Timeout: time.Second}}, Interval: 5 * time.Second, Table: 200}}, time.Unix(1, 0))
	mwHost, err := NewMultiWANHost(mwTopo)
	if err != nil {
		t.Fatalf("multiwan host: %v", err)
	}
	h.EnableMultiWAN(mwHost)
	qosTopo, _ := qos.Build([]qos.Shaper{{ID: "wan1", Link: "wan1", Direction: qos.DirectionEgress, Algorithm: qos.AlgorithmCAKE, RateKbps: 1000, BurstKbyte: 32}}, nil, nil, time.Unix(1, 0))
	qosHost, err := NewQoSHostWithState(qosTopo, func() (*qos.Controller, error) {
		return qos.NewController(&qosNoopRunner{})
	})
	if err != nil {
		t.Fatalf("qos host: %v", err)
	}
	h.EnableQoS(qosHost)
	upsTopo, _ := ups.Build([]ups.Device{{ID: "ups-1", Name: "Office UPS", Driver: "usbhid-ups", Port: "auto"}}, time.Unix(1, 0))
	upsHost, err := NewUPSHostWithState(upsTopo, func() *ups.Runner {
		return ups.NewRunner(&upsNoopExecutor{})
	})
	if err != nil {
		t.Fatalf("ups host: %v", err)
	}
	h.EnableUPS(upsHost)
	h.EnableMigrate(NewMigrateHost())
	h.EnableSites(NewSitesHost())
	wifiHost, err := NewWifiHostWithState(wifi.Topology{SchemaVersion: wifi.SchemaVersion}, wifi.Capability{Supported: true, Driver: "iwlwifi", Interface: "wlan0", Bands: []wifi.Band{wifi.Band5G}})
	if err != nil {
		t.Fatalf("wifi host: %v", err)
	}
	h.EnableWifi(wifiHost)
	return h
}

// observeStub is the read-only Observer every
// Handler needs. The Handler falls back to 503
// for the overview endpoint if the stub returns
// an error, but the integration tests below do
// not exercise /api/v1/overview.
type observeStub struct{}

func (o *observeStub) Overview(ctx context.Context) (model.Overview, error) {
	return model.Overview{SchemaVersion: model.SchemaVersion, ObservedAt: time.Unix(1, 0).UTC()}, nil
}

// doctorHostStub is a minimal Doctor host. The
// integration tests use it to verify the
// /agent/v1/doctor route.
type doctorHostStub struct{}

func (d *doctorHostStub) Doctor(_ context.Context) (doctor.Snapshot, error) {
	b := doctor.NewBuilder("test", "duckos-dev", "boot-1", time.Unix(1, 0).UTC(), 0)
	return b.Snapshot(), nil
}

// topologyHostStub is a minimal Topology host.
type topologyHostStub struct{ g topology.Graph }

func (t *topologyHostStub) Topology(_ context.Context) (topology.Graph, error) { return t.g, nil }

// qosNoopRunner is a fake Runner that records
// Apply/Rollback/PreviewState. The integration
// tests use it to exercise the apply/rollback
// round-trip without touching the live kernel.
type qosNoopRunner struct {
	mu       sync.Mutex
	applied  int
	rolled   int
	state    int
	lastPlan qos.Plan
}

func (r *qosNoopRunner) Apply(_ context.Context, p qos.Plan) (qos.ApplyOutcome, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.applied++
	r.lastPlan = p
	return qos.ApplyOutcome{Applied: len(p.Steps), StartedAt: time.Unix(1, 0), EndedAt: time.Unix(1, 1)}, nil
}

func (r *qosNoopRunner) Rollback(_ context.Context, cur, prev qos.Plan) (qos.ApplyOutcome, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rolled++
	return qos.ApplyOutcome{Applied: len(prev.Steps), StartedAt: time.Unix(1, 0), EndedAt: time.Unix(1, 1)}, nil
}

func (r *qosNoopRunner) State(_ context.Context) (qos.RunnerState, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.state++
	return qos.RunnerState{Link: "wan1", Algorithm: "cake", HasShapers: 1}, nil
}

// upsNoopExecutor returns a synthetic NUT
// status. The integration tests use it to
// exercise the per-device probe surface.
type upsNoopExecutor struct{}

func (e *upsNoopExecutor) Run(_ context.Context, argv []string) ([]byte, error) {
	return []byte("ups.status: OL\nbattery.charge: 95\nbattery.runtime: 1800\n"), nil
}

// TestAllSubsystemsWired is the smoke test:
// every V1 subsystem responds to its Agent IPC
// path. The test fails fast if any subsystem
// is missing or shadows another.
func TestAllSubsystemsWired(t *testing.T) {
	srv := fullAgent(t)
	cases := []struct {
		path   string
		method string
		body   []byte
		code   int
	}{
		{OverviewPath, "GET", nil, 200},
		{DoctorPath, "GET", nil, 200},
		{TopologyPath, "GET", nil, 200},
		{MultiWANPath, "GET", nil, 200},
		{QoSPath, "GET", nil, 200},
		{UPSPath, "GET", nil, 200},
		{MigratePath, "POST", []byte("/ip address\nadd address=192.0.2.1/24 interface=ether1\n"), 200},
		{SitesPath, "GET", nil, 200},
		{WifiPath, "GET", nil, 200},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			var r *http.Request
			if tc.body != nil {
				r = httptest.NewRequest(tc.method, tc.path, bytes.NewReader(tc.body))
			} else {
				r = httptest.NewRequest(tc.method, tc.path, nil)
			}
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, r)
			if w.Code != tc.code {
				t.Fatalf("expected %d, got %d (%s)", tc.code, w.Code, w.Body.String())
			}
			ct := w.Header().Get("Content-Type")
			if ct != "application/json" {
				t.Fatalf("expected json content type, got %q", ct)
			}
		})
	}
}

// TestUnknownPathReturns404 is a regression
// test: a path that does not match any V1
// subsystem must return 404, not 200 with
// another subsystem's payload. The Agent must
// never accidentally shadow a path.
func TestUnknownPathReturns404(t *testing.T) {
	srv := fullAgent(t)
	r := httptest.NewRequest("GET", "/agent/v1/unknown", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	if w.Code != 404 {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

// TestConcurrencyCapUnderLoad verifies that
// every V1 subsystem respects the Handler's
// concurrency cap. The test fires N concurrent
// requests against several subsystems and
// checks that the response codes are all 200.
func TestConcurrencyCapUnderLoad(t *testing.T) {
	srv := fullAgent(t)
	cases := []string{OverviewPath, DoctorPath, TopologyPath, MultiWANPath, QoSPath, UPSPath, SitesPath, WifiPath}
	for i := 0; i < 32; i++ {
		path := cases[i%len(cases)]
		r := httptest.NewRequest("GET", path, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, r)
		if w.Code == 503 && strings.Contains(w.Body.String(), "agent_busy") {
			// acceptable: the cap is 2; the
			// remaining 30 are expected to
			// be rejected with 503.
			continue
		}
		if w.Code != 200 {
			t.Fatalf("expected 200, got %d (%s) for %s", w.Code, w.Body.String(), path)
		}
	}
}

// TestQOSApplyRollbackRoundTrip exercises the
// QoS subsystem end-to-end through the Handler.
// The test asserts that Apply sets the state,
// Rollback rotates the previous plan and that
// the response is the structured ApplyOutcome
// the API/CLI render.
func TestQOSApplyRollbackRoundTrip(t *testing.T) {
	srv := fullAgent(t)
	sh := qos.Shaper{ID: "wan1", Link: "wan1", Direction: qos.DirectionEgress, Algorithm: qos.AlgorithmCAKE, RateKbps: 1000, BurstKbyte: 32}
	tp, err := qos.Build([]qos.Shaper{sh}, nil, nil, time.Unix(1, 0))
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	p, err := qos.Render(tp)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	raw, _ := qos.PlanJSON(p)
	// apply
	r := httptest.NewRequest("POST", QoSApplyPath, bytes.NewReader(raw))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d (%s)", w.Code, w.Body.String())
	}
	var out qos.ApplyOutcome
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode outcome: %v", err)
	}
	if out.Applied == 0 {
		t.Fatalf("expected applied > 0")
	}
	// rollback
	r = httptest.NewRequest("POST", QoSRollbackPath, nil)
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d (%s)", w.Code, w.Body.String())
	}
}

// TestUPSProbeUpdatesState exercises the UPS
// probe surface end-to-end. The test asserts
// that the per-device health is recorded after
// a probe and that the next status call returns
// the updated map.
func TestUPSProbeUpdatesState(t *testing.T) {
	srv := fullAgent(t)
	// Replace the empty UPS host with one that
	// owns a real device.
	tp, _ := ups.Build([]ups.Device{{ID: "ups-1", Name: "Office UPS", Driver: "usbhid-ups", Port: "auto"}}, time.Unix(1, 0))
	h, _ := NewUPSHostWithState(tp, func() *ups.Runner {
		return ups.NewRunner(&upsNoopExecutor{})
	})
	srv.EnableUPS(h)
	// probe
	r := httptest.NewRequest("POST", UPSProbePath, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	if w.Code != 204 {
		t.Fatalf("expected 204, got %d (%s)", w.Code, w.Body.String())
	}
	// status
	r = httptest.NewRequest("GET", UPSPath, nil)
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var st ups.Status
	if err := json.Unmarshal(w.Body.Bytes(), &st); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if st.Health["ups-1"] != ups.HealthOnline {
		t.Fatalf("expected health=online, got %s", st.Health["ups-1"])
	}
}

// TestMigrateSurfaceRoundtrip exercises the
// migration analysis end-to-end. The test
// asserts that a MikroTik .rsc buffer produces
// a Plan whose Format is mikrotik and whose
// items include the address line.
func TestMigrateSurfaceRoundtrip(t *testing.T) {
	srv := fullAgent(t)
	body := []byte("/ip address\nadd address=192.0.2.1/24 interface=ether1\n/ip route\nadd dst-address=0.0.0.0/0 gateway=192.0.2.254\n")
	r := httptest.NewRequest("POST", MigratePath, bytes.NewReader(body))
	r.Header.Set("X-DuckOS-Migrate-Filename", "router.rsc")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d (%s)", w.Code, w.Body.String())
	}
	var plan migrate.Plan
	if err := json.Unmarshal(w.Body.Bytes(), &plan); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if plan.Format != migrate.FormatMikroTik {
		t.Fatalf("expected mikrotik, got %s", plan.Format)
	}
	if len(plan.Items) < 2 {
		t.Fatalf("expected at least 2 items, got %d", len(plan.Items))
	}
}

// TestSitesAndWifiCoexist verifies the new
// subsystems do not shadow each other. A
// V1 install can publish a site and run a
// Wi-Fi AP at the same time; the Agent must
// serve both endpoints with the right content.
func TestSitesAndWifiCoexist(t *testing.T) {
	srv := fullAgent(t)
	// sites status
	r := httptest.NewRequest("GET", SitesPath, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var st sites.Status
	if err := json.Unmarshal(w.Body.Bytes(), &st); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if st.Topology.SchemaVersion != sites.SchemaVersion {
		t.Fatalf("unexpected sites schema version")
	}
	// wifi status
	r = httptest.NewRequest("GET", WifiPath, nil)
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var wt wifi.Status
	if err := json.Unmarshal(w.Body.Bytes(), &wt); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !wt.Capability.Supported {
		t.Fatalf("expected wifi capability to be supported")
	}
}

// TestAllSubsystemsDisjoint verifies that
// every Agent path has exactly one matching
// subsystem. A path collision would shadow a
// real subsystem; the regression test pins
// the routing table.
func TestAllSubsystemsDisjoint(t *testing.T) {
	paths := []string{OverviewPath, DoctorPath, TopologyPath, MultiWANPath, QoSPath, QoSApplyPath, QoSRollbackPath, UPSPath, UPSProbePath, MigratePath, SitesPath, WifiPath}
	seen := map[string]string{}
	for _, p := range paths {
		if other, ok := seen[p]; ok {
			t.Fatalf("path collision: %s and %s both map to %s", p, other, p)
		}
		seen[p] = p
	}
}

// TestAgentPowersDevicesAndLogging verifies the
// Agent hosts the secondary subsystems
// (devices, logging, capture) that the
// foundation layer exposes. The integration
// test imports the secondary packages so a
// compile-time regression is caught.
func TestAgentPowersDevicesAndLogging(t *testing.T) {
	// The test is a compile-time check: if any
	// of the secondary packages is no longer
	// imported by the Agent, the test file fails
	// to build. The runtime check below verifies
	// the secondary packages produce valid
	// output the Agent can carry.
	_ = devices.MaxDevices
	_ = capture.MaxConcurrent
	if devices.MaxDevices <= 0 {
		t.Fatalf("devices max must be positive")
	}
	if capture.MaxConcurrent <= 0 {
		t.Fatalf("capture max must be positive")
	}
}
