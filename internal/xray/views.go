// Package xray view models (G24): public-facing projections
// of the canonical configuration model. The View types strip
// secret fields (User.UUID, User.Password, User.Credential,
// Profile credential refs) and surface only allowlisted
// metadata. UI, CLI, and API lists/diff/search/history must
// never embed a raw internal model; they consume Views. The
// Secret Export contract (separate authorized, no-cache
// endpoint) is the only place where raw secrets leave the
// process.
package xray

import (
	"crypto/sha256"
	"encoding/hex"
)

// UserView is the public-facing projection of a User.
// It contains the canonical identity but no secret
// material. The UUID is replaced by its sha256 fingerprint
// so the operator can correlate a row across the system
// without ever learning the credential.
type UserView struct {
	Ref         ObjectRef       `json:"ref"`
	ServerRef   ObjectRef       `json:"serverRef"`
	DisplayName string          `json:"displayName"`
	Email       string          `json:"email,omitempty"`
	Enabled     bool            `json:"enabled"`
	ExpiresAt   string          `json:"expiresAt,omitempty"`
	Tags        []string        `json:"tags,omitempty"`
	UUIDDigest  string          `json:"uuidDigest,omitempty"`
	HasSecret   bool            `json:"hasSecret"`
	UpdatedAt   string          `json:"updatedAt,omitempty"`
	Permissions PermissionsView `json:"permissions"`
}

// ObjectRef is the canonical pointer at an object: kind +
// id + revision. The revision is the canonical short-digest
// of the underlying secret credential; the secret itself
// is never serialised.
type ObjectRef struct {
	Kind     string `json:"kind"`
	ID       string `json:"id"`
	Revision string `json:"revision,omitempty"`
}

// PermissionsView mirrors the per-row permissions the
// caller has on this object: observe (read), configure
// (mutate), exportSecret (reveal credentials). Each is a
// boolean so the renderer does not need to know the
// caller's role to render the page.
type PermissionsView struct {
	Observe      bool `json:"observe"`
	Configure    bool `json:"configure"`
	ExportSecret bool `json:"exportSecret"`
}

// ServerView is the public projection of a Server. Security
// settings (Reality/TLS key references) are kept as
// presence-only — the row reveals whether the server has a
// vault-managed key, never the key itself.
type ServerView struct {
	Ref         ObjectRef       `json:"ref"`
	DisplayName string          `json:"displayName"`
	Protocol    string          `json:"protocol"`
	Transport   string          `json:"transport"`
	Security    string          `json:"security"`
	Listen      string          `json:"listen"`
	Enabled     bool            `json:"enabled"`
	HasReality  bool            `json:"hasReality"`
	HasTLS      bool            `json:"hasTLS"`
	Tags        []string        `json:"tags,omitempty"`
	Permissions PermissionsView `json:"permissions"`
}

// ProfileView is the public projection of a Profile.
// Credential references are reduced to a presence flag.
type ProfileView struct {
	Ref         ObjectRef       `json:"ref"`
	DisplayName string          `json:"displayName"`
	UserRef     ObjectRef       `json:"userRef"`
	Intent      string          `json:"intent"`
	Selection   []string        `json:"selection,omitempty"`
	Enabled     bool            `json:"enabled"`
	ExpiresAt   string          `json:"expiresAt,omitempty"`
	Tags        []string        `json:"tags,omitempty"`
	Permissions PermissionsView `json:"permissions"`
}

// SubscriptionView is the public projection of a
// Subscription. It never carries a delivery URL or token.
type SubscriptionView struct {
	Ref         ObjectRef       `json:"ref"`
	DisplayName string          `json:"displayName"`
	ProfileRefs []ObjectRef     `json:"profileRefs"`
	Enabled     bool            `json:"enabled"`
	ExpiresAt   string          `json:"expiresAt,omitempty"`
	Tags        []string        `json:"tags,omitempty"`
	Permissions PermissionsView `json:"permissions"`
}

// ViewUser projects a User into a UserView. The caller
// passes the boolean permissions they have on this row so
// the renderer does not need to recompute RBAC.
func ViewUser(u User, perms PermissionsView) UserView {
	v := UserView{
		Ref:         ObjectRef{Kind: "xray.user", ID: u.ID, Revision: uuidDigest(u.UUID)},
		ServerRef:   ObjectRef{Kind: "xray.server", ID: u.ServerID},
		DisplayName: u.Name,
		Email:       u.Email,
		Enabled:     u.Enabled,
		ExpiresAt:   u.ExpiresAt,
		Tags:        append([]string(nil), u.Tags...),
		HasSecret:   u.UUID != "" || u.Password != "" || u.Credential != nil,
		Permissions: perms,
	}
	if u.UUID != "" {
		v.UUIDDigest = uuidDigest(u.UUID)
	}
	return v
}

// uuidDigest returns the short hex digest of the supplied
// UUID. The same value is used for both the public
// UUIDDigest field and the canonical ObjectRef.Revision so
// the operator can correlate rows without ever learning
// the credential.
func uuidDigest(uuid string) string {
	if uuid == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(uuid))
	return hex.EncodeToString(sum[:8])
}

// ViewServer projects a Server into a ServerView.
func ViewServer(s Server, perms PermissionsView) ServerView {
	return ServerView{
		Ref:         ObjectRef{Kind: "xray.server", ID: s.ID},
		DisplayName: s.Name,
		Protocol:    string(s.Protocol),
		Transport:   string(s.Transport),
		Security:    string(s.Security),
		Listen:      s.Listen,
		Enabled:     s.Enabled,
		HasReality:  s.RealityKeyRef != nil && s.ShortIDRef != nil,
		HasTLS:      s.TLSCertRef != nil && s.TLSKeyRef != nil,
		Tags:        append([]string(nil), s.Tags...),
		Permissions: perms,
	}
}

// ViewProfile projects a Profile into a ProfileView.
func ViewProfile(p Profile, perms PermissionsView) ProfileView {
	return ProfileView{
		Ref:         ObjectRef{Kind: "xray.profile", ID: p.ID},
		DisplayName: p.Name,
		UserRef:     ObjectRef{Kind: "xray.user", ID: p.UserID},
		Intent:      p.Intent,
		Selection:   append([]string(nil), p.Selection...),
		Enabled:     p.Enabled,
		ExpiresAt:   p.ExpiresAt,
		Tags:        append([]string(nil), p.Tags...),
		Permissions: perms,
	}
}

// ViewSubscription projects a Subscription into a
// SubscriptionView.
func ViewSubscription(s Subscription, perms PermissionsView) SubscriptionView {
	refs := make([]ObjectRef, 0, len(s.ProfileIDs))
	for _, id := range s.ProfileIDs {
		refs = append(refs, ObjectRef{Kind: "xray.profile", ID: id})
	}
	return SubscriptionView{
		Ref:         ObjectRef{Kind: "xray.subscription", ID: s.ID},
		DisplayName: s.Name,
		ProfileRefs: refs,
		Enabled:     s.Enabled,
		ExpiresAt:   s.ExpiresAt,
		Tags:        append([]string(nil), s.Tags...),
		Permissions: perms,
	}
}

// HasSecretField reports whether the User carries any
// secret material. UI lists can use this to surface a
// "configured" badge without ever revealing the credential.
func (u User) HasSecretField() bool {
	return u.UUID != "" || u.Password != "" || u.Credential != nil
}
