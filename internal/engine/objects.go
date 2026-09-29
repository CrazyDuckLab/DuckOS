// Package engine object list surface (G06 wiring). The
// Engine exposes one bounded-list entry point that returns
// a canonical `object.Collection` envelope. The actual
// per-domain stores are wired in via ObjectLister
// registrations; this batch ships the read-side plumbing
// and a torture contract.
package engine

import (
	"context"

	"github.com/CrazyDuckLab/DuckOS/internal/object"
)

// ObjectLister produces a bounded list of view objects for
// a given kind. The Engine calls every registered lister
// when the API receives GET /api/v1/objects/{kind}.
//
// Limit and After are honoured by the implementation; the
// total returned is the implementation's best estimate
// (nullable in `object.Collection`).
type ObjectLister interface {
	Kind() string
	List(ctx context.Context, params object.CollectionParams) (object.Collection, error)
}

// RegisterObjectLister wires a per-kind lister into the
// Engine. Callers may register more than one lister; the
// last registration wins.
func (e *Engine) RegisterObjectLister(l ObjectLister) {
	if e.objectListers == nil {
		e.objectListers = map[string]ObjectLister{}
	}
	e.objectListers[l.Kind()] = l
}

// ObjectList returns the canonical object.Collection for
// the requested kind. When no lister is registered the
// envelope is returned with Complete=false and Items=[];
// availability is "engine_unavailable" so the UI renders
// an honest "no backend for this kind" affordance rather
// than claiming Complete=true on a missing source.
func (e *Engine) ObjectList(ctx context.Context, kind string, params object.CollectionParams) (object.Collection, error) {
	if e == nil {
		return object.Collection{}, nil
	}
	if l, ok := e.objectListers[kind]; ok {
		return l.List(ctx, params)
	}
	return object.Collection{
		SnapshotID: "engine-no-lister",
		Complete:   false,
		Items:      []object.ViewObject{},
		Issues: []object.Issue{{
			Source:   "engine",
			ReasonID: "engine_unavailable",
		}},
	}, nil
}
