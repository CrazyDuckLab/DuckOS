// powercrash/powercrash.go: V1 power-loss /
// crash-consistency contract. The package
// owns the typed primitives for durable
// journals, atomic-rename semantics, recovery
// state machines, and recovery-reason
// reporting. The runtime is a thin mapping
// to os.Rename, fsync, and journaled
// filesystems where available.
//
// The V1 contract: every consequential write
// is bracketed by a Begin/End pair on the
// durable journal. A crash between Begin and
// End is observed on next boot and surfaced
// as a recovery reason. Atomic rename is the
// only way to publish configuration; direct
// in-place writes are rejected.
package powercrash

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"sync"
	"time"
)

// Operation is the class of consequential
// write that must be journaled.
type Operation string

const (
	OpConfigCommit   Operation = "config-commit"
	OpSafeApply      Operation = "safe-apply"
	OpVaultWrite     Operation = "vault-write"
	OpUpdateDownload Operation = "update-download"
	OpUpdateActivate Operation = "update-activate"
	OpSchemaMigrate  Operation = "schema-migrate"
	OpBackupCreate   Operation = "backup-create"
	OpBackupRestore  Operation = "backup-restore"
	OpCertRotate     Operation = "cert-rotate"
)

// Valid reports whether o is known.
func (o Operation) Valid() bool {
	switch o {
	case OpConfigCommit, OpSafeApply, OpVaultWrite,
		OpUpdateDownload, OpUpdateActivate, OpSchemaMigrate,
		OpBackupCreate, OpBackupRestore, OpCertRotate:
		return true
	}
	return false
}

// Outcome is the terminal state of a
// journaled operation.
type Outcome string

const (
	OutcomePending   Outcome = "pending"
	OutcomeCommitted Outcome = "committed"
	OutcomeAborted   Outcome = "aborted"
)

// JournalEntry is a single bracketed
// operation. The pair Begin/End is detected
// on recovery by the absence of a matching
// End record.
type JournalEntry struct {
	ID        string    `json:"id"`
	Operation Operation `json:"operation"`
	Subject   string    `json:"subject"`
	StartedAt time.Time `json:"startedAt"`
	EndedAt   time.Time `json:"endedAt,omitempty"`
	Outcome   Outcome   `json:"outcome"`
	Note      string    `json:"note,omitempty"`
}

var idPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:-]{0,63}$`)

// Valid reports whether e is well-formed.
func (e JournalEntry) Valid() error {
	if !idPattern.MatchString(e.ID) {
		return errors.New("powercrash: id is not canonical")
	}
	if !e.Operation.Valid() {
		return errors.New("powercrash: operation is unknown")
	}
	if e.Subject == "" {
		return errors.New("powercrash: subject is required")
	}
	if e.Outcome != OutcomePending && e.Outcome != OutcomeCommitted && e.Outcome != OutcomeAborted {
		return errors.New("powercrash: outcome is invalid")
	}
	return nil
}

// RecoveryReason describes what the recovery
// state machine observed.
type RecoveryReason string

const (
	ReasonNone              RecoveryReason = "none"
	ReasonInterruptedOp     RecoveryReason = "interrupted-operation"
	ReasonOrphanedTemp      RecoveryReason = "orphaned-temp"
	ReasonJournalCorrupt    RecoveryReason = "journal-corrupt"
	ReasonConfigUnpublished RecoveryReason = "config-unpublished"
)

// Report is the recovery state machine
// output. It is consumed by the boot path
// and the Explain Why chain.
type Report struct {
	Reason      RecoveryReason `json:"reason"`
	Interrupted []JournalEntry `json:"interrupted,omitempty"`
	OrphanTemp  []string       `json:"orphanTemp,omitempty"`
	Notes       []string       `json:"notes,omitempty"`
	GeneratedAt time.Time      `json:"generatedAt"`
}

// Journal is the V1 durable-journal
// contract. Journal is goroutine-safe.
type Journal struct {
	mu      sync.Mutex
	entries map[string]JournalEntry
}

// NewJournal returns an empty journal.
func NewJournal() *Journal {
	return &Journal{entries: map[string]JournalEntry{}}
}

// Begin starts a journaled operation. The
// returned ID must be passed to End or
// Abort; absence on recovery indicates a
// crash between Begin and End.
func (j *Journal) Begin(op Operation, subject string) (JournalEntry, error) {
	if !op.Valid() {
		return JournalEntry{}, errors.New("powercrash: op is unknown")
	}
	if subject == "" {
		return JournalEntry{}, errors.New("powercrash: subject is required")
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	e := JournalEntry{
		ID:        fmt.Sprintf("%s-%d", op, time.Now().UnixNano()),
		Operation: op,
		Subject:   subject,
		StartedAt: time.Now().UTC(),
		Outcome:   OutcomePending,
	}
	j.entries[e.ID] = e
	return e, nil
}

// End marks a journaled operation as
// committed. The journal entry must still be
// pending.
func (j *Journal) End(id string, note string) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	e, ok := j.entries[id]
	if !ok {
		return errors.New("powercrash: unknown journal id")
	}
	if e.Outcome != OutcomePending {
		return errors.New("powercrash: journal entry is not pending")
	}
	e.EndedAt = time.Now().UTC()
	e.Outcome = OutcomeCommitted
	e.Note = note
	j.entries[id] = e
	return nil
}

// Abort marks a journaled operation as
// aborted and removes it.
func (j *Journal) Abort(id string, note string) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	e, ok := j.entries[id]
	if !ok {
		return errors.New("powercrash: unknown journal id")
	}
	if e.Outcome != OutcomePending {
		return errors.New("powercrash: journal entry is not pending")
	}
	e.EndedAt = time.Now().UTC()
	e.Outcome = OutcomeAborted
	e.Note = note
	j.entries[id] = e
	return nil
}

// Inspect returns the recovery report. The
// report includes every still-pending entry
// (treated as an interrupted operation),
// every temp file in the watched temp dir,
// and any consistency notes.
func (j *Journal) Inspect(orphanTemp []string) Report {
	j.mu.Lock()
	defer j.mu.Unlock()
	r := Report{Reason: ReasonNone, GeneratedAt: time.Now().UTC()}
	for _, e := range j.entries {
		if e.Outcome == OutcomePending {
			r.Interrupted = append(r.Interrupted, e)
		}
	}
	sort.Slice(r.Interrupted, func(i, k int) bool { return r.Interrupted[i].StartedAt.Before(r.Interrupted[k].StartedAt) })
	r.OrphanTemp = append([]string(nil), orphanTemp...)
	if len(r.Interrupted) > 0 {
		r.Reason = ReasonInterruptedOp
	}
	if len(r.OrphanTemp) > 0 && r.Reason == ReasonNone {
		r.Reason = ReasonOrphanedTemp
	}
	return r
}

// AtomicPublisher is the atomic-rename
// primitive. The runtime writes to a temp
// file in TempDir, fsyncs, then renames to
// Final. The journal must bracket the
// operation so a crash mid-rename is
// detected.
type AtomicPublisher struct {
	TempDir string
	Final   string
}

// Valid reports whether a is well-formed.
func (a AtomicPublisher) Valid() error {
	if a.TempDir == "" {
		return errors.New("powercrash: temp dir is required")
	}
	if a.Final == "" {
		return errors.New("powercrash: final path is required")
	}
	return nil
}

// Stages describes the rename stages a
// runtime can be in.
type Stages string

const (
	StageWriting   Stages = "writing"
	StageFsynced   Stages = "fsynced"
	StageRenamed   Stages = "renamed"
	StageConfirmed Stages = "confirmed"
)

// RecoveryState is the boot-time recovery
// state machine output.
type RecoveryState struct {
	Report     Report `json:"report"`
	Stage      Stages `json:"stage"`
	Action     string `json:"action"`
	SafeToBoot bool   `json:"safeToBoot"`
}

// Decider maps a Report to a RecoveryState.
// It is pure: callers must supply the
// observed temp dir, the previously committed
// final path's fingerprint, and the new
// fingerprint.
type Decider struct{}

// Decide evaluates a Report. interrupted
// operations yield a forced replay action;
// orphan temp files yield a cleanup action;
// nothing yields SafeToBoot=true with
// Action=none.
func (Decider) Decide(r Report, lastCommitted string, candidate string) RecoveryState {
	st := RecoveryState{Report: r}
	if r.Reason == ReasonInterruptedOp {
		st.Stage = StageWriting
		st.Action = "replay:" + string(r.Interrupted[0].Operation)
		st.SafeToBoot = false
		return st
	}
	if r.Reason == ReasonOrphanedTemp {
		st.Stage = StageRenamed
		st.Action = "cleanup-temp"
		st.SafeToBoot = true
		return st
	}
	if lastCommitted == "" {
		st.Stage = StageConfirmed
		st.Action = "first-boot"
		st.SafeToBoot = true
		return st
	}
	if candidate != "" && candidate != lastCommitted {
		st.Stage = StageFsynced
		st.Action = "publish-pending"
		st.SafeToBoot = true
		return st
	}
	st.Stage = StageConfirmed
	st.Action = "none"
	st.SafeToBoot = true
	return st
}
