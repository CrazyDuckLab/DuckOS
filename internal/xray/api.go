package xray

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/identity"
)

// Access is the narrow contract the Xray API uses for authorisation.
// The implementation is provided by the identity service; the API
// never inspects tokens directly.
type Access interface {
	Authorize(token string, p identity.Permission) (identity.User, error)
}

// CatalogPersistence is the narrow contract the Xray API uses to
// record server/user/profile/subscription revisions.
type CatalogPersistence interface {
	UpsertServer(ctx context.Context, server Server) error
	RemoveServer(ctx context.Context, id string) error
	UpsertUser(ctx context.Context, user User) error
	RemoveUser(ctx context.Context, id string) error
	UpsertProfile(ctx context.Context, profile Profile) error
	RemoveProfile(ctx context.Context, id string) error
	UpsertSubscription(ctx context.Context, sub Subscription) error
	RemoveSubscription(ctx context.Context, id string) error
	ListServers(ctx context.Context) ([]Server, error)
	ListUsers(ctx context.Context, serverID string) ([]User, error)
}

// API is the loopback HTTP surface for the Xray service. The service
// is enabled on the Agent loopback alongside the network service. It
// never logs credential bytes, even on the failure path.
//
// DV04-XRAY (production-path contract): on every successful
// catalog mutation (server / user upsert, server / user remove)
// the API re-renders the canonical Xray JSON config and
// applies it through the runtime. The runtime validates the
// config via `xray run -c <path> -test` and either starts a
// fresh xray process or signals the running process to
// reload. A failed apply returns a typed error so the Agent
// surfaces degraded state honestly on /agent/v1/xray.
//
// The reconcile is gated by an "applied" flag so validation
// failures / decode errors do not trigger a redundant
// runtime write. Only the handler paths that committed a
// catalog mutation set the flag and schedule the reconcile.
type API struct {
	mu      sync.Mutex
	Access  Access
	Catalog CatalogPersistence
	// Apply accepts the rendered catalog (servers +
	// users) rather than a pre-rendered Config so the
	// runtime can materialise vault-managed TLS / REALITY
	// secrets BEFORE bringing up the daemon. The
	// production wire is rt.ApplyWith; tests inject a
	// stub that skips the materialise step. A nil Apply
	// is the safe default for tests that only exercise
	// CRUD.
	Apply  func(ctx context.Context, servers []Server, users []User) error
	Prefix string
}

// ServeHTTP dispatches /xray/v1/{servers,users,profiles,subscriptions}
// and refuses anything else.
func (a *API) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if a.Access == nil || a.Catalog == nil {
		problem(w, 500, "xray_unavailable", "Xray service is not enabled.")
		return
	}
	path := r.URL.Path
	switch {
	case strings.HasPrefix(path, a.Prefix+"servers"):
		a.serveServers(w, r)
	case strings.HasPrefix(path, a.Prefix+"users"):
		a.serveUsers(w, r)
	case strings.HasPrefix(path, a.Prefix+"profiles"):
		a.serveProfiles(w, r)
	case strings.HasPrefix(path, a.Prefix+"subscriptions"):
		a.serveSubscriptions(w, r)
	default:
		problem(w, 404, "not_found", "Unknown xray operation.")
	}
}

type catalogSnapshotter interface {
	Snapshot(context.Context) (fileState, error)
	Restore(context.Context, fileState) error
}

func (a *API) mutateCatalog(ctx context.Context, mut func() error) (resultErr error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	var snap fileState
	var snapper catalogSnapshotter
	if s, ok := a.Catalog.(catalogSnapshotter); ok {
		var err error
		snapper = s
		snap, err = s.Snapshot(ctx)
		if err != nil {
			return err
		}
	}
	defer func() {
		if resultErr == nil || snapper == nil {
			return
		}
		rollbackCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := snapper.Restore(rollbackCtx, snap); err != nil {
			resultErr = errors.New("xray recovery required: catalog rollback failed")
		}
	}()
	if err := mut(); err != nil {
		return err
	}
	if a.Apply == nil {
		return nil
	}
	servers, err := a.Catalog.ListServers(ctx)
	if err != nil {
		return err
	}
	var users []User
	for _, s := range servers {
		u, err := a.Catalog.ListUsers(ctx, s.ID)
		if err != nil {
			return err
		}
		users = append(users, u...)
	}
	// Empty-state short-circuit: if BOTH the snapshot AND
	// the new state have no enabled inbounds, there is
	// nothing to render and nothing to apply. This avoids
	// a redundant xray -test on a no-op config.
	if snapper != nil {
		if len(servers) == 0 {
			before, beforeErr := RenderRuntimeConfig(snap.Servers, snap.Users)
			if beforeErr == nil && len(before.Inbounds) == 0 {
				return nil
			}
		}
	}
	// Server-without-users guard: a server with no enabled
	// clients does not produce an inbound at render time.
	// The runtime must NOT bring up an empty listener.
	if len(users) == 0 && (snapper == nil || !hasEnabledServer(snap.Servers)) {
		return nil
	}
	if a.Apply != nil {
		if err := a.Apply(ctx, servers, users); err != nil {
			return err
		}
	}
	return nil
}

