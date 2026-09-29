package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/topology"
)

const TopologyPath = "/agent/v1/topology"

// TopologyMaxBytes is the Agent IPC cap for Topology payloads.
// The Engine uses the same cap; both layers must agree.
const TopologyMaxBytes = 80 << 10

// TopologyTimeout is the bound the Agent places on a single
// Topology build. A topology is intentionally cheap: it is
// built from in-memory state, not from network round trips.
const TopologyTimeout = 4 * time.Second

// TopologyObserver is the Agent-facing source of the relationship
// graph. The implementation is the TopologyHost; tests can
// substitute a fake.
type TopologyObserver interface {
	Topology(context.Context) (topology.Graph, error)
}

// ServeTopology is the Agent IPC handler for the Topology surface.
func (h *Handler) ServeTopology(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		problem(w, 405, "method_not_allowed")
		return
	}
	if r.URL.RawQuery != "" || r.URL.ForceQuery || r.ContentLength != 0 || len(r.URL.Path) > len(TopologyPath)+16 {
		problem(w, 400, "invalid_request")
		return
	}
	if !strings.HasSuffix(r.URL.Path, TopologyPath) {
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
	host := h.topology
	if host == nil {
		problem(w, 503, "topology_unavailable")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), TopologyTimeout)
	defer cancel()
	graph, err := host.Topology(ctx)
	if err != nil {
		problem(w, 503, "topology_unavailable")
		return
	}
	if err := graph.Valid(); err != nil {
		problem(w, 503, "topology_invalid")
		return
	}
	data, err := json.Marshal(graph)
	if err != nil || len(data) > TopologyMaxBytes {
		problem(w, 503, "topology_limit")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
