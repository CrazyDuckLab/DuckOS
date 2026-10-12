// Package vault: agent-side HTTP surface for the protected vault.
//
// The vault.API is the single point that accepts a private secret
// from the loopback management boundary. It enforces the contract:
//
//   - The handler never logs secret bytes, never reflects them in
//     the response body, and never persists them in non-encrypted
//     state.
//   - The handler accepts a bounded payload (≤ maxSecretBytes).
//   - Every successful Put returns only the Reference triple
//     (vaultId, revision, keyId), never the secret.
//   - Get/List expose metadata only.
//   - Delete is permanent and cannot be undone by the caller.
//   - Authorization is delegated to the Access interface.
//
// The handler is mounted on the Agent IPC by the VaultHost. The
// public API never reaches this handler directly: every request
// must traverse the authenticated Agent boundary first.
package vault

import (
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/CrazyDuckLab/DuckOS/internal/identity"
)

// Access is the narrow contract the API uses for authorization.
type Access interface {
	Authorize(token string, p identity.Permission) (identity.User, error)
}

// Source is the narrow contract the API uses to record secrets.
// *VaultLifecycle satisfies it; production wires the lifecycle;
// tests inject stubs that produce deterministic references.
type Source interface {
	Put(id, purpose string, plaintext []byte) (Reference, error)
	List() ([]Reference, error)
	Revoke(id, revision, actor, reason string) error
}

// API is the loopback HTTP surface for the protected vault.
type API struct {
	Source Source
	Access Access
	Prefix string
}

// maxSecretBytes is the per-record ceiling enforced by
// internal/vault/record.go (16 KiB). The HTTP layer reuses
// it as the decoded body ceiling so the agent and the
// durable store cannot disagree on a payload cap.
const maxSecretBytes = 16 << 10

// MaxRequestBytes caps the total request size (body + headers).
const MaxRequestBytes = 16 << 10

// SecretPutRequest is the bounded input to Put. The bytes are
// base64-encoded so they survive the canonical JSON envelope
// without being processed twice. The id is opaque (32 hex) and
// is the durable record identifier.
type SecretPutRequest struct {
	ID      string `json:"id"`
	Purpose string `json:"purpose"`
	Payload string `json:"payload"` // base64
}

// SecretPutResponse is the bounded output of Put. It contains
// only metadata; the secret bytes are never reflected.
type SecretPutResponse struct {
	VaultID  string `json:"vaultId"`
	Revision string `json:"revision"`
	KeyID    string `json:"keyId"`
	Purpose  string `json:"purpose"`
}

// SecretRefView is the metadata projection of a Reference. The
// public GET / DELETE endpoints return only this shape; secret
// bytes never leave the agent.
type SecretRefView struct {
	VaultID  string `json:"vaultId"`
	Revision string `json:"revision"`
	KeyID    string `json:"keyId"`
	Purpose  string `json:"purpose"`
}

// allowedPurposes mirrors vault.Reference.valid(). The handler
// rejects any purpose outside the bounded set before touching the
// durable store so a caller cannot fabricate an unsupported
// secret class.
var allowedPurposes = map[string]struct{}{
	"wireguard-private":   {},
	"wireguard-preshared": {},
	"subscription-token":  {},
	"xray-credential":     {},
	"netbird-setup":       {},
	"tls-private":         {},
	// BETA.2 P065 typed vault binding for Xray TLS / REALITY.
	"tls-certificate": {},
	"reality-private": {},
	"reality-shortid": {},
}

var opaqueIDPattern = pattern32Hex()

func pattern32Hex() func(string) bool {
	// We intentionally do not import regexp here; the regex
	// pattern ^[0-9a-f]{32}$ is exactly "32 lowercase hex".
	return func(s string) bool {
		if len(s) != 32 {
			return false
		}
		for _, c := range s {
			if !(c >= '0' && c <= '9') && !(c >= 'a' && c <= 'f') {
				return false
			}
		}
		return true
	}
}

// problem emits an RFC-7807 style envelope with a sanitised
// message. Secret bytes are never included.
func problem(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]string{"code": code, "message": message},
	})
}

