// Package xnode / b4reaudit2_test.go: regressions for the second
// independent re-audit (2026-10-23) — durable tombstone atomicity,
// confirmation replay versus node lifecycle, status precedence and
// live runtime adoption of the durable identity store.
package xnode

import (
	"context"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/persist"
)

func b4rDurableRegistry(t *testing.T) (*Manager, *persist.File[DurableState]) {
	t.Helper()
	d, err := persist.Open[DurableState](filepath.Join(t.TempDir(), "registry"), "state.json")
	if err != nil {
		t.Fatal(err)
	}
	m := NewManager(nil)
	m.SetPersist(func(st DurableState) (CommitResult, error) {
		out, err := d.Update(func(p *DurableState) { *p = st })
		return CommitResult{Uncertain: out == persist.CommitUncertain}, err
	})
	return m, d
}

func b4rEnrollAndStage(t *testing.T, m *Manager) (string, string, string) {
	t.Helper()
	tok, _, err := m.IssueEnrollmentToken(time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	n, old, err := m.Enroll(tok, NodeInfo{Name: "b4r-node"})
	if err != nil {
		t.Fatal(err)
	}
	next, _, err := m.StageRotation(n.ID)
	if err != nil {
		t.Fatal(err)
	}
	return n.ID, old, next
}

// B4-02 (re-audit): the credential hash and the confirmation
// tombstone must commit as ONE durable snapshot — a physical reopen
// of the registry file must still allow the idempotent confirm
// replay (a tombstone written after the snapshot is lost).
func TestB402_TombstoneAtomicWithRegistryOnPhysicalReopen(t *testing.T) {
	dir := t.TempDir()
	disk, err := persist.Open[DurableState](dir, "state.json")
	if err != nil {
		t.Fatal(err)
	}
	m := NewManager(nil)
	m.SetPersist(func(st DurableState) (CommitResult, error) {
		out, err := disk.Update(func(p *DurableState) { *p = st })
		return CommitResult{Uncertain: out == persist.CommitUncertain}, err
	})
	id, old, next := b4rEnrollAndStage(t, m)
	if _, err := m.ConfirmStagedRotation(id, next); err != nil {
		t.Fatal(err)
	}
	reopened, err := persist.Open[DurableState](dir, "state.json")
	if err != nil {
		t.Fatal(err)
	}
	fresh := NewManager(nil)
	fresh.LoadState(reopened.Get())
	if fresh.Authenticate(id, old) || !fresh.Authenticate(id, next) {
		t.Fatal("registry snapshot incorrect after physical reopen")
	}
	if _, err := fresh.ConfirmStagedRotation(id, next); err != nil {
		t.Fatalf("durable credential committed but tombstone missing on physical reopen: %v", err)
	}
}

// B4-02 (re-audit): confirmation replay must never report success
// for a revoked identity — the lifecycle gate runs BEFORE the
// completed-rotation replay.
func TestB402_RevokedReplayRejectedAfterRestart(t *testing.T) {
	m, d := b4rDurableRegistry(t)
	id, _, next := b4rEnrollAndStage(t, m)
	if _, err := m.ConfirmStagedRotation(id, next); err != nil {
		t.Fatal(err)
	}
	if err := m.Revoke(id); err != nil {
		t.Fatal(err)
	}
	fresh := NewManager(nil)
	fresh.LoadState(d.Get())
	if fresh.Authenticate(id, next) {
		t.Fatal("revoked authentication resurrected")
	}
	if state, err := fresh.ConfirmStagedRotation(id, next); err == nil {
		t.Fatalf("revoked node received successful replay %s after restart", state)
	}
}

// B4-02 (re-audit): a suspended node cannot complete a rotation.
func TestB402_SuspendedNodeCannotConfirm(t *testing.T) {
	m, _ := b4rDurableRegistry(t)
	id, _, next := b4rEnrollAndStage(t, m)
	if err := m.Suspend(id); err != nil {
		t.Fatal(err)
	}
	if state, err := m.ConfirmStagedRotation(id, next); err == nil {
		t.Fatalf("suspended node confirmed: %s", state)
	}
}

// B4-02 (re-audit): a freshly staged rotation must report PENDING —
// the previous rotation's tombstone must not mask it.
func TestB402_NewRotationStatusNotMaskedByTombstone(t *testing.T) {
	m, _ := b4rDurableRegistry(t)
	id, _, next := b4rEnrollAndStage(t, m)
	if _, err := m.ConfirmStagedRotation(id, next); err != nil {
		t.Fatal(err)
	}
	if _, _, err := m.StageRotation(id); err != nil {
		t.Fatal(err)
	}
	if st := m.RotationStatus(id); st != RotationPending {
		t.Fatalf("new unconfirmed rotation reports %s", st)
	}
}

// Tombstones stay bounded: one entry per node, replaced per
// rotation.
func TestB402_TombstonesBoundedPerNode(t *testing.T) {
	m := NewManager(nil)
	for i := 0; i < MaxNodes; i++ {
		id, _, next := b4rEnrollAndStage(t, m)
		if _, err := m.ConfirmStagedRotation(id, next); err != nil {
			t.Fatal(err)
		}
		for j := 0; j < 5; j++ {
			n2, _, err := m.StageRotation(id)
			if err != nil {
				t.Fatal(err)
			}
			next = n2
			if _, err := m.ConfirmStagedRotation(id, next); err != nil {
				t.Fatal(err)
			}
		}
	}
	if len(m.completed) != MaxNodes {
		t.Fatalf("unexpected tombstone count %d", len(m.completed))
	}
}

// B4-01 live-node coordination: a RUNNING NodeServer adopts the
// durable identity store per request — after another process
// commits the rotation into the store, the live server must accept
// the new credential and reject the old one WITHOUT a restart.
func TestB401_NodeServerAdoptsRotationFromDurableStore(t *testing.T) {
	dir := t.TempDir()
	live, err := OpenNodeIdentity(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := live.Adopt("b4r-live-old"); err != nil {
		t.Fatal(err)
	}
	server := NewNodeServer(nil, func([]byte) error { return nil },
		func() NodeInfo { return NodeInfo{Name: "live"} })
	server.Identity = live
	tlsNode := httptest.NewTLSServer(server)
	defer tlsNode.Close()

	// Another process (the duck CLI) performs the rotation against
	// the same durable store through its own instance.
	cli, err := OpenNodeIdentity(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := cli.BeginTransition("b4r-live-new"); err != nil {
		t.Fatal(err)
	}
	node := Node{ID: "b4r", Endpoint: tlsNode.URL}
	adapter := NewNodeHTTPAdapter()
	adapter.HTTPClient = tlsNode.Client()
	// During the transition both credentials work on the live node.
	if _, err := adapter.Discover(WithCredential(context.Background(), "b4r-live-old"), node); err != nil {
		t.Fatalf("old credential rejected during transition: %v", err)
	}
	if _, err := adapter.Discover(WithCredential(context.Background(), "b4r-live-new"), node); err != nil {
		t.Fatalf("new credential not adopted by the live node: %v", err)
	}
	if err := cli.Commit("b4r-live-new"); err != nil {
		t.Fatal(err)
	}
	// After the durable commit the OLD credential must stop working
	// on the RUNNING server (no restart), the new one must work.
	if _, err := adapter.Discover(WithCredential(context.Background(), "b4r-live-old"), node); err == nil {
		t.Fatal("running node still accepts the old credential after durable commit")
	}
	if _, err := adapter.Discover(WithCredential(context.Background(), "b4r-live-new"), node); err != nil {
		t.Fatalf("running node rejected the new credential after durable commit: %v", err)
	}
}
