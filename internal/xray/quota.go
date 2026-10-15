// Package xray / quota.go: traffic quota and expiration policy
// engine.
//
// HONESTY CONTRACT: Xray-core stats are polled counters, not an
// enforcement hook. This engine therefore models quotas and
// produces DECISIONS (ok / warn / suspend) from observed usage; it
// can auto-suspend a credential in the DuckOS catalog (which does
// remove the client on the next apply), but it must never be
// described as hard per-packet enforcement until a runtime-verified
// cutoff exists. The Enforced flag on every Decision keeps that
// distinction explicit in the data model.
package xray

import (
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"
)

// ResetPeriod is the quota renewal cadence.
type ResetPeriod string

const (
	ResetNever   ResetPeriod = "never"
	ResetDaily   ResetPeriod = "daily"
	ResetWeekly  ResetPeriod = "weekly"
	ResetMonthly ResetPeriod = "monthly"
)

// Quota is a traffic budget bound to a catalog object (user,
// credential or subscription id in ScopeID).
type Quota struct {
	ID         string      `json:"id"`
	ScopeKind  string      `json:"scopeKind"` // user|credential|subscription
	ScopeID    string      `json:"scopeId"`
	LimitBytes uint64      `json:"limitBytes"`
	Reset      ResetPeriod `json:"reset"`
	// WarnAtFraction triggers the warn decision (default 0.8).
	WarnAtFraction float64 `json:"warnAtFraction,omitempty"`
	// AutoSuspend lets the engine set Enabled=false on the scoped
	// object when the quota is exhausted.
	AutoSuspend bool      `json:"autoSuspend"`
	CreatedAt   time.Time `json:"createdAt"`
}

// Validate enforces the quota contract.
func (q Quota) Validate() error {
	if q.ID == "" || q.ScopeID == "" || q.ScopeKind == "" {
		return errors.New("quota: id and scope are required")
	}
	switch q.ScopeKind {
	case "user", "credential", "subscription":
	default:
		return fmt.Errorf("quota: scope kind %q is not canonical", q.ScopeKind)
	}
	switch q.Reset {
	case ResetNever, ResetDaily, ResetWeekly, ResetMonthly:
	default:
		return fmt.Errorf("quota: reset period %q is not canonical", q.Reset)
	}
	if q.LimitBytes == 0 {
		return errors.New("quota: limit must be positive")
	}
	return nil
}

// UsageSample is one accounting observation for a scope.
type UsageSample struct {
	At    time.Time `json:"at"`
	Bytes uint64    `json:"bytes"`
	// Source names the collector ("stats:26.3.27"). Samples without
	// a source are refused: the history must stay attributable.
	Source string `json:"source"`
}

// UsageHistory is a bounded, per-scope sample ring.
type UsageHistory struct {
	mu      sync.Mutex
	samples map[string][]UsageSample
	max     int
}

const MaxSamplesPerScope = 256

func NewUsageHistory() *UsageHistory {
	return &UsageHistory{samples: map[string][]UsageSample{}, max: MaxSamplesPerScope}
}

