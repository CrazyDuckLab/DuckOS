package agent

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/topology"
)

type fakeTopologyObserver struct {
	graph topology.Graph
	err   error
}

func (f *fakeTopologyObserver) Topology(context.Context) (topology.Graph, error) {
	return f.graph, f.err
}

func newGraph() topology.Graph {
	b := topology.NewBuilder(time.Now().Unix())
	b.AddInterface(topology.InterfaceSpec{Name: "lan0", State: "up", Addresses: []string{"192.0.2.1/24"}})
	b.AddInterface(topology.InterfaceSpec{Name: "br0", Kind: topology.NodeBridge, Parent: "lan0", State: "up"})
	return b.Snapshot()
}

func TestServeTopologyRoundTrip(t *testing.T) {
	h := NewHandler(&fakeObserver{})
	h.EnableTopology(&fakeTopologyObserver{graph: newGraph()})
	req := httptest.NewRequest(http.MethodGet, TopologyPath, nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d (%s)", w.Code, w.Body.String())
	}
	var g topology.Graph
	if err := json.NewDecoder(w.Body).Decode(&g); err != nil {
		t.Fatal(err)
	}
	if len(g.Nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(g.Nodes))
	}
}

func TestServeTopologyMissingHost(t *testing.T) {
	h := NewHandler(&fakeObserver{})
	req := httptest.NewRequest(http.MethodGet, TopologyPath, nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 503 {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

func TestServeTopologyRejectsMutation(t *testing.T) {
	h := NewHandler(&fakeObserver{})
	h.EnableTopology(&fakeTopologyObserver{graph: newGraph()})
	req := httptest.NewRequest(http.MethodPost, TopologyPath, nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 405 {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestServeTopologyRejectsQuery(t *testing.T) {
	h := NewHandler(&fakeObserver{})
	h.EnableTopology(&fakeTopologyObserver{graph: newGraph()})
	req := httptest.NewRequest(http.MethodGet, TopologyPath+"?exec=1", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 400 {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestServeTopologyRejectsInvalidGraph(t *testing.T) {
	h := NewHandler(&fakeObserver{})
	h.EnableTopology(&fakeTopologyObserver{graph: topology.Graph{SchemaVersion: "wrong"}})
	req := httptest.NewRequest(http.MethodGet, TopologyPath, nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 503 {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

func TestServeTopologyObserverError(t *testing.T) {
	h := NewHandler(&fakeObserver{})
	h.EnableTopology(&fakeTopologyObserver{err: errors.New("nope")})
	req := httptest.NewRequest(http.MethodGet, TopologyPath, nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 503 {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}
