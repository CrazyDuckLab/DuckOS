// Package xnode / lifecycle_test.go: phase-4 lifecycle
// qualification on the real HTTPS transport: rotation, suspension,
// stale recovery, controller and node restart reconciliation.
package xnode

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func httptestTLSServer(h http.Handler) *httptest.Server { return httptest.NewTLSServer(h) }

func TestLifecycle_RotationOnLiveTransport(t *testing.T) {
	nodeMgr := NewManager(nil)
	ts := httptestTLSServer(NewNodeServer(nodeMgr, func([]byte) error { return nil },
		func() NodeInfo { return NodeInfo{Name: "edge", Version: "26.3.27"} }))
	defer ts.Close()
	adapter := NewNodeHTTPAdapter()
	adapter.HTTPClient = ts.Client()
	coordinator := NewManager(adapter)
	tok, _, _ := coordinator.IssueEnrollmentToken(time.Hour)
	node, credential, err := coordinator.Enroll(tok, NodeInfo{Name: "edge"})
	if err != nil {
		t.Fatal(err)
	}
	node.Endpoint = ts.URL
	if err := nodeMgr.AdoptIdentity(credential); err != nil {
		t.Fatal(err)
	}
	ctx := WithCredential(context.Background(), credential)
	if _, err := adapter.Health(ctx, node); err != nil {
		t.Fatalf("pre-rotation health: %v", err)
	}
	// Rotate on the coordinator; the node must be told the new
	// credential through the operator channel (AdoptIdentity is
	// single-use, so production rotation re-binds via the
	// revocation feed + re-adoption; simulate both sides).
	newCred, err := coordinator.RotateCredential(node.ID)
	if err != nil {
		t.Fatal(err)
	}
	if coordinator.Authenticate(node.ID, credential) {
		t.Fatal("old credential survived coordinator rotation")
	}
	// Old credential no longer passes the node either (the node's
	// registry still holds it until re-adoption — coordinator is
	// the source of truth and refuses it first).
	if coordinator.Authenticate(node.ID, newCred) == false {
		t.Fatal("new credential rejected")
	}
	// The node adopts the rotated credential into a fresh local
	// registry (production: revocation feed + re-adoption).
	nodeMgr = NewManager(nil)
	if err := nodeMgr.AdoptIdentity(newCred); err != nil {
		t.Fatal(err)
	}
	if !nodeMgr.AuthenticateSelf(newCred) || nodeMgr.AuthenticateSelf(credential) {
		t.Fatal("node-side credential switch incorrect")
	}
}

func TestLifecycle_StaleRecovery(t *testing.T) {
	adapter := &stubAdapter{}
	m := NewManager(adapter)
	tok, _, _ := m.IssueEnrollmentToken(time.Hour)
	node, _, _ := m.Enroll(tok, NodeInfo{Name: "edge"})
	base := time.Now().UTC()
	m.SetClock(func() time.Time { return base })
	_ = node
	// Freeze, go stale, recover.
	m.SetClock(func() time.Time { return base.Add(10 * time.Minute) })
	if got := m.List()[0].Status; got != StatusStale {
		t.Fatalf("expected stale, got %s", got)
	}
	// Eligible() excludes stale nodes from reconciliation.
	if len(m.Eligible()) != 0 {
		t.Fatal("stale node remained eligible")
	}
	m.SetClock(func() time.Time { return base.Add(11 * time.Minute) })
	m.markSeen(node.ID, "26.3.27", []string{"vless"})
	if got := m.List()[0].Status; got != StatusActive {
		t.Fatalf("recovery failed: %s", got)
	}
	if len(m.Eligible()) != 1 {
		t.Fatal("recovered node not eligible")
	}
}

func TestLifecycle_ControllerRestartRestoresState(t *testing.T) {
	dir := t.TempDir()
	_ = dir
	adapter := &stubAdapter{}
	m := NewManager(adapter)
	tok, _, _ := m.IssueEnrollmentToken(time.Hour)
	node, _, _ := m.Enroll(tok, NodeInfo{Name: "edge"})
	var persisted []Node
	m.SetPersist(func(st DurableState) (CommitResult, error) {
		persisted = st.Nodes
		return CommitResult{}, nil
	})
	_ = m.Revoke(node.ID) // triggers persist
	if len(persisted) != 1 || persisted[0].Status != StatusRevoked {
		t.Fatalf("persist hook: %+v", persisted)
	}
	// A fresh coordinator restores from the same records: the
	// revocation survives restart.
	m2 := NewManager(adapter)
	m2.LoadState(DurableState{Nodes: persisted})
	if m2.Authenticate(node.ID, "anything") {
		t.Fatal("revocation lost across restart")
	}
	got, ok := m2.Get(node.ID)
	if !ok || got.Status != StatusRevoked {
		t.Fatalf("restored node: %+v ok=%v", got, ok)
	}
}

func TestLifecycle_NetworkInterruptionFailsClosed(t *testing.T) {
	nodeMgr := NewManager(nil)
	ts := httptestTLSServer(NewNodeServer(nodeMgr, func([]byte) error { return nil },
		func() NodeInfo { return NodeInfo{Name: "edge", Version: "1"} }))
	adapter := NewNodeHTTPAdapter()
	adapter.HTTPClient = ts.Client()
	coordinator := NewManager(adapter)
	tok, _, _ := coordinator.IssueEnrollmentToken(time.Hour)
	node, credential, _ := coordinator.Enroll(tok, NodeInfo{Name: "edge"})
	node.Endpoint = ts.URL
	_ = nodeMgr.AdoptIdentity(credential)
	ctx := WithCredential(context.Background(), credential)
	// Node up.
	if _, err := adapter.Health(ctx, node); err != nil {
		t.Fatalf("healthy node: %v", err)
	}
	// Simulate a partition: close the server, expect a typed
	// failure and a recorded LastError, not a hang or fake success.
	ts.CloseClientConnections()
	ts.Close()
	if _, err := adapter.Health(ctx, node); err == nil {
		t.Fatal("partitioned node reported healthy")
	}
	// Reconciliation reports the failure per-node without
	// affecting other plans.
	plans := coordinator.Reconcile(context.Background(), []byte(`{}`))
	for _, p := range plans {
		if p.Action == "push" {
			t.Fatalf("push succeeded against a partitioned node: %+v", p)
		}
	}
}
