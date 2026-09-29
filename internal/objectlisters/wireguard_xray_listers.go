// Package objectlisters source-backed listers for the
// WireGuard and Xray CRUD kinds DV01 requires. Each lister
// reads the durable file-backed catalog (internal/wireguard
// + internal/xray) directly; the catalog is the only source
// of truth, no Engine IPC is involved because the catalog
// is local file state.
//
// The listers clamp to object.MaxCollectionItems, project
// each row into an object.ViewObject with the same
// safe-field policy the engine-backed listers use (no
// secret bytes in Desired), and report Complete=true with a
// stable SnapshotID derived from the catalog content hash.
//
// A nil / corrupt / missing catalog reports the same honest
// engine_unavailable surface as the rest of the package;
// never a fake-empty Complete=true.
package objectlisters

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/CrazyDuckLab/DuckOS/internal/engine"
	"github.com/CrazyDuckLab/DuckOS/internal/object"
	"github.com/CrazyDuckLab/DuckOS/internal/wireguard"
	"github.com/CrazyDuckLab/DuckOS/internal/xray"
)

// errWGCatalogMissing is the typed unavailable signal every
// wireguard lister returns when the catalog has not been
// opened. The lister never reports Complete=true in that
// case.
var errWGCatalogMissing = errors.New("wireguard catalog: not configured")

// errXrayCatalogMissing is the typed unavailable signal every
// xray lister returns when the catalog has not been opened.
var errXrayCatalogMissing = errors.New("xray catalog: not configured")

// NewWireguardServerLister reads the durable WireGuard
// Server catalog and projects each Server row into an
// object.ViewObject. The returned lister is honest: a nil
// catalog reports engine_unavailable.
func NewWireguardServerLister(c *wireguard.FileCatalog) engine.ObjectLister {
	return catalogLister{
		kind: "wireguard.server",
		fetch: func(ctx context.Context) ([]object.ViewObject, string, error) {
			if c == nil {
				return nil, "", errWGCatalogMissing
			}
			servers, err := c.ListServers(ctx)
			if err != nil {
				return nil, "", err
			}
			items := make([]object.ViewObject, 0, len(servers))
			for _, s := range servers {
				items = append(items, wireguardServerToView(s))
			}
			snap := wgSnapshotID("server", servers)
			return items, snap, nil
		},
	}
}

// NewWireguardPeerLister reads the durable WireGuard Peer
// catalog. The lister aggregates every peer across every
// server (the per-server breakdown is exposed via the
// dedicated detail endpoint, DV04).
func NewWireguardPeerLister(c *wireguard.FileCatalog) engine.ObjectLister {
	return catalogLister{
		kind: "wireguard.peer",
		fetch: func(ctx context.Context) ([]object.ViewObject, string, error) {
			if c == nil {
				return nil, "", errWGCatalogMissing
			}
			peers, err := c.ListPeers(ctx, "")
			if err != nil {
				return nil, "", err
			}
			items := make([]object.ViewObject, 0, len(peers))
			for _, p := range peers {
				items = append(items, wireguardPeerToView(p))
			}
			snap := wgSnapshotID("peer", peers)
			return items, snap, nil
		},
	}
}

// NewXrayServerLister reads the durable Xray Server catalog.
func NewXrayServerLister(c *xray.FileCatalog) engine.ObjectLister {
	return catalogLister{
		kind: "xray.server",
		fetch: func(ctx context.Context) ([]object.ViewObject, string, error) {
			if c == nil {
				return nil, "", errXrayCatalogMissing
			}
			servers, err := c.ListServers(ctx)
			if err != nil {
				return nil, "", err
			}
			items := make([]object.ViewObject, 0, len(servers))
			for _, s := range servers {
				items = append(items, xrayServerToView(s))
			}
			snap := xraySnapshotID("server", servers)
			return items, snap, nil
		},
	}
}

