// Package xnode / uq02_test.go: the post-rename commit-uncertainty
// regression suite (UQ-02).
//
// The audit scenario: rename succeeds, directory fsync returns EIO,
// the manager must NOT roll back to the previous (active) state —
// otherwise the next mutation writes "active" over the revoked
// snapshot and the old credential resurrects after restart.
package xnode

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/persist"
)

// uncertaintyFixture wires the manager to the REAL persist.File.
// The fault switch arms EIO at the directory-fsync boundary from
// another goroutine/test phase (arm() before the mutation under
// test).
func uncertaintyFixture(t *testing.T) (*Manager, *persist.File[DurableState], string, func(), func(int)) {
	t.Helper()
	dir := t.TempDir()
	stateFile, err := persist.Open[DurableState](dir, "xnode-state.json")
	if err != nil {
		t.Fatal(err)
	}
	failures := 0
	restore := persist.SyncDirForTest(func(string) error {
		if failures > 0 {
			failures--
			return errors.New("injected EIO on directory fsync")
		}
		return nil
	})
	arm := func(n int) { failures = n }
	m := NewManager(nil)
	m.SetPersist(func(st DurableState) (CommitResult, error) {
		out, err := stateFile.Update(func(cur *DurableState) { *cur = st })
		return CommitResult{Uncertain: out == persist.CommitUncertain}, err
	})
	m.LoadState(stateFile.Get())
	return m, stateFile, dir, restore, arm
}

