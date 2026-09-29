// Package api config transaction endpoints (G01). CLI and
// UI go through the same Engine-mediated surface; there is
// no hidden second configuration path.
package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/CrazyDuckLab/DuckOS/internal/engine"
	"github.com/CrazyDuckLab/DuckOS/internal/identity"
	"github.com/CrazyDuckLab/DuckOS/internal/state"
)

// handleConfig dispatches the /api/v1/config/* endpoints.
// The Engine owns the canonical state; this handler is a
// thin translation layer that maps the public HTTP surface
// onto the typed Engine methods.
func (h *Handler) handleConfig(w http.ResponseWriter, r *http.Request) {
	permission := identity.ConfigureNetwork
	if r.URL.Path == "/api/v1/config/status" {
		permission = identity.Observe
	}
	user, ok := h.authorizedUser(w, r, permission)
	if !ok {
		return
	}
	if permission == identity.ConfigureNetwork {
		r = r.WithContext(engine.WithAuthenticatedActor(r.Context(), user.Username))
	}
	if r.URL.RawQuery != "" || r.URL.ForceQuery || r.URL.RawPath != "" {
		problem(w, r, 400, CatValidation, "invalid_request", "Configuration parameters must not appear in URLs.")
		return
	}
	select {
	case h.slots <- struct{}{}:
		defer func() { <-h.slots }()
	default:
		problem(w, r, 503, CatTimeout, "api_busy", "Configuration capacity is busy.")
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/config/")
	operation, txID, hasID := strings.Cut(path, "/")
	if hasID && ((operation != "confirm" && operation != "rollback") || !state.ValidTransactionID(txID)) {
		problem(w, r, 400, CatValidation, "invalid_request", "A canonical transaction ID is required.")
		return
	}
	cfg := h.engine.Config()
	if cfg == nil {
		problem(w, r, 503, errorCategoryFromCode("agent_unavailable"), "agent_unavailable", "Config journal is not enabled on this node.")
		return
	}
	switch operation {
	case "status":
		if r.Method != http.MethodGet {
			writeAllow(w, "GET")
			problem(w, r, 405, errorCategoryFromCode("method_not_allowed"), "method_not_allowed", "Config status is read-only.")
			return
		}
		status, err := cfg.Status(r.Context())
		if err != nil {
			problem(w, r, 503, CatRuntimeFailure, "agent_unavailable", "Configuration state is unavailable.")
			return
		}
		writeJSON(w, 200, status)
	case "diff":
		if r.Method != http.MethodPost {
			writeAllow(w, "POST")
			problem(w, r, 405, errorCategoryFromCode("method_not_allowed"), "method_not_allowed", "Config diff requires POST.")
			return
		}
		body, err := readBoundedBody(r, maxDiffBodyBytes)
		if err != nil {
			problem(w, r, 400, errorCategoryFromCode("invalid_request"), "invalid_request", err.Error())
			return
		}
		var req engine.ConfigDiffRequest
		if err := json.Unmarshal(body, &req); err != nil {
			problem(w, r, 400, errorCategoryFromCode("invalid_request"), "invalid_request", "Diff body could not be decoded.")
			return
		}
		diff, err := cfg.Diff(r.Context(), req)
		if err != nil {
			writeEngineError(w, r, err)
			return
		}
		writeJSON(w, 200, diff)
	case "apply":
		if r.Method != http.MethodPost {
			writeAllow(w, "POST")
			problem(w, r, 405, errorCategoryFromCode("method_not_allowed"), "method_not_allowed", "Config apply requires POST.")
			return
		}
		body, err := readBoundedBody(r, maxApplyBodyBytes)
		if err != nil {
			problem(w, r, 400, errorCategoryFromCode("invalid_request"), "invalid_request", err.Error())
			return
		}
		var req engine.ConfigApplyRequest
		if err := json.Unmarshal(body, &req); err != nil {
			problem(w, r, 400, errorCategoryFromCode("invalid_request"), "invalid_request", "Apply body could not be decoded.")
			return
		}
		tx, err := cfg.Prepare(r.Context(), req)
		if err != nil {
			writeEngineError(w, r, err)
			return
		}
		status := http.StatusAccepted
		if tx.Phase == "committed" || tx.Phase == "rolled-back" {
			status = http.StatusOK
		}
		writeJSON(w, status, tx)
	case "confirm":
		if r.Method != http.MethodPost {
			writeAllow(w, "POST")
			problem(w, r, 405, errorCategoryFromCode("method_not_allowed"), "method_not_allowed", "Config confirm requires POST.")
			return
		}
		if !hasID {
			problem(w, r, 400, errorCategoryFromCode("invalid_request"), "invalid_request", "Transaction id required in URL path.")
			return
		}
		out, err := cfg.Confirm(r.Context(), txID)
		if err != nil {
			writeEngineError(w, r, err)
			return
		}
		writeJSON(w, 200, out)
	case "rollback":
		if r.Method != http.MethodPost {
			writeAllow(w, "POST")
			problem(w, r, 405, errorCategoryFromCode("method_not_allowed"), "method_not_allowed", "Config rollback requires POST.")
			return
		}
		if !hasID {
			problem(w, r, 400, errorCategoryFromCode("invalid_request"), "invalid_request", "Transaction id required in URL path.")
			return
		}
		out, err := cfg.Rollback(r.Context(), txID, r.Header.Get("X-Request-Id"))
		if err != nil {
			writeEngineError(w, r, err)
			return
		}
		writeJSON(w, 200, out)
	default:
		problem(w, r, 404, errorCategoryFromCode("not_found"), "not_found", "Unknown config operation.")
	}
}

// writeAllow sets the Allow header for 405 responses.
func writeAllow(w http.ResponseWriter, methods string) {
	w.Header().Set("Allow", methods)
}

// writeEngineError maps an engine error to the right
// status + category. The state package uses ErrConflict for
// revision conflicts and ErrRecovery for recovery-required
// states; everything else maps to a 400 invalid_request that
// also carries the underlying validator/network message in a
// `detail` field so operators (and the UI) can act on it.
func writeEngineError(w http.ResponseWriter, r *http.Request, err error) {
	requestID := r.Header.Get("X-Request-Id")
	if requestID == "" {
		requestID = newRequestID()
	}
	w.Header().Set("X-Request-Id", requestID)
	detail := err.Error()
	fmt.Fprintf(os.Stderr, "duck-api: config-error request=%s method=%s path=%s err=%q\n", requestID, r.Method, r.URL.Path, detail)
	switch {
	case errors.Is(err, engine.ErrConfigRuntimeUnavailable):
		problemWithDetail(w, r, 503, CatUnsupported, "config_runtime_unavailable", "The verified configuration coordinator is not connected; no change was applied.", detail, requestID)
	case isRevisionConflict(err):
		problemWithDetail(w, r, 409, CatRevisionConflict, "revision_conflict", "Configuration revision or transaction conflicts with current state.", detail, requestID)
	case isRecoveryRequired(err):
		problemWithDetail(w, r, 409, CatRecoveryRequired, "recovery_required", "Configuration state requires recovery.", detail, requestID)
	default:
		problemWithDetail(w, r, 400, CatValidation, "invalid_request", "Configuration request was rejected; inspect validated input and current state.", detail, requestID)
	}
}

// problemWithDetail extends problem() with an optional `detail`
// field. The detail is the underlying error string from the
// engine/validator/network decoder; it is safe to expose here
// because every caller already runs it through their own public
// validators (Decode errors are produced for user-supplied input
// and the network decoder only returns lexical messages).
func problemWithDetail(w http.ResponseWriter, r *http.Request, status int, category ErrorCategory, code, message, detail, requestID string) {
	body := map[string]any{
		"category":  category,
		"code":      code,
		"message":   message,
		"detail":    detail,
		"requestId": requestID,
	}
	writeJSON(w, status, map[string]any{"error": body})
}

const (
	maxDiffBodyBytes  = 1 << 20
	maxApplyBodyBytes = 1 << 20
)

// readBoundedBody reads the request body up to the supplied
// cap. The cap protects the API from accidentally loading a
// multi-megabyte payload into memory.
func readBoundedBody(r *http.Request, cap int64) ([]byte, error) {
	if r.Body == nil {
		return nil, nil
	}
	r.Body = http.MaxBytesReader(nil, r.Body, cap)
	defer r.Body.Close()
	return io.ReadAll(r.Body)
}

// isRevisionConflict returns true when the engine error
// represents a base/current revision mismatch. The state
// package surfaces this through ErrConflict wrapped errors.
func isRevisionConflict(err error) bool {
	return errors.Is(err, state.ErrConflict) || errors.Is(err, state.ErrPending)
}

// isRecoveryRequired returns true when the engine signals
// that the journal requires operator intervention.
func isRecoveryRequired(err error) bool {
	return errors.Is(err, state.ErrRecovery)
}
