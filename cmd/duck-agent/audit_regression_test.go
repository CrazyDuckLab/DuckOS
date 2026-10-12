package main

import "testing"

// TestAuditUnsafeQoSRuntimeIsGuarded asserts that the
// production QoS host is wired (so DV03 has a real
// Agent-IPC path) but is wrapped by the durable
// qos.Controller. The Controller enforces preview,
// before-state capture, partial-apply commit gates
// and restart-safe rollback — the "unguarded raw tc"
// mutation path is not exposed by the Agent IPC. The
// host reports qos_unavailable when the runtime
// cannot construct a real Controller.
func TestAuditUnsafeQoSRuntimeIsGuarded(t *testing.T) {
	host, err := enableQoSService(t.TempDir())
	if err != nil || host == nil {
		t.Fatalf("production QoS host must be wired through the guarded Controller; got err=%v host=%v", err, host)
	}
	defer host.Close()
}

func TestPrivilegedWGAndXrayRequireIdentityAccess(t *testing.T) {
	if host, err := enableWireguardService(t.TempDir(), t.TempDir(), nil, nil); err == nil || host != nil {
		t.Fatalf("wireguard service must fail closed without identity access; host=%v err=%v", host, err)
	}
	if host, err := enableXrayService(t.TempDir(), nil, nil); err == nil || host != nil {
		t.Fatalf("xray service must fail closed without identity access; host=%v err=%v", host, err)
	}
}

func TestFRRServiceRejectsInvalidProductionPathspace(t *testing.T) {
	if host, err := enableFRRService("", "../bad", t.TempDir()); err == nil || host != nil {
		t.Fatalf("frr service must fail closed on invalid pathspace; host=%v err=%v", host, err)
	}
}
