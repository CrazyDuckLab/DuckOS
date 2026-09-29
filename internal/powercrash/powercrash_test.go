// powercrash/powercrash_test.go pins the V1
// crash-consistency contract.
package powercrash

import "testing"

func TestOperationValid(t *testing.T) {
	if !OpConfigCommit.Valid() || !OpVaultWrite.Valid() {
		t.Fatal("known ops must be valid")
	}
	if Operation("garbage").Valid() {
		t.Fatal("garbage must be invalid")
	}
}

func TestJournalEntryValid(t *testing.T) {
	good := JournalEntry{ID: "x1", Operation: OpConfigCommit, Subject: "default", Outcome: OutcomeCommitted}
	if err := good.Valid(); err != nil {
		t.Fatal(err)
	}
	bad := JournalEntry{ID: "!bad", Operation: OpConfigCommit, Subject: "default", Outcome: OutcomeCommitted}
	if err := bad.Valid(); err == nil {
		t.Fatal("non-canonical id must fail")
	}
	bad2 := JournalEntry{ID: "x1", Operation: "garbage", Subject: "default", Outcome: OutcomeCommitted}
	if err := bad2.Valid(); err == nil {
		t.Fatal("unknown op must fail")
	}
	bad3 := JournalEntry{ID: "x1", Operation: OpConfigCommit, Subject: "", Outcome: OutcomeCommitted}
	if err := bad3.Valid(); err == nil {
		t.Fatal("missing subject must fail")
	}
	bad4 := JournalEntry{ID: "x1", Operation: OpConfigCommit, Subject: "x", Outcome: "garbage"}
	if err := bad4.Valid(); err == nil {
		t.Fatal("invalid outcome must fail")
	}
}

func TestJournalBeginEnd(t *testing.T) {
	j := NewJournal()
	e, err := j.Begin(OpConfigCommit, "default")
	if err != nil {
		t.Fatal(err)
	}
	if e.Outcome != OutcomePending {
		t.Fatalf("new entry must be pending, got %s", e.Outcome)
	}
	if err := j.End(e.ID, "ok"); err != nil {
		t.Fatal(err)
	}
	// double-end must fail
	if err := j.End(e.ID, "ok"); err == nil {
		t.Fatal("double End must fail")
	}
}

func TestJournalAbort(t *testing.T) {
	j := NewJournal()
	e, err := j.Begin(OpVaultWrite, "secret-1")
	if err != nil {
		t.Fatal(err)
	}
	if err := j.Abort(e.ID, "user-cancel"); err != nil {
		t.Fatal(err)
	}
	if err := j.End(e.ID, "ok"); err == nil {
		t.Fatal("End after Abort must fail")
	}
}

func TestJournalInspectNoOp(t *testing.T) {
	j := NewJournal()
	r := j.Inspect(nil)
	if r.Reason != ReasonNone || len(r.Interrupted) != 0 || len(r.OrphanTemp) != 0 {
		t.Fatalf("clean journal must report none: %+v", r)
	}
}

func TestJournalInspectInterrupted(t *testing.T) {
	j := NewJournal()
	if _, err := j.Begin(OpConfigCommit, "default"); err != nil {
		t.Fatal(err)
	}
	r := j.Inspect(nil)
	if r.Reason != ReasonInterruptedOp {
		t.Fatalf("expected interrupted-op, got %s", r.Reason)
	}
	if len(r.Interrupted) != 1 {
		t.Fatalf("interrupted count: %d", len(r.Interrupted))
	}
}

func TestJournalInspectOrphan(t *testing.T) {
	j := NewJournal()
	r := j.Inspect([]string{"/tmp/leftover.tmp"})
	if r.Reason != ReasonOrphanedTemp {
		t.Fatalf("expected orphan, got %s", r.Reason)
	}
	if len(r.OrphanTemp) != 1 {
		t.Fatalf("orphan count: %d", len(r.OrphanTemp))
	}
}

func TestAtomicPublisherValid(t *testing.T) {
	good := AtomicPublisher{TempDir: "/tmp", Final: "/etc/duckos/active.json"}
	if err := good.Valid(); err != nil {
		t.Fatal(err)
	}
	if err := (AtomicPublisher{}).Valid(); err == nil {
		t.Fatal("empty must fail")
	}
}

func TestDeciderInterrupted(t *testing.T) {
	j := NewJournal()
	if _, err := j.Begin(OpUpdateActivate, "v2"); err != nil {
		t.Fatal(err)
	}
	r := j.Inspect(nil)
	st := Decider{}.Decide(r, "v1", "v1")
	if st.SafeToBoot {
		t.Fatal("interrupted state must not be safe-to-boot")
	}
	if st.Action == "" {
		t.Fatal("action must be set")
	}
}

func TestDeciderOrphanSafe(t *testing.T) {
	j := NewJournal()
	r := j.Inspect([]string{"/tmp/x.tmp"})
	st := Decider{}.Decide(r, "v1", "v1")
	if !st.SafeToBoot {
		t.Fatal("orphan-temp must be safe-to-boot")
	}
	if st.Action != "cleanup-temp" {
		t.Fatalf("action: %s", st.Action)
	}
}

func TestDeciderFirstBoot(t *testing.T) {
	j := NewJournal()
	r := j.Inspect(nil)
	st := Decider{}.Decide(r, "", "")
	if !st.SafeToBoot {
		t.Fatal("first boot must be safe")
	}
	if st.Action != "first-boot" {
		t.Fatalf("action: %s", st.Action)
	}
}

func TestDeciderPublishPending(t *testing.T) {
	j := NewJournal()
	r := j.Inspect(nil)
	st := Decider{}.Decide(r, "v1", "v2")
	if !st.SafeToBoot {
		t.Fatal("publish-pending must be safe")
	}
	if st.Action != "publish-pending" {
		t.Fatalf("action: %s", st.Action)
	}
}

func TestDeciderNoOp(t *testing.T) {
	j := NewJournal()
	r := j.Inspect(nil)
	st := Decider{}.Decide(r, "v1", "v1")
	if !st.SafeToBoot {
		t.Fatal("clean state must be safe")
	}
	if st.Action != "none" {
		t.Fatalf("action: %s", st.Action)
	}
}
