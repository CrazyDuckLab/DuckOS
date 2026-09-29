// Provisioning / ZTP contract torture regression.
package provisioning

import (
	"crypto/rand"
	"errors"
	"strings"
	"testing"
	"time"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	s := NewStore()
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	if err := s.SetProductionKey(key, "test-key-1"); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestStore_SetProductionKey_RejectsShortKey(t *testing.T) {
	s := NewStore()
	if err := s.SetProductionKey([]byte("short"), "x"); err == nil {
		t.Fatalf("short key must be rejected")
	}
}

func TestStore_SetProductionKey_NilDisables(t *testing.T) {
	s := newStore(t)
	if err := s.SetProductionKey(nil, ""); err != nil {
		t.Fatal(err)
	}
	if s.ProductionKeyID() != "" {
		t.Fatalf("nil key must clear productionKeyID")
	}
	if _, err := s.Issue(ScopeNodeEnrol, "", time.Minute); !errors.Is(err, ErrNoProductionKey) {
		t.Fatalf("Issue without key must surface ErrNoProductionKey, got %v", err)
	}
}

func TestIssue_RejectsInvalidScope(t *testing.T) {
	s := newStore(t)
	if _, err := s.Issue(Scope("garbage"), "", time.Minute); !errors.Is(err, ErrScopeRequired) {
		t.Fatalf("invalid scope must surface ErrScopeRequired, got %v", err)
	}
}

func TestIssue_RejectsInvalidBinding(t *testing.T) {
	s := newStore(t)
	if _, err := s.Issue(ScopeNodeEnrol, "BAD!", time.Minute); err == nil {
		t.Fatalf("invalid node binding must be rejected")
	}
}

func TestIssue_ClampsTTL(t *testing.T) {
	s := newStore(t)
	// Zero TTL → DefaultTTL
	sec, err := s.Issue(ScopeNodeEnrol, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if sec.ExpiresAt.Sub(time.Now().UTC()) < DefaultTTL-time.Minute {
		t.Fatalf("zero TTL must default to DefaultTTL, got %v", sec.ExpiresAt)
	}
	// Excessive TTL → MaxTTL
	sec, err = s.Issue(ScopeNodeEnrol, "", 30*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if sec.ExpiresAt.Sub(time.Now().UTC()) > MaxTTL+time.Minute {
		t.Fatalf("excessive TTL must clamp to MaxTTL, got %v", sec.ExpiresAt)
	}
}

func TestIssueAndConsume_Success(t *testing.T) {
	s := newStore(t)
	sec, err := s.Issue(ScopeNodeEnrol, "node-001", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if sec.Plaintext == "" || sec.ID == "" {
		t.Fatalf("secret must be populated")
	}
	if err := s.ConsumeFor(sec.ID, sec.Plaintext, ScopeNodeEnrol, "node-001", "agent-001"); err != nil {
		t.Fatalf("Consume: %v", err)
	}
	cred, ok := s.Peek(sec.ID)
	if !ok || !cred.Consumed {
		t.Fatalf("credential must be marked consumed")
	}
}

func TestConsume_RejectsUnknownID(t *testing.T) {
	s := newStore(t)
	if err := s.Consume("no-such-id-1234567890", "x", "agent"); !errors.Is(err, ErrUnknownID) {
		t.Fatalf("unknown id must surface ErrUnknownID, got %v", err)
	}
}

func TestConsume_RejectsBadIDFormat(t *testing.T) {
	s := newStore(t)
	if err := s.Consume("X", "x", "agent"); !errors.Is(err, ErrUnknownID) {
		t.Fatalf("bad id format must surface ErrUnknownID")
	}
}

func TestConsume_RejectsDoubleUse(t *testing.T) {
	s := newStore(t)
	sec, err := s.Issue(ScopeServiceBind, "", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ConsumeFor(sec.ID, sec.Plaintext, ScopeServiceBind, "", "agent"); err != nil {
		t.Fatal(err)
	}
	if err := s.ConsumeFor(sec.ID, sec.Plaintext, ScopeServiceBind, "", "agent"); !errors.Is(err, ErrAlreadyConsumed) {
		t.Fatalf("double consume must surface ErrAlreadyConsumed, got %v", err)
	}
}

func TestConsume_RejectsExpired(t *testing.T) {
	s := newStore(t)
	sec, err := s.Issue(ScopeNodeEnrol, "", 50*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(60 * time.Millisecond)
	if err := s.Consume(sec.ID, sec.Plaintext, "agent"); !errors.Is(err, ErrExpired) {
		t.Fatalf("expired consume must surface ErrExpired, got %v", err)
	}
}

func TestConsume_RejectsWrongPlaintext(t *testing.T) {
	s := newStore(t)
	sec, err := s.Issue(ScopeNodeEnrol, "", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Consume(sec.ID, "wrong-plaintext", "agent"); err == nil {
		t.Fatalf("wrong plaintext must be rejected")
	}
}

func TestConsume_RejectsEmptyPlaintext(t *testing.T) {
	s := newStore(t)
	sec, _ := s.Issue(ScopeNodeEnrol, "", time.Hour)
	if err := s.Consume(sec.ID, "", "agent"); err == nil {
		t.Fatalf("empty plaintext must be rejected")
	}
}

func TestConsume_RejectsEmptyConsumedBy(t *testing.T) {
	s := newStore(t)
	sec, _ := s.Issue(ScopeNodeEnrol, "", time.Hour)
	if err := s.Consume(sec.ID, sec.Plaintext, ""); err == nil {
		t.Fatalf("empty consumedBy must be rejected")
	}
}

func TestGCExpired_RemovesUnconsumed(t *testing.T) {
	s := newStore(t)
	sec, _ := s.Issue(ScopeNodeEnrol, "", 50*time.Millisecond)
	time.Sleep(60 * time.Millisecond)
	if removed := s.GCExpired(); removed != 1 {
		t.Fatalf("expected 1 expired credential removed, got %d", removed)
	}
	if _, ok := s.Peek(sec.ID); ok {
		t.Fatalf("expired credential must be removed")
	}
}

func TestGCExpired_KeepsConsumed(t *testing.T) {
	s := newStore(t)
	sec, _ := s.Issue(ScopeNodeEnrol, "", time.Hour)
	if err := s.Consume(sec.ID, sec.Plaintext, "agent"); err != nil {
		t.Fatal(err)
	}
	if removed := s.GCExpired(); removed != 0 {
		t.Fatalf("consumed credential must not be GC'd, got %d", removed)
	}
}

func TestAll_NewestFirst(t *testing.T) {
	s := newStore(t)
	base := time.Now().UTC()
	for i := 0; i < 3; i++ {
		sec, _ := s.Issue(ScopeNodeEnrol, "", time.Hour)
		_ = sec
		// back-date the CreatedAt for ordering
		s.mu.Lock()
		cred := s.creds[sec.ID]
		cred.CreatedAt = base.Add(time.Duration(i) * time.Second)
		s.creds[sec.ID] = cred
		s.mu.Unlock()
	}
	all := s.All()
	if len(all) != 3 || all[0].CreatedAt.Sub(base) < 2*time.Second {
		t.Fatalf("newest first failed: %+v", all)
	}
}

func TestPlaintextFormat_NoPadding(t *testing.T) {
	s := newStore(t)
	sec, _ := s.Issue(ScopeServiceBind, "", time.Hour)
	if strings.Contains(sec.Plaintext, "=") {
		t.Fatalf("plaintext must use base32 without padding, got %q", sec.Plaintext)
	}
}

func TestHash_DifferentKeysDifferentHashes(t *testing.T) {
	s1 := newStore(t)
	sec1, _ := s1.Issue(ScopeNodeEnrol, "node-1", time.Hour)
	cred1, _ := s1.Peek(sec1.ID)
	s2 := newStore(t)
	sec2, _ := s2.Issue(ScopeNodeEnrol, "node-1", time.Hour)
	cred2, _ := s2.Peek(sec2.ID)
	if cred1.Hash == cred2.Hash {
		t.Fatalf("different production keys must yield different hashes")
	}
}
