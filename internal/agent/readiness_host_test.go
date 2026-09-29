// G04 readiness IPC torture regression.
package agent

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/CrazyDuckLab/DuckOS/internal/chaos"
)

func newTestHost(t *testing.T) (*ReadinessHost, *chaos.ReadinessRecorder) {
	t.Helper()
	store := chaos.NewReadinessStore()
	rec := chaos.NewReadinessRecorder(store)
	return NewReadinessHost(rec), rec
}

func TestReadinessHost_SnapshotIsSorted(t *testing.T) {
	h, rec := newTestHost(t)
	rec.Register(chaos.ProbeFunc{Name: "zeta", Run: func(context.Context) (chaos.Readiness, string, error) {
		return chaos.ReadySupported, "z", nil
	}})
	rec.Register(chaos.ProbeFunc{Name: "alpha", Run: func(context.Context) (chaos.Readiness, string, error) {
		return chaos.ReadyExperimental, "a", nil
	}})
	if err := rec.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	rows, err := h.Snapshot(context.Background())
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if len(rows) != 2 || rows[0].Feature != "alpha" || rows[1].Feature != "zeta" {
		t.Fatalf("rows not sorted: %+v", rows)
	}
}

func TestReadinessHost_ServeReadiness_GET(t *testing.T) {
	h, rec := newTestHost(t)
	rec.Register(chaos.ProbeFunc{Name: "vpn.wireguard", Run: func(context.Context) (chaos.Readiness, string, error) {
		return chaos.ReadySupported, "kernel module present", nil
	}})
	if err := rec.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, ReadinessPath, nil)
	h.ServeReadiness(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("GET: expected 200, got %d body=%s", resp.Code, resp.Body.String())
	}
	if ct := resp.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("expected JSON content-type, got %q", ct)
	}
	var msg readinessResponse
	if err := json.Unmarshal(resp.Body.Bytes(), &msg); err != nil {
		t.Fatalf("decode: %v body=%s", err, resp.Body.String())
	}
	if msg.Version != 1 {
		t.Fatalf("version drift: %d", msg.Version)
	}
	if len(msg.Features) != 1 || msg.Features[0].Feature != "vpn.wireguard" {
		t.Fatalf("features drift: %+v", msg.Features)
	}
}

func TestReadinessHost_ServeReadiness_RejectsNonGET(t *testing.T) {
	h, _ := newTestHost(t)
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(method, ReadinessPath, strings.NewReader("{}"))
		req.Header.Set("Content-Type", "application/json")
		h.ServeReadiness(rec, req)
		if rec.Code/100 != 4 {
			t.Errorf("%s: expected 4xx, got %d", method, rec.Code)
		}
		if rec.Header().Get("Allow") != "GET" {
			t.Errorf("%s: missing Allow header", method)
		}
	}
}

func TestReadinessHost_ServeReadiness_RejectsQueryAndBody(t *testing.T) {
	h, _ := newTestHost(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, ReadinessPath+"?limit=1", nil)
	h.ServeReadiness(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("query must be rejected, got %d", rec.Code)
	}
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, ReadinessPath, strings.NewReader("not empty"))
	req2.ContentLength = int64(len("not empty"))
	h.ServeReadiness(rec2, req2)
	if rec2.Code != http.StatusBadRequest {
		t.Fatalf("body must be rejected, got %d", rec2.Code)
	}
}

func TestReadinessHost_ServeReadiness_OverSizeIsRejected(t *testing.T) {
	rec := chaos.NewReadinessRecorder(nil)
	rec.SetMaxNotes(readinessMaxBytes + 4096)
	host := &ReadinessHost{recorder: rec, timeout: 100}
	host.recorder.Register(chaos.ProbeFunc{Name: "huge", Run: func(context.Context) (chaos.Readiness, string, error) {
		return chaos.ReadySupported, strings.Repeat("x", readinessMaxBytes+1024), nil
	}})
	if err := host.recorder.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, ReadinessPath, nil)
	host.ServeReadiness(resp, req)
	if resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("oversized snapshot must yield 503, got %d body=%s", resp.Code, resp.Body.String())
	}
}

func TestReadinessHost_NilRecorder(t *testing.T) {
	var h *ReadinessHost
	if _, err := h.Snapshot(context.Background()); err != nil {
		t.Fatalf("nil host snapshot must be benign, got %v", err)
	}
	h = NewReadinessHost(nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, ReadinessPath, nil)
	h.ServeReadiness(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("nil recorder host must render empty 200, got %d", rec.Code)
	}
	body, _ := io.ReadAll(rec.Body)
	if !strings.Contains(string(body), `"features":[]`) {
		t.Fatalf("expected empty features, got %s", body)
	}
}

func TestReadinessHost_RecorderErrorsPropagate(t *testing.T) {
	store := chaos.NewReadinessStore()
	rec := chaos.NewReadinessRecorder(store)
	rec.Register(chaos.ProbeFunc{Name: "broken", Run: func(context.Context) (chaos.Readiness, string, error) {
		return "", "", errors.New("boom")
	}})
	if err := rec.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	host := NewReadinessHost(rec)
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, ReadinessPath, nil)
	host.ServeReadiness(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200 with degraded row, got %d", resp.Code)
	}
	var msg readinessResponse
	if err := json.Unmarshal(resp.Body.Bytes(), &msg); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(msg.Features) != 1 || msg.Features[0].State != chaos.ReadyDegraded {
		t.Fatalf("broken probe must be degraded, got %+v", msg.Features)
	}
}
