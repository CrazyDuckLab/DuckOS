// scheduler/scheduler_test.go pins the V1
// scheduler contract.
package scheduler

import (
	"testing"
	"time"
)

func TestScheduleValid(t *testing.T) {
	if err := (Schedule{}).Valid(); err == nil {
		t.Fatal("empty schedule must fail")
	}
	if err := (Schedule{Cron: "* * * * *"}).Valid(); err != nil {
		t.Fatal(err)
	}
	if err := (Schedule{Cron: "too few"}).Valid(); err == nil {
		t.Fatal("cron with wrong field count must fail")
	}
}

func TestJobValid(t *testing.T) {
	good := Job{ID: "j1", Kind: JobFirewallRule, Schedule: Schedule{Cron: "* * * * *"}, Operator: "admin", Enabled: true, CreatedAt: time.Now()}
	if err := good.Valid(); err != nil {
		t.Fatal(err)
	}
	bad := Job{ID: "j1", Kind: "garbage", Schedule: Schedule{Cron: "* * * * *"}, Operator: "admin", CreatedAt: time.Now()}
	if err := bad.Valid(); err == nil {
		t.Fatal("unknown kind must fail")
	}
	noOp := Job{ID: "j1", Kind: JobFirewallRule, Schedule: Schedule{Cron: "* * * * *"}}
	if err := noOp.Valid(); err == nil {
		t.Fatal("missing operator must fail")
	}
}

func TestManagerCRUD(t *testing.T) {
	m := NewManager(time.Now)
	if err := m.Add(Job{ID: "j1", Kind: JobFirewallRule, Schedule: Schedule{Cron: "* * * * *"}, Operator: "admin", Enabled: true, CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if err := m.Add(Job{ID: "j1", Kind: JobFirewallRule, Schedule: Schedule{Cron: "0 * * * *"}, Operator: "admin", Enabled: true, CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if _, ok := m.Get("j1"); !ok {
		t.Fatal("get failed")
	}
	if err := m.Disable("j1"); err != nil {
		t.Fatal(err)
	}
	if err := m.Remove("j1"); err != nil {
		t.Fatal(err)
	}
	if err := m.Remove("j1"); err == nil {
		t.Fatal("remove of unknown must fail")
	}
}

func TestManagerTickCron(t *testing.T) {
	now := time.Date(2025, 1, 1, 12, 30, 0, 0, time.UTC)
	m := NewManager(func() time.Time { return now })
	if err := m.Add(Job{ID: "every-minute", Kind: JobFirewallRule, Schedule: Schedule{Cron: "* * * * *"}, Operator: "admin", Enabled: true, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := m.Add(Job{ID: "specific", Kind: JobFirewallRule, Schedule: Schedule{Cron: "30 12 1 1 *"}, Operator: "admin", Enabled: true, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := m.Add(Job{ID: "different-hour", Kind: JobFirewallRule, Schedule: Schedule{Cron: "0 1 * * *"}, Operator: "admin", Enabled: true, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	r := m.Tick(now)
	if len(r.Due) != 2 {
		t.Fatalf("due: %v", r.Due)
	}
}

func TestManagerTickDisabled(t *testing.T) {
	now := time.Now()
	m := NewManager(func() time.Time { return now })
	if err := m.Add(Job{ID: "j1", Kind: JobFirewallRule, Schedule: Schedule{Cron: "* * * * *"}, Operator: "admin", Enabled: false, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	r := m.Tick(now)
	if len(r.Due) != 0 || len(r.Disabled) != 1 {
		t.Fatalf("expected disabled: %+v", r)
	}
}

func TestManagerTickExpired(t *testing.T) {
	now := time.Now()
	m := NewManager(func() time.Time { return now })
	if err := m.Add(Job{ID: "j1", Kind: JobFirewallRule, Schedule: Schedule{Cron: "* * * * *"}, Operator: "admin", Enabled: true, CreatedAt: now.Add(-2 * time.Hour), ExpiresAt: now.Add(-time.Hour)}); err != nil {
		t.Fatal(err)
	}
	r := m.Tick(now)
	if len(r.Expired) != 1 {
		t.Fatalf("expected expired: %+v", r)
	}
}
