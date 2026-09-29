package provisioning

import (
	"bytes"
	"testing"
	"time"
)

func TestAuditCredentialScopeAndNodeBinding(t *testing.T) {
	s := NewStore()
	if err := s.SetProductionKey(bytes.Repeat([]byte{1}, 32), "test-key"); err != nil {
		t.Fatal(err)
	}
	sec, err := s.Issue(ScopeNodeEnrol, "node-001", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Consume(sec.ID, sec.Plaintext, "attacker"); err == nil {
		t.Fatal("legacy API bypassed binding")
	}
	if err = s.ConsumeFor(sec.ID, sec.Plaintext, ScopeNodeEnrol, "node-002", "actor"); err == nil {
		t.Fatal("wrong node accepted")
	}
	if err = s.ConsumeFor(sec.ID, sec.Plaintext, ScopeServiceBind, "node-001", "actor"); err == nil {
		t.Fatal("wrong scope accepted")
	}
	if err = s.ConsumeFor(sec.ID, sec.Plaintext, ScopeNodeEnrol, "node-001", "actor"); err != nil {
		t.Fatal(err)
	}
	if err = s.ConsumeFor(sec.ID, sec.Plaintext, ScopeNodeEnrol, "node-001", "actor"); err == nil {
		t.Fatal("replay accepted")
	}
}
