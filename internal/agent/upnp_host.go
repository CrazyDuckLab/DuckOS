// agent/upnp_host.go: agent-side mount for the protected
// UPnP / NAT-PMP / PCP service-discovery surface.
//
// P065 production-path contract:
//
//	PUBLIC -> Engine.UPnPProxy -> Agent IPC
//	       -> UPnPHost -> Manager / Runtime -> bounded NAT mapping
//
// The agent owns execution. The public API never opens a
// network listener itself. The runtime is OFF by default
// and exposes typed operations: enable, disable, list
// networks, list mappings, revoke mapping.
package agent

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"

	"github.com/CrazyDuckLab/DuckOS/internal/identity"
	"github.com/CrazyDuckLab/DuckOS/internal/upnp"
)

// UPnPPath is the IPC route prefix for the production
// UPnP host. The public API proxies /api/v1/upnp/* here.
const UPnPPath = "/agent/v1/upnp"

// UPnPSource is the narrow contract the host needs to
// drive the UPnP runtime. *upnp.Runtime (linux) and
// *upnp.RuntimeStub (cross-platform tests) satisfy it.
type UPnPSource interface {
	EnableNetwork(n upnp.Network, iface string) error
	DisableNetwork(ref string)
	Networks() []upnp.Network
	Mappings() []upnp.Mapping
	RevokeMapping(id string)
	Stats() upnp.RuntimeStats
	// TrustIface is the runtime seam the host uses to push
	// a registry-approved LAN into the backend so the next
	// EnableNetwork(iface) finds AllowListen true. The
	// registry has already done the role check upstream.
	TrustIface(iface string)
	// UntrustIface removes iface from the backend's
	// trusted-iface list on the disable path so a revoked
	// ref does not leak into the next Enable cycle.
	UntrustIface(iface string)
}

// UPnPAccess is the narrow contract the host uses to
// enforce authorization.
type UPnPAccess interface {
	Authorize(token string, p identity.Permission) (identity.User, error)
}

// UPnPHost wraps the upnp.Runtime behind a goroutine-safe
// IPC entry point.
type UPnPHost struct {
	mu         sync.Mutex
	Source     UPnPSource
	Access     UPnPAccess
	Registry   *upnp.TrustedLANRegistry
	Enrollment upnp.EnrollmentSnapshot
}

// NewUPnPHost wires the upnp.Runtime into the host.
// registry and enrollment are optional; when registry is
// nil the host skips the role check (preserving the
// cross-platform test stub). Production wiring in
// cmd/duck-agent/main.go always passes both.
func NewUPnPHost(src UPnPSource, acc UPnPAccess, registry *upnp.TrustedLANRegistry, enrollment upnp.EnrollmentSnapshot) *UPnPHost {
	if src == nil {
		src = stubUPnPSource{}
	}
	if acc == nil {
		acc = stubUPnPAccess{}
	}
	return &UPnPHost{Source: src, Access: acc, Registry: registry, Enrollment: enrollment}
}

// NewUPnPHostNoRegistry is the legacy constructor used by
// tests that don't need a real registry. Production wiring
// always uses NewUPnPHost with both arguments populated.
func NewUPnPHostNoRegistry(src UPnPSource, acc UPnPAccess) *UPnPHost {
	return NewUPnPHost(src, acc, nil, upnp.EnrollmentSnapshot{})
}

// ServeHTTP dispatches /agent/v1/upnp/* to the underlying
// upnp.Runtime. Read operations (GET) require Observe.
// Mutating operations (PUT / DELETE on networks or
// mappings) require ManageServices.
//
// Paths:
//
//	GET    /upnp/v1/state                  -> stats + networks + mappings
//	PUT    /upnp/v1/networks/{ref}         -> EnableNetwork
//	DELETE /upnp/v1/networks/{ref}         -> DisableNetwork
//	DELETE /upnp/v1/mappings/{id}          -> RevokeMapping
func (h *UPnPHost) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	src := h.Source
	acc := h.Access
	h.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	tail := r.URL.EscapedPath()
	// The Engine proxy concatenates agent.UPnPPath + "/upnp/v1/"
	// (see internal/engine/engine.go UPnPProxy) so the
	// incoming path can carry both prefixes; strip every
	// recognised one before dispatch. Without this loop
	// (or with a `break` after the first match) state /
	// networks / mappings paths would never resolve and
	// the public API would 404 every UPnP call.
	for _, p := range []string{UPnPPath, "/upnp/v1/"} {
		if len(tail) >= len(p) && tail[:len(p)] == p {
			tail = tail[len(p):]
		}
	}
	// Strip the leading "/" so the verb branches resolve
	// ("state", "networks/<ref>", "mappings/<id>"). The
	// Engine proxy preserves the original /audit/ prefix
	// upstream.
	if strings.HasPrefix(tail, "/") {
		tail = strings.TrimPrefix(tail, "/")
	}
	switch r.Method {
	case http.MethodGet:
		if tail == "state" {
			h.handleState(w, r, src, acc)
			return
		}
		upnpProblem(w, 404, "not_found", "UPnP endpoint not found.")
	case http.MethodPut:
		if strings.HasPrefix(tail, "networks/") {
			ref := strings.TrimPrefix(tail, "networks/")
			h.handleEnable(w, r, ref, src, acc)
			return
		}
		upnpProblem(w, 404, "not_found", "UPnP endpoint not found.")
	case http.MethodDelete:
		if strings.HasPrefix(tail, "networks/") {
			ref := strings.TrimPrefix(tail, "networks/")
			h.handleDisable(w, r, ref, src, acc)
			return
		}
		if strings.HasPrefix(tail, "mappings/") {
			id := strings.TrimPrefix(tail, "mappings/")
			h.handleRevoke(w, r, id, src, acc)
			return
		}
		upnpProblem(w, 404, "not_found", "UPnP endpoint not found.")
	default:
		w.Header().Set("Allow", "GET, PUT, DELETE")
		upnpProblem(w, 405, "method_not_allowed", "UPnP endpoint method is not allowed.")
	}
}

