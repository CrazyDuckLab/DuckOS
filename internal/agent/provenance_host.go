// G08 dependency / ownership / provenance IPC surface.
// The Agent composition root holds the canonical
// lifecycle.Graph; this host exposes read-only
// ownership and provenance views over IPC so the API,
// CLI and UI share one definition of "who owns this and
// how did it get here".
package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/lifecycle"
)

// ProvenancePath is the IPC route for ownership/provenance
// queries. The endpoint accepts ?kind=...&id=... and
// returns the combined view.
const ProvenancePath = "/agent/v1/provenance"

// ProvenanceObserver is the small surface the Agent
// handler dispatches to. The lifecycle.Graph implements
// it directly.
type ProvenanceObserver interface {
	Ownership(obj lifecycle.Object) (lifecycle.OwnershipRecord, bool)
	Provenance(obj lifecycle.Object) []lifecycle.ProvenanceEvent
}

// ProvenanceHost wraps a lifecycle.Graph and exposes the
// read-only IPC surface.
type ProvenanceHost struct {
	*lifecycle.Graph
	timeout time.Duration
}

// NewProvenanceHost wraps the graph.
func NewProvenanceHost(g *lifecycle.Graph) *ProvenanceHost {
	if g == nil {
		g = lifecycle.NewGraph()
	}
	return &ProvenanceHost{Graph: g, timeout: 1500 * time.Millisecond}
}

// SetTimeout overrides the per-request ceiling.
func (h *ProvenanceHost) SetTimeout(d time.Duration) {
	if d <= 0 {
		d = 1500 * time.Millisecond
	}
	h.timeout = d
}

// ServeProvenance renders the combined view over IPC.
// Query: ?kind=...&id=... Both parameters are required
// when one is supplied; missing parameters return an
// empty envelope so callers can probe the IPC channel
// without surfacing a 400.
func (h *ProvenanceHost) ServeProvenance(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.Graph == nil {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"version":1,"updatedAt":"0001-01-01T00:00:00Z"}`))
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
	kind := r.URL.Query().Get("kind")
	id := r.URL.Query().Get("id")
	if (kind == "") != (id == "") {
		problem(w, 400, "invalid_request")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()
	resp := h.render(ctx, kind, id)
	data, err := json.Marshal(resp)
	if err != nil || len(data) > provenanceMaxBytes {
		problem(w, 503, "provenance_limit")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// render builds the response envelope. The function is
// exported through ServeProvenance only; tests may call it
// directly with a synthetic context.
func (h *ProvenanceHost) render(_ context.Context, kind, id string) provenanceResponse {
	resp := provenanceResponse{Version: 1, UpdatedAt: time.Now().UTC()}
	if h == nil || h.Graph == nil {
		return resp
	}
	if kind == "" || id == "" {
		return resp
	}
	obj := lifecycle.Object{Kind: lifecycle.ObjectKind(kind), ID: id}
	if !obj.Valid() {
		return resp
	}
	if rec, ok := h.Ownership(obj); ok {
		resp.Ownership = &rec
	}
	resp.History = h.Provenance(obj)
	return resp
}

// provenanceMaxBytes caps the per-response size.
const provenanceMaxBytes = 64 * 1024

type provenanceResponse struct {
	Version   int                         `json:"version"`
	UpdatedAt time.Time                   `json:"updatedAt"`
	Ownership *lifecycle.OwnershipRecord  `json:"ownership,omitempty"`
	History   []lifecycle.ProvenanceEvent `json:"history,omitempty"`
}

var idQueryPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:-]{0,63}$`)

// validateQueryID is a small defensive guard so the IPC
// surface refuses absurd query strings without dispatching
// to the lifecycle graph.
func validateQueryID(s string) bool {
	return idQueryPattern.MatchString(strings.TrimSpace(s))
}
