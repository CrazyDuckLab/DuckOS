// G19 contextual diagnostic job IPC surface. The Agent
// composition root owns a *diagjob.Store; this host
// exposes a read-only list/get view so the API and CLI
// can render a compact "recent diagnostics" timeline.
// Writes are privileged: the API has no create/update
// endpoint for jobs because the actual diagnostic runs
// are scheduled by the Agent on operator command.
package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/diagjob"
)

// DiagJobPath is the IPC route for the diagnostic job
// list / detail.
const DiagJobPath = "/agent/v1/diagjobs"

// DiagJobObserver is the small surface the handler
// dispatches to. The *diagjob.Store satisfies it.
type DiagJobObserver interface {
	List() []diagjob.Job
	Get(id string) (diagjob.Job, bool)
}

// DiagJobHost wraps the store.
type DiagJobHost struct {
	*diagjob.Store
	timeout time.Duration
}

// NewDiagJobHost wraps the store.
func NewDiagJobHost(s *diagjob.Store) *DiagJobHost {
	if s == nil {
		s = diagjob.NewStore()
	}
	return &DiagJobHost{Store: s, timeout: 1500 * time.Millisecond}
}

// SetTimeout overrides the per-request ceiling.
func (h *DiagJobHost) SetTimeout(d time.Duration) {
	if d <= 0 {
		d = 1500 * time.Millisecond
	}
	h.timeout = d
}

// ServeDiagJobs renders the bounded list. Optional ?id=
// selects a single job; missing ?id= returns every job.
func (h *DiagJobHost) ServeDiagJobs(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.Store == nil {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"version":1,"jobs":[]}`))
		return
	}
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		problem(w, 405, "method_not_allowed")
		return
	}
	if r.URL.ForceQuery || r.ContentLength != 0 || len(r.TransferEncoding) != 0 {
		problem(w, 400, "invalid_request")
		return
	}
	_ = r // context is unused; the store is in-memory.
	resp := diagJobsResponse{Version: 1, UpdatedAt: time.Now().UTC()}
	if id := r.URL.Query().Get("id"); id != "" {
		if j, ok := h.Get(id); ok {
			resp.Jobs = []diagjob.Job{j}
		}
	} else {
		resp.Jobs = h.List()
	}
	data, err := json.Marshal(resp)
	if err != nil || len(data) > diagJobsMaxBytes {
		problem(w, 503, "diagjobs_limit")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// diagJobsMaxBytes caps the per-response payload.
const diagJobsMaxBytes = 256 << 10

// diagJobsResponse is the public IPC envelope.
type diagJobsResponse struct {
	Version   int           `json:"version"`
	UpdatedAt time.Time     `json:"updatedAt"`
	Jobs      []diagjob.Job `json:"jobs"`
}

// silence unused-import warning for context when no helper uses it.
var _ = context.Background
