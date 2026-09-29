// Package safeapply coordinates a durable privileged apply journal and an
// independent confirmation watchdog. Adapter implementations must be bounded,
// idempotent, and restricted to enrolled DuckOS-owned host objects.
package safeapply

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/state"
)

// Adapter must validate all generated daemon configurations before changing
// live state. Restore must also work after an interrupted partial Apply.
type Adapter interface {
	Preflight(context.Context, json.RawMessage, json.RawMessage) error
	Apply(context.Context, json.RawMessage, json.RawMessage) error
	Verify(context.Context, json.RawMessage) error
	Restore(context.Context, json.RawMessage, json.RawMessage) error
}

// SnapshotAdapter records bounded host before-state in the private Agent
// journal before a mutation. It must exclude secrets and revalidate recovery
// evidence on restore. No client can supply this evidence through Apply.
type SnapshotAdapter interface {
	Capture(context.Context, json.RawMessage, json.RawMessage) (json.RawMessage, error)
	RestoreCaptured(context.Context, json.RawMessage, json.RawMessage, json.RawMessage) error
}

type Controller struct {
	mu          sync.Mutex
	store       *state.Store
	adapter     Adapter
	stop        chan struct{}
	done        chan struct{}
	closed      bool
	recoveryErr error
	expires     time.Time // In-process monotonic deadline; restart always rolls back.
}

// Open performs recovery before admitting new work. The caller owns Store and
// must keep it open until Close has completed. Pending state is never resumed
// or confirmed implicitly, including when its deadline has not expired.
func Open(store *state.Store, adapter Adapter) (*Controller, error) {
	if store == nil || adapter == nil {
		return nil, errors.New("store and adapter are required")
	}
	c := &Controller{store: store, adapter: adapter, stop: make(chan struct{}), done: make(chan struct{})}
	snap, err := store.Snapshot()
	if err != nil {
		return nil, err
	}
	if snap.Pending != nil {
		// RecoveryRequired is evidence of an unknown live state, not a
		// completed rollback. Replay durable undo and verify it before
		// admitting work; another failure must preserve the pending record.
		if err = c.rollbackLocked(); err != nil {
			return nil, err
		}
	}
	go c.watch()
	return c, nil
}

func (c *Controller) Status() (state.Snapshot, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.recoveryErr != nil {
		return state.Snapshot{}, c.recoveryErr
	}
	snap, err := c.store.Snapshot()
	if snap.Pending != nil {
		snap.Pending.Recovery = nil
	}
	return snap, err
}

// Apply does not inherit an HTTP request's lifetime. A disconnected browser
// cannot cancel the rollback controller. All actual work still has a hard cap.
func (c *Controller) Apply(expected string, candidate json.RawMessage, actor, reason string, window time.Duration) (state.Transaction, error) {
	return c.ApplyRequest(expected, "", candidate, actor, reason, window)
}

// ApplyRequest carries one Engine identity through both durable journals.
func (c *Controller) ApplyRequest(expected, requestID string, candidate json.RawMessage, actor, reason string, window time.Duration) (state.Transaction, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.recoveryErr != nil {
		return state.Transaction{}, state.ErrRecovery
	}
	if (requestID != "" && !state.ValidTransactionID(requestID)) || len(candidate) > state.MaxConfigBytes || !json.Valid(candidate) || len(actor) == 0 || len(actor) > 128 || len(reason) == 0 || len(reason) > 512 {
		return state.Transaction{}, errors.New("invalid apply request identity, metadata or size")
	}
	if window < 2*time.Minute || window > 15*time.Minute {
		return state.Transaction{}, errors.New("confirmation window must be 2–15 minutes")
	}
	snap, err := c.store.Snapshot()
	if err != nil {
		return state.Transaction{}, err
	}
	if snap.Pending != nil {
		return state.Transaction{}, state.ErrPending
	}
	if snap.Current.ID != expected {
		return state.Transaction{}, state.ErrConflict
	}
	// Copy caller data before an adapter is allowed to inspect it.
	candidate = append(json.RawMessage(nil), candidate...)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	err = c.adapter.Preflight(ctx, snap.Current.Config, candidate)
	cancel()
	if err != nil {
		fmt.Fprintf(os.Stderr, "safeapply: Preflight failed requestID=%s err=%q\n", requestID, err.Error())
		return state.Transaction{}, fmt.Errorf("candidate preflight failed; live state unchanged: %w", err)
	}
	var recovery json.RawMessage
	if adapter, ok := c.adapter.(SnapshotAdapter); ok {
		ctx, cancel = context.WithTimeout(context.Background(), 15*time.Second)
		recovery, err = adapter.Capture(ctx, snap.Current.Config, candidate)
		cancel()
		if err != nil {
			fmt.Fprintf(os.Stderr, "safeapply: Capture failed requestID=%s err=%q\n", requestID, err.Error())
			return state.Transaction{}, fmt.Errorf("could not preserve host before-state; live state unchanged: %w", err)
		}
		if len(recovery) == 0 || !json.Valid(recovery) {
			return state.Transaction{}, errors.New("invalid host recovery evidence; live state unchanged")
		}
	}
	tx, err := c.store.PrepareRequest(expected, requestID, candidate, actor, reason, recovery)
	if err != nil {
		return state.Transaction{}, err
	}
	if err = c.store.Transition(tx.ID, state.Applying, time.Time{}); err != nil {
		return state.Transaction{}, err
	}
	ctx, cancel = context.WithTimeout(context.Background(), 90*time.Second)
	err = c.adapter.Apply(ctx, snap.Current.Config, tx.Candidate.Config)
	if err == nil {
		err = c.adapter.Verify(ctx, tx.Candidate.Config)
	}
	cancel()
	if err != nil {
		if rollbackErr := c.rollbackLocked(); rollbackErr != nil {
			return state.Transaction{}, rollbackErr
		}
		return state.Transaction{}, errors.New("apply or verification failed; previous configuration restored and verified")
	}
	c.expires = time.Now().Add(window)
	if err = c.store.Transition(tx.ID, state.AwaitingConfirmation, c.expires); err != nil {
		// A journal write may have reached disk even when fsync failed. Do not
		// pretend rollback was durably recorded; restore host state and fail closed.
		ctx, cancel = context.WithTimeout(context.Background(), 90*time.Second)
		restoreErr := c.restore(ctx, snap.Current.Config, tx)
		if restoreErr == nil {
			restoreErr = c.adapter.Verify(ctx, snap.Current.Config)
		}
		cancel()
		c.recoveryErr = state.ErrRecovery
		return state.Transaction{}, state.ErrRecovery
	}
	snap, err = c.store.Snapshot()
	if err != nil {
		return state.Transaction{}, err
	}
	tx = *snap.Pending
	tx.Recovery = nil
	return tx, nil
}