func (h *UPnPHost) authorize(w http.ResponseWriter, r *http.Request, perm identity.Permission) (identity.User, bool) {
	v := r.Header.Get("Authorization")
	if len(v) < 7 || v[:7] != "Bearer " {
		upnpProblem(w, 401, "unauthorized", "Bearer token is required.")
		return identity.User{}, false
	}
	if len(r.Header.Values("Authorization")) != 1 {
		upnpProblem(w, 401, "unauthorized", "Bearer token is required.")
		return identity.User{}, false
	}
	user, err := h.Access.Authorize(v[7:], perm)
	if err != nil {
		upnpProblem(w, 403, "forbidden", "Required permission is missing.")
		return identity.User{}, false
	}
	return user, true
}

func (h *UPnPHost) handleState(w http.ResponseWriter, r *http.Request, src UPnPSource, acc UPnPAccess) {
	if _, ok := h.authorize(w, r, identity.Observe); !ok {
		return
	}
	out := struct {
		Stats    upnp.RuntimeStats `json:"stats"`
		Networks []upnp.Network    `json:"networks"`
		Mappings []upnp.Mapping    `json:"mappings"`
	}{Stats: src.Stats(), Networks: src.Networks(), Mappings: src.Mappings()}
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(out)
}

func (h *UPnPHost) handleEnable(w http.ResponseWriter, r *http.Request, ref string, src UPnPSource, acc UPnPAccess) {
	if _, ok := h.authorize(w, r, identity.ConfigureNetwork); !ok {
		return
	}
	defer r.Body.Close()
	var body struct {
		Network upnp.Network `json:"network"`
		Iface   string       `json:"iface"`
	}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil {
		upnpProblem(w, 400, "invalid_request", "UPnP enable body is not valid JSON.")
		return
	}
	if body.Network.NetworkRef == "" {
		body.Network.NetworkRef = ref
	}
	// Push iface into the backend's trusted list BEFORE the
	// runtime's EnableNetwork check. The runtime's
	// AllowListen(iface, nil) is satisfied only when the
	// backend has already admitted iface. The agent layer is
	// the single seam that synchronises the registry /
	// backend / runtime, so do not reorder these calls.
	if h.Registry != nil {
		if err := h.Registry.Trust(body.Network.NetworkRef, h.Enrollment); err != nil {
			upnpProblem(w, 400, "invalid_request", err.Error())
			return
		}
	}
	if body.Iface != "" {
		src.TrustIface(body.Iface)
	}
	if err := src.EnableNetwork(body.Network, body.Iface); err != nil {
		// Roll back the trust update so a rejected enable
		// does not leak iface into the next enable cycle.
		if body.Iface != "" {
			src.UntrustIface(body.Iface)
		}
		upnpProblem(w, 400, "invalid_request", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *UPnPHost) handleDisable(w http.ResponseWriter, r *http.Request, ref string, src UPnPSource, acc UPnPAccess) {
	if _, ok := h.authorize(w, r, identity.ConfigureNetwork); !ok {
		return
	}
	src.DisableNetwork(ref)
	// Untrust the iface after the runtime's DisableNetwork
	// completes so a re-enable of the same ref cannot bypass
	// the AllowListen check via the previous trust. The
	// handleEnable path will TrustIface once the registry
	// admits the ref again.
	src.UntrustIface(ref)
	if h.Registry != nil {
		_ = h.Registry.Revoke(ref)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *UPnPHost) handleRevoke(w http.ResponseWriter, r *http.Request, id string, src UPnPSource, acc UPnPAccess) {
	if _, ok := h.authorize(w, r, identity.ConfigureNetwork); !ok {
		return
	}
	if id == "" {
		upnpProblem(w, 400, "invalid_request", "Mapping id is required.")
		return
	}
	src.RevokeMapping(id)
	w.WriteHeader(http.StatusNoContent)
}

// upnpProblem writes an RFC-7807 problem document.
func upnpProblem(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]string{"code": code, "message": message},
	})
}

// Compile-time check.
var _ UPnPSource = (*upnp.RuntimeStub)(nil)

// stub implementations.
type stubUPnPSource struct{}

func (stubUPnPSource) EnableNetwork(upnp.Network, string) error {
	return errors.New("upnp: agent is unavailable")
}
func (stubUPnPSource) DisableNetwork(string)               {}
func (stubUPnPSource) Networks() []upnp.Network            { return nil }
func (stubUPnPSource) Mappings() []upnp.Mapping            { return nil }
func (stubUPnPSource) RevokeMapping(string)                {}
func (stubUPnPSource) Stats() upnp.RuntimeStats            { return upnp.RuntimeStats{} }
func (stubUPnPSource) TrustIface(string)                   {}
func (stubUPnPSource) UntrustIface(string)                 {}

type stubUPnPAccess struct{}

func (stubUPnPAccess) Authorize(string, identity.Permission) (identity.User, error) {
	return identity.User{}, errors.New("identity service unavailable")
}