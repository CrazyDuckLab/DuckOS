package agent

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/qos"
)

// fakeSink records the calls the host made. The
// sink is optional; the test does not need to wire
// one unless it asserts on the recorded values.
type fakeSink struct {
	applied int
	err     string
}

func (f *fakeSink) MarkApplied(_ time.Time) { f.applied++ }
func (f *fakeSink) MarkError(msg string)    { f.err = msg }

// fakeController is a small in-memory substitute
// for *qos.Controller. The host depends on the
// qos.Runner contract through a Controller; the
// tests use a closure-based factory so the host is
// exercised without an io-bound runner.
func fakeControllerFactory(t *testing.T, failApply bool, failRollback bool) QoSControllerFactory {
	t.Helper()
	return func() (*qos.Controller, error) {
		runner := &recordingRunner{failApply: failApply, failRollback: failRollback}
		c, err := qos.NewController(runner)
		if err != nil {
			return nil, err
		}
		return c, nil
	}
}

// recordingRunner is a Runner that records calls
// and reports a deterministic outcome. The Runner
// never exercises the kernel; the host integration
// is the contract under test.
type recordingRunner struct {
	failApply    bool
	failRollback bool
	apply        int
	roll         int
}

func (r *recordingRunner) Apply(_ context.Context, p qos.Plan) (qos.ApplyOutcome, error) {
	r.apply++
	if r.failApply {
		return qos.ApplyOutcome{Error: "synthetic apply failure"}, errors.New("synthetic apply failure")
	}
	return qos.ApplyOutcome{Applied: len(p.Steps), StartedAt: time.Unix(1, 0), EndedAt: time.Unix(1, 1)}, nil
}

func (r *recordingRunner) Rollback(_ context.Context, cur, prev qos.Plan) (qos.ApplyOutcome, error) {
	r.roll++
	if r.failRollback {
		return qos.ApplyOutcome{Error: "synthetic rollback failure"}, errors.New("synthetic rollback failure")
	}
	return qos.ApplyOutcome{Applied: len(cur.Steps) + len(prev.Steps), StartedAt: time.Unix(1, 0), EndedAt: time.Unix(1, 1)}, nil
}

func (r *recordingRunner) State(_ context.Context) (qos.RunnerState, error) {
	return qos.RunnerState{HasShapers: 1, Algorithm: "cake"}, nil
}

// hostPlan returns a small plan the host tests
// can apply. The plan is built by the model so
// the host test does not depend on hand-rolled
// internals.
func hostPlan(t *testing.T) qos.Plan {
	t.Helper()
	sh := qos.Shaper{ID: "wan1", Link: "wan1", Direction: qos.DirectionEgress, Algorithm: qos.AlgorithmCAKE, RateKbps: 1000, BurstKbyte: 32}
	tp, err := qos.Build([]qos.Shaper{sh}, nil, nil, time.Unix(1, 0))
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	p, err := qos.Render(tp)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	return p
}

