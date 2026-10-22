// Package xray / diagnostics.go: structured, actionable diagnostics.
//
// A Diagnostic is the answer to "why is X not working" in data:
// a stable code, the human summary, the evidence actually observed
// (with timestamps, never invented), and the suggested remediation.
// The explainers correlate configured state with observed evidence
// from the event log; they never leak credential material.
package xray

import (
	"fmt"
	"sort"
	"time"
)

// Diagnostic is one structured finding.
type Diagnostic struct {
	// Code is stable and machine-readable ("inbound.no-users").
	Code string `json:"code"`
	// Severity: info | warning | critical.
	Severity string `json:"severity"`
	// Summary is the plain-language answer.
	Summary string `json:"summary"`
	// Evidence carries the observed facts (timestamps, counters,
	// event snippets). Empty evidence means "no signal observed".
	Evidence []string `json:"evidence,omitempty"`
	// Remediation is the suggested next step.
	Remediation string `json:"remediation"`
	// CheckedAt is when the diagnostic ran.
	CheckedAt time.Time `json:"checkedAt"`
}

// Diagnostician computes structured diagnostics from configured
// state and observed evidence. All inputs are optional; a nil input
// degrades the answer instead of crashing it.
type Diagnostician struct {
	Servers func() []Server
	Users   func() []User
	Events  *EventLog
	// StaleStats reports the accountant's last collection state.
	StaleStats func() (time.Time, bool)
	Now        func() time.Time
}

// Diagnose explains the state of one subject: a server id, a user
// id, "stats" or "" (fleet overview). Multiple findings may apply.
func (d *Diagnostician) Diagnose(subject string) ([]Diagnostic, error) {
	now := time.Now().UTC()
	if d.Now != nil {
		now = d.Now().UTC()
	}
	var out []Diagnostic
	switch {
	case subject == "" || subject == "stats":
		out = append(out, d.diagnoseStats(now)...)
		fallthrough
	case subject == "":
		out = append(out, d.diagnoseFleet(now)...)
	case d.lookupServer(subject) != nil:
		out = append(out, d.diagnoseInbound(*d.lookupServer(subject), now)...)
	case d.lookupUser(subject) != nil:
		out = append(out, d.diagnoseUser(*d.lookupUser(subject), now)...)
	default:
		return nil, fmt.Errorf("diagnostics: subject %q is unknown", subject)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Severity != out[j].Severity {
			return severityRank(out[i].Severity) > severityRank(out[j].Severity)
		}
		return out[i].Code < out[j].Code
	})
	return out, nil
}

func severityRank(s string) int {
	switch s {
	case "critical":
		return 3
	case "warning":
		return 2
	default:
		return 1
	}
}

func (d *Diagnostician) lookupServer(id string) *Server {
	if d.Servers == nil {
		return nil
	}
	for i := range d.Servers() {
		if d.Servers()[i].ID == id {
			s := d.Servers()[i]
			return &s
		}
	}
	return nil
}

func (d *Diagnostician) lookupUser(id string) *User {
	if d.Users == nil {
		return nil
	}
	for i := range d.Users() {
		if d.Users()[i].ID == id {
			u := d.Users()[i]
			return &u
		}
	}
	return nil
}

