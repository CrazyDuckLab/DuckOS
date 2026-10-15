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
	store.SetPersist(func(recs []TokenRecord) error {
		return tokenFile.Update(func(cur *[]TokenRecord) { *cur = recs })
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
	// Injected fault via hook:
	failing := NewTokenStore()
	failing.SetPersist(func([]TokenRecord) error { return errENOSPC })
	t2, _, _ := failing.Issue("g1", time.Hour)
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
	failing.SetPersist(func(recs []TokenRecord) error { committed = true; return nil })
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
	store.SetPersist(func(recs []TokenRecord) error {
		return tokenFile.Update(func(cur *[]TokenRecord) { *cur = recs })
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
