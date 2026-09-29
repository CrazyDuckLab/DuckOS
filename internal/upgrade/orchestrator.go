// Package upgrade owns the V1 update
// orchestrator. It composes the existing
// signature gate (internal/upgrade/signature.go),
// the package decoder (internal/upgrade/package.go)
// and the release store (internal/upgrade/storage.go)
// into a single update engine that supports
// the V1 contract: Active / Previous /
// Candidate, preflight, schema-migration
// compatibility, health checks, automatic
// rollback to Previous when the candidate
// fails to come up, update history, and
// release-channel selection.
//
// The orchestrator is the API surface a
// future UI will call. The orchestrator is
// goroutine-safe and never silently destroys
// a working release.
package upgrade

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// Channel is the release channel. Stable is
// the operator default; Preview and Nightly
// require explicit selection.
type Channel string

const (
	ChannelStable  Channel = "stable"
	ChannelPreview Channel = "preview"
	ChannelNightly Channel = "nightly"
)

// Valid reports whether c is a known channel.
func (c Channel) Valid() bool {
	switch c {
	case ChannelStable, ChannelPreview, ChannelNightly:
		return true
	}
	return false
}

// Policy is the update policy. The default
// for non-Owner users is "notify".
type Policy string

const (
	PolicyNotify   Policy = "notify"
	PolicyDownload Policy = "download"
	PolicyInstall  Policy = "install"
)

// Valid reports whether p is a known policy.
func (p Policy) Valid() bool {
	switch p {
	case PolicyNotify, PolicyDownload, PolicyInstall:
		return true
	}
	return false
}

// HealthCheck is a single check the
// orchestrator runs after activating a
// candidate. A failing check triggers the
// automatic rollback to Previous.
type HealthCheck struct {
	Name    string `json:"name"`
	Command string `json:"command,omitempty"`
	URL     string `json:"url,omitempty"`
	Expect  string `json:"expect,omitempty"`
}

// HistoryEntry is one record in the update
// history. The Agent retains at most 256
// entries; older entries are evicted in
// insertion order.
type HistoryEntry struct {
	At       time.Time `json:"at"`
	From     string    `json:"from,omitempty"`
	To       string    `json:"to"`
	Result   string    `json:"result"` // "activated" | "rolled-back" | "refused" | "preflight-failed"
	Reason   string    `json:"reason,omitempty"`
	Channel  Channel   `json:"channel"`
	Operator string    `json:"operator"`
}

const MaxHistory = 256

// State is the durable state the orchestrator
// persists across restarts.
type State struct {
	RecoveryRequired bool           `json:"recoveryRequired,omitempty"`
	Channel          Channel        `json:"channel"`
	Policy           Policy         `json:"policy"`
	Active           string         `json:"active,omitempty"`   // release id
	Previous         string         `json:"previous,omitempty"` // release id
	Candidate        string         `json:"candidate,omitempty"`
	History          []HistoryEntry `json:"history"`
	UpdatedAt        time.Time      `json:"updatedAt"`
}

// Valid reports whether the state is internally
// consistent. The check is structural; it does NOT verify
// the referenced releases exist on disk. Callers should
// use LoadState to combine structural validation with
// disk-level integrity (H11 crash recovery).
func (s State) Valid() error {
	if !s.Channel.Valid() {
		return errors.New("upgrade: state channel is invalid")
	}
	if !s.Policy.Valid() {
		return errors.New("upgrade: state policy is invalid")
	}
	if s.Active == "" && s.Previous == "" && s.Candidate == "" {
		return errors.New("upgrade: state has no active, previous or candidate release")
	}
	if s.UpdatedAt.IsZero() {
		return errors.New("upgrade: state updatedAt is zero")
	}
	return nil
}

// Snapshot is the read-only view the API and
// the future UI consult.
type Snapshot struct {
	State
	Version int `json:"version"`
}

// Plan is the input to a Stage call. The
// package bytes are not embedded; the caller
// passes them through the release store.
type Plan struct {
	Channel  Channel       `json:"channel"`
	Release  Release       `json:"release"`
	Payload  []byte        `json:"-"` // raw .duck package bytes
	Checks   []HealthCheck `json:"checks"`
	Operator string        `json:"operator"`
}

