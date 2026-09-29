package agent

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/ups"
)

// makeFactory returns a factory that produces a
// Runner bound to a fake Executor. The fake
// returns the supplied map of per-device
// results; the Runner uses the standard parse
// path so the test exercises the production
// classifier.
func makeFactory(t *testing.T, per map[string]string, errs map[string]error) UPSProberFactory {
	t.Helper()
	return func() *ups.Runner {
		return ups.NewRunner(&fakeExec{results: per, errs: errs})
	}
}

type fakeExec struct {
	results map[string]string
	errs    map[string]error
}

func (f *fakeExec) Run(_ context.Context, argv []string) ([]byte, error) {
	if len(argv) < 2 {
		return nil, nil
	}
	dev := argv[1]
	if err, ok := f.errs[dev]; ok {
		return nil, err
	}
	if out, ok := f.results[dev]; ok {
		return []byte(out), nil
	}
	return []byte("ups.status: OL\n"), nil
}

func TestNewUPSHostFactoryError(t *testing.T) {
	if _, err := NewUPSHost(nil); err == nil {
		t.Fatalf("expected nil factory to be rejected")
	}
}

func TestNewUPSHostInitialTopology(t *testing.T) {
	factory := makeFactory(t, nil, nil)
	d := ups.Device{ID: "ups-1", Name: "Office UPS", Driver: "usbhid-ups", Port: "auto"}
	tp, err := ups.Build([]ups.Device{d}, time.Unix(1, 0))
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	h, err := NewUPSHostWithState(tp, factory)
	if err != nil {
		t.Fatalf("new host: %v", err)
	}
	st, err := h.UPS(context.Background())
	if err != nil {
		t.Fatalf("ups: %v", err)
	}
	if st.HasDevices != 1 {
		t.Fatalf("expected 1 device, got %d", st.HasDevices)
	}
}

func TestUPSHostProbeUpdatesHealth(t *testing.T) {
	results := map[string]string{
		"ups-1": "ups.status: OB DISCHRG\nbattery.charge: 80\n",
	}
	factory := makeFactory(t, results, nil)
	d := ups.Device{ID: "ups-1", Name: "Office UPS", Driver: "usbhid-ups", Port: "auto"}
	tp, _ := ups.Build([]ups.Device{d}, time.Unix(1, 0))
	h, _ := NewUPSHostWithState(tp, factory)
	if err := h.Probe(context.Background()); err != nil {
		t.Fatalf("probe: %v", err)
	}
	st, _ := h.UPS(context.Background())
	if st.Health["ups-1"] != ups.HealthOnBattery {
		t.Fatalf("expected on-battery, got %s", st.Health["ups-1"])
	}
	if st.ProbedAt == 0 {
		t.Fatalf("expected probedAt to be set")
	}
	if len(st.Variables["ups-1"]) == 0 {
		t.Fatalf("expected variables to be recorded")
	}
}

func TestUPSHostProbeMarksError(t *testing.T) {
	errs := map[string]error{"ups-1": context.DeadlineExceeded}
	factory := makeFactory(t, nil, errs)
	d := ups.Device{ID: "ups-1", Name: "Office UPS", Driver: "usbhid-ups", Port: "auto"}
	tp, _ := ups.Build([]ups.Device{d}, time.Unix(1, 0))
	h, _ := NewUPSHostWithState(tp, factory)
	if err := h.Probe(context.Background()); err != nil {
		t.Fatalf("probe should not return error, got %v", err)
	}
	st, _ := h.UPS(context.Background())
	if st.LastError == "" {
		t.Fatalf("expected last error to be recorded")
	}
}

func TestUPSHostNilReceiver(t *testing.T) {
	var h *UPSHost
	if _, err := h.UPS(context.Background()); err == nil {
		t.Fatalf("expected nil receiver to fail")
	}
}

func TestServeUPSEndpointWithHost(t *testing.T) {
	factory := makeFactory(t, nil, nil)
	h, _ := NewUPSHost(factory)
	srv := NewHandler(nil)
	srv.EnableUPS(h)
	r := httptest.NewRequest("GET", UPSPath, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d (%s)", w.Code, w.Body.String())
	}
	if w.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("expected json content type")
	}
}

func TestServeUPSEndpointWithoutHost(t *testing.T) {
	srv := NewHandler(nil)
	r := httptest.NewRequest("GET", UPSPath, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	if w.Code != 503 {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

func TestServeUPSMethodNotAllowed(t *testing.T) {
	srv := NewHandler(nil)
	r := httptest.NewRequest("POST", UPSPath, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	if w.Code != 405 {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestServeUPSProbeEndpoint(t *testing.T) {
	results := map[string]string{
		"ups-1": "ups.status: OL\nbattery.charge: 100\n",
	}
	factory := makeFactory(t, results, nil)
	d := ups.Device{ID: "ups-1", Name: "Office UPS", Driver: "usbhid-ups", Port: "auto"}
	tp, _ := ups.Build([]ups.Device{d}, time.Unix(1, 0))
	h, _ := NewUPSHostWithState(tp, factory)
	srv := NewHandler(nil)
	srv.EnableUPS(h)
	r := httptest.NewRequest("POST", UPSProbePath, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	if w.Code != 204 {
		t.Fatalf("expected 204, got %d (%s)", w.Code, w.Body.String())
	}
	st, _ := h.UPS(context.Background())
	if st.Health["ups-1"] != ups.HealthOnline {
		t.Fatalf("expected online after probe, got %s", st.Health["ups-1"])
	}
}

func TestServeUPSProbeRecordsError(t *testing.T) {
	// A probe whose executor fails is still a
	// successful round-trip: the error is
	// recorded in the state and the handler
	// returns 204. The handler does not surface
	// per-device probe errors as a service
	// failure; the GUI/CLI reads the per-device
	// health to know which device failed.
	errs := map[string]error{"ups-1": context.DeadlineExceeded}
	factory := makeFactory(t, nil, errs)
	d := ups.Device{ID: "ups-1", Name: "Office UPS", Driver: "usbhid-ups", Port: "auto"}
	tp, _ := ups.Build([]ups.Device{d}, time.Unix(1, 0))
	h, _ := NewUPSHostWithState(tp, factory)
	srv := NewHandler(nil)
	srv.EnableUPS(h)
	r := httptest.NewRequest("POST", UPSProbePath, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	if w.Code != 204 {
		t.Fatalf("expected 204, got %d", w.Code)
	}
	st, _ := h.UPS(context.Background())
	if st.LastError == "" {
		t.Fatalf("expected last error to be recorded in state")
	}
}
