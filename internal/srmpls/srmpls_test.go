// srmpls/srmpls_test.go pins the V1 SR/MPLS
// surface.
package srmpls

import "testing"

func TestRouteDistinguisher(t *testing.T) {
	rd := RouteDistinguisher{Type: "as", AS: 65000, Number: 100}
	if err := rd.Valid(); err != nil {
		t.Fatal(err)
	}
	if rd.String() != "65000:100" {
		t.Fatalf("rd: %s", rd.String())
	}
	rdIP := RouteDistinguisher{Type: "ip", IP: "10.0.0.1", Number: 42}
	if err := rdIP.Valid(); err != nil {
		t.Fatal(err)
	}
	if rdIP.String() != "10.0.0.1:42" {
		t.Fatalf("rd: %s", rdIP.String())
	}
	badType := RouteDistinguisher{Type: "garbage"}
	if err := badType.Valid(); err == nil {
		t.Fatal("bad type must fail")
	}
	badAS := RouteDistinguisher{Type: "as"}
	if err := badAS.Valid(); err == nil {
		t.Fatal("missing as must fail")
	}
}

func TestRouteTarget(t *testing.T) {
	good := RouteTarget{Value: "65000:100"}
	if err := good.Valid(); err != nil {
		t.Fatal(err)
	}
	bad := RouteTarget{Value: "garbage"}
	if err := bad.Valid(); err == nil {
		t.Fatal("bad rt must fail")
	}
}

func TestL3VPNValid(t *testing.T) {
	good := L3VPN{
		Name:      "vpn1",
		RD:        RouteDistinguisher{Type: "as", AS: 65000, Number: 100},
		ImportRTs: []RouteTarget{{Value: "65000:100"}},
		ExportRTs: []RouteTarget{{Value: "65000:200"}},
		VRF:       "customer1",
	}
	if err := good.Valid(); err != nil {
		t.Fatal(err)
	}
	noRD := L3VPN{Name: "x"}
	if err := noRD.Valid(); err == nil {
		t.Fatal("missing RD must fail")
	}
}

func TestRenderDeterministic(t *testing.T) {
	c := Config{
		L3VPNs: []L3VPN{
			{Name: "b", RD: RouteDistinguisher{Type: "as", AS: 65000, Number: 1}, VRF: "v2"},
			{Name: "a", RD: RouteDistinguisher{Type: "as", AS: 65000, Number: 2}, VRF: "v1"},
		},
		Paths: []MPLSPath{
			{Name: "p1", Labels: []int{100, 200}, Nexthop: "10.0.0.1"},
		},
		Policies: []SRPolicy{
			{Name: "s1", Endpoint: "10.0.0.0/8", Color: 100, Preference: 100, SegmentList: []int{16001, 16002}},
		},
	}
	first, err := Render(c)
	if err != nil {
		t.Fatal(err)
	}
	hash := hashOf(first)
	for i := 0; i < 10; i++ {
		got, err := Render(c)
		if err != nil {
			t.Fatal(err)
		}
		if hashOf(got) != hash {
			t.Fatalf("render not deterministic")
		}
	}
}

func TestCapabilities(t *testing.T) {
	c := NewCapabilities()
	c.Set(CapabilitySet{Capabilities: []Capability{CapMPLSFwd, CapLDP}})
	got := c.Get()
	if !got.Supports(CapMPLSFwd) || !got.Supports(CapLDP) {
		t.Fatal("supports failed")
	}
	if got.Supports(CapSRv6) {
		t.Fatal("must not support SRv6")
	}
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
