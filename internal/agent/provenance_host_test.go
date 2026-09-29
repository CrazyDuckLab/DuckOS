// G08 dependency/ownership/provenance IPC torture regression.
package agent

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/lifecycle"
)

func newProvHost(t *testing.T) *ProvenanceHost {
	t.Helper()
	g := lifecycle.NewGraph()
	obj := lifecycle.Object{Kind: lifecycle.KindVLAN, ID: "vlan-10"}
	if err := g.SetOwnership(obj, lifecycle.OwnershipRecord{Generator: "template", Manager: "operator", Source: "vpn-defaults", Tags: []string{"a", "b"}}); err != nil {
		t.Fatal(err)
	}
	if err := g.AppendProvenance(obj, lifecycle.ProvenanceEvent{Kind: lifecycle.ProvCreated, Actor: "agent", At: time.Unix(1, 0).UTC()}); err != nil {
		t.Fatal(err)
	}
	if err := g.AppendProvenance(obj, lifecycle.ProvenanceEvent{Kind: lifecycle.ProvModified, Actor: "operator", At: time.Unix(2, 0).UTC(), Revision: "abc"}); err != nil {
		t.Fatal(err)
	}
	return NewProvenanceHost(g)
}

func TestProvenanceHost_GET_ReturnsOwnershipAndHistory(t *testing.T) {
	h := newProvHost(t)
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, ProvenancePath+"?kind=vlan&id=vlan-10", nil)
	h.ServeProvenance(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.Code, resp.Body.String())
	}
	var msg provenanceResponse
	if err := json.Unmarshal(resp.Body.Bytes(), &msg); err != nil {
		t.Fatalf("decode: %v body=%s", err, resp.Body.String())
	}
	if msg.Ownership == nil || msg.Ownership.Generator != "template" {
		t.Fatalf("ownership missing: %+v", msg.Ownership)
	}
	if len(msg.History) != 2 {
		t.Fatalf("history must contain 2 events, got %d", len(msg.History))
	}
	if msg.History[0].Kind != lifecycle.ProvModified {
		t.Fatalf("newest first: got %v", msg.History)
	}
}

func TestProvenanceHost_GET_EmptyEnvelopeForUnknownObject(t *testing.T) {
	h := newProvHost(t)
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, ProvenancePath+"?kind=vlan&id=missing", nil)
	h.ServeProvenance(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	var msg provenanceResponse
	if err := json.Unmarshal(resp.Body.Bytes(), &msg); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if msg.Ownership != nil || len(msg.History) != 0 {
		t.Fatalf("unknown object must return empty ownership + history, got %+v", msg)
	}
}

func TestProvenanceHost_GET_NoQuery(t *testing.T) {
	h := newProvHost(t)
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, ProvenancePath, nil)
	h.ServeProvenance(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
}

func TestProvenanceHost_RejectsPartialQuery(t *testing.T) {
	h := newProvHost(t)
	for _, q := range []string{"?kind=vlan", "?id=vlan-10"} {
		resp := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, ProvenancePath+q, nil)
		h.ServeProvenance(resp, req)
		if resp.Code != http.StatusBadRequest {
			t.Fatalf("partial query %q must return 400, got %d", q, resp.Code)
		}
	}
}

func TestProvenanceHost_RejectsNonGET(t *testing.T) {
	h := newProvHost(t)
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		resp := httptest.NewRecorder()
		req := httptest.NewRequest(method, ProvenancePath, strings.NewReader("{}"))
		req.Header.Set("Content-Type", "application/json")
		h.ServeProvenance(resp, req)
		if resp.Code/100 != 4 {
			t.Errorf("%s: expected 4xx, got %d", method, resp.Code)
		}
		if resp.Header().Get("Allow") != "GET" {
			t.Errorf("%s: missing Allow header", method)
		}
	}
}

func TestProvenanceHost_RejectsBody(t *testing.T) {
	h := newProvHost(t)
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, ProvenancePath, strings.NewReader("not empty"))
	req.ContentLength = int64(len("not empty"))
	h.ServeProvenance(resp, req)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("body must be rejected, got %d", resp.Code)
	}
}

func TestProvenanceHost_RejectsInvalidKind(t *testing.T) {
	h := newProvHost(t)
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, ProvenancePath+"?kind=bogus&id=x", nil)
	h.ServeProvenance(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("invalid kind must still respond 200 with empty envelope, got %d", resp.Code)
	}
	var msg provenanceResponse
	_ = json.Unmarshal(resp.Body.Bytes(), &msg)
	if msg.Ownership != nil || len(msg.History) != 0 {
		t.Fatalf("invalid kind must return empty envelope, got %+v", msg)
	}
}

func TestProvenanceHost_ObserverFallback(t *testing.T) {
	// Wire a generic observer into the handler dispatcher.
	g := lifecycle.NewGraph()
	_ = g.SetOwnership(lifecycle.Object{Kind: lifecycle.KindWAN, ID: "wan-1"}, lifecycle.OwnershipRecord{Generator: "manual", Manager: "operator"})
	h := &Handler{provenance: &genericObserver{g: g}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, ProvenancePath+"?kind=wan&id=wan-1", nil)
	h.serveProvenance(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"generator":"manual"`) {
		t.Fatalf("fallback must surface ownership, got %s", rec.Body.String())
	}
}

func TestProvenanceHost_NilHostReturnsEmpty(t *testing.T) {
	var h *ProvenanceHost
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, ProvenancePath, nil)
	h.ServeProvenance(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("nil host must still render 200, got %d", resp.Code)
	}
	var msg provenanceResponse
	if err := json.Unmarshal(resp.Body.Bytes(), &msg); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if msg.Ownership != nil || len(msg.History) != 0 {
		t.Fatalf("nil host must return empty envelope, got %+v", msg)
	}
}

func TestProvenanceHost_HandlesNilGraph(t *testing.T) {
	h := NewProvenanceHost(nil)
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, ProvenancePath, nil)
	h.ServeProvenance(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("nil graph must still render 200, got %d", resp.Code)
	}
}

// genericObserver implements ProvenanceObserver without
// embedding lifecycle.Graph so the handler fallback path
// is exercised.
type genericObserver struct{ g *lifecycle.Graph }

func (o *genericObserver) Ownership(obj lifecycle.Object) (lifecycle.OwnershipRecord, bool) {
	return o.g.Ownership(obj)
}
func (o *genericObserver) Provenance(obj lifecycle.Object) []lifecycle.ProvenanceEvent {
	return o.g.Provenance(obj)
}

// silence unused-import warning for errors.
var _ = errors.New
