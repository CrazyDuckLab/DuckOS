// lifecycle/provenance.go: G08 dependency / ownership /
// provenance overlay on top of Graph.
//
// The dependency Graph is the read-side surface for the
// "delete preflight" use case. Ownership and provenance
// extend it with the audit information the operator needs
// to decide whether a destructive action is safe:
//
//	OwnershipRecord  — who manages the object, who
//	                   generated it, how it was tagged,
//	                   when it was first seen.
//
//	ProvenanceEvent   — a single mutation against the
//	                    object. The store keeps the most
//	                    recent MaxProvenanceEvents entries
//	                    so the UI can render a compact
//	                    "last 10 changes" timeline.
//
// Every record is goroutine-safe and bounded; the store
// never grows unbounded even on long-running installations.
package lifecycle

import (
	"errors"
	"sort"
	"sync"
	"time"
)

// MaxProvenanceEvents is the per-object retention ceiling
// for the provenance log. The constant is small so the
// IPC payload stays bounded; older events fall off the end.
const MaxProvenanceEvents = 64

// ProvenanceKind enumerates the canonical provenance
// event types. New kinds must be appended so the wire
// format stays backward compatible.
type ProvenanceKind string

const (
	ProvCreated  ProvenanceKind = "created"
	ProvModified ProvenanceKind = "modified"
	ProvRestored ProvenanceKind = "restored"
	ProvDetached ProvenanceKind = "detached"
	ProvImported ProvenanceKind = "imported"
)

// Valid reports whether the kind is recognised.
func (k ProvenanceKind) Valid() bool {
	switch k {
	case ProvCreated, ProvModified, ProvRestored, ProvDetached, ProvImported:
		return true
	}
	return false
}

