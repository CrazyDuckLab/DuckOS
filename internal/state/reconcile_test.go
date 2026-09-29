package state

import (
	"encoding/json"
	"testing"
	"time"
)

func TestSharedRequestIdentityAndAuthenticatedReceiptReconciliation(t *testing.T) {
	engine := openTest(t, t.TempDir())
	agent := openTest(t, t.TempDir())
	before, _ := engine.Snapshot()
	const id = "0123456789abcdef0123456789abcdef"
	input := json.RawMessage(`{"value":1}`)
	e, err := engine.PrepareRequest(before.Current.ID, id, input, "owner", "test", nil)
	if err != nil {
		t.Fatal(err)
	}
	a, err := agent.PrepareRequest(before.Current.ID, id, input, "owner", "test", nil)
	if err != nil || e.ID != a.ID || e.Candidate.ID != a.Candidate.ID {
		t.Fatal("journals do not share transaction identity", err)
	}
	if err = agent.Transition(id, Applying, time.Time{}); err != nil {
		t.Fatal(err)
	}
	if err = agent.Transition(id, AwaitingConfirmation, time.Now().Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err = agent.Commit(id); err != nil {
		t.Fatal(err)
	}
	remote, _ := agent.Snapshot()
	wrong := remote.Outcomes[0]
	wrong.Before = wrong.Candidate
	if err = engine.Reconcile(wrong); err == nil {
		t.Fatal("mismatched receipt accepted")
	}
	if err = engine.Reconcile(remote.Outcomes[0]); err != nil {
		t.Fatal(err)
	}
	if err = engine.Reconcile(remote.Outcomes[0]); err != nil {
		t.Fatal("receipt replay not idempotent", err)
	}
	equivalent := remote.Outcomes[0]
	equivalent.CompletedAt = equivalent.CompletedAt.In(time.FixedZone("equivalent", 3*60*60))
	if err = engine.Reconcile(equivalent); err != nil {
		t.Fatal("equivalent timestamp rejected", err)
	}
	local, _ := engine.Snapshot()
	if local.Pending != nil || local.Current.ID != remote.Current.ID || len(local.Outcomes) != 1 {
		t.Fatal("engine did not converge")
	}
	if _, err = engine.PrepareRequest(local.Current.ID, id, json.RawMessage(`{"value":2}`), "owner", "replay", nil); err == nil {
		t.Fatal("retained transaction identity reused")
	}
}

func TestAdmissionTombstoneSurvivesRestart(t *testing.T) {
	path := t.TempDir()
	s := openTest(t, path)
	before, _ := s.Snapshot()
	const id = "0123456789abcdef0123456789abcdef"
	candidate := json.RawMessage(`{"value":1}`)
	if err := s.RejectRequest(before.Current.ID, id, candidate, "owner", "lost request"); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s = openTest(t, path)
	if _, err := s.PrepareRequest(before.Current.ID, id, candidate, "owner", "late apply", nil); err == nil {
		t.Fatal("restart lost cancellation tombstone")
	}
	after, _ := s.Snapshot()
	if after.Current.ID != before.Current.ID || after.Pending != nil || len(after.History) != 0 {
		t.Fatal("admission cancellation mutated current configuration")
	}
}

func TestRollbackReceiptNeverCommitsCandidate(t *testing.T) {
	path := t.TempDir()
	s := openTest(t, path)
	tx := prepareTest(t, s, 1)
	s.Close()
	s = openTest(t, path)
	o := Outcome{TransactionID: tx.ID, Before: tx.Before, Candidate: tx.Candidate.ID, Result: "rolled-back", CompletedAt: time.Now().UTC()}
	if err := s.Reconcile(o); err != nil {
		t.Fatal(err)
	}
	snap, _ := s.Snapshot()
	if snap.Pending != nil || snap.Current.ID != tx.Before || len(snap.History) != 0 {
		t.Fatal("rollback receipt changed committed intent")
	}
	if _, err := s.PrepareRequest(snap.Current.ID, "../../invalid", json.RawMessage(`{"value":1}`), "owner", "test", nil); err == nil {
		t.Fatal("invalid identity admitted")
	}
}
