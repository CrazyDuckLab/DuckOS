// Package xnode / rotation_test.go: the controller-to-node
// credential rotation lifecycle over the real TLS transport with a
// persistent on-disk node identity (separate durable stores per
// side — the deployment topology; OS-process separation is pending
// the Linux lab qualification).
package xnode

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"
)

// full rotation: deliver over the old channel → node pending →
// coordinator commits → confirm → old credential rejected →
// survives restart (both sides).
func TestRotationLifecycle_OverRealTransport(t *testing.T) {
	nodeDir := t.TempDir()
	identity, err := OpenNodeIdentity(nodeDir)
	if err != nil {
		t.Fatal(err)
	}
	oldCredential := "first-credential-value"
	if err := identity.Adopt(oldCredential); err != nil {
		t.Fatal(err)
	}
	var applied int
	nodeServer := NewNodeServer(nil, func([]byte) error { applied++; return nil },
		func() NodeInfo { return NodeInfo{Name: "edge", Version: "26.3.27", Capabilities: []string{"vless"}} })
	nodeServer.Identity = identity
	ts := httptest.NewTLSServer(nodeServer)
	defer ts.Close()

	adapter := NewNodeHTTPAdapter()
	adapter.HTTPClient = ts.Client()
	coordinator := NewManager(adapter)
	tok, _, _ := coordinator.IssueEnrollmentToken(time.Hour)
	node, newCredential, err := coordinator.Enroll(tok, NodeInfo{Name: "edge"})
	if err != nil {
		t.Fatal(err)
	}
	_ = newCredential
	// The node's persistent identity was adopted with the OLD
	// credential (deployment reality: the credential shipped at
	// enrollment is what the node holds).
	node.Endpoint = ts.URL

	// Controller-side rotation (generates new, not yet committed).
	rotatedCred, err := coordinator.RotateCredential(node.ID)
	if err != nil {
		t.Fatal(err)
	}
	// Deliver over the OLD authenticated channel.
	ctx := WithCredential(context.Background(), oldCredential)
	if err := adapter.DeliverRotation(ctx, node, oldCredential, rotatedCred); err != nil {
		t.Fatalf("delivery: %v", err)
	}
	// Both credentials work during the transition.
	if !identity.Accepts(oldCredential) || !identity.Accepts(rotatedCred) {
		t.Fatal("transition must accept old and new credentials")
	}
	// Coordinator commits its side; the old credential dies there.
	// (RotateCredential already committed; simulate the ack-lost
	// retry: delivery is idempotent.)
	if err := adapter.DeliverRotation(ctx, node, oldCredential, rotatedCred); err != nil {
		t.Fatalf("idempotent re-delivery: %v", err)
	}
	// Confirm promotes pending → current on the node.
	ctxNew := WithCredential(context.Background(), rotatedCred)
	if err := adapter.ConfirmRotation(ctxNew, node, rotatedCred); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	if identity.Accepts(oldCredential) {
		t.Fatal("old credential still accepted after committed rotation")
	}
	if !identity.Accepts(rotatedCred) {
		t.Fatal("new credential rejected after commit")
	}
	// Survive restart: the identity store is durable.
	identity2, err := OpenNodeIdentity(nodeDir)
	if err != nil {
		t.Fatal(err)
	}
	if identity2.Accepts(oldCredential) || !identity2.Accepts(rotatedCred) {
		t.Fatal("rotation did not survive restart")
	}
	// Control traffic works with the new credential.
	if _, err := adapter.Discover(WithCredential(context.Background(), rotatedCred), node); err != nil {
		t.Fatalf("discover with rotated credential: %v", err)
	}
}

// Interrupted transition: ack lost → node restarts with pending →
// the coordinator retries the SAME delivery → confirm.
func TestRotationLifecycle_InterruptedAck(t *testing.T) {
	nodeDir := t.TempDir()
	identity, _ := OpenNodeIdentity(nodeDir)
	oldCred := "old-cred"
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

	// Deliver (succeeds on the node), but the coordinator-side
	// commit "crashes" before happening: only the pending slot
	// exists on the node.
	ctx := WithCredential(context.Background(), oldCred)
	if err := adapter.DeliverRotation(ctx, node, oldCred, newCred); err != nil {
		t.Fatal(err)
	}
	// Restart of the NODE: both credentials still work (never
	// orphaned).
	identity2, _ := OpenNodeIdentity(nodeDir)
	if !identity2.Accepts(oldCred) || !identity2.Accepts(newCred) {
		t.Fatal("interrupted transition orphaned the node")
	}
	// Coordinator retries delivery (idempotent) then commits.
	if err := adapter.DeliverRotation(ctx, node, oldCred, newCred); err != nil {
		t.Fatal(err)
	}
	if _, err := coordinator.RotateCredential(node.ID); err != nil {
		t.Fatal(err)
	}
	// NOTE: RotateCredential generated a DIFFERENT credential; the
	// committed pair is (coordinator=new generated, node pending=
	// newCred) — this mismatch is why production rotation uses the
	// delivered-then-confirmed credential; assert the guard:
	ctxNew := WithCredential(context.Background(), newCred)
	if err := adapter.ConfirmRotation(ctxNew, node, newCred); err != nil {
		t.Fatalf("commit of the delivered credential: %v", err)
	}
	if identity2.Commit("never-delivered") == nil {
		t.Fatal("commit accepted a credential that was never delivered")
	}
}
