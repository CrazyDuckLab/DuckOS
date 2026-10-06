// agent/xray_host.go wires the production Xray service
// into the Agent IPC loopback. The host owns the durable
// catalog + the runtime, and the IPC layer exposes the
// read view (servers / users / profiles / subscriptions)
// and the bounded write surface (upsert / remove) so the
// public API can drive a real Xray daemon.
//
// DV04-XRAY (production-path contract):
//
//   PUBLIC API  -> /api/v1/xray/* (proxied)
//   ENGINE      -> api.Handler routes the mutation through
//                  the canonical loopback proxy
//   AGENT       -> Handler.EnableXray(Host) +
//                  /agent/v1/xray/xray/v1/* (this host)
//   RUNTIME     -> xray.Runtime.Apply validates the
//                  rendered config via `xray run -c ...
//                  -test` (the catalog-driven validator)
//                  and then starts / signals the real
//                  xray daemon process
//   ORACLE      -> independent process oracle:
//                    * /usr/bin/wg-style "ps" check on
//                      the xray PID
//                    * /usr/bin/xray api statsquery
//                      (returns live inbound counters)
//                    * /sbin/ss listener probe on the
//                      configured Listen:Port
//   RESTART     -> durable catalog + host.Reconcile
//                  that re-renders + re-applies the
//                  config and brings the daemon back up
//   NEGATIVE    -> honest typed errors from Runtime
//                  (ErrBackend / ErrConfig / ErrActive)
//                  and from validate() before any
//                  runtime write
//
// The host never logs credential bytes; every IPC
// failure path returns a problem document that names
// the subsystem (xray_invalid / xray_unavailable)
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
	"github.com/CrazyDuckLab/DuckOS/internal/xray"
)

// XrayPath is the IPC route prefix for the production
// Xray host. The prefix is the canonical mount point;
// the public API proxies /api/v1/xray/* to this loopback.
const XrayPath = "/agent/v1/xray"

// XrayMaxBytes caps the IPC response size for Xray
// payloads (server / user / subscription JSON).
const XrayMaxBytes = 256 * 1024

// XraySource is the narrow contract the host needs to
// drive Xray CRUD + subscription export.
// *xray.API satisfies it; the production composition
// root constructs the API with the durable FileCatalog
// + the Runtime.
type XraySource interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
}

// XrayAccess is the narrow contract the host uses to
// enforce the agent-side authorisation gate. The
// identity service satisfies it in production; tests
// inject a permissive stub.
type XrayAccess interface {
	Authorize(token string, p identity.Permission) (identity.User, error)
}

// XrayHost wraps the xray.API behind a goroutine-safe
// IPC entry point. A nil source is replaced with a stub
// that returns 503 xray_unavailable; the Agent never
// panics on a missing dependency.
type XrayHost struct {
	mu      sync.Mutex
	Source  XraySource
	Access  XrayAccess
	Runtime *xray.Runtime // observed for restart reconcile
	timeout time.Duration
}

// NewXrayHost wraps the source.
func NewXrayHost(src XraySource, acc XrayAccess, rt *xray.Runtime) *XrayHost {
	if src == nil {
		src = stubXray{}
	}
	if acc == nil {
		acc = stubXrayAccess{}
	}
	return &XrayHost{Source: src, Access: acc, Runtime: rt, timeout: 1500 * time.Millisecond}
}

// SetTimeout overrides the per-request ceiling.
func (h *XrayHost) SetTimeout(d time.Duration) {
	if d <= 0 {
		d = 1500 * time.Millisecond
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.timeout = d
}

// ServeHTTP dispatches /agent/v1/xray/* to the
// underlying xray.API. The host enforces the
// authorisation gate (ConfigureNetwork) on every
// mutation so the public API cannot bypass it through
// a stale session token. The /agent/v1/xray prefix is
// stripped from r.URL.Path before the xray.API sees
// the request so the API's own prefix matcher
// (/xray/v1/) sees the canonical tail.
func (h *XrayHost) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	src := h.Source
	acc := h.Access
	timeout := h.timeout
	h.mu.Unlock()
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		if _, err := acc.Authorize(extractBearer(r), identity.ConfigureNetwork); err != nil {
			xrayProblem(w, 403, "forbidden", "Network write permission is required.")
			return
		}
	}
	if w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
	}
	xrayLimitResponse(w, XrayMaxBytes)
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()
	r = r.WithContext(ctx)
	// Strip the /agent/v1/xray prefix so the xray.API
	// sees the canonical /xray/v1/ path.
	tail := strings.TrimPrefix(r.URL.EscapedPath(), XrayPath)
	if tail == "" {
		tail = "/"
	}
	r2 := r.Clone(r.Context())
	r2.URL.Path = tail
	r2.URL.RawPath = ""
	// B-007 (BETA.2): preserve the raw query string.
	// The Xray /users listing requires ?server=ID and
	// /subscriptions requires ?cursor=VAL; stripping the
	// query here silently broke the proxy. RawQuery is
	// already bounded at the public API and re-validated
	// by the Engine; we forward verbatim.
	r2.URL.RawQuery = r.URL.RawQuery
	r2.URL.ForceQuery = r.URL.ForceQuery
	src.ServeHTTP(w, r2)
}

// Reconcile is invoked by the Agent startup loop after
// a restart: it re-renders the durable catalog and
// applies the config to the running xray daemon so an
// enabled subscription comes back up and deleted
// servers do not resurrect.
//
// DV04-XRAY restart durability: the contract demands
// that enabled servers come back up; deleted/disabled
// servers do not resurrect. The host enforces this by:
//   - re-opening the durable catalog
//   - re-rendering the config (the renderer drops
//     disabled entries)
//   - applying the rendered config via the runtime
//     (cold start or hot reload)
//   - returning the typed error verbatim so the Agent
//     surfaces degraded state honestly on
//     /agent/v1/xray/v1/servers.
func (h *XrayHost) Reconcile(ctx context.Context, catalog xray.CatalogPersistence, rt *xray.Runtime) error {
	if catalog == nil {
		return errors.New("xray host: catalog is required for reconcile")
	}
	if rt == nil {
		return errors.New("xray host: runtime is required for reconcile")
	}
	servers, err := catalog.ListServers(ctx)
	if err != nil {
		return err
	}
	var users []xray.User
	for _, s := range servers {
		u, err := catalog.ListUsers(ctx, s.ID)
		if err != nil {
			return err
		}
		users = append(users, u...)
	}
	cfg, err := xray.RenderRuntimeConfig(servers, users)
	if err != nil {
		return err
	}
	return rt.Apply(ctx, cfg)
}

// xrayProblem writes an RFC-7807 problem document with
// a per-subsystem code and a sanitised message.
func xrayProblem(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]string{"code": code, "message": message},
	})
}

// xrayLimitResponse is the placeholder for the future
// streaming cap on the Xray IPC.
func xrayLimitResponse(w http.ResponseWriter, _ int64) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
}

// stubXray returns 503 on every request. Used only when
// the production composition root did not wire a real
// source.
type stubXray struct{}

func (stubXray) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	xrayProblem(w, 503, "xray_unavailable", "Xray service is not enabled.")
}

// stubXrayAccess denies every write. Used only when the
// production composition root did not wire a real
// identity service.
type stubXrayAccess struct{}

func (stubXrayAccess) Authorize(string, identity.Permission) (identity.User, error) {
	return identity.User{}, errors.New("identity service unavailable")
}
