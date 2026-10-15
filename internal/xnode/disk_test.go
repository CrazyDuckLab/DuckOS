// Package xnode / disk_test.go: UQ-01 real-filesystem end-to-end:
// the production persist.File adapter carries the registry; revoke
// → reopen from disk → credential denied. Also enrollment-token
// durability and permission checks on the durable files.
package xnode

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/persist"
)

func diskFixture(t *testing.T) (*Manager, string) {
	t.Helper()
	dir := t.TempDir()
	stateFile, err := persist.Open[DurableState](dir, "xnode-state.json")
	if err != nil {
		t.Fatal(err)
	}
	m := NewManager(nil)
	m.SetPersist(func(st DurableState) error {
		return stateFile.Update(func(cur *DurableState) { *cur = st })
	})
	m.LoadState(stateFile.Get())
	return m, dir
}

func TestDisk_RevokeSurvivesRealRestart(t *testing.T) {
	m, dir := diskFixture(t)
	tok, _, _ := m.IssueEnrollmentToken(time.Hour)
	node, cred, err := m.Enroll(tok, NodeInfo{Name: "edge"})
	if err != nil {
		t.Fatal(err)
	}
	if !m.Authenticate(node.ID, cred) {
		t.Fatal("setup: credential rejected while active")
	}
	if err := m.Revoke(node.ID); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	// Restart: a fresh manager opens the same on-disk state.
	stateFile2, err := persist.Open[DurableState](dir, "xnode-state.json")
	if err != nil {
		t.Fatal(err)
	}
	m2 := NewManager(nil)
	m2.SetPersist(func(st DurableState) error {
		return stateFile2.Update(func(cur *DurableState) { *cur = st })
	})
	m2.LoadState(stateFile2.Get())
	if m2.Authenticate(node.ID, cred) {
		t.Fatal("UQ-01 on real disk: revoked credential resurrected after restart")
	}
	if n, _ := m2.Get(node.ID); n.Status != StatusRevoked {
		t.Fatalf("restored status %s", n.Status)
	}
	// Durable file hygiene: 0600, valid JSON, no plaintext
	// credential material (only hashes are stored by design).
	info, err := os.Stat(filepath.Join(dir, "xnode-state.json"))
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("durable state permissions %o, want 600", perm)
	}
}

func TestDisk_EnrollmentTokenDurabilityReal(t *testing.T) {
	m, dir := diskFixture(t)
	tok, _, _ := m.IssueEnrollmentToken(time.Hour)
	if _, _, err := m.Enroll(tok, NodeInfo{Name: "edge"}); err != nil {
		t.Fatal(err)
	}
	stateFile2, err := persist.Open[DurableState](dir, "xnode-state.json")
	if err != nil {
		t.Fatal(err)
	}
	m2 := NewManager(nil)
	m2.LoadState(stateFile2.Get())
	if _, _, err := m2.Enroll(tok, NodeInfo{Name: "again"}); err == nil {
		t.Fatal("consumed token reusable after a real restart")
	}
}

func TestDisk_RotationSurvivesRealRestart(t *testing.T) {
	m, dir := diskFixture(t)
	tok, _, _ := m.IssueEnrollmentToken(time.Hour)
	node, oldCred, _ := m.Enroll(tok, NodeInfo{Name: "edge"})
	newCred, err := m.RotateCredential(node.ID)
	if err != nil {
		t.Fatal(err)
	}
	stateFile2, _ := persist.Open[DurableState](dir, "xnode-state.json")
	m2 := NewManager(nil)
	m2.LoadState(stateFile2.Get())
	if m2.Authenticate(node.ID, oldCred) {
		t.Fatal("old credential resurrected after real restart")
	}
	if !m2.Authenticate(node.ID, newCred) {
		t.Fatal("new credential lost after real restart")
	}
}
