// Package state implements the Engine's bounded durable revision journal.
// It has no Linux mutation authority and never treats a persisted phase as
// evidence that an external operation succeeded.
package state

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"syscall"
	"time"
)

const MaxConfigBytes = 256 << 10

// History and bounded completed-request replay records both retain configurations.
const maxStateBytes = 32 << 20
const historyLimit = 32

var ErrConflict = errors.New("configuration revision conflict")
var ErrPending = errors.New("configuration transaction already pending")
var ErrRecovery = errors.New("state requires recovery; reopen and inspect durable journal")

type Revision struct {
	ID        string          `json:"id"`
	Config    json.RawMessage `json:"config"`
	Actor     string          `json:"actor"`
	Reason    string          `json:"reason"`
	CreatedAt time.Time       `json:"createdAt"`
}

type Phase string

const (
	Prepared             Phase = "prepared"
	Applying             Phase = "applying"
	AwaitingConfirmation Phase = "awaiting-confirmation"
	RollingBack          Phase = "rolling-back"
	RecoveryRequired     Phase = "recovery-required"
	Committed            Phase = "committed"
	RolledBackPhase      Phase = "rolled-back"
)

type Transaction struct {
	ID                 string          `json:"id"`
	Before             string          `json:"before"`
	Candidate          Revision        `json:"candidate"`
	Phase              Phase           `json:"phase"`
	Deadline           time.Time       `json:"deadline"`
	Recovery           json.RawMessage `json:"recovery,omitempty"`
	RequestFingerprint string          `json:"requestFingerprint,omitempty"`
	CompletedAt        time.Time       `json:"completedAt,omitempty"`
}

type Snapshot struct {
	Restore   *RestoreAdmission `json:"restore,omitempty"`
	Version   int               `json:"version"`
	Current   Revision          `json:"current"`
	Pending   *Transaction      `json:"pending"`
	History   []Revision        `json:"history"`
	Outcomes  []Outcome         `json:"outcomes,omitempty"`
	Completed []Transaction     `json:"completed,omitempty"`
}

// Outcome is a durable coordinator receipt, not independent proof of Linux
// health. It is written atomically with committing or clearing the candidate.
type Outcome struct {
	TransactionID string    `json:"transactionId"`
	Before        string    `json:"before"`
	Candidate     string    `json:"candidate"`
	Result        string    `json:"result"`
	CompletedAt   time.Time `json:"completedAt"`
}

func recordOutcome(next *Snapshot, result string) {
	p := next.Pending
	o := Outcome{TransactionID: p.ID, Before: p.Before, Candidate: p.Candidate.ID, Result: result, CompletedAt: time.Now().UTC()}
	appendOutcome(next, o)
}

func appendOutcome(next *Snapshot, o Outcome) {
	if p := next.Pending; p != nil && p.ID == o.TransactionID && p.RequestFingerprint != "" {
		tx := *p
		tx.Phase = Phase(o.Result)
		tx.CompletedAt = o.CompletedAt
		tx.Recovery = nil
		next.Completed = append([]Transaction{tx}, next.Completed...)
		if len(next.Completed) > historyLimit {
			next.Completed = next.Completed[:historyLimit]
		}
	}
	next.Outcomes = append([]Outcome{o}, next.Outcomes...)
	if len(next.Outcomes) > historyLimit {
		next.Outcomes = next.Outcomes[:historyLimit]
	}
	retained := make(map[string]bool, len(next.Outcomes))
	for _, outcome := range next.Outcomes {
		retained[outcome.TransactionID] = true
	}
	completed := next.Completed[:0]
	for _, tx := range next.Completed {
		if retained[tx.ID] {
			completed = append(completed, tx)
		}
	}
	next.Completed = completed
}

