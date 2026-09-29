// ptpd/ptpd_test.go pins the V1 linuxptp
// configuration model and renderer.
package ptpd

import (
	"strings"
	"testing"
)

func TestClockValid(t *testing.T) {
	good := Clock{Interface: "eth0", Profile: ProfileDefault, Domain: 0, CapableHW: TSHardware}
	if err := good.Valid(); err != nil {
		t.Fatal(err)
	}
	bothRoles := Clock{Interface: "eth0", Profile: ProfileDefault, MasterOnly: true, SlaveOnly: true}
	if err := bothRoles.Valid(); err == nil {
		t.Fatal("master+slave must fail")
	}
	badTrans := Clock{Interface: "eth0", Profile: ProfileDefault, Transport: "garbage"}
	if err := badTrans.Valid(); err == nil {
		t.Fatal("unknown transport must fail")
	}
	badDom := Clock{Interface: "eth0", Profile: ProfileDefault, Domain: 999}
	if err := badDom.Valid(); err == nil {
		t.Fatal("domain out of range must fail")
	}
}

func TestRenderDeterministic(t *testing.T) {
	c := Clock{Interface: "eth0", Profile: ProfileDefault, Domain: 0, TwoStep: true, CapableHW: TSHardware}
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
	if !strings.Contains(first, "[eth0]") {
		t.Fatalf("missing per-iface section: %s", first)
	}
	if !strings.Contains(first, "time_stamping hardware") {
		t.Fatalf("missing HW ts: %s", first)
	}
}

func TestPHCSysValid(t *testing.T) {
	good := PHCSys{Clock: "/dev/ptp0", Source: "CLOCK_REALTIME", Slave: true, StepThresholdNs: 100}
	if err := good.Valid(); err != nil {
		t.Fatal(err)
	}
	bad := PHCSys{Clock: "/dev/ptp0", Slave: true, Master: true}
	if err := bad.Valid(); err == nil {
		t.Fatal("slave+master must fail")
	}
	noSource := PHCSys{Clock: "/dev/ptp0"}
	if err := noSource.Valid(); err == nil {
		t.Fatal("missing source must fail")
	}
}

func TestRenderPHCSys(t *testing.T) {
	p := PHCSys{Clock: "/dev/ptp0", Source: "CLOCK_REALTIME", Slave: true, ForceStep: true}
	out, err := RenderPHCSys(p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "clock /dev/ptp0") || !strings.Contains(out, "slave") {
		t.Fatalf("missing phc2sys fields: %s", out)
	}
}

func TestManager(t *testing.T) {
	m := NewManager()
	m.SetClocks([]Clock{
		{Interface: "eth1", Profile: ProfileDefault, Domain: 0},
		{Interface: "eth0", Profile: ProfilePower, Domain: 0},
	})
	m.SetCapability(Capability{Interface: "eth0", Capable: TSHardware})
	clocks := m.Clocks()
	if clocks[0].Interface != "eth0" || clocks[1].Interface != "eth1" {
		t.Fatalf("clocks not sorted: %+v", clocks)
	}
	caps := m.Capabilities()
	if len(caps) != 1 || caps[0].Interface != "eth0" {
		t.Fatalf("caps: %+v", caps)
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
