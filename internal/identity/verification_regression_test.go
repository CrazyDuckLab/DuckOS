package identity

import (
	"errors"
	"testing"
)

func TestVerificationWildcardDenyAppliesToConcreteObject(t *testing.T) {
	p := NewPolicy()
	key := ObjectKey{Kind: "network.firewall.rule", ID: "rule-1"}
	for _, grant := range []Grant{
		{Role: Owner, Object: ObjectKey{Kind: key.Kind, ID: "*"}, Permission: ConfigureNetwork, Decision: DecisionDeny, Reason: "locked kind"},
		{Role: Owner, Object: key, Permission: ConfigureNetwork, Decision: DecisionAllow, Reason: "old exception"},
	} {
		if err := p.Set(grant); err != nil {
			t.Fatal(err)
		}
	}
	if err := p.AuthorizeObject(Owner, key, ConfigureNetwork); !errors.Is(err, ErrForbidden) {
		t.Fatal("kind-wide deny bypassed by concrete object")
	}
}

func TestVerificationSpecificDenyWinsWildcardAllow(t *testing.T) {
	p := NewPolicy()
	key := ObjectKey{Kind: "network.route", ID: "route-1"}
	for _, grant := range []Grant{
		{Role: Helpdesk, Object: ObjectKey{Kind: key.Kind, ID: "*"}, Permission: ConfigureNetwork, Decision: DecisionAllow, Reason: "kind grant"},
		{Role: Helpdesk, Object: key, Permission: ConfigureNetwork, Decision: DecisionDeny, Reason: "protected"},
	} {
		if err := p.Set(grant); err != nil {
			t.Fatal(err)
		}
	}
	if err := p.AuthorizeObject(Helpdesk, key, ConfigureNetwork); !errors.Is(err, ErrForbidden) {
		t.Fatal("specific deny bypassed")
	}
	if err := p.AuthorizeObject(Helpdesk, ObjectKey{Kind: key.Kind, ID: "route-2"}, ConfigureNetwork); err != nil {
		t.Fatal("wildcard grant not applied:", err)
	}
}
