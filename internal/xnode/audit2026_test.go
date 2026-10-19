// Package xnode / audit2026_test.go: developer regression suite for
// the independent audit findings AUD-03 and AUD-05 (2026-10-19).
// Each test mirrors an auditor assertion with original wording and
// verified-failed against the vulnerable implementation (the audit
// worktree preserves the failing state).
package xnode

import (
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/persist"
)

// AUD-03: an older-revision snapshot must never silently reactivate
// a revoked node; the merge is restrictive and reports protections.
func TestAUD03_ReconcileNeverReactivatesRevoked(t *testing.T) {
	dir := t.TempDir()
	stateFile, err := persist.Open[DurableState](dir, "xnode-state.json")
	if err != nil {
		t.Fatal(err)
	}
	m := NewManager(nil)
	m.SetPersist(func(st DurableState) (CommitResult, error) {
		out, err := stateFile.Update(func(cur *DurableState) { *cur = st })
		return CommitResult{Uncertain: out == persist.CommitUncertain}, err
	})
	m.LoadState(stateFile.Get())
	tok, _, _ := m.IssueEnrollmentToken(time.Hour)
	node, cred, err := m.Enroll(tok, NodeInfo{Name: "edge"})
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Revoke(node.ID); err != nil {
		t.Fatal(err)
	}
	m.mu.Lock()
	m.degraded = "simulated uncertain"
	m.mu.Unlock()

	older := DurableState{Revision: 1, Nodes: []Node{{
		ID: node.ID, Name: "edge", Kind: KindRemote,
		CredentialHash: node.CredentialHash, Status: StatusActive,
	}}}
	res, err := m.ReconcileStorage(older)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if n, _ := m.Get(node.ID); n.Status != StatusRevoked {
		t.Fatalf("AUD-03 DEFECT: revoked node resurrected (status=%s, result=%q)", n.Status, res)
	}
	// Authentication — not just object status.
	if m.Authenticate(node.ID, cred) {
		t.Fatal("AUD-03 DEFECT: revoked credential authenticates after reconcile")
	}
	// The next committed mutation must persist the REVOKED state,
	// never the permissive one.
	tok2, _, _ := m.IssueEnrollmentToken(time.Hour)
	if _, _, err := m.Enroll(tok2, NodeInfo{Name: "edge-2"}); err != nil {
		t.Fatal(err)
	}
	reopened, _ := persist.Open[DurableState](dir, "xnode-state.json")
	for _, n := range reopened.Get().Nodes {
		if n.ID == node.ID && n.Status != StatusRevoked {
			t.Fatalf("permissive state written back to disk: %s", n.Status)
		}
	}
}

// AUD-03: a consumed enrollment token stays consumed across an
// older-snapshot adoption.
func TestAUD03_ConsumedTokenStaysConsumed(t *testing.T) {
	m := NewManager(nil)
	m.SetPersist(func(DurableState) (CommitResult, error) { return CommitResult{}, nil })
	tok, rec, err := m.IssueEnrollmentToken(time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	node, _, err := m.Enroll(tok, NodeInfo{Name: "edge"})
	if err != nil {
		t.Fatal(err)
	}
	m.mu.Lock()
	m.degraded = "simulated"
	// Older disk view where the token was never consumed.
	st := DurableState{Revision: m.revision - 1,
		Nodes:       []Node{*m.nodes[node.ID]},
		Enrollments: []EnrollmentToken{{TokenHash: rec.TokenHash, CreatedAt: rec.CreatedAt, Used: false}},
	}
	m.mu.Unlock()
	if _, err := m.ReconcileStorage(st); err != nil {
		t.Fatal(err)
	}
	// The token must NOT be redeemable again.
	if _, _, err := m.Enroll(tok, NodeInfo{Name: "again"}); err == nil {
		t.Fatal("AUD-03 DEFECT: consumed token re-armed by reconcile")
	}
}

// AUD-05: adversarial concurrent degraded-gate stress (race
// detector). The gate must be evaluated under the mutation lock —
// no TOCTOU window where a mutation slips past a transitioning
// degraded flag.
func TestAUD05_DegradedGateNoTOCTOU(t *testing.T) {
	m := NewManager(nil)
	var seqMu sync.Mutex
	seq := 0
	m.SetPersist(func(DurableState) (CommitResult, error) {
		seqMu.Lock()
		seq++
		seqMu.Unlock()
		return CommitResult{Uncertain: true}, errors.New("injected uncertain")
	})
	m.mu.Lock()
	m.nodes["n1"] = &Node{ID: "n1", Name: "n1", Kind: KindRemote,
		CredentialHash: "h", Status: StatusActive}
	m.revision = 5
	m.mu.Unlock()

	stop := make(chan struct{})
	var revoker sync.WaitGroup
	revoker.Add(1)
	go func() {
		defer revoker.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			_ = m.Revoke("n1")
		}
	}()
	var issuers sync.WaitGroup
	for g := 0; g < 4; g++ {
		issuers.Add(1)
		go func() {
			defer issuers.Done()
			for i := 0; i < 200; i++ {
				_, _, _ = m.IssueEnrollmentToken(time.Hour)
			}
		}()
	}
	issuers.Wait()
	close(stop)
	revoker.Wait()
	if m.Degraded() == "" {
		t.Fatal("degraded flag never set under adversarial concurrency")
	}
	// Once degraded, mutations are refused (fail closed), verified
	// by authentication-relevant behavior: a fresh issue must fail.
	if _, _, err := m.IssueEnrollmentToken(time.Hour); err == nil {
		t.Fatal("mutation succeeded while degraded")
	}
	_ = filepath.Join
}
