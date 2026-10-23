// Package xnode / b4_remediation_test.go: remediation regressions for
// the 2026-10-23 independent audit (B4-01/02/03). Ported from the
// auditor's tests; the composition contract was corrected per the
// audit's own closure condition: the CLI durably adopts the pending
// credential BEFORE controller confirm and finalizes AFTER it.
package xnode

import (
	"testing"
	"time"
)

// B4-01/B4-02 full staged rotation: durable adoption on the node,
// durable commit on the controller, both surviving restarts of both
// sides, old rejected on both sides.
func TestB401_StagedRotationDurableBothSides(t *testing.T) {
	dir := t.TempDir()
	var durable DurableState
	m := NewManager(nil)
	m.SetPersist(func(st DurableState) (CommitResult, error) {
		durable = st
		return CommitResult{}, nil
	})
	tok, _, _ := m.IssueEnrollmentToken(time.Hour)
	node, old, _ := m.Enroll(tok, NodeInfo{Name: "edge"})
	identity, err := OpenNodeIdentity(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := identity.Adopt(old); err != nil {
		t.Fatal(err)
	}
	// Stage (controller), pull (node, current credential), durable
	// pending BEFORE confirm.
	next, _, _ := m.StageRotation(node.ID)
	if _, _, err := m.PullStagedRotation(node.ID, old); err != nil {
		t.Fatal(err)
	}
	if err := identity.BeginTransition(next); err != nil {
		t.Fatalf("durable pending failed: %v", err)
	}
	// Confirm (controller commit; old dies here).
	if st, err := m.ConfirmStagedRotation(node.ID, next); err != nil || st != RotationConfirmed {
		t.Fatalf("confirm: %s %v", st, err)
	}
	// Local finalization after proven controller commit.
	if err := identity.Commit(next); err != nil {
		t.Fatal(err)
	}
	// Restart BOTH sides from durable state.
	restartedController := NewManager(nil)
	restartedController.LoadState(durable)
	restartedNode, err := OpenNodeIdentity(dir)
	if err != nil {
		t.Fatal(err)
	}
	if restartedController.Authenticate(node.ID, old) {
		t.Fatal("B4-01 DEFECT: old credential accepted by restarted controller")
	}
	if !restartedController.Authenticate(node.ID, next) {
		t.Fatal("new credential lost by restarted controller")
	}
	if restartedNode.Accepts(old) {
		t.Fatal("B4-01 DEFECT: old credential accepted by restarted node")
	}
	if !restartedNode.Accepts(next) {
		t.Fatal("new credential lost by restarted node")
	}
}

// B4-02: confirm retry after a lost ack is idempotent (durable
// tombstone), including after a controller restart.
func TestB402_ConfirmIdempotentAndRestartSafe(t *testing.T) {
	dir := t.TempDir()
	var durable DurableState
	m := NewManager(nil)
	m.SetPersist(func(st DurableState) (CommitResult, error) {
		durable = st
		return CommitResult{}, nil
	})
	tok, _, _ := m.IssueEnrollmentToken(time.Hour)
	node, _, _ := m.Enroll(tok, NodeInfo{Name: "edge"})
	identity, _ := OpenNodeIdentity(dir)
	_ = identity.Adopt("current")
	next, _, _ := m.StageRotation(node.ID)
	_, _, _ = m.PullStagedRotation(node.ID, "current")
	_ = identity.BeginTransition(next)
	if st, err := m.ConfirmStagedRotation(node.ID, next); err != nil || st != RotationConfirmed {
		t.Fatalf("first confirm: %s %v", st, err)
	}
	// Lost-ack retry on the SAME manager.
	if st, err := m.ConfirmStagedRotation(node.ID, next); err != nil || st != RotationConfirmed {
		t.Fatalf("B4-02 DEFECT: lost-ack retry: %s %v", st, err)
	}
	// Controller restart: the tombstone survives via the durable
	// snapshot; the retry still returns CONFIRMED.
	restarted := NewManager(nil)
	restarted.LoadState(durable)
	if st, err := restarted.ConfirmStagedRotation(node.ID, next); err != nil || st != RotationConfirmed {
		t.Fatalf("B4-02 DEFECT: retry after controller restart: %s %v", st, err)
	}
	// Wrong credential on the tombstone is refused.
	if _, err := restarted.ConfirmStagedRotation(node.ID, "wrong"); err == nil {
		t.Fatal("wrong credential accepted on tombstone replay")
	}
	// Status reports CONFIRMED from the tombstone.
	if restarted.RotationStatus(node.ID) != RotationConfirmed {
		t.Fatal("status lost the confirmed tombstone")
	}
}

// B4-02: revocation during/after staged rotation kills the node
// credential regardless of tombstones.
func TestB402_RevocationDuringRotation(t *testing.T) {
	m := NewManager(nil)
	m.SetPersist(func(DurableState) (CommitResult, error) { return CommitResult{}, nil })
	tok, _, _ := m.IssueEnrollmentToken(time.Hour)
	node, old, _ := m.Enroll(tok, NodeInfo{Name: "edge"})
	if _, _, err := m.StageRotation(node.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := m.PullStagedRotation(node.ID, old); err != nil {
		t.Fatal(err)
	}
	if err := m.Revoke(node.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := m.ConfirmStagedRotation(node.ID, "whatever"); err == nil {
		t.Fatal("confirm allowed on revoked node")
	}
	if m.Authenticate(node.ID, old) {
		t.Fatal("revoked node still authenticates")
	}
}

// Staged rotation cancel: safe no-op abort; nothing committed.
func TestB402_CancelLeavesOldValid(t *testing.T) {
	m := NewManager(nil)
	tok, _, _ := m.IssueEnrollmentToken(time.Hour)
	node, old, _ := m.Enroll(tok, NodeInfo{Name: "edge"})
	if _, _, err := m.StageRotation(node.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := m.PullStagedRotation(node.ID, old); err != nil {
		t.Fatal(err)
	}
	if err := m.CancelStagedRotation(node.ID); err != nil {
		t.Fatal(err)
	}
	if !m.Authenticate(node.ID, old) {
		t.Fatal("cancel invalidated the old credential")
	}
	if m.RotationStatus(node.ID) != RotationNone {
		t.Fatal("cancel left rotation state")
	}
}