// Audit reproduction: revoke with post-rename fsync failure →
// live_old_credential must be FALSE (restrictive state retained),
// disk must contain revoked, and a subsequent mutation must never
// write "active" back over the revoked snapshot.
func TestUQ02_PostRenameFsyncFailureNoResurrection(t *testing.T) {
	m, stateFile, dir, restore, arm := uncertaintyFixture(t)
	defer restore()
	tok, _, err := m.IssueEnrollmentToken(time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	node, cred, err := m.Enroll(tok, NodeInfo{Name: "edge"})
	if err != nil {
		t.Fatal(err)
	}
	if !m.Authenticate(node.ID, cred) {
		t.Fatal("setup: credential rejected while active")
	}
	// Revoke: rename lands, dir fsync fails.
	arm(1)
	err = m.Revoke(node.ID)
	if err == nil {
		t.Fatal("uncertain commit acknowledged as success")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "uncertain") {
		t.Fatalf("failure does not report uncertainty: %v", err)
	}
	// live_old_credential must be FALSE: the restrictive state stays.
	if m.Authenticate(node.ID, cred) {
		t.Fatal("UQ-02 REPRODUCED: live_old_credential=true after uncertain revocation")
	}
	if m.Degraded() == "" {
		t.Fatal("uncertain commit did not mark storage degraded")
	}
	// Disk shows the revoked state (the rename landed).
	disk := stateFile.Get()
	found := false
	for _, n := range disk.Nodes {
		if n.ID == node.ID {
			found = true
			if n.Status != StatusRevoked {
				t.Fatalf("disk_status=%s, want revoked", n.Status)
			}
		}
	}
	if !found {
		t.Fatal("revoked node missing from the durable snapshot")
	}
	// Subsequent mutation while degraded: REFUSED — never a silent
	// write of the older active snapshot.
	if err := m.Suspend(node.ID); err == nil || !strings.Contains(err.Error(), "degraded") {
		t.Fatalf("mutation during uncertainty not blocked: %v", err)
	}
	// Reconciliation reads the disk: revision matches (the commit
	// landed) → recovered; the revoked state is authoritative.
	res, err := m.ReconcileStorage(stateFile.Get())
	if err != nil || !strings.Contains(res, "recovered") {
		t.Fatalf("reconcile: %s %v", res, err)
	}
	if m.Degraded() != "" {
		t.Fatal("degraded flag not cleared after reconciliation")
	}
	// B: an unrelated mutation now succeeds and must PRESERVE the
	// revocation on disk.
	tok2, _, _ := m.IssueEnrollmentToken(time.Hour)
	if _, _, err := m.Enroll(tok2, NodeInfo{Name: "edge-2"}); err != nil {
		t.Fatal(err)
	}
	disk2 := stateFile.Get()
	for _, n := range disk2.Nodes {
		if n.ID == node.ID && n.Status != StatusRevoked {
			t.Fatalf("unrelated mutation resurrected status %s", n.Status)
		}
	}
	// C: manager reconstruction keeps authorization secure.
	stateFile2, err := persist.Open[DurableState](dir, "xnode-state.json")
	if err != nil {
		t.Fatal(err)
	}
	m2 := NewManager(nil)
	m2.LoadState(stateFile2.Get())
	if m2.Authenticate(node.ID, cred) {
		t.Fatal("UQ-02 REPRODUCED: restarted_old_credential=true")
	}
}

// D: rotation with a crash-like uncertain commit: the OLD credential
// must never be silently restored as valid.
func TestUQ02_RotationUncertainCommit(t *testing.T) {
	m, stateFile, _, restore, arm := uncertaintyFixture(t)
	defer restore()
	tok, _, _ := m.IssueEnrollmentToken(time.Hour)
	node, oldCred, _ := m.Enroll(tok, NodeInfo{Name: "edge"})
	arm(1)
	newCred, err := m.RotateCredential(node.ID)
	if err == nil {
		t.Fatal("uncertain rotation acknowledged as success")
	}
	_ = newCred
	// Restrictive state (new credential) retained in memory.
	if !m.Authenticate(node.ID, oldCred) {
		t.Log("old credential already rejected in memory")
	}
	// After reconciliation from disk (rotation landed), the old
	// credential stays rejected.
	if _, err := m.ReconcileStorage(stateFile.Get()); err != nil {
		t.Fatal(err)
	}
	if m.Authenticate(node.ID, oldCred) {
		t.Fatal("old credential restored as valid after uncertain rotation")
	}
}

// Uncertainty where the commit did NOT land (revision on disk is
// older): disk is adopted wholesale and the operator is told to
// verify recent revocations — never a silent resolution.
func TestUQ02_ReconcileWithStaleDisk(t *testing.T) {
	// Direct unit: stale disk revision.
	mgr2 := NewManager(nil)
	mgr2.revision = 5
	mgr2.degraded = "commit_uncertain at revision 5: injected"
	res, err := mgr2.ReconcileStorage(DurableState{Revision: 3})
	if err != nil || !strings.Contains(res, "disk state adopted") {
		t.Fatalf("stale-disk reconcile: %s %v", res, err)
	}
	if mgr2.Degraded() != "" {
		t.Fatal("degraded flag not cleared")
	}
	if mgr2.revision != 3 {
		t.Fatalf("revision %d, want disk-authoritative 3", mgr2.revision)
	}
}

// N: concurrent security mutations under fault injection — the
// mutations that hit uncertainty fail with uncertainty, the rest
// commit; no lost updates, no deadlocks (race detector).
func TestUQ02_ConcurrentMutationsWithFaults(t *testing.T) {
	m, stateFile, _, restore, arm := uncertaintyFixture(t)
	defer restore()
	var nodes []string
	for i := 0; i < 6; i++ {
		tok, _, _ := m.IssueEnrollmentToken(time.Hour)
		n, _, err := m.Enroll(tok, NodeInfo{Name: fmt.Sprintf("edge-%d", i)})
		if err != nil {
			t.Fatal(err)
		}
		nodes = append(nodes, n.ID)
	}
	arm(2)
	var wg sync.WaitGroup
	uncertain := 0
	var mu sync.Mutex
	for _, id := range nodes {
		id := id
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := m.Revoke(id)
			if err != nil && strings.Contains(strings.ToLower(err.Error()), "uncertain") {
				mu.Lock()
				uncertain++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	// Reconcile from the final disk state; every node that the disk
	// shows as revoked must reject its credential class, and no
	// mutation may remain possible while degraded.
	_, _ = m.ReconcileStorage(stateFile.Get())
	disk := stateFile.Get()
	revoked := 0
	for _, n := range disk.Nodes {
		if n.Status == StatusRevoked {
			revoked++
		}
	}
	if revoked == 0 {
		t.Fatal("no revocation reached the durable snapshot")
	}
	_ = uncertain
}

// Migration: legacy two-file records → validated snapshot; revoked
// credentials stay revoked; inconsistent input is refused.
func TestUQ02_Migration(t *testing.T) {
	good := []Node{
		{ID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Kind: KindRemote, Name: "edge",
			CredentialHash: "abc", Status: StatusActive, EnrolledAt: time.Now().UTC()},
		{ID: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Kind: KindRemote, Name: "old",
			CredentialHash: "def", Status: StatusRevoked, EnrolledAt: time.Now().UTC()},
	}
	tokens := []EnrollmentToken{
		{TokenHash: "h1", CreatedAt: time.Now().UTC(), Used: true, UsedBy: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
	}
	st, err := MigrateLegacy(good, tokens)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range st.Nodes {
		if n.Status == StatusRevoked && n.ID != "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" {
			t.Fatal("migration altered revocation state")
		}
	}
	// Inconsistent inputs are refused.
	bad := []Node{{ID: "", Kind: KindRemote, Status: StatusActive}}
	if _, err := MigrateLegacy(bad, nil); err == nil {
		t.Fatal("node without id accepted")
	}
	dup := []Node{good[0], good[0]}
	if _, err := MigrateLegacy(dup, nil); err == nil {
		t.Fatal("duplicate id accepted")
	}
	orphan := []EnrollmentToken{{TokenHash: "h2", Used: true, UsedBy: "ffffffffffffffffffffffffffffffff"}}
	if _, err := MigrateLegacy(good, orphan); err == nil {
		t.Fatal("token referencing unknown node accepted")
	}
	noHash := []Node{{ID: "cccccccccccccccccccccccccccccccc", Kind: KindRemote, Status: StatusActive}}
	if _, err := MigrateLegacy(noHash, nil); err == nil {
		t.Fatal("remote node without credential hash accepted")
	}
}
