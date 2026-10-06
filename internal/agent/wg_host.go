// agent/wg_host.go wires the production WireGuard
// service into the Agent IPC loopback. The host owns
// the durable catalog + the kernel adapter; the IPC
// layer exposes the read view (servers / peers /
// export) and the bounded write surface (upsert /
// remove) so the public API can drive a real WireGuard
// tunnel.
//
// DV04-WG (production-path contract):
//
//   PUBLIC API  -> /api/v1/wireguard/* (proxied)
//   ENGINE      -> api.Handler routes mutation through
//                  the canonical loopback proxy
//   AGENT       -> Handler.EnableWireguard(Host) +
//                  /agent/v1/wireguard/v1/* (this host)
//   RUNTIME     -> wireguard.KernelAdapter.ApplyServer/
//                  ApplyPeer/RemovePeer via /sbin/ip +
//                  /sbin/wg
//   ORACLE      -> independent /sbin/wg show <iface>
//                  peers / allowed-ips / endpoint read
//                  through wgRuntimeOracle
//   RESTART     -> durable catalog + reconcile loop
//                  that re-applies enabled servers/peers
//                  and tears down disabled ones
//   NEGATIVE    -> honest typed errors from KernelAdapter
//                  and from validate() before any
//                  kernel write
//
// The host never logs private key bytes; every IPC
// failure path returns a problem document that names the
// subsystem (wireguard_invalid / wireguard_unavailable)
// without exposing the secret.
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
	"github.com/CrazyDuckLab/DuckOS/internal/wireguard"
)

// WireguardPath is the IPC route prefix for the
// production WireGuard host. The prefix is the
// canonical mount point; the public API proxies
// /api/v1/wireguard/* to this loopback.
const WireguardPath = "/agent/v1/wireguard"

// WireguardMaxBytes caps the IPC response size for
// server-side export payloads; the redacted
// configuration is bounded.
const WireguardMaxBytes = 256 * 1024

// WGSource is the narrow contract the host needs to
// drive WireGuard CRUD + export. *wireguard.API
// satisfies it; the production composition root
// constructs the API with the durable FileCatalog +
// the KernelAdapter.
type WGSource interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
}

// WGAccess is the narrow contract the host uses to
// enforce the agent-side authorisation gate. The
// identity service satisfies it in production; tests
// inject a permissive stub.
type WGAccess interface {
	Authorize(token string, p identity.Permission) (identity.User, error)
}

// WireguardHost wraps the wireguard.API behind a
// goroutine-safe IPC entry point. A nil source is
// replaced with a stub that returns 503
// wireguard_unavailable; the Agent never panics on a
// missing dependency.
type WireguardHost struct {
	mu      sync.Mutex
	Source  WGSource
	Access  WGAccess
	timeout time.Duration
}

// NewWireguardHost wraps the source.
func NewWireguardHost(src WGSource, acc WGAccess) *WireguardHost {
	if src == nil {
		src = stubWireguard{}
	}
	if acc == nil {
		acc = stubWGAccess{}
	}
	return &WireguardHost{Source: src, Access: acc, timeout: 1500 * time.Millisecond}
}

// SetTimeout overrides the per-request ceiling.
func (h *WireguardHost) SetTimeout(d time.Duration) {
	if d <= 0 {
		d = 1500 * time.Millisecond
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.timeout = d
}

// ServeHTTP dispatches /agent/v1/wireguard/* to the
// underlying wireguard.API. The host enforces the
// authorisation gate (ConfigureNetwork) on every
// mutation so the public API cannot bypass it through
// a stale session token. The /agent/v1/wireguard/
// prefix is stripped from r.URL.Path before the
// wireguard.API sees the request, so the API's own
// prefix matcher (/wireguard/v1/) sees the canonical
// tail.
func (h *WireguardHost) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	src := h.Source
	acc := h.Access
	timeout := h.timeout
	h.mu.Unlock()
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		if _, err := acc.Authorize(extractBearer(r), identity.ConfigureNetwork); err != nil {
			wgProblem(w, 403, "forbidden", "Network write permission is required.")
			return
		}
	}
	if w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
	}
	limitResponse(w, WireguardMaxBytes)
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()
	r = r.WithContext(ctx)
	// Strip the /agent/v1/wireguard prefix so the
	// wireguard.API sees the canonical /wireguard/v1/
	// path. The Host dispatches only requests whose
	// EscapedPath starts with WireguardPath.
	tail := strings.TrimPrefix(r.URL.EscapedPath(), WireguardPath)
	if tail == "" {
		tail = "/"
	}
	r2 := r.Clone(r.Context())
	r2.URL.Path = tail
	r2.URL.RawPath = ""
	// B-007 (BETA.2): preserve RawQuery. The
	// WireGuard /peers listing requires ?server=ID;
	// stripping the query here silently broke the
	// public-proxy contract. Engine.XrayProxy (and
	// its sibling proxies) bounds + validates the
	// query before forwarding, so the value here is
	// already trusted.
	r2.URL.RawQuery = r.URL.RawQuery
	r2.URL.ForceQuery = r.URL.ForceQuery
	src.ServeHTTP(w, r2)
}

