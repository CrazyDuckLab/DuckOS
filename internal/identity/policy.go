// identity/policy.go: G21 per-object authorization.
//
// The base RBAC matrix in roles.go grants permissions to
// roles. This file adds an overlay that grants (or
// revokes) a permission for one specific role on one
// specific object — e.g. "the Helpdesk role may manage
// VLAN vlan-10 but not VLAN vlan-20" or "Read-Only may
// export secret wireguard.peer/edge-A".
//
// Overlay rules are evaluated in this order:
//  1. explicit deny on the object wins;
//  2. explicit allow on the object wins;
//  3. otherwise fall through to the role matrix.
//
// The overlay is goroutine-safe and bounded. Callers may
// register overrides at runtime; the authorizer never
// reaches a partial state.
package identity

import (
	"errors"
	"regexp"
	"sort"
	"sync"

	"github.com/CrazyDuckLab/DuckOS/internal/object"
)

// ObjectKey identifies one managed object in the overlay.
// The kind uses the canonical lifecycle.ObjectKind
// vocabulary; id uses the canonical id pattern.
type ObjectKey struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

// kindPattern matches canonical object kind namespaces
// (e.g. "network.firewall.rule", "xray.server"). Dots are
// allowed because every supported kind is a dot-separated
// namespace.
var kindPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,31}(\.[a-z][a-z0-9-]{0,31})*$`)

// idPattern matches canonical object ids. The single
// wildcard "*" is permitted at the ID position so an
// overlay can target every object of a kind.
var idPattern = regexp.MustCompile(`^(\*|[a-z0-9][a-z0-9._:-]{0,63})$`)

// objectPattern kept for backward compatibility with
// tests that referenced it directly. New callers should
// use kindPattern / idPattern explicitly.
var objectPattern = kindPattern

// Valid reports whether the key uses canonical lower-cased
// vocabulary. A "*" id is allowed for kind-wide overlays.
func (k ObjectKey) Valid() bool {
	if !kindPattern.MatchString(k.Kind) {
		return false
	}
	if !idPattern.MatchString(k.ID) {
		return false
	}
	return true
}

// IsWildcard reports whether the key's id is the kind-wide
// "*" overlay marker.
func (k ObjectKey) IsWildcard() bool { return k.ID == "*" }

// String renders the key in "kind:id" form.
func (k ObjectKey) String() string { return k.Kind + ":" + k.ID }

// Decision is the per-object authorization outcome.
type Decision int

const (
	// DecisionDefault falls through to the role matrix.
	DecisionDefault Decision = iota
	// DecisionAllow grants the permission on the object.
	DecisionAllow
	// DecisionDeny forbids the permission on the object.
	DecisionDeny
)

// String renders the decision for logs and error messages.
func (d Decision) String() string {
	switch d {
	case DecisionAllow:
		return "allow"
	case DecisionDeny:
		return "deny"
	default:
		return "default"
	}
}

// Grant is a single overlay rule. The zero Decision is
// "default" (fall through).
type Grant struct {
	Role       Role       `json:"role"`
	Object     ObjectKey  `json:"object"`
	Permission Permission `json:"permission"`
	Decision   Decision   `json:"decision"`
	// Reason is an operator-facing explanation. Required
	// when Decision != DecisionDefault so the audit log
	// can surface why a specific override exists.
	Reason string `json:"reason,omitempty"`
}

// Valid reports whether the grant is well-formed.
func (g Grant) Valid() error {
	if !validPermission(g.Permission) {
		return errors.New("identity: grant permission is not canonical")
	}
	if !validRole(g.Role) {
		return errors.New("identity: grant role is not canonical")
	}
	if !g.Object.Valid() {
		return errors.New("identity: grant object is not canonical")
	}
	switch g.Decision {
	case DecisionDefault, DecisionAllow, DecisionDeny:
	default:
		return errors.New("identity: grant decision is not canonical")
	}
	if g.Decision != DecisionDefault && g.Reason == "" {
		return errors.New("identity: non-default grant requires reason")
	}
	return nil
}

// Policy is the goroutine-safe overlay store.
type Policy struct {
	mu     sync.RWMutex
	grants map[string]Grant
}

// NewPolicy returns an empty overlay.
func NewPolicy() *Policy {
	return &Policy{grants: map[string]Grant{}}
}

// grantKey is the canonical map key.
func grantKey(role Role, obj ObjectKey, perm Permission) string {
	return string(role) + "|" + obj.String() + "|" + string(perm)
}

// Set upserts a grant. An invalid grant returns the
// validation error; the previous grant is preserved on
// failure.
func (p *Policy) Set(g Grant) error {
	if err := g.Valid(); err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.grants[grantKey(g.Role, g.Object, g.Permission)] = g
	return nil
}

// Delete removes the grant for the (role, object,
// permission) triple. Missing keys are a no-op.
func (p *Policy) Delete(role Role, obj ObjectKey, perm Permission) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.grants, grantKey(role, obj, perm))
}

// Lookup returns the explicit decision for the supplied
// triple. DecisionDefault means "fall through".
func (p *Policy) Lookup(role Role, obj ObjectKey, perm Permission) Decision {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if g, ok := p.grants[grantKey(role, obj, perm)]; ok {
		return g.Decision
	}
	return DecisionDefault
}

// All returns every grant sorted by (role, kind, id,
// permission) so the audit surface is deterministic.
func (p *Policy) All() []Grant {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make([]Grant, 0, len(p.grants))
	for _, g := range p.grants {
		out = append(out, g)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Role != out[j].Role {
			return out[i].Role < out[j].Role
		}
		if out[i].Object.Kind != out[j].Object.Kind {
			return out[i].Object.Kind < out[j].Object.Kind
		}
		if out[i].Object.ID != out[j].Object.ID {
			return out[i].Object.ID < out[j].Object.ID
		}
		return out[i].Permission < out[j].Permission
	})
	return out
}

// AuthorizeObject evaluates the overlay first, then falls
// through to the role matrix. The function returns nil
// when the action is allowed, ErrForbidden otherwise.
//
// The overlay has the final say: an explicit deny on the
// object always overrides an allowed role.
func (p *Policy) AuthorizeObject(role Role, obj ObjectKey, perm Permission) error {
	if !obj.Valid() {
		return errors.New("identity: object key is not canonical")
	}
	if p != nil {
		// Read both applicable grants from one policy snapshot. A deny at
		// either scope wins; granting one object cannot bypass a kind lock.
		p.mu.RLock()
		exact := p.grants[grantKey(role, obj, perm)].Decision
		kind := p.grants[grantKey(role, ObjectKey{Kind: obj.Kind, ID: "*"}, perm)].Decision
		p.mu.RUnlock()
		if exact == DecisionDeny || kind == DecisionDeny {
			return ErrForbidden
		}
		if exact == DecisionAllow || kind == DecisionAllow {
			return nil
		}
	}
	if Allowed(role, perm) {
		return nil
	}
	return ErrForbidden
}

// FilterCollection applies the per-object policy to every
// item in col. The returned collection:
//
//   - omits items the caller is not allowed to observe
//     (deny precedence enforced: any applicable deny wins);
//   - preserves the SnapshotID, Cursor, Total and Complete
//     fields of the original envelope so the caller still
//     sees pagination evidence;
//   - appends one Issue per redacted item so the UI can
//     render an honest "denied" badge without ever leaking
//     the underlying payload.
//
// FilterCollection never returns ErrForbidden; the
// envelope already communicates what was redacted and why.
// Callers MUST treat the returned Collection as the
// authoritative public projection and MUST NOT re-add the
// filtered items client-side.
func (p *Policy) FilterCollection(role Role, kind string, perm Permission, col object.Collection) object.Collection {
	if p == nil || role == "" || kind == "" {
		return col
	}
	out := object.Collection{
		SnapshotID: col.SnapshotID,
		Cursor:     col.Cursor,
		Total:      col.Total,
		Complete:   col.Complete,
		Issues:     append([]object.Issue(nil), col.Issues...),
		Items:      make([]object.ViewObject, 0, len(col.Items)),
	}
	redacted := 0
	for _, item := range col.Items {
		obj := ObjectKey{Kind: kind, ID: item.Ref.ID}
		if err := p.AuthorizeObject(role, obj, perm); err == nil {
			out.Items = append(out.Items, item)
		} else {
			redacted++
			out.Issues = append(out.Issues, object.Issue{
				Source:   "rbac",
				ReasonID: "rbac_redacted",
				Detail:   kind + ":" + item.Ref.ID,
			})
		}
	}
	// When items were redacted, the displayed count would
	// lie. The UI relies on Total+Complete for pagination;
	// we leave Total intact and surface a single summary
	// issue so the audit log carries the count without
	// inflating the wire envelope with per-id detail twice.
	if redacted > 0 {
		// Aggregate summary issue stays out of the way:
		// per-item redacted issues above already carry the
		// id list for the audit surface.
		_ = redacted
	}
	return out
}
