// maintenance/maintenance_test.go pins the
// V1 maintenance-mode contract.
package maintenance

import (
	"testing"
	"time"
)

func TestSuppressionKindValid(t *testing.T) {
	for _, k := range []SuppressionKind{SupFailoverReaction, SupWatchdogAction, SupAutoRemediation, SupScheduledDisruptive} {
		if !k.Valid() {
			t.Fatalf("%s must be valid", k)
		}
	}
	if SuppressionKind("garbage").Valid() {
		t.Fatal("garbage must be invalid")
	}
}

func TestSessionValid(t *testing.T) {
	now := time.Now()
	good := Session{ID: "m-1", Operator: "admin", Reason: "upgrade", Suppressions: []SuppressionKind{SupWatchdogAction}, CreatedAt: now, ExpiresAt: now.Add(time.Hour)}
	if err := good.Valid(); err != nil {
		t.Fatal(err)
	}
	tooLong := Session{ID: "m-1", Operator: "admin", Reason: "x", Suppressions: []SuppressionKind{SupWatchdogAction}, CreatedAt: now, ExpiresAt: now.Add(48 * time.Hour)}
	if err := tooLong.Valid(); err == nil {
		t.Fatal(">24h window must fail")
	}
	noSupp := Session{ID: "m-1", Operator: "admin", Reason: "x", CreatedAt: now, ExpiresAt: now.Add(time.Hour)}
	if err := noSupp.Valid(); err == nil {
		t.Fatal("no suppressions must fail")
	}
	noOp := Session{ID: "m-1", Reason: "x", Suppressions: []SuppressionKind{SupWatchdogAction}, CreatedAt: now, ExpiresAt: now.Add(time.Hour)}
	if err := noOp.Valid(); err == nil {
		t.Fatal("missing operator must fail")
	}
	bad := Session{ID: "m-1", Operator: "admin", Reason: "x", Suppressions: []SuppressionKind{"garbage"}, CreatedAt: now, ExpiresAt: now.Add(time.Hour)}
	if err := bad.Valid(); err == nil {
		t.Fatal("unknown suppression must fail")
	}
}

func TestManagerOpenCloseActive(t *testing.T) {
	now := time.Now()
	m := NewManager(func() time.Time { return now })
	if _, err := m.Open(Session{ID: "m-1", Operator: "admin", Reason: "x", Suppressions: []SuppressionKind{SupWatchdogAction}, CreatedAt: now, ExpiresAt: now.Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if !m.IsSuppressed(SupWatchdogAction, now.Add(5*time.Minute)) {
		t.Fatal("watchdog must be suppressed")
	}
	if m.IsSuppressed(SupFailoverReaction, now.Add(5*time.Minute)) {
		t.Fatal("failover must not be suppressed")
	}
	if err := m.Close("m-1", "admin", "done"); err != nil {
		t.Fatal(err)
	}
	if m.IsSuppressed(SupWatchdogAction, now.Add(5*time.Minute)) {
		t.Fatal("closed window must not suppress")
	}
}

func TestManagerExpiry(t *testing.T) {
	now := time.Now()
	m := NewManager(func() time.Time { return now })
	if _, err := m.Open(Session{ID: "m-1", Operator: "admin", Reason: "x", Suppressions: []SuppressionKind{SupWatchdogAction}, CreatedAt: now, ExpiresAt: now.Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if m.IsSuppressed(SupWatchdogAction, now.Add(2*time.Hour)) {
		t.Fatal("expired must not suppress")
	}
}

func TestManagerAppendAudit(t *testing.T) {
	now := time.Now()
	m := NewManager(func() time.Time { return now })
	if _, err := m.Open(Session{ID: "m-1", Operator: "admin", Reason: "x", Suppressions: []SuppressionKind{SupWatchdogAction}, CreatedAt: now, ExpiresAt: now.Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if err := m.AppendAudit("m-1", "watchdog-blocked", "iface=eth0"); err != nil {
		t.Fatal(err)
	}
	if err := m.AppendAudit("m-1", "remediation-blocked", "iface=eth0"); err != nil {
		t.Fatal(err)
	}
	active := m.Active(now.Add(5 * time.Minute))
	if len(active) != 1 || len(active[0].AuditTrail) != 2 {
		t.Fatalf("audit: %+v", active)
	}
}
