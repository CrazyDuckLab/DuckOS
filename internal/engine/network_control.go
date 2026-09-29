package engine

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/agent"
	"github.com/CrazyDuckLab/DuckOS/internal/network"
	"github.com/CrazyDuckLab/DuckOS/internal/state"
)

// nextPhaseTowards returns the single legal transition step that
// moves `from` one step closer to `to`. The Agent's rollback
// chain walks Applying → RollingBack → RecoveryRequired; the
// store only accepts single-step edges, so the Engine must
// traverse the chain via intermediate RollingBack. An empty
// result means no legal edge reaches `to` from `from`.
func nextPhaseTowards(from, to state.Phase) state.Phase {
	if from == to {
		return ""
	}
	switch to {
	case state.AwaitingConfirmation:
		if from == state.Applying {
			return state.AwaitingConfirmation
		}
		return ""
	case state.RollingBack:
		// RollingBack is reachable from Prepared, Applying, or
		// any phase except RollingBack/RecoveryRequired itself.
		if from == state.Prepared || from == state.Applying {
			return state.RollingBack
		}
		return ""
	case state.RecoveryRequired:
		// RecoveryRequired is only reachable from RollingBack.
		// If we are still upstream of RollingBack, take that
		// edge first.
		if from == state.RollingBack {
			return state.RecoveryRequired
		}
		if from == state.Prepared || from == state.Applying {
			return state.RollingBack
		}
		return ""
	}
	return ""
}

// Networking owns canonical Engine intent. Agent owns execution/recovery;
// neither an RPC timeout nor a matching config digest is a commit receipt.
type Networking struct {
	mu       sync.Mutex
	store    *state.Store
	client   *http.Client
	ctx      context.Context
	cancel   context.CancelFunc
	done     chan struct{}
	requests sync.WaitGroup
	closed   bool
}

type NetworkState struct {
	Version    int                  `json:"version"`
	Current    state.Revision       `json:"current"`
	Pending    *state.Transaction   `json:"pending"`
	Outcomes   []state.Outcome      `json:"outcomes"`
	Agent      *agent.NetworkStatus `json:"agent,omitempty"`
	Reconciled bool                 `json:"reconciled"`
}

func (e *Engine) EnableNetworking(store *state.Store) (*Networking, error) {
	if store == nil {
		return nil, errors.New("canonical network store required")
	}
	snap, err := store.Snapshot()
	if err != nil {
		return nil, err
	}
	if _, err = network.Decode(snap.Current.Config); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	n := &Networking{store: store, client: e.client, ctx: ctx, cancel: cancel, done: make(chan struct{})}
	go n.bootstrap(store, snap)
	return n, nil
}

// bootstrap installs the Agent's Current revision as the canonical
// journal Current when no committed work has landed on the host
// (no Pending, no committed History). Rolled-back outcomes are
// tolerated; they are noise from rejected apply attempts against a
// placeholder Current that the engine is about to replace. The
// journal's placeholder Current.ID was the SHA-256 of the
// placeholder bytes, which never matched the Agent's startup
// Current — without this adoption the very first Prepare from the
// UI races against the Agent's startup state and always loses with
// HTTP 409 revision_conflict. After any successful Apply the
// History is non-empty and bootstrap becomes a no-op; the Agent's
// authoritative Current is the only durable baseline.
func (n *Networking) bootstrap(store *state.Store, snap state.Snapshot) {
	defer close(n.done)
	if snap.Pending != nil || len(snap.History) != 0 {
		// Either a transaction is in flight or the host has
		// already received a committed configuration. Do not
		// disturb the journal in either case.
		n.runWatch()
		return
	}
	// Borrow a short-lived context for the initial
	// adoption — the Engine has not provided a
	// long-lived one and the bootstrap is bounded.
	// Tests that construct Networking directly bypass
	// EnableNetworking and leave n.ctx nil; fall back to
	// context.Background() in that case so the bootstrap
	// helper stays safe to call from tests.
	parent := n.ctx
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	if remote, err := n.remoteStatus(ctx); err == nil && remote.Current.ID != "" && remote.Current.ID != snap.Current.ID {
		if err := store.SeedCurrent(remote.Current); err != nil {
			fmt.Fprintf(os.Stderr, "engine: bootstrap SeedCurrent failed: %v\n", err)
		}
	}
	n.runWatch()
}

