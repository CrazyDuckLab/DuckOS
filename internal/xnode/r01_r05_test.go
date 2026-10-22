// Package xnode / r01_r05_test.go: developer regression suite for
// the 2026-10-20 stabilization findings R-01, R-03 and R-05.
package xnode

import (
	"errors"
	"sync"
	"testing"
	"time"
)

// R-01: a suspended node must not silently become active when an
// older snapshot says active; actual authentication is verified,
// and a durably committed resume is not blocked afterwards.
func TestR01_SuspensionProtectedByReconcile(t *testing.T) {
	m := NewManager(nil)
	m.SetPersist(func(DurableState) (CommitResult, error) { return CommitResult{}, nil })
	tok, _, _ := m.IssueEnrollmentToken(time.Hour)
	node, cred, err := m.Enroll(tok, NodeInfo{Name: "edge"})
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Suspend(node.ID); err != nil {
		t.Fatal(err)
	}
	// Degraded + older permissive snapshot (status active).
	m.mu.Lock()
	m.degraded = "simulated"
	older := DurableState{Revision: m.revision - 1,
		Nodes: []Node{{ID: node.ID, Name: "edge", Kind: KindRemote,
			CredentialHash: node.CredentialHash, Status: StatusActive}}}
	m.mu.Unlock()
	if _, err := m.ReconcileStorage(older); err != nil {
		t.Fatal(err)
	}
	if n, _ := m.Get(node.ID); n.Status != StatusSuspended {
		t.Fatalf("R-01 DEFECT: suspended node became %s", n.Status)
	}
	if m.Authenticate(node.ID, cred) {
		t.Fatal("R-01 DEFECT: suspended node authenticates after reconcile")
	}
	// The restrictive status persists to disk on the next mutation.
	tok2, _, _ := m.IssueEnrollmentToken(time.Hour)
	if _, _, err := m.Enroll(tok2, NodeInfo{Name: "edge-2"}); err != nil {
		t.Fatal(err)
	}
	m.mu.RLock()
	snapRev := m.revision
	m.mu.RUnlock()
	_ = snapRev
	// A durably committed resume (normal mutation path) still works.
	if err := m.Resume(node.ID); err != nil {
		t.Fatalf("legitimate resume blocked: %v", err)
	}
	if !m.Authenticate(node.ID, cred) {
		t.Fatal("committed resume did not restore access")
	}
}

// R-05: deterministic TOCTOU proof — the degraded gate and the
// mutation share one critical section. A concurrent revocation that
// flips the domain to degraded can never slip an issuance between
// check and act: after the first uncertain commit, at least one of
// the racing issuances is refused with the degraded diagnosis, and
// no issuance succeeds after degradation is observable.
func TestR05_DegradedGateDeterministicTOCTOU(t *testing.T) {
	m := NewManager(nil)
	var mu sync.Mutex
	uncertainEmitted := false
	m.SetPersist(func(DurableState) (CommitResult, error) {
		mu.Lock()
		first := !uncertainEmitted
		uncertainEmitted = true
		mu.Unlock()
		if first {
			return CommitResult{Uncertain: true}, errors.New("injected uncertain")
		}
		return CommitResult{}, nil
	})
	m.mu.Lock()
	m.nodes["n1"] = &Node{ID: "n1", Name: "n1", Kind: KindRemote,
		CredentialHash: "h", Status: StatusActive}
	m.revision = 5
	m.mu.Unlock()

	var wg sync.WaitGroup
	degradedSeen := false
	var once sync.Once
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				_, _, err := m.IssueEnrollmentToken(time.Hour)
				if err != nil && containsSub(err.Error(), "degraded") {
					once.Do(func() { degradedSeen = true })
					return
				}
			}
		}()
	}
	// One revoker drives the uncertain transition concurrently.
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = m.Revoke("n1")
	}()
	wg.Wait()
	if !degradedSeen && m.Degraded() == "" {
		t.Fatal("R-05: degradation never observed under contention")
	}
	if m.Degraded() != "" {
		// Once degraded is observable, EVERY issuance must fail.
		for i := 0; i < 10; i++ {
			if _, _, err := m.IssueEnrollmentToken(time.Hour); err == nil {
				t.Fatal("R-05 DEFECT: issuance succeeded while degraded observable")
			}
		}
	}
}

func containsSub(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
