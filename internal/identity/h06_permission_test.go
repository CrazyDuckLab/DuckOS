package identity

import "testing"

func TestH06PolicyRequiresCanonicalPermissions(t *testing.T) {
	for _, decision := range []Decision{DecisionDefault, DecisionAllow, DecisionDeny} {
		for _, permission := range []Permission{"", "network.writX", "*", "network.write "} {
			p := NewPolicy()
			if err := p.Set(Grant{Role: Owner, Object: sampleKey(), Permission: permission, Decision: decision, Reason: "test"}); err == nil {
				t.Fatalf("invalid permission %q admitted", permission)
			}
		}
	}
	for _, permission := range []Permission{Observe, ConfigureNetwork, Diagnose, ManageGuests, ManageUsers, ManageSecurity, ManageUpdates, ManageBackups, ExportClients} {
		if err := (Grant{Role: Owner, Object: sampleKey(), Permission: permission, Decision: DecisionDeny, Reason: "test"}).Valid(); err != nil {
			t.Fatalf("known permission %q: %v", permission, err)
		}
	}
}
