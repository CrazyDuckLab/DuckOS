// G17 portable / selective restore bridge. The Agent
// owns the backup catalog; this host exposes a read-only
// view over IPC so the API and CLI can render an honest
// "available backups" + "restore plan" surface. Actual
// restore execution remains Agent-privileged because it
// requires systemd-level coordination.
package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

// BackupPath is the IPC route for backup list / plan.
const BackupPath = "/agent/v1/backup"

// BackupObserver is the small surface the handler
// dispatches to.
type BackupObserver interface {
	List(ctx context.Context) ([]BackupSummary, error)
	Plan(ctx context.Context, backupID string) (RestorePlanSummary, error)
}

// BackupSummary is one row of the backup catalog.
type BackupSummary struct {
	BackupID       string    `json:"backupId"`
	Mode           string    `json:"mode"`
	CreatedAt      time.Time `json:"createdAt"`
	SizeBytes      int64     `json:"sizeBytes"`
	ComponentCount int       `json:"componentCount"`
	Portable       bool      `json:"portable"`
	SecretGated    bool      `json:"secretGated"`
	Hostname       string    `json:"hostname,omitempty"`
	Note           string    `json:"note,omitempty"`
}

// RestorePlanSummary is the public projection of the
// backup package's RestorePlan. The struct is intentionally
// read-only; the underlying PlanRestore already enforces
// the secret / conflict gates.
type RestorePlanSummary struct {
	BackupID         string    `json:"backupId"`
	Safe             bool      `json:"safe"`
	RequiresSecret   bool      `json:"requiresSecret"`
	Conflicts        []string  `json:"conflicts,omitempty"`
	Missing          []string  `json:"missing,omitempty"`
	Components       []string  `json:"components,omitempty"`
	EstimatedSeconds int       `json:"estimatedSeconds"`
	GeneratedAt      time.Time `json:"generatedAt"`
}

// BackupHost wraps a BackupObserver.
type BackupHost struct {
	BackupObserver
	timeout time.Duration
}

// NewBackupHost wraps the observer. A nil observer is
// replaced with a stub so the handler never panics on a
// missing dependency.
func NewBackupHost(obs BackupObserver) *BackupHost {
	if obs == nil {
		obs = stubBackupObserver{}
	}
	return &BackupHost{BackupObserver: obs, timeout: 1500 * time.Millisecond}
}

// SetTimeout overrides the per-request ceiling.
func (h *BackupHost) SetTimeout(d time.Duration) {
	if d <= 0 {
		d = 1500 * time.Millisecond
	}
	h.timeout = d
}

// ServeBackup renders the backup list when ?id= is
// absent, or the restore plan for the supplied id.
func (h *BackupHost) ServeBackup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		problem(w, 405, "method_not_allowed")
		return
	}
	if r.URL.ForceQuery || r.ContentLength != 0 || len(r.TransferEncoding) != 0 {
		problem(w, 400, "invalid_request")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()
	id := r.URL.Query().Get("id")
	if id == "" {
		backups, err := h.List(ctx)
		if err != nil {
			problem(w, 503, "backup_unavailable")
			return
		}
		resp := backupListResponse{Version: 1, UpdatedAt: time.Now().UTC(), Backups: backups}
		writeJSONOrFail(w, resp, backupMaxBytes, "backup_limit")
		return
	}
	plan, err := h.Plan(ctx, id)
	if err != nil {
		problem(w, 503, "backup_plan_unavailable")
		return
	}
	resp := backupPlanResponse{Version: 1, UpdatedAt: time.Now().UTC(), Plan: plan}
	writeJSONOrFail(w, resp, backupMaxBytes, "backup_limit")
}

func writeJSONOrFail(w http.ResponseWriter, payload any, maxBytes int, errCode string) {
	data, err := json.Marshal(payload)
	if err != nil || len(data) > maxBytes {
		problem(w, 503, errCode)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// backupMaxBytes caps the per-response payload.
const backupMaxBytes = 64 * 1024

type backupListResponse struct {
	Version   int             `json:"version"`
	UpdatedAt time.Time       `json:"updatedAt"`
	Backups   []BackupSummary `json:"backups"`
}

type backupPlanResponse struct {
	Version   int                `json:"version"`
	UpdatedAt time.Time          `json:"updatedAt"`
	Plan      RestorePlanSummary `json:"plan"`
}

// stubBackupObserver is the safe fallback when the
// privileged catalog is not yet configured.
type stubBackupObserver struct{}

func (stubBackupObserver) List(_ context.Context) ([]BackupSummary, error) {
	return []BackupSummary{}, nil
}

func (stubBackupObserver) Plan(_ context.Context, _ string) (RestorePlanSummary, error) {
	return RestorePlanSummary{GeneratedAt: time.Now().UTC(), Safe: false, Conflicts: []string{"backup catalog not configured"}}, nil
}
