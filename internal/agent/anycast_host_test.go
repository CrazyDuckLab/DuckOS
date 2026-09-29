// anycast_host_test.go pins the IPC surface for the
// Anycast evidence view (C-blocker #9). The host MUST
// render the bounded evidence + counters, never invent
// availability, and refuse non-GET methods.
package agent

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/anycast"
)

type fakeAnycastSource struct {
	evidence []anycast.Evidence
	counters anycast.Counters
	err      error
}

func (f *fakeAnycastSource) AllEvidence() []anycast.Evidence {
	return f.evidence
}

func (f *fakeAnycastSource) Counters() anycast.Counters {
	return f.counters
}

// TestAnycastHostRendersEvidence proves the read view
// surfaces the evidence + counters the Engine consumes.
func TestAnycastHostRendersEvidence(t *testing.T) {
	src := &fakeAnycastSource{
		evidence: []anycast.Evidence{
			{
				VIP: anycast.VIPIdentifier{Name: "primary", IP: "192.0.2.1"},
				Desired: &anycast.DesiredRecord{
					VIP:       anycast.VIPIdentifier{Name: "primary", IP: "192.0.2.1"},
					State:     anycast.HysteresisAnnounce,
					UpdatedAt: time.Unix(1, 0).UTC(),
				},
				LatestHealth: &anycast.HealthRecord{
					VIP:        anycast.VIPIdentifier{Name: "primary", IP: "192.0.2.1"},
					ObservedAt: time.Unix(2, 0).UTC(),
				},
			},
		},
		counters: anycast.Counters{},
	}
	h := NewAnycastHost(src)
	req := httptest.NewRequest(http.MethodGet, AnycastPath, nil)
	w := httptest.NewRecorder()
	h.ServeAnycast(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("got %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("content-type: %q, want application/json", ct)
	}
	var got AnycastStatus
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Version != 1 {
		t.Fatalf("version: %d, want 1", got.Version)
	}
	if len(got.Evidence) != 1 {
		t.Fatalf("evidence: %d, want 1", len(got.Evidence))
	}
	if got.Evidence[0].VIP.IP != "192.0.2.1" {
		t.Fatalf("VIP: %+v", got.Evidence[0].VIP)
	}
}

// TestAnycastHostRejectsNonGET pins the read-only
// contract. The host MUST never accept mutations over
// IPC; mutations live on the privileged side.
func TestAnycastHostRejectsNonGET(t *testing.T) {
	h := NewAnycastHost(nil)
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch} {
		req := httptest.NewRequest(method, AnycastPath, nil)
		w := httptest.NewRecorder()
		h.ServeAnycast(w, req)
		if w.Code != http.StatusMethodNotAllowed {
			t.Fatalf("%s: got %d, want 405", method, w.Code)
		}
	}
}

// TestAnycastHostEmptySourceIsHonest proves a nil
// source returns the honest empty view, not a fake
// availability badge.
func TestAnycastHostEmptySourceIsHonest(t *testing.T) {
	h := NewAnycastHost(nil)
	req := httptest.NewRequest(http.MethodGet, AnycastPath, nil)
	w := httptest.NewRecorder()
	h.ServeAnycast(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("got %d, want 200", w.Code)
	}
	var got AnycastStatus
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Evidence == nil {
		t.Fatal("evidence: nil, want empty slice")
	}
	if got.Startup != "ready" {
		t.Fatalf("startup: %q, want ready", got.Startup)
	}
}

// silence unused-import warning when the helper is
// unused after a future edit.
var _ = errors.New