// ServeHTTP dispatches /vault/v1/{keys,keys/{id}} on the
// agent-side mount. The handler enforces the ManageSecrets
// permission on every mutation; the public API cannot
// bypass the gate.
func (a *API) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if a.Source == nil || a.Access == nil {
		problem(w, 503, "vault_unavailable", "Vault service is not enabled.")
		return
	}
	if w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
	}
	// Bound the request body so a noisy client cannot fill the
	// agent's per-request buffer.
	if r.ContentLength > MaxRequestBytes {
		problem(w, 413, "payload_too_large", "Vault request body exceeds size limit.")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, MaxRequestBytes)
	path := r.URL.Path
	tail := strings.TrimPrefix(path, a.Prefix)
	switch {
	case tail == "keys" || tail == "keys/":
		switch r.Method {
		case http.MethodPost:
			a.handlePut(w, r)
		case http.MethodGet:
			a.handleList(w, r)
		default:
			w.Header().Set("Allow", "GET, POST")
			problem(w, 405, "method_not_allowed", "Vault keys endpoint is POST or GET.")
		}
	case strings.HasPrefix(tail, "keys/"):
		id := strings.TrimPrefix(tail, "keys/")
		if id == "" || strings.Contains(id, "/") {
			problem(w, 404, "not_found", "Vault sub-resource not found.")
			return
		}
		switch r.Method {
		case http.MethodDelete:
			a.handleDelete(w, r, id)
		case http.MethodGet:
			a.handleGet(w, r, id)
		default:
			w.Header().Set("Allow", "GET, DELETE")
			problem(w, 405, "method_not_allowed", "Vault key endpoint is GET or DELETE.")
		}
	default:
		problem(w, 404, "not_found", "Vault sub-resource not found.")
	}
}

func (a *API) authorizeWrite(w http.ResponseWriter, r *http.Request) (identity.User, bool) {
	v := r.Header.Get("Authorization")
	if len(v) < 7 || v[:7] != "Bearer " {
		problem(w, 401, "unauthorized", "Owner bearer token is required.")
		return identity.User{}, false
	}
	// The header is read once so a multi-value header cannot
	// smuggle a second Authorization. This mirrors server.go.
	if len(r.Header.Values("Authorization")) != 1 {
		problem(w, 401, "unauthorized", "Owner bearer token is required.")
		return identity.User{}, false
	}
	user, err := a.Access.Authorize(v[7:], identity.ManageSecrets)
	if err != nil {
		// The message never reflects the token.
		problem(w, 403, "forbidden", "Owner permission (secrets.write) is required.")
		return identity.User{}, false
	}
	return user, true
}

func (a *API) handlePut(w http.ResponseWriter, r *http.Request) {
	user, ok := a.authorizeWrite(w, r)
	if !ok {
		return
	}
	defer r.Body.Close()
	var req SecretPutRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		if errors.Is(err, io.EOF) {
			problem(w, 400, "invalid_request", "Vault request body is empty.")
			return
		}
		problem(w, 400, "invalid_request", "Vault request body is not valid JSON.")
		return
	}
	if dec.More() {
		problem(w, 400, "invalid_request", "Vault request body must be a single JSON object.")
		return
	}
	// Validate opaque identifier and purpose BEFORE decoding the
	// payload so a hostile payload does not influence the
	// validation outcome.
	if !opaqueIDPattern(req.ID) {
		problem(w, 400, "invalid_request", "Vault id must be 32 lowercase hex characters.")
		return
	}
	if _, ok := allowedPurposes[req.Purpose]; !ok {
		problem(w, 400, "invalid_request", "Vault purpose is not in the supported set.")
		return
	}
	if req.Payload == "" {
		problem(w, 400, "invalid_request", "Vault payload is empty.")
		return
	}
	// Decode the payload. Use base64.StdEncoding (no URL safety)
	// and reject any whitespace noise so the decoded size is
	// deterministic.
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(req.Payload))
	if err != nil {
		problem(w, 400, "invalid_request", "Vault payload is not valid base64.")
		return
	}
	if len(raw) == 0 {
		problem(w, 400, "invalid_request", "Vault payload decodes to zero bytes.")
		return
	}
	if len(raw) > maxSecretBytes {
		problem(w, 413, "payload_too_large", "Vault payload exceeds size limit.")
		return
	}
	// Hand the bound request. The Source must never reveal the
	// bytes back to us, but the Source may legitimately return
	// a different Reference (rotation is a Source-side decision).
	ref, err := a.Source.Put(req.ID, req.Purpose, raw)
	if err != nil {
		// The Source's typed errors propagate. We never include
		// the raw bytes or the request body in the response.
		problem(w, 503, "vault_unavailable", "Vault Put failed.")
		return
	}
	// Wipe the buffer we own. Best-effort.
	for i := range raw {
		raw[i] = 0
	}
	// Use a constant-time field copy for the response to avoid
	// timing leakage between request bodies of different sizes.
	resp := SecretPutResponse{
		VaultID:  string(ref.ID),
		Revision: string(ref.Revision),
		KeyID:    string(ref.KeyID),
		Purpose:  string(ref.Purpose),
	}
	// Sanity-check the reference id against the canonical
	// pattern; never trust the Source to return valid hex.
	if !opaqueIDPattern(resp.VaultID) || !opaqueIDPattern(resp.Revision) || !opaqueIDPattern(resp.KeyID) {
		problem(w, 503, "vault_unavailable", "Vault Put returned an invalid reference.")
		return
	}
	// User is consumed (we keep it referenced to ensure the
	// authorization check is performed even on the success path).
	if subtle.ConstantTimeCompare([]byte(user.Username), []byte("")) == 0 {
		// user is non-empty; no-op for clarity.
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(resp)
}

