// accesslifecycle/accesslifecycle.go: V1
// user / access lifecycle. The package
// owns the typed contract for temporary
// WireGuard peers, Xray users, firewall
// rules, published services, and access
// grants. Every grant carries expiry,
// rotation, revoke, and history.
//
// The runtime is a thin mapping to the
// existing per-subsystem stores; the
// package itself does not perform any
// state mutation.
package accesslifecycle

import (
	"errors"
	"regexp"
	"sort"
	"sync"
	"time"
)

// SubjectKind is the access subject class.
type SubjectKind string

const (
	SubjWireGuard    SubjectKind = "wireguard-peer"
	SubjXrayUser     SubjectKind = "xray-user"
	SubjFirewallRule SubjectKind = "firewall-rule"
	SubjPublish      SubjectKind = "publish"
	SubjGrant        SubjectKind = "grant"
)

// Valid reports whether k is known.
func (k SubjectKind) Valid() bool {
	switch k {
	case SubjWireGuard, SubjXrayUser, SubjFirewallRule, SubjPublish, SubjGrant:
		return true
	}
	return false
}

// Subject is a typed access subject.
type Subject struct {
	ID          string      `json:"id"`
	Kind        SubjectKind `json:"kind"`
	Label       string      `json:"label,omitempty"`
	Owner       string      `json:"owner,omitempty"`
	CreatedAt   time.Time   `json:"createdAt"`
	ExpiresAt   time.Time   `json:"expiresAt,omitempty"`
	RevokedAt   time.Time   `json:"revokedAt,omitempty"`
	LastRotated time.Time   `json:"lastRotated,omitempty"`
	Notes       string      `json:"notes,omitempty"`
}

var idPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:-]{0,63}$`)

// Valid reports whether s is well-formed.
func (s Subject) Valid() error {
	if !idPattern.MatchString(s.ID) {
		return errors.New("accesslifecycle: id is not canonical")
	}
	if !s.Kind.Valid() {
		return errors.New("accesslifecycle: kind is unknown")
	}
	if s.CreatedAt.IsZero() {
		return errors.New("accesslifecycle: createdAt is required")
	}
	if !s.ExpiresAt.IsZero() && !s.ExpiresAt.After(s.CreatedAt) {
		return errors.New("accesslifecycle: expiresAt must be after createdAt")
	}
	if !s.RevokedAt.IsZero() && s.RevokedAt.Before(s.CreatedAt) {
		return errors.New("accesslifecycle: revokedAt must be after createdAt")
	}
	return nil
}

// Active reports whether the subject is
// active at t. A subject is active when it
// has not been revoked and the supplied
// time is before the expiry.
func (s Subject) Active(t time.Time) bool {
	if !s.RevokedAt.IsZero() && !t.Before(s.RevokedAt) {
		return false
	}
	if !s.ExpiresAt.IsZero() && !t.Before(s.ExpiresAt) {
		return false
	}
	return true
}

// HistoryEntry is one lifecycle event for
// a subject.
type HistoryEntry struct {
	At     time.Time `json:"at"`
	Action string    `json:"action"` // "created" | "rotated" | "revoked" | "expired"
	Actor  string    `json:"actor,omitempty"`
	Reason string    `json:"reason,omitempty"`
	Seq    uint64    `json:"seq,omitempty"`
}

// Manager is the V1 access-lifecycle
// store. Manager is goroutine-safe.
type Manager struct {
	mu       sync.Mutex
	subjects map[string]Subject
	history  map[string][]HistoryEntry
	clock    func() time.Time
	seq      uint64
}

// NewManager returns an empty manager.
func NewManager(clock func() time.Time) *Manager {
	if clock == nil {
		clock = time.Now
	}
	return &Manager{subjects: map[string]Subject{}, history: map[string][]HistoryEntry{}, clock: clock}
}

func (m *Manager) nextSeq() uint64 {
	m.seq++
	return m.seq
}

// Add inserts a new subject. The action
// is recorded in the history as "created".
func (m *Manager) Add(s Subject) error {
	if err := s.Valid(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.subjects[s.ID]; ok {
		return errors.New("accesslifecycle: id already exists")
	}
	if s.CreatedAt.IsZero() {
		s.CreatedAt = m.clock().UTC()
	} else {
		s.CreatedAt = s.CreatedAt.UTC()
	}
	m.subjects[s.ID] = s
	m.history[s.ID] = append(m.history[s.ID], HistoryEntry{At: s.CreatedAt, Action: "created", Actor: s.Owner, Seq: m.nextSeq()})
	return nil
}

// Rotate marks the subject as rotated at
// the current clock. The action is
// recorded in the history.
func (m *Manager) Rotate(id, actor string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.subjects[id]
	if !ok {
		return errors.New("accesslifecycle: unknown id")
	}
	s.LastRotated = m.clock().UTC()
	m.subjects[id] = s
	m.history[id] = append(m.history[id], HistoryEntry{At: s.LastRotated, Action: "rotated", Actor: actor, Seq: m.nextSeq()})
	return nil
}

// Revoke marks the subject as revoked.
// The action is recorded in the history.
// After Revoke, Active() returns false.
func (m *Manager) Revoke(id, actor, reason string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.subjects[id]
	if !ok {
		return errors.New("accesslifecycle: unknown id")
	}
	if !s.RevokedAt.IsZero() {
		return errors.New("accesslifecycle: already revoked")
	}
	s.RevokedAt = m.clock().UTC()
	m.subjects[id] = s
	m.history[id] = append(m.history[id], HistoryEntry{At: s.RevokedAt, Action: "revoked", Actor: actor, Reason: reason, Seq: m.nextSeq()})
	return nil
}

// ExpireSweep walks the active subjects
// and records "expired" in the history for
// every subject whose ExpiresAt has passed.
// The function returns the IDs that were
// expired.
func (m *Manager) ExpireSweep(t time.Time) []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []string{}
	for id, s := range m.subjects {
		if s.RevokedAt.IsZero() == false {
			continue
		}
		if s.ExpiresAt.IsZero() {
			continue
		}
		if t.Before(s.ExpiresAt) {
			continue
		}
		m.history[id] = append(m.history[id], HistoryEntry{At: t.UTC(), Action: "expired", Seq: m.nextSeq()})
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// Get returns a copy of the subject.
func (m *Manager) Get(id string) (Subject, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.subjects[id]
	return s, ok
}

// List returns every subject sorted by id.
func (m *Manager) List() []Subject {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Subject, 0, len(m.subjects))
	for _, s := range m.subjects {
		out = append(out, s)
	}
	sort.Slice(out, func(i, k int) bool { return out[i].ID < out[k].ID })
	return out
}

// ActiveAt returns the active subjects at
// t, sorted by id.
func (m *Manager) ActiveAt(t time.Time) []Subject {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []Subject{}
	for _, s := range m.subjects {
		if s.Active(t) {
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, k int) bool { return out[i].ID < out[k].ID })
	return out
}

// History returns a copy of the history
// for a subject, newest first.
func (m *Manager) History(id string) []HistoryEntry {
	m.mu.Lock()
	defer m.mu.Unlock()
	h := m.history[id]
	out := make([]HistoryEntry, len(h))
	copy(out, h)
	sort.SliceStable(out, func(i, k int) bool {
		if !out[i].At.Equal(out[k].At) {
			return out[i].At.After(out[k].At)
		}
		return out[i].Seq > out[k].Seq
	})
	return out
}
