// diagjob/diagjob.go: G19 contextual diagnostic job service.
//
// The Agent owns a bounded diagnostic job store. Jobs are
// short-lived, named, contextual diagnostic runs that the
// operator triggers from the UI or CLI when a specific
// subsystem is misbehaving — e.g. "trace DNS resolution
// for host.example.com for 30 seconds" or "measure WAN
// failover latency over the next minute". The store keeps
// the last MaxJobs records (newest first) so the UI can
// render a compact "recent diagnostics" timeline and the
// CLI can stream the latest run output.
//
// Jobs never touch secrets: the output is sanitized
// through the doctor package's redactor before storage.
// The store never grows unbounded even on long-running
// installations.
package diagjob

import (
	"context"
	"errors"
	"regexp"
	"sort"
	"sync"
	"time"
)

// MaxJobs is the per-store retention ceiling.
const MaxJobs = 64

// MaxOutputBytes is the per-job output cap. Larger
// outputs are truncated and recorded with Truncated=true.
const MaxOutputBytes = 256 << 10

// JobStatus enumerates the lifecycle states.
type JobStatus string

const (
	StatusQueued    JobStatus = "queued"
	StatusRunning   JobStatus = "running"
	StatusSucceeded JobStatus = "succeeded"
	StatusFailed    JobStatus = "failed"
	StatusCanceled  JobStatus = "canceled"
)

// Valid reports whether s is a known status.
func (s JobStatus) Valid() bool {
	switch s {
	case StatusQueued, StatusRunning, StatusSucceeded, StatusFailed, StatusCanceled:
		return true
	}
	return false
}

// JobKind enumerates the canonical job kinds the Agent
// recognises. New kinds must be appended.
type JobKind string

const (
	KindDNSTrace    JobKind = "dns-trace"
	KindWANProbe    JobKind = "wan-probe"
	KindRouteAudit  JobKind = "route-audit"
	KindWiFiSurvey  JobKind = "wifi-survey"
	KindFirewallLog JobKind = "firewall-log"
	KindGenericLog  JobKind = "log-snapshot"
)

// Valid reports whether k is a known kind.
func (k JobKind) Valid() bool {
	switch k {
	case KindDNSTrace, KindWANProbe, KindRouteAudit, KindWiFiSurvey, KindFirewallLog, KindGenericLog:
		return true
	}
	return false
}

// Job is one diagnostic job record.
type Job struct {
	ID string `json:"id"`
	// Kind is the canonical job kind.
	Kind JobKind `json:"kind"`
	// Subject is the operator-provided target (host,
	// interface, service). Optional.
	Subject string `json:"subject,omitempty"`
	// Status is the current lifecycle state.
	Status JobStatus `json:"status"`
	// RequestedBy is the actor (role or system) that
	// triggered the job.
	RequestedBy string `json:"requestedBy,omitempty"`
	// Reason is an operator-facing explanation.
	Reason string `json:"reason,omitempty"`
	// StartedAt is the wall-clock start time (UTC).
	StartedAt time.Time `json:"startedAt,omitempty"`
	// EndedAt is the wall-clock end time (UTC). Zero
	// while running.
	EndedAt time.Time `json:"endedAt,omitempty"`
	// OutputLines is the bounded, sanitized output.
	OutputLines []string `json:"outputLines,omitempty"`
	// Truncated indicates OutputLines was cut to fit.
	Truncated bool `json:"truncated,omitempty"`
	// Error carries the failure reason when Status ==
	// StatusFailed.
	Error string `json:"error,omitempty"`
	// Tags are free-form labels. Stored sorted + deduped.
	Tags []string `json:"tags,omitempty"`
}

// idPattern mirrors the lifecycle object id rules so
// caller-supplied job ids cannot smuggle shell
// metacharacters into the audit log. Minimum length is
// 4 chars to avoid single-letter or numeric-only ids.
var idPattern = regexp.MustCompile(`^[a-z][a-z0-9._-]{3,63}$`)

