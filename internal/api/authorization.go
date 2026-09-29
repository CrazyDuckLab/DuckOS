package api

import (
	"crypto/sha256"
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/CrazyDuckLab/DuckOS/internal/identity"
)

// authorizeManagement replaces the bootstrap credential after enrollment.
// Bootstrap credentials allow observations only, never configuration changes.
func (h *Handler) authorizeManagement(w http.ResponseWriter, r *http.Request, permission identity.Permission) bool {
	_, ok := h.authorizedUser(w, r, permission)
	return ok
}

func (h *Handler) authorizedUser(w http.ResponseWriter, r *http.Request, permission identity.Permission) (identity.User, bool) {
	token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok || len(r.Header.Values("Authorization")) != 1 {
		identityError(w, r, identity.ErrUnauthorized)
		return identity.User{}, false
	}
	if h.identity != nil && h.identity.Enrolled() {
		u, err := h.identity.Authorize(token, permission)
		if err != nil {
			identityError(w, r, err)
			return identity.User{}, false
		}
		return u, true
	}
	hash := sha256.Sum256([]byte(token))
	if subtle.ConstantTimeCompare(hash[:], h.tokenHash[:]) != 1 {
		identityError(w, r, identity.ErrUnauthorized)
		return identity.User{}, false
	}
	if permission != identity.Observe {
		problem(w, r, 403, CatPermissionDenied, "enrollment_required", "Enroll a local Owner before changing configuration.")
		return identity.User{}, false
	}
	return identity.User{Username: "bootstrap"}, true
}
