// accesslifecycle/accesslifecycle_test.go
// pins the V1 access-lifecycle contract.
package accesslifecycle

import (
	"testing"
	"time"
)

func TestSubjectKindValid(t *testing.T) {
	for _, k := range []SubjectKind{SubjWireGuard, SubjXrayUser, SubjFirewallRule, SubjPublish, SubjGrant} {
		if !k.Valid() {
			t.Fatalf("%s must be valid", k)
		}
	}
	if SubjectKind("garbage").Valid() {
		t.Fatal("garbage must be invalid")
	}
}

func TestSubjectValid(t *testing.T) {
	now := time.Now()
	good := Subject{ID: "wg-1", Kind: SubjWireGuard, CreatedAt: now}
	if err := good.Valid(); err != nil {
		t.Fatal(err)
	}
	bad := Subject{ID: "wg-1", Kind: "garbage", CreatedAt: now}
	if err := bad.Valid(); err == nil {
		t.Fatal("unknown kind must fail")
	}
	noCreate := Subject{ID: "wg-1", Kind: SubjWireGuard}
	if err := noCreate.Valid(); err == nil {
		t.Fatal("missing createdAt must fail")
	}
	expBefore := Subject{ID: "wg-1", Kind: SubjWireGuard, CreatedAt: now, ExpiresAt: now.Add(-time.Hour)}
	if err := expBefore.Valid(); err == nil {
		t.Fatal("expiresAt before createdAt must fail")
	}
	revBefore := Subject{ID: "wg-1", Kind: SubjWireGuard, CreatedAt: now, RevokedAt: now.Add(-time.Hour)}
	if err := revBefore.Valid(); err == nil {
		t.Fatal("revokedAt before createdAt must fail")
	}
}

func TestSubjectActive(t *testing.T) {
	now := time.Now()
	active := Subject{ID: "x", Kind: SubjWireGuard, CreatedAt: now, ExpiresAt: now.Add(time.Hour)}
	if !active.Active(now.Add(5 * time.Minute)) {
		t.Fatal("must be active before expiry")
	}
	if active.Active(now.Add(2 * time.Hour)) {
		t.Fatal("must be inactive after expiry")
	}
	revoked := Subject{ID: "x", Kind: SubjWireGuard, CreatedAt: now, RevokedAt: now.Add(time.Minute)}
	if revoked.Active(now.Add(2 * time.Minute)) {
		t.Fatal("must be inactive after revoke")
	}
}

func TestManagerAdd(t *testing.T) {
	now := time.Now()
	m := NewManager(func() time.Time { return now })
	if err := m.Add(Subject{ID: "wg-1", Kind: SubjWireGuard, Owner: "admin", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := m.Add(Subject{ID: "wg-1", Kind: SubjWireGuard, Owner: "admin", CreatedAt: now}); err == nil {
		t.Fatal("duplicate must fail")
	}
	if err := m.Add(Subject{ID: "wg-1", Kind: "garbage", Owner: "admin", CreatedAt: now}); err == nil {
		t.Fatal("invalid kind must fail")
	}
}

func TestManagerRotateRevoke(t *testing.T) {
	now := time.Now()
	m := NewManager(func() time.Time { return now })
	m.Add(Subject{ID: "x", Kind: SubjWireGuard, CreatedAt: now})
	if err := m.Rotate("x", "admin"); err != nil {
		t.Fatal(err)
	}
	if err := m.Revoke("x", "admin", "user-left"); err != nil {
		t.Fatal(err)
	}
	if err := m.Revoke("x", "admin", "again"); err == nil {
		t.Fatal("double revoke must fail")
	}
}

func TestManagerExpireSweep(t *testing.T) {
	now := time.Now()
	m := NewManager(func() time.Time { return now })
	m.Add(Subject{ID: "x", Kind: SubjWireGuard, CreatedAt: now.Add(-2 * time.Hour), ExpiresAt: now.Add(-time.Hour)})
	m.Add(Subject{ID: "y", Kind: SubjXrayUser, CreatedAt: now})
	expired := m.ExpireSweep(now)
	if len(expired) != 1 || expired[0] != "x" {
		t.Fatalf("expired: %v", expired)
	}
}

func TestManagerActiveAt(t *testing.T) {
	now := time.Now()
	m := NewManager(func() time.Time { return now })
	m.Add(Subject{ID: "a", Kind: SubjWireGuard, CreatedAt: now})
	m.Add(Subject{ID: "b", Kind: SubjXrayUser, CreatedAt: now, ExpiresAt: now.Add(-time.Hour)})
	active := m.ActiveAt(now)
	if len(active) != 1 || active[0].ID != "a" {
		t.Fatalf("active: %+v", active)
	}
}

func TestManagerHistory(t *testing.T) {
	now := time.Now()
	m := NewManager(func() time.Time { return now })
	m.Add(Subject{ID: "x", Kind: SubjWireGuard, CreatedAt: now})
	m.Rotate("x", "admin")
	m.Revoke("x", "admin", "done")
	h := m.History("x")
	if len(h) != 3 {
		t.Fatalf("history size: %d", len(h))
	}
	if h[0].Action != "revoked" {
		t.Fatalf("newest first: %+v", h)
	}
}