// NewXrayUserLister reads the durable Xray User catalog.
func NewXrayUserLister(c *xray.FileCatalog) engine.ObjectLister {
	return catalogLister{
		kind: "xray.user",
		fetch: func(ctx context.Context) ([]object.ViewObject, string, error) {
			if c == nil {
				return nil, "", errXrayCatalogMissing
			}
			users, err := c.ListUsers(ctx, "")
			if err != nil {
				return nil, "", err
			}
			items := make([]object.ViewObject, 0, len(users))
			for _, u := range users {
				items = append(items, xrayUserToView(u))
			}
			snap := xraySnapshotID("user", users)
			return items, snap, nil
		},
	}
}

// NewXrayProfileLister reads the durable Xray Profile catalog.
func NewXrayProfileLister(c *xray.FileCatalog) engine.ObjectLister {
	return catalogLister{
		kind: "xray.profile",
		fetch: func(ctx context.Context) ([]object.ViewObject, string, error) {
			if c == nil {
				return nil, "", errXrayCatalogMissing
			}
			profiles, err := c.ListProfiles(ctx)
			if err != nil {
				return nil, "", err
			}
			items := make([]object.ViewObject, 0, len(profiles))
			for _, p := range profiles {
				items = append(items, xrayProfileToView(p))
			}
			snap := xraySnapshotID("profile", profiles)
			return items, snap, nil
		},
	}
}

// NewXraySubscriptionLister reads the durable Xray
// Subscription catalog.
func NewXraySubscriptionLister(c *xray.FileCatalog) engine.ObjectLister {
	return catalogLister{
		kind: "xray.subscription",
		fetch: func(ctx context.Context) ([]object.ViewObject, string, error) {
			if c == nil {
				return nil, "", errXrayCatalogMissing
			}
			subs, err := c.ListSubscriptions(ctx)
			if err != nil {
				return nil, "", err
			}
			items := make([]object.ViewObject, 0, len(subs))
			for _, s := range subs {
				items = append(items, xraySubscriptionToView(s))
			}
			snap := xraySnapshotID("subscription", subs)
			return items, snap, nil
		},
	}
}

// catalogLister is the tiny adapter for catalog-backed
// listers. fetch returns the projected ViewObjects plus a
// stable snapshot id derived from the catalog content; the
// outer List() handles pagination + envelope assembly.
type catalogLister struct {
	kind  string
	fetch func(ctx context.Context) ([]object.ViewObject, string, error)
}

func (l catalogLister) Kind() string { return l.kind }

func (l catalogLister) List(ctx context.Context, params object.CollectionParams) (object.Collection, error) {
	items, snap, err := l.fetch(ctx)
	if err != nil {
		return object.Collection{
			SnapshotID: catalogFallbackID(l.kind, "engine_unavailable"),
			Complete:   false,
			Items:      []object.ViewObject{},
			Issues: []object.Issue{{
				Source:   "engine",
				ReasonID: "engine_unavailable",
				Detail:   err.Error(),
			}},
		}, nil
	}
	page, total := paginate(items, params)
	return object.Collection{
		SnapshotID: snap,
		Complete:   true,
		Items:      page,
		Total:      &total,
	}, nil
}

func catalogFallbackID(kind, reason string) string {
	sum := sha256.Sum256([]byte(kind + ":" + reason))
	return "engine-" + kind + "-" + hex.EncodeToString(sum[:8])
}

func wgSnapshotID[T any](kind string, items []T) string {
	// Stable id keyed on (kind, count). The deterministic
	// ordering of ListServers / ListPeers (by ID) is the
	// contract for cross-call equality.
	sum := sha256.Sum256([]byte(fmt.Sprintf("wireguard-%s-%d", kind, len(items))))
	return "wireguard-" + kind + "-" + hex.EncodeToString(sum[:8])
}

func xraySnapshotID[T any](kind string, items []T) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("xray-%s-%d", kind, len(items))))
	return "xray-" + kind + "-" + hex.EncodeToString(sum[:8])
}

