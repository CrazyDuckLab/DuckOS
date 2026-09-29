package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/wifi"
)

// WifiPath is the Agent IPC path for the Wi-Fi
// read-only surface.
const WifiPath = "/agent/v1/wifi"

// WifiMaxBytes is the Agent IPC cap for Wi-Fi
// payloads.
const WifiMaxBytes = 32 << 10

// WifiTimeout is the bound the Agent places on
// a single Wi-Fi round.
const WifiTimeout = 4 * time.Second

// WifiObserver is the Agent-facing read-only
// source of Wi-Fi data. The implementation
// owns the topology, the host capability and
// the per-AP health.
type WifiObserver interface {
	Wifi(ctx context.Context) (wifi.Status, error)
}

// WifiObservable is the alias used by the
// Handler.
type WifiObservable = WifiObserver

// ServeWifi is the Agent IPC handler for the
// Wi-Fi read-only surface.
func (h *Handler) ServeWifi(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		problem(w, 405, "method_not_allowed")
		return
	}
	if r.URL.RawQuery != "" || r.URL.ForceQuery || r.ContentLength != 0 || len(r.URL.Path) > len(WifiPath)+16 {
		problem(w, 400, "invalid_request")
		return
	}
	if !strings.HasSuffix(r.URL.Path, WifiPath) {
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
	host, ok := h.wifiHost.(WifiObserver)
	if !ok || host == nil {
		problem(w, 503, "wifi_unavailable")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), WifiTimeout)
	defer cancel()
	st, err := host.Wifi(ctx)
	if err != nil {
		problem(w, 503, "wifi_unavailable")
		return
	}
	if err := st.Topology.Valid(); err != nil && len(st.Topology.APs) > 0 {
		problem(w, 503, "wifi_invalid")
		return
	}
	data, err := json.Marshal(st)
	if err != nil || len(data) > WifiMaxBytes {
		problem(w, 503, "wifi_limit")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
