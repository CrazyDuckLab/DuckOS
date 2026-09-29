// G08 ownership/provenance torture regression. The store
// must:
//   - reject invalid objects and empty ownership records;
//   - preserve FirstSeen across updates;
//   - dedupe and sort tags;
//   - trim provenance to maxEvents;
//   - return Provenance newest first;
//   - allow zero At timestamps (auto-stamped);
//   - be safe under concurrent access.
package lifecycle

import (
	"errors"
	"sync"
	"testing"
	"time"
)

func sampleObj() Object {
	return Object{Kind: KindVLAN, ID: "vlan-10"}
}

func TestOwnership_RejectsInvalidObject(t *testing.T) {
	s := NewOwnershipStore()
	if err := s.SetOwnership(Object{Kind: "bogus", ID: "x"}, OwnershipRecord{Generator: "manual"}); !errors.Is(err, ErrInvalidObject) {
		t.Fatalf("expected ErrInvalidObject, got %v", err)
	}
	if err := s.SetOwnership(Object{Kind: KindVLAN, ID: "bad id!"}, OwnershipRecord{Generator: "manual"}); !errors.Is(err, ErrInvalidObject) {
		t.Fatalf("expected ErrInvalidObject on bad id, got %v", err)
	}
}

func TestOwnership_RejectsEmptyRecord(t *testing.T) {
	s := NewOwnershipStore()
	if err := s.SetOwnership(sampleObj(), OwnershipRecord{}); !errors.Is(err, ErrInvalidOwnership) {
		t.Fatalf("expected ErrInvalidOwnership, got %v", err)
	}
}

func TestOwnership_PreservesFirstSeen(t *testing.T) {
	s := NewOwnershipStore()
	obj := sampleObj()
	if err := s.SetOwnership(obj, OwnershipRecord{Generator: "template", Manager: "operator", Source: "vpn-defaults"}); err != nil {
		t.Fatalf("SetOwnership: %v", err)
	}
	first, _ := s.Ownership(obj)
	time.Sleep(10 * time.Millisecond)
	if err := s.SetOwnership(obj, OwnershipRecord{Generator: "template", Manager: "auto-policy"}); err != nil {
		t.Fatalf("SetOwnership update: %v", err)
	}
	second, _ := s.Ownership(obj)
	if !second.FirstSeen.Equal(first.FirstSeen) {
		t.Fatalf("FirstSeen must be preserved across updates: %v vs %v", first.FirstSeen, second.FirstSeen)
	}
	if !second.UpdatedAt.After(first.UpdatedAt) {
		t.Fatalf("UpdatedAt must advance on update: %v vs %v", first.UpdatedAt, second.UpdatedAt)
	}
}

func TestOwnership_DedupesAndSortsTags(t *testing.T) {
	s := NewOwnershipStore()
	obj := sampleObj()
	err := s.SetOwnership(obj, OwnershipRecord{
		Generator: "manual",
		Manager:   "operator",
		Tags:      []string{"z", "a", "a", "m", "b"},
	})
	if err != nil {
		t.Fatalf("SetOwnership: %v", err)
	}
	rec, _ := s.Ownership(obj)
	if want := []string{"a", "b", "m", "z"}; !equalStrings(rec.Tags, want) {
		t.Fatalf("tags not deduped/sorted: got %v want %v", rec.Tags, want)
	}
}

func TestOwnership_AllOwnershipIsSorted(t *testing.T) {
	s := NewOwnershipStore()
	rec := OwnershipRecord{Generator: "manual", Manager: "operator"}
	_ = s.SetOwnership(Object{Kind: KindWAN, ID: "wan-2"}, rec)
	_ = s.SetOwnership(Object{Kind: KindVLAN, ID: "vlan-10"}, rec)
	_ = s.SetOwnership(Object{Kind: KindVLAN, ID: "vlan-2"}, rec)
	all := s.AllOwnership()
	if len(all) != 3 {
		t.Fatalf("expected 3 records, got %d", len(all))
	}
	if all[0].Object.Kind != KindVLAN || all[0].Object.ID != "vlan-10" {
		// sorted by (Kind, ID); vlan-10 > vlan-2 lexically.
		t.Fatalf("first row should be vlan-10, got %+v", all[0].Object)
	}
}

