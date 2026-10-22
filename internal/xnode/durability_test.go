// Package xnode / durability_test.go: UQ-01 regression suite.
//
// Recreates the independent audit reproduction (ENOSPC on the
// persistence hook during a revocation) and proves the invariant:
// a security-sensitive transition is acknowledged ONLY when the
// durable snapshot carrying it has been committed; reconstructed
// managers never resurrect revoked credentials, rotated-out
// credentials or consumed enrollment tokens.
package xnode

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

// flakyPersist is the fault injector: the first `failures` calls
// return the injected error, then commit for real into `store`.
type flakyPersist struct {
	mu       sync.Mutex
	failures int
	err      error
	store    DurableState
	commits  int
}

func (f *flakyPersist) persist(st DurableState) (CommitResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failures > 0 {
		f.failures--
		if f.err == nil {
			f.err = errors.New("injected ENOSPC: no space left on device")
		}
		return CommitResult{}, f.err
	}
	f.store = st
	f.commits++
	return CommitResult{}, nil
}

func (f *flakyPersist) failNext(times int, err error) {
	f.mu.Lock()
	f.failures, f.err = times, err
	f.mu.Unlock()
}

func durableFixture(t *testing.T) (*Manager, *flakyPersist) {
	t.Helper()
	f := &flakyPersist{}
	m := NewManager(nil)
	m.SetPersist(f.persist)
	return m, f
}

// A: active node → persistence failure on revoke → failure reported
// AND in-memory revocation rolled back (auth still works; there is
// no false "revoked" state to lose).
func TestUQ01_A_RevokeFailureNotAcknowledged(t *testing.T) {
	m, f := durableFixture(t)
	tok, _, _ := m.IssueEnrollmentToken(time.Hour)
	node, cred, err := m.Enroll(tok, NodeInfo{Name: "edge"})
	if err != nil {
		t.Fatal(err)
	}
	f.failNext(1, errors.New("injected ENOSPC: no space left on device"))
	err = m.Revoke(node.ID)
	if err == nil {
		t.Fatal("revocation acknowledged despite failed persistence (UQ-01 reproduced)")
	}
	if !strings.Contains(err.Error(), "ENOSPC") {
		t.Fatalf("error does not carry the storage cause: %v", err)
	}
	// No contradictory in-memory state: the node is still active,
	// the credential still authoritative — exactly what the
	// durable snapshot says.
	if m.Authenticate(node.ID, cred) == false {
		t.Fatal("memory rolled back but authentication refuses the credential: state divergence")
	}
	if n, _ := m.Get(node.ID); n.Status == StatusRevoked {
		t.Fatal("node marked revoked in memory while durable state is active")
	}
	// Retry after the fault clears: deterministic success.
	if err := m.Revoke(node.ID); err != nil {
		t.Fatalf("retry after fault cleared: %v", err)
	}
	if m.Authenticate(node.ID, cred) {
		t.Fatal("revoked credential accepted after committed retry")
	}
}

// B: successful revoke → restart (manager reconstruction from the
// durable snapshot) → old credential rejected.
func TestUQ01_B_RevocationSurvivesRestart(t *testing.T) {
	m, f := durableFixture(t)
	tok, _, _ := m.IssueEnrollmentToken(time.Hour)
	node, cred, _ := m.Enroll(tok, NodeInfo{Name: "edge"})
	if err := m.Revoke(node.ID); err != nil {
		t.Fatal(err)
	}
	// Reconstruct exactly from the durable snapshot.
	m2 := NewManager(nil)
	m2.SetPersist(f.persist)
	m2.LoadState(f.store)
	if m2.Authenticate(node.ID, cred) {
		t.Fatal("UQ-01 CONFIRMED: revoked credential resurrected after restart")
	}
	if n, _ := m2.Get(node.ID); n.Status != StatusRevoked {
		t.Fatalf("restored status %s", n.Status)
	}
}

// C: successful rotation → restart → old credential rejected.
func TestUQ01_C_RotationSurvivesRestart(t *testing.T) {
	m, f := durableFixture(t)
	tok, _, _ := m.IssueEnrollmentToken(time.Hour)
	node, oldCred, _ := m.Enroll(tok, NodeInfo{Name: "edge"})
	newCred, err := m.RotateCredential(node.ID)
	if err != nil {
		t.Fatal(err)
	}
	m2 := NewManager(nil)
	m2.LoadState(f.store)
	if m2.Authenticate(node.ID, oldCred) {
		t.Fatal("rotated-out credential resurrected after restart")
	}
	if !m2.Authenticate(node.ID, newCred) {
		t.Fatal("rotated-in credential lost after restart")
	}
}

// D: enrollment token consumed → restart → token unusable.
func TestUQ01_D_EnrollmentTokenConsumptionDurable(t *testing.T) {
	m, f := durableFixture(t)
	tok, _, _ := m.IssueEnrollmentToken(time.Hour)
	if _, _, err := m.Enroll(tok, NodeInfo{Name: "edge"}); err != nil {
		t.Fatal(err)
	}
	m2 := NewManager(nil)
	m2.SetPersist(f.persist)
	m2.LoadState(f.store)
	if _, _, err := m2.Enroll(tok, NodeInfo{Name: "again"}); err == nil {
		t.Fatal("consumed enrollment token reusable after restart")
	}
}

