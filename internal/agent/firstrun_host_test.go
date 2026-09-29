// G18 first-run IPC torture regression.
package agent

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/firstrun"
)

func newFirstRunHost(t *testing.T) *FirstRunHost {
	t.Helper()
	dir := t.TempDir()
	return NewFirstRunHost(firstrun.NewStore(filepath.Join(dir, "firstrun.json")))
}

func TestFirstRunHost_PendingResponse(t *testing.T) {
	h := newFirstRunHost(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, FirstRunPath, nil)
	h.ServeFirstRun(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var msg firstRunResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &msg); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !msg.Pending || msg.Completed {
		t.Fatalf("first run must be pending, got %+v", msg)
	}
}

func TestFirstRunHost_CompletedResponse(t *testing.T) {
	h := newFirstRunHost(t)
	if _, err := h.store.MarkComplete(firstrun.State{}, "secret", "router", "en", "dark", "router-1", []string{"language"}); err != nil {
		t.Fatalf("MarkComplete: %v", err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, FirstRunPath, nil)
	h.ServeFirstRun(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var msg firstRunResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &msg); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !msg.Completed || msg.Pending {
		t.Fatalf("must report completed, got %+v", msg)
	}
	if msg.State == nil || msg.State.Mode != "router" {
		t.Fatalf("state missing or wrong: %+v", msg.State)
	}
}

func TestFirstRunHost_RejectsNonGET(t *testing.T) {
	h := newFirstRunHost(t)
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(method, FirstRunPath, strings.NewReader("{}"))
		req.Header.Set("Content-Type", "application/json")
		h.ServeFirstRun(rec, req)
		if rec.Code/100 != 4 {
			t.Errorf("%s: expected 4xx, got %d", method, rec.Code)
		}
	}
}

func TestFirstRunHost_RejectsBodyAndQuery(t *testing.T) {
	h := newFirstRunHost(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, FirstRunPath+"?foo=bar", nil)
	h.ServeFirstRun(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("query must be rejected, got %d", rec.Code)
	}
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, FirstRunPath, strings.NewReader("not empty"))
	req2.ContentLength = int64(len("not empty"))
	h.ServeFirstRun(rec2, req2)
	if rec2.Code != http.StatusBadRequest {
		t.Fatalf("body must be rejected, got %d", rec2.Code)
	}
}

func TestFirstRunHost_NilStoreStaysBenign(t *testing.T) {
	h := NewFirstRunHost(nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, FirstRunPath, nil)
	h.ServeFirstRun(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("nil store must still respond 200, got %d", rec.Code)
	}
}

func TestFirstRunHost_GenericObserverFallback(t *testing.T) {
	store := firstrun.NewStore("")
	_ = store // satisfy linter
	h := &Handler{firstrun: &genericFirstRun{state: firstrun.State{Version: 1, Mode: "full", CompletedAt: time.Now().UTC()}}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, FirstRunPath, nil)
	h.serveFirstRun(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"completed":true`) {
		t.Fatalf("fallback must surface completed, got %s", rec.Body.String())
	}
}

func TestFirstRunHost_GenericObserverMissing(t *testing.T) {
	h := &Handler{firstrun: &genericFirstRun{err: errors.New("nope")}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, FirstRunPath, nil)
	h.serveFirstRun(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("unexpected error must yield 503, got %d", rec.Code)
	}
}

func TestFirstRunHost_SetTimeout(t *testing.T) {
	h := NewFirstRunHost(nil)
	h.SetTimeout(0)
	if h.timeout != 1500*time.Millisecond {
		t.Fatalf("SetTimeout(0) must reset to 1500ms, got %v", h.timeout)
	}
	h.SetTimeout(500 * time.Millisecond)
	if h.timeout != 500*time.Millisecond {
		t.Fatalf("SetTimeout(500ms) must stick, got %v", h.timeout)
	}
}

func TestFirstRunHost_NilReceiverIsBenign(t *testing.T) {
	var h *FirstRunHost
	if _, err := h.Status(context.Background()); err == nil {
		t.Fatalf("nil host Status must error")
	}
}

// genericFirstRun implements FirstRunObserver without
// embedding firstrun.Store so the handler fallback path
// is exercised.
type genericFirstRun struct {
	state firstrun.State
	err   error
}

func (g *genericFirstRun) Status(_ context.Context) (firstrun.State, error) {
	if g.err != nil {
		return firstrun.State{}, g.err
	}
	return g.state, nil
}
