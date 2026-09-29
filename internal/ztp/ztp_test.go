// ztp_test.go pins the durable single-use/replay-safe
// behaviour of the ZTP Manager (C-blocker #10). The
// manager MUST persist across restarts, refuse replays,
// authenticate issued tokens via HMAC, enforce TTL
// freshness, honour scope/node binding, surface
// ErrNoProductionKey / ErrTokenMismatch / ErrTokenStale /
// ErrCapacityExhausted as the typed failure surface, and
// never invent availability.
package ztp

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// tempKey returns a small random production key.
func tempKey(t *testing.T) []byte {
	t.Helper()
	k := make([]byte, 32)
	if _, err := rand.Read(k); err != nil {
		t.Fatal(err)
	}
	return k
}

// mintToken returns a token whose MAC is computed over the
// supplied claims. Tests that want to forge a token use
// forgeToken instead.
func mintToken(t *testing.T, m *Manager, req IssueRequest) IssuedToken {
	t.Helper()
	tok, err := m.Issue(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

// forgeToken rewrites the MAC over the supplied claims
// using a separate key the manager never sees. The
// resulting token MUST be rejected with ErrTokenMismatch
// (or ErrTokenStale if the forged claims are already
// expired — that is the expected pair).
func forgeToken(_ *testing.T, tok IssuedToken, key []byte) IssuedToken {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(tok.Subject))
	mac.Write([]byte{0})
	mac.Write([]byte(tok.Scope))
	mac.Write([]byte{0})
	mac.Write([]byte(tok.Node))
	mac.Write([]byte{0})
	mac.Write([]byte(tok.IssuedAt.UTC().Format(time.RFC3339Nano)))
	mac.Write([]byte{0})
	mac.Write([]byte(tok.ExpiresAt.UTC().Format(time.RFC3339Nano)))
	tok.MAC = hex.EncodeToString(mac.Sum(nil))
	return tok
}

// mintClaims returns an IssuedToken with the supplied
// claims and a freshly-computed MAC over those claims
// keyed on key. The IssuedAt/ExpiresAt are caller-controlled
// so tests can construct expired tokens.
func mintClaims(_ *testing.T, key []byte, subject, scope, node string, issuedAt, expiresAt time.Time) IssuedToken {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(subject))
	mac.Write([]byte{0})
	mac.Write([]byte(scope))
	mac.Write([]byte{0})
	mac.Write([]byte(node))
	mac.Write([]byte{0})
	mac.Write([]byte(issuedAt.UTC().Format(time.RFC3339Nano)))
	mac.Write([]byte{0})
	mac.Write([]byte(expiresAt.UTC().Format(time.RFC3339Nano)))
	return IssuedToken{
		Subject:   subject,
		Scope:     scope,
		Node:      node,
		IssuedAt:  issuedAt,
		ExpiresAt: expiresAt,
		MAC:       hex.EncodeToString(mac.Sum(nil)),
	}
}

// TestManagerIssueAndRedeemPersistsAcrossRestart proves
// the issuance path is durable across Agent restarts.
func TestManagerIssueAndRedeemPersistsAcrossRestart(t *testing.T) {
	dir := t.TempDir()
	key := tempKey(t)
	m, err := OpenManager(dir, key)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	tok := mintToken(t, m, IssueRequest{Subject: "node-1", Scope: "enroll", Node: "node-1", Validity: time.Minute})
	if tok.MAC == "" {
		t.Fatal("issued token must carry a MAC")
	}
	rec, err := m.Redeem(context.Background(), tok, "node-1")
	if err != nil {
		t.Fatal(err)
	}
	if rec.Phase != PhaseRedeemed {
		t.Fatalf("Phase: %q, want redeemed", rec.Phase)
	}
	// Close m so the lock is released before the second
	// OpenManager simulates a restart on the same dir.
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	// Reopen the manager and verify the redemption
	// survives a restart.
	m2, err := OpenManager(dir, key)
	if err != nil {
		t.Fatal(err)
	}
	defer m2.Close()
	got, ok := m2.FindByDigest(rec.TokenDigest)
	if !ok {
		t.Fatal("redemption did not survive restart")
	}
	if got.Phase != PhaseRedeemed {
		t.Fatalf("Phase: %q, want redeemed", got.Phase)
	}
}

