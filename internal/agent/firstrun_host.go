// G18 first-run persistence IPC surface. The Agent
// composition root owns the firstrun.Store; this host
// exposes a read-only status over IPC so the API and CLI
// can render an honest "first-run pending" badge. The
// privileged side is the only writer; the loopback API
// has no write endpoint for first-run state to keep the
// privileged surface explicit.
package agent

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/firstrun"
)

// FirstRunPath is the IPC route for first-run status.
const FirstRunPath = "/agent/v1/firstrun"

// FirstRunObserver is the small surface the handler
// dispatches to.
type FirstRunObserver interface {
	Status(ctx context.Context) (firstrun.State, error)
}

// FirstRunHost wraps a *firstrun.Store.
type FirstRunHost struct {
	store   *firstrun.Store
	timeout time.Duration
}

// NewFirstRunHost wraps store. A nil store is replaced
// with an in-memory stub so the handler never panics on
// a missing dependency.
func NewFirstRunHost(store *firstrun.Store) *FirstRunHost {
	if store == nil {
		store = firstrun.NewStore("")
	}
	return &FirstRunHost{store: store, timeout: 1500 * time.Millisecond}
}

// SetTimeout overrides the per-request ceiling.
func (h *FirstRunHost) SetTimeout(d time.Duration) {
	if d <= 0 {
		d = 1500 * time.Millisecond
	}
	h.timeout = d
}

// Status returns the current first-run state. A missing
// document yields ErrNotInitialised so the API can render
// a pending badge honestly.
func (h *FirstRunHost) Status(ctx context.Context) (firstrun.State, error) {
	if h == nil {
		return firstrun.State{}, errors.New("firstrun: host is nil")
	}
	return h.store.Load()
}

// ServeFirstRun renders the first-run status over IPC.
// GET only. The endpoint always returns 200 with an
// explicit pending/completed flag — first-run is a normal
// state, not an error.
func (h *FirstRunHost) ServeFirstRun(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.store == nil || h.store.Path() == "" {
		writePendingFirstRun(w)
		return
	}
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
	state, err := h.Status(ctx)
	resp := firstRunResponse{Version: firstrun.SchemaVersion, UpdatedAt: time.Now().UTC()}
	if err == nil {
		resp.Completed = state.Completed()
		resp.State = &state
	} else if errors.Is(err, firstrun.ErrNotInitialised) {
		resp.Pending = true
	} else {
		problem(w, 503, "firstrun_unavailable")
		return
	}
	data, err := json.Marshal(resp)
	if err != nil || len(data) > firstRunMaxBytes {
		problem(w, 503, "firstrun_limit")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// writePendingFirstRun renders a 200 with Pending=true so
// the IPC channel is observable even when the privileged
// Agent is not yet configured (e.g. a fresh install before
// the store path is provisioned).
func writePendingFirstRun(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"version":1,"pending":true}`))
}

// firstRunMaxBytes caps the per-response payload.
const firstRunMaxBytes = 16 << 10

// firstRunResponse is the public shape over IPC.
type firstRunResponse struct {
	Version   int             `json:"version"`
	UpdatedAt time.Time       `json:"updatedAt"`
	Pending   bool            `json:"pending,omitempty"`
	Completed bool            `json:"completed,omitempty"`
	State     *firstrun.State `json:"state,omitempty"`
}