func (c *Controller) Confirm(id string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.recoveryErr != nil {
		return state.ErrRecovery
	}
	snap, err := c.store.Snapshot()
	if err != nil {
		return err
	}
	if snap.Pending == nil || snap.Pending.ID != id {
		return state.ErrConflict
	}
	if snap.Pending.Phase != state.AwaitingConfirmation || !time.Now().Before(snap.Pending.Deadline) || (!c.expires.IsZero() && !time.Now().Before(c.expires)) {
		return errors.New("confirmation expired or transaction not ready")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	err = c.adapter.Verify(ctx, snap.Pending.Candidate.Config)
	cancel()
	if err != nil {
		return c.rollbackWithError("confirmation verification failed")
	}
	// Verification may consume the remaining window while holding the lock.
	// The watchdog cannot run here; enforce both deadlines again before commit.
	if !time.Now().Before(snap.Pending.Deadline) || (!c.expires.IsZero() && !time.Now().Before(c.expires)) {
		return c.rollbackWithError("confirmation expired during verification")
	}
	if err = c.store.Commit(id); err != nil {
		c.recoveryErr = state.ErrRecovery
		return err
	}
	return nil
}

func (c *Controller) Rollback(id string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.recoveryErr != nil {
		return state.ErrRecovery
	}
	snap, err := c.store.Snapshot()
	if err != nil {
		return err
	}
	if snap.Pending == nil || snap.Pending.ID != id {
		return state.ErrConflict
	}
	return c.rollbackLocked()
}

func (c *Controller) rollbackWithError(message string) error {
	if err := c.rollbackLocked(); err != nil {
		return err
	}
	return errors.New(message + "; previous configuration restored and verified")
}

func (c *Controller) rollbackLocked() error {
	snap, err := c.store.Snapshot()
	if err != nil {
		return err
	}
	if snap.Pending == nil {
		return nil
	}
	p := snap.Pending
	if p.Phase != state.RollingBack {
		if err = c.store.Transition(p.ID, state.RollingBack, time.Time{}); err != nil {
			c.recoveryErr = state.ErrRecovery
			return err
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	err = c.restore(ctx, snap.Current.Config, *p)
	if err == nil {
		err = c.adapter.Verify(ctx, snap.Current.Config)
	}
	cancel()
	if err != nil {
		_ = c.store.Transition(p.ID, state.RecoveryRequired, time.Time{})
		fmt.Fprintf(os.Stderr, "safeapply: rollbackLocked underlying err=%q\n", err.Error())
		c.recoveryErr = fmt.Errorf("rollback failed; network state unknown; local recovery required: %w", err)
		return c.recoveryErr
	}
	if err = c.store.RolledBack(p.ID); err != nil {
		c.recoveryErr = state.ErrRecovery
		return err
	}
	return nil
}

func (c *Controller) restore(ctx context.Context, before json.RawMessage, p state.Transaction) error {
	if adapter, ok := c.adapter.(SnapshotAdapter); ok {
		if len(p.Recovery) == 0 {
			return errors.New("host recovery evidence missing")
		}
		return adapter.RestoreCaptured(ctx, before, p.Candidate.Config, p.Recovery)
	}
	if len(p.Recovery) > 0 {
		return errors.New("installed adapter cannot restore this host evidence")
	}
	return c.adapter.Restore(ctx, before, p.Candidate.Config)
}

func (c *Controller) watch() {
	defer close(c.done)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-c.stop:
			return
		case <-ticker.C:
			c.expire(time.Now())
		}
	}
}

func (c *Controller) expire(now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.recoveryErr != nil {
		return
	}
	snap, err := c.store.Snapshot()
	if err != nil {
		c.recoveryErr = state.ErrRecovery
		return
	}
	if snap.Pending != nil && snap.Pending.Phase == state.AwaitingConfirmation &&
		(!now.Before(snap.Pending.Deadline) || (!c.expires.IsZero() && !now.Before(c.expires))) {
		_ = c.rollbackLocked()
	}
}

// Close tries to restore any unconfirmed change before stopping the watchdog.
// A crash cannot call Close; Open's durable recovery handles that case instead.
func (c *Controller) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	err := c.rollbackLocked()
	close(c.stop)
	c.mu.Unlock()
	<-c.done
	return err
}