// TestManagerRedeemRejectsReplay proves the single-use
// invariant: a redeemed IssuedToken is refused on the
// second call with ErrTokenReplay.
func TestManagerRedeemRejectsReplay(t *testing.T) {
	dir := t.TempDir()
	m, err := OpenManager(dir, tempKey(t))
	if err != nil {
		t.Fatal(err)
	}
	tok := mintToken(t, m, IssueRequest{Subject: "node-1", Validity: time.Minute})
	if _, err := m.Redeem(context.Background(), tok, "node-1"); err != nil {
		t.Fatal(err)
	}
	_, err = m.Redeem(context.Background(), tok, "node-1")
	if !errors.Is(err, ErrTokenReplay) {
		t.Fatalf("err: %v, want ErrTokenReplay", err)
	}
}

// TestManagerRedeemRefusesWithoutProductionKey proves
// the no-production-key gate.
func TestManagerRedeemRefusesWithoutProductionKey(t *testing.T) {
	dir := t.TempDir()
	m, err := OpenManager(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	tok := IssuedToken{
		Subject: "node-1", IssuedAt: time.Now().UTC(),
		ExpiresAt: time.Now().Add(time.Minute), MAC: "deadbeef",
	}
	_, err = m.Redeem(context.Background(), tok, "node-1")
	if !errors.Is(err, ErrNoProductionKey) {
		t.Fatalf("err: %v, want ErrNoProductionKey", err)
	}
}

// TestManagerRedeemRejectsForgedMAC proves the MAC is
// required; a token whose MAC was produced with a key
// the manager never sees is rejected.
func TestManagerRedeemRejectsForgedMAC(t *testing.T) {
	dir := t.TempDir()
	m, err := OpenManager(dir, tempKey(t))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	tok := forgeToken(t, IssuedToken{
		Subject: "node-1", IssuedAt: now,
		ExpiresAt: now.Add(time.Minute),
	}, tempKey(t))
	_, err = m.Redeem(context.Background(), tok, "node-1")
	if !errors.Is(err, ErrTokenMismatch) {
		t.Fatalf("err: %v, want ErrTokenMismatch", err)
	}
}

// TestManagerRedeemRejectsExpiredToken proves the TTL
// gate: a token whose ExpiresAt is in the past is refused
// even if the MAC checks out.
func TestManagerRedeemRejectsExpiredToken(t *testing.T) {
	dir := t.TempDir()
	key := tempKey(t)
	m, err := OpenManager(dir, key)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	// IssuedAt/ExpiresAt are not adjustable via Issue
	// after the fact; rebuild the MAC over the expired
	// claims with the manager's own key by minting a
	// claim-set whose ExpiresAt is in the past.
	expired := mintClaims(t, key, "node-1", "", "node-1",
		time.Now().Add(-2*time.Hour), time.Now().Add(-time.Minute))
	_, err = m.Redeem(context.Background(), expired, "node-1")
	if !errors.Is(err, ErrTokenStale) {
		t.Fatalf("err: %v, want ErrTokenStale", err)
	}
}

// TestManagerRedeemRejectsNodeMismatch proves the
// scope/node binding: a token whose Node does not match
// the redeeming device is rejected.
func TestManagerRedeemRejectsNodeMismatch(t *testing.T) {
	dir := t.TempDir()
	m, err := OpenManager(dir, tempKey(t))
	if err != nil {
		t.Fatal(err)
	}
	tok := mintToken(t, m, IssueRequest{
		Subject: "node-1", Node: "node-1", Validity: time.Minute,
	})
	_, err = m.Redeem(context.Background(), tok, "node-2")
	if !errors.Is(err, ErrTokenMismatch) {
		t.Fatalf("err: %v, want ErrTokenMismatch", err)
	}
}

// TestManagerBindAdvancesPhase proves the post-Redeem
// Bind step. A bound record is still replay-protected.
func TestManagerBindAdvancesPhase(t *testing.T) {
	dir := t.TempDir()
	m, err := OpenManager(dir, tempKey(t))
	if err != nil {
		t.Fatal(err)
	}
	tok := mintToken(t, m, IssueRequest{Subject: "node-1", Validity: time.Minute})
	rec, err := m.Redeem(context.Background(), tok, "node-1")
	if err != nil {
		t.Fatal(err)
	}
	bound, err := m.Bind(context.Background(), rec.TokenDigest)
	if err != nil {
		t.Fatal(err)
	}
	if bound.Phase != PhaseBound {
		t.Fatalf("Phase: %q, want bound", bound.Phase)
	}
	// Double-bind: refused as replay.
	if _, err := m.Bind(context.Background(), rec.TokenDigest); !errors.Is(err, ErrTokenReplay) {
		t.Fatalf("double-bind err: %v, want ErrTokenReplay", err)
	}
}

// TestManagerFileModeIs600 pins the durable audit-trail
// permission contract.
func TestManagerFileModeIs600(t *testing.T) {
	dir := t.TempDir()
	m, err := OpenManager(dir, tempKey(t))
	if err != nil {
		t.Fatal(err)
	}
	tok := mintToken(t, m, IssueRequest{Subject: "node-1", Validity: time.Minute})
	if _, err := m.Redeem(context.Background(), tok, "node-1"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(dir, "redemption-journal.log"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode: %v, want 0600", info.Mode().Perm())
	}
}

// TestManagerCapacityExhaustedFailsClosed proves the
// audit-critical eviction defence: when the journal is
// full AND every existing record is unexpired, Redeem
// refuses with ErrCapacityExhausted rather than evict a
// record that could still be replayed.
func TestManagerCapacityExhaustedFailsClosed(t *testing.T) {
	dir := t.TempDir()
	m, err := OpenManager(dir, tempKey(t))
	if err != nil {
		t.Fatal(err)
	}
	// Force the manager into the "full" state by
	// pre-seeding MaxRedemptionJournal unexpired records.
	pre := make([]BootstrapRecord, MaxRedemptionJournal)
	base := time.Now().UTC()
	for i := 0; i < MaxRedemptionJournal; i++ {
		seed := []byte{byte('a' + i)}
		pre[i] = BootstrapRecord{
			ID:          hex.EncodeToString(sha256.New().Sum(seed))[:16],
			Phase:       PhaseRedeemed,
			TokenDigest: hex.EncodeToString(sha256.New().Sum(seed)),
			Subject:     "node-1",
			IssuedAt:    base,
			ExpiresAt:   base.Add(time.Hour),
			RedeemedAt:  base,
		}
	}
	m.mu.Lock()
	for _, rec := range pre {
		if err := m.appendLocked(rec); err != nil {
			m.mu.Unlock()
			t.Fatal(err)
		}
	}
	// The appendLocked call drops expired records; none
	// here are expired, so the journal is full.
	if len(m.journal) != MaxRedemptionJournal {
		m.mu.Unlock()
		t.Fatalf("journal len: %d, want %d", len(m.journal), MaxRedemptionJournal)
	}
	m.mu.Unlock()
	tok := mintToken(t, m, IssueRequest{Subject: "node-1", Validity: time.Minute})
	_, err = m.Redeem(context.Background(), tok, "node-1")
	if !errors.Is(err, ErrCapacityExhausted) {
		t.Fatalf("err: %v, want ErrCapacityExhausted", err)
	}
}

// TestManagerMalformedLinesFailClosed proves the
// reload path FAILS CLOSED when the journal contains a
// malformed line. The redemption journal is the
// single-use replay anchor: silently dropping a
// redeemed-token record on corruption re-opens the
// replay window (audit DV10, high severity). The
// production composition root must refuse to start
// until the operator restores a known-good journal;
// after restore the good record is preserved.
func TestManagerMalformedLinesFailClosed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "redemption-journal.log")
	good := BootstrapRecord{
		ID: "aaaaaaaaaaaa", Phase: PhaseRedeemed,
		TokenDigest: "deadbeef", IssuedAt: time.Unix(1, 0).UTC(),
	}
	line, _ := json.Marshal(good)
	body := append([]byte("{not-json\n"), line...)
	body = append(body, '\n')
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenManager(dir, tempKey(t)); err == nil {
		t.Fatalf("reload with corrupt journal must FAIL CLOSED; got nil error")
	}
	// Restore the journal to the single good record;
	// reload succeeds and the good digest is preserved.
	goodOnly := append(line, '\n')
	if err := os.WriteFile(path, goodOnly, 0o600); err != nil {
		t.Fatal(err)
	}
	m, err := OpenManager(dir, tempKey(t))
	if err != nil {
		t.Fatalf("reload of restored journal: %v", err)
	}
	defer m.Close()
	if _, ok := m.FindByDigest("deadbeef"); !ok {
		t.Fatal("good record was not loaded after restore")
	}
}
