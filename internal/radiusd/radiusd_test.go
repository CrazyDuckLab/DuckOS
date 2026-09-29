// radiusd/radiusd_test.go pins the V1
// FreeRADIUS configuration model and
// renderer.
package radiusd

import "testing"

func TestServerValid(t *testing.T) {
	good := Server{
		Name: "core", Listen: "0.0.0.0",
		Clients: []Client{{Name: "switch1", Address: "10.0.0.1", Secret: "vault/radius/switch1"}},
		Realms:  []Realm{{Name: "example.com", Auth: "local"}},
	}
	if err := good.Valid(); err != nil {
		t.Fatal(err)
	}
	bad := Server{Name: "core", LogLevel: 99}
	if err := bad.Valid(); err == nil {
		t.Fatal("bad log level must fail")
	}
	noSecret := Server{Name: "core", Clients: []Client{{Name: "x", Address: "10.0.0.1"}}}
	if err := noSecret.Valid(); err == nil {
		t.Fatal("client without secret must fail")
	}
}

func TestRenderDeterministic(t *testing.T) {
	s := Server{
		Name: "core", Listen: "0.0.0.0",
		Clients: []Client{
			{Name: "switch1", Address: "10.0.0.2", Secret: "v1"},
			{Name: "switch0", Address: "10.0.0.1", Secret: "v0", Shortname: "sw0"},
		},
		Realms: []Realm{
			{Name: "z.example.com", Auth: "local"},
			{Name: "a.example.com", Auth: "ldap"},
		},
	}
	first, err := Render(s)
	if err != nil {
		t.Fatal(err)
	}
	hash := hashOf(first)
	for i := 0; i < 10; i++ {
		got, err := Render(s)
		if err != nil {
			t.Fatal(err)
		}
		if hashOf(got) != hash {
			t.Fatalf("render not deterministic")
		}
	}
	if !contains(first, "client switch0") || !contains(first, "client switch1") {
		t.Fatalf("missing clients: %s", first)
	}
	if !contains(first, "realm a.example.com") || !contains(first, "realm z.example.com") {
		t.Fatalf("missing realms: %s", first)
	}
}

func TestManagerLifecycle(t *testing.T) {
	m := NewManager(nil)
	s1 := Server{Name: "core", Clients: []Client{{Name: "x", Address: "10.0.0.1", Secret: "v"}}}
	s2 := Server{Name: "core", Clients: []Client{{Name: "x", Address: "10.0.0.2", Secret: "v"}}}
	m.Set(s1)
	m.Set(s2)
	if err := m.Rollback(); err != nil {
		t.Fatal(err)
	}
	if err := m.Rollback(); err == nil {
		t.Fatal("double rollback must fail")
	}
}

func TestCoARecord(t *testing.T) {
	m := NewManager(nil)
	m.RecordCoA(CoARecord{Action: "coa", Client: "switch1", User: "alice", State: CoASuccess})
	m.RecordCoA(CoARecord{Action: "disconnect", Client: "switch1", State: CoAFailed})
	m.RecordCoA(CoARecord{Action: "garbage", Client: "x"}) // rejected
	r := m.CoARecords()
	if len(r) != 2 {
		t.Fatalf("records: %d", len(r))
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func hashOf(s string) string {
	var h uint64 = 1469598103934665603
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= 1099511628211
	}
	if h == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for h > 0 {
		i--
		buf[i] = byte('0' + h%10)
		h /= 10
	}
	return string(buf[i:])
}
