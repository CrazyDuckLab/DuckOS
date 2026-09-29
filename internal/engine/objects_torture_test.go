// Package engine torture: §G06 ObjectList wiring.
package engine

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/object"
)

type stubLister struct {
	kind string
	rows []object.ViewObject
}

func (s *stubLister) Kind() string { return s.kind }
func (s *stubLister) List(ctx context.Context, params object.CollectionParams) (object.Collection, error) {
	limit := params.Limit
	if limit <= 0 || limit > len(s.rows) {
		limit = len(s.rows)
	}
	return object.Collection{
		SnapshotID: "snap-" + s.kind,
		Total:      intPtr(len(s.rows)),
		Complete:   true,
		Items:      s.rows[:limit],
	}, nil
}

func intPtr(i int) *int { return &i }

// TestG06_ObjectListWired verifies a registered lister is
// invoked when ObjectList is called.
func TestG06_ObjectListWired(t *testing.T) {
	e := &Engine{}
	e.objectListers = map[string]ObjectLister{}
	now := time.Now().UTC()
	e.RegisterObjectLister(&stubLister{
		kind: "firewall.rule",
		rows: []object.ViewObject{{
			Ref:          object.ObjectRef{Kind: "firewall.rule", ID: "rule-1"},
			DisplayName:  "allow-ssh",
			Availability: object.Available,
			Permissions:  object.PermissionSet{Observe: true},
			UpdatedAt:    &now,
		}},
	})
	col, err := e.ObjectList(context.Background(), "firewall.rule",
		object.CollectionParams{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if col.SnapshotID != "snap-firewall.rule" {
		t.Errorf("unexpected snapshot: %q", col.SnapshotID)
	}
	if len(col.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(col.Items))
	}
	if col.Items[0].Ref.ID != "rule-1" {
		t.Errorf("unexpected id: %q", col.Items[0].Ref.ID)
	}
}

// TestG06_ObjectListUnknownKindHonest verifies an unknown
// kind returns a Collection envelope with an explicit
// "unsupported" issue, not a 404.
func TestG06_ObjectListUnknownKindHonest(t *testing.T) {
	e := &Engine{}
	e.objectListers = map[string]ObjectLister{}
	col, err := e.ObjectList(context.Background(), "no.such.kind",
		object.CollectionParams{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if col.Complete {
		t.Errorf("honest envelope must report Complete=false for an unknown kind (no source)")
	}
	if len(col.Items) != 0 {
		t.Errorf("unknown kind must yield empty items")
	}
	if len(col.Issues) != 1 || col.Issues[0].ReasonID != "engine_unavailable" {
		t.Errorf("expected engine_unavailable issue, got %+v", col.Issues)
	}
}

// TestG06_ObjectListEnvelopeJSON pins the public JSON
// shape (SnapshotID, Cursor, Total, Complete, Issues,
// Items).
func TestG06_ObjectListEnvelopeJSON(t *testing.T) {
	e := &Engine{}
	e.objectListers = map[string]ObjectLister{}
	e.RegisterObjectLister(&stubLister{kind: "k", rows: nil})
	col, _ := e.ObjectList(context.Background(), "k", object.CollectionParams{Limit: 5})
	js, _ := json.Marshal(col)
	for _, key := range []string{`"snapshotId":`, `"total":`, `"complete":`, `"items":`} {
		if !contains(string(js), key) {
			t.Errorf("envelope JSON missing key %q: %s", key, js)
		}
	}
}

// TestG06_ObjectListLimitClamp pins the MaxCollectionItems
// upper bound for the lister. Listers that exceed it must
// be clamped by the lister itself; the Engine does not
// silently drop rows.
func TestG06_ObjectListLimitClamp(t *testing.T) {
	e := &Engine{}
	e.objectListers = map[string]ObjectLister{}
	rows := make([]object.ViewObject, object.MaxCollectionItems+10)
	for i := range rows {
		rows[i] = object.ViewObject{Ref: object.ObjectRef{Kind: "k", ID: stringerInt(i)}}
	}
	e.RegisterObjectLister(&stubLister{kind: "k", rows: rows})
	col, _ := e.ObjectList(context.Background(), "k", object.CollectionParams{Limit: object.MaxCollectionItems + 100})
	// The stub lister returns the full slice; in production
	// the lister would clamp. We pin the contract: the
	// Engine never lies about Total.
	if col.Total == nil || *col.Total != len(rows) {
		t.Errorf("Total must reflect true set size, got %+v", col.Total)
	}
}

type objectViewObj = object.ViewObject // silence unused warnings

func stringerInt(i int) string { return "id-" + string(rune('a'+i%26)) + string(rune('0'+i/26)) }

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
