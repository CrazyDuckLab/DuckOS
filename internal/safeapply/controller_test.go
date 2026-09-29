package safeapply

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/state"
)

type testAdapter struct {
	live                               string
	failApply, failRestore, failVerify bool
	restores                           int
}

func (a *testAdapter) Preflight(context.Context, json.RawMessage, json.RawMessage) error { return nil }
func (a *testAdapter) Apply(_ context.Context, _, next json.RawMessage) error {
	a.live = string(next)
	if a.failApply {
		return errors.New("synthetic partial failure")
	}
	return nil
}
func (a *testAdapter) Verify(_ context.Context, want json.RawMessage) error {
	if a.failVerify || a.live != string(want) {
		return errors.New("synthetic verification failure")
	}
	return nil
}
func (a *testAdapter) Restore(_ context.Context, before, _ json.RawMessage) error {
	a.restores++
	if a.failRestore {
		return errors.New("synthetic restore failure")
	}
	a.live = string(before)
	return nil
}
func testStore(t *testing.T) *state.Store {
	t.Helper()
	path := t.TempDir()
	if err := os.Chmod(path, 0700); err != nil {
		t.Fatal(err)
	}
	s, err := state.Open(path, json.RawMessage(`{"value":0}`), func(b json.RawMessage) error {
		if !json.Valid(b) {
			return errors.New("invalid JSON")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}
func TestConfirmCommitsOnlyVerifiedCandidate(t *testing.T) {
	s := testStore(t)
	a := &testAdapter{live: `{"value":0}`}
	c, err := Open(s, a)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	snap, _ := s.Snapshot()
	tx, err := c.Apply(snap.Current.ID, json.RawMessage(`{"value":1}`), "owner", "test", 2*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	snap, _ = s.Snapshot()
	if string(snap.Current.Config) != `{"value":0}` || tx.Phase != state.AwaitingConfirmation {
		t.Fatal("committed before confirmation")
	}
	if err = c.Confirm("wrong"); err == nil {
		t.Fatal("wrong confirmation admitted")
	}
	if err = c.Confirm(tx.ID); err != nil {
		t.Fatal(err)
	}
	snap, _ = s.Snapshot()
	if string(snap.Current.Config) != `{"value":1}` || snap.Pending != nil {
		t.Fatal("candidate not committed")
	}
}
func TestPartialFailureRestoresBeforeState(t *testing.T) {
	s := testStore(t)
	a := &testAdapter{live: `{"value":0}`, failApply: true}
	c, err := Open(s, a)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	snap, _ := s.Snapshot()
	if _, err = c.Apply(snap.Current.ID, json.RawMessage(`{"value":1}`), "owner", "test", 2*time.Minute); err == nil {
		t.Fatal("failure hidden")
	}
	snap, _ = s.Snapshot()
	if a.live != `{"value":0}` || a.restores != 1 || snap.Pending != nil {
		t.Fatal("partial operation not rolled back")
	}
}
func TestStartupRestoresInterruptedApply(t *testing.T) {
	s := testStore(t)
	snap, _ := s.Snapshot()
	tx, err := s.Prepare(snap.Current.ID, json.RawMessage(`{"value":1}`), "owner", "test")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Transition(tx.ID, state.Applying, time.Time{}); err != nil {
		t.Fatal(err)
	}
	a := &testAdapter{live: `{"value":1}`}
	c, err := Open(s, a)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	snap, _ = s.Snapshot()
	if a.restores != 1 || a.live != `{"value":0}` || snap.Pending != nil {
		t.Fatal("restart adopted unconfirmed configuration")
	}
}
func TestRollbackFailureBlocksFurtherMutations(t *testing.T) {
	s := testStore(t)
	a := &testAdapter{live: `{"value":0}`, failApply: true, failRestore: true}
	c, err := Open(s, a)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	snap, _ := s.Snapshot()
	if _, err = c.Apply(snap.Current.ID, json.RawMessage(`{"value":1}`), "owner", "test", 2*time.Minute); err == nil {
		t.Fatal("rollback failure hidden")
	}
	snap, _ = s.Snapshot()
	if snap.Pending == nil || snap.Pending.Phase != state.RecoveryRequired {
		t.Fatal("lost recovery journal")
	}
	if _, err = c.Apply(snap.Current.ID, json.RawMessage(`{"value":2}`), "owner", "test", 2*time.Minute); !errors.Is(err, state.ErrRecovery) {
		t.Fatal(err)
	}
}

func TestStartupRecoveryRequiredNeverFabricatesRollback(t *testing.T) {
	for _, fail := range []bool{true, false} {
		t.Run(map[bool]string{true: "failure-preserves-journal", false: "verified-recovery"}[fail], func(t *testing.T) {
			s := testStore(t)
			initial, _ := s.Snapshot()
			tx, err := s.Prepare(initial.Current.ID, json.RawMessage(`{"value":1}`), "owner", "interrupted rollback")
			if err != nil {
				t.Fatal(err)
			}
			for _, phase := range []state.Phase{state.Applying, state.RollingBack, state.RecoveryRequired} {
				if err := s.Transition(tx.ID, phase, time.Time{}); err != nil {
					t.Fatal(err)
				}
			}
			a := &testAdapter{live: `{"value":1}`, failRestore: fail}
			c, err := Open(s, a)
			if c != nil {
				defer c.Close()
			}
			snap, _ := s.Snapshot()
			if a.restores != 1 {
				t.Fatal("startup did not attempt real undo")
			}
			if fail {
				if err == nil || snap.Pending == nil || snap.Pending.Phase != state.RecoveryRequired || len(snap.Outcomes) != 0 || a.live != `{"value":1}` {
					t.Fatal("failed recovery was cleared", err, snap)
				}
			} else if err != nil || snap.Pending != nil || a.live != `{"value":0}` || len(snap.Outcomes) != 1 || snap.Outcomes[0].Result != "rolled-back" {
				t.Fatal("verified recovery not recorded", err, snap)
			}
		})
	}
}
func TestCloseRollsBackUnconfirmedChange(t *testing.T) {
	s := testStore(t)
	a := &testAdapter{live: `{"value":0}`}
	c, err := Open(s, a)
	if err != nil {
		t.Fatal(err)
	}
	snap, _ := s.Snapshot()
	if _, err = c.Apply(snap.Current.ID, json.RawMessage(`{"value":1}`), "owner", "test", 2*time.Minute); err != nil {
		t.Fatal(err)
	}
	if err = c.Close(); err != nil {
		t.Fatal(err)
	}
	if a.live != `{"value":0}` || a.restores != 1 {
		t.Fatal("shutdown left candidate live")
	}
}

func TestWatchdogExpiryRestoresWithoutBrowser(t *testing.T) {
	s := testStore(t)
	a := &testAdapter{live: `{"value":0}`}
	c, err := Open(s, a)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	snap, _ := s.Snapshot()
	tx, err := c.Apply(snap.Current.ID, json.RawMessage(`{"value":1}`), "owner", "test", 2*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	c.expire(tx.Deadline.Add(-time.Second))
	snap, _ = c.Status()
	if snap.Pending == nil {
		t.Fatal("rolled back before deadline")
	}
	c.expire(tx.Deadline.Add(time.Second))
	snap, _ = c.Status()
	if snap.Pending != nil || string(snap.Current.Config) != `{"value":0}` {
		t.Fatal("watchdog did not restore")
	}
	if err = c.Confirm(tx.ID); err == nil {
		t.Fatal("late confirmation succeeded")
	}
}

type capturingAdapter struct {
	testAdapter
	captured      string
	restored      string
	captureFailed bool
}

func (a *capturingAdapter) Capture(context.Context, json.RawMessage, json.RawMessage) (json.RawMessage, error) {
	if a.captureFailed {
		return nil, errors.New("synthetic capture failure")
	}
	a.captured = `{"version":1,"privateEvidence":"synthetic host before-state"}`
	return json.RawMessage(a.captured), nil
}
func (a *capturingAdapter) RestoreCaptured(ctx context.Context, before, next, evidence json.RawMessage) error {
	a.restored = string(evidence)
	return a.testAdapter.Restore(ctx, before, next)
}
func TestPrivateHostEvidenceDurableButNotPublic(t *testing.T) {
	s := testStore(t)
	a := &capturingAdapter{testAdapter: testAdapter{live: `{"value":0}`}}
	c, err := Open(s, a)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	snap, _ := s.Snapshot()
	tx, err := c.Apply(snap.Current.ID, json.RawMessage(`{"value":1}`), "owner", "test", 2*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if len(tx.Recovery) > 0 {
		t.Fatal("apply leaked host recovery evidence")
	}
	public, err := c.Status()
	if err != nil || len(public.Pending.Recovery) > 0 {
		t.Fatal("status leaked host recovery evidence")
	}
	private, _ := s.Snapshot()
	if string(private.Pending.Recovery) != a.captured {
		t.Fatal("recovery evidence not journaled")
	}
	if err = c.Rollback(tx.ID); err != nil {
		t.Fatal(err)
	}
	if a.restored != a.captured {
		t.Fatal("rollback lost host before-state")
	}
}
func TestHostCaptureFailurePrecedesMutation(t *testing.T) {
	s := testStore(t)
	a := &capturingAdapter{testAdapter: testAdapter{live: `{"value":0}`}, captureFailed: true}
	c, err := Open(s, a)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	snap, _ := s.Snapshot()
	if _, err = c.Apply(snap.Current.ID, json.RawMessage(`{"value":1}`), "owner", "test", 2*time.Minute); err == nil {
		t.Fatal("capture failure ignored")
	}
	snap, _ = s.Snapshot()
	if snap.Pending != nil || a.live != `{"value":0}` {
		t.Fatal("mutation preceded durable recovery evidence")
	}
}
func TestStartupUsesCapturedHostEvidence(t *testing.T) {
	s := testStore(t)
	snap, _ := s.Snapshot()
	evidence := json.RawMessage(`{"version":1,"privateEvidence":"synthetic interrupted baseline"}`)
	tx, err := s.PrepareWithRecovery(snap.Current.ID, json.RawMessage(`{"value":1}`), "owner", "test", evidence)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Transition(tx.ID, state.Applying, time.Time{}); err != nil {
		t.Fatal(err)
	}
	a := &capturingAdapter{testAdapter: testAdapter{live: `{"value":1}`}}
	c, err := Open(s, a)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if a.restored != string(evidence) || a.live != `{"value":0}` {
		t.Fatal("startup did not restore captured host state")
	}
}
