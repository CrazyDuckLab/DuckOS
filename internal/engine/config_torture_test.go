package engine

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/CrazyDuckLab/DuckOS/internal/state"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

func freshConfig(t *testing.T) *Config {
	t.Helper()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	s, err := state.Open(dir, []byte(`{"secret":"synthetic-private-config"}`), func(json.RawMessage) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	c, err := (&Engine{}).EnableConfig(s)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	return c
}

// Journal-only operations must not advertise or record Linux success.
func TestAuditConfigWithoutCoordinatorFailsClosed(t *testing.T) {
	c := freshConfig(t)
	ctx := context.Background()
	before, err := c.store.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.Prepare(ctx, ConfigApplyRequest{Expected: before.Current.ID, Candidate: json.RawMessage(`{"v":2}`), Actor: "test", Reason: "test", Window: 300})
	if !errors.Is(err, ErrConfigRuntimeUnavailable) {
		t.Fatal("prepare:", err)
	}
	if _, err = c.Confirm(ctx, strings.Repeat("a", 32)); !errors.Is(err, ErrConfigRuntimeUnavailable) {
		t.Fatal("confirm:", err)
	}
	if _, err = c.Rollback(ctx, strings.Repeat("a", 32), "test"); !errors.Is(err, ErrConfigRuntimeUnavailable) {
		t.Fatal("rollback:", err)
	}
	if err = c.Reconcile(ctx, state.Outcome{}); !errors.Is(err, ErrConfigRuntimeUnavailable) {
		t.Fatal("reconcile:", err)
	}
	if _, err = c.Validate(ctx, json.RawMessage(`{"bad":true}`), before.Current.ID); !errors.Is(err, ErrConfigRuntimeUnavailable) {
		t.Fatal("validation:", err)
	}
	if _, err = c.Diff(ctx, ConfigDiffRequest{Candidate: json.RawMessage(`{"bad":true}`)}); !errors.Is(err, ErrConfigRuntimeUnavailable) {
		t.Fatal("diff:", err)
	}
	after, _ := c.store.Snapshot()
	if !reflect.DeepEqual(before, after) {
		t.Fatal("unavailable operation changed durable state")
	}
}
func TestAuditConfigStatusExcludesPayloadAndUnsupportedActions(t *testing.T) {
	c := freshConfig(t)
	before, _ := c.store.Snapshot()
	tx, err := c.store.Prepare(before.Current.ID, json.RawMessage(`{"secret":"synthetic-candidate-secret"}`), "synthetic-actor-secret", "synthetic-reason-secret")
	if err != nil {
		t.Fatal(err)
	}
	status, err := c.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "synthetic-") {
		t.Fatal("raw configuration or untrusted metadata leaked")
	}
	if status.Pending == nil || status.Pending.ID != tx.ID || len(status.Pending.SupportedActions) != 0 {
		t.Fatal("unsupported actions advertised")
	}
}
func TestG01_DiffNoCandidateAndStaleRevision(t *testing.T) {
	c := freshConfig(t)
	s, _ := c.store.Snapshot()
	diff, err := c.Diff(context.Background(), ConfigDiffRequest{Expected: s.Current.ID})
	if err != nil || diff.BaseRevision != s.Current.ID {
		t.Fatal("current revision read failed")
	}
	if _, err = c.Diff(context.Background(), ConfigDiffRequest{Expected: "stale"}); !errors.Is(err, state.ErrConflict) {
		t.Fatal("stale revision accepted")
	}
}

// This is a Store receipt-format regression, not proof of Linux reconciliation.
func TestG01_M25_ReconcileAcceptsRevisionIDs(t *testing.T) {
	c := freshConfig(t)
	s, _ := c.store.Snapshot()
	tx, err := c.store.Prepare(s.Current.ID, json.RawMessage(`{"v":2}`), "test", "test")
	if err != nil {
		t.Fatal(err)
	}
	err = c.store.Reconcile(state.Outcome{TransactionID: tx.ID, Before: tx.Before, Candidate: tx.Candidate.ID, Result: "rolled-back", CompletedAt: time.Now().UTC()})
	if err != nil {
		t.Fatal(err)
	}
}