// Reconcile records an authenticated coordinator outcome after an uncertain
// IPC response or restart. Only the Engine may use this after validating an
// Agent receipt and its current revision. A receipt is not accepted from UI.
// Unlike interactive Commit, a decision already made by Agent is not reversed
// merely because the local confirmation deadline elapsed during disconnection.
func (s *Store) Reconcile(outcome Outcome) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.poisoned {
		return ErrRecovery
	}
	// M-25: state.Revision.ID is a 64-char sha256 hex. The
	// original contract required 32-char hex which never
	// matched the real revision identity. Accept both
	// shapes so the Engine surface can call Reconcile
	// with the natural state.Revision.ID value.
	if !ValidTransactionID(outcome.TransactionID) || !validHex(outcome.Before, 32) && !validHex(outcome.Before, 64) || !validHex(outcome.Candidate, 32) && !validHex(outcome.Candidate, 64) || outcome.Before == outcome.Candidate || outcome.CompletedAt.IsZero() || (outcome.Result != "committed" && outcome.Result != "rolled-back") {
		return ErrConflict
	}
	p := s.snapshot.Pending
	if p == nil {
		for _, old := range s.snapshot.Outcomes {
			if old.TransactionID == outcome.TransactionID && old.Before == outcome.Before && old.Candidate == outcome.Candidate && old.Result == outcome.Result && old.CompletedAt.Equal(outcome.CompletedAt) {
				return nil
			}
		}
		return ErrConflict
	}
	if p.ID != outcome.TransactionID || p.Before != outcome.Before || p.Candidate.ID != outcome.Candidate {
		return ErrConflict
	}
	next := clone(s.snapshot)
	if outcome.Result == "committed" {
		next.History = append([]Revision{next.Current}, next.History...)
		if len(next.History) > historyLimit {
			next.History = next.History[:historyLimit]
		}
		next.Current = next.Pending.Candidate
	}
	appendOutcome(&next, outcome)
	next.Pending = nil
	return s.persist(next)
}

type envelope struct {
	Digest  string          `json:"digest"`
	Payload json.RawMessage `json:"payload"`
}

// RejectRequest durably closes an admission that never reached host mutation.
// The caller must hold the Agent worker admission lock so no matching request
// can start between this check and persistence. This is not a host rollback.
func (s *Store) RejectRequest(expected, requestID string, config json.RawMessage, actor, reason string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.poisoned {
		return ErrRecovery
	}
	if !ValidTransactionID(requestID) || s.snapshot.Pending != nil || s.snapshot.Current.ID != expected {
		return ErrConflict
	}
	r, err := s.revision(config, actor, reason)
	if err != nil {
		return err
	}
	if r.ID == expected {
		return ErrConflict
	}
	for _, old := range s.snapshot.Outcomes {
		if old.TransactionID == requestID {
			if old.Before == expected && old.Candidate == r.ID && old.Result == "rolled-back" {
				return nil
			}
			return ErrConflict
		}
	}
	next := clone(s.snapshot)
	appendOutcome(&next, Outcome{TransactionID: requestID, Before: expected, Candidate: r.ID, Result: "rolled-back", CompletedAt: time.Now().UTC()})
	return s.persist(next)
}

type Store struct {
	mu       sync.Mutex
	root     *os.Root
	lock     *os.File
	validate func(json.RawMessage) error
	snapshot Snapshot
	poisoned bool
	closed   bool
}

// Open requires an existing private directory. Initial is used only when no
// state file exists; malformed or inaccessible state is never overwritten.
func Open(path string, initial json.RawMessage, validate func(json.RawMessage) error) (*Store, error) {
	return open(path, initial, validate, true)
}

// OpenExisting never replaces a missing journal with an empty baseline. Boot
// recovery and already-provisioned services must fail closed on lost state.
func OpenExisting(path string, validate func(json.RawMessage) error) (*Store, error) {
	return open(path, nil, validate, false)
}

