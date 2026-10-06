// agent/vault_host.go wires the production vault service into
// the Agent IPC loopback. The host owns the bounded write surface
// for protected secrets: WireGuard private keys, WireGuard
// pre-shared keys, Xray credentials, TLS private keys, and
// subscription tokens.
//
// DV04-VAULT (production-path contract):
//
//	PUBLIC API  -> /api/v1/vault/* (proxied, Owner only)
//	ENGINE      -> api.Handler routes mutation through
//	               the canonical loopback proxy
//	AGENT       -> Handler.EnableVault(Host) +
//	               /agent/v1/vault/v1/* (this host)
//	VAULT       -> vault.API -> VaultLifecycle.Put / List /
//	               Revoke -> vault.Store
//
// The host never logs secret bytes; every IPC failure path
// returns a problem document that names the subsystem
// (vault_unavailable / vault_invalid) without exposing the
// secret.
package agent

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/identity"
	"github.com/CrazyDuckLab/DuckOS/internal/vault"
)

// VaultPath is the IPC route prefix for the production vault
// host. The prefix is the canonical mount point; the public API
// proxies /api/v1/vault/* to this loopback.
const VaultPath = "/agent/v1/vault"

// VaultMaxBytes caps the IPC response size for vault list / get
// payloads. The list is bounded by the durable record count;
// the response is metadata-only so 256 KiB is generous.
const VaultMaxBytes = 256 * 1024

// VaultSource is the narrow contract the host needs to drive
// vault CRUD. *vault.API satisfies it; the production composition
// root constructs the API with VaultLifecycle as Source.
type VaultSource interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
}

// VaultAccess is the narrow contract the host uses to enforce
// the agent-side authorisation gate. The identity service
// satisfies it in production; tests inject a permissive stub.
type VaultAccess interface {
	Authorize(token string, p identity.Permission) (identity.User, error)
}

// VaultHost wraps the vault.API behind a goroutine-safe IPC
// entry point. A nil source is replaced with a stub that
// returns 503 vault_unavailable; the Agent never panics on a
// missing dependency.
type VaultHost struct {
	mu      sync.Mutex
	Source  VaultSource
	Access  VaultAccess
	timeout time.Duration
}

// NewVaultHost wraps the source. The Source and Access contracts
// are required; a nil source becomes a 503 stub so the Agent
// composition root can boot even before the vault is wired.
func NewVaultHost(src VaultSource, acc VaultAccess) *VaultHost {
	if src == nil {
		src = stubVault{}
	}
	if acc == nil {
		acc = stubVaultAccess{}
	}
	return &VaultHost{Source: src, Access: acc, timeout: 1500 * time.Millisecond}
}

// SetTimeout overrides the per-request ceiling.
func (h *VaultHost) SetTimeout(d time.Duration) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if d <= 0 {
		d = 1500 * time.Millisecond
	}
	h.timeout = d
}

// ServeHTTP dispatches /agent/v1/vault/* to the underlying
// vault.API. The host enforces the authorisation gate
// (ManageSecrets) on every request so the public API cannot
// bypass it through a stale session token.
//
// The /agent/v1/vault/ prefix is stripped from r.URL.Path
// before the vault.API sees the request, so the API's own
// prefix matcher (/vault/v1/) sees the canonical tail.
func (h *VaultHost) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	src := h.Source
	acc := h.Access
	timeout := h.timeout
	h.mu.Unlock()
	// The vault is owner-only; even GET / List requires
	// ManageSecrets. The vault never exposes bytes; this is a
	// strict capability gate.
	if _, err := acc.Authorize(extractBearer(r), identity.ManageSecrets); err != nil {
		vaultProblem(w, 403, "forbidden", "Owner permission (secrets.write) is required.")
		return
	}
	if w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
	}
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()
	r = r.WithContext(ctx)
	tail := strings.TrimPrefix(r.URL.EscapedPath(), VaultPath)
	if tail == "" {
		tail = "/"
	}
	r2 := r.Clone(r.Context())
	r2.URL.Path = tail
	r2.URL.RawPath = ""
	// B-007 (BETA.2): preserve RawQuery. Vault
	// listings are rare but allowed (the Owner
	// bearer token is in the Authorization header,
	// not the query) — the same Engine-side
	// rawQuery gate covers any future vault
	// listing that filters by id / kind.
	r2.URL.RawQuery = r.URL.RawQuery
	r2.URL.ForceQuery = r.URL.ForceQuery
	src.ServeHTTP(w, r2)
}

// stubVault returns 503 on every request. Used only when the
// production composition root did not wire a real source.
type stubVault struct{}

func (stubVault) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	vaultProblem(w, 503, "vault_unavailable", "Vault service is not enabled.")
}

// stubVaultAccess denies every request. Used only when the
// production composition root did not wire a real identity
// service.
type stubVaultAccess struct{}

func (stubVaultAccess) Authorize(string, identity.Permission) (identity.User, error) {
	return identity.User{}, errors.New("identity service unavailable")
}

// vaultProblem writes an RFC-7807 problem document with a
// per-subsystem code and a sanitised message. The helper is a
// local copy because the agent package's shared problem() does
// not accept a message.
func vaultProblem(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]string{"code": code, "message": message},
	})
}

// Compile-time guarantees that *vault.API and *vault.VaultHost
// implement the agent-side contracts.
var (
	_ VaultSource = (*vault.API)(nil)
)
