package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/identity"
	"github.com/CrazyDuckLab/DuckOS/internal/state"
)

type networkPlanner interface {
	PreviewNetwork(context.Context, json.RawMessage) ([]byte, error)
}
type networkSimulator interface {
	SimulateNetwork(context.Context, json.RawMessage) ([]byte, error)
}

func (h *Handler) serveNetwork(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store, private")
	if r.URL.RawQuery != "" || r.URL.ForceQuery || r.URL.RawPath != "" {
		problem(w, r, 400, errorCategoryFromCode("invalid_request"), "invalid_request", "Network parameters must not appear in URLs.")
		return
	}
	simulate := r.URL.Path == "/api/v1/network/simulate"
	if r.URL.Path != "/api/v1/network/preview" && !simulate {
		problem(w, r, 404, errorCategoryFromCode("not_found"), "not_found", "Unknown network operation.")
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		problem(w, r, 405, errorCategoryFromCode("method_not_allowed"), "method_not_allowed", "Preview requires POST and does not change host state.")
		return
	}
	if h.identity == nil || !h.identity.Enrolled() {
		problem(w, r, 403, errorCategoryFromCode("enrollment_required"), "enrollment_required", "Enroll a local Owner before planning network configuration.")
		return
	}
	token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok || len(r.Header.Values("Authorization")) != 1 {
		identityError(w, r, identity.ErrUnauthorized)
		return
	}
	if _, err := h.identity.Authorize(token, identity.ConfigureNetwork); err != nil {
		identityError(w, r, err)
		return
	}
	if r.Header.Get("Content-Type") != "application/json" || r.Header.Get("Content-Encoding") != "" {
		problem(w, r, 415, errorCategoryFromCode("invalid_content_type"), "invalid_content_type", "Use uncompressed application/json.")
		return
	}
	select {
	case h.slots <- struct{}{}:
		defer func() { <-h.slots }()
	default:
		problem(w, r, 503, errorCategoryFromCode("api_busy"), "api_busy", "Planning capacity is busy; retry later.")
		return
	}
	var evaluate func(context.Context, json.RawMessage) ([]byte, error)
	if simulate {
		if simulator, ok := h.engine.(networkSimulator); ok {
			evaluate = simulator.SimulateNetwork
		}
	} else {
		if planner, ok := h.engine.(networkPlanner); ok {
			evaluate = planner.PreviewNetwork
		}
	}
	if evaluate == nil {
		problem(w, r, 503, errorCategoryFromCode("planner_unavailable"), "planner_unavailable", "Network planner is unavailable.")
		return
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, state.MaxConfigBytes))
	if err != nil {
		problem(w, r, 400, errorCategoryFromCode("invalid_request"), "invalid_request", "Network request is unreadable or oversized.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	preview, err := evaluate(ctx, data)
	if err != nil {
		problem(w, r, 422, errorCategoryFromCode("invalid_network_configuration"), "invalid_network_configuration", "Network configuration is invalid, unsupported, or exceeds planning limits.")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(preview)
}
