// hardware/hardware_test.go pins the V1
// hardware surface.
package hardware

import "testing"

func TestLLDPSettingsValid(t *testing.T) {
	good := LLDPSettings{Link: "eth0", Mode: LLDPTxRx, SystemName: "duckos"}
	if err := good.Valid(); err != nil {
		t.Fatal(err)
	}
	bad := LLDPSettings{Link: "0bad", Mode: LLDPTxRx}
	if err := bad.Valid(); err == nil {
		t.Fatal("link must not start with a digit")
	}
	bad2 := LLDPSettings{Link: "eth0", Mode: "garbage"}
	if err := bad2.Valid(); err == nil {
		t.Fatal("unknown mode must fail")
	}
}

func TestPoEStateValid(t *testing.T) {
	good := PoEState{Port: "poe1", Enabled: true, Delivering: true, Class: PoEClass4, PowerW: 25.5}
	if err := good.Valid(); err != nil {
		t.Fatal(err)
	}
	bad := PoEState{Port: "P-OE_BAD!", Enabled: true}
	if err := bad.Valid(); err == nil {
		t.Fatal("non-canonical port must fail")
	}
}

func TestPoEWatchdogValid(t *testing.T) {
	good := PoEWatchdog{Port: "poe1", ProbeTarget: "10.0.0.5", ProbeMethod: "icmp", MaxCycles: 6, CooldownSec: 600, Enabled: true}
	if err := good.Valid(); err != nil {
		t.Fatal(err)
	}
	tooMany := PoEWatchdog{Port: "poe1", ProbeTarget: "10.0.0.5", ProbeMethod: "icmp", MaxCycles: 200, CooldownSec: 1}
	if err := tooMany.Valid(); err == nil {
		t.Fatal(">60 cycles/hour must fail")
	}
	noTarget := PoEWatchdog{Port: "poe1", ProbeMethod: "icmp", MaxCycles: 1}
	if err := noTarget.Valid(); err == nil {
		t.Fatal("missing target must fail")
	}
}

func TestRADIUSValid(t *testing.T) {
	good := RADIUSServer{Name: "core", Host: "10.0.0.1", AuthPort: 1812, AcctPort: 1813, SecretRef: "vault/radius/secret"}
	if err := good.Valid(); err != nil {
		t.Fatal(err)
	}
	noSecret := RADIUSServer{Name: "core", Host: "10.0.0.1", AuthPort: 1812, AcctPort: 1813}
	if err := noSecret.Valid(); err == nil {
		t.Fatal("secret must come from vault")
	}
	badPort := RADIUSServer{Name: "core", Host: "10.0.0.1", AuthPort: 99999, AcctPort: 1813, SecretRef: "v"}
	if err := badPort.Valid(); err == nil {
		t.Fatal("port out of range must fail")
	}
}

func TestManagerCRUD(t *testing.T) {
	m := NewManager()
	if err := m.SetLLDP(LLDPSettings{Link: "eth0", Mode: LLDPTxRx}); err != nil {
		t.Fatal(err)
	}
	m.SetNIC(NICCapabilities{Link: "eth0", SpeedMbps: 1000, Flaps: 3})
	if err := m.SetPoEState(PoEState{Port: "poe1", Enabled: true, Delivering: true, Class: PoEClass4, PowerW: 25.5}); err != nil {
		t.Fatal(err)
	}
	if err := m.SetPoEWatchdog(PoEWatchdog{Port: "poe1", ProbeTarget: "10.0.0.5", ProbeMethod: "icmp", MaxCycles: 1, CooldownSec: 60, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if err := m.SetRADIUS(RADIUSServer{Name: "core", Host: "10.0.0.1", AuthPort: 1812, AcctPort: 1813, SecretRef: "vault/radius/core"}); err != nil {
		t.Fatal(err)
	}
	m.SetModem(ModemSummary{Path: "/sys/devices/wwan0", Signal: 78, Technology: "LTE"})
	m.SetUPS(UPSSummary{Name: "ups0", BatteryPct: 88.0, LoadPct: 35.0, RuntimeSec: 1200, State: "online"})
	m.SetCapabilities([]Capability{{Key: "nic-offload", Value: "supported"}, {Key: "poe", Value: "unsupported", Detail: "no PoE hardware"}})
	snap := m.Snapshot()
	if len(snap.LLDP) != 1 || len(snap.NIC) != 1 || len(snap.PoE) != 1 || len(snap.PoEWatchdogs) != 1 {
		t.Fatalf("counts: lldp=%d nic=%d poe=%d wd=%d", len(snap.LLDP), len(snap.NIC), len(snap.PoE), len(snap.PoEWatchdogs))
	}
	if len(snap.RADIUS) != 1 || len(snap.Modems) != 1 || len(snap.UPS) != 1 || len(snap.Capabilities) != 2 {
		t.Fatalf("counts: radius=%d modems=%d ups=%d caps=%d", len(snap.RADIUS), len(snap.Modems), len(snap.UPS), len(snap.Capabilities))
	}
}
