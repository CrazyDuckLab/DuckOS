// Package subscription / group_test.go: ordered multi-node groups.
package subscription

import (
	"testing"
	"time"
)

func TestGroupValidateAndOrder(t *testing.T) {
	g := Group{
		ID: "personal-vpn", Name: "Personal VPN", Format: FormatClash, Enabled: true,
		Members: []GroupMember{
			{NodeID: "n1", ServerID: "srv-nl", UserID: "u1", DisplayName: "Netherlands", Enabled: true},
			{NodeID: "n2", ServerID: "srv-de", UserID: "u2", DisplayName: "Germany", Enabled: true},
			{NodeID: "n3", ServerID: "srv-se", UserID: "u3", DisplayName: "Sweden", Enabled: false},
		},
	}
	if err := g.Validate(); err != nil {
		t.Fatal(err)
	}
	active := g.ActiveMembers()
	if len(active) != 2 || active[0].DisplayName != "Netherlands" || active[1].DisplayName != "Germany" {
		t.Fatalf("order/enabled mismatch: %+v", active)
	}
	dup := g
	dup.Members[1].DisplayName = "Netherlands"
	if err := dup.Validate(); err == nil {
		t.Fatal("duplicate display name accepted")
	}
	noEnabled := g
	for i := range noEnabled.Members {
		noEnabled.Members[i].Enabled = false
	}
	if err := noEnabled.Validate(); err == nil {
		t.Fatal("all-disabled group accepted")
	}
	if _, err := NewGroupStore().Upsert(g); err == nil {
		t.Fatal("group without created ids... should need valid member ids")
	}
}

func TestGroupStoreLifecycle(t *testing.T) {
	store := NewGroupStore()
	g := Group{
		ID: "g1", Name: "G1", Format: FormatBase64, Enabled: true,
		Members: []GroupMember{{ServerID: "s1", UserID: "u1", DisplayName: "Alpha", Enabled: true}},
	}
	saved, err := store.Upsert(g)
	if err != nil {
		t.Fatal(err)
	}
	if saved.CreatedAt.IsZero() {
		t.Fatal("createdAt not stamped")
	}
	if _, ok := store.Get("g1"); !ok {
		t.Fatal("get failed")
	}
	if err := store.Delete("g1"); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete("g1"); err == nil {
		t.Fatal("double delete accepted")
	}
}

func TestGroupProxies(t *testing.T) {
	store := NewGroupStore()
	g := Group{
		ID: "g1", Name: "G1", Format: FormatClash, Enabled: true,
		Members: []GroupMember{
			{ServerID: "s1", UserID: "u1", DisplayName: "Alpha", Enabled: true},
			{ServerID: "s2", UserID: "u2", DisplayName: "Beta", Enabled: true},
			{ServerID: "s3", UserID: "u3", DisplayName: "Gamma", Enabled: false},
		},
	}
	if _, err := store.Upsert(g); err != nil {
		t.Fatal(err)
	}
	var resolved []string
	proxies, err := store.GroupProxies("g1", time.Now(), func(m GroupMember) (Proxy, error) {
		resolved = append(resolved, m.DisplayName)
		return Proxy{Name: m.DisplayName, Protocol: "ss", Host: "203.0.113.1", Port: 8388,
			Cipher: "aes-256-gcm", Password: "pw", Transport: "tcp", Security: "none"}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(proxies) != 2 || resolved[0] != "Alpha" || resolved[1] != "Beta" {
		t.Fatalf("resolution: %v", resolved)
	}
	// Expiry.
	exp := g
	exp.ExpiresAt = time.Now().Add(-time.Hour).Format(time.RFC3339)
	if _, err := store.Upsert(exp); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GroupProxies("g1", time.Now(), func(GroupMember) (Proxy, error) { return Proxy{}, nil }); err == nil {
		t.Fatal("expired group served")
	}
}