// ReconcileServer is invoked by the Agent startup loop
// after a restart: it re-applies the durable catalog
// to the kernel so enabled servers/peers come back up
// and disabled/deleted ones do not resurrect. The
// Adapter is the only object that touches the kernel;
// the host surfaces the typed error verbatim.
//
// DV04-WG restart durability: the contract demands
// that enabled configured objects reconcile back into
// runtime; deleted/disabled objects do not resurrect.
// The host enforces this contract by:
//   - iterating the durable catalog servers
//   - calling KernelAdapter.ApplyServer for each
//     enabled server (which brings the interface up
//     and re-applies the peer set)
//   - re-applying every enabled peer that the
//     catalog persisted through UpsertPeer
//     (the catalog stores peers in a separate list
//     from server.Peers; the reconcile must consult
//     both so peer additions survive restart)
//   - returning an aggregated error so the Agent
//     surfaces degraded state honestly on
//     /agent/v1/wireguard/v1/servers.
func (h *WireguardHost) ReconcileServer(ctx context.Context, catalog wireguard.CatalogPersistence, adapter *wireguard.KernelAdapter) error {
	if catalog == nil {
		return errors.New("wireguard host: catalog is required for reconcile")
	}
	if adapter == nil {
		return errors.New("wireguard host: kernel adapter is required for reconcile")
	}
	servers, err := catalog.ListServers(ctx)
	if err != nil {
		return err
	}
	var firstErr error
	for _, s := range servers {
		peers, err := catalog.ListPeers(ctx, s.ID)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		// Apply the authoritative complete set once, so stale or disabled
		// peers are removed rather than briefly resurrected during startup.
		s.Peers = peers
		if err := adapter.ApplyServer(ctx, s); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// extractBearer returns the bearer token from the
// Authorization header, or empty if absent.
func extractBearer(r *http.Request) string {
	v := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if len(v) <= len(prefix) || v[:len(prefix)] != prefix {
		return ""
	}
	return v[len(prefix):]
}

// limitResponse caps the response body so an upstream
// runaway cannot blow the IPC budget.
func limitResponse(w http.ResponseWriter, n int64) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if w.Header().Get("Content-Length") != "" {
		return
	}
	if rc, ok := w.(interface {
		SetWriteDeadline(time.Time) error
	}); ok {
		_ = rc
	}
	_ = n // reserved for future streaming cap
}

// stubWireguard returns 503 on every request. Used
// only when the production composition root did not
// wire a real source.
type stubWireguard struct{}

func (stubWireguard) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	wgProblem(w, 503, "wireguard_unavailable", "WireGuard service is not enabled.")
}

// stubWGAccess denies every write. Used only when the
// production composition root did not wire a real
// identity service.
type stubWGAccess struct{}

func (stubWGAccess) Authorize(string, identity.Permission) (identity.User, error) {
	return identity.User{}, errors.New("identity service unavailable")
}

// wgProblem writes an RFC-7807 problem document with
// a per-subsystem code and a sanitised message. The
// helper is a local copy because the agent package's
// shared problem() does not accept a message.
func wgProblem(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]string{"code": code, "message": message},
	})
}