func open(path string, initial json.RawMessage, validate func(json.RawMessage) error, initialize bool) (*Store, error) {
	if validate == nil {
		return nil, errors.New("configuration validator required")
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return nil, errors.New("state directory must be private and not a symlink")
	}
	if st, ok := info.Sys().(*syscall.Stat_t); !ok || int(st.Uid) != os.Geteuid() {
		return nil, errors.New("state directory must belong to the current service identity")
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return nil, err
	}
	s := &Store{root: root, validate: validate}
	ok := false
	defer func() {
		if !ok {
			s.Close()
		}
	}()
	if err = privateFile(root, "writer.lock", true); err != nil {
		return nil, err
	}
	s.lock, err = root.OpenFile("writer.lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = syscall.Flock(int(s.lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return nil, errors.New("state store already owned by another writer")
	}
	if err = privateFile(root, "state.json", true); err != nil {
		return nil, err
	}
	f, err := root.Open("state.json")
	if errors.Is(err, os.ErrNotExist) {
		if !initialize {
			return nil, errors.New("existing state journal is missing; recovery required")
		}
		r, e := s.revision(initial, "system", "initialize")
		if e != nil {
			return nil, e
		}
		err = s.persist(Snapshot{Version: 1, Current: r, History: []Revision{}})
	} else if err == nil {
		data, e := io.ReadAll(io.LimitReader(f, maxStateBytes+1))
		f.Close()
		if e != nil {
			return nil, e
		}
		if len(data) > maxStateBytes {
			return nil, errors.New("state exceeds size limit")
		}
		var wrapped envelope
		if err = decode(data, &wrapped); err == nil {
			if digest(wrapped.Payload) != wrapped.Digest {
				return nil, errors.New("state checksum mismatch")
			}
			err = decode(wrapped.Payload, &s.snapshot)
		}
		if err == nil {
			err = s.check(s.snapshot)
		}
	}
	if err != nil {
		return nil, fmt.Errorf("open state: %w", err)
	}
	ok = true
	return s, nil
}

func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	if s.lock != nil {
		s.lock.Close()
	}
	if s.root != nil {
		return s.root.Close()
	}
	return nil
}

func (s *Store) Snapshot() (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.poisoned {
		return Snapshot{}, ErrRecovery
	}
	return clone(s.snapshot), nil
}

// SeedCurrent installs a verified Agent Current revision as the
// canonical journal Current. It is only safe when no committed
// work exists (no Pending, no committed History). Rolled-back
// outcomes are tolerated: those are noise from rejected apply
// attempts against a placeholder Current that the engine is
// about to replace; replaying them through the new Current is
// harmless because every rollback-receipt is terminal. The
// Engine calls this at startup so the journal's baseline
// revision matches the Agent's startup Current; without that
// match the first Prepare fails with revision_conflict because
// the journal's Current.ID was the SHA-256 of the placeholder
// `{"version":1}` and not the revision the Agent captures from
// the live kernel.
func (s *Store) SeedCurrent(current Revision) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.poisoned {
		return ErrRecovery
	}
	if !validHex(current.ID, 32) && !validHex(current.ID, 64) {
		return ErrConflict
	}
	if s.snapshot.Pending != nil {
		return ErrConflict
	}
	if len(s.snapshot.History) != 0 {
		// Committed work exists; never silently rebase.
		return ErrConflict
	}
	for _, o := range s.snapshot.Outcomes {
		if o.Result == "committed" {
			// A committed outcome is evidence of real work
			// landing on the host. Re-seeding would orphan
			// that receipt.
			return ErrConflict
		}
	}
	next := clone(s.snapshot)
	next.Current = current
	return s.persist(next)
}

// Prepare uses the caller's observed revision as an optimistic concurrency
// precondition. The durable candidate always precedes any external mutation.
func (s *Store) Prepare(expected string, config json.RawMessage, actor, reason string) (Transaction, error) {
	return s.PrepareWithRecovery(expected, config, actor, reason, nil)
}

// PrepareWithRecovery is for the privileged journal only. Recovery evidence is
// not canonical desired state and must never be returned by a public status API.
func (s *Store) PrepareWithRecovery(expected string, config json.RawMessage, actor, reason string, recovery json.RawMessage) (Transaction, error) {
	return s.PrepareRequest(expected, "", config, actor, reason, recovery)
}

// PrepareRequest binds the Engine's durably admitted transaction ID to the
// Agent journal. An empty ID preserves locally generated transaction behavior.
// Existing pending/retained IDs are never reapplied; callers reconcile them.
func (s *Store) PrepareRequest(expected, requestID string, config json.RawMessage, actor, reason string, recovery json.RawMessage) (Transaction, error) {
	return s.PrepareRequestWithWindow(expected, requestID, config, actor, reason, recovery, 0)
}