// OwnershipRecord is the per-object ownership projection.
// At least one of Generator or Manager must be non-empty;
// both may be present when a template generated an object
// that the operator later reassigned.
type OwnershipRecord struct {
	// Generator is the system that first produced the
	// object (e.g. "template", "import", "manual").
	Generator string `json:"generator,omitempty"`
	// Manager is the actor or system currently responsible
	// for the object ("operator", "auto-policy",
	// "duck-cli").
	Manager string `json:"manager,omitempty"`
	// Tags are free-form labels. Tags are deduplicated and
	// stored in canonical order so the API output is
	// deterministic.
	Tags []string `json:"tags,omitempty"`
	// Source is a stable identifier for the originating
	// input (template id, import filename, transaction
	// id). Optional.
	Source string `json:"source,omitempty"`
	// FirstSeen is when the record was first inserted;
	// UpdatedAt is the last mutation timestamp.
	FirstSeen time.Time `json:"firstSeen"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// IsEmpty reports whether r carries no information.
func (r OwnershipRecord) IsEmpty() bool {
	return r.Generator == "" && r.Manager == "" && len(r.Tags) == 0 && r.Source == ""
}

// ProvenanceEvent is a single entry in the per-object
// provenance log. Events are append-only.
type ProvenanceEvent struct {
	Kind ProvenanceKind `json:"kind"`
	// Actor is the human or system that caused the event.
	Actor string `json:"actor,omitempty"`
	// Reason is an operator-facing explanation.
	Reason string `json:"reason,omitempty"`
	// Revision ties the event to a state revision when
	// available. Empty for events that don't touch a
	// revision (e.g. Detached).
	Revision string `json:"revision,omitempty"`
	// At is the wall-clock time of the event (UTC).
	At time.Time `json:"at"`
}

// Valid reports whether the event has a recognised kind
// and a non-zero timestamp.
func (e ProvenanceEvent) Valid() bool {
	if !e.Kind.Valid() {
		return false
	}
	if e.At.IsZero() {
		return false
	}
	return true
}

// OwnershipStore extends Graph with per-object ownership
// and provenance. The store is goroutine-safe.
type OwnershipStore struct {
	opMu       sync.RWMutex
	ownership  map[Object]OwnershipRecord
	provenance map[Object][]ProvenanceEvent
	maxEvents  int
}

// NewOwnershipStore returns an empty store.
func NewOwnershipStore() *OwnershipStore {
	return &OwnershipStore{
		ownership:  map[Object]OwnershipRecord{},
		provenance: map[Object][]ProvenanceEvent{},
		maxEvents:  MaxProvenanceEvents,
	}
}

// SetMaxEvents overrides the per-object retention ceiling.
// A non-positive value resets to MaxProvenanceEvents.
func (s *OwnershipStore) SetMaxEvents(n int) {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	if n <= 0 {
		n = MaxProvenanceEvents
	}
	s.maxEvents = n
}

// ErrInvalidOwnership is returned when the supplied record
// fails validation (both Generator and Manager empty).
var ErrInvalidOwnership = errors.New("lifecycle: ownership record requires generator or manager")

// ErrInvalidProvenance is returned when an event fails
// ProvenanceEvent.Valid.
var ErrInvalidProvenance = errors.New("lifecycle: provenance event has unknown kind or zero timestamp")

// SetOwnership upserts the ownership record for obj.
// FirstSeen is preserved across updates. Tags are
// deduplicated and sorted.
func (s *OwnershipStore) SetOwnership(obj Object, rec OwnershipRecord) error {
	if !obj.Valid() {
		return ErrInvalidObject
	}
	if rec.IsEmpty() {
		return ErrInvalidOwnership
	}
	rec.Tags = dedupSorted(rec.Tags)
	now := time.Now().UTC()
	s.opMu.Lock()
	defer s.opMu.Unlock()
	if existing, ok := s.ownership[obj]; ok {
		if existing.FirstSeen.IsZero() {
			rec.FirstSeen = now
		} else {
			rec.FirstSeen = existing.FirstSeen
		}
	} else {
		rec.FirstSeen = now
	}
	rec.UpdatedAt = now
	s.ownership[obj] = rec
	return nil
}

// Ownership returns the ownership record for obj.
// ok is false when no record has been registered.
func (s *OwnershipStore) Ownership(obj Object) (OwnershipRecord, bool) {
	s.opMu.RLock()
	defer s.opMu.RUnlock()
	r, ok := s.ownership[obj]
	return r, ok
}

// AllOwnership returns every record sorted by
// (Kind, ID) so callers can render a stable table.
func (s *OwnershipStore) AllOwnership() []OwnershipEntry {
	s.opMu.RLock()
	defer s.opMu.RUnlock()
	out := make([]OwnershipEntry, 0, len(s.ownership))
	for obj, rec := range s.ownership {
		out = append(out, OwnershipEntry{Object: obj, Record: rec})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Object.Kind != out[j].Object.Kind {
			return out[i].Object.Kind < out[j].Object.Kind
		}
		return out[i].Object.ID < out[j].Object.ID
	})
	return out
}

// OwnershipEntry pairs an Object with its record.
type OwnershipEntry struct {
	Object Object          `json:"object"`
	Record OwnershipRecord `json:"record"`
}

// AppendProvenance records an event against obj. The
// per-object log is trimmed to maxEvents entries. The
// caller may pass an event with At zero; the store stamps
// the wall-clock time automatically.
func (s *OwnershipStore) AppendProvenance(obj Object, event ProvenanceEvent) error {
	if !obj.Valid() {
		return ErrInvalidObject
	}
	if event.At.IsZero() {
		event.At = time.Now().UTC()
	}
	if !event.Kind.Valid() {
		return ErrInvalidProvenance
	}
	s.opMu.Lock()
	defer s.opMu.Unlock()
	events := append(s.provenance[obj], event)
	if s.maxEvents > 0 && len(events) > s.maxEvents {
		events = events[len(events)-s.maxEvents:]
	}
	s.provenance[obj] = events
	return nil
}

// Provenance returns the per-object log sorted newest
// first. The slice is a copy; mutating it does not affect
// the store.
func (s *OwnershipStore) Provenance(obj Object) []ProvenanceEvent {
	s.opMu.RLock()
	defer s.opMu.RUnlock()
	src := s.provenance[obj]
	out := make([]ProvenanceEvent, len(src))
	copy(out, src)
	sort.Slice(out, func(i, j int) bool { return out[i].At.After(out[j].At) })
	return out
}

// AllProvenance returns every event across every object,
// newest first, for the audit surface.
func (s *OwnershipStore) AllProvenance() []ProvenanceEvent {
	s.opMu.RLock()
	defer s.opMu.RUnlock()
	var out []ProvenanceEvent
	for _, events := range s.provenance {
		out = append(out, events...)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].At.After(out[j].At) })
	return out
}

// Purge removes every record for obj. The function is
// idempotent.
func (s *OwnershipStore) Purge(obj Object) {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	delete(s.ownership, obj)
	delete(s.provenance, obj)
}

// dedupSorted trims duplicates and sorts the slice in
// ascending lexicographic order.
func dedupSorted(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	set := map[string]struct{}{}
	for _, s := range in {
		if s == "" {
			continue
		}
		set[s] = struct{}{}
	}
	out := make([]string, 0, len(set))
	for s := range set {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}
