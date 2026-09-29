package secpolicy

import "testing"

func TestPolicyValid(t *testing.T) {
	good := Policy{ID: "no-wan-mgmt", Severity: "error", Description: "x", Enforce: true}
	if err := good.Valid(); err != nil {
		t.Fatal(err)
	}
	bad := Policy{ID: "BAD", Severity: "error"}
	if err := bad.Valid(); err == nil {
		t.Fatal("bad id must fail")
	}
}

func TestPolicyStore(t *testing.T) {
	s := NewPolicyStore()
	s.Set(Policy{ID: "no-wan-mgmt", Severity: "error", Description: "x"})
	s.Override("no-wan-mgmt", "admin")
	if len(s.All()) != 1 {
		t.Fatal("all")
	}
}

func TestCrashManager(t *testing.T) {
	m := NewCrashManager()
	if state := m.Record("xray", 3); state != CrashOK {
		t.Fatalf("first: %s", state)
	}
	if state := m.Record("xray", 3); state != CrashLoop {
		t.Fatalf("second: %s", state)
	}
	m.Record("xray", 3)
	m.Record("xray", 3)
	state := m.Record("xray", 3)
	if state != CrashQuarantine {
		t.Fatalf("fifth: %s", state)
	}
	m.Reset("xray")
	if m.Record("xray", 3) != CrashOK {
		t.Fatal("after reset must be ok")
	}
}

func TestWatchdog(t *testing.T) {
	m := NewWatchdogManager()
	m.Set(WatchdogState{Service: "xray", ProcessUp: true, Functioning: true})
	if len(m.All()) != 1 {
		t.Fatal("all")
	}
}

func TestBudget(t *testing.T) {
	s := NewBudgetStore()
	s.Set(ResourceBudget{Component: "xray", CPUPercent: 80, MemoryPercent: 70, Pressure: "elevated", Warn: true})
	if len(s.All()) != 1 {
		t.Fatal("all")
	}
}

func TestQuota(t *testing.T) {
	s := NewQuotaStore()
	s.Set(Quota{Tenant: "tenant1", ConntrackMax: 10000, SessionsMax: 5000})
	if len(s.All()) != 1 {
		t.Fatal("all")
	}
}

func TestWebhook(t *testing.T) {
	s := NewWebhookStore()
	s.Add(Webhook{ID: "w1", URL: "https://example.com/hook", SecretRef: "vault/webhook/secret"})
	if err := s.Add(Webhook{ID: ""}); err == nil {
		t.Fatal("empty id must fail")
	}
}
