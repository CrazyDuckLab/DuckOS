// Package subscription / durability_test.go: UQ-01 companion
// suite — subscription token revocation and rotation durability,
// including the real on-disk persist.File adapter.
package subscription

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/persist"
)

// E: subscription token revoked → restart (store reconstructed from
// persisted records) → old URL rejected.
func TestUQ01_E_SubscriptionRevocationSurvivesRestart(t *testing.T) {
	dir := t.TempDir()
	tokenFile, err := persist.Open[[]TokenRecord](dir, "tokens.json")
	if err != nil {
		t.Fatal(err)
	}
	store := NewTokenStore()
	store.SetPersist(func(recs []TokenRecord) (persist.CommitOutcome, error) {
		out, err := tokenFile.Update(func(cur *[]TokenRecord) { *cur = recs })
		return out, err
	})
	tok, _, _ := store.Issue("g1", time.Hour)
	if n, err := store.RevokeSubscription("g1"); err != nil || n != 1 {
		t.Fatalf("revoke: n=%d err=%v", n, err)
	}
	// Restart: reconstruct from the durable file.
	store2 := NewTokenStore()
	store2.LoadRecords(tokenFile.Get())
	if _, ok := store2.Lookup(tok); ok {
		t.Fatal("UQ-01: revoked subscription URL resurrected after restart")
	}
}

// F: storage failure during revocation → no false success, old
// tokens stay active, and a later retry commits. (The real
// filesystem failure modes of the atomic adapter are covered by
// internal/persist: partial-write rollback and rename failure.)
func TestUQ01_F_SubscriptionRevokeFailureNotAcknowledged(t *testing.T) {
	// The token is issued against a working hook, then the hook is
	// armed with a NOT_COMMITTED fault for the revocation. (Since
	// AUD-02, an Issue itself never succeeds against a failing
	// hook — no usable token is acknowledged.)
	failing := NewTokenStore()
	failing.SetPersist(func([]TokenRecord) (persist.CommitOutcome, error) {
		return persist.CommitOK, nil
	})
	t2, _, err := failing.Issue("g1", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	failing.SetPersist(func([]TokenRecord) (persist.CommitOutcome, error) {
		return persist.CommitNotCommitted, errENOSPC
	})
	if _, err := failing.RevokeSubscription("g1"); err == nil || !strings.Contains(err.Error(), "ENOSPC") {
		t.Fatalf("revocation acknowledged despite failed commit: %v", err)
	}
	// In-memory state matches the durable truth: the token is
	// still active, exactly as the last committed snapshot says.
	if _, ok := failing.Lookup(t2); !ok {
		t.Fatal("memory claims revoked while durable state is active")
	}
	// Retry with a working hook commits and takes effect.
	committed := false
	failing.SetPersist(func(recs []TokenRecord) (persist.CommitOutcome, error) {
		committed = true
		return persist.CommitOK, nil
	})
	if _, err := failing.RevokeSubscription("g1"); err != nil || !committed {
		t.Fatalf("retry after fault: err=%v committed=%v", err, committed)
	}
	if _, ok := failing.Lookup(t2); ok {
		t.Fatal("revocation ineffective after committed retry")
	}
}

// Rotation durability: rotated-out URLs die durably.
func TestUQ01_RotationSurvivesRestart(t *testing.T) {
	dir := t.TempDir()
	tokenFile, err := persist.Open[[]TokenRecord](dir, "tokens.json")
	if err != nil {
		t.Fatal(err)
	}
	store := NewTokenStore()
	store.SetPersist(func(recs []TokenRecord) (persist.CommitOutcome, error) {
		out, err := tokenFile.Update(func(cur *[]TokenRecord) { *cur = recs })
		return out, err
	})
	oldTok, _, _ := store.Issue("g1", time.Hour)
	newTok, _, err := store.Rotate("g1", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	store2 := NewTokenStore()
	store2.LoadRecords(tokenFile.Get())
	if _, ok := store2.Lookup(oldTok); ok {
		t.Fatal("rotated-out URL resurrected after restart")
	}
	if _, ok := store2.Lookup(newTok); !ok {
		t.Fatal("rotated-in URL lost after restart")
	}
}

var errENOSPC = errors.New("injected ENOSPC: no space left on device")

// UQ-02: post-rename uncertainty during subscription revocation —
// the revoked state is KEPT, the store degrades, mutations block,
// and reconciliation from disk resolves; a subsequent unrelated
// mutation cannot resurrect revoked URLs.
func TestUQ02_SubscriptionUncertainRevocation(t *testing.T) {
	store := NewTokenStore()
	var failures int
	store.SetPersist(func(recs []TokenRecord) (persist.CommitOutcome, error) {
		if failures > 0 {
			failures--
			return persist.CommitUncertain, errENOSPC
		}
		return persist.CommitOK, nil
	})
	tok, _, _ := store.Issue("g1", time.Hour)
	failures = 1
	if _, err := store.RevokeSubscription("g1"); err == nil || !strings.Contains(err.Error(), "UNCERTAIN") {
		t.Fatalf("uncertain revocation not reported: %v", err)
	}
	// Restrictive state retained: the URL stays dead.
	if _, ok := store.Lookup(tok); ok {
		t.Fatal("revoked URL resurrected in memory after uncertain commit")
	}
	// Mutations are blocked while degraded.
	if _, _, err := store.Issue("g1", time.Hour); err == nil || !strings.Contains(err.Error(), "degraded") {
		t.Fatalf("mutation during degradation allowed: %v", err)
	}
	// Reconciliation adopts the disk snapshot; revoked stays revoked.
	res := store.Reconcile([]TokenRecord{{TokenHash: hashTokenForTest(tok), SubID: "g1", Revoked: true}})
	if !strings.Contains(res, "recovered") {
		t.Fatalf("reconcile: %s", res)
	}
	if _, ok := store.Lookup(tok); ok {
		t.Fatal("revoked URL resurrected after reconciliation")
	}
	if store.Degraded() != "" {
		t.Fatal("degraded flag not cleared")
	}
}

func hashTokenForTest(token string) string { return hashToken(token) }