func (d *Diagnostician) diagnoseInbound(s Server, now time.Time) []Diagnostic {
	var out []Diagnostic
	if !s.Enabled {
		out = append(out, Diagnostic{Code: "inbound.disabled", Severity: "info",
			Summary:     fmt.Sprintf("inbound %s is configured but disabled", s.Name),
			Remediation: "enable the server to include it in the rendered runtime config",
			CheckedAt:   now})
	}
	users := d.Users()
	matched := 0
	for _, u := range users {
		if u.ServerID == s.ID && u.Enabled && !u.IsExpired(now) {
			matched++
		}
	}
	if s.Enabled && matched == 0 {
		out = append(out, Diagnostic{Code: "inbound.no-users", Severity: "warning",
			Summary:     fmt.Sprintf("inbound %s has no enabled, unexpired clients; the runtime renders no listener for it", s.Name),
			Remediation: "add an enabled client or accept that this inbound stays down",
			CheckedAt:   now})
	}
	// Event evidence: rejections and restarts around this subject.
	if d.Events != nil {
		rejects := d.Events.Query([]EventKind{EventConfigRejected}, s.ID, now.Add(-24*time.Hour), 10)
		for _, e := range rejects {
			out = append(out, Diagnostic{Code: "inbound.config-rejected", Severity: "critical",
				Summary:     "the last runtime apply rejected the configuration",
				Evidence:    []string{e.At.Format(time.RFC3339) + ": " + e.Message},
				Remediation: "fix the reported field; the previous good config kept running",
				CheckedAt:   now})
			break
		}
		restarts := d.Events.Query([]EventKind{EventRestart}, s.ID, now.Add(-24*time.Hour), 10)
		if len(restarts) > 3 {
			out = append(out, Diagnostic{Code: "inbound.restart-loop", Severity: "warning",
				Summary:     fmt.Sprintf("%d restarts in 24h suggest an unstable runtime", len(restarts)),
				Evidence:    []string{"latest: " + restarts[0].At.Format(time.RFC3339)},
				Remediation: "check the crash reason in the runtime log before re-applying",
				CheckedAt:   now})
		}
	}
	if len(out) == 0 {
		out = append(out, Diagnostic{Code: "inbound.ok", Severity: "info",
			Summary:     fmt.Sprintf("inbound %s is enabled with %d active client(s); no adverse events in 24h", s.Name, matched),
			Remediation: "no action needed", CheckedAt: now})
	}
	return out
}

func (d *Diagnostician) diagnoseUser(u User, now time.Time) []Diagnostic {
	var out []Diagnostic
	switch {
	case !u.Enabled:
		out = append(out, Diagnostic{Code: "user.suspended", Severity: "warning",
			Summary:     "the client is suspended; new connections are rejected by the rendered config",
			Remediation: "re-enable the client to restore access on the next apply",
			CheckedAt:   now})
	case u.IsExpired(now):
		out = append(out, Diagnostic{Code: "user.expired", Severity: "warning",
			Summary:     "the client is past its expiration timestamp",
			Remediation: "extend expiresAt to reactivate, or delete the client",
			CheckedAt:   now})
	default:
		out = append(out, Diagnostic{Code: "user.ok", Severity: "info",
			Summary:     "the client is enabled and unexpired",
			Remediation: "no action needed", CheckedAt: now})
	}
	return out
}

func (d *Diagnostician) diagnoseStats(now time.Time) []Diagnostic {
	if d.StaleStats == nil {
		return nil
	}
	at, stale := d.StaleStats()
	if !stale {
		return []Diagnostic{{
			Code: "stats.ok", Severity: "info",
			Summary:     "traffic statistics are current",
			Remediation: "no action needed", CheckedAt: now}}
	}
	return []Diagnostic{{
		Code: "stats.stale", Severity: "warning",
		Summary:     "the last stats collection failed; usage numbers are not current",
		Evidence:    []string{"last successful collection: " + at.Format(time.RFC3339)},
		Remediation: "verify the xray binary and the loopback stats listener (default 127.0.0.1:10085)",
		CheckedAt:   now,
	}}
}

func (d *Diagnostician) diagnoseFleet(now time.Time) []Diagnostic {
	var out []Diagnostic
	if d.Servers != nil && len(d.Servers()) == 0 {
		out = append(out, Diagnostic{Code: "fleet.empty", Severity: "info",
			Summary:     "no Xray servers are configured",
			Remediation: "create a server to start serving VPN clients", CheckedAt: now})
	}
	if d.Events != nil {
		unreachable := d.Events.Query([]EventKind{EventNodeUnreachable}, "", now.Add(-1*time.Hour), 10)
		if len(unreachable) > 0 {
			out = append(out, Diagnostic{Code: "fleet.node-unreachable", Severity: "warning",
				Summary:     "at least one remote node missed its health checks in the last hour",
				Evidence:    []string{unreachable[0].Subject + " at " + unreachable[0].At.Format(time.RFC3339)},
				Remediation: "check the node's network path; fleet reconciliation skipped it",
				CheckedAt:   now})
		}
	}
	return out
}