// Validate ensures the job is internally consistent.
func (j Job) Validate() error {
	if !idPattern.MatchString(j.ID) {
		return errors.New("diagjob: id is not canonical")
	}
	if !j.Kind.Valid() {
		return errors.New("diagjob: kind is not canonical")
	}
	if !j.Status.Valid() {
		return errors.New("diagjob: status is not canonical")
	}
	if len(j.Subject) > 256 {
		return errors.New("diagjob: subject too long")
	}
	if len(j.Reason) > 256 {
		return errors.New("diagjob: reason too long")
	}
	if len(j.RequestedBy) > 64 {
		return errors.New("diagjob: requestedBy too long")
	}
	for _, line := range j.OutputLines {
		if len(line) > 4096 {
			return errors.New("diagjob: output line too long")
		}
	}
	return nil
}

// Duration returns the wall-clock duration when the job
// has ended. Returns zero when the job is still active.
func (j Job) Duration() time.Duration {
	if j.StartedAt.IsZero() || j.EndedAt.IsZero() {
		return 0
	}
	return j.EndedAt.Sub(j.StartedAt)
}

// Store is the goroutine-safe bounded diagnostic job
// store.
type Store struct {
	mu       sync.Mutex
	jobs     map[string]Job
	order    []string // newest first
	maxJobs  int
	maxBytes int
}

// NewStore returns an empty store.
func NewStore() *Store {
	return &Store{jobs: map[string]Job{}, order: []string{}, maxJobs: MaxJobs, maxBytes: MaxOutputBytes}
}

// SetMax overrides the retention + per-job output caps.
// Non-positive values reset to defaults.
func (s *Store) SetMax(jobs, bytes int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if jobs <= 0 {
		jobs = MaxJobs
	}
	if bytes <= 0 {
		bytes = MaxOutputBytes
	}
	s.maxJobs = jobs
	s.maxBytes = bytes
}

// Append inserts or updates a job. The store trims the
// per-job output to the configured cap.
func (s *Store) Append(job Job) error {
	if err := job.Validate(); err != nil {
		return err
	}
	if len(job.OutputLines) > 0 {
		total := 0
		kept := make([]string, 0, len(job.OutputLines))
		truncated := false
		for _, line := range job.OutputLines {
			if total+len(line)+1 > s.maxBytes {
				truncated = true
				break
			}
			kept = append(kept, line)
			total += len(line) + 1
		}
		job.OutputLines = kept
		job.Truncated = truncated
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.jobs[job.ID]; !exists {
		s.order = append([]string{job.ID}, s.order...)
	}
	s.jobs[job.ID] = job
	if s.maxJobs > 0 && len(s.order) > s.maxJobs {
		for _, id := range s.order[s.maxJobs:] {
			delete(s.jobs, id)
		}
		s.order = s.order[:s.maxJobs]
	}
	return nil
}

// Get returns the job with the supplied id.
func (s *Store) Get(id string) (Job, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	j, ok := s.jobs[id]
	return j, ok
}

// List returns every job sorted newest first.
func (s *Store) List() []Job {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Job, 0, len(s.order))
	for _, id := range s.order {
		if j, ok := s.jobs[id]; ok {
			out = append(out, j)
		}
	}
	return out
}

// ListByKind returns jobs matching the supplied kind
// sorted newest first.
func (s *Store) ListByKind(kind JobKind) []Job {
	all := s.List()
	out := make([]Job, 0, len(all))
	for _, j := range all {
		if j.Kind == kind {
			out = append(out, j)
		}
	}
	return out
}

// Purge removes the job with the supplied id.
func (s *Store) Purge(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.jobs, id)
	for i, x := range s.order {
		if x == id {
			s.order = append(s.order[:i], s.order[i+1:]...)
			return
		}
	}
}

// AllKinds returns every canonical kind for callers that
// want to render an "available diagnostics" list.
func AllKinds() []JobKind {
	out := []JobKind{KindDNSTrace, KindWANProbe, KindRouteAudit, KindWiFiSurvey, KindFirewallLog, KindGenericLog}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// Silence unused import warning for context when no
// helper uses it.
var _ = context.Background
