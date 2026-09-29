// G17 backup IPC torture regression.
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

type fakeBackupObs struct {
	list []BackupSummary
	plan RestorePlanSummary
	err  error
}

func (f *fakeBackupObs) List(_ context.Context) ([]BackupSummary, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.list, nil
}

func (f *fakeBackupObs) Plan(_ context.Context, _ string) (RestorePlanSummary, error) {
	if f.err != nil {
		return RestorePlanSummary{}, f.err
	}
	return f.plan, nil
}

func TestBackupHost_List(t *testing.T) {
	obs := &fakeBackupObs{list: []BackupSummary{
		{BackupID: "b1", Mode: "full-protected", Portable: true, ComponentCount: 5, SizeBytes: 1024, CreatedAt: time.Now().UTC()},
	}}
	h := NewBackupHost(obs)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, BackupPath, nil)
	h.ServeBackup(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var msg backupListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &msg); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(msg.Backups) != 1 || msg.Backups[0].BackupID != "b1" {
		t.Fatalf("drift: %+v", msg.Backups)
	}
}

func TestBackupHost_Plan(t *testing.T) {
	obs := &fakeBackupObs{plan: RestorePlanSummary{
		BackupID:         "b1",
		Safe:             true,
		RequiresSecret:   false,
		Components:       []string{"network.firewall", "dhcp.subnet"},
		EstimatedSeconds: 30,
		GeneratedAt:      time.Now().UTC(),
	}}
	h := NewBackupHost(obs)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, BackupPath+"?id=b1", nil)
	h.ServeBackup(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var msg backupPlanResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &msg)
	if msg.Plan.BackupID != "b1" || !msg.Plan.Safe {
		t.Fatalf("drift: %+v", msg.Plan)
	}
}

func TestBackupHost_RejectsNonGET(t *testing.T) {
	h := NewBackupHost(nil)
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(method, BackupPath, strings.NewReader("{}"))
		req.Header.Set("Content-Type", "application/json")
		h.ServeBackup(rec, req)
		if rec.Code/100 != 4 {
			t.Errorf("%s: expected 4xx, got %d", method, rec.Code)
		}
	}
}

func TestBackupHost_RejectsBody(t *testing.T) {
	h := NewBackupHost(nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, BackupPath, strings.NewReader("not empty"))
	req.ContentLength = int64(len("not empty"))
	h.ServeBackup(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("body must be rejected, got %d", rec.Code)
	}
}

func TestBackupHost_ObserverErrorYields503(t *testing.T) {
	obs := &fakeBackupObs{err: errors.New("boom")}
	h := NewBackupHost(obs)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, BackupPath, nil)
	h.ServeBackup(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
}

func TestBackupHost_NilObserverYieldsEmptyList(t *testing.T) {
	h := NewBackupHost(nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, BackupPath, nil)
	h.ServeBackup(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"backups":[]`) {
		t.Fatalf("nil observer must yield empty list, got %s", rec.Body.String())
	}
}

func TestBackupHost_NoObserverFallsBackToStub(t *testing.T) {
	h := &Handler{}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, BackupPath, nil)
	h.serveBackup(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("missing observer must yield 200 via stub, got %d", rec.Code)
	}
}
