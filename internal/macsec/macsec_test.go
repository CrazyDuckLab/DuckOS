package macsec

import (
	"testing"
	"time"
)

func nowTest() time.Time { return time.Unix(1700000000, 0).UTC() }

func TestCapabilityValid(t *testing.T) {
	for _, c := range []Capability{CapSoftware, CapHardware, CapUnsupported} {
		if !c.Valid() {
			t.Fatalf("%s must be valid", c)
		}
	}
	if Capability("garbage").Valid() {
		t.Fatal("garbage must be invalid")
	}
}

func TestSAValid(t *testing.T) {
	good := SA{Interface: "eth0", KeyName: "vault/macsec/eth0", SCI: "01:02:03", KeyServer: 1, CreatedAt: nowTest(), ExpiresAt: nowTest().Add(time.Hour)}
	if err := good.Valid(); err != nil {
		t.Fatal(err)
	}
	noKey := SA{Interface: "eth0", CreatedAt: nowTest()}
	if err := noKey.Valid(); err == nil {
		t.Fatal("missing key must fail")
	}
	badIface := SA{Interface: "ETH0", KeyName: "v", CreatedAt: nowTest()}
	if err := badIface.Valid(); err == nil {
		t.Fatal("non-canonical interface must fail")
	}
}

func TestManager(t *testing.T) {
	m := NewManager()
	m.SetCapability(CapHardware)
	if m.Capability() != CapHardware {
		t.Fatal("cap not set")
	}
	if err := m.AddSA(SA{Interface: "eth0", KeyName: "v", CreatedAt: nowTest()}); err != nil {
		t.Fatal(err)
	}
	if err := m.AddSA(SA{Interface: "eth0", KeyName: "v", CreatedAt: nowTest()}); err != nil {
		t.Fatal(err)
	}
	if len(m.SAs()) != 1 {
		t.Fatal("sa size")
	}
	m.RemoveSA("eth0")
	if len(m.SAs()) != 0 {
		t.Fatal("sa size after remove")
	}
}
