// Package objectlisters real listers (G06 wiring). This file
// replaces the unsupported placeholder for three representative V1
// domains with listers that delegate to existing Engine accessors:
//
//	qos.class        → Engine.QoS()      (privileged Agent IPC)
//	network.interface → Engine.Doctor()  (live kernel snapshot)
//	network.route     → Engine.Doctor()  (live kernel snapshot)
//
// Each lister holds an Engine reference supplied at registration
// time; the Engine is the only owner of the privileged IPC
// client and is goroutine-safe. Listers MUST clamp their page
// to object.MaxCollectionItems and MUST report availability
// honestly: a successful Engine call becomes "available", an
// error becomes an Issue so the UI can render the gap.
package objectlisters

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/CrazyDuckLab/DuckOS/internal/doctor"
	"github.com/CrazyDuckLab/DuckOS/internal/engine"
	"github.com/CrazyDuckLab/DuckOS/internal/object"
	"github.com/CrazyDuckLab/DuckOS/internal/qos"
)

// ListerSource is the narrow Engine contract the real listers
// need. *engine.Engine satisfies it. Defining the contract in
// this package keeps the listers independent of the wider
// api.Engine surface (which carries unrelated methods) and
// makes the production wiring explicit.
type ListerSource interface {
	Doctor(context.Context) (doctor.Snapshot, error)
	QoS(context.Context) (qos.Status, error)
}

// engineLister is the tiny adapter that backs every real
// lister in this file. The ListerSource is the only owner of
// the privileged IPC client; the lister asks the ListerSource
// for the read-only data and projects it into the
// object.Collection envelope. The reference is captured at
// registration time so the lister contract is explicit: there
// is no path to data that bypasses the privileged side.
type engineLister struct {
	kind string
	eng  ListerSource
	list func(ctx context.Context, e ListerSource) ([]object.ViewObject, error)
}

func (l engineLister) Kind() string { return l.kind }

