// stp/stp_test.go pins the V1 STP model.
package stp

import "testing"
import "time"

func TestBridgeValid(t *testing.T) {
	good := Bridge{Bridge: "br-1", Mode: ModeRSTP, Priority: 32768, HelloTime: 2, MaxAge: 20, FwdDelay: 15}
	if err := good.Valid(); err != nil {
		t.Fatal(err)
	}
	bad := Bridge{Bridge: "br-1", Mode: "garbage", Priority: 32768, HelloTime: 2, MaxAge: 20, FwdDelay: 15}
	if err := bad.Valid(); err == nil {
		t.Fatal("unknown mode must fail")
	}
}

func TestPortValid(t *testing.T) {
	good := Port{Bridge: "br-1", Link: "eth0", Priority: 128, PathCost: 4, BPDUGuard: true, Edge: true}
	if err := good.Valid(); err != nil {
		t.Fatal(err)
	}
}

func TestManagerSetBridgePortStorm(t *testing.T) {
	m := NewManager()
	if err := m.SetBridge(Bridge{Bridge: "br-1", Mode: ModeRSTP, Priority: 32768, HelloTime: 2, MaxAge: 20, FwdDelay: 15}); err != nil {
		t.Fatal(err)
	}
	if err := m.SetPort(Port{Bridge: "br-1", Link: "eth0", Priority: 128, PathCost: 4}); err != nil {
		t.Fatal(err)
	}
	if err := m.SetStorm("eth0", StormConfig{BroadcastPPS: 200, CPUProtection: true}); err != nil {
		t.Fatal(err)
	}
}

func TestMacFlapDetection(t *testing.T) {
	m := NewManager()
	now := time.Now()
	moves := []MACMove{
		{MAC: "aa:bb:cc:dd:ee:01", FromPort: "eth0", ToPort: "eth1", VLAN: 10, At: now},
		{MAC: "aa:bb:cc:dd:ee:01", FromPort: "eth1", ToPort: "eth0", VLAN: 10, At: now.Add(1 * time.Second)},
		{MAC: "aa:bb:cc:dd:ee:01", FromPort: "eth0", ToPort: "eth1", VLAN: 10, At: now.Add(2 * time.Second)},
	}
	for _, mv := range moves {
		m.RecordMove(mv, time.Hour, 3)
	}
	flaps := m.Flaps()
	if len(flaps) != 1 {
		t.Fatalf("expected 1 flap, got %d", len(flaps))
	}
	if flaps[0].Rate != 3 {
		t.Fatalf("rate: %d", flaps[0].Rate)
	}
	if !flaps[0].DetectedLoop {
		t.Fatal("DetectedLoop must be true at threshold")
	}
}

func TestMacFlapWindowReset(t *testing.T) {
	m := NewManager()
	now := time.Now()
	m.RecordMove(MACMove{MAC: "aa:bb:cc:dd:ee:01", FromPort: "eth0", ToPort: "eth1", VLAN: 10, At: now}, time.Minute, 5)
	m.RecordMove(MACMove{MAC: "aa:bb:cc:dd:ee:01", FromPort: "eth0", ToPort: "eth1", VLAN: 10, At: now.Add(2 * time.Minute)}, time.Minute, 5)
	flaps := m.Flaps()
	if flaps[0].Rate != 1 {
		t.Fatalf("rate after window reset: %d", flaps[0].Rate)
	}
}
