// ddns/ddns_test.go pins the V1 DDNS model.
package ddns

import "testing"

func TestProviderKindValid(t *testing.T) {
	for _, k := range []ProviderKind{ProviderCloudflare, ProviderRoute53, ProviderHetzner, ProviderDigitalOcean, ProviderCustom, "garbage"} {
		want := k != "garbage"
		if got := k.Valid(); got != want {
			t.Errorf("%s.Valid=%v want %v", k, got, want)
		}
	}
}

func TestRecordValid(t *testing.T) {
	good := Record{ID: "blog-ddns", Hostname: "blog.example.com", Provider: ProviderCloudflare, VaultRef: "vault-ref", WANID: "wan1", TTL: 300, CGNATAware: true}
	if err := good.Valid(); err != nil {
		t.Fatal(err)
	}
	wildcard := Record{ID: "wild-ddns", Hostname: "*.example.com", Provider: ProviderCloudflare, VaultRef: "v", WANID: "wan1", TTL: 60}
	if err := wildcard.Valid(); err != nil {
		t.Fatal(err)
	}
	noVault := Record{ID: "x", Hostname: "blog.example.com", Provider: ProviderCloudflare, WANID: "wan1"}
	if err := noVault.Valid(); err == nil {
		t.Fatal("vault ref required must fail")
	}
	badHost := Record{ID: "x", Hostname: "not a domain", Provider: ProviderCloudflare, VaultRef: "v", WANID: "wan1"}
	if err := badHost.Valid(); err == nil {
		t.Fatal("non-canonical hostname must fail")
	}
}

func TestCanEnableForPublicIP(t *testing.T) {
	m := NewManager()
	_ = m.SetRecord(Record{ID: "blog", Hostname: "blog.example.com", Provider: ProviderCloudflare, VaultRef: "v", WANID: "wan1", CGNATAware: true})
	if !m.CanEnableForPublicIP("blog", "203.0.113.1") {
		t.Fatal("public IP must be allowed for CGNAT-aware record")
	}
	if m.CanEnableForPublicIP("blog", "100.64.0.1") {
		t.Fatal("CGNAT IP must be refused for CGNAT-aware record")
	}
	_ = m.SetRecord(Record{ID: "plain", Hostname: "p.example.com", Provider: ProviderCloudflare, VaultRef: "v", WANID: "wan1", CGNATAware: false})
	if !m.CanEnableForPublicIP("plain", "100.64.0.1") {
		t.Fatal("non-CGNAT-aware record may use any IP")
	}
}

func TestManagerCRUD(t *testing.T) {
	m := NewManager()
	r := Record{ID: "blog", Hostname: "blog.example.com", Provider: ProviderCloudflare, VaultRef: "v", WANID: "wan1"}
	if err := m.SetRecord(r); err != nil {
		t.Fatal(err)
	}
	m.SetState(State{RecordID: "blog", Hostname: "blog.example.com", CurrentAddress: "203.0.113.1", DetectedPublic: "203.0.113.1"})
	if _, ok := m.GetState("blog"); !ok {
		t.Fatal("GetState must find")
	}
	m.Delete("blog")
	if _, ok := m.GetRecord("blog"); ok {
		t.Fatal("GetRecord after Delete must miss")
	}
}