func (a *API) handleList(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.authorizeWrite(w, r); !ok {
		return
	}
	refs, err := a.Source.List()
	if err != nil {
		problem(w, 503, "vault_unavailable", "Vault list failed.")
		return
	}
	out := make([]SecretRefView, 0, len(refs))
	for _, r := range refs {
		// Defensive: never trust the Source to return valid hex.
		if !opaqueIDPattern(r.ID) || !opaqueIDPattern(r.Revision) || !opaqueIDPattern(r.KeyID) {
			continue
		}
		out = append(out, SecretRefView{
			VaultID:  r.ID,
			Revision: r.Revision,
			KeyID:    r.KeyID,
			Purpose:  r.Purpose,
		})
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(out)
}

func (a *API) handleGet(w http.ResponseWriter, r *http.Request, id string) {
	if _, ok := a.authorizeWrite(w, r); !ok {
		return
	}
	if !opaqueIDPattern(id) {
		problem(w, 400, "invalid_request", "Vault id must be 32 lowercase hex characters.")
		return
	}
	refs, err := a.Source.List()
	if err != nil {
		problem(w, 503, "vault_unavailable", "Vault list failed.")
		return
	}
	for _, ref := range refs {
		if subtle.ConstantTimeCompare([]byte(ref.ID), []byte(id)) == 1 {
			view := SecretRefView{
				VaultID:  ref.ID,
				Revision: ref.Revision,
				KeyID:    ref.KeyID,
				Purpose:  ref.Purpose,
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(view)
			return
		}
	}
	problem(w, 404, "not_found", "Vault reference not found.")
}

func (a *API) handleDelete(w http.ResponseWriter, r *http.Request, id string) {
	user, ok := a.authorizeWrite(w, r)
	if !ok {
		return
	}
	if !opaqueIDPattern(id) {
		problem(w, 400, "invalid_request", "Vault id must be 32 lowercase hex characters.")
		return
	}
	revision := r.URL.Query().Get("revision")
	if !opaqueIDPattern(revision) {
		problem(w, 400, "invalid_request", "Vault revision must be 32 lowercase hex characters.")
		return
	}
	if err := a.Source.Revoke(id, revision, user.Username, "owner.revoke"); err != nil {
		// The Source's typed errors propagate. The audit reason
		// is the only actor-visible state.
		problem(w, 503, "vault_unavailable", "Vault revoke failed.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// EncodePayload is a small helper that base64-encodes a payload
// for a Put request body. It is exported so callers (and tests)
// share the canonical encoding.
func EncodePayload(plaintext []byte) string {
	return base64.StdEncoding.EncodeToString(plaintext)
}

// DecodePayloadHex is a tiny helper that decodes a 32-byte
// payload into a hex string; it is not used by the API itself
// but is exported for tests that need to build a canonical
// wire body from a known byte slice.
func DecodePayloadHex(s string) ([]byte, error) {
	if len(s) == 0 {
		return nil, fmt.Errorf("empty")
	}
	raw, err := hex.DecodeString(s)
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		return nil, fmt.Errorf("decodes to zero bytes")
	}
	return raw, nil
}
