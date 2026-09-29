// frr/frr_test.go pins the V1 FRR
// configuration model and renderer.
package frr

import (
	"regexp"
	"strings"
	"testing"
)

func TestConfigValid(t *testing.T) {
	good := Config{
		Daemons: []Daemon{DaemonZebra, DaemonBGP},
		BGP: []BGPSession{{
			Name: "core", LocalAS: 65000, RouterID: "1.1.1.1",
			IPv4Unicast: true,
			Networks:    []Network{{Prefix: "10.0.0.0/8"}},
			Neighbors:   []Neighbor{{Address: "10.0.0.1", RemoteAS: 65001}},
		}},
	}
	if err := good.Valid(); err != nil {
		t.Fatal(err)
	}
	noDaemon := Config{}
	if err := noDaemon.Valid(); err == nil {
		t.Fatal("missing daemons must fail")
	}
	noAF := Config{
		Daemons: []Daemon{DaemonBGP},
		BGP:     []BGPSession{{Name: "x", LocalAS: 1}},
	}
	if err := noAF.Valid(); err == nil {
		t.Fatal("missing address family must fail")
	}
	badNeighbor := Config{
		Daemons: []Daemon{DaemonBGP},
		BGP: []BGPSession{{
			Name: "x", LocalAS: 1, IPv4Unicast: true,
			Neighbors: []Neighbor{{RemoteAS: 1}},
		}},
	}
	if err := badNeighbor.Valid(); err == nil {
		t.Fatal("missing neighbor address must fail")
	}
}

func TestRenderDeterministic(t *testing.T) {
	c := Config{
		Daemons: []Daemon{DaemonBGP, DaemonZebra},
		BGP: []BGPSession{{
			Name: "core", LocalAS: 65000, RouterID: "1.1.1.1",
			IPv4Unicast: true,
			Networks:    []Network{{Prefix: "10.0.0.0/8"}, {Prefix: "192.168.0.0/16"}},
			Neighbors: []Neighbor{
				{Address: "10.0.0.2", RemoteAS: 65002},
				{Address: "10.0.0.1", RemoteAS: 65001, Description: "primary"},
			},
		}},
		OSPF: []OSPFInstance{{
			Name: "core", RouterID: "2.2.2.2",
			Areas: []OSPFArea{
				{ID: "0.0.0.1", Networks: []Network{{Prefix: "172.16.0.0/12"}}},
				{ID: "0.0.0.0"},
			},
		}},
	}
	first, err := Render(c)
	if err != nil {
		t.Fatal(err)
	}
	// Render 10 times: output must be byte-stable.
	hash := hashOf(first)
	for i := 0; i < 10; i++ {
		got, err := Render(c)
		if err != nil {
			t.Fatal(err)
		}
		if hashOf(got) != hash {
			t.Fatalf("render not deterministic: %d", i)
		}
	}
	// Sanity: neighbors and networks sorted.
	if !regexp.MustCompile(`(?s)neighbor 10\.0\.0\.1.*neighbor 10\.0\.0\.2`).MatchString(first) {
		t.Fatalf("neighbors not sorted: %s", first)
	}
	if !strings.Contains(first, "router bgp 65000\n") || strings.Contains(first, "router bgp 65000 core") {
		t.Fatalf("missing router bgp: %s", first)
	}
	if !strings.Contains(first, "router ospf core") {
		t.Fatalf("missing router ospf: %s", first)
	}
}

func TestManagerLifecycle(t *testing.T) {
	m := NewManager(nil)
	c1 := Config{
		Daemons: []Daemon{DaemonZebra, DaemonBGP},
		BGP: []BGPSession{{
			Name: "v1", LocalAS: 65000, RouterID: "1.1.1.1", IPv4Unicast: true,
		}},
	}
	c2 := Config{
		Daemons: []Daemon{DaemonZebra, DaemonBGP},
		BGP: []BGPSession{{
			Name: "v2", LocalAS: 65000, RouterID: "1.1.1.1", IPv4Unicast: true,
		}},
	}
	if err := m.Set(c1); err != nil {
		t.Fatal(err)
	}
	if err := m.Set(c2); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Render(); err != nil {
		t.Fatal(err)
	}
	if err := m.Rollback("test"); err != nil {
		t.Fatal(err)
	}
	// After Rollback, the previous config was
	// rotated out, so a second rollback fails.
	if err := m.Rollback("again"); err == nil {
		t.Fatal("double rollback must fail")
	}
	hist := m.History()
	if len(hist) < 3 {
		t.Fatalf("history: %d", len(hist))
	}
}

func hashOf(s string) string {
	// Tiny non-crypto hash; we just need
	// byte-equality across renders.
	var h uint64 = 1469598103934665603
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= 1099511628211
	}
	return formatUint(h)
}

func formatUint(h uint64) string {
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
