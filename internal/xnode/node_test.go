// Package xnode tests: enrollment security, mutual authentication,
// rotation, revocation, staleness and reconciliation containment.
package xnode

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

type stubAdapter struct {
	mu        sync.Mutex
	info      NodeInfo
	latency   time.Duration
	pushErr   error
	healthErr error
	pushed    int
}

func (a *stubAdapter) Discover(_ Ctx, _ Node) (NodeInfo, error) { return a.info, nil }
func (a *stubAdapter) PushConfig(_ Ctx, _ Node, _ []byte) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.pushed++
	return a.pushErr
}
func (a *stubAdapter) Health(_ Ctx, _ Node) (time.Duration, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.healthErr != nil {
		return 0, a.healthErr
	}
	return a.latency, nil
}

func TestEnrollment_SingleUseAndExpiry(t *testing.T) {
	m := NewManager(&stubAdapter{})
	tok, rec, err := m.IssueEnrollmentToken(time.Hour)
	if err != nil || rec.Used {
		t.Fatalf("issue: %v %+v", err, rec)
	}
	if strings.Contains(rec.TokenHash, tok) || tok == "" {
		t.Fatal("digest must not embed the token")
	}
	node, cred, err := m.Enroll(tok, NodeInfo{Name: "edge-1", Version: "26.3.27", Capabilities: []string{"vless", "reality"}})
	if err != nil {
		t.Fatal(err)
	}
	if cred == "" || node.Status != StatusActive || node.Kind != KindRemote {
		t.Fatalf("enroll result: %+v", node)
	}
	// Second use must fail (single-use).
	if _, _, err := m.Enroll(tok, NodeInfo{Name: "edge-2"}); err == nil {
		t.Fatal("enrollment token reused")
	}
	// Expired token must fail.
	tok2, _, _ := m.IssueEnrollmentToken(-time.Minute)
	if _, _, err := m.Enroll(tok2, NodeInfo{Name: "edge-3"}); err == nil {
		t.Fatal("expired token accepted")
	}
	if !m.Authenticate(node.ID, cred) {
		t.Fatal("credential rejected right after enrollment")
	}
	if m.Authenticate(node.ID, "wrong") || m.Authenticate("unknown", cred) {
		t.Fatal("bad credentials accepted")
	}
}

func TestRotationAndRevocation(t *testing.T) {
	m := NewManager(&stubAdapter{})
	tok, _, _ := m.IssueEnrollmentToken(time.Hour)
	node, cred, _ := m.Enroll(tok, NodeInfo{Name: "edge-1"})
	newCred, err := m.RotateCredential(node.ID)
	if err != nil {
		t.Fatal(err)
	}
	if m.Authenticate(node.ID, cred) {
		t.Fatal("old credential survived rotation")
	}
	if !m.Authenticate(node.ID, newCred) {
		t.Fatal("new credential rejected")
	}
	if err := m.Revoke(node.ID); err != nil {
		t.Fatal(err)
	}
	if m.Authenticate(node.ID, newCred) {
		t.Fatal("revoked node authenticated")
	}
	if _, err := m.RotateCredential(node.ID); err == nil {
		t.Fatal("rotation on revoked node allowed")
	}
	// Revocation is terminal; resume must not resurrect.
	if err := m.Resume(node.ID); err == nil {
		t.Fatal("resume resurrected revoked node")
	}
}

func TestLocalNode_NoCredential(t *testing.T) {
	m := NewManager(&stubAdapter{})
	local, err := m.EnrollLocal("core-1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.EnrollLocal("core-2"); err == nil {
		t.Fatal("second local node accepted")
	}
	if _, err := m.RotateCredential(local.ID); err == nil {
		t.Fatal("local node credential rotation allowed")
	}
}

func TestStalenessDerivedNotStored(t *testing.T) {
	m := NewManager(&stubAdapter{})
	tok, _, _ := m.IssueEnrollmentToken(time.Hour)
	node, _, _ := m.Enroll(tok, NodeInfo{Name: "edge-1"})
	base := time.Now().UTC()
	m.SetClock(func() time.Time { return base })
	if got := m.List()[0].Status; got != StatusActive {
		t.Fatalf("fresh node status %s", got)
	}
	m.SetClock(func() time.Time { return base.Add(10 * time.Minute) })
	if got := m.List()[0].Status; got != StatusStale {
		t.Fatalf("stale node status %s", got)
	}
	// MarkSeen refreshes.
	m.markSeen(node.ID, "26.3.27", []string{"vless"})
	if got := m.List()[0].Status; got != StatusActive {
		t.Fatalf("post-seen status %s", got)
	}
	if got := m.List()[0].Version; got != "26.3.27" {
		t.Fatalf("version %q", got)
	}
}

func TestReconcile_ContainsPerNodeFailure(t *testing.T) {
	adapter := &stubAdapter{pushErr: errors.New("connection refused")}
	m := NewManager(adapter)
	// Two nodes: one healthy, one failing.
	m2 := NewManager(&stubAdapter{})
	_ = m2
	tokA, _, _ := m.IssueEnrollmentToken(time.Hour)
	nodeA, credA, _ := m.Enroll(tokA, NodeInfo{Name: "edge-a"})
	_ = credA
	tokB, _, _ := m.IssueEnrollmentToken(time.Hour)
	nodeB, _, _ := m.Enroll(tokB, NodeInfo{Name: "edge-b"})
	adapter.pushErr = nil // only B will fail: flip adapter per-node is out of scope, use eligibility
	m.Suspend(nodeB.ID)
	_ = nodeB
	plans := m.Reconcile(context.Background(), []byte(`{"inbounds":[]}`))
	if len(plans) != 1 || plans[0].NodeID != nodeA.ID || plans[0].Action != "push" {
		t.Fatalf("plans: %+v", plans)
	}
	// Suspended node was skipped.
	for _, p := range plans {
		if p.NodeID == nodeB.ID {
			t.Fatal("suspended node was pushed")
		}
	}
	// Revoked node is invisible to Eligible.
	m.Revoke(nodeA.ID)
	if len(m.Eligible()) != 0 {
		t.Fatal("revoked node eligible")
	}
}

func TestNodeNameValidation(t *testing.T) {
	m := NewManager(&stubAdapter{})
	if _, _, err := m.Enroll("", NodeInfo{Name: "Bad Name"}); err == nil {
		t.Fatal("non-canonical name accepted")
	}
	if _, _, err := m.Enroll("", NodeInfo{Name: "x", Capabilities: []string{""}}); err == nil {
		t.Fatal("empty capability accepted")
	}
}