// PrepareRequestWithWindow binds the authenticated request, including its
// confirmation policy, to an immutable durable replay record.
func (s *Store) PrepareRequestWithWindow(expected, requestID string, config json.RawMessage, actor, reason string, recovery json.RawMessage, window int) (Transaction, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.snapshot.Restore != nil {
		return Transaction{}, ErrPending
	}
	if requestID != "" && !ValidTransactionID(requestID) {
		return Transaction{}, errors.New("invalid transaction request identity")
	}
	if len(recovery) > 1<<20 || (len(recovery) > 0 && !json.Valid(recovery)) {
		return Transaction{}, errors.New("invalid or oversized recovery evidence")
	}
	if s.closed || s.poisoned {
		return Transaction{}, ErrRecovery
	}
	r, err := s.revision(config, actor, reason)
	if err != nil {
		return Transaction{}, err
	}
	canonical, err := json.Marshal(struct {
		Expected, Candidate, Actor, Reason string
		Window                             int
	}{expected, r.ID, actor, reason, window})
	if err != nil {
		return Transaction{}, err
	}
	fingerprint := digest(canonical)
	// Caller-supplied requestID idempotency contract: a
	// repeated Prepare with the same requestID and the same
	// candidate body returns the original transaction's
	// outcome (idempotent dedup, survives restart because
	// the outcome is durably recorded). A repeated Prepare
	// with the same requestID and a different candidate is
	// rejected with ErrConflict — caller identity reuse with
	// a conflicting body is never silently admitted.
	if requestID != "" {
		// Check the in-flight Pending first: a retry against
		// the very same requestID is idempotent (the prior
		// Prepare already admitted; the caller just didn't get
		// the response).
		if p := s.snapshot.Pending; p != nil && p.ID == requestID {
			if p.RequestFingerprint == fingerprint {
				return *clone(s.snapshot).Pending, nil
			}
			return Transaction{}, ErrConflict
		}
		for _, tx := range clone(s.snapshot).Completed {
			if tx.ID == requestID {
				if tx.RequestFingerprint != fingerprint {
					return Transaction{}, ErrConflict
				}
				return tx, nil
			}
		}
		for _, outcome := range s.snapshot.Outcomes {
			if outcome.TransactionID != requestID {
				continue
			}
			// Legacy receipts lack the original request metadata. Refuse reuse
			// rather than invent a successful replay or execute it again.
			return Transaction{}, ErrConflict
		}
	}
	if s.snapshot.Pending != nil {
		return Transaction{}, ErrPending
	}
	if s.snapshot.Current.ID != expected {
		return Transaction{}, ErrConflict
	}
	if r.ID == expected {
		return Transaction{}, errors.New("candidate has no changes")
	}
	if requestID == "" {
		var id [16]byte
		if _, err = rand.Read(id[:]); err != nil {
			return Transaction{}, err
		}
		requestID = hex.EncodeToString(id[:])
	}
	t := Transaction{ID: requestID, Before: expected, Candidate: r, Phase: Prepared, Recovery: append(json.RawMessage(nil), recovery...), RequestFingerprint: fingerprint}
	next := clone(s.snapshot)
	next.Pending = &t
	if err = s.persist(next); err != nil {
		return Transaction{}, err
	}
	return *clone(s.snapshot).Pending, nil
}

func (s *Store) Transition(id string, phase Phase, deadline time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.poisoned {
		return ErrRecovery
	}
	p := s.snapshot.Pending
	if p == nil || p.ID != id {
		return ErrConflict
	}
	allowed := (p.Phase == Prepared && phase == Applying) || (p.Phase == Applying && phase == AwaitingConfirmation) || (p.Phase != RollingBack && phase == RollingBack) || (p.Phase == RollingBack && phase == RecoveryRequired)
	if !allowed {
		return errors.New("invalid transaction transition")
	}
	if phase == AwaitingConfirmation {
		d := time.Until(deadline)
		if d < time.Minute || d > 15*time.Minute {
			return errors.New("confirmation deadline must be 1–15 minutes from readiness")
		}
	} else if !deadline.IsZero() {
		return errors.New("deadline belongs to confirmation phase only")
	}
	next := clone(s.snapshot)
	next.Pending.Phase = phase
	next.Pending.Deadline = deadline
	return s.persist(next)
}

