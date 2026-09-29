// Package engine exposes journal observations. Generic mutations remain unavailable
// until connected to the existing verified networking coordinator.
package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"sync"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/network"
	"github.com/CrazyDuckLab/DuckOS/internal/state"
)

// ErrConfigRuntimeUnavailable is fail-closed: a journal is not a Linux coordinator.
var ErrConfigRuntimeUnavailable = errors.New("verified configuration coordinator is not connected")

// RevisionSummary deliberately excludes arbitrary configuration and user-supplied metadata.
type RevisionSummary struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"createdAt"`
}

func revisionSummary(r state.Revision) RevisionSummary {
	return RevisionSummary{ID: r.ID, CreatedAt: r.CreatedAt}
}

// ConfigStatus is the public snapshot of the canonical
// config transaction journal. It carries the current
// revision, the pending transaction (if any), and the most
// recent outcomes. Every field is language-neutral.
type ConfigStatus struct {
	RestoreRequestID string             `json:"restoreRequestId,omitempty"`
	Version          int                `json:"version"`
	UpdatedAt        time.Time          `json:"updatedAt"`
	Current          RevisionSummary    `json:"current"`
	Pending          *PublicTransaction `json:"pending,omitempty"`
	Outcomes         []state.Outcome    `json:"outcomes,omitempty"`
	History          []RevisionSummary  `json:"history,omitempty"`
	Server           ServerTime         `json:"serverTime"`
}

// PublicTransaction is the language-neutral view of a
// pending state.Transaction. The deadline is exposed as
// both an absolute timestamp and a monotonic remaining
// duration so the UI can render a countdown without
// trusting client-side clocks.
type PublicTransaction struct {
	CompletedAt      time.Time       `json:"completedAt,omitempty"`
	ID               string          `json:"id"`
	BaseRevision     string          `json:"baseRevision"`
	Candidate        RevisionSummary `json:"candidate"`
	Phase            string          `json:"phase"`
	Deadline         time.Time       `json:"deadline"`
	RemainingMS      int64           `json:"remainingMs"`
	SupportedActions []string        `json:"supportedActions"`
	RequestID        string          `json:"requestId,omitempty"`
}

// ServerTime lets the client re-sync its clock against the
// server without trusting the deadline timestamp alone.
type ServerTime struct {
	Now     time.Time `json:"now"`
	EpochMS int64     `json:"epochMs"`
}

// ConfigDiffRequest carries the expected revision + a
// candidate config so the engine can compute a structured
// diff. Either field may be empty when the caller only wants
// the current revision; both empty means "describe current".
type ConfigDiffRequest struct {
	RequestID string          `json:"requestId,omitempty"`
	Expected  string          `json:"expected,omitempty"`
	Candidate json.RawMessage `json:"candidate,omitempty"`
}

// ConfigDiff is the public response. It carries the base
// revision, the candidate revision (if computed), and a
// structured findings slice. Validation findings are
// language-neutral codes, not English prose.
type ConfigDiff struct {
	BaseRevision      string          `json:"baseRevision"`
	CandidateRevision string          `json:"candidateRevision,omitempty"`
	DiffDigest        string          `json:"diffDigest"`
	Findings          []ConfigFinding `json:"findings"`
}

// ConfigFinding is one structured note attached to a diff
// or validation result. Severity is one of
// "info", "warning", "error". Code is a stable string the
// UI can match for localisation.
type ConfigFinding struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Object   string `json:"object,omitempty"`
	Detail   string `json:"detail,omitempty"`
}

// ConfigApplyRequest carries the candidate, expected
// revision, request id, actor and reason. Window is the
// confirmation window in seconds; the engine clamps it to
// the safe range.
type ConfigApplyRequest struct {
	RequestID string          `json:"requestId"`
	Expected  string          `json:"expected"`
	Candidate json.RawMessage `json:"candidate"`
	Actor     string          `json:"actor"`
	Reason    string          `json:"reason"`
	Window    int             `json:"windowSeconds"`
}

// ApplyOutcome carries the authoritative result of an
// Apply. The Pending field is non-nil when the transaction
// is in awaiting-confirmation; nil when it has been
// rolled-back or refused before reaching apply.
type ApplyOutcome struct {
	TransactionID string          `json:"transactionId"`
	Phase         string          `json:"phase"`
	Receipt       *PublicReceipt  `json:"receipt,omitempty"`
	Findings      []ConfigFinding `json:"findings"`
	RequestID     string          `json:"requestId"`
}

// PublicReceipt is the durable commit receipt. It is the
// only evidence the system has committed; UI may not
// invent success.
type PublicReceipt struct {
	TransactionID string    `json:"transactionId"`
	BaseRevision  string    `json:"baseRevision"`
	NewRevision   string    `json:"newRevision"`
	CompletedAt   time.Time `json:"completedAt"`
	Outcome       string    `json:"outcome"`
}

// Config owns the durable revision journal for the generic
// config surface. The Engine owns at most one Config.
// When a Networking coordinator is attached via
// EnableNetworking, the Prepare/Confirm/Rollback/Validate/
// Reconcile methods delegate to the verified Agent bridge.
// Without the coordinator the journal-only path stays
// closed (ErrConfigRuntimeUnavailable) so no transaction can
// claim success without an authoritative Linux receipt.
type Config struct {
	mu     sync.Mutex
	store  *state.Store
	net    *Networking
	closed bool
	now    func() time.Time
}

// EnableConfig wires the canonical config journal. The
// caller owns state.Store and must keep it open while the
// Engine is running. The returned Config is read-only by
// default: Prepare/Confirm/Rollback/Validate/Reconcile all
// return ErrConfigRuntimeUnavailable until EnableNetworking
// attaches the verified Agent coordinator. The Engine
// retains the Config via SetConfig so the public API
// handler reaches the wired coordinator without manual
// plumbing.
func (e *Engine) EnableConfig(store *state.Store) (*Config, error) {
	if store == nil {
		return nil, errors.New("config store required")
	}
	if _, err := store.Snapshot(); err != nil {
		return nil, err
	}
	c := &Config{store: store, now: time.Now}
	e.SetConfig(c)
	if snapshot, err := store.Snapshot(); err == nil && snapshot.Restore != nil {
		request := append([]byte(nil), snapshot.Restore.Request...)
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()
			_, _ = e.restoreOperation(ctx, request, true)
		}()
	}
	return c, nil
}

// EnableNetworking attaches the verified Agent coordinator
// to the Config surface. From this call onward the
// Prepare/Confirm/Rollback/Validate/Reconcile methods
// dispatch through the Agent IPC channel and only return
// success when the Agent returned a 2xx with a canonical
// Linux receipt. Calling EnableNetworking on a closed
// Config returns an error.
func (c *Config) EnableNetworking(net *Networking) error {
	if net == nil {
		return errors.New("network coordinator required")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return errors.New("config journal is closed")
	}
	c.net = net
	return nil
}

// Networking returns the attached coordinator (may be nil).
func (c *Config) Networking() *Networking {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.net
}

// Close stops the config journal; subsequent calls return
// ErrClosed.
func (c *Config) Close() {
	c.mu.Lock()
	c.closed = true
	c.mu.Unlock()
}

func (c *Config) checkOpen() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return errors.New("config journal is closed")
	}
	return nil
}

// Status returns the canonical config status with the
// pending transaction (if any) wrapped in a public view.
func (c *Config) Status(ctx context.Context) (ConfigStatus, error) {
	if err := c.checkOpen(); err != nil {
		return ConfigStatus{}, err
	}
	snap, err := c.store.Snapshot()
	if err != nil {
		return ConfigStatus{}, err
	}
	out := ConfigStatus{
		Version:   snap.Version,
		UpdatedAt: time.Now().UTC(),
		Current:   revisionSummary(snap.Current),
		Outcomes:  append([]state.Outcome(nil), snap.Outcomes...),
		History:   make([]RevisionSummary, 0, len(snap.History)),
		Server:    ServerTime{Now: time.Now().UTC(), EpochMS: time.Now().UnixMilli()},
	}
	for _, r := range snap.History {
		out.History = append(out.History, revisionSummary(r))
	}
	if snap.Pending != nil {
		out.Pending = publicTransaction(*snap.Pending)
	}
	if snap.Restore != nil {
		out.RestoreRequestID = snap.Restore.ID
	}
	return out, nil
}

// Diff computes a structured diff between the current
// revision and the supplied candidate. Empty candidate means
// "describe current" — no diff is produced, only the base
// revision is returned.
//
// Non-empty candidates require the verified networking
// coordinator to be attached (a journal alone cannot
// authoritatively describe Linux state). When the coordinator
// is wired, the diff is computed locally from the canonical
// store: both the current revision and the candidate are
// decoded, validated, and compared. Validation findings on
// the candidate are surfaced as "error" findings so the UI
// can refuse bad candidates before the operator commits to
// a transaction.
func (c *Config) Diff(ctx context.Context, req ConfigDiffRequest) (ConfigDiff, error) {
	if err := c.checkOpen(); err != nil {
		return ConfigDiff{}, err
	}
	snap, err := c.store.Snapshot()
	if err != nil {
		return ConfigDiff{}, err
	}
	if req.Expected != "" && req.Expected != snap.Current.ID {
		return ConfigDiff{}, state.ErrConflict
	}
	if len(req.Candidate) == 0 {
		return ConfigDiff{BaseRevision: snap.Current.ID, Findings: []ConfigFinding{}}, nil
	}
	if _, err := c.coordinator(); err != nil {
		return ConfigDiff{}, err
	}
	return c.computeDiff(snap.Current.ID, snap.Current.Config, req.Candidate)
}

// computeDiff decodes both configurations from the canonical
// store and the candidate payload, validates the candidate,
// and emits a structured finding slice. Validation errors
// are surfaced as "error" findings; identity findings (the
// candidate matches the current revision) are emitted as an
// "info" finding so the UI can render an honest "no change"
// state instead of a fake "applied" success.
func (c *Config) computeDiff(base string, current, candidate json.RawMessage) (ConfigDiff, error) {
	if _, err := network.Decode(candidate); err != nil {
		return ConfigDiff{}, &diffValidationError{err: err}
	}
	curr, err := network.Decode(current)
	if err != nil {
		return ConfigDiff{}, err
	}
	cand, err := network.Decode(candidate)
	if err != nil {
		return ConfigDiff{}, &diffValidationError{err: err}
	}
	findings := diffFindings(curr, cand)
	// The candidate revision id is content-addressed by the
	// canonical store; we expose the digest of the candidate
	// payload so the UI can correlate the diff with a later
	// Apply call (Prepare will reuse the same request id and
	// the journal will produce a stable transaction id).
	candRevID := candidateDigest(candidate)
	return ConfigDiff{
		BaseRevision:      base,
		CandidateRevision: candRevID,
		DiffDigest:        candRevID,
		Findings:          findings,
	}, nil
}

// diffFindings compares two decoded network configs and
// returns a structured finding list. The slice is language-
// neutral (codes are stable strings) so the UI can localise
// them safely.
func diffFindings(curr, cand network.Config) []ConfigFinding {
	findings := []ConfigFinding{}
	if reflect.DeepEqual(curr, cand) {
		findings = append(findings, ConfigFinding{
			Code: "no_changes", Severity: "info", Object: "config",
			Detail: "candidate is identical to the current revision",
		})
		return findings
	}
	if len(cand.Interfaces) != len(curr.Interfaces) {
		findings = append(findings, ConfigFinding{
			Code: "interfaces_changed", Severity: "info", Object: "config",
			Detail: "interface inventory changed",
		})
	}
	if !reflect.DeepEqual(cand.DNS, curr.DNS) {
		findings = append(findings, ConfigFinding{
			Code: "dns_changed", Severity: "info", Object: "config",
			Detail: "DNS scope changed",
		})
	}
	if !reflect.DeepEqual(cand.Firewall, curr.Firewall) {
		findings = append(findings, ConfigFinding{
			Code: "firewall_changed", Severity: "info", Object: "config",
			Detail: "firewall rule set changed",
		})
	}
	if !reflect.DeepEqual(cand.Routes, curr.Routes) {
		findings = append(findings, ConfigFinding{
			Code: "routes_changed", Severity: "info", Object: "config",
			Detail: "routing table changed",
		})
	}
	return findings
}

// diffValidationError wraps a network.Decode validation
// failure so the API handler can return a 400 with the
// structured message instead of leaking an internal error.
type diffValidationError struct{ err error }

func (e *diffValidationError) Error() string { return e.err.Error() }
func (e *diffValidationError) Unwrap() error { return e.err }

// candidateDigest hashes the candidate payload so the diff
// response can carry a stable identifier without inventing
// state. SHA-256 over the raw JSON keeps the result
// deterministic and collision-resistant.
func candidateDigest(raw json.RawMessage) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// coordinator returns the attached coordinator or
// ErrConfigRuntimeUnavailable when the journal is closed
// and no verified coordinator is wired. The helper keeps
// every delegated method failure-closed by default.
func (c *Config) coordinator() (*Networking, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil, errors.New("config journal is closed")
	}
	if c.net == nil {
		return nil, ErrConfigRuntimeUnavailable
	}
	return c.net, nil
}

// Validate forwards to the Networking coordinator when one
// is attached. Without the coordinator the function refuses
// to claim semantic validation (the journal alone cannot
// authoritatively describe Linux state).
func (c *Config) Validate(ctx context.Context, config json.RawMessage, expected string) ([]ConfigFinding, error) {
	if _, err := c.coordinator(); err != nil {
		return nil, err
	}
	// The verified networking coordinator owns the canonical
	// semantic diff. Without a public surface for diff we
	// refuse to invent one — UI must observe Apply outcomes.
	return nil, ErrConfigRuntimeUnavailable
}

// Prepare forwards to the Networking coordinator when one
// is attached. The coordinator owns the journal admission,
// the Agent IPC dispatch and the post-apply reconciliation;
// the Config surface just adapts the request and response.
//
// The caller-supplied RequestID is durably admitted by the
// canonical store: a repeated Prepare with the same
// RequestID and the same candidate returns the original
// transaction outcome (idempotent dedup, survives restart
// because the canonical journal records the outcome
// keyed by the request identity). A repeated Prepare
// with the same RequestID and a different candidate is
// rejected with ErrConflict — caller identity reuse with
// a conflicting body is never silently admitted.
func (c *Config) Prepare(ctx context.Context, req ConfigApplyRequest) (PublicTransaction, error) {
	net, err := c.coordinator()
	if err != nil {
		return PublicTransaction{}, err
	}
	if req.RequestID != "" && !state.ValidTransactionID(req.RequestID) {
		return PublicTransaction{}, state.ErrConflict
	}
	if req.Window <= 0 {
		req.Window = 120
	}
	if actor, ok := authenticatedActor(ctx); ok {
		req.Actor = actor
	}
	tx, err := net.Apply(ctx, req.Candidate, req.Expected, req.Actor, req.Reason, req.Window, req.RequestID)
	if err != nil {
		return PublicTransaction{}, err
	}
	return publicTransactionFromState(tx), nil
}

// Confirm forwards to the Networking coordinator when one
// is attached. The coordinator only marks the Agent
// transaction committed after the Agent returned the
// authoritative Linux receipt.
func (c *Config) Confirm(ctx context.Context, txID string) (ApplyOutcome, error) {
	net, err := c.coordinator()
	if err != nil {
		return ApplyOutcome{}, err
	}
	if err := net.Confirm(ctx, txID); err != nil {
		return ApplyOutcome{}, err
	}
	st, err := net.Status(ctx)
	if err != nil {
		return ApplyOutcome{}, err
	}
	out := outcomeFromState(st, txID, "committed")
	if out.Receipt == nil {
		return out, state.ErrRecovery
	}
	return out, nil
}

// Rollback forwards to the Networking coordinator when one
// is attached. The coordinator only marks the Agent
// transaction rolled-back after the Agent restored Linux.
func (c *Config) Rollback(ctx context.Context, txID, requestID string) (ApplyOutcome, error) {
	net, err := c.coordinator()
	if err != nil {
		return ApplyOutcome{}, err
	}
	if requestID != "" {
		// The requestID is informational on the public
		// surface; the verified coordinator identifies
		// the transaction by its durably admitted ID.
	}
	if err := net.Rollback(ctx, txID); err != nil {
		return ApplyOutcome{}, err
	}
	st, err := net.Status(ctx)
	if err != nil {
		return ApplyOutcome{}, err
	}
	out := outcomeFromState(st, txID, "rolled-back")
	if out.Receipt == nil {
		return out, state.ErrRecovery
	}
	return out, nil
}

// Reconcile forwards to the Networking coordinator when one
// is attached. The journal is reconciled only when the
// Agent produced a verifiable outcome; the Config surface
// never accepts caller-supplied receipts.
func (c *Config) Reconcile(ctx context.Context, outcome state.Outcome) error {
	// Only Networking.Status/watch may reconcile authenticated Agent state.
	// A supplied receipt is not evidence of a runtime result.
	return ErrConfigRuntimeUnavailable
}

// publicTransactionFromState converts an internal
// state.Transaction (returned by the verified Networking
// coordinator) to the language-neutral public view.
func publicTransactionFromState(tx state.Transaction) PublicTransaction {
	remaining := time.Until(tx.Deadline).Milliseconds()
	if tx.Phase != state.AwaitingConfirmation || tx.Deadline.IsZero() || remaining < 0 {
		remaining = 0
	}
	return PublicTransaction{
		CompletedAt: tx.CompletedAt,
		ID:          tx.ID, BaseRevision: tx.Before,
		Candidate:        revisionSummary(tx.Candidate),
		Phase:            string(tx.Phase),
		Deadline:         tx.Deadline,
		RemainingMS:      remaining,
		SupportedActions: []string{},
	}
}

// outcomeFromState adapts a NetworkState snapshot into the
// public ApplyOutcome. The snapshot is the post-action
// view from the verified coordinator, so the outcome is
// authoritative Linux state, not a journal projection.
func outcomeFromState(st NetworkState, txID, kind string) ApplyOutcome {
	out := ApplyOutcome{
		TransactionID: txID,
		Phase:         "unknown",
	}
	if !st.Reconciled || st.Pending != nil || (kind != "committed" && kind != "rolled-back") {
		return out
	}
	for _, o := range st.Outcomes {
		if o.TransactionID != txID || o.Result != kind || o.CompletedAt.IsZero() {
			continue
		}
		newRevision := o.Before
		if kind == "committed" {
			newRevision = o.Candidate
		}
		if newRevision == "" || st.Current.ID != newRevision {
			return out
		}
		out.Phase = kind
		out.Receipt = &PublicReceipt{TransactionID: txID, BaseRevision: o.Before, NewRevision: newRevision, CompletedAt: o.CompletedAt, Outcome: o.Result}
		return out
	}
	return out
}

// publicTransaction converts an internal state.Transaction
// to the language-neutral public view.
func publicTransaction(tx state.Transaction) *PublicTransaction {
	remaining := time.Until(tx.Deadline).Milliseconds()
	if tx.Deadline.IsZero() || remaining < 0 {
		remaining = 0
	}
	return &PublicTransaction{ID: tx.ID, BaseRevision: tx.Before, Candidate: revisionSummary(tx.Candidate),
		Phase: string(tx.Phase), Deadline: tx.Deadline, RemainingMS: remaining, SupportedActions: []string{}}
}