// runWatch is the periodic Status poll that reconciles the
// journal with the Agent. Eagerly runs once at startup so the
// first user-visible transaction does not race against the
// Agent's boot state (the BD-001 syndrome caught in live
// qualification).
func (n *Networking) runWatch() {
	// Tests that construct Networking directly bypass
	// EnableNetworking and leave n.store / n.client / n.ctx
	// nil. The watcher is meaningless without them; signal
	// completion so the test goroutine unblocks.
	if n.store == nil || n.client == nil || n.ctx == nil {
		return
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	_, _ = n.Status(n.ctx)
	for {
		select {
		case <-n.ctx.Done():
			return
		case <-ticker.C:
			snap, err := n.store.Snapshot()
			if err == nil && snap.Pending != nil {
				_, _ = n.Status(n.ctx)
			}
		}
	}
}

func (n *Networking) Close() {
	n.mu.Lock()
	n.closed = true
	n.cancel()
	n.mu.Unlock()
	<-n.done
	n.requests.Wait()
}

func (n *Networking) Apply(ctx context.Context, candidate json.RawMessage, expected, actor, reason string, window int, requestID string) (state.Transaction, error) {
	if _, err := network.Decode(candidate); err != nil {
		return state.Transaction{}, err
	}
	if window < 120 || window > 900 {
		return state.Transaction{}, errors.New("invalid confirmation window")
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.closed {
		return state.Transaction{}, state.ErrRecovery
	}
	// Caller-supplied requestID is durably admitted by the
	// canonical journal. A repeated Prepare with the same
	// requestID + same candidate returns the original
	// transaction outcome (idempotent dedup). A repeated
	// Prepare with the same requestID + different candidate
	// is rejected with ErrConflict — caller identity reuse
	// with a conflicting body is never silently admitted.
	tx, err := n.store.PrepareRequestWithWindow(expected, requestID, candidate, actor, reason, nil, window)
	if err != nil {
		return state.Transaction{}, err
	}
	if tx.Phase != state.Prepared && tx.Phase != state.Applying {
		return tx, nil
	}
	remote, err := n.remoteStatus(ctx)
	if err != nil {
		return state.Transaction{}, err
	}
	if remote.Current.ID != expected || (remote.Pending != nil && remote.Pending.ID != tx.ID) || (remote.Job.Phase == "preflight" && remote.Job.RequestID != tx.ID) {
		_ = n.store.Reconcile(state.Outcome{TransactionID: tx.ID, Before: tx.Before, Candidate: tx.Candidate.ID, Result: "rolled-back", CompletedAt: time.Now().UTC()})
		return state.Transaction{}, state.ErrConflict
	}
	if tx.Phase == state.Prepared {
		if err = n.store.Transition(tx.ID, state.Applying, time.Time{}); err != nil {
			return state.Transaction{}, err
		}
		tx.Phase = state.Applying
	}
	// If the transaction is already in-flight or completed
	// (a retried Prepare that found the prior submission
	// committed or rolled-back), the canonical journal is
	// already the source of truth; no further Agent IPC is
	// needed. The Engine only fires the Agent POST for a
	// freshly admitted transaction.
	if tx.Phase != state.Applying {
		return tx, nil
	}
	input := agent.NetworkApply{RequestID: tx.ID, ExpectedRevision: expected, Candidate: tx.Candidate.Config, Actor: actor, Reason: reason, ConfirmSeconds: window, CreatedAt: tx.Candidate.CreatedAt}
	n.requests.Add(1)
	go func() {
		defer n.requests.Done()
		// Exactly one submission. Reconciliation polls Agent after any response,
		// including a lost acknowledgement; it never blindly replays a write.
		_, _ = n.call(n.ctx, "apply", input)
	}()
	return tx, nil
}

func (n *Networking) Status(ctx context.Context) (NetworkState, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.closed {
		return NetworkState{}, state.ErrRecovery
	}
	remote, err := n.remoteStatus(ctx)
	if err != nil {
		return NetworkState{}, err
	}
	if err = n.reconcile(remote); err != nil {
		return NetworkState{}, err
	}
	snap, err := n.store.Snapshot()
	if err != nil {
		return NetworkState{}, err
	}
	if snap.Pending != nil {
		snap.Pending.Recovery = nil
	}
	return NetworkState{Version: 1, Current: snap.Current, Pending: snap.Pending, Outcomes: snap.Outcomes, Agent: &remote, Reconciled: snap.Pending == nil && snap.Current.ID == remote.Current.ID}, nil
}

func (n *Networking) Confirm(ctx context.Context, id string) error {
	return n.control(ctx, id, "confirm")
}
func (n *Networking) Rollback(ctx context.Context, id string) error {
	return n.control(ctx, id, "rollback")
}
func (n *Networking) control(ctx context.Context, id, operation string) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.closed || !state.ValidTransactionID(id) {
		return state.ErrConflict
	}
	snap, err := n.store.Snapshot()
	if err != nil {
		return err
	}
	if snap.Pending == nil || snap.Pending.ID != id {
		return state.ErrConflict
	}
	data, err := n.call(ctx, operation, map[string]string{"transactionId": id})
	if err != nil {
		return err
	}
	_ = data
	// Poll the Agent's status until the canonical journal is
	// reconciled — the Agent's control handler runs the actual
	// rollback/confirm in a goroutine; we must wait for the
	// outcome to land in remote.Outcomes before the canonical
	// Pending record is cleared and an outcome is appended.
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		// Re-read canonical Pending each iteration: reconcile
		// may have cleared it.
		currentSnap, _ := n.store.Snapshot()
		if currentSnap.Pending == nil || currentSnap.Pending.ID != id {
			return nil
		}
		remote, err := n.remoteStatus(ctx)
		if err != nil {
			break
		}
		_ = n.reconcile(remote)
		time.Sleep(50 * time.Millisecond)
	}
	return nil
}

