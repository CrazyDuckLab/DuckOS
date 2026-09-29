// G04 readiness IPC surface (agent write side). The host
// wraps a *chaos.ReadinessRecorder and exposes the recorded
// feature readiness rows over the Agent IPC boundary so the
// Engine and the API can render an honest availability
// matrix without ever inventing support from package
// presence.
package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/chaos"
)

// ReadinessPath is the IPC route for the readiness manifest.
const ReadinessPath = "/agent/v1/readiness"

// ReadinessObserver is the small surface the handler
// consumes. The Recorder implements it directly.
type ReadinessObserver interface {
	// Snapshot returns the canonical readiness rows for
	// the caller. Rows are sorted by feature name so the
	// output is deterministic across calls.
	Snapshot(ctx context.Context) ([]chaos.FeatureReadiness, error)
}

// ReadinessHost wires the recorder into the Agent IPC
// surface. The host never mutates the recorder.
type ReadinessHost struct {
	recorder *chaos.ReadinessRecorder
	timeout  time.Duration
}

// NewReadinessHost wraps an existing recorder.
func NewReadinessHost(rec *chaos.ReadinessRecorder) *ReadinessHost {
	if rec == nil {
		rec = chaos.NewReadinessRecorder(nil)
	}
	return &ReadinessHost{recorder: rec, timeout: 1500 * time.Millisecond}
}

// SetTimeout overrides the per-snapshot ceiling. A non-positive
// value resets to 1500 ms.
func (h *ReadinessHost) SetTimeout(d time.Duration) {
	if d <= 0 {
		d = 1500 * time.Millisecond
	}
	h.timeout = d
}

// Recorder exposes the underlying recorder for the agent
// composition root to register probes against.
func (h *ReadinessHost) Recorder() *chaos.ReadinessRecorder { return h.recorder }

// Snapshot returns the readiness rows recorded by the
// recorder. Rows are returned in canonical feature order.
func (h *ReadinessHost) Snapshot(ctx context.Context) ([]chaos.FeatureReadiness, error) {
	if h == nil || h.recorder == nil {
		return nil, nil
	}
	return h.recorder.Store().All(), nil
}

// ServeReadiness renders the snapshot over IPC. The body
// is bounded to model.MaxResponseBytes (enforced by the
// shared encoder).
func (h *ReadinessHost) ServeReadiness(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		problem(w, 405, "method_not_allowed")
		return
	}
	if r.URL.RawQuery != "" || r.URL.ForceQuery || r.ContentLength != 0 || len(r.TransferEncoding) != 0 {
		problem(w, 400, "invalid_request")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()
	rows, err := h.Snapshot(ctx)
	if err != nil {
		problem(w, 503, "readiness_unavailable")
		return
	}
	resp := readinessResponse{
		Version:   1,
		UpdatedAt: time.Now().UTC(),
		Features:  rows,
	}
	data, err := json.Marshal(resp)
	if err != nil || len(data) > readinessMaxBytes {
		problem(w, 503, "readiness_limit")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// readinessMaxBytes is the per-response cap. The snapshot
// is small (one entry per feature) but the guard prevents
// a future expansion from accidentally inflating the IPC
// payload.
const readinessMaxBytes = 64 * 1024

// readinessResponse is the public shape over IPC.
type readinessResponse struct {
	Version   int                      `json:"version"`
	UpdatedAt time.Time                `json:"updatedAt"`
	Features  []chaos.FeatureReadiness `json:"features"`
}
