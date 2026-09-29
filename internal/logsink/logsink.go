// logsink/logsink.go: G14 bounded logging / trace jobs.
//
// The Agent owns a bounded log retention for diagnostic
// trace jobs. A trace job is a short-lived context that
// captures (component, severity, message) entries over
// a fixed window. The UI can render "last 10 minutes of
// network.firewall warnings" without re-querying the
// underlying daemon logs.
//
// The package is pure data: producers feed entries, the
// Store trims by retention, consumers query. No I/O,
// no callbacks, no timers.
//
// Retention:
//   - Per-trace MaxEntries (default 1024)
//   - Global MaxTraces (default 32)
//   - Per-entry TTL MaxAge (default 1 hour)
//
// Older entries and traces fall off the end.
package logsink

import (
	"errors"
	"regexp"
	"sort"
	"sync"
	"time"
)

// SchemaVersion is the on-disk schema version.
const SchemaVersion = 1

// MaxEntriesPerTrace is the default per-trace retention.
const MaxEntriesPerTrace = 1024

// MaxTraces is the default global trace retention.
const MaxTraces = 32

// MaxAge is the default per-entry age ceiling.
const MaxAge = time.Hour

// Severity enumerates the canonical severity levels.
type Severity string

const (
	SeverityTrace   Severity = "trace"
	SeverityDebug   Severity = "debug"
	SeverityInfo    Severity = "info"
	SeverityWarning Severity = "warning"
	SeverityError   Severity = "error"
	SeverityFatal   Severity = "fatal"
)

// Valid reports whether s is a known severity.
func (s Severity) Valid() bool {
	switch s {
	case SeverityTrace, SeverityDebug, SeverityInfo, SeverityWarning, SeverityError, SeverityFatal:
		return true
	}
	return false
}

// Entry is one timestamped log record.
type Entry struct {
	// TraceID identifies the parent trace job.
	TraceID string `json:"traceId"`
	// Component is the canonical subsystem name
	// (e.g. "network.firewall", "vpn.wireguard").
	Component string   `json:"component"`
	Severity  Severity `json:"severity"`
	// Message is the human-facing body. Must not contain
	// shell metacharacters beyond the canonical set.
	Message string    `json:"message"`
	At      time.Time `json:"at"`
	// Fields are structured key/value pairs. Values
	// must be short (max 256 chars).
	Fields map[string]string `json:"fields,omitempty"`
}

// idPattern mirrors the lifecycle id rules.
var idPattern = regexp.MustCompile(`^[a-z][a-z0-9._-]{3,63}$`)
var componentPattern = regexp.MustCompile(`^[a-z][a-z0-9._-]{2,63}$`)

// Validate reports whether e is internally consistent.
func (e Entry) Validate() error {
	if !idPattern.MatchString(e.TraceID) {
		return errors.New("logsink: traceId is not canonical")
	}
	if !componentPattern.MatchString(e.Component) {
		return errors.New("logsink: component is not canonical")
	}
	if !e.Severity.Valid() {
		return errors.New("logsink: severity is not canonical")
	}
	if e.At.IsZero() {
		return errors.New("logsink: at is zero")
	}
	if len(e.Message) > 1024 {
		return errors.New("logsink: message too long")
	}
	for k, v := range e.Fields {
		if len(k) > 64 || len(v) > 256 {
			return errors.New("logsink: field too long")
		}
	}
	return nil
}

// Trace is one diagnostic trace job.
type Trace struct {
	ID        string    `json:"id"`
	Component string    `json:"component"`
	StartedAt time.Time `json:"startedAt"`
	EndedAt   time.Time `json:"endedAt,omitempty"`
	// Reason is the operator-facing explanation.
	Reason string `json:"reason,omitempty"`
	// State is one of "running", "succeeded", "failed".
	State string `json:"state"`
}

// Validate reports whether t is internally consistent.
func (t Trace) Validate() error {
	if !idPattern.MatchString(t.ID) {
		return errors.New("logsink: trace id is not canonical")
	}
	if !componentPattern.MatchString(t.Component) {
		return errors.New("logsink: trace component is not canonical")
	}
	switch t.State {
	case "running", "succeeded", "failed":
	default:
		return errors.New("logsink: trace state is not canonical")
	}
	if t.StartedAt.IsZero() {
		return errors.New("logsink: trace startedAt is zero")
	}
	if len(t.Reason) > 256 {
		return errors.New("logsink: trace reason too long")
	}
	return nil
}

// Store is the goroutine-safe bounded trace + entry store.
type Store struct {
	mu         sync.Mutex
	traces     map[string]Trace
	entries    map[string][]Entry
	maxEntries int
	maxTraces  int
	maxAge     time.Duration
}

// NewStore returns an empty store.
func NewStore() *Store {
	return &Store{
		traces:     map[string]Trace{},
		entries:    map[string][]Entry{},
		maxEntries: MaxEntriesPerTrace,
		maxTraces:  MaxTraces,
		maxAge:     MaxAge,
	}
}