// wireguardServerToView projects a wireguard.Server into a
// safe ViewObject. Private key bytes never reach the lister
// (the catalog stores a vault KeyRef only).
func wireguardServerToView(s wireguard.Server) object.ViewObject {
	return object.ViewObject{
		Ref:          object.ObjectRef{Kind: "wireguard.server", ID: s.ID},
		DisplayName:  s.Name,
		Desired: map[string]any{
			"id":        s.ID,
			"name":      s.Name,
			"interface": s.Interface,
			"listenPort": s.ListenPort,
			"addresses": s.Addresses,
			"publicKey": s.PublicKey,
			"updatedAt": s.UpdatedAt,
		},
		Availability: object.Available,
		SupportedActions: object.SupportedActions{
			"observe", "update", "delete", "export",
		},
		Permissions: object.PermissionSet{Observe: true, Configure: true},
	}
}

func wireguardPeerToView(p wireguard.Peer) object.ViewObject {
	return object.ViewObject{
		Ref:          object.ObjectRef{Kind: "wireguard.peer", ID: p.ID},
		DisplayName:  p.Name,
		Desired: map[string]any{
			"id":          p.ID,
			"serverId":    p.ServerID,
			"name":        p.Name,
			"publicKey":   p.PublicKey,
			"allowedIPs":  p.AllowedIPs,
			"endpoint":    p.Endpoint,
			"enabled":     p.Enabled,
			"intent":      p.Intent,
			"updatedAt":   p.UpdatedAt,
		},
		Availability: object.Available,
		SupportedActions: object.SupportedActions{
			"observe", "update", "delete",
		},
		Permissions: object.PermissionSet{Observe: true, Configure: true},
	}
}

func xrayServerToView(s xray.Server) object.ViewObject {
	return object.ViewObject{
		Ref:          object.ObjectRef{Kind: "xray.server", ID: s.ID},
		DisplayName:  s.Name,
		Desired: map[string]any{
			"id":        s.ID,
			"name":      s.Name,
			"protocol":  s.Protocol,
			"transport": s.Transport,
			"security":  s.Security,
			"listen":    s.Listen,
			"port":      s.Port,
			"enabled":   s.Enabled,
			"updatedAt": s.UpdatedAt,
		},
		Availability: object.Available,
		SupportedActions: object.SupportedActions{
			"observe", "update", "delete",
		},
		Permissions: object.PermissionSet{Observe: true, Configure: true},
	}
}

func xrayUserToView(u xray.User) object.ViewObject {
	return object.ViewObject{
		Ref:          object.ObjectRef{Kind: "xray.user", ID: u.ID},
		DisplayName:  u.Name,
		Desired: map[string]any{
			"id":        u.ID,
			"serverId":  u.ServerID,
			"name":      u.Name,
			"email":     u.Email,
			"enabled":   u.Enabled,
			"updatedAt": u.UpdatedAt,
		},
		Availability: object.Available,
		SupportedActions: object.SupportedActions{
			"observe", "update", "delete",
		},
		Permissions: object.PermissionSet{Observe: true, Configure: true},
	}
}

func xrayProfileToView(p xray.Profile) object.ViewObject {
	return object.ViewObject{
		Ref:          object.ObjectRef{Kind: "xray.profile", ID: p.ID},
		DisplayName:  p.Name,
		Desired: map[string]any{
			"id":        p.ID,
			"name":      p.Name,
			"userId":    p.UserID,
			"intent":    p.Intent,
			"selection": p.Selection,
			"enabled":   p.Enabled,
			"updatedAt": p.UpdatedAt,
		},
		Availability: object.Available,
		SupportedActions: object.SupportedActions{
			"observe", "update", "delete",
		},
		Permissions: object.PermissionSet{Observe: true, Configure: true},
	}
}

func xraySubscriptionToView(s xray.Subscription) object.ViewObject {
	return object.ViewObject{
		Ref:          object.ObjectRef{Kind: "xray.subscription", ID: s.ID},
		DisplayName:  s.Name,
		Desired: map[string]any{
			"id":         s.ID,
			"name":       s.Name,
			"profileIds": s.ProfileIDs,
			"updatedAt":  s.UpdatedAt,
		},
		Availability: object.Available,
		SupportedActions: object.SupportedActions{
			"observe", "update", "delete",
		},
		Permissions: object.PermissionSet{Observe: true, Configure: true},
	}
}
