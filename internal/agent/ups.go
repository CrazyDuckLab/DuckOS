package agent

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/ups"
)

// UPSPath is the Agent IPC path for the UPS
// read-only surface. The path mirrors the
// other observers.
const UPSPath = "/agent/v1/ups"

// UPSProbePath is the Agent IPC path for an
// on-demand probe. The endpoint is RBAC-gated
// upstream by the API.
const UPSProbePath = "/agent/v1/ups/probe"

// UPSMaxBytes is the Agent IPC cap for UPS
// payloads. The cap matches the topology
// serialiser; the Agent never inflates it.
const UPSMaxBytes = 32 << 10

// UPSTimeout is the bound the Agent places on
// a single UPS round. The runner is in-process;
// the bound is a safety net for slow `upsc`
// invocations.
const UPSTimeout = 8 * time.Second

// UPSObserver is the Agent-facing read-only
// source of UPS data. The implementation owns
// the topology, the runtime state and the
// per-device health.
type UPSObserver interface {
	UPS(ctx context.Context) (ups.Status, error)
}

// UPSProber is the Agent-facing source of
// mutation for UPS. The Agent wires the
// production runner on Linux; non-Linux builds
// leave the prober nil and the IPC layer
// returns 503.
type UPSProber interface {
	Probe(ctx context.Context) error
}

// UPSObservable is the combined Agent-facing
// contract. The handler keeps a single
// reference so the read view and the probe
// view cannot diverge.
type UPSObservable interface {
	UPSObserver
	UPSProber
}

// ServeUPS is the Agent IPC handler for the
// read-only UPS surface. The handler is
// bounded, validated and concurrency capped.
func (h *Handler) ServeUPS(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		problem(w, 405, "method_not_allowed")
		return
	}
	if r.URL.RawQuery != "" || r.URL.ForceQuery || r.ContentLength != 0 || len(r.URL.Path) > len(UPSPath)+16 {
		problem(w, 400, "invalid_request")
		return
	}
	if !strings.HasSuffix(r.URL.Path, UPSPath) {
		problem(w, 404, "unknown_operation")
		return
	}
	select {
	case h.slots <- struct{}{}:
		defer func() { <-h.slots }()
	default:
		problem(w, 503, "agent_busy")
		return
	}
	host, ok := h.ups.(UPSObserver)
	if !ok || host == nil {
		problem(w, 503, "ups_unavailable")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), UPSTimeout)
	defer cancel()
	st, err := host.UPS(ctx)
	if err != nil {
		problem(w, 503, "ups_unavailable")
		return
	}
	if err := st.Topology.Valid(); err != nil && len(st.Topology.Devices) > 0 {
		problem(w, 503, "ups_invalid")
		return
	}
	data, err := json.Marshal(st)
	if err != nil || len(data) > UPSMaxBytes {
		problem(w, 503, "ups_limit")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// ServeUPSProbe is the Agent IPC handler for
// running an on-demand probe. The handler is
// concurrency capped and bounded.
func (h *Handler) ServeUPSProbe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		problem(w, 405, "method_not_allowed")
		return
	}
	if r.URL.RawQuery != "" || r.URL.ForceQuery || r.ContentLength != 0 {
		problem(w, 400, "invalid_request")
		return
	}
	if !strings.HasSuffix(r.URL.Path, UPSProbePath) {
		problem(w, 404, "unknown_operation")
		return
	}
	select {
	case h.slots <- struct{}{}:
		defer func() { <-h.slots }()
	default:
		problem(w, 503, "agent_busy")
		return
	}
	host, ok := h.ups.(UPSProber)
	if !ok || host == nil {
		problem(w, 503, "ups_unavailable")
		return
	}
	defer func() { _ = r.Body.Close() }()
	// the body is optional; an empty body
	// triggers a probe of every device.
	_, _ = io.ReadAll(io.LimitReader(r.Body, 1))
	ctx, cancel := context.WithTimeout(r.Context(), UPSTimeout)
	defer cancel()
	if err := host.Probe(ctx); err != nil {
		payload, _ := json.Marshal(map[string]any{"error": map[string]string{"code": "ups_probe_failed", "message": err.Error()}})
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write(payload)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
