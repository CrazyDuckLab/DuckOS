// Package xray / enforce.go: the enforcement loop that connects
// observed traffic to catalog state.
//
// Loop: Collect (injected collector over the Xray stats service)
// → Accountant.Apply (restart-resistant deltas)
// → Engine.Observe (quota decisions from attributed usage)
// → Suspend callback (catalog mutation → the ordinary apply path,
// so a suspended client is actually removed from the rendered
// runtime config on the next apply).
//
// HONESTY CONTRACT: suspension takes effect when the runtime apply
// completes; established Xray sessions are not torn mid-connection
// by the catalog update — new connections stop. The loop records
// every suspension in the event log and never fabricates traffic.
package xray

import (
	"context"
	"fmt"
	"time"
)

// Enforcer is the closed loop.
type Enforcer struct {
	Accountant *Accountant
	Engine     *Engine
	Events     *EventLog
	// Collect produces one stats snapshot (subprocess collector in
	// production; injectable in tests).
	Collect Collector
	// EmailForScope maps a quota scope onto the Xray stats email
	// label (production wiring: the catalog lookup). Without it,
	// per-user usage cannot be attributed and quotas stay advisory.
	EmailForScope func(scopeKind, scopeID string) string
	Now           func() time.Time
}

// Round executes one collection + evaluation pass. It returns the
// decisions taken. A failed collection marks the accountant stale
// and is recorded — it never fabricates usage.
func (e *Enforcer) Round(ctx context.Context) ([]Decision, error) {
	now := time.Now().UTC()
	if e.Now != nil {
		now = e.Now().UTC()
	}
	snap, err := e.Collect(ctx)
	if err != nil {
		_ = e.Accountant.Apply(StatsSnapshot{Stale: true})
		if e.Events != nil {
			_ = e.Events.Record(Event{At: now, Kind: EventStatsStale, Severity: SeverityWarning,
				Source: "enforcer", Message: "stats collection failed"})
		}
		return nil, fmt.Errorf("enforcer: %w", err)
	}
	if err := e.Accountant.Apply(snap); err != nil {
		return nil, fmt.Errorf("enforcer: ledger: %w", err)
	}
	var decisions []Decision
	// Evaluate quotas for every scope that has one; the engine
	// triggers Suspend on exhausted auto-suspend quotas. Usage is
	// attributed through the scope→email mapping; an unmappable
	// scope cannot observe usage and is skipped honestly.
	for _, id := range e.Engine.QuotaIDs() {
		q, ok := e.Engine.Quota(id)
		if !ok {
			continue
		}
		var total uint64
		if e.EmailForScope != nil {
			if email := e.EmailForScope(q.ScopeKind, q.ScopeID); email != "" {
				_, _, total = e.Accountant.UserUsage(email)
			}
		}
		ds, err := e.Engine.Observe(q.ScopeKind, q.ScopeID, total, "enforcer")
		if err != nil && e.Events != nil {
			_ = e.Events.Record(Event{At: now, Kind: EventQuotaExhausted, Severity: SeverityWarning,
				Subject: q.ScopeID, Source: "enforcer",
				Message: "auto-suspend failed for an exhausted quota"})
		}
		for _, d := range ds {
			if d.State == "exhausted" {
				severity := SeverityWarning
				if d.Enforced {
					severity = SeverityCritical
				}
				if e.Events != nil {
					_ = e.Events.Record(Event{At: now, Kind: EventQuotaExhausted, Severity: severity,
						Subject: q.ScopeID, Source: "enforcer",
						Message: fmt.Sprintf("quota exhausted: %d/%d bytes, enforced=%v", d.UsedBytes, d.LimitBytes, d.Enforced)})
				}
			}
			decisions = append(decisions, d)
		}
	}
	return decisions, nil
}
