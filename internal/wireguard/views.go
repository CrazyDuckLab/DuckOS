// Package wireguard view models (G24 + G12): public-facing
// projections of the canonical WireGuard configuration. The
// View types strip secret fields (Server.PrivateKeyRef
// internals) and surface only allowlisted metadata. UI, CLI
// and API lists / search / diff / history consume Views;
// secret bytes never leave the process except via the
// explicit Secret Export contract (G13).
package wireguard

import (
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/object"
)

// ServerView is the public projection of a Server. The
// PrivateKeyRef is reduced to a presence flag; the actual
// bytes are only available through Secret Export (G13).
type ServerView struct {
	Ref              object.ObjectRef        `json:"ref"`
	DisplayName      string                  `json:"displayName"`
	Interface        string                  `json:"interface"`
	ListenPort       int                     `json:"listenPort"`
	Addresses        []string                `json:"addresses"`
	DNS              []string                `json:"dns,omitempty"`
	MTU              int                     `json:"mtu,omitempty"`
	PublicKeyDigest  string                  `json:"publicKeyDigest"`
	HasPrivateKey    bool                    `json:"hasPrivateKey"`
	PeerCount        int                     `json:"peerCount"`
	Availability     object.Availability     `json:"availability"`
	SupportedActions object.SupportedActions `json:"supportedActions"`
	Permissions      object.PermissionSet    `json:"permissions"`
	Tags             []string                `json:"tags,omitempty"`
	UpdatedAt        *time.Time              `json:"updatedAt,omitempty"`
}

// PeerView is the public projection of a Peer. The
// PresharedKeyRef is reduced to a presence flag; the public
// key is replaced by its sha256 digest so the operator can
// correlate rows without ever learning the credential.
type PeerView struct {
	Ref                 object.ObjectRef        `json:"ref"`
	ServerRef           object.ObjectRef        `json:"serverRef"`
	DisplayName         string                  `json:"displayName"`
	PublicKeyDigest     string                  `json:"publicKeyDigest"`
	HasPresharedKey     bool                    `json:"hasPresharedKey"`
	AllowedIPs          []string                `json:"allowedIPs"`
	Endpoint            string                  `json:"endpoint,omitempty"`
	PersistentKeepalive int                     `json:"persistentKeepalive,omitempty"`
	ExpiresAt           string                  `json:"expiresAt,omitempty"`
	Intent              string                  `json:"intent"`
	Enabled             bool                    `json:"enabled"`
	Availability        object.Availability     `json:"availability"`
	SupportedActions    object.SupportedActions `json:"supportedActions"`
	Permissions         object.PermissionSet    `json:"permissions"`
	Tags                []string                `json:"tags,omitempty"`
	UpdatedAt           *time.Time              `json:"updatedAt,omitempty"`
}

// ViewServer projects a Server into a ServerView. The
// caller passes the boolean permissions they have on this
// row so the renderer does not need to recompute RBAC.
func ViewServer(s Server, perms object.PermissionSet) ServerView {
	t := s.UpdatedAt
	return ServerView{
		Ref:              object.ObjectRef{Kind: "wireguard.server", ID: s.ID, Revision: digest(s.PublicKey)},
		DisplayName:      s.Name,
		Interface:        s.Interface,
		ListenPort:       s.ListenPort,
		Addresses:        append([]string(nil), s.Addresses...),
		DNS:              append([]string(nil), s.DNS...),
		MTU:              s.MTU,
		PublicKeyDigest:  digest(s.PublicKey),
		HasPrivateKey:    s.PrivateKeyRef.VaultID != "",
		PeerCount:        len(s.Peers),
		Availability:     object.Available,
		SupportedActions: object.SupportedActions{"show", "edit", "delete", "rotate", "add-peer", "remove-peer"},
		Permissions:      perms,
		UpdatedAt:        &t,
	}
}

// ViewPeer projects a Peer into a PeerView.
func ViewPeer(p Peer, perms object.PermissionSet) PeerView {
	t := p.UpdatedAt
	return PeerView{
		Ref:                 object.ObjectRef{Kind: "wireguard.peer", ID: p.ID, Revision: digest(p.PublicKey)},
		ServerRef:           object.ObjectRef{Kind: "wireguard.server", ID: p.ServerID},
		DisplayName:         p.Name,
		PublicKeyDigest:     digest(p.PublicKey),
		HasPresharedKey:     p.PresharedKeyRef != nil,
		AllowedIPs:          append([]string(nil), p.AllowedIPs...),
		Endpoint:            p.Endpoint,
		PersistentKeepalive: p.PersistentKeepalive,
		ExpiresAt:           p.ExpiresAt,
		Intent:              p.Intent,
		Enabled:             p.Enabled,
		Availability:        object.Available,
		SupportedActions:    object.SupportedActions{"show", "edit", "delete", "disable", "revoke", "rotate"},
		Permissions:         perms,
		UpdatedAt:           &t,
	}
}

// digest returns the sha256 hex of the supplied bytes; it
// is used to replace the public key in views so the
// operator can correlate rows without ever learning the
// credential.
func digest(s string) string {
	if s == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:8])
}