// SetLimits overrides the retention caps.
func (s *Store) SetLimits(maxEntries, maxTraces int, maxAge time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if maxEntries > 0 {
		s.maxEntries = maxEntries
	}
	if maxTraces > 0 {
		s.maxTraces = maxTraces
	}
	if maxAge > 0 {
		s.maxAge = maxAge
	}
}

// StartTrace opens a new trace. The returned bool is
// false when a trace with the same ID already exists; the
// existing trace is left untouched.
func (s *Store) StartTrace(t Trace) (bool, error) {
	if err := t.Validate(); err != nil {
		return false, err
	}
	if t.StartedAt.IsZero() {
		t.StartedAt = time.Now().UTC()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.traces[t.ID]; ok {
		return false, nil
	}
	s.traces[t.ID] = t
	s.entries[t.ID] = nil
	if s.maxTraces > 0 && len(s.traces) > s.maxTraces {
		s.trimTracesLocked()
	}
	return true, nil
}

// EndTrace marks the trace as ended. EndedAt is stamped
// if zero.
func (s *Store) EndTrace(id string, state string) error {
	if state != "succeeded" && state != "failed" {
		return errors.New("logsink: end state must be succeeded or failed")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	trace, ok := s.traces[id]
	if !ok {
		return errors.New("logsink: unknown trace")
	}
	if trace.EndedAt.IsZero() {
		trace.EndedAt = time.Now().UTC()
	}
	trace.State = state
	s.traces[id] = trace
	return nil
}

// Append appends an entry to the supplied trace. The
// store enforces the per-trace ring cap and the per-entry
// age ceiling.
func (s *Store) Append(e Entry) error {
	if err := e.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	trace, ok := s.traces[e.TraceID]
	if !ok {
		return errors.New("logsink: unknown trace")
	}
	if trace.State != "running" {
		return errors.New("logsink: trace is no longer running")
	}
	ring := append(s.entries[e.TraceID], e)
	if s.maxEntries > 0 && len(ring) > s.maxEntries {
		ring = ring[len(ring)-s.maxEntries:]
	}
	s.entries[e.TraceID] = ring
	return nil
}

// Trace returns the trace with the supplied id.
func (s *Store) Trace(id string) (Trace, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.traces[id]
	return t, ok
}

// Entries returns the bounded entry ring for the supplied
// trace. The slice is a copy; mutating it does not affect
// the store.
func (s *Store) Entries(traceID string) []Entry {
	s.mu.Lock()
	defer s.mu.Unlock()
	ring := s.entries[traceID]
	out := make([]Entry, len(ring))
	copy(out, ring)
	return out
}

// AllTraces returns every trace sorted newest first.
func (s *Store) AllTraces() []Trace {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Trace, 0, len(s.traces))
	for _, t := range s.traces {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].StartedAt.Equal(out[j].StartedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].StartedAt.After(out[j].StartedAt)
	})
	return out
}

// GCExpired removes traces whose end is older than the
// configured age ceiling. Returns the count removed.
func (s *Store) GCExpired() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	var removed int
	for id, t := range s.traces {
		if !t.EndedAt.IsZero() && now.Sub(t.EndedAt) > s.maxAge {
			delete(s.traces, id)
			delete(s.entries, id)
			removed++
		}
	}
	return removed
}

// trimTracesLocked removes the oldest traces beyond the
// global cap. Caller must hold s.mu.
func (s *Store) trimTracesLocked() {
	if len(s.traces) <= s.maxTraces {
		return
	}
	type kv struct {
		id  string
		old time.Time
	}
	var all []kv
	for id, t := range s.traces {
		all = append(all, kv{id: id, old: t.StartedAt})
	}
	sort.Slice(all, func(i, j int) bool { return all[i].old.Before(all[j].old) })
	for _, k := range all[:len(all)-s.maxTraces] {
		delete(s.traces, k.id)
		delete(s.entries, k.id)
	}
}

// Counters summarises the store.
type Counters struct {
	Traces  int `json:"traces"`
	Running int `json:"running"`
	Entries int `json:"entries"`
	Expired int `json:"expired"`
}

// Counters returns the current counts.
func (s *Store) Counters() Counters {
	s.mu.Lock()
	defer s.mu.Unlock()
	running := 0
	for _, t := range s.traces {
		if t.State == "running" {
			running++
		}
	}
	entries := 0
	for _, ring := range s.entries {
		entries += len(ring)
	}
	now := time.Now().UTC()
	expired := 0
	for _, t := range s.traces {
		if !t.EndedAt.IsZero() && now.Sub(t.EndedAt) > s.maxAge {
			expired++
		}
	}
	return Counters{Traces: len(s.traces), Running: running, Entries: entries, Expired: expired}
}
