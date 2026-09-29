// G21 per-object authorization torture regression.
package identity

import (
	"errors"
	"sync"
	"testing"
)

func sampleKey() ObjectKey {
	return ObjectKey{Kind: "vlan", ID: "vlan-10"}
}

func TestObjectKey_Valid(t *testing.T) {
	for _, k := range []ObjectKey{
		{Kind: "vlan", ID: "vlan-10"},
		{Kind: "xray-user", ID: "edge-a"},
		{Kind: "wireguard-peer", ID: "laptop"},
	} {
		if !k.Valid() {
			t.Errorf("%v must be valid", k)
		}
	}
	for _, k := range []ObjectKey{
		{Kind: "", ID: "x"},
		{Kind: "Vlan", ID: "x"}, // capital
		{Kind: "vlan", ID: ""},
		{Kind: "vlan", ID: "x y"}, // space
	} {
		if k.Valid() {
			t.Errorf("%v must be invalid", k)
		}
	}
}

func TestGrant_Valid(t *testing.T) {
	g := Grant{Role: Owner, Object: sampleKey(), Permission: Observe, Decision: DecisionAllow, Reason: "rbac override"}
	if err := g.Valid(); err != nil {
		t.Fatalf("valid grant: %v", err)
	}
	// Reason is required when decision != default.
	if err := (Grant{Role: Owner, Object: sampleKey(), Permission: Observe, Decision: DecisionAllow}).Valid(); err == nil {
		t.Fatalf("non-default grant without reason must be rejected")
	}
	// DecisionDefault does not need a reason.
	if err := (Grant{Role: Owner, Object: sampleKey(), Permission: Observe, Decision: DecisionDefault}).Valid(); err != nil {
		t.Fatalf("default grant must be valid: %v", err)
	}
}

func TestGrant_RejectsInvalidRole(t *testing.T) {
	if err := (Grant{Role: Role("bogus"), Object: sampleKey(), Permission: Observe}).Valid(); err == nil {
		t.Fatalf("invalid role must be rejected")
	}
}

func TestGrant_RejectsInvalidDecision(t *testing.T) {
	if err := (Grant{Role: Owner, Object: sampleKey(), Permission: Observe, Decision: Decision(99), Reason: "x"}).Valid(); err == nil {
		t.Fatalf("invalid decision must be rejected")
	}
}

