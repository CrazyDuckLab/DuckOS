// scheduler/scheduler.go: V1 typed
// scheduler / automation primitive. The
// package owns the contract for scheduled
// jobs (temporary firewall rules, scheduled
// access, scheduled WAN behaviour,
// maintenance windows, timed tunnel/peer,
// PoE cycle policy, scheduled diagnostics).
//
// The runtime is a thin mapping to a
// in-process tick scheduler; the package
// never spawns shell.
package scheduler

import (
	"errors"
	"regexp"
	"sort"
	"sync"
	"time"
)

// JobKind is the class of scheduled job.
type JobKind string

const (
	JobFirewallRule JobKind = "firewall-rule"
	JobAccessGrant  JobKind = "access-grant"
	JobWANBehavior  JobKind = "wan-behavior"
	JobMaintenance  JobKind = "maintenance"
	JobTunnelPeer   JobKind = "tunnel-peer"
	JobPoECycle     JobKind = "poe-cycle"
	JobDiagnostics  JobKind = "diagnostics"
)

// Valid reports whether k is known.
func (k JobKind) Valid() bool {
	switch k {
	case JobFirewallRule, JobAccessGrant, JobWANBehavior, JobMaintenance,
		JobTunnelPeer, JobPoECycle, JobDiagnostics:
		return true
	}
	return false
}

// Schedule is the cron-style or one-shot
// schedule. The Cron field uses the V1
// "M H DoM Mon DoW" 5-field syntax; the
// OneShot field is set for one-time jobs.
type Schedule struct {
	Cron    string    `json:"cron,omitempty"`
	OneShot time.Time `json:"oneShot,omitempty"`
}

// Valid reports whether s is well-formed.
func (s Schedule) Valid() error {
	if s.Cron == "" && s.OneShot.IsZero() {
		return errors.New("scheduler: cron or oneShot is required")
	}
	if s.Cron != "" {
		if err := validateCron(s.Cron); err != nil {
			return err
		}
	}
	if s.OneShot.IsZero() == false && s.OneShot.Before(time.Now()) {
		return errors.New("scheduler: oneShot must be in the future")
	}
	return nil
}

// Job is a single scheduled job.
type Job struct {
	ID         string            `json:"id"`
	Kind       JobKind           `json:"kind"`
	Schedule   Schedule          `json:"schedule"`
	Payload    map[string]string `json:"payload,omitempty"`
	Operator   string            `json:"operator"`
	Enabled    bool              `json:"enabled"`
	CreatedAt  time.Time         `json:"createdAt"`
	LastRun    time.Time         `json:"lastRun,omitempty"`
	LastResult string            `json:"lastResult,omitempty"`
	ExpiresAt  time.Time         `json:"expiresAt,omitempty"`
}

var idPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:-]{0,63}$`)

// Valid reports whether j is well-formed.
func (j Job) Valid() error {
	if !idPattern.MatchString(j.ID) {
		return errors.New("scheduler: id is not canonical")
	}
	if !j.Kind.Valid() {
		return errors.New("scheduler: kind is unknown")
	}
	if err := j.Schedule.Valid(); err != nil {
		return err
	}
	if j.Operator == "" {
		return errors.New("scheduler: operator is required")
	}
	if !j.ExpiresAt.IsZero() && !j.ExpiresAt.After(j.CreatedAt) {
		return errors.New("scheduler: expiresAt must be after createdAt")
	}
	return nil
}

// Manager is the V1 scheduler store.
// Manager is goroutine-safe.
type Manager struct {
	mu    sync.Mutex
	jobs  map[string]Job
	clock func() time.Time
}

// NewManager returns an empty manager.
func NewManager(clock func() time.Time) *Manager {
	if clock == nil {
		clock = time.Now
	}
	return &Manager{jobs: map[string]Job{}, clock: clock}
}

// Add inserts or replaces a job.
func (m *Manager) Add(j Job) error {
	if err := j.Valid(); err != nil {
		return err
	}
	if j.CreatedAt.IsZero() {
		j.CreatedAt = m.clock().UTC()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.jobs[j.ID] = j
	return nil
}

// Remove drops a job.
func (m *Manager) Remove(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.jobs[id]; !ok {
		return errors.New("scheduler: unknown id")
	}
	delete(m.jobs, id)
	return nil
}

// Disable flips a job's Enabled flag.
func (m *Manager) Disable(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	j, ok := m.jobs[id]
	if !ok {
		return errors.New("scheduler: unknown id")
	}
	j.Enabled = false
	m.jobs[id] = j
	return nil
}

// Get returns a copy of the job.
func (m *Manager) Get(id string) (Job, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	j, ok := m.jobs[id]
	return j, ok
}

// List returns every job sorted by id.
func (m *Manager) List() []Job {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Job, 0, len(m.jobs))
	for _, j := range m.jobs {
		out = append(out, j)
	}
	sort.Slice(out, func(i, k int) bool { return out[i].ID < out[k].ID })
	return out
}

// TickResult is the result of a single
// scheduler tick. The runtime supplies the
// actual job execution; the manager only
// tracks which jobs are due.
type TickResult struct {
	Now      time.Time `json:"now"`
	Due      []string  `json:"due"`
	Expired  []string  `json:"expired"`
	Disabled []string  `json:"disabled"`
}

// Tick evaluates the schedule against the
// supplied clock and returns the jobs that
// are due, the jobs that have expired, and
// the jobs that the operator disabled but
// have not yet been pruned. Disabled jobs
// are reported but not executed.
func (m *Manager) Tick(now time.Time) TickResult {
	m.mu.Lock()
	defer m.mu.Unlock()
	r := TickResult{Now: now.UTC()}
	for _, j := range m.jobs {
		if j.ExpiresAt.IsZero() == false && !now.Before(j.ExpiresAt) {
			r.Expired = append(r.Expired, j.ID)
			continue
		}
		if !j.Enabled {
			r.Disabled = append(r.Disabled, j.ID)
			continue
		}
		if due, ok := isDue(j, now); ok && due {
			r.Due = append(r.Due, j.ID)
		}
	}
	sort.Strings(r.Due)
	sort.Strings(r.Expired)
	sort.Strings(r.Disabled)
	return r
}

func isDue(j Job, now time.Time) (bool, bool) {
	if !j.Schedule.OneShot.IsZero() {
		return now.After(j.Schedule.OneShot) || now.Equal(j.Schedule.OneShot), true
	}
	if j.Schedule.Cron != "" {
		// Minimal cron: support "* * * * *"
		// and "M * * * *" forms; sufficient
		// for the V1 contract (the runtime
		// is expected to integrate with a
		// real cron parser later).
		return cronMatches(j.Schedule.Cron, now), true
	}
	return false, false
}

// validateCron performs a structural check
// on a 5-field cron expression. Field-level
// validation is intentionally permissive
// for V1; the runtime is expected to use a
// real cron parser when scheduling.
func validateCron(c string) error {
	parts := splitFields(c, 5)
	if parts == nil {
		return errors.New("scheduler: cron must have 5 fields")
	}
	return nil
}

func splitFields(s string, n int) []string {
	out := []string{}
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == ' ' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	out = append(out, s[start:])
	if len(out) != n {
		return nil
	}
	return out
}

func cronMatches(cron string, t time.Time) bool {
	parts := splitFields(cron, 5)
	if parts == nil {
		return false
	}
	if !matchField(parts[0], t.Minute(), 0, 59) {
		return false
	}
	if !matchField(parts[1], t.Hour(), 0, 23) {
		return false
	}
	if !matchField(parts[2], t.Day(), 1, 31) {
		return false
	}
	if !matchField(parts[3], int(t.Month()), 1, 12) {
		return false
	}
	if !matchField(parts[4], int(t.Weekday()), 0, 6) {
		return false
	}
	return true
}

func matchField(field string, val, lo, hi int) bool {
	if field == "*" {
		return true
	}
	for i := lo; i <= hi; i++ {
		if field == itoa(i) {
			return val == i
		}
	}
	return false
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [12]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