// Commit must only be called after the coordinator has obtained explicit
// confirmation and independently reconciled the Agent's operation outcome.
func (s *Store) Commit(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.poisoned {
		return ErrRecovery
	}
	p := s.snapshot.Pending
	if p == nil || p.ID != id {
		return ErrConflict
	}
	if p.Phase != AwaitingConfirmation || !time.Now().Before(p.Deadline) {
		return errors.New("transaction is not confirmable")
	}
	next := clone(s.snapshot)
	next.History = append([]Revision{next.Current}, next.History...)
	if len(next.History) > historyLimit {
		next.History = next.History[:historyLimit]
	}
	next.Current = next.Pending.Candidate
	recordOutcome(&next, "committed")
	next.Pending = nil
	return s.persist(next)
}

// RolledBack records a verified external rollback, not a request to roll back.
func (s *Store) RolledBack(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.poisoned {
		return ErrRecovery
	}
	p := s.snapshot.Pending
	if p == nil || p.ID != id {
		return ErrConflict
	}
	if p.Phase != RollingBack {
		return errors.New("rollback has not been requested")
	}
	next := clone(s.snapshot)
	recordOutcome(&next, "rolled-back")
	next.Pending = nil
	return s.persist(next)
}

func (s *Store) revision(raw json.RawMessage, actor, reason string) (Revision, error) {
	if len(raw) == 0 || len(raw) > MaxConfigBytes || len(actor) == 0 || len(actor) > 128 || len(reason) == 0 || len(reason) > 512 {
		return Revision{}, errors.New("invalid revision metadata or size")
	}
	if err := s.validate(raw); err != nil {
		return Revision{}, err
	}
	var value any
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	if err := d.Decode(&value); err != nil {
		return Revision{}, err
	}
	if d.Decode(new(any)) != io.EOF {
		return Revision{}, errors.New("trailing configuration data")
	}
	canonical, err := json.Marshal(value)
	if err != nil {
		return Revision{}, err
	}
	return Revision{ID: digest(canonical), Config: canonical, Actor: actor, Reason: reason, CreatedAt: time.Now().UTC()}, nil
}

