package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/multiwan"
)

const MultiWANPath = "/agent/v1/multiwan"

// MultiWANMaxBytes is the Agent IPC cap for Multi-WAN payloads.
const MultiWANMaxBytes = 80 << 10

// MultiWANTimeout is the bound the Agent places on a single
// MultiWAN call. A MultiWAN round is in-process and never
// performs I/O beyond the agent itself; the bound is a
// safety net for future plug-ins.
const MultiWANTimeout = 4 * time.Second

// MultiWANObserver is the Agent-facing source of multi-WAN
// data. The implementation owns the topology, the probe
// results and the per-group selection. The reference
// implementation is the MultiWANHost; tests can substitute
// a fake.
type MultiWANObserver interface {
	MultiWAN(ctx context.Context) (multiwan.Topology, map[string]multiwan.Selection, map[string]string, error)
}

// ServeMultiWAN is the Agent IPC handler for the MultiWAN
// surface. The handler is bounded, validated and concurrency
// capped: a single slow Observer cannot poison the rest of
// the response, and the Agent publishes the payload only
// after the implementation has confirmed the contract.
func (h *Handler) ServeMultiWAN(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		problem(w, 405, "method_not_allowed")
		return
	}
	if r.URL.RawQuery != "" || r.URL.ForceQuery || r.ContentLength != 0 || len(r.URL.Path) > len(MultiWANPath)+16 {
		problem(w, 400, "invalid_request")
		return
	}
	if !strings.HasSuffix(r.URL.Path, MultiWANPath) {
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
	host, ok := h.multiwan.(MultiWANObserver)
	if !ok || host == nil {
		problem(w, 503, "multiwan_unavailable")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), MultiWANTimeout)
	defer cancel()
	t, sel, health, err := host.MultiWAN(ctx)
	if err != nil {
		problem(w, 503, "multiwan_unavailable")
		return
	}
	if err := t.Valid(); err != nil && len(t.Groups) > 0 {
		problem(w, 503, "multiwan_invalid")
		return
	}
	payload := map[string]any{
		"topology":   t,
		"selections": sel,
		"health":     health,
	}
	data, err := json.Marshal(payload)
	if err != nil || len(data) > MultiWANMaxBytes {
		problem(w, 503, "multiwan_limit")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