func TestProvenance_RejectsInvalid(t *testing.T) {
	s := NewOwnershipStore()
	if err := s.AppendProvenance(sampleObj(), ProvenanceEvent{Kind: "bogus"}); !errors.Is(err, ErrInvalidProvenance) {
		t.Fatalf("expected ErrInvalidProvenance for unknown kind")
	}
	if err := s.AppendProvenance(Object{Kind: "bogus", ID: "x"}, ProvenanceEvent{Kind: ProvCreated, At: time.Now()}); !errors.Is(err, ErrInvalidObject) {
		t.Fatalf("expected ErrInvalidObject for invalid object")
	}
}

func TestProvenance_AutoStampsZeroTime(t *testing.T) {
	s := NewOwnershipStore()
	if err := s.AppendProvenance(sampleObj(), ProvenanceEvent{Kind: ProvCreated}); err != nil {
		t.Fatalf("AppendProvenance: %v", err)
	}
	events := s.Provenance(sampleObj())
	if len(events) != 1 || events[0].At.IsZero() {
		t.Fatalf("At must be stamped automatically, got %+v", events)
	}
}

func TestProvenance_TrimsToMaxEvents(t *testing.T) {
	s := NewOwnershipStore()
	s.SetMaxEvents(3)
	obj := sampleObj()
	for i := 0; i < 10; i++ {
		if err := s.AppendProvenance(obj, ProvenanceEvent{
			Kind: ProvModified,
			At:   time.Unix(int64(i), 0).UTC(),
		}); err != nil {
			t.Fatalf("AppendProvenance: %v", err)
		}
	}
	events := s.Provenance(obj)
	if len(events) != 3 {
		t.Fatalf("expected 3 events after trim, got %d", len(events))
	}
	// Newest first: 9, 8, 7
	if !events[0].At.Equal(time.Unix(9, 0).UTC()) || !events[2].At.Equal(time.Unix(7, 0).UTC()) {
		t.Fatalf("trim ordering broken: %+v", events)
	}
}

func TestProvenance_AllProvenanceMergesNewestFirst(t *testing.T) {
	s := NewOwnershipStore()
	_ = s.AppendProvenance(Object{Kind: KindVLAN, ID: "a"}, ProvenanceEvent{Kind: ProvCreated, At: time.Unix(2, 0).UTC()})
	_ = s.AppendProvenance(Object{Kind: KindVLAN, ID: "b"}, ProvenanceEvent{Kind: ProvCreated, At: time.Unix(5, 0).UTC()})
	_ = s.AppendProvenance(Object{Kind: KindVLAN, ID: "a"}, ProvenanceEvent{Kind: ProvModified, At: time.Unix(7, 0).UTC()})
	all := s.AllProvenance()
	if len(all) != 3 {
		t.Fatalf("expected 3 merged events, got %d", len(all))
	}
	if !all[0].At.Equal(time.Unix(7, 0).UTC()) {
		t.Fatalf("merged events must be newest first: %+v", all)
	}
}

func TestOwnership_PurgeRemovesEverything(t *testing.T) {
	s := NewOwnershipStore()
	obj := sampleObj()
	_ = s.SetOwnership(obj, OwnershipRecord{Generator: "manual"})
	_ = s.AppendProvenance(obj, ProvenanceEvent{Kind: ProvCreated, At: time.Now().UTC()})
	s.Purge(obj)
	if _, ok := s.Ownership(obj); ok {
		t.Fatalf("ownership must be purged")
	}
	if len(s.Provenance(obj)) != 0 {
		t.Fatalf("provenance must be purged")
	}
}

func TestOwnership_ConcurrentAccess(t *testing.T) {
	s := NewOwnershipStore()
	obj := sampleObj()
	_ = s.SetOwnership(obj, OwnershipRecord{Generator: "manual", Manager: "operator"})
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			_ = s.AppendProvenance(obj, ProvenanceEvent{Kind: ProvModified, At: time.Now().UTC()})
		}()
		go func() {
			defer wg.Done()
			_, _ = s.Ownership(obj)
		}()
	}
	wg.Wait()
	if _, ok := s.Ownership(obj); !ok {
		t.Fatalf("ownership must survive concurrent reads")
	}
}

func TestProvenanceKind_Valid(t *testing.T) {
	for _, k := range []ProvenanceKind{ProvCreated, ProvModified, ProvRestored, ProvDetached, ProvImported} {
		if !k.Valid() {
			t.Errorf("expected %q to be valid", k)
		}
	}
	if ProvenanceKind("garbage").Valid() {
		t.Errorf("garbage must be invalid")
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
