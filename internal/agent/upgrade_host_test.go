// G16 upgrade activation IPC torture regression.
package agent

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type fakeUpgradeObs struct {
	out UpgradeStatus
	err error
}

func (f *fakeUpgradeObs) Status(_ context.Context) (UpgradeStatus, error) {
	if f.err != nil {
		return UpgradeStatus{}, f.err
	}
	return f.out, nil
}

func TestUpgradeHost_GET_ReturnsStatus(t *testing.T) {
	obs := &fakeUpgradeObs{out: UpgradeStatus{
		Version:        1,
		CurrentRelease: "duckos-1.0",
		CurrentVersion: "1.0.0",
		Healthy:        true,
	}}
	h := NewUpgradeHost(obs)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, UpgradePath, nil)
	h.ServeUpgrade(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var msg UpgradeStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &msg); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if msg.CurrentRelease != "duckos-1.0" {
		t.Fatalf("drift: %+v", msg)
	}
}

func TestUpgradeHost_ObserverErrorYieldsHealthyFalse(t *testing.T) {
	obs := &fakeUpgradeObs{err: errors.New("boom")}
	h := NewUpgradeHost(obs)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, UpgradePath, nil)
	h.ServeUpgrade(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var msg UpgradeStatus
	_ = json.Unmarshal(rec.Body.Bytes(), &msg)
	if msg.Healthy {
		t.Fatalf("error must yield Healthy=false")
	}
	if len(msg.Notes) == 0 {
		t.Fatalf("error must surface a note")
	}
}

func TestUpgradeHost_NilObserverYieldsStub(t *testing.T) {
	h := NewUpgradeHost(nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, UpgradePath, nil)
	h.ServeUpgrade(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"healthy":false`) {
		t.Fatalf("nil observer must yield healthy=false, got %s", rec.Body.String())
	}
}

func TestUpgradeHost_RejectsNonGET(t *testing.T) {
	h := NewUpgradeHost(nil)
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(method, UpgradePath, strings.NewReader("{}"))
		req.Header.Set("Content-Type", "application/json")
		h.ServeUpgrade(rec, req)
		if rec.Code/100 != 4 {
			t.Errorf("%s: expected 4xx, got %d", method, rec.Code)
		}
	}
}

func TestUpgradeHost_RejectsQueryAndBody(t *testing.T) {
	h := NewUpgradeHost(nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, UpgradePath+"?foo=bar", nil)
	h.ServeUpgrade(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("query must be rejected, got %d", rec.Code)
	}
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, UpgradePath, strings.NewReader("not empty"))
	req2.ContentLength = int64(len("not empty"))
	h.ServeUpgrade(rec2, req2)
	if rec2.Code != http.StatusBadRequest {
		t.Fatalf("body must be rejected, got %d", rec2.Code)
	}
}

func TestUpgradeHost_GenericObserverFallback(t *testing.T) {
	obs := &fakeUpgradeObs{out: UpgradeStatus{Version: 1, Healthy: true}}
	h := &Handler{upgrade: obs}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, UpgradePath, nil)
	h.serveUpgrade(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"healthy":true`) {
		t.Fatalf("fallback must surface status, got %s", rec.Body.String())
	}
}

func TestUpgradeHost_NoObserverFallsBackToStub(t *testing.T) {
	h := &Handler{}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, UpgradePath, nil)
	h.serveUpgrade(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("missing observer must yield 200 via stub, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"healthy":false`) {
		t.Fatalf("stub must yield healthy=false, got %s", rec.Body.String())
	}
}

func TestUpgradeHost_SetTimeout(t *testing.T) {
	h := NewUpgradeHost(nil)
	h.SetTimeout(0)
	if h.timeout != 1500*time.Millisecond {
		t.Fatalf("default timeout: %v", h.timeout)
	}
}