// doQoS is a tiny helper that issues a request to
// the supplied handler and returns the recorded
// response. The helper is local to the test file
// to keep the package API surface small.
func doQoS(t *testing.T, h http.Handler, method, path string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	var r *http.Request
	if body != nil {
		r = httptest.NewRequest(method, path, bytes.NewReader(body))
	} else {
		r = httptest.NewRequest(method, path, nil)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

// TestNewQoSHostFactoryError confirms a bad
// factory is reported at construction time. The
// Agent never exposes a half-constructed host.
func TestNewQoSHostFactoryError(t *testing.T) {
	if _, err := NewQoSHost(nil); err == nil {
		t.Fatalf("expected nil factory to be rejected")
	}
	bad := QoSControllerFactory(func() (*qos.Controller, error) { return nil, errors.New("boom") })
	if _, err := NewQoSHost(bad); err == nil {
		t.Fatalf("expected factory error to propagate")
	}
}

// TestNewQoSHostInitialTopology covers the
// with-state constructor: the supplied topology
// is committed to the host's state.
func TestNewQoSHostInitialTopology(t *testing.T) {
	factory := fakeControllerFactory(t, false, false)
	sh := qos.Shaper{ID: "wan1", Link: "wan1", Direction: qos.DirectionEgress, Algorithm: qos.AlgorithmCAKE, RateKbps: 1000, BurstKbyte: 32}
	tp, err := qos.Build([]qos.Shaper{sh}, nil, nil, time.Unix(1, 0))
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	h, err := NewQoSHostWithState(tp, factory)
	if err != nil {
		t.Fatalf("new host: %v", err)
	}
	st, err := h.QoS(context.Background())
	if err != nil {
		t.Fatalf("qos: %v", err)
	}
	if !st.Topology.HasShapers() {
		t.Fatalf("expected topology to be loaded, got %+v", st.Topology)
	}
}

// TestQoSHostApplyRollsState covers the happy
// path: a successful Apply records the timestamp
// and clears the previous error.
func TestQoSHostApplyRollsState(t *testing.T) {
	factory := fakeControllerFactory(t, false, false)
	h, err := NewQoSHost(factory)
	if err != nil {
		t.Fatalf("new host: %v", err)
	}
	sink := &fakeSink{}
	h.SetSink(sink)
	p := hostPlan(t)
	if _, err := h.Apply(context.Background(), p); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if sink.applied != 1 {
		t.Fatalf("expected sink to record 1 apply, got %d", sink.applied)
	}
	if sink.err != "" {
		t.Fatalf("expected sink error to be cleared, got %q", sink.err)
	}
	st, _ := h.QoS(context.Background())
	if st.LastApply != 1 {
		t.Fatalf("expected lastApply=1, got %d", st.LastApply)
	}
	if !st.Topology.HasShapers() {
		t.Fatalf("expected topology to be updated")
	}
}

// TestQoSHostApplyFailurePreservesState covers the
// failure path: a failed Apply records an error
// and the previous successful topology stays put.
func TestQoSHostApplyFailurePreservesState(t *testing.T) {
	factory := fakeControllerFactory(t, true, false)
	h, _ := NewQoSHost(factory)
	sink := &fakeSink{}
	h.SetSink(sink)
	p := hostPlan(t)
	if _, err := h.Apply(context.Background(), p); err == nil {
		t.Fatalf("expected apply to fail")
	}
	if sink.err == "" {
		t.Fatalf("expected sink to record error")
	}
	if sink.applied != 0 {
		t.Fatalf("expected sink to record 0 applies, got %d", sink.applied)
	}
}

// TestQoSHostRollback covers the rollback surface:
// the host asks the controller to roll back and
// records the outcome.
func TestQoSHostRollback(t *testing.T) {
	factory := fakeControllerFactory(t, false, false)
	h, _ := NewQoSHost(factory)
	sink := &fakeSink{}
	h.SetSink(sink)
	p1 := hostPlan(t)
	if _, err := h.Apply(context.Background(), p1); err != nil {
		t.Fatalf("apply p1: %v", err)
	}
	p2 := hostPlan(t)
	if _, err := h.Apply(context.Background(), p2); err != nil {
		t.Fatalf("apply p2: %v", err)
	}
	if _, err := h.Rollback(context.Background()); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	if sink.applied < 3 {
		t.Fatalf("expected at least 3 sink updates, got %d", sink.applied)
	}
}

// TestQoSHostRollbackFailureRecordsError covers
// the rollback failure path: the sink records the
// error and the host keeps its previous state.
func TestQoSHostRollbackFailureRecordsError(t *testing.T) {
	factory := fakeControllerFactory(t, false, true)
	h, _ := NewQoSHost(factory)
	sink := &fakeSink{}
	h.SetSink(sink)
	p := hostPlan(t)
	if _, err := h.Apply(context.Background(), p); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if _, err := h.Rollback(context.Background()); err == nil {
		t.Fatalf("expected rollback to fail")
	}
	if sink.err == "" {
		t.Fatalf("expected sink to record rollback error")
	}
}

// TestQoSHostPreviewState covers the runner state
// passthrough. The host does not cache the result.
func TestQoSHostPreviewState(t *testing.T) {
	factory := fakeControllerFactory(t, false, false)
	h, _ := NewQoSHost(factory)
	got, err := h.PreviewState(context.Background())
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if got.Algorithm != "cake" {
		t.Fatalf("expected algorithm=cake, got %q", got.Algorithm)
	}
	if got.HasShapers != 1 {
		t.Fatalf("expected hasShapers=1, got %d", got.HasShapers)
	}
}

// TestQoSHostApplyInvalidPlan covers the
// pre-runner gate: an invalid Plan is rejected
// without touching the controller.
func TestQoSHostApplyInvalidPlan(t *testing.T) {
	factory := fakeControllerFactory(t, false, false)
	h, _ := NewQoSHost(factory)
	bad := qos.Plan{SchemaVersion: "0"}
	if _, err := h.Apply(context.Background(), bad); err == nil {
		t.Fatalf("expected bad plan to be rejected")
	}
}

// TestServeQoSEndpointWithHost exercises the
// integration between ServeQoS and a real host.
// The handler must return the host's status
// payload, content-type application/json and a
// 200 status code.
func TestServeQoSEndpointWithHost(t *testing.T) {
	factory := fakeControllerFactory(t, false, false)
	h, err := NewQoSHost(factory)
	if err != nil {
		t.Fatalf("new host: %v", err)
	}
	srv := NewHandler(nil)
	srv.EnableQoS(h)
	rr := doQoS(t, srv, "GET", QoSPath, nil)
	if rr.Code != 200 {
		t.Fatalf("expected 200, got %d (%s)", rr.Code, rr.Body.String())
	}
	if rr.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("expected json content type, got %q", rr.Header().Get("Content-Type"))
	}
}

// TestServeQoSEndpointWithoutHost covers the
// "no host wired" path: the handler must return
// 503 qos_unavailable rather than crashing.
func TestServeQoSEndpointWithoutHost(t *testing.T) {
	srv := NewHandler(nil)
	rr := doQoS(t, srv, "GET", QoSPath, nil)
	if rr.Code != 503 {
		t.Fatalf("expected 503, got %d", rr.Code)
	}
}

// TestServeQoSEndpointMethodNotAllowed confirms
// the handler enforces the GET method.
func TestServeQoSEndpointMethodNotAllowed(t *testing.T) {
	srv := NewHandler(nil)
	rr := doQoS(t, srv, "POST", QoSPath, nil)
	if rr.Code != 405 {
		t.Fatalf("expected 405, got %d", rr.Code)
	}
}

// TestServeQoSApplyEndpoint exercises the apply
// IPC path. The handler must decode a valid plan,
// call the host and return the ApplyOutcome as
// JSON.
func TestServeQoSApplyEndpoint(t *testing.T) {
	factory := fakeControllerFactory(t, false, false)
	h, _ := NewQoSHost(factory)
	srv := NewHandler(nil)
	srv.EnableQoS(h)
	p := hostPlan(t)
	raw, _ := qos.PlanJSON(p)
	rr := doQoS(t, srv, "POST", QoSApplyPath, raw)
	if rr.Code != 200 {
		t.Fatalf("expected 200, got %d (%s)", rr.Code, rr.Body.String())
	}
}

// TestServeQoSApplyInvalidBody covers the
// reject path: a malformed plan body returns a
// 4xx rather than crashing.
func TestServeQoSApplyInvalidBody(t *testing.T) {
	factory := fakeControllerFactory(t, false, false)
	h, _ := NewQoSHost(factory)
	srv := NewHandler(nil)
	srv.EnableQoS(h)
	rr := doQoS(t, srv, "POST", QoSApplyPath, []byte(`{"schemaVersion":"0"}`))
	if rr.Code != 400 {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

// TestServeQoSApplyOversized covers the body cap.
// A plan larger than QoSMaxBytes is rejected.
func TestServeQoSApplyOversized(t *testing.T) {
	factory := fakeControllerFactory(t, false, false)
	h, _ := NewQoSHost(factory)
	srv := NewHandler(nil)
	srv.EnableQoS(h)
	body := make([]byte, QoSMaxBytes+2)
	for i := range body {
		body[i] = 'a'
	}
	rr := doQoS(t, srv, "POST", QoSApplyPath, body)
	if rr.Code != 413 && rr.Code != 400 {
		t.Fatalf("expected 413 or 400, got %d", rr.Code)
	}
}

// TestServeQoSRollbackEndpoint exercises the
// rollback IPC path. The handler must call the
// host and return the ApplyOutcome as JSON.
func TestServeQoSRollbackEndpoint(t *testing.T) {
	factory := fakeControllerFactory(t, false, false)
	h, _ := NewQoSHost(factory)
	srv := NewHandler(nil)
	srv.EnableQoS(h)
	p := hostPlan(t)
	raw, _ := qos.PlanJSON(p)
	if rr := doQoS(t, srv, "POST", QoSApplyPath, raw); rr.Code != 200 {
		t.Fatalf("expected 200 on apply, got %d (%s)", rr.Code, rr.Body.String())
	}
	rr := doQoS(t, srv, "POST", QoSRollbackPath, nil)
	if rr.Code != 200 {
		t.Fatalf("expected 200, got %d (%s)", rr.Code, rr.Body.String())
	}
}

// TestServeQoSRollbackEndpointNoPlan covers the
// no-current path: a rollback before any apply is
// a structured failure rather than a crash.
func TestServeQoSRollbackEndpointNoPlan(t *testing.T) {
	factory := fakeControllerFactory(t, false, false)
	h, _ := NewQoSHost(factory)
	srv := NewHandler(nil)
	srv.EnableQoS(h)
	rr := doQoS(t, srv, "POST", QoSRollbackPath, nil)
	if rr.Code != 503 {
		t.Fatalf("expected 503, got %d (%s)", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "qos_rollback_failed") {
		t.Fatalf("expected structured error, got %q", rr.Body.String())
	}
}
