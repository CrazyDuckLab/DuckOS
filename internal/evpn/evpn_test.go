// evpn/evpn_test.go pins the V1 EVPN
// surface.
package evpn

import "testing"

func TestVTEPValid(t *testing.T) {
	good := VTEP{Name: "vtep1", Address: "10.0.0.1"}
	if err := good.Valid(); err != nil {
		t.Fatal(err)
	}
	noName := VTEP{Address: "10.0.0.1"}
	if err := noName.Valid(); err == nil {
		t.Fatal("missing name must fail")
	}
}

func TestEthernetSegmentValid(t *testing.T) {
	good := EthernetSegment{ESI: "00:11:22:33:44:55:66:77:88:99", Mode: ModeAllActive}
	if err := good.Valid(); err != nil {
		t.Fatal(err)
	}
	badESI := EthernetSegment{ESI: "garbage", Mode: ModeAllActive}
	if err := badESI.Valid(); err == nil {
		t.Fatal("bad ESI must fail")
	}
}

func TestVNIValid(t *testing.T) {
	good := VNI{ID: 10001, VTEP: "vtep1", Bridge: "br-10001", ARPSuppression: true, VRF: "tenant1"}
	if err := good.Valid(); err != nil {
		t.Fatal(err)
	}
	noVTEP := VNI{ID: 1, Bridge: "br-1"}
	if err := noVTEP.Valid(); err == nil {
		t.Fatal("missing vtep must fail")
	}
	badVNI := VNI{ID: 0, VTEP: "x", Bridge: "x"}
	if err := badVNI.Valid(); err == nil {
		t.Fatal("VNI 0 must fail")
	}
}

func TestRenderDeterministic(t *testing.T) {
	c := Config{
		VTEPs:    []VTEP{{Name: "v1", Address: "10.0.0.1"}},
		VNIs:     []VNI{{ID: 10001, VTEP: "v1", Bridge: "br-1"}},
		Segments: []EthernetSegment{{ESI: "00:11:22:33:44:55:66:77:88:99", Mode: ModeAllActive}},
	}
	first, err := Render(c)
	if err != nil {
		t.Fatal(err)
	}
	hash := hashOf(first)
	for i := 0; i < 10; i++ {
		got, _ := Render(c)
		if hashOf(got) != hash {
			t.Fatalf("render not deterministic")
		}
	}
}

func TestDFStore(t *testing.T) {
	m := NewManager()
	m.SetDF(DFResult{ESI: "00:11:22:33:44:55:66:77:88:99", State: DFPrimary, VTEP: "v1"})
	m.SetDF(DFResult{ESI: "00:11:22:33:44:55:66:77:88:99", State: "garbage"}) // rejected
	if r, ok := m.DF("00:11:22:33:44:55:66:77:88:99"); !ok || r.State != DFPrimary {
		t.Fatalf("df: %+v ok=%v", r, ok)
	}
	if len(m.DFAll()) != 1 {
		t.Fatal("df all size")
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
