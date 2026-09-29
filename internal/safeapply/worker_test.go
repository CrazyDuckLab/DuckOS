package safeapply

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

type blockedAdapter struct {
	testAdapter
	started chan struct{}
	release chan struct{}
}

func TestWorkerFailureAndRecoveryEvidence(t *testing.T) {
	for _, restoreFails := range []bool{false, true} {
		t.Run(map[bool]string{false: "restored", true: "recovery-required"}[restoreFails], func(t *testing.T) {
			s := testStore(t)
			a := &capturingAdapter{testAdapter: testAdapter{live: `{"value":0}`, failApply: true, failRestore: restoreFails}}
			c, err := Open(s, a)
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			w := NewWorker(c)
			snap, _ := s.Snapshot()
			if _, err = w.Submit(snap.Current.ID, json.RawMessage(`{"value":1}`), "owner", "test", 2*time.Minute); err != nil {
				t.Fatal(err)
			}
			w.Close()
			job, public, err := w.Status()
			if err != nil || job.Error == "" {
				t.Fatal("failure not reported", err)
			}
			if restoreFails {
				if job.Phase != "recovery-required" || public.Pending == nil || len(public.Pending.Recovery) != 0 {
					t.Fatal("recovery state missing or private evidence exposed")
				}
			} else if job.Phase != "failed" || public.Pending != nil {
				t.Fatal("restored failure reported incorrectly")
			}
			encoded, _ := json.Marshal(struct {
				Job   Job
				State any
			}{job, public})
			if strings.Contains(string(encoded), "privateEvidence") {
				t.Fatal("private evidence leaked")
			}
			if _, err = w.Submit(snap.Current.ID, json.RawMessage(`{"value":2}`), "owner", "test", 2*time.Minute); err == nil {
				t.Fatal("closed worker accepted work")
			}
		})
	}
}

func TestWorkerLaterCommitDoesNotRewriteEarlierOutcome(t *testing.T) {
	s := testStore(t)
	c, err := Open(s, &testAdapter{live: `{"value":0}`})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	w := NewWorker(c)
	snap, _ := s.Snapshot()
	candidate := json.RawMessage(`{"value":1}`)
	if _, err = w.Submit(snap.Current.ID, candidate, "owner", "first attempt", 2*time.Minute); err != nil {
		t.Fatal(err)
	}
	w.Close()
	_, pending, err := w.Status()
	if err != nil || pending.Pending == nil {
		t.Fatal("missing pending transaction", err)
	}
	if err = c.Rollback(pending.Pending.ID); err != nil {
		t.Fatal(err)
	}
	later, err := c.Apply(snap.Current.ID, candidate, "owner", "second attempt", 2*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err = c.Confirm(later.ID); err != nil {
		t.Fatal(err)
	}
	job, _, err := w.Status()
	if err != nil || job.Phase != "rolled-back" {
		t.Fatal("inferred outcome from equal revision digest", job, err)
	}
}

func (a *blockedAdapter) Apply(ctx context.Context, before, next json.RawMessage) error {
	close(a.started)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-a.release:
		return a.testAdapter.Apply(ctx, before, next)
	}
}
func TestWorkerAdmissionAndStatusWhileApplying(t *testing.T) {
	s := testStore(t)
	a := &blockedAdapter{testAdapter: testAdapter{live: `{"value":0}`}, started: make(chan struct{}), release: make(chan struct{})}
	c, err := Open(s, a)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	w := NewWorker(c)
	defer w.Close()
	defer func() {
		select {
		case <-a.release:
		default:
			close(a.release)
		}
	}()
	snap, _ := s.Snapshot()
	request, err := w.Submit(snap.Current.ID, json.RawMessage(`{"value":1}`), "owner", "test", 2*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-a.started:
	case <-time.After(time.Second):
		t.Fatal("worker did not start")
	}
	job, pending, err := w.Status()
	if err != nil || job.RequestID != request.RequestID || job.Phase != "applying" || pending.Pending == nil {
		t.Fatal("status unavailable during apply")
	}
	if _, err = w.Submit(snap.Current.ID, json.RawMessage(`{"value":2}`), "owner", "test", 2*time.Minute); err == nil {
		t.Fatal("concurrent apply admitted")
	}
	close(a.release)
	w.Close()
	job, pending, err = w.Status()
	if err != nil || job.Phase != "awaiting-confirmation" {
		t.Fatal("worker outcome missing")
	}
	if err = c.Confirm(pending.Pending.ID); err != nil {
		t.Fatal(err)
	}
	job, _, err = w.Status()
	if err != nil || job.Phase != "committed" {
		t.Fatal("confirmed transaction still pending")
	}
}
