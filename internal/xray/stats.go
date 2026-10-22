// Package xray / stats.go: bounded traffic accounting on top of the
// Xray stats service.
//
// Xray-core exposes per-user and per-inbound counters through its
// stats API (statsService). Counters are monotonic but RESET TO
// ZERO on every process restart, so the accountant never persists
// the raw counter: it persists cumulative deltas. A decrease
// between collections is interpreted as a restart and folded into
// the accumulated totals rather than treated as negative traffic.
//
// The collector is injected: on Linux the production collector runs
// `xray api statsquery` against the loopback stats listener that
// the agent renders into the config when stats are enabled; tests
// inject canned snapshots. Nothing here generates plausible numbers
// — if collection fails, the last observed state is marked Stale.
package xray

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"
)

// StatCounter is one counter from the stats service. Name follows
// the Xray convention: user>>>EMAIL>>>traffic>>>uplink,
// inbound>>>TAG>>>traffic>>>downlink, ...
type StatCounter struct {
	Name  string `json:"name"`
	Value uint64 `json:"value"`
}

// StatsSnapshot is one collection round.
type StatsSnapshot struct {
	Counters    []StatCounter `json:"counters"`
	CollectedAt time.Time     `json:"collectedAt"`
	// Stale marks a failed collection over previous data.
	Stale bool `json:"stale,omitempty"`
}

// Collector produces one snapshot. Production: the xray api
// subprocess; tests: canned data.
type Collector func(ctx context.Context) (StatsSnapshot, error)

// Accountant accumulates traffic across restarts. It is
// goroutine-safe and bounded: counter keys are capped so a hostile
// email label cannot grow the ledger without bound.
type Accountant struct {
	mu      sync.Mutex
	totals  map[string]uint64 // counter name -> accumulated bytes
	lastRaw map[string]uint64 // last observed raw counter
	lastOk  time.Time
	stale   bool
	maxKeys int
	persist func(map[string]uint64) error
}

// NewAccountant builds an accountant. persist, when non-nil, is
// called after every successful collection with the cumulative
// ledger (the caller decides where to store it).
func NewAccountant(maxKeys int, persist func(map[string]uint64) error) *Accountant {
	if maxKeys <= 0 {
		maxKeys = 4096
	}
	return &Accountant{totals: map[string]uint64{}, lastRaw: map[string]uint64{}, maxKeys: maxKeys, persist: persist}
}

// Apply folds one snapshot into the ledger. Decreases are treated
// as restart resets and their full value is added (the pre-restart
// bytes were already accumulated in earlier rounds).
func (a *Accountant) Apply(snap StatsSnapshot) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if snap.Stale {
		a.stale = true
		return nil
	}
	for _, c := range snap.Counters {
		if c.Name == "" {
			continue
		}
		var delta uint64
		if prev, seen := a.lastRaw[c.Name]; seen {
			if c.Value >= prev {
				delta = c.Value - prev
			} else {
				// Counter reset (process restart): the whole new
				// value is new traffic; the pre-restart bytes are
				// already in the ledger.
				delta = c.Value
			}
		} else {
			// First observation: the raw value is the delta.
			delta = c.Value
		}
		a.lastRaw[c.Name] = c.Value
		if _, known := a.totals[c.Name]; !known && len(a.totals) >= a.maxKeys {
			// Ledger full: drop accounting for unknown counters
			// rather than grow unboundedly.
			delete(a.lastRaw, c.Name)
			continue
		}
		a.totals[c.Name] += delta
	}
	a.lastOk = snap.CollectedAt
	a.stale = false
	if a.persist != nil {
		// Synchronous persist: the ledger is bounded (maxKeys) so
		// the write is small; async persistence would race the
		// next Apply and lose ordering guarantees.
		ledger := make(map[string]uint64, len(a.totals))
		for k, v := range a.totals {
			ledger[k] = v
		}
		if err := a.persist(ledger); err != nil {
			return fmt.Errorf("xray: persist traffic ledger: %w", err)
		}
	}
	return nil
}

// Ledger returns a copy of the cumulative ledger.
func (a *Accountant) Ledger() map[string]uint64 {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make(map[string]uint64, len(a.totals))
	for k, v := range a.totals {
		out[k] = v
	}
	return out
}

// UserUsage sums the uplink+downlink bytes accumulated for one
// client email (Xray counter naming: user>>><email>).
func (a *Accountant) UserUsage(email string) (uplink, downlink, total uint64) {
	a.mu.Lock()
	defer a.mu.Unlock()
	uplink = a.totals["user>>>"+email+">>>traffic>>>uplink"]
	downlink = a.totals["user>>>"+email+">>>traffic>>>downlink"]
	total = uplink + downlink
	return uplink, downlink, total
}

// LastCollection reports when the ledger was last refreshed and
// whether the latest round failed (stale observed state — never
// presented as live).
func (a *Accountant) LastCollection() (time.Time, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.lastOk, a.stale
}

// Restore seeds the cumulative ledger from persisted state (call
// before the first Apply).
func (a *Accountant) Restore(ledger map[string]uint64) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for k, v := range ledger {
		a.totals[k] = v
	}
}

// ErrStatsUnavailable is the typed failure when the stats listener
// or the xray binary is unreachable.
var ErrStatsUnavailable = errors.New("xray: stats service unavailable")

// ParseStatsQueryOutput parses the JSON document
// `xray api statsquery` prints: {"stat":[{"name":..., "value":"123"}]}.
func ParseStatsQueryOutput(data []byte) (StatsSnapshot, error) {
	var doc struct {
		Stat []struct {
			Name  string      `json:"name"`
			Value json.Number `json:"value"`
		} `json:"stat"`
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(&doc); err != nil {
		return StatsSnapshot{}, fmt.Errorf("xray: statsquery output: %w", err)
	}
	snap := StatsSnapshot{CollectedAt: time.Now().UTC()}
	for _, s := range doc.Stat {
		v, err := strconv.ParseUint(s.Value.String(), 10, 64)
		if err != nil {
			continue
		}
		snap.Counters = append(snap.Counters, StatCounter{Name: s.Name, Value: v})
	}
	return snap, nil
}