// Record appends a sample, attributing it to the counter reset
// window that contains it. Unattributable samples (no source) are
// refused so history can never contain invented values.
func (h *UsageHistory) Record(scopeID string, s UsageSample) error {
	if s.Source == "" || s.At.IsZero() {
		return errors.New("quota: usage sample needs a source and a timestamp")
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	ring := append(h.samples[scopeID], s)
	if len(ring) > h.max {
		ring = ring[len(ring)-h.max:]
	}
	h.samples[scopeID] = ring
	return nil
}

// Since returns the samples inside the current reset window ending
// at now for the given period.
func (h *UsageHistory) Since(scopeID string, period ResetPeriod, now time.Time) []UsageSample {
	h.mu.Lock()
	defer h.mu.Unlock()
	all := h.samples[scopeID]
	start := windowStart(now, period)
	out := make([]UsageSample, 0, len(all))
	for _, s := range all {
		if !s.At.Before(start) {
			out = append(out, s)
		}
	}
	return out
}

// windowStart computes the start of the reset window containing now.
func windowStart(now time.Time, period ResetPeriod) time.Time {
	now = now.UTC()
	switch period {
	case ResetDaily:
		return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	case ResetWeekly:
		weekday := int(now.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		day := now.AddDate(0, 0, -(weekday - 1))
		return time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC)
	case ResetMonthly:
		return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	default:
		return time.Time{} // never: the whole history counts
	}
}

// Decision is the engine output for one scope.
type Decision struct {
	ScopeID    string `json:"scopeId"`
	State      string `json:"state"` // ok | warn | exhausted
	UsedBytes  uint64 `json:"usedBytes"`
	LimitBytes uint64 `json:"limitBytes"`
	// Enforced is true only when the engine actually disabled the
	// scoped object. Advisory decisions carry false.
	Enforced bool      `json:"enforced"`
	Reason   string    `json:"reason,omitempty"`
	At       time.Time `json:"at"`
}

// Evaluate derives the decision for one quota from observed usage.
func Evaluate(q Quota, used uint64, now time.Time) Decision {
	d := Decision{ScopeID: q.ScopeID, LimitBytes: q.LimitBytes, UsedBytes: used, At: now.UTC()}
	warnAt := q.WarnAtFraction
	if warnAt <= 0 || warnAt >= 1 {
		warnAt = 0.8
	}
	switch {
	case used >= q.LimitBytes:
		d.State = "exhausted"
		d.Reason = "traffic limit reached"
	case float64(used) >= float64(q.LimitBytes)*warnAt:
		d.State = "warn"
	}
	return d
}

// Engine binds quotas, history and a suspension callback into the
// evaluation loop.
type Engine struct {
	mu      sync.RWMutex
	quotas  map[string]Quota
	history *UsageHistory
	// Suspend flips the scoped object disabled in the catalog.
	// Returning an error keeps the decision advisory for this round.
	Suspend func(scopeKind, scopeID string) error
	clock   func() time.Time
}

// SetClock overrides the wall clock (testing).
func (e *Engine) SetClock(fn func() time.Time) { e.clock = fn }

func NewEngine(history *UsageHistory) *Engine {
	if history == nil {
		history = NewUsageHistory()
	}
	return &Engine{quotas: map[string]Quota{}, history: history, clock: time.Now}
}

// SetQuota upserts a quota.
func (e *Engine) SetQuota(q Quota) error {
	if err := q.Validate(); err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if q.CreatedAt.IsZero() {
		q.CreatedAt = e.clock().UTC()
	}
	e.quotas[q.ID] = q
	return nil
}

// Quota returns a quota by id.
func (e *Engine) Quota(id string) (Quota, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	q, ok := e.quotas[id]
	return q, ok
}

// QuotaIDs lists the managed quota ids sorted.
func (e *Engine) QuotaIDs() []string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := make([]string, 0, len(e.quotas))
	for id := range e.quotas {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// Observe records one usage sample and evaluates every quota of the
// scope. Exhausted quotas with AutoSuspend attempt suspension; the
// decision's Enforced flag reports whether that succeeded.
func (e *Engine) Observe(scopeKind, scopeID string, bytes uint64, source string) ([]Decision, error) {
	now := e.clock().UTC()
	if err := e.history.Record(scopeID, UsageSample{At: now, Bytes: bytes, Source: source}); err != nil {
		return nil, err
	}
	var decisions []Decision
	e.mu.RLock()
	var scoped []Quota
	for _, q := range e.quotas {
		if q.ScopeID == scopeID {
			scoped = append(scoped, q)
		}
	}
	e.mu.RUnlock()
	var firstErr error
	for _, q := range scoped {
		var used uint64
		for _, s := range e.history.Since(scopeID, q.Reset, now) {
			used += s.Bytes
		}
		d := Evaluate(q, used, now)
		if d.State == "exhausted" && q.AutoSuspend && e.Suspend != nil {
			if err := e.Suspend(q.ScopeKind, q.ScopeID); err != nil {
				d.Reason += "; auto-suspend failed: " + err.Error()
				firstErr = err
			} else {
				d.Enforced = true
			}
		}
		decisions = append(decisions, d)
	}
	return decisions, firstErr
}
