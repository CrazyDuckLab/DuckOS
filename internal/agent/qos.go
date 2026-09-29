package agent

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/qos"
)

// QoSPath is the Agent IPC path for QoS. The
// path mirrors the read-only pattern used by the
// other observers; mutation goes through the
// /agent/v1/qos/apply|rollback endpoints.
const QoSPath = "/agent/v1/qos"

// QoSApplyPath is the Agent IPC path for applying
// a QoS plan. The apply endpoint is RBAC-gated
// upstream by the API; the Agent only enforces the
// bounded contract.
const QoSApplyPath = "/agent/v1/qos/apply"

// QoSRollbackPath is the Agent IPC path for rolling
// the live QoS controller back to the previous
// plan. A rollback without a previous plan returns
// a structured error.
const QoSRollbackPath = "/agent/v1/qos/rollback"

// QoSMaxBytes is the Agent IPC cap for QoS payloads.
// 32 KiB is plenty for a small topology; the
// renderer rejects larger plans at the model layer.
const QoSMaxBytes = 32 << 10

// QoSTimeout is the bound the Agent places on a
// single QoS round. The renderer is in-process; the
// bound is a safety net for the future Linux
// adapter and a hard cap on slow callers.
const QoSTimeout = 6 * time.Second

// QoSObserver is the Agent-facing read-only source
// of QoS data. The implementation owns the
// topology, the controller, the previous plan and
// the live state. The reference implementation is
// the QoSHost; tests can substitute a fake.
type QoSObserver interface {
	QoS(ctx context.Context) (qos.Status, error)
}

// QoSController is the Agent-facing source of
// mutation for QoS. The Agent wires the production
// controller on Linux and a "not-implemented"
// controller elsewhere. The interface is the small
// one the API needs; it does not expose the live
// Runner.
type QoSController interface {
	Apply(ctx context.Context, p qos.Plan) (qos.ApplyOutcome, error)
	Rollback(ctx context.Context) (qos.ApplyOutcome, error)
	PreviewState(ctx context.Context) (qos.RunnerState, error)
}

// QoSObservable is the combined Agent-facing
// contract. A single host object owns the
// read-only state, the apply surface and the
// rollback surface; the IPC paths share the same
// reference so the read view and the mutation
// view cannot diverge.
type QoSObservable interface {
	QoSObserver
	QoSController
}

// ServeQoS is the Agent IPC handler for the
// read-only QoS surface. The handler is bounded,
// validated and concurrency capped.
func (h *Handler) ServeQoS(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		problem(w, 405, "method_not_allowed")
		return
	}
	if r.URL.RawQuery != "" || r.URL.ForceQuery || r.ContentLength != 0 || len(r.URL.Path) > len(QoSPath)+16 {
		problem(w, 400, "invalid_request")
		return
	}
	if !strings.HasSuffix(r.URL.Path, QoSPath) {
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
	host, ok := h.qos.(QoSObserver)
	if !ok || host == nil {
		problem(w, 503, "qos_unavailable")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), QoSTimeout)
	defer cancel()
	st, err := host.QoS(ctx)
	if err != nil {
		problem(w, 503, "qos_unavailable")
		return
	}
	if err := st.Topology.Valid(); err != nil && len(st.Topology.Shapers) > 0 {
		problem(w, 503, "qos_invalid")
		return
	}
	data, err := json.Marshal(st)
	if err != nil || len(data) > QoSMaxBytes {
		problem(w, 503, "qos_limit")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// ServeQoSApply is the Agent IPC handler for
// applying a plan. The handler is bounded,
// validated and concurrency capped. The plan body
// is read with a hard cap so a hostile client
// cannot OOM the Agent.
func (h *Handler) ServeQoSApply(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		problem(w, 405, "method_not_allowed")
		return
	}
	if r.URL.RawQuery != "" || r.URL.ForceQuery || r.ContentLength < 0 {
		problem(w, 400, "invalid_request")
		return
	}
	if !strings.HasSuffix(r.URL.Path, QoSApplyPath) {
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
	host, ok := h.qos.(QoSController)
	if !ok || host == nil {
		problem(w, 503, "qos_unavailable")
		return
	}
	defer func() { _ = r.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(r.Body, QoSMaxBytes+1))
	if err != nil {
		problem(w, 400, "invalid_body")
		return
	}
	if len(data) > QoSMaxBytes {
		problem(w, 413, "payload_too_large")
		return
	}
	var p qos.Plan
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&p); err != nil {
		problem(w, 400, "invalid_plan")
		return
	}
	if err := p.Valid(); err != nil {
		problem(w, 400, "invalid_plan")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), QoSTimeout)
	defer cancel()
	out, err := host.Apply(ctx, p)
	if err != nil {
		// The agent never crashes a caller; a failed
		// apply returns a structured response and a
		// 503 status so the API can show the
		// outcome. The body always includes the
		// ApplyOutcome the Runner returned.
		payload, _ := json.Marshal(map[string]any{"error": map[string]string{"code": "qos_apply_failed", "message": err.Error()}, "outcome": out})
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write(payload)
		return
	}
	payload, err := json.Marshal(out)
	if err != nil || len(payload) > QoSMaxBytes {
		problem(w, 503, "qos_limit")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

// ServeQoSRollback is the Agent IPC handler for
// rolling back the active plan. The handler is
// bounded and concurrency capped; the controller
// itself refuses to roll back without a previous
// plan.
func (h *Handler) ServeQoSRollback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		problem(w, 405, "method_not_allowed")
		return
	}
	if r.URL.RawQuery != "" || r.URL.ForceQuery || r.ContentLength != 0 {
		problem(w, 400, "invalid_request")
		return
	}
	if !strings.HasSuffix(r.URL.Path, QoSRollbackPath) {
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
	host, ok := h.qos.(QoSController)
	if !ok || host == nil {
		problem(w, 503, "qos_unavailable")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), QoSTimeout)
	defer cancel()
	out, err := host.Rollback(ctx)
	if err != nil {
		if errors.Is(err, qos.ErrUnavailable) {
			payload, _ := json.Marshal(map[string]any{"error": map[string]string{"code": "qos_unavailable", "message": err.Error()}, "outcome": out})
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Cache-Control", "no-store")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write(payload)
			return
		}
		payload, _ := json.Marshal(map[string]any{"error": map[string]string{"code": "qos_rollback_failed", "message": err.Error()}, "outcome": out})
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write(payload)
		return
	}
	payload, err := json.Marshal(out)
	if err != nil || len(payload) > QoSMaxBytes {
		problem(w, 503, "qos_limit")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}
