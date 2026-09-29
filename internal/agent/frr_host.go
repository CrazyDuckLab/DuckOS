// agent/frr_host.go: production bridge between the
// DV09 IPC layer and the real FRR runtime.
//
// The Agent owns the *frr.Runtime lifecycle; the
// runtime owns the real /usr/lib/frr/{zebra,bgpd}
// daemons inside a private netns. The IPC layer is
// a thin HTTP wrapper:
//
//   GET  /agent/v1/frr             -> canonical
//                                    status snapshot
//                                    (sessions,
//                                    routes,
//                                    lastReload,
//                                    lastError,
//                                    pathspace)
//   POST /agent/v1/frr/apply       -> stage + reload
//                                    (canonical config
//                                    JSON body)
//   POST /agent/v1/frr/rollback    -> restore the
//                                    previous config
//                                    and reload
//
// A nil runtime returns 503 frr_unavailable on
// every route; the Agent composition root refuses
// to register the host without a real *frr.Runtime.

package agent

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/CrazyDuckLab/DuckOS/internal/frr"
)

// FRRHost wraps the real *frr.Runtime so the IPC
// layer never has to know about FRR internals.
type FRRHost struct {
	runtime *frr.Runtime
}

// NewFRRHost returns the host bound to the production
// runtime. A nil runtime returns nil — callers MUST
// refuse to register a nil host so the IPC layer
// never invents availability.
func NewFRRHost(rt *frr.Runtime) *FRRHost {
	if rt == nil {
		return nil
	}
	return &FRRHost{runtime: rt}
}

func (h *FRRHost) Close() {
	if h != nil && h.runtime != nil {
		h.runtime.Stop()
	}
}

// ServeFRR routes GET /agent/v1/frr to Status and the
// sub-paths to Apply / Rollback. The handler enforces
// the canonical HTTP contract; the runtime owns the
// FRR lifecycle.
func (h *FRRHost) ServeFRR(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.runtime == nil {
		writeFRRProblem(w, 503, "frr_unavailable", "FRR runtime is not configured.")
		return
	}
	switch r.URL.EscapedPath() {
	case frr.FRRPath:
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", "GET")
			writeFRRProblem(w, 405, "method_not_allowed", "FRR status accepts GET only.")
			return
		}
		data, err := h.runtime.MarshalStatus("")
		if err != nil {
			writeFRRProblem(w, 503, "encode_failed", "Could not encode FRR status.")
			return
		}
		writeFRRJSON(w, 200, data)
		return
	case frr.FRRApplyPath:
		h.serveApply(w, r)
		return
	case frr.FRRRollbackPath:
		h.serveRollback(w, r)
		return
	}
	writeFRRProblem(w, 404, "not_found", "Unknown FRR operation.")
}

// serveApply reads the canonical Config JSON from
// the body, runs runtime.Apply, and returns the
// lifecycle response.
func (h *FRRHost) serveApply(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		writeFRRProblem(w, 405, "method_not_allowed", "FRR apply accepts POST only.")
		return
	}
	if r.ContentLength > frr.ApplyMaxBytes {
		writeFRRProblem(w, 413, "payload_too_large", "FRR config exceeds the Agent cap.")
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, frr.ApplyMaxBytes+1))
	if err != nil {
		writeFRRProblem(w, 400, "invalid_request", "Could not read FRR config.")
		return
	}
	if len(body) > frr.ApplyMaxBytes {
		writeFRRProblem(w, 413, "payload_too_large", "FRR config exceeds the Agent cap.")
		return
	}
	var cfg frr.Config
	if err := json.Unmarshal(body, &cfg); err != nil {
		writeFRRProblem(w, 400, "invalid_request", "FRR config is not canonical JSON.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*1e9) // 30s
	defer cancel()
	if err := h.runtime.Apply(ctx, cfg); err != nil {
		if errors.Is(err, frr.ErrConfig) {
			writeFRRProblem(w, 422, "invalid_config", err.Error())
			return
		}
		writeFRRProblem(w, 422, "apply_refused", err.Error())
		return
	}
	data, _ := h.runtime.MarshalLifecycle("applied", "")
	writeFRRJSON(w, 200, data)
}

// serveRollback reads {"reason": "..."} from the
// body, runs runtime.Rollback, and returns the
// lifecycle response.
func (h *FRRHost) serveRollback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		writeFRRProblem(w, 405, "method_not_allowed", "FRR rollback accepts POST only.")
		return
	}
	var body struct {
		Reason string `json:"reason"`
	}
	raw, _ := io.ReadAll(io.LimitReader(r.Body, 1<<16))
	_ = json.Unmarshal(raw, &body)
	ctx, cancel := context.WithTimeout(r.Context(), 30*1e9) // 30s
	defer cancel()
	if err := h.runtime.Rollback(ctx, body.Reason); err != nil {
		writeFRRProblem(w, 422, "rollback_refused", err.Error())
		return
	}
	data, _ := h.runtime.MarshalLifecycle("rolled-back", body.Reason)
	writeFRRJSON(w, 200, data)
}

func writeFRRJSON(w http.ResponseWriter, status int, body []byte) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

func writeFRRProblem(w http.ResponseWriter, status int, code, message string) {
	writeFRRJSON(w, status, mustEncode(map[string]any{
		"error": map[string]string{"code": code, "message": message},
	}))
}

func mustEncode(v any) []byte {
	data, err := json.Marshal(v)
	if err != nil {
		return []byte(`{"error":{"code":"encode_failed","message":"could not encode"}}`)
	}
	return data
}