func (s *Store) check(v Snapshot) error {
	if r := v.Restore; r != nil {
		if v.Pending != nil || !ValidTransactionID(r.ID) || r.Expected != v.Current.ID || !validHex(r.Digest, 32) || !json.Valid(r.Request) || len(r.Request) > 65536 || digest(r.Request) != r.Digest {
			return ErrRecovery
		}
	}
	if (v.Version != 1 && v.Version != 2) || len(v.History) > historyLimit || len(v.Outcomes) > historyLimit || len(v.Completed) > historyLimit || (v.Version == 1 && (len(v.Outcomes) != 0 || len(v.Completed) != 0)) {
		return errors.New("unsupported or oversized state")
	}
	seen := make(map[string]bool, len(v.Outcomes))
	for _, o := range v.Outcomes {
		if !validHex(o.TransactionID, 16) || !validHex(o.Before, 32) && !validHex(o.Before, 64) || !validHex(o.Candidate, 32) && !validHex(o.Candidate, 64) || o.Before == o.Candidate || o.CompletedAt.IsZero() || (o.Result != "committed" && o.Result != "rolled-back") || seen[o.TransactionID] || (v.Pending != nil && v.Pending.ID == o.TransactionID) {
			return errors.New("invalid transaction outcome")
		}
		seen[o.TransactionID] = true
	}
	all := append([]Revision{v.Current}, v.History...)
	completedIDs := make(map[string]bool)
	for _, tx := range v.Completed {
		if completedIDs[tx.ID] || !ValidTransactionID(tx.ID) || !validHex(tx.RequestFingerprint, 32) || tx.CompletedAt.IsZero() || len(tx.Recovery) != 0 {
			return errors.New("invalid completed request")
		}
		matched := false
		for _, o := range v.Outcomes {
			if o.TransactionID == tx.ID && o.Before == tx.Before && o.Candidate == tx.Candidate.ID && Phase(o.Result) == tx.Phase && o.CompletedAt.Equal(tx.CompletedAt) {
				matched = true
				break
			}
		}
		if !matched {
			return errors.New("completed request has no matching receipt")
		}
		completedIDs[tx.ID] = true
		all = append(all, tx.Candidate)
	}
	if v.Pending != nil {
		p := v.Pending
		if p.RequestFingerprint != "" && !validHex(p.RequestFingerprint, 32) {
			return errors.New("invalid pending request fingerprint")
		}
		if len(p.Recovery) > 1<<20 || (len(p.Recovery) > 0 && !json.Valid(p.Recovery)) {
			return errors.New("invalid or oversized recovery evidence")
		}
		b, e := hex.DecodeString(p.ID)
		if e != nil || len(b) != 16 || p.Before != v.Current.ID {
			return errors.New("invalid pending transaction identity")
		}
		switch p.Phase {
		case Prepared, Applying, RollingBack, RecoveryRequired:
			if !p.Deadline.IsZero() {
				return errors.New("unexpected transaction deadline")
			}
		case AwaitingConfirmation:
			if p.Deadline.IsZero() {
				return errors.New("missing transaction deadline")
			}
		default:
			return errors.New("unknown transaction phase")
		}
		all = append(all, p.Candidate)
	}
	for _, r := range all {
		if len(r.Config) > MaxConfigBytes || digest(r.Config) != r.ID || r.CreatedAt.IsZero() || len(r.Actor) == 0 || len(r.Actor) > 128 || len(r.Reason) == 0 || len(r.Reason) > 512 {
			return errors.New("invalid revision")
		}
		if err := s.validate(r.Config); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) persist(next Snapshot) error {
	// Read v1 without rewriting it. The next normal write upgrades atomically;
	// old binaries reject v2 instead of discarding transaction receipts.
	next.Version = 2
	if err := s.check(next); err != nil {
		return err
	}
	payload, err := json.Marshal(next)
	if err != nil {
		return err
	}
	data, err := json.Marshal(envelope{Digest: digest(payload), Payload: payload})
	if err != nil {
		return err
	}
	if len(data) > maxStateBytes {
		return errors.New("state exceeds size limit")
	}
	if err = privateFile(s.root, "state.tmp", true); err != nil {
		return err
	}
	f, err := s.root.OpenFile("state.tmp", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err == nil {
		err = s.root.Rename("state.tmp", "state.json")
	}
	if err == nil {
		var dir *os.File
		dir, err = s.root.Open(".")
		if err == nil {
			err = dir.Sync()
			dir.Close()
		}
	}
	if err != nil {
		s.poisoned = true
		return fmt.Errorf("durability failure: %w", err)
	}
	s.snapshot = clone(next)
	return nil
}

func validHex(value string, size int) bool {
	b, err := hex.DecodeString(value)
	return err == nil && len(b) == size && hex.EncodeToString(b) == value
}

func ValidTransactionID(value string) bool { return validHex(value, 16) }

func privateFile(root *os.Root, name string, missing bool) error {
	i, err := root.Lstat(name)
	if missing && errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !i.Mode().IsRegular() || i.Mode().Perm()&0077 != 0 {
		return errors.New("state file must be private regular file")
	}
	if st, ok := i.Sys().(*syscall.Stat_t); ok && (st.Nlink != 1 || int(st.Uid) != os.Geteuid()) {
		return errors.New("unsafe state file ownership or hardlink")
	}
	return nil
}
func digest(data []byte) string { h := sha256.Sum256(data); return hex.EncodeToString(h[:]) }
func clone(v Snapshot) Snapshot {
	b, _ := json.Marshal(v)
	var out Snapshot
	_ = json.Unmarshal(b, &out)
	return out
}
func decode(data []byte, out any) error {
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(out); err != nil {
		return err
	}
	if d.Decode(new(any)) != io.EOF {
		return errors.New("trailing JSON data")
	}
	return nil
}