func TestPolicy_SetAndLookup(t *testing.T) {
	p := NewPolicy()
	if err := p.Set(Grant{Role: Helpdesk, Object: sampleKey(), Permission: ConfigureNetwork, Decision: DecisionAllow, Reason: "approved"}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if p.Lookup(Helpdesk, sampleKey(), ConfigureNetwork) != DecisionAllow {
		t.Fatalf("Lookup must return DecisionAllow")
	}
	if p.Lookup(Owner, sampleKey(), ConfigureNetwork) != DecisionDefault {
		t.Fatalf("missing entry must be DecisionDefault")
	}
}

func TestPolicy_SetRejectsInvalid(t *testing.T) {
	p := NewPolicy()
	if err := p.Set(Grant{Role: Role("bogus"), Object: sampleKey(), Permission: Observe}); err == nil {
		t.Fatalf("invalid role must be rejected")
	}
	if err := p.Set(Grant{Role: Owner, Object: sampleKey(), Permission: Observe, Decision: DecisionAllow}); err == nil {
		t.Fatalf("missing reason must be rejected")
	}
}

func TestPolicy_Delete(t *testing.T) {
	p := NewPolicy()
	g := Grant{Role: Helpdesk, Object: sampleKey(), Permission: ConfigureNetwork, Decision: DecisionAllow, Reason: "x"}
	_ = p.Set(g)
	p.Delete(Helpdesk, sampleKey(), ConfigureNetwork)
	if p.Lookup(Helpdesk, sampleKey(), ConfigureNetwork) != DecisionDefault {
		t.Fatalf("Delete must remove the entry")
	}
	// Idempotent
	p.Delete(Helpdesk, sampleKey(), ConfigureNetwork)
}

func TestPolicy_AllIsSorted(t *testing.T) {
	p := NewPolicy()
	_ = p.Set(Grant{Role: Helpdesk, Object: ObjectKey{Kind: "vlan", ID: "vlan-2"}, Permission: Observe, Decision: DecisionAllow, Reason: "r"})
	_ = p.Set(Grant{Role: Owner, Object: ObjectKey{Kind: "vlan", ID: "vlan-10"}, Permission: Observe, Decision: DecisionAllow, Reason: "r"})
	_ = p.Set(Grant{Role: NetworkAdmin, Object: ObjectKey{Kind: "xray-user", ID: "edge"}, Permission: ManageGuests, Decision: DecisionDeny, Reason: "r"})
	all := p.All()
	if len(all) != 3 {
		t.Fatalf("expected 3 grants, got %d", len(all))
	}
	if all[0].Role != Helpdesk {
		t.Fatalf("first must be Helpdesk (alpha), got %q", all[0].Role)
	}
}

func TestPolicy_AuthorizeObject_AllowOverlay(t *testing.T) {
	p := NewPolicy()
	// Reception can normally Observe but not ConfigureNetwork.
	// Grant them ConfigureNetwork on this object only.
	_ = p.Set(Grant{Role: Reception, Object: sampleKey(), Permission: ConfigureNetwork, Decision: DecisionAllow, Reason: "scope test"})
	if err := p.AuthorizeObject(Reception, sampleKey(), ConfigureNetwork); err != nil {
		t.Fatalf("overlay allow must grant: %v", err)
	}
}

func TestPolicy_AuthorizeObject_DenyOverridesRole(t *testing.T) {
	p := NewPolicy()
	// Owner can normally ConfigureNetwork, but explicitly
	// deny on this object.
	_ = p.Set(Grant{Role: Owner, Object: sampleKey(), Permission: ConfigureNetwork, Decision: DecisionDeny, Reason: "emergency"})
	if err := p.AuthorizeObject(Owner, sampleKey(), ConfigureNetwork); !errors.Is(err, ErrForbidden) {
		t.Fatalf("overlay deny must override role, got %v", err)
	}
}

func TestPolicy_AuthorizeObject_FallThroughToRole(t *testing.T) {
	p := NewPolicy()
	// No overlay: Owner may ConfigureNetwork.
	if err := p.AuthorizeObject(Owner, sampleKey(), ConfigureNetwork); err != nil {
		t.Fatalf("role matrix must allow: %v", err)
	}
	// Reception normally may not ConfigureNetwork.
	if err := p.AuthorizeObject(Reception, sampleKey(), ConfigureNetwork); !errors.Is(err, ErrForbidden) {
		t.Fatalf("role matrix must deny, got %v", err)
	}
}

func TestPolicy_AuthorizeObject_InvalidKey(t *testing.T) {
	p := NewPolicy()
	if err := p.AuthorizeObject(Owner, ObjectKey{Kind: "BAD", ID: "x"}, Observe); err == nil {
		t.Fatalf("invalid object key must be rejected")
	}
}

func TestPolicy_NilPolicyIsBenign(t *testing.T) {
	var p *Policy
	if err := p.AuthorizeObject(Owner, sampleKey(), Observe); err != nil {
		t.Fatalf("nil policy + owner + observe must be benign, got %v", err)
	}
	if err := p.AuthorizeObject(Helpdesk, sampleKey(), ManageUsers); !errors.Is(err, ErrForbidden) {
		t.Fatalf("nil policy + helpdesk + users.write must be forbidden")
	}
}

func TestPolicy_ConcurrentAccess(t *testing.T) {
	p := NewPolicy()
	g := Grant{Role: Helpdesk, Object: sampleKey(), Permission: ConfigureNetwork, Decision: DecisionAllow, Reason: "x"}
	_ = p.Set(g)
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			_ = p.AuthorizeObject(Helpdesk, sampleKey(), ConfigureNetwork)
		}()
		go func() {
			defer wg.Done()
			_ = p.All()
		}()
	}
	wg.Wait()
}

func TestDecisionString(t *testing.T) {
	if DecisionAllow.String() != "allow" {
		t.Fatalf("DecisionAllow must be 'allow'")
	}
	if DecisionDeny.String() != "deny" {
		t.Fatalf("DecisionDeny must be 'deny'")
	}
	if DecisionDefault.String() != "default" {
		t.Fatalf("DecisionDefault must be 'default'")
	}
}