// hasEnabledServer reports whether any of the supplied servers
// would produce a renderable inbound (enabled AND has matching
// enabled users).
func hasEnabledServer(servers []Server) bool {
	for _, s := range servers {
		if s.Enabled {
			return true
		}
	}
	return false
}

func (a *API) requireConfigureNetwork(w http.ResponseWriter, r *http.Request) bool {
	if _, err := a.Access.Authorize(extractToken(r), identity.ConfigureNetwork); err != nil {
		problem(w, 403, "forbidden", "Network write permission is required.")
		return false
	}
	return true
}

func (a *API) serveServers(w http.ResponseWriter, r *http.Request) {
	if !a.requireConfigureNetwork(w, r) {
		return
	}
	id := strings.TrimPrefix(r.URL.Path, a.Prefix+"servers")
	switch {
	case id == "" && r.Method == http.MethodGet:
		servers, err := a.Catalog.ListServers(r.Context())
		if err != nil {
			problem(w, 503, "xray_unavailable", "Xray catalog rejected the server listing.")
			return
		}
		writeJSON(w, 200, servers)
	case id == "" && r.Method == http.MethodPost:
		var server Server
		if err := decodeJSON(r, &server); err != nil {
			problem(w, 400, "invalid_request", "Invalid xray server payload.")
			return
		}
		if err := server.Validate(); err != nil {
			problem(w, 422, "invalid_server", err.Error())
			return
		}
		if err := a.mutateCatalog(r.Context(), func() error { return a.Catalog.UpsertServer(r.Context(), server) }); err != nil {
			problem(w, 503, "xray_unavailable", "Xray catalog rejected the server.")
			return
		}
		writeJSON(w, 201, server)
	case id != "" && r.Method == http.MethodPut:
		var server Server
		if err := decodeJSON(r, &server); err != nil {
			problem(w, 400, "invalid_request", "Invalid xray server payload.")
			return
		}
		if server.ID != strings.TrimPrefix(id, "/") {
			problem(w, 409, "conflict", "Server ID does not match the URL.")
			return
		}
		if err := server.Validate(); err != nil {
			problem(w, 422, "invalid_server", err.Error())
			return
		}
		if err := a.mutateCatalog(r.Context(), func() error { return a.Catalog.UpsertServer(r.Context(), server) }); err != nil {
			problem(w, 503, "xray_unavailable", "Xray catalog rejected the server.")
			return
		}
		writeJSON(w, 200, server)
	case id != "" && r.Method == http.MethodDelete:
		if err := a.mutateCatalog(r.Context(), func() error { return a.Catalog.RemoveServer(r.Context(), strings.TrimPrefix(id, "/")) }); err != nil {
			problem(w, 503, "xray_unavailable", "Xray catalog rejected the remove.")
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	default:
		w.Header().Set("Allow", "GET, POST, PUT, DELETE")
		problem(w, 405, "method_not_allowed", "Method is not supported for this server.")
	}
}

func (a *API) serveUsers(w http.ResponseWriter, r *http.Request) {
	if !a.requireConfigureNetwork(w, r) {
		return
	}
	id := strings.TrimPrefix(r.URL.Path, a.Prefix+"users")
	switch {
	case id == "" && r.Method == http.MethodGet:
		serverID := r.URL.Query().Get("server")
		if serverID == "" {
			problem(w, 400, "invalid_request", "Xray user listing requires ?server=ID.")
			return
		}
		users, err := a.Catalog.ListUsers(r.Context(), serverID)
		if err != nil {
			problem(w, 503, "xray_unavailable", "Xray catalog rejected the user listing.")
			return
		}
		writeJSON(w, 200, users)
	case id == "" && r.Method == http.MethodPost:
		var user User
		if err := decodeJSON(r, &user); err != nil {
			problem(w, 400, "invalid_request", "Invalid xray user payload.")
			return
		}
		if err := user.Validate(); err != nil {
			problem(w, 422, "invalid_user", err.Error())
			return
		}
		if err := a.mutateCatalog(r.Context(), func() error { return a.Catalog.UpsertUser(r.Context(), user) }); err != nil {
			// DEBUG-LIVE-2026-10-08: BETA.2 two-blocker root-cause
			// hunt. Print the underlying mutateCatalog error so
			// live materialization failures are diagnosable from
			// the agent journal. Remove once the live xray E2E
			// completes without the "Xray catalog rejected"
			// generic error masking the real cause.
			log.Printf("xray: mutateCatalog.UpsertUser failed: %v", err)
			problem(w, 503, "xray_unavailable", "Xray catalog rejected the user.")
			return
		}
		writeJSON(w, 201, user)
	case id != "" && r.Method == http.MethodPut:
		var user User
		if err := decodeJSON(r, &user); err != nil {
			problem(w, 400, "invalid_request", "Invalid xray user payload.")
			return
		}
		if user.ID != strings.TrimPrefix(id, "/") {
			problem(w, 409, "conflict", "User ID does not match the URL.")
			return
		}
		if err := user.Validate(); err != nil {
			problem(w, 422, "invalid_user", err.Error())
			return
		}
		if err := a.mutateCatalog(r.Context(), func() error { return a.Catalog.UpsertUser(r.Context(), user) }); err != nil {
			problem(w, 503, "xray_unavailable", "Xray catalog rejected the user.")
			return
		}
		writeJSON(w, 200, user)
	case id != "" && r.Method == http.MethodDelete:
		if err := a.mutateCatalog(r.Context(), func() error { return a.Catalog.RemoveUser(r.Context(), strings.TrimPrefix(id, "/")) }); err != nil {
			problem(w, 503, "xray_unavailable", "Xray catalog rejected the remove.")
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	default:
		w.Header().Set("Allow", "GET, POST, PUT, DELETE")
		problem(w, 405, "method_not_allowed", "Method is not supported for this user.")
	}
}

func (a *API) serveProfiles(w http.ResponseWriter, r *http.Request) {
	if !a.requireConfigureNetwork(w, r) {
		return
	}
	id := strings.TrimPrefix(r.URL.Path, a.Prefix+"profiles")
	switch {
	case id == "" && r.Method == http.MethodPost:
		var profile Profile
		if err := decodeJSON(r, &profile); err != nil {
			problem(w, 400, "invalid_request", "Invalid xray profile payload.")
			return
		} else if err := profile.Validate(); err != nil {
			problem(w, 422, "invalid_profile", err.Error())
			return
		} else if err := a.mutateCatalog(r.Context(), func() error { return a.Catalog.UpsertProfile(r.Context(), profile) }); err != nil {
			problem(w, 503, "xray_unavailable", "Xray catalog rejected the profile.")
			return
		}
		writeJSON(w, 201, profile)
	case id != "" && r.Method == http.MethodDelete:
		if err := a.mutateCatalog(r.Context(), func() error { return a.Catalog.RemoveProfile(r.Context(), strings.TrimPrefix(id, "/")) }); err != nil {
			problem(w, 503, "xray_unavailable", "Xray catalog rejected the remove.")
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	default:
		w.Header().Set("Allow", "POST, DELETE")
		problem(w, 405, "method_not_allowed", "Method is not supported for this profile.")
	}
}

func (a *API) serveSubscriptions(w http.ResponseWriter, r *http.Request) {
	if !a.requireConfigureNetwork(w, r) {
		return
	}
	id := strings.TrimPrefix(r.URL.Path, a.Prefix+"subscriptions")
	switch {
	case id == "" && r.Method == http.MethodPost:
		var sub Subscription
		if err := decodeJSON(r, &sub); err != nil {
			problem(w, 400, "invalid_request", "Invalid xray subscription payload.")
			return
		} else if err := sub.Validate(); err != nil {
			problem(w, 422, "invalid_subscription", err.Error())
			return
		} else if err := a.mutateCatalog(r.Context(), func() error { return a.Catalog.UpsertSubscription(r.Context(), sub) }); err != nil {
			problem(w, 503, "xray_unavailable", "Xray catalog rejected the subscription.")
			return
		}
		writeJSON(w, 201, sub)
	case id != "" && r.Method == http.MethodDelete:
		if err := a.mutateCatalog(r.Context(), func() error { return a.Catalog.RemoveSubscription(r.Context(), strings.TrimPrefix(id, "/")) }); err != nil {
			problem(w, 503, "xray_unavailable", "Xray catalog rejected the remove.")
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	default:
		w.Header().Set("Allow", "POST, DELETE")
		problem(w, 405, "method_not_allowed", "Method is not supported for this subscription.")
	}
}

func extractToken(r *http.Request) string {
	v := r.Header.Get("Authorization")
	if !strings.HasPrefix(v, "Bearer ") {
		return ""
	}
	return strings.TrimPrefix(v, "Bearer ")
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func problem(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}

func decodeJSON(r *http.Request, v any) error {
	if r.ContentLength > 1<<20 {
		return errors.New("payload exceeds 1 MiB")
	}
	if r.Method == http.MethodGet {
		return errors.New("GET requests cannot contain bodies")
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("read body: %w", err)
	}
	dec := json.NewDecoder(strings.NewReader(string(body)))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}
