// Package object torture: §G02 canonical kind/id/revision
// contract and Collection envelope. Each subtest pins one
// boundary of the public surface that UI, CLI and API must
// share.
package object

import (
	"encoding/json"
	"testing"
	"time"
)

// TestTorture_ObjectRefCanonical verifies the canonical
// triple (kind, id, revision) round-trips through JSON and
// that the empty and equal helpers work.
func TestTorture_ObjectRefCanonical(t *testing.T) {
	r := ObjectRef{Kind: "network.interface", ID: NewID(), Revision: "rev-1"}
	if r.IsZero() {
		t.Fatal("populated ref must not be zero")
	}
	if r.Equal(ObjectRef{}) {
		t.Fatal("populated ref must not equal empty ref")
	}
	if !r.Equal(ObjectRef{Kind: "network.interface", ID: r.ID, Revision: "rev-1"}) {
		t.Fatal("equal refs must compare equal")
	}
	js, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var back ObjectRef
	if err := json.Unmarshal(js, &back); err != nil {
		t.Fatal(err)
	}
	if !back.Equal(r) {
		t.Fatalf("round-trip mismatch: %+v vs %+v", back, r)
	}
}

// TestTorture_CollectionEnvelope verifies the bounded
// Collection envelope preserves total / cursor / complete.
func TestTorture_CollectionEnvelope(t *testing.T) {
	items := []ViewObject{{Ref: ObjectRef{Kind: "k", ID: NewID()}}, {Ref: ObjectRef{Kind: "k", ID: NewID()}}}
	col := Collection{
		SnapshotID: "snap-1",
		Cursor:     "next",
		Total:      intPtr(42),
		Complete:   true,
		Items:      items,
	}
	if !col.Complete {
		t.Fatal("Complete=true expected")
	}
	if col.Total == nil || *col.Total != 42 {
		t.Fatal("Total=42 expected")
	}
	js, _ := json.Marshal(col)
	var back Collection
	if err := json.Unmarshal(js, &back); err != nil {
		t.Fatal(err)
	}
	if back.SnapshotID != "snap-1" || back.Cursor != "next" || *back.Total != 42 || !back.Complete {
		t.Fatalf("envelope round-trip mismatch: %+v", back)
	}
}

// TestTorture_CollectionTotalUnknown pins the "null total"
// semantic: a Collection from a live capture may not know
// the underlying set size; Total is *int and may be nil.
func TestTorture_CollectionTotalUnknown(t *testing.T) {
	col := Collection{SnapshotID: "live", Complete: false}
	js, _ := json.Marshal(col)
	if !contains(string(js), `"total":null`) {
		t.Fatalf("unknown total must serialise to null: %s", js)
	}
}

// TestTorture_AvailabilityCategories pins the six
// language-neutral availability categories.
func TestTorture_AvailabilityCategories(t *testing.T) {
	for _, a := range []Availability{Available, Experimental, HardwarePending, CredentialPending, Unsupported, Unknown} {
		if a == "" {
			t.Error("empty availability forbidden")
		}
	}
}

// TestTorture_SupportedActionsIncludes verifies the action
// set membership helper.
func TestTorture_SupportedActionsIncludes(t *testing.T) {
	actions := SupportedActions{"list", "show", "edit", "delete", "rotate", "revoke"}
	for _, a := range actions {
		if !actions.Includes(a) {
			t.Errorf("must include %q", a)
		}
	}
	if actions.Includes("unknown-action") {
		t.Error("must not include unknown-action")
	}
}

// TestTorture_PermissionSet verifies the language-neutral
// permission projection.
func TestTorture_PermissionSet(t *testing.T) {
	p := PermissionSet{Observe: true, Configure: true, Action: true, ExportSecret: false, Delete: true}
	if !p.Observe || !p.Configure || !p.Action {
		t.Fatal("expected observe/configure/action true")
	}
	if p.ExportSecret {
		t.Fatal("expected exportSecret false")
	}
	js, _ := json.Marshal(p)
	var back PermissionSet
	if err := json.Unmarshal(js, &back); err != nil {
		t.Fatal(err)
	}
	if back != p {
		t.Fatalf("permission round-trip mismatch: %+v", back)
	}
}

// TestTorture_OwnerRef pins the owner reference contract.
func TestTorture_OwnerRef(t *testing.T) {
	o := OwnerRef{Kind: "template", ID: NewID(), Name: "vpn-defaults"}
	if o.IsZero() {
		t.Fatal("populated owner must not be zero")
	}
}

// TestTorture_NewIDUniqueness generates 256 ids and asserts
// all distinct.
func TestTorture_NewIDUniqueness(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 256; i++ {
		id := NewID()
		if len(id) != 32 {
			t.Fatalf("expected 32-char hex, got %d", len(id))
		}
		if seen[id] {
			t.Fatalf("duplicate id %q", id)
		}
		seen[id] = true
	}
}

// TestTorture_ViewObjectEnvelope verifies the canonical
// ViewObject shape (ref, displayName, desired, observed,
// availability, owner, supportedActions, permissions,
// tags, updatedAt).
func TestTorture_ViewObjectEnvelope(t *testing.T) {
	now := time.Now().UTC()
	v := ViewObject{
		Ref:              ObjectRef{Kind: "firewall.rule", ID: NewID(), Revision: "rev-1"},
		DisplayName:      "allow-ssh",
		Desired:          map[string]any{"action": "accept", "destPort": 22},
		Observed:         map[string]any{"hits": 0},
		Availability:     Available,
		Owner:            OwnerRef{Kind: "template", ID: NewID()},
		SupportedActions: SupportedActions{"show", "edit", "delete"},
		Permissions:      PermissionSet{Observe: true, Configure: true, Delete: true},
		Tags:             []string{"ssh", "ops"},
		UpdatedAt:        &now,
	}
	js, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{`"ref":`, `"displayName":`, `"desired":`, `"observed":`, `"availability":`, `"owner":`, `"supportedActions":`, `"permissions":`} {
		if !contains(string(js), key) {
			t.Errorf("view JSON missing key %q: %s", key, js)
		}
	}
}

func intPtr(i int) *int { return &i }

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
