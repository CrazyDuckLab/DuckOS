package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/sites"
)

// SitesPath is the Agent IPC path for the
// sites read-only surface.
const SitesPath = "/agent/v1/sites"

// SitesMaxBytes is the Agent IPC cap for sites
// payloads.
const SitesMaxBytes = 32 << 10

// SitesTimeout is the bound the Agent places on
// a single sites round.
const SitesTimeout = 4 * time.Second

// SitesObserver is the Agent-facing read-only
// source of sites data.
type SitesObserver interface {
	Sites(ctx context.Context) (sites.Status, error)
}

// SitesObservable is the alias used by the
// Handler; the package exposes a single
// Agent-facing contract.
type SitesObservable = SitesObserver

// ServeSites is the Agent IPC handler for the
// sites read-only surface.
func (h *Handler) ServeSites(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		problem(w, 405, "method_not_allowed")
		return
	}
	if r.URL.RawQuery != "" || r.URL.ForceQuery || r.ContentLength != 0 || len(r.URL.Path) > len(SitesPath)+16 {
		problem(w, 400, "invalid_request")
		return
	}
	if !strings.HasSuffix(r.URL.Path, SitesPath) {
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
	host, ok := h.sitesHost.(SitesObserver)
	if !ok || host == nil {
		problem(w, 503, "sites_unavailable")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), SitesTimeout)
	defer cancel()
	st, err := host.Sites(ctx)
	if err != nil {
		problem(w, 503, "sites_unavailable")
		return
	}
	if err := st.Topology.Valid(); err != nil && len(st.Topology.Sites) > 0 {
		problem(w, 503, "sites_invalid")
		return
	}
	data, err := json.Marshal(st)
	if err != nil || len(data) > SitesMaxBytes {
		problem(w, 503, "sites_limit")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
