// install/role_test.go pins the role-model
// contract. The Detector is exercised against
// representative network snapshots; the
// Decision validator refuses malformed
// records. The tests are the executable
// specification for the V1 first-run.
package install

import (
	"testing"
	"time"
)

func nowTime() time.Time { return time.Unix(1735689600, 0).UTC() }

func TestRoleValid(t *testing.T) {
	for _, r := range []Role{RoleUnset, RoleRouter, RoleServer, RoleCustom, "garbage"} {
		want := r == RoleRouter || r == RoleServer || r == RoleCustom
		if got := r.Valid(); got != want {
			t.Fatalf("Role(%q).Valid() = %v, want %v", r, got, want)
		}
	}
}

func TestDecisionValidateRejectsUnsetRole(t *testing.T) {
	d := Decision{DecidedAt: nowTime(), DecidedBy: "operator-1"}
	if err := d.Validate(); err == nil {
		t.Fatal("Validate must reject an unset role")
	}
}

func TestDecisionValidateRejectsZeroTime(t *testing.T) {
	d := Decision{Role: RoleServer, DecidedBy: "operator-1"}
	if err := d.Validate(); err == nil {
		t.Fatal("Validate must reject a zero decidedAt")
	}
}

func TestDecisionValidateRejectsBadInterfaceName(t *testing.T) {
	d := Decision{Role: RoleServer, DecidedAt: nowTime(), DecidedBy: "operator-1", NetworkSnapshot: NetworkSnapshot{Interfaces: []Interface{{Name: "../etc/passwd", Up: true}}}}
	if err := d.Validate(); err == nil {
		t.Fatal("Validate must reject a non-canonical interface name")
	}
}

func TestDecisionJSONRoundTrip(t *testing.T) {
	d := Decision{Role: RoleServer, DecidedAt: nowTime(), DecidedBy: "operator-1", Reason: "VPS template", NetworkSnapshot: NetworkSnapshot{Interfaces: []Interface{{Name: "eth0", Address: "172.18.4.149/16", Up: true, Kind: "wan"}}, Management: "eth0"}}
	raw, err := d.JSON()
	if err != nil {
		t.Fatalf("JSON: %v", err)
	}
	back, err := DecodeDecision(raw)
	if err != nil {
		t.Fatalf("DecodeDecision: %v", err)
	}
	if back.Role != d.Role {
		t.Fatalf("role: %q vs %q", back.Role, d.Role)
	}
	if back.NetworkSnapshot.Management != "eth0" {
		t.Fatalf("management: %q", back.NetworkSnapshot.Management)
	}
}

func TestDetectorSingleNICWithAddressIsServer(t *testing.T) {
	d := NewDetector()
	snap := NetworkSnapshot{Interfaces: []Interface{{Name: "eth0", Address: "172.18.4.149/16", Up: true}}, Management: "eth0"}
	p := d.Propose(snap)
	if p.Role != RoleServer {
		t.Fatalf("Propose = %s, want server (reason: %s)", p.Role, p.Reason)
	}
}

func TestDetectorSingleNICWithoutAddressIsCustom(t *testing.T) {
	d := NewDetector()
	snap := NetworkSnapshot{Interfaces: []Interface{{Name: "eth0", Up: true}}, Management: "eth0"}
	p := d.Propose(snap)
	if p.Role != RoleCustom {
		t.Fatalf("Propose = %s, want custom", p.Role)
	}
}

func TestDetectorMultiNICWithWanAndLanIsRouter(t *testing.T) {
	d := NewDetector()
	snap := NetworkSnapshot{Interfaces: []Interface{
		{Name: "eth0", Address: "203.0.113.1/24", Up: true, Kind: "wan"},
		{Name: "eth1", Address: "192.168.1.1/24", Up: true, Kind: "lan"},
	}, Management: "eth1"}
	p := d.Propose(snap)
	if p.Role != RoleRouter {
		t.Fatalf("Propose = %s, want router (reason: %s)", p.Role, p.Reason)
	}
}

func TestDetectorNoInterfacesUpIsCustom(t *testing.T) {
	d := NewDetector()
	snap := NetworkSnapshot{}
	if p := d.Propose(snap); p.Role != RoleCustom {
		t.Fatalf("Propose = %s, want custom", p.Role)
	}
}

func TestDetectorManagementMismatchIsCustom(t *testing.T) {
	d := NewDetector()
	snap := NetworkSnapshot{Interfaces: []Interface{{Name: "eth0", Address: "172.18.4.149/16", Up: true}}, Management: "eth1"}
	if p := d.Propose(snap); p.Role != RoleCustom {
		t.Fatalf("Propose = %s, want custom when management does not match the only NIC", p.Role)
	}
}
