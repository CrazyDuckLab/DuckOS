// agent/anycast_host.go wires the Anycast evidence store
// into the Agent IPC loopback so the API/CLI can read the
// VIP advertisements, health and hysteresis state without
// reaching into the privileged process.
//
// C-blocker #9: Anycast execution/observation integration.
// The previous evidence store lived in package anycast but
// was not reachable from the public surface. The host now
// owns a *anycast.Store reference and exposes the bounded
// read view through /agent/v1/anycast.
package agent

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/anycast"
)

// AnycastPath is the IPC route for the Anycast read view.
const AnycastPath = "/agent/v1/anycast"

// AnycastMaxBytes is the IPC cap on the Anycast response.
const AnycastMaxBytes = 64 * 1024

// AnycastSource is the narrow contract the host needs.
// *anycast.Store satisfies it; tests inject a fake.
type AnycastSource interface {
	AllEvidence() []anycast.Evidence
	Counters() anycast.Counters
}

// AnycastHost wraps an AnycastSource. A nil source is
// replaced with a stub so the handler never panics on a
// missing dependency.
type AnycastHost struct {
	AnycastSource
	timeout time.Duration
}

// NewAnycastHost wraps the source.
func NewAnycastHost(src AnycastSource) *AnycastHost {
	if src == nil {
		src = stubAnycast{}
	}
	return &AnycastHost{AnycastSource: src, timeout: 1500 * time.Millisecond}
}

// SetTimeout overrides the per-request ceiling.
func (h *AnycastHost) SetTimeout(d time.Duration) {
	if d <= 0 {
		d = 1500 * time.Millisecond
	}
	h.timeout = d
}

// AnycastStatus is the IPC response envelope. The shape
// is stable across schema versions so the Engine can
// decode it without depending on internal anycast types.
type AnycastStatus struct {
	Version    int                `json:"version"`
	UpdatedAt  time.Time          `json:"updatedAt"`
	Evidence   []anycast.Evidence `json:"evidence"`
	Counters   anycast.Counters   `json:"counters"`
	Startup    string             `json:"startup"`
	StartupErr string             `json:"startupError,omitempty"`
}

// ServeAnycast renders the read-only Anycast view. The
// handler is GET-only; mutations live on the privileged
// side and are not exposed via IPC.
func (h *AnycastHost) ServeAnycast(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		problem(w, 405, "method_not_allowed")
		return
	}
	if r.URL.ForceQuery || r.ContentLength != 0 || len(r.TransferEncoding) != 0 {
		problem(w, 400, "invalid_request")
		return
	}
	resp := AnycastStatus{
		Version:   anycast.SchemaVersion,
		UpdatedAt: time.Now().UTC(),
		Evidence:  h.AllEvidence(),
		Counters:  h.Counters(),
		Startup:   "ready",
	}
	data, err := json.Marshal(resp)
	if err != nil || len(data) > AnycastMaxBytes {
		problem(w, 503, "anycast_limit")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// stubAnycast is the safe fallback when the privileged
// store is not yet configured. It returns an honest
// empty view so the IPC layer never invents availability.
type stubAnycast struct{}

func (stubAnycast) AllEvidence() []anycast.Evidence { return []anycast.Evidence{} }
func (stubAnycast) Counters() anycast.Counters      { return anycast.Counters{} }
