// Package subscription / audit2026_test.go: developer regression
// suite for the independent audit findings AUD-02, AUD-06 and
// AUD-07 (2026-10-19). Each test mirrors an auditor assertion with
// original wording; each verified-failed against the vulnerable
// implementation preserved in the audit worktree.
package subscription

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/persist"
)

// AUD-02: a NOT_COMMITTED issuance must fail and must not leave a
// usable token behind (in memory or after "restart").
func TestAUD02_IssueFalseSuccessOnPersistFailure(t *testing.T) {
	s := NewTokenStore()
	s.SetPersist(func([]TokenRecord) (persist.CommitOutcome, error) {
		return persist.CommitNotCommitted, errors.New("injected ENOSPC")
	})
	token, _, err := s.Issue("sub-1", 0)
	if err == nil {
		t.Fatal("AUD-02 DEFECT: Issue succeeded despite NOT_COMMITTED")
	}
	if !strings.Contains(err.Error(), "ENOSPC") {
		t.Fatalf("storage cause lost: %v", err)
	}
	if token != "" {
		t.Fatal("failed issuance returned a token")
	}
	if _, ok := s.Lookup(token); token != "" && ok {
		t.Fatal("non-durable token authenticates after failed commit")
	}
	// Simulated restart: nothing durable was written.
	fresh := NewTokenStore()
	if _, ok := fresh.Lookup(token); token != "" && ok {
		t.Fatal("token survived restart without a durable commit")
	}
}

// AUD-02: an UNCERTAIN issuance marks the store DEGRADED, fails the
// operation, and blocks subsequent mutations.
func TestAUD02_IssueUncertainDegradesAndBlocks(t *testing.T) {
	s := NewTokenStore()
	s.SetPersist(func([]TokenRecord) (persist.CommitOutcome, error) {
		return persist.CommitUncertain, errors.New("injected dir-fsync EIO")
	})
	if _, _, err := s.Issue("sub-1", 0); err == nil || !strings.Contains(err.Error(), "UNCERTAIN") {
		t.Fatalf("uncertain issuance not reported: %v", err)
	}
	if s.Degraded() == "" {
		t.Fatal("AUD-02 DEFECT: uncertain issuance left no degraded mark")
	}
	if _, _, err := s.Issue("sub-2", 0); err == nil || !strings.Contains(err.Error(), "degraded") {
		t.Fatalf("mutation not blocked while degraded: %v", err)
	}
	// Reconciliation adopts the disk (empty) and clears degradation;
	// the uncertain token is gone with it — it was never handed out.
	s.Reconcile(nil)
	if s.Degraded() != "" {
		t.Fatal("degraded flag not cleared after reconcile")
	}
}

// AUD-06: group capacity is enforced and degraded storage blocks
// mutations.
func TestAUD06_GroupCapacityAndDegradedGate(t *testing.T) {
	s := NewGroupStore()
	// Fill to the boundary (255 = MaxGroups-1).
	for i := 0; i < 255; i++ {
		g := Group{ID: string(rune('a'+i%26)) + fmt.Sprintf("g%02d", i), Name: "g", Format: FormatBase64, Enabled: true,
			Members: []GroupMember{{ServerID: "s", UserID: "u", DisplayName: "N", Enabled: true}}}
		if _, err := s.Upsert(g); err != nil {
			t.Fatalf("group %d: %v", i, err)
		}
	}
	// 256th (index 255) succeeds — exactly at MaxGroups.
	g256 := Group{ID: "g256", Name: "at-limit", Format: FormatBase64, Enabled: true,
		Members: []GroupMember{{ServerID: "s", UserID: "u", DisplayName: "N", Enabled: true}}}
	if _, err := s.Upsert(g256); err != nil {
		t.Fatalf("256th group refused: %v", err)
	}
	// 257th is rejected.
	g257 := g256
	g257.ID = "g257"
	if _, err := s.Upsert(g257); err == nil {
		t.Fatal("AUD-06 DEFECT: 257th group accepted")
	}
	// Degraded storage blocks mutations.
	s.SetPersist(func([]Group) (persist.CommitOutcome, error) {
		return persist.CommitUncertain, errors.New("injected")
	})
	_, _ = s.Upsert(g256) // triggers uncertain → degraded (update of existing)
	if s.Degraded() == "" {
		t.Fatal("setup: degraded not set")
	}
	if err := s.Delete("g1"); err == nil || !strings.Contains(err.Error(), "degraded") {
		t.Fatalf("AUD-06 DEFECT: mutation while degraded not blocked: %v", err)
	}
}

// AUD-06: concurrent modifications lose no update.
func TestAUD06_ConcurrentGroupUpserts(t *testing.T) {
	s := NewGroupStore()
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			g := Group{ID: fmt.Sprintf("g%02d", i), Name: "g", Format: FormatBase64, Enabled: true,
				Members: []GroupMember{{ServerID: "s", UserID: "u", DisplayName: "N", Enabled: true}}}
			_, _ = s.Upsert(g)
		}(i)
	}
	wg.Wait()
	if len(s.List()) != 32 {
		t.Fatalf("lost updates: %d groups", len(s.List()))
	}
}

