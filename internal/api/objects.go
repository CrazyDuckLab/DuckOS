// Package api generic object list endpoint (G02 + G06
// wiring). Every supported kind uses the same URL,
// the same Collection envelope, and the same bounded
// query parameters (limit, after).
package api

import (
	"net/http"
	"strings"

	"github.com/CrazyDuckLab/DuckOS/internal/identity"
	"github.com/CrazyDuckLab/DuckOS/internal/object"
)

// handleObjects dispatches GET /api/v1/objects and
// GET /api/v1/objects/{kind} to the Engine's typed
// ObjectList entry point. Unknown kinds return a
// Collection envelope with an explicit "engine_unavailable"
// issue so the UI / CLI can render the gap honestly.
//
// Authentication is enforced here directly (rather than
// delegating to serveAPI) because this route is matched
// before serveAPI in the outer dispatcher.
//
// Per-row RBAC: when the identity service is enrolled AND
// a Policy overlay is wired, the handler enforces the
// overlay on every returned ViewObject. Objects whose
// (kind, id) is denied are filtered out of the public
// Items slice, and a per-item redacted summary is added to
// the envelope's Issues so the UI can render an honest
// "denied" badge. Deny precedence is enforced per the
// identity.Policy contract: any applicable deny wins over
// any applicable allow.
func (h *Handler) handleObjects(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET, HEAD")
		problem(w, r, 405, errorCategoryFromCode("method_not_allowed"), "method_not_allowed", "Object list is read-only.")
		return
	}
	if !h.authorizeManagement(w, r, identity.Observe) {
		return
	}
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if r.ContentLength != 0 || len(r.TransferEncoding) != 0 {
		problem(w, r, 400, errorCategoryFromCode("invalid_request"), "invalid_request", "Request bodies are not accepted on the list endpoint.")
		return
	}
	kind := strings.TrimPrefix(r.URL.Path, "/api/v1/objects/")
	if kind == "" || strings.Contains(kind, "/") {
		problem(w, r, 404, errorCategoryFromCode("not_found"), "not_found", "Object list requires a kind: /api/v1/objects/{kind}")
		return
	}
	if h.engine == nil {
		problem(w, r, 503, errorCategoryFromCode("agent_unavailable"), "agent_unavailable", "Engine is unavailable.")
		return
	}
	// Role-level RBAC first: when the identity service is
	// enrolled, the caller's role must be allowed to observe
	// at all. The overlay then refines on a per-object basis.
	var role identity.Role
	if h.identity != nil && h.identity.Enrolled() {
		if _, err := h.identity.Authorize(token, identity.Observe); err != nil {
			problem(w, r, 403, errorCategoryFromCode("forbidden"), "forbidden", "Caller is not allowed to observe this object.")
			return
		}
		role, _ = h.identity.Role(token)
	} else if h.policy != nil {
		// The overlay is wired but the identity service is
		// not yet enrolled. Fail closed: refuse rather than
		// silently bypass the overlay.
		problem(w, r, 503, errorCategoryFromCode("enrollment_required"), "enrollment_required", "Per-object policy is wired; local Owner must be enrolled before observe.")
		return
	}
	if h.policy != nil && role == "" {
		// The overlay is wired but no role can be resolved
		// for the bearer token. Refuse rather than bypass.
		problem(w, r, 403, errorCategoryFromCode("forbidden"), "forbidden", "Caller role could not be resolved; per-object policy requires a known role.")
		return
	}
	_ = role // role is non-empty only when policy is wired
	params := ParseCollectionParams(r.URL.RawQuery)
	col, err := h.engine.ObjectList(r.Context(), kind, object.CollectionParams{Limit: params.Limit, After: params.After})
	if err != nil {
		writeEngineError(w, r, err)
		return
	}
	// Per-row filter. The Engine returns the raw list;
	// the API is the only place that can apply the
	// per-identity RBAC contract end-to-end.
	if h.policy != nil && role != "" {
		col = h.policy.FilterCollection(role, kind, identity.Observe, col)
	}
	writeJSON(w, 200, col)
}

// objectListKinds returns the canonical list of supported
// kinds. The CLI uses this to render honest help text; the
// API uses it to validate /api/v1/objects/ requests.
func objectListKinds() []string { return supportedObjectKinds }

// supportedObjectKinds mirrors objectlisters.SupportedKinds.
// Duplicated here to avoid an import cycle between
// internal/api and internal/objectlisters.
var supportedObjectKinds = []string{
	"network.interface",
	"network.route",
	"network.firewall.rule",
	"network.firewall.set",
	"network.lease",
	"network.address",
	"wireguard.server",
	"wireguard.peer",
	"xray.server",
	"xray.user",
	"xray.profile",
	"xray.subscription",
	"qos.class",
	"dns.zone",
	"dhcp.subnet",
	"bgp.peer",
	"ospf.area",
	"isis.area",
	"route.map",
}

// IsObjectKind reports whether the supplied kind is one of
// the canonical DuckOS object kinds. The check is
// case-sensitive; kinds use a dot-lower-cased namespace.
func IsObjectKind(kind string) bool {
	for _, k := range supportedObjectKinds {
		if k == kind {
			return true
		}
	}
	return false
}

// touch unused import.
var _ = object.MaxCollectionItems
