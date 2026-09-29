package api

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/CrazyDuckLab/DuckOS/internal/identity"
)

// EnableIdentity is startup-only; ownership and Close remain with the caller.
// Without this explicit opt-in the existing observation-only contract remains.
func (h *Handler) EnableIdentity(s *identity.Service) {
	h.identity = s
	h.identitySlots = make(chan struct{}, 2)
}

// EnablePolicy wires the durable per-object policy
// overlay. The policy is the production authorization
// surface for every protected list / detail / mutation /
// action / export handler. Without EnablePolicy the
// Handler falls back to the in-memory NewPolicy()
// default — DV05 requires that the production startup
// loads the durable store, not the unit-test fallback.
func (h *Handler) EnablePolicy(p *identity.Policy) {
	if p == nil {
		return
	}
	h.policy = p
}

func (h *Handler) serveIdentity(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawQuery != "" || r.URL.ForceQuery || r.URL.RawPath != "" {
		problem(w, r, 400, errorCategoryFromCode("invalid_request"), "invalid_request", "Authentication parameters must not appear in URLs.")
		return
	}
	methods := map[string]string{
		"/api/v1/auth/status": "GET", "/api/v1/auth/enroll": "POST", "/api/v1/auth/login": "POST", "/api/v1/auth/me": "GET", "/api/v1/auth/logout": "POST", "/api/v1/auth/sessions": "GET", "/api/v1/auth/revoke": "POST", "/api/v1/auth/users": "GET, POST, PUT", "/api/v1/auth/password": "POST", "/api/v1/auth/audit": "GET",
	}
	method, exists := methods[r.URL.Path]
	if !exists {
		problem(w, r, 404, errorCategoryFromCode("not_found"), "not_found", "Unknown authentication operation.")
		return
	}
	allowed := false
	for _, m := range strings.Split(method, ", ") {
		allowed = allowed || r.Method == m
	}
	if !allowed {
		w.Header().Set("Allow", method)
		problem(w, r, 405, errorCategoryFromCode("method_not_allowed"), "method_not_allowed", "Method is not supported for this operation.")
		return
	}
	if r.Method == http.MethodGet && (r.ContentLength != 0 || len(r.TransferEncoding) != 0) {
		problem(w, r, 400, errorCategoryFromCode("invalid_request"), "invalid_request", "GET requests cannot contain bodies.")
		return
	}
	select {
	case h.identitySlots <- struct{}{}:
		defer func() { <-h.identitySlots }()
	default:
		problem(w, r, 429, errorCategoryFromCode("auth_busy"), "auth_busy", "Authentication capacity is busy; retry later.")
		return
	}
	token, hasToken := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !hasToken || len(r.Header.Values("Authorization")) != 1 {
		token = ""
	}
	if r.URL.Path == "/api/v1/auth/status" {
		writeJSON(w, 200, map[string]any{"enrolled": h.identity.Enrolled(), "mode": "local-sessions"})
		return
	}
	if r.URL.Path == "/api/v1/auth/enroll" {
		hash := sha256.Sum256([]byte(token))
		if subtle.ConstantTimeCompare(hash[:], h.tokenHash[:]) != 1 {
			identityError(w, r, identity.ErrUnauthorized)
			return
		}
		var input struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if !identityBody(w, r, &input) {
			return
		}
		user, err := h.identity.Enroll(input.Username, input.Password)
		if err != nil {
			identityError(w, r, err)
			return
		}
		writeJSON(w, 201, user)
		return
	}
	if r.URL.Path == "/api/v1/auth/login" {
		var input struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if !identityBody(w, r, &input) {
			return
		}
		issued, session, err := h.identity.Login(input.Username, input.Password)
		if err != nil {
			identityError(w, r, err)
			return
		}
		writeJSON(w, 200, map[string]any{"token": issued, "session": session})
		return
	}
	if _, err := h.identity.Current(token); err != nil {
		identityError(w, r, err)
		return
	}
	switch r.URL.Path {
	case "/api/v1/auth/me":
		session, err := h.identity.Current(token)
		if err != nil {
			identityError(w, r, err)
			return
		}
		writeJSON(w, 200, session)
	case "/api/v1/auth/logout":
		var input struct{}
		if !identityBody(w, r, &input) {
			return
		}
		h.identity.Logout(token)
		writeJSON(w, 200, map[string]bool{"ok": true})
	case "/api/v1/auth/sessions":
		sessions, err := h.identity.Sessions(token)
		if err != nil {
			identityError(w, r, err)
			return
		}
		writeJSON(w, 200, sessions)
	case "/api/v1/auth/revoke":
		var input struct {
			ID string `json:"id"`
		}
		if !identityBody(w, r, &input) {
			return
		}
		if err := h.identity.Revoke(token, input.ID); err != nil {
			identityError(w, r, err)
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	case "/api/v1/auth/password":
		var input struct {
			Old string `json:"oldPassword"`
			New string `json:"newPassword"`
		}
		if !identityBody(w, r, &input) {
			return
		}
		if err := h.identity.ChangePassword(token, input.Old, input.New); err != nil {
			identityError(w, r, err)
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	case "/api/v1/auth/audit":
		entries, err := h.identity.Audit(token)
		if err != nil {
			identityError(w, r, err)
			return
		}
		writeJSON(w, 200, entries)
	case "/api/v1/auth/users":
		if r.Method == http.MethodGet {
			users, err := h.identity.Users(token)
			if err != nil {
				identityError(w, r, err)
				return
			}
			writeJSON(w, 200, users)
			return
		}
		if r.Method == http.MethodPost {
			var input struct {
				Username string        `json:"username"`
				Password string        `json:"password"`
				Role     identity.Role `json:"role"`
			}
			if !identityBody(w, r, &input) {
				return
			}
			u, err := h.identity.CreateUser(token, input.Username, input.Password, input.Role)
			if err != nil {
				identityError(w, r, err)
				return
			}
			writeJSON(w, 201, u)
			return
		}
		var input struct {
			ID       string        `json:"id"`
			Role     identity.Role `json:"role"`
			Disabled bool          `json:"disabled"`
		}
		if !identityBody(w, r, &input) {
			return
		}
		if err := h.identity.UpdateUser(token, input.ID, input.Role, input.Disabled); err != nil {
			identityError(w, r, err)
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	}
}
func identityBody(w http.ResponseWriter, r *http.Request, out any) bool {
	if r.Header.Get("Content-Type") != "application/json" {
		problem(w, r, 415, errorCategoryFromCode("invalid_content_type"), "invalid_content_type", "Use application/json.")
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(out); err != nil {
		problem(w, r, 400, errorCategoryFromCode("invalid_request"), "invalid_request", "Malformed or oversized authentication request.")
		return false
	}
	if d.Decode(new(any)) != io.EOF {
		problem(w, r, 400, errorCategoryFromCode("invalid_request"), "invalid_request", "Trailing authentication data.")
		return false
	}
	return true
}
func identityError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, identity.ErrUnauthorized):
		problem(w, r, 401, errorCategoryFromCode("unauthorized"), "unauthorized", "Credentials are invalid or the session expired.")
	case errors.Is(err, identity.ErrForbidden):
		problem(w, r, 403, errorCategoryFromCode("forbidden"), "forbidden", "This identity cannot perform the operation.")
	case errors.Is(err, identity.ErrRateLimited):
		w.Header().Set("Retry-After", "60")
		problem(w, r, 429, errorCategoryFromCode("rate_limited"), "rate_limited", "Authentication capacity exceeded; retry later.")
	default:
		problem(w, r, 400, errorCategoryFromCode("identity_change_rejected"), "identity_change_rejected", "Identity change rejected. Check input, password requirements, and the last active Owner; persistent failures require local recovery.")
	}
}
