// Package xray / insights.go: bounded event history for the Xray
// Center (the "Insights" substrate).
//
// The log records actual observations only — restarts, config
// changes, certificate expirations, subscription deliveries,
// collection failures — each with a real timestamp and the source
// that reported it. Nothing here generates or extrapolates values.
// Retention is bounded two ways (count and age) so low-power
// hardware never accumulates unbounded history.
package xray

import (
	"errors"
	"sort"
	"sync"
	"time"
)

// EventKind enumerates the observation types.
type EventKind string

const (
	EventRestart         EventKind = "restart"
	EventConfigChange    EventKind = "config-change"
	EventConfigRejected  EventKind = "config-rejected"
	EventCertExpired     EventKind = "cert-expired"
	EventCertRenewed     EventKind = "cert-renewed"
	EventSubDelivered    EventKind = "subscription-delivered"
	EventSubRevoked      EventKind = "subscription-revoked"
	EventQuotaExhausted  EventKind = "quota-exhausted"
	EventStatsStale      EventKind = "stats-stale"
	EventNodeUnreachable EventKind = "node-unreachable"
	EventNodeRecovered   EventKind = "node-recovered"
)

// Severity classifies events for the diagnostics view.
type Severity string

const (
	SeverityInfo     Severity = "info"
	SeverityWarning  Severity = "warning"
	SeverityCritical Severity = "critical"
)

// Event is one observation.
type Event struct {
	At       time.Time `json:"at"`
	Kind     EventKind `json:"kind"`
	Severity Severity  `json:"severity"`
	// Subject is the object id (server, user, node, subscription...).
	Subject string `json:"subject,omitempty"`
	// Source names the reporter ("stats:26.3.27", "agent", ...).
	Source string `json:"source"`
	// Message is a bounded human summary. It must never carry
	// secret material; callers redact before recording.
	Message string `json:"message,omitempty"`
}

// Validate refuses unattributable or oversized events.
func (e Event) Validate() error {
	if e.At.IsZero() {
		return errors.New("insights: event needs a timestamp")
	}
	if e.Source == "" {
		return errors.New("insights: event needs a reporting source")
	}
	if len(e.Message) > 512 {
		return errors.New("insights: message exceeds 512 bytes")
	}
	return nil
}

// EventLog is a bounded ring with age-based pruning.
type EventLog struct {
	mu       sync.Mutex
	events   []Event
	maxCount int
	maxAge   time.Duration
	clock    func() time.Time
	persist  func([]Event) error
}

const (
	MaxEvents          = 1024
	DefaultEventMaxAge = 7 * 24 * time.Hour
)

func NewEventLog() *EventLog {
	return &EventLog{maxCount: MaxEvents, maxAge: DefaultEventMaxAge, clock: time.Now}
}

// SetPersist installs the durability hook: after every successful
// Record the retained ring is handed to persistence (bounded, so
// the write stays small).
func (l *EventLog) SetPersist(fn func([]Event) error) { l.persist = fn }

// LoadEvents seeds the log from persisted state (startup).
func (l *EventLog) LoadEvents(events []Event) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.events = append(l.events, events...)
	if len(l.events) > l.maxCount {
		l.events = l.events[len(l.events)-l.maxCount:]
	}
}

// Record appends an event and prunes. Recording never fails on
// capacity: the oldest events fall off the ring.
func (l *EventLog) Record(e Event) error {
	if err := e.Validate(); err != nil {
		return err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.clock()
	l.events = append(l.events, e)
	if len(l.events) > l.maxCount {
		l.events = l.events[len(l.events)-l.maxCount:]
	}
	cutoff := now.Add(-l.maxAge)
	kept := l.events[:0]
	for _, ev := range l.events {
		if ev.At.After(cutoff) {
			kept = append(kept, ev)
		}
	}
	l.events = kept
	if l.persist != nil {
		_ = l.persist(append([]Event(nil), l.events...))
	}
	return nil
}

// Query returns events matching the filters, newest first.
// Zero filters match everything; limit caps the result (default 100).
func (l *EventLog) Query(kinds []EventKind, subject string, since time.Time, limit int) []Event {
	l.mu.Lock()
	all := append([]Event(nil), l.events...)
	l.mu.Unlock()
	sort.Slice(all, func(i, j int) bool { return all[i].At.After(all[j].At) })
	kindSet := map[EventKind]bool{}
	for _, k := range kinds {
		kindSet[k] = true
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	out := make([]Event, 0, limit)
	for _, e := range all {
		if len(kindSet) > 0 && !kindSet[e.Kind] {
			continue
		}
		if subject != "" && e.Subject != subject {
			continue
		}
		if !since.IsZero() && e.At.Before(since) {
			continue
		}
		out = append(out, e)
		if len(out) >= limit {
			break
		}
	}
	return out
}

// Count reports the retained event count (observability of the log
// itself).
func (l *EventLog) Count() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.events)
}
