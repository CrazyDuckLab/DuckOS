// G16 upgrade activation public bridge. The Agent owns
// the upgrade orchestrator; this host exposes a read-only
// snapshot of the current / staged / previous release so
// the API and CLI can render an honest activation status.
// Materialize and Activate remain Agent-only because they
// require systemd-level coordination.
package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

// UpgradePath is the IPC route for the upgrade status.
const UpgradePath = "/agent/v1/upgrade"

// UpgradeStagePath is the IPC route for staging a
// signed .duck package (DV07 production-path). POST
// only; body is upgradeStageRequest with the
// base64-encoded package payload.
const UpgradeStagePath = "/agent/v1/upgrade/stage"

// UpgradeActivatePath promotes the staged Candidate to
// Active. POST only.
const UpgradeActivatePath = "/agent/v1/upgrade/activate"

// UpgradeRollbackPath promotes Previous back to Active.
// POST only; body is {"reason": "..."}.
const UpgradeRollbackPath = "/agent/v1/upgrade/rollback"

// UpgradeObserver is the small surface the handler
// dispatches to. The *upgrade.Orchestrator or any
// equivalent implementation satisfies it.
type UpgradeObserver interface {
	// Status returns the canonical upgrade status snapshot.
	Status(ctx context.Context) (UpgradeStatus, error)
}

// UpgradeStatus is the public read-only projection of the
// orchestrator state. The struct intentionally omits
// payloads and signature material so it can be served
// over the loopback API without leaking the upgrade
// journal.
type UpgradeStatus struct {
	Version          int       `json:"version"`
	UpdatedAt        time.Time `json:"updatedAt"`
	CurrentRelease   string    `json:"currentRelease,omitempty"`
	CurrentVersion   string    `json:"currentVersion,omitempty"`
	CandidateRelease string    `json:"candidateRelease,omitempty"`
	CandidateVersion string    `json:"candidateVersion,omitempty"`
	PreviousRelease  string    `json:"previousRelease,omitempty"`
	PreviousVersion  string    `json:"previousVersion,omitempty"`
	Healthcheck      string    `json:"healthcheck,omitempty"`
	Healthy          bool      `json:"healthy"`
	Notes            []string  `json:"notes,omitempty"`
}

// UpgradeHost wraps an UpgradeObserver.
type UpgradeHost struct {
	UpgradeObserver
	timeout time.Duration
}

// NewUpgradeHost wraps the observer. A nil observer is
// replaced with a stub so the handler never panics on a
// missing dependency.
func NewUpgradeHost(obs UpgradeObserver) *UpgradeHost {
	if obs == nil {
		obs = stubUpgradeObserver{}
	}
	return &UpgradeHost{UpgradeObserver: obs, timeout: 1500 * time.Millisecond}
}

// SetTimeout overrides the per-request ceiling.
func (h *UpgradeHost) SetTimeout(d time.Duration) {
	if d <= 0 {
		d = 1500 * time.Millisecond
	}
	h.timeout = d
}

// ServeUpgrade renders the read-only status. The endpoint
// always returns 200; missing observer yields Healthy=false
// with a note so the UI can render "not configured" honestly.
func (h *UpgradeHost) ServeUpgrade(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		problem(w, 405, "method_not_allowed")
		return
	}
	if r.URL.ForceQuery || r.ContentLength != 0 || len(r.TransferEncoding) != 0 || r.URL.RawQuery != "" {
		problem(w, 400, "invalid_request")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()
	resp := UpgradeStatus{Version: 1, UpdatedAt: time.Now().UTC()}
	if h.UpgradeObserver != nil {
		status, err := h.Status(ctx)
		if err == nil {
			resp = status
			if resp.Version == 0 {
				resp.Version = 1
			}
			if resp.UpdatedAt.IsZero() {
				resp.UpdatedAt = time.Now().UTC()
			}
		} else {
			resp.Healthy = false
			resp.Notes = append(resp.Notes, "upgrade status unavailable: "+err.Error())
		}
	} else {
		resp.Healthy = false
		resp.Notes = append(resp.Notes, "upgrade observer not configured")
	}
	data, err := json.Marshal(resp)
	if err != nil || len(data) > upgradeMaxBytes {
		problem(w, 503, "upgrade_limit")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// upgradeMaxBytes caps the per-response payload.
const upgradeMaxBytes = 16 << 10

// stubUpgradeObserver is the safe fallback when the
// privileged orchestrator is not yet configured.
type stubUpgradeObserver struct{}

func (stubUpgradeObserver) Status(_ context.Context) (UpgradeStatus, error) {
	return UpgradeStatus{Version: 1, UpdatedAt: time.Now().UTC(), Healthy: false, Notes: []string{"upgrade not configured"}}, nil
}