func (n *Networking) reconcile(remote agent.NetworkStatus) error {
	snap, err := n.store.Snapshot()
	if err != nil {
		return err
	}
	p := snap.Pending
	if p == nil {
		if remote.Current.ID != snap.Current.ID || remote.Pending != nil {
			return state.ErrConflict
		}
		return nil
	}
	for _, o := range remote.Outcomes {
		if o.TransactionID != p.ID {
			continue
		}
		want := o.Before
		if o.Result == "committed" {
			want = o.Candidate
		}
		if remote.Current.ID != want || remote.Pending != nil {
			return state.ErrConflict
		}
		return n.store.Reconcile(o)
	}
	if remote.Current.ID != p.Before {
		return state.ErrConflict
	}
	if remote.Pending != nil {
		if remote.Pending.ID != p.ID || remote.Pending.Before != p.Before || remote.Pending.Candidate.ID != p.Candidate.ID {
			return state.ErrConflict
		}
		// Forward-sync the phase the Agent has advanced to so the
		// canonical journal mirrors the host state. The Agent may
		// walk the transition chain faster than the Engine polls,
		// e.g. Applying → RollingBack → RecoveryRequired; the
		// store only allows single-step transitions, so we must
		// step the Engine's view through the intermediate phases
		// until it reaches the Agent's authoritative phase.
		for p.Phase != state.Phase(remote.Pending.Phase) || !p.Deadline.Equal(remote.Pending.Deadline) {
			next := nextPhaseTowards(p.Phase, state.Phase(remote.Pending.Phase))
			if next == "" {
				return errors.New("engine cannot mirror agent phase transition")
			}
			deadline := remote.Pending.Deadline
			if next != state.AwaitingConfirmation {
				deadline = time.Time{}
			}
			if err := n.store.Transition(p.ID, next, deadline); err != nil {
				return err
			}
			s, err := n.store.Snapshot()
			if err != nil {
				return err
			}
			p = s.Pending
		}
		return nil
	}
	// Absence is not an abort receipt. Ask Agent to persist an admission
	// tombstone while holding its worker lock. This is idempotent cancellation,
	// never a retry of apply. A still-active preflight refuses cancellation.
	failed := remote.Job.RequestID == p.ID && remote.Job.Phase == "failed"
	if failed || time.Since(p.Candidate.CreatedAt) > 65*time.Second {
		input := agent.NetworkApply{RequestID: p.ID, ExpectedRevision: p.Before, Candidate: p.Candidate.Config, Actor: p.Candidate.Actor, Reason: p.Candidate.Reason, ConfirmSeconds: 120, CreatedAt: p.Candidate.CreatedAt}
		_, _ = n.call(n.ctx, "cancel-admission", input)
		// Only a subsequent status receipt authorizes canonical reconciliation.
	}
	return nil
}