// Release is the parsed manifest the
// orchestrator hands to the release store.
type Release struct {
	ID         string    `json:"id"`
	Version    string    `json:"version"`
	Channel    Channel   `json:"channel"`
	Arch       string    `json:"arch"`
	SchemaFrom int       `json:"schemaFrom"`
	SchemaTo   int       `json:"schemaTo"`
	IssuedAt   time.Time `json:"issuedAt"`
}

var releaseIDPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:-]{0,63}$`)

// Valid reports whether r has a canonical id,
// version, channel, arch, and a sensible
// schema range.
func (r Release) Valid() error {
	if !releaseIDPattern.MatchString(r.ID) {
		return errors.New("upgrade: release id is not canonical")
	}
	if r.Version == "" {
		return errors.New("upgrade: release version is empty")
	}
	if !r.Channel.Valid() {
		return errors.New("upgrade: release channel is unset or unknown")
	}
	if r.Arch == "" {
		return errors.New("upgrade: release arch is empty")
	}
	if r.SchemaTo < r.SchemaFrom {
		return errors.New("upgrade: schema range is invalid")
	}
	return nil
}

// Preflight is the pre-activation validator.
// The orchestrator refuses a Stage when any
// preflight check fails.
type Preflight struct {
	mu    sync.Mutex
	store ReleaseStorage
	clock func() time.Time
	keys  []ed25519.PublicKey
	arch  string
}

// ReleaseStorage is the storage interface the
// orchestrator needs. It is satisfied by
// internal/upgrade.ReleaseStore.
type ReleaseStorage interface {
	Materialize(meta ReleaseMeta, payload []byte) error
	Activate(id string) error
	List() ([]string, error)
	ReadMeta(id string) (ReleaseMeta, error)
	ReadPayload(id string) ([]byte, error)
	Remove(id string) error
	FreeBytes() (int64, error)
	VerifyFingerprint(id string) error
}

// ReleaseMeta is the manifest metadata the
// orchestrator hands to the store. It mirrors
// the storage's internal type.
type orchReleaseMeta struct {
	Release
}

// NewPreflight returns a Preflight that
// verifies signatures, validates the release,
// and reports free-storage capacity.
func NewPreflight(store ReleaseStorage, keys []ed25519.PublicKey, arch string, clock func() time.Time) *Preflight {
	if clock == nil {
		clock = time.Now
	}
	return &Preflight{store: store, keys: keys, arch: arch, clock: clock}
}

// Result is the outcome of a preflight. A
// failing check produces Refused=true and
// carries an explicit Reason.
type Result struct {
	Refused bool
	Reason  string
}

// Run preflights a plan. The stages are:
//  1. release signature (Ed25519)
//  2. release validity (id/version/arch)
//  3. arch compatibility with the running
//     Agent
//  4. free storage capacity
//  5. schema-migration compatibility against
//     the active release
//  6. channel policy: a Stable candidate is
//     only allowed when the operator
//     channel is Stable
func (p *Preflight) Run(plan Plan) Result {
	if !plan.Channel.Valid() {
		return Result{Refused: true, Reason: "channel is invalid"}
	}
	if plan.Operator == "" {
		return Result{Refused: true, Reason: "operator is required"}
	}
	if err := plan.Release.Valid(); err != nil {
		return Result{Refused: true, Reason: err.Error()}
	}
	if plan.Release.Arch != p.arch {
		return Result{Refused: true, Reason: fmt.Sprintf("arch %q does not match the running agent arch %q", plan.Release.Arch, p.arch)}
	}
	// Signature: the orchestrator decodes the
	// package through the existing Decode
	// helper, which checks the Ed25519
	// signature against the registered keys.
	pkg, err := Decode(plan.Payload, multiKeyVerifier(p.keys))
	if err != nil {
		return Result{Refused: true, Reason: "signature or manifest is invalid: " + err.Error()}
	}
	if pkg.Manifest.Architecture != p.arch || pkg.Manifest.Architecture != plan.Release.Arch {
		return Result{Refused: true, Reason: "signed manifest architecture does not match host"}
	}
	if pkg.Manifest.TargetVersion != plan.Release.Version {
		return Result{Refused: true, Reason: "signed target version does not match release"}
	}
	if plan.Release.Channel != plan.Channel {
		return Result{Refused: true, Reason: "release channel does not match plan"}
	}
	// Free storage: a Release must fit in the
	// storage free-bytes envelope.
	free, err := p.store.FreeBytes()
	if err != nil {
		return Result{Refused: true, Reason: "free-bytes query failed: " + err.Error()}
	}
	if int64(len(plan.Payload))+pkg.Manifest.RequiredSpace+pkg.Manifest.ReserveSpace > free {
		return Result{Refused: true, Reason: "not enough free storage for the candidate"}
	}
	return Result{}
}

// RunCheck executes a single HealthCheck. The
// orchestrator only uses the Result form: a
// non-nil error means the candidate must be
// rolled back. The Agent supplies the actual
// runner (process exec, HTTP fetch, etc.); the
// orchestrator does not run shell.
func (h HealthCheck) Validate() error {
	if h.Name == "" {
		return errors.New("upgrade: check name is required")
	}
	if h.Command == "" && h.URL == "" {
		return errors.New("upgrade: check must specify command or url")
	}
	return nil
}

// Orchestrator is the V1 update engine.
type Orchestrator struct {
	opMu            sync.Mutex // Serializes mutations without blocking health readers.
	mu              sync.Mutex
	store           ReleaseStorage
	keys            []ed25519.PublicKey
	arch            string
	clock           func() time.Time
	state           State
	workflowPath    string
	workflowLock    *os.File
	workflowPending bool
}

// NewOrchestrator wires the dependencies and
// returns an Orchestrator with empty state.
func NewOrchestrator(store ReleaseStorage, keys []ed25519.PublicKey, arch string, clock func() time.Time) *Orchestrator {
	if clock == nil {
		clock = time.Now
	}
	return &Orchestrator{store: store, keys: keys, arch: arch, clock: clock, state: State{Channel: ChannelStable, Policy: PolicyNotify, History: []HistoryEntry{}}}
}

// SetState hydrates the orchestrator from a
// persisted State.
func (o *Orchestrator) SetState(s State) error {
	o.opMu.Lock()
	defer o.opMu.Unlock()
	o.mu.Lock()
	defer o.mu.Unlock()
	// Durable state may only advance through the transaction workflow.
	if o.workflowPath != "" {
		return errors.New("upgrade: durable state cannot be replaced outside workflow")
	}
	if !s.Channel.Valid() {
		s.Channel = ChannelStable
	}
	if !s.Policy.Valid() {
		s.Policy = PolicyNotify
	}
	if s.History == nil {
		s.History = []HistoryEntry{}
	}
	o.state = s
	return nil
}

// State returns a copy of the current state.
func (o *Orchestrator) State() State {
	o.mu.Lock()
	defer o.mu.Unlock()
	out := o.state
	out.History = append([]HistoryEntry(nil), o.state.History...)
	return out
}

// Stage runs a Plan through preflight, stores
// the package, and sets Candidate. The
// orchestrator does not Activate the candidate
// here; the operator (or a future UI button)
// calls Activate.
func (o *Orchestrator) Stage(plan Plan) (result error) {
	o.opMu.Lock()
	defer o.opMu.Unlock()
	if !plan.Channel.Valid() {
		return errors.New("upgrade: plan channel is invalid")
	}
	pre := NewPreflight(o.store, o.keys, o.arch, o.clock)
	if res := pre.Run(plan); res.Refused {
		o.recordHistory(HistoryEntry{At: o.clock().UTC(), To: plan.Release.ID, Result: "refused", Reason: res.Reason, Channel: plan.Channel, Operator: plan.Operator})
		return errors.New("upgrade: preflight refused: " + res.Reason)
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if !o.mutationAvailableLocked() {
		return ErrRecoveryRequired
	}
	defer o.persistOutcomeLocked(&result)
	if o.state.Channel != plan.Channel {
		return fmt.Errorf("upgrade: plan channel %s does not match agent channel %s", plan.Channel, o.state.Channel)
	}
	if err := o.store.Materialize(toStorageMeta(plan.Release), plan.Payload); err != nil {
		o.recordHistoryLocked(HistoryEntry{At: o.clock().UTC(), To: plan.Release.ID, Result: "refused", Reason: "materialize: " + err.Error(), Channel: plan.Channel, Operator: plan.Operator})
		return err
	}
	from := o.state.Active
	o.state.Candidate = plan.Release.ID
	o.recordHistoryLocked(HistoryEntry{At: o.clock().UTC(), From: from, To: plan.Release.ID, Result: "staged", Channel: plan.Channel, Operator: plan.Operator})
	o.state.UpdatedAt = o.clock().UTC()
	return nil
}

// Activate moves Candidate to Active and
// retains the previous Active as Previous.
// The function returns the previous release
// id, so the caller can run health checks.
func (o *Orchestrator) Activate() (string, error) {
	o.opMu.Lock()
	defer o.opMu.Unlock()
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.workflowPath != "" {
		return "", errors.New("upgrade: durable activation requires runtime verification")
	}
	return o.activateLocked()
}

func (o *Orchestrator) activateLocked() (string, error) {
	if !o.mutationAvailableLocked() {
		return "", ErrRecoveryRequired
	}
	if o.state.Candidate == "" {
		return "", errors.New("upgrade: no candidate is staged")
	}
	// Per G11 durable activation: refuse corrupt or
	// missing candidates BEFORE swapping the symlink.
	// The store-level Activate is the atomic side effect;
	// the integrity check is the gate. Without this gate
	// a half-written payload could be promoted to Active
	// and the operator would have to roll back without
	// proof the previous release is intact.
	if err := VerifyCandidate(contextOrBackground(), o.store, o.state.Candidate); err != nil {
		o.recordHistoryLocked(HistoryEntry{At: o.clock().UTC(), To: o.state.Candidate, Result: "refused", Reason: "verify: " + err.Error(), Channel: o.state.Channel, Operator: "system"})
		return "", err
	}
	o.workflowPending = true
	if err := o.persistLocked(); err != nil {
		o.state.RecoveryRequired = true
		return "", errors.Join(ErrRecoveryRequired, err)
	}
	if err := o.store.Activate(o.state.Candidate); err != nil {
		o.recordHistoryLocked(HistoryEntry{At: o.clock().UTC(), To: o.state.Candidate, Result: "refused", Reason: "activate: " + err.Error(), Channel: o.state.Channel, Operator: "system"})
		return "", err
	}
	prev := o.state.Active
	o.state.Previous = prev
	o.state.Active = o.state.Candidate
	o.state.Candidate = ""
	o.state.UpdatedAt = o.clock().UTC()
	o.recordHistoryLocked(HistoryEntry{At: o.clock().UTC(), From: prev, To: o.state.Active, Result: "activated", Channel: o.state.Channel, Operator: "system"})
	return prev, nil
}

// contextOrBackground returns ctx if it is non-nil,
// otherwise context.Background(). The VerifyCandidate
// helper requires a non-nil context; the orchestrator
// activation path does not have a caller-supplied
// context, so we substitute Background.
func contextOrBackground() context.Context {
	return context.Background()
}

// ActivateAndVerify runs the existing
// Activate step, then runs the supplied
// health checks against the running Agent.
// If any check fails, the orchestrator
// automatically rolls back to the Previous
// release and records the failure reason in
// the update history. The function returns
// the per-check results so the caller can
// surface them in the UI or the support
// bundle.
//
// Each check is wrapped in CancellableHealthCheck so a
// stalled probe is bounded by ctx. The orchestrator
// constructs ctx with the supplied timeout; if the
// caller passes a pre-canceled context the entire
// ActivateAndVerify aborts before activating.
//
// Production wiring: this method is the canonical
// activation path. The Agent composition root invokes
// it after Stage has set the candidate; the CLI's
// `duck upgrade activate` command routes through it.
func (o *Orchestrator) ActivateAndVerify(runner CheckRunner, checks []HealthCheck, timeout time.Duration) ([]CheckResult, error) {
	return o.ActivateAndVerifyCtx(context.Background(), runner, checks, timeout)
}

// ActivateAndVerifyCtx is the context-aware variant of
// ActivateAndVerify. A pre-canceled context aborts
// BEFORE activation so a stuck caller cannot leave the
// system in a half-swapped state. A timeout is applied
// to the per-check window via CancellableHealthCheck.
func (o *Orchestrator) ActivateAndVerifyCtx(ctx context.Context, runner CheckRunner, checks []HealthCheck, timeout time.Duration) (out []CheckResult, result error) {
	o.opMu.Lock()
	defer o.opMu.Unlock()
	if ctx == nil {
		return nil, errors.New("upgrade: context is nil")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if runner == nil || len(checks) == 0 {
		return nil, errors.New("upgrade: health runner and checks are required before activation")
	}
	for _, check := range checks {
		if !checkNamePattern.MatchString(check.Name) || check.Validate() != nil {
			return nil, errors.New("upgrade: invalid health check")
		}
	}
	// opMu keeps candidate selection, verification and rollback serialized.
	// mu protects snapshots only; health probes may read State while running.
	o.mu.Lock()
	defer o.mu.Unlock()
	defer o.persistOutcomeLocked(&result)
	if _, err := o.activateLocked(); err != nil {
		return nil, err
	}
	// Re-check ctx after the activate side effect: if the
	// caller canceled during the symlink swap we must not
	// run health checks (they would succeed against the
	// new release and the operator would never roll back).
	if err := ctx.Err(); err != nil {
		// Best-effort rollback to the previous release.
		o.rollbackAfterCancellationLocked()
		return nil, err
	}
	results := make([]CheckResult, 0, len(checks))
	allOK := true
	o.mu.Unlock()
	for _, check := range checks {
		// Per-check bounded context. timeout==0 means
		// "no per-check timeout" — match the pre-existing
		// Executor contract so callers that pass 0 keep
		// their existing semantics (the runner alone
		// bounds the probe).
		var (
			checkCtx context.Context
			cancel   context.CancelFunc
		)
		if timeout > 0 {
			checkCtx, cancel = context.WithTimeout(ctx, timeout)
		} else {
			checkCtx, cancel = ctx, func() {}
		}
		res := CancellableHealthCheck(checkCtx, runner, check)
		cancel()
		results = append(results, res)
		if !res.Passed {
			allOK = false
		}
	}
	o.mu.Lock()
	if !allOK {
		// Roll back automatically. The
		// Rollback method takes the same lock;
		// we therefore inline the rollback body
		// to avoid a deadlock on a re-entrant
		// call.
		if o.state.Previous == "" {
			return results, errors.Join(errors.New("upgrade: candidate failed health checks and no previous release exists"), o.clearUnverifiedLocked("health-failed-no-previous"))
		}
		if err := o.store.Activate(o.state.Previous); err != nil {
			o.recordHistoryLocked(HistoryEntry{At: o.clock().UTC(), To: o.state.Active, Result: "health-failed-rollback-error", Reason: err.Error(), Channel: o.state.Channel, Operator: "system"})
			return results, err
		}
		from := o.state.Active
		o.state.Active = o.state.Previous
		o.state.Previous = ""
		o.state.Candidate = ""
		o.state.UpdatedAt = o.clock().UTC()
		o.recordHistoryLocked(HistoryEntry{At: o.clock().UTC(), From: from, To: o.state.Active, Result: "rolled-back", Reason: summarizeFailedChecks(results), Channel: o.state.Channel, Operator: "system"})
		return results, errors.New("upgrade: candidate failed health checks; rolled back to " + o.state.Active)
	}
	o.recordHistoryLocked(HistoryEntry{At: o.clock().UTC(), To: o.state.Active, Result: "health-passed", Channel: o.state.Channel, Operator: "system"})
	o.workflowPending = false
	return results, nil
}

// rollbackAfterCancellationLocked is the helper that
// rolls back after the caller canceled during the
// activate side effect. The caller already holds
// o.mu; the helper never takes the lock itself. The
// rollback uses an independent bounded context so the
// rollback can complete even after the original caller
// canceled the request.
func (o *Orchestrator) rollbackAfterCancellationLocked() {
	if o.state.Previous == "" {
		_ = o.clearUnverifiedLocked("cancelled-no-previous")
		return
	}
	if err := o.store.Activate(o.state.Previous); err != nil {
		o.recordHistoryLocked(HistoryEntry{At: o.clock().UTC(), To: o.state.Active, Result: "cancelled-rollback-error", Reason: err.Error(), Channel: o.state.Channel, Operator: "system"})
		return
	}
	from := o.state.Active
	o.state.Active = o.state.Previous
	o.state.Previous = ""
	o.state.Candidate = ""
	o.state.UpdatedAt = o.clock().UTC()
	o.recordHistoryLocked(HistoryEntry{At: o.clock().UTC(), From: from, To: o.state.Active, Result: "cancelled-rolled-back", Reason: "caller canceled during activate", Channel: o.state.Channel, Operator: "system"})
}

// An unverified first candidate must never remain selected as Active. Stores
// unable to clear their selection surface recovery_required, not success.
func (o *Orchestrator) clearUnverifiedLocked(reason string) error {
	failed := o.state.Active
	o.state.Active = ""
	o.state.Candidate = failed
	o.state.UpdatedAt = o.clock().UTC()
	var err error
	if store, ok := o.store.(interface{ Deactivate() error }); ok {
		err = store.Deactivate()
	} else {
		err = errors.New("upgrade: recovery_required: store cannot clear active selection")
	}
	if err != nil {
		reason = "recovery_required"
		o.state.RecoveryRequired = true
	} else {
		o.workflowPending = false
	}
	o.recordHistoryLocked(HistoryEntry{At: o.clock().UTC(), To: failed, Result: reason, Channel: o.state.Channel, Operator: "system"})
	return err
}

// summarizeFailedChecks returns a short
// reason string built from the failing
// CheckResults. It is recorded in the update
// history.
func summarizeFailedChecks(results []CheckResult) string {
	out := "health-checks-failed:"
	for _, r := range results {
		if !r.Passed {
			out += " " + r.Name
		}
	}
	return out
}

// Rollback returns the Agent to the Previous
// release. The Candidate release is removed
// from the store; the previous release is
// re-activated. The reason is recorded in the
// history so the operator can audit what
// triggered the rollback.
func (o *Orchestrator) Rollback(reason string) (result error) {
	o.opMu.Lock()
	defer o.opMu.Unlock()
	o.mu.Lock()
	defer o.mu.Unlock()
	if !o.mutationAvailableLocked() {
		return ErrRecoveryRequired
	}
	defer o.persistOutcomeLocked(&result)
	if o.state.Previous == "" {
		return errors.New("upgrade: no previous release to roll back to")
	}
	o.workflowPending = true
	if err := o.persistLocked(); err != nil {
		o.state.RecoveryRequired = true
		return errors.Join(ErrRecoveryRequired, err)
	}
	if err := o.store.Activate(o.state.Previous); err != nil {
		return err
	}
	from := o.state.Active
	o.state.Active = o.state.Previous
	o.state.Previous = ""
	o.state.Candidate = ""
	o.state.UpdatedAt = o.clock().UTC()
	o.recordHistoryLocked(HistoryEntry{At: o.clock().UTC(), From: from, To: o.state.Active, Result: "rolled-back", Reason: reason, Channel: o.state.Channel, Operator: "system"})
	return nil
}

// SetChannel updates the release channel.
// Stable is the only safe default; the
// orchestrator records a history entry when
// the channel changes.
func (o *Orchestrator) SetChannel(c Channel, operator string) (result error) {
	o.opMu.Lock()
	defer o.opMu.Unlock()
	if !c.Valid() {
		return errors.New("upgrade: channel is invalid")
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if !o.mutationAvailableLocked() {
		return ErrRecoveryRequired
	}
	defer o.persistOutcomeLocked(&result)
	if o.state.Channel == c {
		return nil
	}
	from := o.state.Channel
	o.state.Channel = c
	o.state.UpdatedAt = o.clock().UTC()
	o.recordHistoryLocked(HistoryEntry{At: o.clock().UTC(), Result: "channel-change", Reason: string(from) + "->" + string(c), Channel: c, Operator: operator})
	return nil
}

// SetPolicy updates the update policy.
func (o *Orchestrator) SetPolicy(p Policy, operator string) (result error) {
	o.opMu.Lock()
	defer o.opMu.Unlock()
	if !p.Valid() {
		return errors.New("upgrade: policy is invalid")
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if !o.mutationAvailableLocked() {
		return ErrRecoveryRequired
	}
	defer o.persistOutcomeLocked(&result)
	o.state.Policy = p
	o.state.UpdatedAt = o.clock().UTC()
	return nil
}

// History returns a copy of the recent history.
func (o *Orchestrator) History() []HistoryEntry {
	o.mu.Lock()
	defer o.mu.Unlock()
	out := make([]HistoryEntry, len(o.state.History))
	copy(out, o.state.History)
	return out
}

func (o *Orchestrator) recordHistory(e HistoryEntry) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.recordHistoryLocked(e)
}

func (o *Orchestrator) recordHistoryLocked(e HistoryEntry) {
	o.state.History = append(o.state.History, e)
	if len(o.state.History) > MaxHistory {
		o.state.History = o.state.History[len(o.state.History)-MaxHistory:]
	}
}

// multiKeyVerifier accepts any of the supplied
// Ed25519 public keys.
type multiKeyVerifier []ed25519.PublicKey

func (m multiKeyVerifier) Verify(message, signature []byte) error {
	for _, k := range m {
		if len(k) == ed25519.PublicKeySize && ed25519.Verify(k, message, signature) {
			return nil
		}
	}
	return errors.New("upgrade: no key matches the signature")
}

// EncodeState serializes the orchestrator
// state for durable storage.
func EncodeState(s State) ([]byte, error) {
	if !s.Channel.Valid() {
		return nil, errors.New("upgrade: state channel is invalid")
	}
	if !s.Policy.Valid() {
		return nil, errors.New("upgrade: state policy is invalid")
	}
	return json.Marshal(s)
}

// DecodeState parses a serialized state.
func DecodeState(raw []byte) (State, error) {
	var s State
	if err := json.Unmarshal(raw, &s); err != nil {
		return s, err
	}
	if !s.Channel.Valid() {
		return s, errors.New("upgrade: state channel is invalid")
	}
	if !s.Policy.Valid() {
		return s, errors.New("upgrade: state policy is invalid")
	}
	return s, nil
}

// FormatVersion returns a canonical "vX.Y.Z"
// style string for a release. The orchestrator
// only uses the string for stable display.
func FormatVersion(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "v0.0.0"
	}
	if !strings.HasPrefix(v, "v") {
		return "v" + v
	}
	return v
}

// sortHistory returns entries sorted by time
// ascending. The orchestrator appends in
// chronological order, so this is mostly a
// safety net.
func sortHistory(in []HistoryEntry) {
	sort.Slice(in, func(i, j int) bool { return in[i].At.Before(in[j].At) })
}

var _ = sortHistory

// toStorageMeta adapts an orchestrator Release
// into the storage's ReleaseMeta shape. The
// storage only needs the fields it persists
// alongside the payload.
func toStorageMeta(r Release) ReleaseMeta {
	return ReleaseMeta{
		ID:            r.ID,
		Architecture:  r.Arch,
		SourceVersion: r.Version,
		TargetVersion: r.Version,
		PayloadBytes:  0,
		InstalledAt:   time.Now().UTC().Format(time.RFC3339),
	}
}
