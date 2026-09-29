// vrf/vrf_test.go pins the V1 VRF model.
package vrf

import "testing"

func TestVRFValidID(t *testing.T) {
	if !ID("mgmt").Valid() {
		t.Fatal("mgmt must be valid")
	}
	if !ID("vrf-100").Valid() {
		t.Fatal("vrf-100 must be valid")
	}
	if ID("MGMT").Valid() {
		t.Fatal("uppercase must be invalid")
	}
	if ID("../bad").Valid() {
		t.Fatal("path traversal must be invalid")
	}
	if ID("a-very-long-very-long-vrf-name").Valid() {
		t.Fatal("over-length name must be invalid")
	}
}

func TestVRFValid(t *testing.T) {
	good := VRF{ID: "mgmt", Name: "management", Table: 1000, Members: []Member{{Kind: "interface", Target: "eth0"}}, Routes: []Route{{Destination: "0.0.0.0/0", NextHop: "10.0.0.1", Interface: "eth0"}}}
	if err := good.Valid(); err != nil {
		t.Fatal(err)
	}
	bad := VRF{ID: "mgmt", Name: "x", Table: 0}
	if err := bad.Valid(); err == nil {
		t.Fatal("zero table must fail")
	}
	badLeak := VRF{ID: "mgmt", Name: "x", Table: 1000, Leaks: []Leak{{Source: "mgmt", Destination: "mgmt", Prefix: "10.0.0.0/8"}}}
	if err := badLeak.Valid(); err == nil {
		t.Fatal("self-leak must fail")
	}
	badRoute := VRF{ID: "mgmt", Name: "x", Table: 1000, Routes: []Route{{Destination: "10.0.0.0/8", Type: "garbage"}}}
	if err := badRoute.Valid(); err == nil {
		t.Fatal("unknown route type must fail")
	}
}

func TestManagerLookupPicksStaticRoute(t *testing.T) {
	m := NewManager()
	v := VRF{ID: "mgmt", Name: "mgmt", Table: 1000, Routes: []Route{{Destination: "10.0.0.0/8", NextHop: "10.0.0.1", Interface: "eth0"}, {Destination: "0.0.0.0/0", NextHop: "10.0.0.254", Interface: "eth0"}}}
	if err := m.Set(v); err != nil {
		t.Fatal(err)
	}
	r := m.Lookup(Lookup{VRF: "mgmt", Dst: "10.0.0.5"})
	if r.NextHop != "10.0.0.1" {
		t.Fatalf("Lookup: %+v", r)
	}
	r2 := m.Lookup(Lookup{VRF: "mgmt", Dst: "8.8.8.8"})
	if r2.NextHop != "10.0.0.254" {
		t.Fatalf("default-route Lookup: %+v", r2)
	}
}

func TestManagerLookupBlackhole(t *testing.T) {
	m := NewManager()
	_ = m.Set(VRF{ID: "guest", Name: "guest", Table: 1100, Routes: []Route{{Destination: "10.10.0.0/16", Type: "blackhole"}}})
	r := m.Lookup(Lookup{VRF: "guest", Dst: "10.10.5.5"})
	if r.Route != "10.10.0.0/16" {
		t.Fatalf("blackhole Lookup: %+v", r)
	}
	if r.Reason == "" {
		t.Fatal("reason must be populated")
	}
}

func TestManagerLookupNoRoute(t *testing.T) {
	m := NewManager()
	_ = m.Set(VRF{ID: "mgmt", Name: "mgmt", Table: 1000})
	r := m.Lookup(Lookup{VRF: "mgmt", Dst: "8.8.8.8"})
	if r.Reason == "" {
		t.Fatal("Reason must be populated when no route is found")
	}
}

func TestManagerLookupLeak(t *testing.T) {
	m := NewManager()
	_ = m.Set(VRF{ID: "lan", Name: "lan", Table: 100, Leaks: []Leak{{Source: "lan", Destination: "mgmt", Prefix: "10.0.0.0/8"}}})
	r := m.Lookup(Lookup{VRF: "lan", Dst: "10.0.0.5"})
	if r.Rule == "" {
		t.Fatal("leak Lookup must surface a Rule")
	}
}

func TestManagerLookupUnknownVRF(t *testing.T) {
	m := NewManager()
	r := m.Lookup(Lookup{VRF: "missing", Dst: "8.8.8.8"})
	if r.Reason == "" {
		t.Fatal("Reason must be populated for unknown vrf")
	}
}
