package api

import "crypto/ed25519"

// EnableUpdaterHealthKey is startup-only. The API receives only a verification
// key; it cannot mint updater assertions or management sessions with it.
func (h *Handler) EnableUpdaterHealthKey(key ed25519.PublicKey) {
	h.updaterHealthKey = append([]byte(nil), key...)
}