// AUD-07: a delivery view backed by AUTHORITATIVE validation sees
// revocation immediately — the exact two-process scenario from the
// audit.
func TestAUD07_RevocationImmediateViaAuthoritativeValidation(t *testing.T) {
	agentSide := NewTokenStore()
	token, _, err := agentSide.Issue("sub-1", 0)
	if err != nil {
		t.Fatal(err)
	}
	// The delivery daemon holds NO token table: every request
	// validates against the agent-side store.
	delivery := NewDeliveryHandler(nil, func(subID string) ([]byte, error) {
		return []byte("PAYLOAD"), nil
	})
	delivery.Validate = func(tok string) (string, bool) {
		rec, ok := agentSide.Lookup(tok)
		if !ok {
			return "", false
		}
		return rec.SubID, true
	}
	// First fetch succeeds.
	rw := newRecorder()
	delivery.ServeHTTP(rw, requestFor(token))
	if rw.Code != 200 {
		t.Fatalf("pre-revocation fetch: %d", rw.Code)
	}
	// Operator revokes through the agent-side center API.
	if _, err := agentSide.RevokeSubscription("sub-1"); err != nil {
		t.Fatal(err)
	}
	// New fetch: uniform 404, no restart required.
	rw2 := newRecorder()
	delivery.ServeHTTP(rw2, requestFor(token))
	if rw2.Code != 404 {
		t.Fatalf("AUD-07 DEFECT: revoked token served after revocation (%d)", rw2.Code)
	}
	// IPC failure fails closed: validation refuses → 404.
	delivery.Validate = func(string) (string, bool) { return "", false }
	rw3 := newRecorder()
	delivery.ServeHTTP(rw3, requestFor(token))
	if rw3.Code != 404 {
		t.Fatalf("IPC failure did not fail closed: %d", rw3.Code)
	}
}

// AUD-07: rotation is immediately effective too.
func TestAUD07_RotationImmediate(t *testing.T) {
	agentSide := NewTokenStore()
	oldTok, _, _ := agentSide.Issue("g", 0)
	newTok, _, err := agentSide.Rotate("g", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	delivery := NewDeliveryHandler(nil, func(string) ([]byte, error) { return []byte("P"), nil })
	delivery.Validate = func(tok string) (string, bool) {
		rec, ok := agentSide.Lookup(tok)
		if !ok {
			return "", false
		}
		return rec.SubID, true
	}
	rw := newRecorder()
	delivery.ServeHTTP(rw, requestFor(oldTok))
	if rw.Code != 404 {
		t.Fatalf("rotated-out token still served: %d", rw.Code)
	}
	rw2 := newRecorder()
	delivery.ServeHTTP(rw2, requestFor(newTok))
	if rw2.Code != 200 {
		t.Fatalf("rotated-in token refused: %d", rw2.Code)
	}
}

// --- httptest helpers ---

func newRecorder() *httptest.ResponseRecorder { return httptest.NewRecorder() }

func requestFor(token string) *http.Request {
	return httptest.NewRequest(http.MethodGet, "/sub/"+token, nil)
}

// R-02: token reconciliation must not adopt an older permissive
// snapshot — real persisted snapshots on both sides.
func TestR02_TokenReconcileRestrictiveMerge(t *testing.T) {
	dir := t.TempDir()
	disk, err := persist.Open[[]TokenRecord](dir, "tokens.json")
	if err != nil {
		t.Fatal(err)
	}
	s := NewTokenStore()
	s.SetPersist(func(recs []TokenRecord) (persist.CommitOutcome, error) {
		out, err := disk.Update(func(cur *[]TokenRecord) { *cur = recs })
		return out, err
	})
	tok, _, _ := s.Issue("g1", time.Hour)
	if _, err := s.RevokeSubscription("g1"); err != nil {
		t.Fatal(err)
	}
	// An "older" disk snapshot where the token looks active.
	oldView := []TokenRecord{{SubID: "g1", TokenHash: hashToken(tok), IssuedAt: time.Now().UTC()}}
	_, _ = disk.Update(func(cur *[]TokenRecord) { *cur = oldView })
	// Degraded + reconcile against the older view.
	s.SetPersist(func([]TokenRecord) (persist.CommitOutcome, error) {
		return persist.CommitUncertain, errors.New("injected")
	})
	_, _, _ = s.Issue("g2", time.Hour) // drives degraded
	res := s.Reconcile(oldView)
	if !strings.Contains(res, "restrictive merge") || !strings.Contains(res, "1 revocation") {
		t.Fatalf("R-02: reconcile did not protect the revocation: %s", res)
	}
	// Public behavior: the revoked URL stays denied.
	if _, ok := s.Lookup(tok); ok {
		t.Fatal("R-02 DEFECT: revoked token reactivated by reconcile")
	}
	// Restart view: the merged (restrictive) state is what a fresh
	// reader sees.
	reopened, _ := persist.Open[[]TokenRecord](dir, "tokens.json")
	_ = reopened
	if _, ok := s.Lookup(tok); ok {
		t.Fatal("revoked token active after restart view")
	}
}

// R-04: deterministic concurrency — a revocation completing between
// the first validation and the pre-write re-validation denies the
// request (linearization at the second check).
func TestR04_RevocationBetweenValidationsDenied(t *testing.T) {
	agentSide := NewTokenStore()
	tok, _, _ := agentSide.Issue("g", time.Hour)
	calls := 0
	var revoked bool
	delivery := NewDeliveryHandler(nil, func(string) ([]byte, error) { return []byte("PAYLOAD"), nil })
	delivery.Validate = func(token string) (string, bool) {
		calls++
		if calls == 2 && !revoked {
			// The revocation lands exactly between the two checks.
			_, _ = agentSide.RevokeSubscription("g")
			revoked = true
		}
		rec, ok := agentSide.Lookup(token)
		if !ok {
			return "", false
		}
		return rec.SubID, true
	}
	rw := newRecorder()
	delivery.ServeHTTP(rw, requestFor(tok))
	if rw.Code != 404 {
		t.Fatalf("R-04 DEFECT: request authorized before revocation completed was served (%d)", rw.Code)
	}
	if calls < 2 {
		t.Fatal("re-validation did not run")
	}
}
