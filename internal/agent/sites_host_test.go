package agent

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/sites"
)

func sampleSite() sites.Site {
	return sites.Site{ID: "blog", Name: "Personal blog", Domain: "blog.example.com", Listener: sites.Listener{InterfaceID: "wan1", Port: 443, Protocol: sites.ProtocolTLS}, Target: sites.Target{Host: "192.168.1.10", Port: 8080, Protocol: sites.ProtocolHTTP}, Enabled: true}
}

func TestNewSitesHostWithStateBad(t *testing.T) {
	bad := sites.Topology{SchemaVersion: "0"}
	if _, err := NewSitesHostWithState(bad); err == nil {
		t.Fatalf("expected bad topology to be rejected")
	}
}

func TestSitesHostRoundTrip(t *testing.T) {
	tp, err := sites.Build([]sites.Site{sampleSite()}, time.Unix(1, 0))
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	h, err := NewSitesHostWithState(tp)
	if err != nil {
		t.Fatalf("new host: %v", err)
	}
	st, err := h.Sites(context.Background())
	if err != nil {
		t.Fatalf("sites: %v", err)
	}
	if st.HasSites != 1 {
		t.Fatalf("expected 1 site, got %d", st.HasSites)
	}
	if st.Health["blog"] != sites.HealthEnabled {
		t.Fatalf("expected enabled, got %s", st.Health["blog"])
	}
}

func TestSitesHostUpdateTopology(t *testing.T) {
	h := NewSitesHost()
	if err := h.UpdateTopology(sites.Topology{SchemaVersion: sites.SchemaVersion}); err == nil {
		t.Fatalf("expected empty topology to be rejected")
	}
	tp, _ := sites.Build([]sites.Site{sampleSite()}, time.Unix(1, 0))
	if err := h.UpdateTopology(tp); err != nil {
		t.Fatalf("update: %v", err)
	}
	if h.state == nil {
		t.Fatalf("expected non-nil state")
	}
	// disabled site
	disabled := sampleSite()
	disabled.ID = "x"
	disabled.Enabled = false
	tp2, _ := sites.Build([]sites.Site{disabled}, time.Unix(1, 0))
	if err := h.UpdateTopology(tp2); err != nil {
		t.Fatalf("update: %v", err)
	}
	st, _ := h.Sites(context.Background())
	if st.Health["x"] != sites.HealthDisabled {
		t.Fatalf("expected disabled, got %s", st.Health["x"])
	}
}

func TestSitesHostNilReceiver(t *testing.T) {
	var h *SitesHost
	if _, err := h.Sites(context.Background()); err == nil {
		t.Fatalf("expected nil receiver to fail")
	}
}

func TestServeSitesEndpoint(t *testing.T) {
	tp, _ := sites.Build([]sites.Site{sampleSite()}, time.Unix(1, 0))
	h, _ := NewSitesHostWithState(tp)
	srv := NewHandler(nil)
	srv.EnableSites(h)
	r := httptest.NewRequest("GET", SitesPath, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d (%s)", w.Code, w.Body.String())
	}
	if w.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("expected json content type")
	}
}

func TestServeSitesWithoutHost(t *testing.T) {
	srv := NewHandler(nil)
	r := httptest.NewRequest("GET", SitesPath, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	if w.Code != 503 {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

func TestServeSitesMethodNotAllowed(t *testing.T) {
	srv := NewHandler(nil)
	r := httptest.NewRequest("POST", SitesPath, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	if w.Code != 405 {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}
