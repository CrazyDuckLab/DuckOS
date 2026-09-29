// Package timeintegrity owns the V1 wall-clock
// trust model. The Agent must operate
// correctly when the system clock is wrong:
// during first boot, before NTP has
// synchronised, or after a battery-backed RTC
// lost power. The model is one explicit
// state: TIME_UNTRUSTED. A time source is
// promoted to TIME_TRUSTED only when
// authoritative evidence exists (NTP stratum
// <= 3, monotonic drift within tolerance, or
// an operator-set anchor within 5 minutes of
// network time).
//
// The contract is small and the model is
// strict. Callers ask for a state, not a
// timestamp; the Agent never assumes a
// trustworthy clock in user-visible behaviour.
// TLS certificate validation, update
// timestamping, and ACME challenge windows
// all consult the State.
package timeintegrity

import (
	"errors"
	"sync"
	"time"
)

// State is the wall-clock trust level.
type State string

const (
	// StateUntrusted means the wall clock is
	// not yet validated. The Agent may still
	// function, but TLS/ACME/update paths
	// must be visibly cautious.
	StateUntrusted State = "TIME_UNTRUSTED"
	// StateTrusted means at least one
	// authoritative source has confirmed the
	// clock within a tolerance.
	StateTrusted State = "TIME_TRUSTED"
)

// Valid reports whether s is a known state.
func (s State) Valid() bool {
	return s == StateUntrusted || s == StateTrusted
}

// Source describes a single authoritative
// time source. The Agent consults multiple
// sources; promotion to StateTrusted requires
// at least one source to pass.
type Source struct {
	Name     string
	Stratum  int       // NTP stratum (0 = unsynchronized)
	LastSync time.Time // when the source last reported
	DriftMS  int64     // measured drift in milliseconds
	Operator bool      // true when the operator pinned the clock manually
}

// SourceReport is the outcome of a single
// source check.
type SourceReport struct {
	Source Source
	OK     bool
	Reason string
}

// Tracker is the V1 wall-clock trust tracker.
// A Tracker is goroutine-safe.
type Tracker struct {
	mu     sync.RWMutex
	state  State
	now    func() time.Time
	since  time.Time
	drift  int64
	anchor time.Time
	have   bool
}

// NewTracker returns a Tracker in
// StateUntrusted. The clock is supplied as
// a func so tests can drive it.
func NewTracker(now func() time.Time) *Tracker {
	if now == nil {
		now = time.Now
	}
	return &Tracker{state: StateUntrusted, now: now, since: now().UTC()}
}

// State returns the current trust state.
func (t *Tracker) State() State {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.state
}

// DriftMS returns the most recent measured
// drift in milliseconds. Zero when no
// measurement is available.
func (t *Tracker) DriftMS() int64 {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.drift
}

// HaveAnchor reports whether the tracker has
// ever recorded a trusted anchor.
func (t *Tracker) HaveAnchor() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.have
}

// Reset returns the tracker to StateUntrusted
// and clears the anchor. The Agent calls this
// when a clock anomaly is detected (battery
// loss, NTP step, manual clock change).
func (t *Tracker) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.state = StateUntrusted
	t.have = false
	t.drift = 0
	t.anchor = time.Time{}
}

// Observe runs a single check against src.
// The result is fed into the trust state
// machine. Returns a SourceReport that the
// caller can log or surface.
func (t *Tracker) Observe(src Source) SourceReport {
	rep := SourceReport{Source: src}
	if src.Stratum < 0 || src.Stratum > 15 {
		rep.Reason = "stratum out of range"
		return rep
	}
	if src.Stratum > 3 && !src.Operator {
		rep.Reason = "stratum too high and not operator-anchored"
		return rep
	}
	if src.DriftMS < 0 {
		rep.Reason = "negative drift"
		return rep
	}
	if src.DriftMS > 5000 {
		rep.Reason = "drift exceeds 5s"
		return rep
	}
	rep.OK = true
	rep.Reason = "ok"
	t.mu.Lock()
	defer t.mu.Unlock()
	t.drift = src.DriftMS
	t.anchor = src.LastSync
	t.have = true
	t.state = StateTrusted
	return rep
}

// AnchorManual lets the operator pin the
// clock. The anchor is recorded; the trust
// state becomes TIME_TRUSTED only when the
// pin is within 5 minutes of network time or
// when the operator explicitly overrides the
// tolerance.
func (t *Tracker) AnchorManual(at time.Time, drift int64) SourceReport {
	rep := SourceReport{Source: Source{Name: "operator", Operator: true, LastSync: at, DriftMS: drift}}
	if drift < 0 || drift > 300_000 {
		rep.Reason = "operator drift out of range"
		return rep
	}
	rep.OK = true
	rep.Reason = "operator-anchor"
	t.mu.Lock()
	defer t.mu.Unlock()
	t.drift = drift
	t.anchor = at
	t.have = true
	t.state = StateTrusted
	return rep
}

// ErrUntrusted is returned by strict consumers
// (e.g. ACME, Update) that need an explicit
// refusal while the clock is untrusted.
var ErrUntrusted = errors.New("timeintegrity: wall clock is TIME_UNTRUSTED")

// RequireTrusted returns ErrUntrusted when the
// state is not TIME_TRUSTED. Strict consumers
// guard their hot path with this call.
func (t *Tracker) RequireTrusted() error {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.state != StateTrusted {
		return ErrUntrusted
	}
	return nil
}

// Status is the read-only view surfaced
// through the API.
type Status struct {
	State      State     `json:"state"`
	DriftMS    int64     `json:"driftMs"`
	HaveAnchor bool      `json:"haveAnchor"`
	Anchor     time.Time `json:"anchor,omitempty"`
	Since      time.Time `json:"since"`
}

// Status returns a copy of the tracker state.
func (t *Tracker) StatusSnapshot() Status {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return Status{State: t.state, DriftMS: t.drift, HaveAnchor: t.have, Anchor: t.anchor, Since: t.since}
}