// List projects the source snapshot into a Collection envelope.
// The lister honors CollectionParams.Limit and never returns more
// than MaxCollectionItems rows. A snapshotID is derived from the
// source content so the caller can echo it back for diff-style
// pagination. The Engine call is bounded by the supplied ctx.
func (l engineLister) List(ctx context.Context, params object.CollectionParams) (object.Collection, error) {
	if l.eng == nil {
		return object.Collection{
			SnapshotID: l.fallbackSnapshotID("engine-not-wired"),
			Complete:   false,
			Items:      []object.ViewObject{},
			Issues: []object.Issue{{
				Source:   "engine",
				ReasonID: "engine_unavailable",
				Detail:   "Engine reference is not available in this lister context",
			}},
		}, nil
	}
	items, err := l.list(ctx, l.eng)
	if err != nil {
		return object.Collection{
			SnapshotID: l.fallbackSnapshotID("engine-error"),
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
	snap := l.fallbackSnapshotID(fmt.Sprintf("%s:%d", l.kind, total))
	return object.Collection{
		SnapshotID: snap,
		Complete:   true,
		Items:      page,
		Total:      &total,
	}, nil
}

// fallbackSnapshotID returns the deterministic snapshot id used
// when the source is unavailable. The id is stable per-kind so
// the UI can correlate repeated calls.
func (l engineLister) fallbackSnapshotID(reason string) string {
	sum := sha256.Sum256([]byte(l.kind + ":" + reason))
	return "engine-" + l.kind + "-" + hex.EncodeToString(sum[:8])
}

// paginate clamps the items slice to the caller's Limit and to
// object.MaxCollectionItems. After is honored as an opaque
// offset; the Engine-side data is bounded by MaxCollectionItems
// already so the cursor is just an integer index in hex.
func paginate(items []object.ViewObject, params object.CollectionParams) ([]object.ViewObject, int) {
	total := len(items)
	if total > object.MaxCollectionItems {
		items = items[:object.MaxCollectionItems]
		total = object.MaxCollectionItems
	}
	offset := 0
	if params.After != "" {
		v, err := decodeCursor(params.After)
		if err == nil && v >= 0 && v <= total {
			offset = v
		}
	}
	if offset > total {
		offset = total
	}
	limit := params.Limit
	if limit <= 0 || limit > object.MaxCollectionItems {
		limit = object.MaxCollectionItems
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return items[offset:end], total
}

// decodeCursor accepts an arbitrary-length hex string and returns
// the integer it represents. The hex is zero-padded to an even
// length so single-digit offsets (e.g. "1") decode cleanly.
func decodeCursor(s string) (int, error) {
	if len(s)%2 == 1 {
		s = "0" + s
	}
	raw, err := hex.DecodeString(s)
	if err != nil || len(raw) > 8 {
		return 0, errors.New("cursor is not canonical")
	}
	var n int
	for _, b := range raw {
		n = n<<8 | int(b)
	}
	return n, nil
}

func encodeCursor(n int) string {
	return fmt.Sprintf("%x", n)
}

// NewQoSClassLister returns an ObjectLister that reads the
// privileged QoS topology through Engine.QoS() and projects
// every qos.Class into a ViewObject. The lister is read-only:
// the Engine never mutates state on List. Availability is
// "available" when the Agent responded with a valid Topology;
// the lister never invents availability for an empty list.
func NewQoSClassLister(e ListerSource) engine.ObjectLister {
	return engineLister{
		kind: "qos.class",
		eng:  e,
		list: func(ctx context.Context, eng ListerSource) ([]object.ViewObject, error) {
			st, err := eng.QoS(ctx)
			if err != nil {
				return nil, err
			}
			items := make([]object.ViewObject, 0, len(st.Topology.Classes))
			for _, cls := range st.Topology.Classes {
				items = append(items, classToView(cls, st))
			}
			return items, nil
		},
	}
}

// classToView projects a qos.Class into the canonical ViewObject
// envelope. The desired field carries the canonical class
// definition; observed carries the live status snapshot when
// present. Availability is "available" when the Agent responded
// with a valid topology.
func classToView(cls qos.Class, st qos.Status) object.ViewObject {
	desired := map[string]any{
		"id":          cls.ID,
		"shaperId":    cls.ShaperID,
		"name":        cls.Name,
		"markBits":    cls.MarkBits,
		"rateShare":   cls.RateShare,
		"description": cls.Description,
	}
	var observed any
	if st.LastApply != 0 {
		observed = map[string]any{
			"lastApplyUnix": st.LastApply,
			"lastError":     st.LastError,
			"hasShapers":    st.HasShapers,
		}
	}
	return object.ViewObject{
		Ref:          object.ObjectRef{Kind: "qos.class", ID: cls.ID},
		DisplayName:  cls.Name,
		Desired:      desired,
		Observed:     observed,
		Availability: object.Available,
		SupportedActions: object.SupportedActions{
			"observe",
		},
		Permissions: object.PermissionSet{Observe: true},
	}
}

// NewNetworkInterfaceLister returns an ObjectLister that reads
// the live interface table from Engine.Doctor() and projects
// every interface into a ViewObject. The lister is read-only and
// never invents availability: an Agent error becomes an Issue.
func NewNetworkInterfaceLister(e ListerSource) engine.ObjectLister {
	return engineLister{
		kind: "network.interface",
		eng:  e,
		list: func(ctx context.Context, eng ListerSource) ([]object.ViewObject, error) {
			snap, err := eng.Doctor(ctx)
			if err != nil {
				return nil, err
			}
			items := make([]object.ViewObject, 0, len(snap.Network.Interfaces))
			for _, iface := range snap.Network.Interfaces {
				items = append(items, interfaceToView(iface))
			}
			return items, nil
		},
	}
}

func interfaceToView(iface doctor.Interface) object.ViewObject {
	state := "down"
	if iface.LinkUp {
		state = "up"
	}
	desired := map[string]any{
		"name":      iface.Name,
		"kind":      iface.Kind,
		"mtu":       iface.MTU,
		"addresses": append([]string(nil), iface.Addresses...),
	}
	observed := map[string]any{
		"linkUp":    iface.LinkUp,
		"operState": iface.OperState,
		"mac":       iface.MAC,
		"driver":    iface.Driver,
		"index":     iface.Index,
	}
	return object.ViewObject{
		Ref:          object.ObjectRef{Kind: "network.interface", ID: iface.Name},
		DisplayName:  iface.Name,
		Desired:      desired,
		Observed:     observed,
		Availability: object.Available,
		SupportedActions: object.SupportedActions{
			"observe",
		},
		Permissions: object.PermissionSet{Observe: true},
		Tags:        []string{state},
	}
}

// NewNetworkRouteLister returns an ObjectLister that reads the
// live route table from Engine.Doctor() and projects every route
// into a ViewObject. Routes are identified by destination so
// duplicate-destination entries collide; the Engine bounds the
// list to the live kernel snapshot.
func NewNetworkRouteLister(e ListerSource) engine.ObjectLister {
	return engineLister{
		kind: "network.route",
		eng:  e,
		list: func(ctx context.Context, eng ListerSource) ([]object.ViewObject, error) {
			snap, err := eng.Doctor(ctx)
			if err != nil {
				return nil, err
			}
			items := make([]object.ViewObject, 0, len(snap.Network.Routes))
			for i, route := range snap.Network.Routes {
				items = append(items, routeToView(route, i))
			}
			return items, nil
		},
	}
}

func routeToView(route doctor.Route, idx int) object.ViewObject {
	id := route.Destination
	if id == "" {
		id = fmt.Sprintf("route-%d", idx)
	}
	desired := map[string]any{
		"destination": route.Destination,
		"gateway":     route.Gateway,
		"device":      route.Device,
		"table":       route.Table,
		"source":      route.Source,
		"metric":      route.Metric,
	}
	return object.ViewObject{
		Ref:          object.ObjectRef{Kind: "network.route", ID: id},
		DisplayName:  fmt.Sprintf("%s via %s", route.Destination, route.Gateway),
		Desired:      desired,
		Availability: object.Available,
		SupportedActions: object.SupportedActions{
			"observe",
		},
		Permissions: object.PermissionSet{Observe: true},
	}
}
