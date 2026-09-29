// G19 diagnostic job IPC torture regression.
package agent

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/diagjob"
)

func newDiagJobHost(t *testing.T) *DiagJobHost {
	t.Helper()
	store := diagjob.NewStore()
	store.Append(diagjob.Job{
		ID:          "dns-1",
		Kind:        diagjob.KindDNSTrace,
		Status:      diagjob.StatusSucceeded,
		Subject:     "host.example.com",
		RequestedBy: "operator",
		StartedAt:   time.Unix(1, 0).UTC(),
		EndedAt:     time.Unix(2, 0).UTC(),
		OutputLines: []string{"resolved A 198.51.100.10"},
	})
	store.Append(diagjob.Job{
		ID:      "wan-1",
		Kind:    diagjob.KindWANProbe,
		Status:  diagjob.StatusRunning,
		Subject: "wan0",
	})
	return NewDiagJobHost(store)
}

func TestDiagJobHost_List(t *testing.T) {
	h := newDiagJobHost(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, DiagJobPath, nil)
	h.ServeDiagJobs(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var msg diagJobsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &msg); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(msg.Jobs) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(msg.Jobs))
	}
}

func TestDiagJobHost_SingleJob(t *testing.T) {
	h := newDiagJobHost(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, DiagJobPath+"?id=dns-1", nil)
	h.ServeDiagJobs(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var msg diagJobsResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &msg)
	if len(msg.Jobs) != 1 || msg.Jobs[0].ID != "dns-1" {
		t.Fatalf("expected dns-1 only, got %+v", msg.Jobs)
	}
}

func TestDiagJobHost_RejectsNonGET(t *testing.T) {
	h := newDiagJobHost(t)
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(method, DiagJobPath, strings.NewReader("{}"))
		req.Header.Set("Content-Type", "application/json")
		h.ServeDiagJobs(rec, req)
		if rec.Code/100 != 4 {
			t.Errorf("%s: expected 4xx, got %d", method, rec.Code)
		}
	}
}

func TestDiagJobHost_RejectsBody(t *testing.T) {
	h := newDiagJobHost(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, DiagJobPath, strings.NewReader("not empty"))
	req.ContentLength = int64(len("not empty"))
	h.ServeDiagJobs(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("body must be rejected, got %d", rec.Code)
	}
}

func TestDiagJobHost_MissingIDReturnsEmpty(t *testing.T) {
	h := newDiagJobHost(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, DiagJobPath+"?id=nope", nil)
	h.ServeDiagJobs(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var msg diagJobsResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &msg)
	if len(msg.Jobs) != 0 {
		t.Fatalf("missing id must return empty jobs, got %+v", msg.Jobs)
	}
}

func TestDiagJobHost_NilStoreStaysBenign(t *testing.T) {
	h := NewDiagJobHost(nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, DiagJobPath, nil)
	h.ServeDiagJobs(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("nil store must respond 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"jobs":[]`) {
		t.Fatalf("empty store must return empty jobs, got %s", rec.Body.String())
	}
}

func TestDiagJobHost_GenericObserverFallback(t *testing.T) {
	store := diagjob.NewStore()
	_ = store.Append(diagjob.Job{ID: "dns-1", Kind: diagjob.KindDNSTrace, Status: diagjob.StatusSucceeded})
	h := &Handler{diagjobs: &genericDiagJob{store: store}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, DiagJobPath, nil)
	h.serveDiagJobs(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"id":"dns-1"`) {
		t.Fatalf("fallback must surface jobs, got %s", rec.Body.String())
	}
}

func TestDiagJobHost_NoObserverReturns503(t *testing.T) {
	h := &Handler{}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, DiagJobPath, nil)
	h.serveDiagJobs(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("missing observer must yield 503, got %d", rec.Code)
	}
}

// genericDiagJob implements DiagJobObserver without
// embedding *diagjob.Store so the handler fallback path
// is exercised.
type genericDiagJob struct{ store *diagjob.Store }

func (g *genericDiagJob) List() []diagjob.Job               { return g.store.List() }
func (g *genericDiagJob) Get(id string) (diagjob.Job, bool) { return g.store.Get(id) }
