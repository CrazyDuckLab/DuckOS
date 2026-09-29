package fwanalysis

import (
	"testing"
	"time"
)

func TestFindingValid(t *testing.T) {
	good := Finding{Severity: SevWarning, Code: "duplicate-rule", Object: "r1", Summary: "x"}
	if err := good.Valid(); err != nil {
		t.Fatal(err)
	}
}

func TestAnalyzeRules(t *testing.T) {
	a := NewAnalyzer()
	rules := []FirewallRule{
		{ID: "r1", Chain: "INPUT", Action: "ACCEPT", Source: "10.0.0.0/8"},
		{ID: "r2", Chain: "INPUT", Action: "ACCEPT", Source: "10.0.0.0/8"},
	}
	findings := a.AnalyzeRules(rules)
	if len(findings) != 1 || findings[0].Code != "duplicate-rule" {
		t.Fatalf("findings: %+v", findings)
	}
}

func TestAnalyzeNAT(t *testing.T) {
	a := NewAnalyzer()
	rules := []NATRule{
		{ID: "n1", ExternalPort: 8080, InternalHost: "10.0.0.10", InternalPort: 80},
		{ID: "n2", ExternalPort: 8080, InternalHost: "10.0.0.11", InternalPort: 80},
	}
	findings := a.AnalyzeNAT(rules)
	if len(findings) != 1 || findings[0].Code != "duplicate-dnat" {
		t.Fatalf("findings: %+v", findings)
	}
}

func TestQuarantine(t *testing.T) {
	now := time.Now()
	m := NewQuarantineManager()
	m.Add(Quarantine{Source: "10.0.0.5", Reason: "syn-flood", CreatedAt: now, ExpiresAt: now.Add(time.Hour)})
	if len(m.Active()) != 1 {
		t.Fatal("active size")
	}
	expired := m.SweepExpired(now.Add(2 * time.Hour))
	if len(expired) != 1 {
		t.Fatal("sweep size")
	}
	if len(m.Active()) != 0 {
		t.Fatal("post-sweep size")
	}
}
