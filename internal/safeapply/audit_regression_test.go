package safeapply

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

type deadlineAdapter struct {
	testAdapter
	duringVerify func()
}

func (a *deadlineAdapter) Verify(ctx context.Context, raw json.RawMessage) error {
	if a.duringVerify != nil {
		f := a.duringVerify
		a.duringVerify = nil
		f()
	}
	return a.testAdapter.Verify(ctx, raw)
}
func TestAuditConfirmRechecksDeadlineAfterVerification(t *testing.T) {
	s := testStore(t)
	a := &deadlineAdapter{testAdapter: testAdapter{live: `{"value":0}`}}
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
	// Deterministically model expiration while Verify holds the controller lock.
	a.duringVerify = func() { c.expires = time.Now().Add(-time.Second) }
	if err = c.Confirm(tx.ID); err == nil {
		t.Error("confirmation committed past monotonic deadline")
	}
	after, _ := s.Snapshot()
	if after.Pending != nil || after.Current.ID != snap.Current.ID || a.live != `{"value":0}` {
		t.Fatal("expired candidate was not restored")
	}
}