// reconcileFromOutcome lets the Config surface ask the
// verified Networking coordinator to apply a single outcome
// receipt. The receipt must originate from the Agent (the
// Config surface refuses caller-supplied receipts); this
// helper exists so the public ApplyOutcome adapter can
// update the journal after a Confirm/Rollback succeeded.
func (n *Networking) reconcileFromOutcome(o state.Outcome) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.closed {
		return state.ErrRecovery
	}
	snap, err := n.store.Snapshot()
	if err != nil {
		return err
	}
	if snap.Pending == nil {
		return state.ErrConflict
	}
	return n.store.Reconcile(o)
}

func (n *Networking) remoteStatus(ctx context.Context) (agent.NetworkStatus, error) {
	data, err := n.call(ctx, "status", nil)
	if err != nil {
		return agent.NetworkStatus{}, err
	}
	var status agent.NetworkStatus
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if d.Decode(&status) != nil || d.Decode(new(any)) != io.EOF || status.Version != 1 || len(status.Outcomes) > 32 || len(status.Current.Config) > state.MaxConfigBytes {
		return status, errors.New("invalid network Agent response")
	}
	if !validNetworkRevision(status.Current) {
		return status, errors.New("invalid Agent network configuration")
	}
	if p := status.Pending; p != nil {
		if len(p.Recovery) > 0 || !state.ValidTransactionID(p.ID) || p.Before != status.Current.ID || p.Candidate.ID == p.Before || !validNetworkRevision(p.Candidate) {
			return status, errors.New("invalid public Agent transaction")
		}
		switch p.Phase {
		case state.Prepared, state.Applying, state.RollingBack, state.RecoveryRequired:
		case state.AwaitingConfirmation:
			if p.Deadline.IsZero() {
				return status, errors.New("missing Agent confirmation deadline")
			}
		default:
			return status, errors.New("invalid Agent transaction phase")
		}
	}
	seen := make(map[string]bool)
	for _, o := range status.Outcomes {
		if !state.ValidTransactionID(o.TransactionID) || seen[o.TransactionID] || !validRevisionID(o.Before) || !validRevisionID(o.Candidate) || o.Before == o.Candidate || o.CompletedAt.IsZero() || (o.Result != "committed" && o.Result != "rolled-back") {
			return status, errors.New("invalid Agent outcome receipt")
		}
		seen[o.TransactionID] = true
	}
	if j := status.Job; (j.RequestID != "" && !state.ValidTransactionID(j.RequestID)) || (j.TransactionID != "" && !state.ValidTransactionID(j.TransactionID)) || len(j.Phase) > 32 || len(j.Error) > 256 || (j.CandidateRevision != "" && !validRevisionID(j.CandidateRevision)) {
		return status, errors.New("invalid Agent progress")
	}
	return status, nil
}

func validRevisionID(id string) bool {
	b, err := hex.DecodeString(id)
	return err == nil && len(b) == sha256.Size && hex.EncodeToString(b) == id
}

func validNetworkRevision(r state.Revision) bool {
	if len(r.Config) > state.MaxConfigBytes || r.CreatedAt.IsZero() || len(r.Actor) == 0 || len(r.Actor) > 128 || len(r.Reason) == 0 || len(r.Reason) > 512 {
		return false
	}
	digest := sha256.Sum256(r.Config)
	if hex.EncodeToString(digest[:]) != r.ID {
		return false
	}
	_, err := network.Decode(r.Config)
	return err == nil
}

func (n *Networking) call(ctx context.Context, operation string, input any) ([]byte, error) {
	method := http.MethodGet
	var body []byte
	if input != nil {
		method = http.MethodPost
		var err error
		body, err = json.Marshal(input)
		if err != nil {
			return nil, err
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, "http://agent"+agent.NetworkPrefix+operation, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	if input != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	response, err := n.client.Do(req)
	if err != nil {
		return nil, errors.New("network Agent unavailable; outcome must be reconciled")
	}
	defer response.Body.Close()
	if response.StatusCode != 200 && response.StatusCode != 202 {
		return nil, errors.New("network Agent rejected request; inspect transaction status")
	}
	if response.Header.Get("Content-Type") != "application/json" {
		return nil, errors.New("invalid network Agent content type")
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if err != nil || len(data) > 1<<20 {
		return nil, errors.New("network Agent response exceeds limits")
	}
	return data, nil
}
