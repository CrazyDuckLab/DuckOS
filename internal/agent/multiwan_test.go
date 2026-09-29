package agent

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/multiwan"
)

type fakeMultiWANObserver struct {
	topology   multiwan.Topology
	selections map[string]multiwan.Selection
	health     map[string]string
	err        error
}

func (f *fakeMultiWANObserver) MultiWAN(context.Context) (multiwan.Topology, map[string]multiwan.Selection, map[string]string, error) {
	return f.topology, f.selections, f.health, f.err
}

func sampleMultiWANTopology() multiwan.Topology {
	g := multiwan.Group{
		ID:       "wan-group",
		Name:     "primary",
		Strategy: multiwan.StrategyActiveActive,
		Members: []multiwan.Member{
			{ID: "wan-1", InterfaceID: "if-wan-1", Weight: 10},
		},
		Probes: []multiwan.Probe{
			{Target: "1.1.1.1", Method: multiwan.HealthMethodICMP, Timeout: 1 * time.Second},
		},
		Interval: 10 * time.Second,
		Table:    200,
	}
	t, _ := multiwan.Build([]multiwan.Group{g}, time.Now())
	return t
}

func TestServeMultiWANRoundTrip(t *testing.T) {
	h := NewHandler(&fakeObserver{})
	t2 := sampleMultiWANTopology()
	h.EnableMultiWAN(&fakeMultiWANObserver{
		topology:   t2,
		selections: map[string]multiwan.Selection{"wan-group": {GroupID: "wan-group", MemberID: "wan-1", Interface: "if-wan-1", Health: "ok", SelectedAt: time.Now()}},
		health:     map[string]string{"wan-1": "up"},
	})
	req := httptest.NewRequest(http.MethodGet, MultiWANPath, nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d (%s)", w.Code, w.Body.String())
	}
	var payload struct {
		Topology   multiwan.Topology             `json:"topology"`
		Selections map[string]multiwan.Selection `json:"selections"`
		Health     map[string]string             `json:"health"`
	}
	if err := json.NewDecoder(w.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Topology.Groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(payload.Topology.Groups))
	}
	if payload.Selections["wan-group"].MemberID != "wan-1" {
		t.Fatalf("unexpected selection: %+v", payload.Selections["wan-group"])
	}
}

func TestServeMultiWANMissingHost(t *testing.T) {
	h := NewHandler(&fakeObserver{})
	req := httptest.NewRequest(http.MethodGet, MultiWANPath, nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 503 {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

func TestServeMultiWANRejectsMutation(t *testing.T) {
	h := NewHandler(&fakeObserver{})
	h.EnableMultiWAN(&fakeMultiWANObserver{topology: sampleMultiWANTopology()})
	req := httptest.NewRequest(http.MethodPost, MultiWANPath, nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 405 {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestServeMultiWANRejectsQuery(t *testing.T) {
	h := NewHandler(&fakeObserver{})
	h.EnableMultiWAN(&fakeMultiWANObserver{topology: sampleMultiWANTopology()})
	req := httptest.NewRequest(http.MethodGet, MultiWANPath+"?exec=1", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 400 {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestServeMultiWANObserverError(t *testing.T) {
	h := NewHandler(&fakeObserver{})
	h.EnableMultiWAN(&fakeMultiWANObserver{err: errors.New("nope")})
	req := httptest.NewRequest(http.MethodGet, MultiWANPath, nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 503 {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}