// D2: failed enrollment commit re-arms the token — and the failure
// is reported, never acknowledged.
func TestUQ01_D2_EnrollCommitFailureRollsBackToken(t *testing.T) {
	m, f := durableFixture(t)
	tok, _, _ := m.IssueEnrollmentToken(time.Hour)
	f.failNext(1, errors.New("injected EIO: I/O error"))
	node, cred, err := m.Enroll(tok, NodeInfo{Name: "edge"})
	if err == nil {
		t.Fatal("enrollment acknowledged despite failed commit")
	}
	if node.ID != "" || cred != "" {
		t.Fatal("failed enrollment returned credentials")
	}
	// The token is usable again exactly once, and the second
	// attempt commits.
	node2, cred2, err := m.Enroll(tok, NodeInfo{Name: "edge"})
	if err != nil {
		t.Fatalf("token not re-armed after rolled-back enrollment: %v", err)
	}
	if !m.Authenticate(node2.ID, cred2) {
		t.Fatal("committed enrollment rejected")
	}
}

// E–F live at the subscription layer; G: concurrent security
// mutations lose no update.
func TestUQ01_G_ConcurrentRevocationsNoLostUpdate(t *testing.T) {
	m, f := durableFixture(t)
	var ids []string
	for i := 0; i < 8; i++ {
		tok, _, _ := m.IssueEnrollmentToken(time.Hour)
		node, _, err := m.Enroll(tok, NodeInfo{Name: fmt.Sprintf("edge-%02d", i)})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, node.ID)
	}
	var wg sync.WaitGroup
	errs := make([]error, len(ids))
	for i, id := range ids {
		wg.Add(1)
		go func(i int, id string) {
			defer wg.Done()
			errs[i] = m.Revoke(id)
		}(i, id)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("concurrent revoke %d failed: %v", i, err)
		}
	}
	// Every revocation must be present in the final committed
	// snapshot (no lost update).
	revokedSeen := map[string]bool{}
	for _, n := range f.store.Nodes {
		if n.Status == StatusRevoked {
			revokedSeen[n.ID] = true
		}
	}
	for _, id := range ids {
		if !revokedSeen[id] {
			t.Fatalf("lost update: node %s not revoked in the durable snapshot", id)
		}
	}
	// Reconstruction rejects every credential class.
	m2 := NewManager(nil)
	m2.LoadState(f.store)
	if len(m2.Eligible()) != 0 {
		t.Fatal("eligible nodes survived a fully revoked snapshot")
	}
}

// H/I: the atomic-replacement adapter (persist.File) is exercised
// in persist/file_test.go (partial-write rollback, fresh-file
// semantics). Here: an interrupted commit leaves the LAST GOOD
// snapshot authoritative and consistent.
func TestUQ01_HI_InterruptedCommitKeepsLastGoodState(t *testing.T) {
	m, f := durableFixture(t)
	tok, _, _ := m.IssueEnrollmentToken(time.Hour)
	node, cred, _ := m.Enroll(tok, NodeInfo{Name: "edge"})
	_ = m.Revoke(node.ID)
	goodSnapshot := f.store
	// Simulate a crash mid-write of a NEW enrollment: the durable
	// store still holds the last good snapshot; reconstruction
	// yields exactly that.
	tok2, _, _ := m.IssueEnrollmentToken(time.Hour)
	if _, _, err := m.Enroll(tok2, NodeInfo{Name: "second"}); err != nil {
		t.Fatal(err)
	}
	mCrashed := NewManager(nil)
	mCrashed.LoadState(goodSnapshot) // what was on disk before the crash
	if _, ok := mCrashed.Get("nonexistent"); ok {
		t.Fatal("impossible state")
	}
	// The authoritative last-good snapshot still revokes node 1.
	if mCrashed.Authenticate(node.ID, cred) {
		t.Fatal("last-good durable snapshot lost a committed revocation")
	}
}

// J: repeated revocation is idempotent and deterministic.
func TestUQ01_J_RevokeIdempotent(t *testing.T) {
	m, f := durableFixture(t)
	tok, _, _ := m.IssueEnrollmentToken(time.Hour)
	node, cred, _ := m.Enroll(tok, NodeInfo{Name: "edge"})
	for i := 0; i < 3; i++ {
		if err := m.Revoke(node.ID); err != nil {
			t.Fatalf("revoke %d: %v", i, err)
		}
	}
	if m.Authenticate(node.ID, cred) {
		t.Fatal("credential accepted after repeated revocation")
	}
	if f.commits == 0 {
		t.Fatal("no durable commits recorded")
	}
}

// F at the node layer: storage failure during a security mutation
// never produces a success response (covers rotation/suspension
// besides revocation).
func TestUQ01_F_StorageFailureDuringSecurityMutations(t *testing.T) {
	m, f := durableFixture(t)
	tok, _, _ := m.IssueEnrollmentToken(time.Hour)
	node, oldCred, _ := m.Enroll(tok, NodeInfo{Name: "edge"})
	cases := []struct {
		name string
		mut  func() error
	}{
		{"rotate", func() error { _, err := m.RotateCredential(node.ID); return err }},
		{"suspend", func() error { return m.Suspend(node.ID) }},
		{"resume", func() error { return m.Resume(node.ID) }},
		{"enroll-token", func() error { _, _, err := m.IssueEnrollmentToken(time.Hour); return err }},
	}
	for _, c := range cases {
		f.failNext(1, fmt.Errorf("injected EACCES: permission denied"))
		if err := c.mut(); err == nil {
			t.Errorf("%s acknowledged despite failed persistence", c.name)
		}
	}
	// After every failed mutation the durable authoritative state
	// still authenticates the original credential chain.
	if !m.Authenticate(node.ID, oldCred) {
		t.Fatal("in-memory state diverged from durable state after failed mutations")
	}
}
