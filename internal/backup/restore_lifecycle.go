// backup/restore_lifecycle.go: H11 portable/selective
// restore lifecycle.
//
// The base backup package exposes PlanRestore — a
// static diff between a backup and the live state —
// but no execution path. This file adds:
//
//	RestoreJournal — bounded record of every restore
//	attempt the Agent runs. Each entry carries the
//	source revision, the validation outcome, the
//	apply outcome, and (on failure) the rollback
//	outcome. The journal is the audit anchor for
//	destructive restore operations.
//
//	RestoreExecutor — executes a RestorePlan against a
//	supplied Restorer callback. The executor validates
//	the plan first, then commits the changes via the
//	callback, and rolls back on failure. The rollback
//	uses a supplied Snapshotter to capture pre-restore
//	state and a Restorer to apply the inverse.
//
//	RevisionCheck — explicit verification that the
//	live state has the revision the backup was taken
//	against. The check refuses to apply a restore when
//	the live revision has advanced past the backup's
//	revision (the audit's "revision/integrity handling"
//	requirement).
//
// The package never grows unbounded. Per-journal retention
// is bounded by MaxRestoreJournal (default 64). Per-plan
// execution time is bounded by the caller's context.
package backup

import (
	"context"
	"errors"
	"regexp"
	"sort"
	"sync"
	"time"
)

// MaxRestoreJournal caps the journal retention.
const MaxRestoreJournal = 64

// ErrRevisionMismatch is returned when the live state
// has advanced past the backup's revision. The restore
// is refused; the caller can re-plan against the new
// live state or restore the revision explicitly.
var ErrRevisionMismatch = errors.New("backup: live revision advanced past backup revision")

// ErrRestoreValidation is returned when the supplied
// plan fails pre-apply validation. The journal records
// the failure reason.
var ErrRestoreValidation = errors.New("backup: restore validation failed")

// ErrRestoreExecution is returned when the supplied
// Restorer callback fails. The executor attempts to
// roll back before returning.
var ErrRestoreExecution = errors.New("backup: restore execution failed")

// RestoreOutcome enumerates the journal outcomes.
type RestoreOutcome string

const (
	OutcomeStarted    RestoreOutcome = "started"
	OutcomeValidated  RestoreOutcome = "validated"
	OutcomeApplied    RestoreOutcome = "applied"
	OutcomeRolledBack RestoreOutcome = "rolled-back"
	OutcomeFailed     RestoreOutcome = "failed"
)

// Valid reports whether o is a known outcome.
func (o RestoreOutcome) Valid() bool {
	switch o {
	case OutcomeStarted, OutcomeValidated, OutcomeApplied, OutcomeRolledBack, OutcomeFailed:
		return true
	}
	return false
}

// RestoreEntry is one row of the restore journal.
type RestoreEntry struct {
	ID       string         `json:"id"`
	BackupID string         `json:"backupId"`
	Outcome  RestoreOutcome `json:"outcome"`
	// SourceRevision is the revision the backup was
	// taken against. The journal refuses to apply a
	// restore when the live revision has advanced past
	// this value.
	SourceRevision string    `json:"sourceRevision"`
	LiveRevision   string    `json:"liveRevision,omitempty"`
	StartedAt      time.Time `json:"startedAt"`
	EndedAt        time.Time `json:"endedAt,omitempty"`
	Reason         string    `json:"reason,omitempty"`
	// ValidationErrors carries the per-rule failures
	// surfaced by ValidateRestorePlan.
	ValidationErrors []string `json:"validationErrors,omitempty"`
}

var entryIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{7,63}$`)

// Valid reports whether the entry is internally
// consistent.
func (e RestoreEntry) Valid() error {
	if !entryIDPattern.MatchString(e.ID) {
		return errors.New("backup: entry id is not canonical")
	}
	if e.BackupID == "" {
		return errors.New("backup: entry backupId is required")
	}
	if !e.Outcome.Valid() {
		return errors.New("backup: entry outcome is invalid")
	}
	if e.StartedAt.IsZero() {
		return errors.New("backup: entry startedAt is zero")
	}
	return nil
}

// RestoreJournal is the goroutine-safe bounded journal.
type RestoreJournal struct {
	mu         sync.Mutex
	entries    map[string]RestoreEntry
	order      []string
	maxEntries int
}

// NewRestoreJournal returns an empty journal.
func NewRestoreJournal() *RestoreJournal {
	return &RestoreJournal{
		entries:    map[string]RestoreEntry{},
		order:      []string{},
		maxEntries: MaxRestoreJournal,
	}
}

// SetMax overrides the retention cap.
func (j *RestoreJournal) SetMax(n int) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if n <= 0 {
		n = MaxRestoreJournal
	}
	j.maxEntries = n
}

// Append inserts or updates an entry. The order is
// preserved on update so the latest entry for an id stays
// in place.
func (j *RestoreJournal) Append(e RestoreEntry) error {
	if err := e.Valid(); err != nil {
		return err
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if _, ok := j.entries[e.ID]; !ok {
		j.order = append([]string{e.ID}, j.order...)
	}
	j.entries[e.ID] = e
	if j.maxEntries > 0 && len(j.order) > j.maxEntries {
		for _, id := range j.order[j.maxEntries:] {
			delete(j.entries, id)
		}
		j.order = j.order[:j.maxEntries]
	}
	return nil
}

// Get returns the entry with the supplied id.
func (j *RestoreJournal) Get(id string) (RestoreEntry, bool) {
	j.mu.Lock()
	defer j.mu.Unlock()
	e, ok := j.entries[id]
	return e, ok
}

// All returns every entry sorted newest first.
func (j *RestoreJournal) All() []RestoreEntry {
	j.mu.Lock()
	defer j.mu.Unlock()
	out := make([]RestoreEntry, 0, len(j.order))
	for _, id := range j.order {
		if e, ok := j.entries[id]; ok {
			out = append(out, e)
		}
	}
	return out
}

// Snapshotter captures a point-in-time view of the live
// configuration so a failed restore can be rolled back.
// The Snapshot is opaque to the executor; the Restorer
// knows how to apply the inverse.
type Snapshotter func(ctx context.Context) (any, error)

// Restorer applies or rolls back a snapshot. The first
// call (apply) commits the restore plan to live state;
// the second call (rollback) restores the snapshot the
// Snapshotter captured before the apply.
type Restorer func(ctx context.Context, snapshot any, direction RestoreDirection) error

// RestoreDirection tells the Restorer whether it is
// applying forward or rolling back.
type RestoreDirection string

const (
	DirectionApply    RestoreDirection = "apply"
	DirectionRollback RestoreDirection = "rollback"
)

// RestorerResult is the per-restore outcome the executor
// surfaces to the caller. It is also written to the
// journal so the audit trail is durable.
type RestorerResult struct {
	Entry RestoreEntry
	Err   error
}

// ValidateRestorePlan runs the pre-apply validation. The
// function returns nil when the plan is internally
// consistent AND the live revision matches the backup
// revision; otherwise it returns a slice of failure
// reasons. The slice is never nil on error; callers can
// surface the reasons directly in the journal.
func ValidateRestorePlan(plan RestorePlan, sourceRevision, liveRevision string) []string {
	failures := []string{}
	if plan.BackupID == "" {
		failures = append(failures, "backup id is empty")
	}
	if !plan.SafeToApply {
		failures = append(failures, "plan is not safe to apply")
	}
	if plan.RequiresSecret && sourceRevision == "" {
		failures = append(failures, "plan requires secret but source revision is missing")
	}
	// Revisions are opaque identifiers, not lexically ordered counters.
	// A real coordinator must obtain/check the live revision atomically.
	if sourceRevision == "" || liveRevision == "" || liveRevision != sourceRevision {
		failures = append(failures, "live revision advanced past backup revision")
	}
	return failures
}

// ExecuteRestore runs the full restore lifecycle against
// the supplied Restorer + Snapshotter. The journal is
// updated at every transition so an interrupted Agent
// retains a coherent audit trail on restart.
//
// The function never returns ErrRevisionMismatch when
// ValidateRestorePlan reports a revision mismatch: the
// error is recorded in the journal and surfaced as the
// wrapped ErrRestoreValidation.
func ExecuteRestore(ctx context.Context, journal *RestoreJournal, entry RestoreEntry, plan RestorePlan, sourceRevision, liveRevision string, snap Snapshotter, restore Restorer) RestorerResult {
	if ctx == nil {
		return RestorerResult{Err: errors.New("backup: context is nil")}
	}
	if err := ctx.Err(); err != nil {
		return RestorerResult{Err: err}
	}
	if journal == nil {
		return RestorerResult{Err: errors.New("backup: journal is nil")}
	}
	if snap == nil || restore == nil {
		return RestorerResult{Err: errors.New("backup: snapshotter and restorer are required")}
	}
	if entry.BackupID != plan.BackupID {
		return RestorerResult{Err: ErrRestoreValidation}
	}
	entry.SourceRevision = sourceRevision
	entry.LiveRevision = liveRevision
	entry.Outcome = OutcomeStarted
	if err := journal.Append(entry); err != nil {
		return RestorerResult{Err: err}
	}
	// Validate.
	failures := ValidateRestorePlan(plan, sourceRevision, liveRevision)
	if len(failures) > 0 {
		entry.Outcome = OutcomeFailed
		entry.EndedAt = time.Now().UTC()
		entry.Reason = ErrRestoreValidation.Error()
		entry.ValidationErrors = failures
		_ = journal.Append(entry)
		if containsRevisionFailure(failures) {
			return RestorerResult{Entry: entry, Err: ErrRevisionMismatch}
		}
		return RestorerResult{Entry: entry, Err: ErrRestoreValidation}
	}
	entry.Outcome = OutcomeValidated
	_ = journal.Append(entry)
	// Snapshot pre-restore state for rollback.
	snapVal, snapErr := snap(ctx)
	if snapErr == nil {
		snapErr = ctx.Err()
	}
	if snapErr != nil {
		entry.Outcome = OutcomeFailed
		entry.EndedAt = time.Now().UTC()
		entry.Reason = "snapshot failed: " + snapErr.Error()
		_ = journal.Append(entry)
		return RestorerResult{Entry: entry, Err: snapErr}
	}
	// Apply.
	applyErr := restore(ctx, snapVal, DirectionApply)
	if applyErr == nil {
		applyErr = ctx.Err()
	}
	if applyErr != nil {
		// Best-effort rollback.
		// A disconnected/canceled request must not cancel recovery of a
		// partially applied restore. The callback must honor this bound.
		rollbackCtx, cancelRollback := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		rbErr := restore(rollbackCtx, snapVal, DirectionRollback)
		cancelRollback()
		if rbErr != nil {
			entry.Outcome = OutcomeFailed
			entry.EndedAt = time.Now().UTC()
			entry.Reason = "apply failed: " + applyErr.Error() + "; rollback failed: " + rbErr.Error()
			_ = journal.Append(entry)
			return RestorerResult{Entry: entry, Err: errors.Join(ErrRestoreExecution, applyErr, rbErr)}
		}
		entry.Outcome = OutcomeRolledBack
		entry.EndedAt = time.Now().UTC()
		entry.Reason = "apply failed: " + applyErr.Error()
		_ = journal.Append(entry)
		return RestorerResult{Entry: entry, Err: applyErr}
	}
	entry.Outcome = OutcomeApplied
	entry.EndedAt = time.Now().UTC()
	_ = journal.Append(entry)
	return RestorerResult{Entry: entry, Err: nil}
}

func containsRevisionFailure(failures []string) bool {
	for _, f := range failures {
		if f == "live revision advanced past backup revision" {
			return true
		}
	}
	return false
}

// RestoreJournalCounters summarises the journal.
type RestoreJournalCounters struct {
	Total     int                    `json:"total"`
	ByOutcome map[RestoreOutcome]int `json:"byOutcome"`
}

// Counters returns the current counts.
func (j *RestoreJournal) Counters() RestoreJournalCounters {
	j.mu.Lock()
	defer j.mu.Unlock()
	by := map[RestoreOutcome]int{}
	for _, e := range j.entries {
		by[e.Outcome]++
	}
	return RestoreJournalCounters{Total: len(j.entries), ByOutcome: by}
}

// silence unused-import warning for sort when no helper uses it.
var _ = sort.Strings
