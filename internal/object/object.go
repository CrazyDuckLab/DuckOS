// Package object is the canonical object identity and
// collection envelope for DuckOS V1 (G02).
//
// Every configurable thing in the system — a network
// interface, a route, a firewall rule, a WireGuard peer, an
// Xray user — carries the same triple: kind, id, revision.
// The ObjectRef type binds that triple to the public
// surface. The Collection envelope bounds every list with
// the same shape so UI and CLI can treat the result
// uniformly.
package object

import (
	"crypto/rand"
	"encoding/hex"
	"time"
)

// ObjectRef is the canonical pointer at an object. The
// Kind is the public kind (e.g. "network.interface",
// "firewall.rule"), the ID is the canonical id assigned at
// creation, and the Revision is the latest committed
// revision id of the object. A zero Revision means the
// caller did not supply it (acceptable for create
// operations and for newly instantiated objects that have
// not yet been committed).
type ObjectRef struct {
	Kind     string `json:"kind"`
	ID       string `json:"id"`
	Revision string `json:"revision,omitempty"`
}

// IsZero reports whether the ref is empty.
func (r ObjectRef) IsZero() bool {
	return r.Kind == "" && r.ID == "" && r.Revision == ""
}

// Equal reports whether two refs describe the same
// canonical object at the same revision.
func (r ObjectRef) Equal(other ObjectRef) bool {
	return r.Kind == other.Kind && r.ID == other.ID && r.Revision == other.Revision
}

// Availability describes whether a feature or operation is
// reachable on this node. Categories are language-neutral
// and stable; UI / CLI localise by category.
type Availability string

const (
	// Available reports the operation is reachable and
	// runnable. The witness field may carry a per-source
	// freshness note (e.g. "agent:1.4.2").
	Available Availability = "available"
	// Experimental reports the operation is reachable but
	// the operator must opt in; UX should warn.
	Experimental Availability = "experimental"
	// HardwarePending reports the operation cannot run
	// because required hardware is missing.
	HardwarePending Availability = "hardware-pending"
	// CredentialPending reports the operation cannot run
	// because a credential is missing.
	CredentialPending Availability = "credential-pending"
	// Unsupported reports the operation is not implemented.
	Unsupported Availability = "unsupported"
	// Unknown reports the system has no positive evidence
	// either way; UI must not invent availability.
	Unknown Availability = "unknown"
)

// SupportedActions is the list of action verbs a caller
// may invoke on a given object. The set is computed by the
// Engine per-kind; UI and CLI render it as a literal list.
type SupportedActions []string

// Includes reports whether the action set contains a given
// verb.
func (s SupportedActions) Includes(action string) bool {
	for _, a := range s {
		if a == action {
			return true
		}
	}
	return false
}

// OwnerRef points at the generator / owner of an object.
// Generated objects (e.g. dependency-derived entries) carry
// an OwnerRef so the operator can trace the chain.
type OwnerRef struct {
	Kind string `json:"kind,omitempty"`
	ID   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
}

// IsZero reports whether the owner ref is empty.
func (o OwnerRef) IsZero() bool { return o.Kind == "" && o.ID == "" }

// PermissionSet is the language-neutral projection of the
// caller's permissions on this object. Each boolean is
// computed by the Engine; UI and CLI never have to know
// the caller's role.
type PermissionSet struct {
	Observe      bool `json:"observe"`
	Configure    bool `json:"configure"`
	Action       bool `json:"action"`
	ExportSecret bool `json:"exportSecret"`
	Delete       bool `json:"delete,omitempty"`
}

// Collection is the public envelope for every list. The
// Items slice is the loaded page; Cursor is the opaque
// pagination handle; Total is the size of the underlying
// set when known or null otherwise; Complete is false when
// the caller may have received a partial view.
//
// The SnapshotID is a server-side version stamp that the
// caller can echo back on the next request for diff-style
// pagination.
type Collection struct {
	SnapshotID string       `json:"snapshotId"`
	Cursor     string       `json:"cursor,omitempty"`
	Total      *int         `json:"total"`
	Complete   bool         `json:"complete"`
	Issues     []Issue      `json:"issues,omitempty"`
	Items      []ViewObject `json:"items"`
}

// Issue is a per-source diagnostic that the UI may surface
// as a non-fatal warning row. ReasonID is a stable code;
// Source identifies the producer (engine, agent, store
// name). Detail is an optional operator-facing hint —
// used by the per-row RBAC filter to record which object
// id was redacted, and by the lister issue code to carry
// short notes. Detail is never authoritative and never
// carries secrets.
type Issue struct {
	Source   string `json:"source"`
	ReasonID string `json:"reasonId"`
	Detail   string `json:"detail,omitempty"`
}

// ViewObject is the canonical public projection of one
// object. It carries the ObjectRef, the desired
// configuration, the observed runtime evidence, the
// availability, the owner / provenance, the supported
// actions, and the caller's permission set.
type ViewObject struct {
	Ref              ObjectRef        `json:"ref"`
	DisplayName      string           `json:"displayName"`
	Desired          any              `json:"desired"`
	Observed         any              `json:"observed,omitempty"`
	Availability     Availability     `json:"availability"`
	Owner            OwnerRef         `json:"owner,omitempty"`
	SupportedActions SupportedActions `json:"supportedActions"`
	Permissions      PermissionSet    `json:"permissions"`
	Tags             []string         `json:"tags,omitempty"`
	UpdatedAt        *time.Time       `json:"updatedAt,omitempty"`
}

// CollectionParams is the parsed representation of the
// pagination query string. Limit is bounded by the caller;
// After is the opaque cursor.
type CollectionParams struct {
	Limit int    `json:"limit"`
	After string `json:"after,omitempty"`
}

// MaxCollectionItems is the largest page a single lister
// may return in one call. Listers MUST clamp their result
// to this bound.
const MaxCollectionItems = 4096

// NewID generates a fresh canonical 16-byte hex id. The
// caller is responsible for assigning it to a unique
// object within its kind.
func NewID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "00000000000000000000000000000000"
	}
	return hex.EncodeToString(b[:])
}
