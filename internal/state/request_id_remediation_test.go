package state

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
	"time"
)

func TestPrepareRequestReplaysCommittedOutcomeAfterRestart(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	validate := func(raw json.RawMessage) error {
		var v map[string]int
		return json.Unmarshal(raw, &v)
	}
	store, err := Open(dir, []byte(`{"version":1}`), validate)
	if err != nil {
		t.Fatal(err)
	}
	snap, err := store.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	requestID := "0123456789abcdef0123456789abcdef"
	candidate := json.RawMessage(`{"version":2}`)
	tx, err := store.PrepareRequest(snap.Current.ID, requestID, candidate, "owner", "test", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Transition(tx.ID, Applying, time.Time{}); err != nil {
		t.Fatal(err)
	}
	if err := store.Transition(tx.ID, AwaitingConfirmation, time.Now().Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := store.Commit(tx.ID); err != nil {
		t.Fatal(err)
	}
	original, err := store.PrepareRequest(snap.Current.ID, requestID, candidate, "owner", "test", nil)
	if err != nil {
		t.Fatal(err)
	}
	if original.Phase != Committed || original.CompletedAt.IsZero() || !original.Candidate.CreatedAt.Equal(tx.Candidate.CreatedAt) || original.Candidate.Actor != "owner" {
		t.Fatalf("noncanonical terminal replay: %+v", original)
	}
	store.Close()

	reopened, err := Open(dir, []byte(`{"version":1}`), validate)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	replay, err := reopened.PrepareRequest(snap.Current.ID, requestID, candidate, "owner", "test", nil)
	if err != nil {
		t.Fatalf("exact committed replay returned error: %v", err)
	}
	if replay.ID != requestID || replay.Candidate.ID != tx.Candidate.ID {
		t.Fatalf("unexpected replay transaction: %#v", replay)
	}
	if !reflect.DeepEqual(original, replay) {
		t.Fatal("restart changed canonical replay")
	}
	for _, args := range []struct {
		expected, actor, reason string
		window                  int
	}{
		{"different-revision", "owner", "test", 0},
		{snap.Current.ID, "different-owner", "test", 0},
		{snap.Current.ID, "owner", "different-reason", 0},
		{snap.Current.ID, "owner", "test", 120},
	} {
		if _, err := reopened.PrepareRequestWithWindow(args.expected, requestID, candidate, args.actor, args.reason, nil, args.window); err != ErrConflict {
			t.Fatalf("changed request accepted: %+v err=%v", args, err)
		}
	}
	if _, err := reopened.PrepareRequest(snap.Current.ID, requestID, json.RawMessage(`{"version":3}`), "owner", "test", nil); err != ErrConflict {
		t.Fatalf("changed payload with same request id got %v, want ErrConflict", err)
	}
}
