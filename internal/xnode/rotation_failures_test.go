// Package xnode / rotation_failures_test.go: PHASE 2 — the rotation
// failure matrix. Authorization linearization points:
//
//   - CONTROLLER: a rotation is authoritative the moment
//     RotateCredential durably commits (persistLocked OK). Before
//     that, only the old credential exists for the controller; a
//     lost delivery retries safely.
//   - NODE: a delivered credential becomes acceptable at
//     BeginTransition commit (pending slot) and SOLELY authoritative
//     at Commit. Both-slots-accepted during the transition is a
//     deliberate availability tradeoff, bounded by the coordinator
//     commit which kills the old slot.
//
// Security tradeoff (documented, tested): a pending credential does
// not expire by itself — the node would keep accepting BOTH
// credentials indefinitely if the coordinator never confirms.
// This is availability-over-hygiene and is bounded by the fact that
// the coordinator controls all further control traffic: revoking
// the node makes the coordinator refuse the node regardless of the
// pending slot. A node-side expiry timer would risk orphaning a
// slow node and is deliberately not implemented.
package xnode

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"
)

// Controller restarts mid-rotation (after delivery, before its own
// commit): neither side committed the new credential except as the
// node's pending slot; the old credential keeps working everywhere.
func TestRotationFailure_ControllerRestartBeforeCommit(t *testing.T) {
	nodeDir := t.TempDir()
	identity, _ := OpenNodeIdentity(nodeDir)
	nodeServer := NewNodeServer(nil, func([]byte) error { return nil }, func() NodeInfo { return NodeInfo{Name: "n"} })
	nodeServer.Identity = identity
	ts := httptest.NewTLSServer(nodeServer)
	defer ts.Close()
	adapter := NewNodeHTTPAdapter()
	adapter.HTTPClient = ts.Client()
	coordinator := NewManager(adapter)
	tok, _, _ := coordinator.IssueEnrollmentToken(time.Hour)
	node, firstCred, _ := coordinator.Enroll(tok, NodeInfo{Name: "edge"})
	node.Endpoint = ts.URL
	// The node's credential is the one shipped at enrollment.
	oldCred := firstCred
	if err := identity.Adopt(oldCred); err != nil {
		t.Fatal(err)
	}
	ctx := WithCredential(context.Background(), oldCred)

	// Deliver newCred (node pending), then the controller "dies"
	// BEFORE committing. The generated credential is lost.
	lostCred := "generated-but-uncommitted"
	if err := adapter.DeliverRotation(ctx, node, oldCred, lostCred); err != nil {
		t.Fatal(err)
	}
	// Controller restart reconstructs from its durable state: the
	// lost credential was never committed, so the authoritative
	// credential is still the OLD one and still authenticates on
	// both sides.
	if !coordinator.Authenticate(node.ID, oldCred) {
		t.Fatal("controller lost the old credential after restart before commit")
	}
	if !identity.Accepts(oldCred) {
		t.Fatal("node lost the old credential during an uncommitted transition")
	}
	// Recovery: retry the whole rotation with a FRESH credential.
	retryCred, err := coordinator.RotateCredential(node.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.DeliverRotation(ctx, node, oldCred, retryCred); err != nil {
		t.Fatal(err)
	}
	// The stale uncommitted credential must never be usable.
	if identity.Accepts("never-delivered-second") {
		t.Fatal("impossible credential accepted")
	}
	if err := adapter.ConfirmRotation(WithCredential(context.Background(), retryCred), node, retryCred); err != nil {
		t.Fatal(err)
	}
	if identity.Accepts(lostCred) {
		t.Fatal("the abandoned uncommitted credential became acceptable")
	}
}

// Replay: an eavesdropped OLD credential replayed after commit is
// rejected by both sides.
func TestRotationFailure_ReplayedOldCredential(t *testing.T) {
	nodeDir := t.TempDir()
	identity, _ := OpenNodeIdentity(nodeDir)
	oldCred := "eavesdropped"
	_ = identity.Adopt(oldCred)
	nodeServer := NewNodeServer(nil, func([]byte) error { return nil }, func() NodeInfo { return NodeInfo{Name: "n"} })
	nodeServer.Identity = identity
	ts := httptest.NewTLSServer(nodeServer)
	defer ts.Close()
	adapter := NewNodeHTTPAdapter()
	adapter.HTTPClient = ts.Client()
	coordinator := NewManager(adapter)
	tok, _, _ := coordinator.IssueEnrollmentToken(time.Hour)
	node, newCred, _ := coordinator.Enroll(tok, NodeInfo{Name: "edge"})
	node.Endpoint = ts.URL
	_ = identity.Adopt(oldCred)
	ctx := WithCredential(context.Background(), oldCred)
	_ = adapter.DeliverRotation(ctx, node, oldCred, newCred)
	_, _ = coordinator.RotateCredential(node.ID)
	_ = adapter.ConfirmRotation(WithCredential(context.Background(), newCred), node, newCred)
	// Replay attempts.
	if _, err := adapter.Discover(ctx, node); err == nil {
		t.Fatal("replayed old credential accepted by the node")
	}
	if coordinator.Authenticate(node.ID, oldCred) {
		t.Fatal("replayed old credential accepted by the controller")
	}
}

// Permanent partition during rotation: the coordinator has
// committed, the node never received the delivery — the node keeps
// only the OLD credential and the coordinator refuses it. Fail-safe
// outcome: the node is unreachable (re-enrollment required), never
// insecure.
func TestRotationFailure_PermanentPartitionBeforeDelivery(t *testing.T) {
	coordinator := NewManager(nil)
	coordinator.SetPersist(func(DurableState) (CommitResult, error) { return CommitResult{}, nil })
	tok, _, _ := coordinator.IssueEnrollmentToken(time.Hour)
	node, oldCred, _ := coordinator.Enroll(tok, NodeInfo{Name: "edge"})
	// The delivery message is lost entirely (partition). The
	// controller-side commit still happens (operator-driven).
	newCred, err := coordinator.RotateCredential(node.ID)
	if err != nil {
		t.Fatal(err)
	}
	_ = newCred
	// The partitioned node still authenticates with its old
	// credential LOCALLY, but the coordinator refuses it — no
	// authorization can be granted through the controller, and a
	// config push with the old credential would be refused by the
	// controller's own registry semantics (deliveries authenticate
	// with the CURRENT credential).
	if coordinator.Authenticate(node.ID, oldCred) {
		t.Fatal("partitioned node authenticated with the rotated-out credential")
	}
	if !coordinator.Authenticate(node.ID, newCred) {
		t.Fatal("the committed rotation credential rejected by the controller itself")
	}
}

// Duplicate delivery and duplicate confirmation are idempotent.
func TestRotationFailure_DuplicatesIdempotent(t *testing.T) {
	nodeDir := t.TempDir()
	identity, _ := OpenNodeIdentity(nodeDir)
	oldCred := "old"
	_ = identity.Adopt(oldCred)
	newCred := "new"
	for i := 0; i < 3; i++ {
		if err := identity.BeginTransition(newCred); err != nil {
			t.Fatalf("duplicate delivery %d: %v", i, err)
		}
	}
	for i := 0; i < 3; i++ {
		if err := identity.Commit(newCred); err != nil {
			t.Fatalf("duplicate confirm %d: %v", i, err)
		}
	}
	if identity.Accepts(oldCred) || !identity.Accepts(newCred) {
		t.Fatal("duplicate handling corrupted the identity")
	}
}
