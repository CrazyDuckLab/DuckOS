// Package engine readiness surface (G04): per-feature
// availability, hardware requirement, credential
// requirement, evidence timestamp. UI / CLI / API consume
// this to render honest "supported / unsupported /
// hardware-pending" affordances without fabricating
// availability from package presence.
package engine

import (
	"context"
	"sort"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/chaos"
)

// ReadinessSnapshot is the public projection of the chaos
// readiness store. It carries every feature that has been
// explicitly classified; unknown features surface as
// "unknown" rather than being silently omitted.
type ReadinessSnapshot struct {
	Version   int                       `json:"version"`
	UpdatedAt time.Time                 `json:"updatedAt"`
	Features  []ReadinessFeatureSummary `json:"features"`
	Counters  ReadinessCounters         `json:"counters"`
}

// ReadinessFeatureSummary is one row of the readiness
// manifest. It carries the evidence timestamp the operator
// needs to decide whether the row is fresh, the reason
// string for any unsupported / hardware-pending state, and
// the canonical state name (see chaos.Readiness constants).
type ReadinessFeatureSummary struct {
	Feature   string    `json:"feature"`
	State     string    `json:"state"`
	Note      string    `json:"note,omitempty"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// ReadinessCounters summarises the snapshot so the UI does
// not have to count rows client-side.
type ReadinessCounters struct {
	Supported         int `json:"supported"`
	Experimental      int `json:"experimental"`
	HardwarePending   int `json:"hardwarePending"`
	CredentialPending int `json:"credentialPending"`
	Unsupported       int `json:"unsupported"`
	Degraded          int `json:"degraded"`
	Total             int `json:"total"`
}

// Readiness returns the canonical readiness snapshot for
// every feature that has been explicitly classified by the
// agent. The primary source is the privileged Agent IPC
// (G04 write side); the local readiness store is used as a
// fallback for unit tests that do not boot an Agent. Rows
// are sorted by feature name so the output is deterministic
// across calls.
func (e *Engine) Readiness(ctx context.Context) (ReadinessSnapshot, error) {
	if e == nil {
		return ReadinessSnapshot{}, nil
	}
	var rows []chaos.FeatureReadiness
	if fetched, err := e.fetchReadiness(ctx); err == nil {
		rows = fetched
	} else if e.readiness != nil {
		rows = e.readiness.All()
	} else {
		return ReadinessSnapshot{}, err
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Feature < rows[j].Feature })
	snap := ReadinessSnapshot{
		Version:   1,
		UpdatedAt: time.Now().UTC(),
		Features:  make([]ReadinessFeatureSummary, 0, len(rows)),
	}
	for _, row := range rows {
		snap.Features = append(snap.Features, ReadinessFeatureSummary{
			Feature:   row.Feature,
			State:     string(row.State),
			Note:      row.Note,
			UpdatedAt: row.UpdatedAt,
		})
		switch row.State {
		case chaos.ReadySupported:
			snap.Counters.Supported++
		case chaos.ReadyExperimental:
			snap.Counters.Experimental++
		case chaos.ReadyHWPending:
			snap.Counters.HardwarePending++
		case chaos.ReadyCredPending:
			snap.Counters.CredentialPending++
		case chaos.ReadyUnsupported:
			snap.Counters.Unsupported++
		case chaos.ReadyDegraded:
			snap.Counters.Degraded++
		}
	}
	snap.Counters.Total = len(rows)
	return snap, nil
}
