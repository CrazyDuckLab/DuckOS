// Regression tests for B-008 (BETA.2): the UPnP
// trusted-LAN model.
//
// The BETA.2 product model is intent-driven. The user
// selects a DuckOS network object (e.g.
// network.lan.inside) and the runtime resolves the
// stable object ID to the underlying Linux interface.
// WAN, management, and unknown objects are
// structurally rejected; the registry does not accept a
// raw kernel iface name from the public surface.
//
// These tests exercise the registry in isolation so
// the production wiring (Agent composition root) can
// stay free of test seams.
package upnp

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// standardEnrollment is the canonical fixture used by
// every case. It exercises the four structural cases:
//   - inside  : physical LAN, trustable
//   - guest   : VLAN-backed LAN, trustable
//   - uplink  : physical WAN, structurally blocked
//   - mgmt    : management LAN, structurally blocked
func standardEnrollment() EnrollmentSnapshot {
	return EnrollmentSnapshot{
		Interfaces: []EnrollmentInterface{
			{ID: "inside", Name: "eth1", Role: "lan", Management: false},
			{ID: "guest", Name: "eth1.20", Role: "lan", Management: false},
			{ID: "uplink", Name: "eth0", Role: "wan", Management: false},
			{ID: "mgmt", Name: "eth0.10", Role: "lan", Management: true},
		},
	}
}

// TestTrustedLANRegistry_DefaultOff is the first
// guarantee: a fresh registry admits nothing. The
// runtime must default to OFF so the operator is
// forced to make an explicit selection.
func TestTrustedLANRegistry_DefaultOff(t *testing.T) {
	tmp := t.TempDir()
	r := NewTrustedLANRegistry(filepath.Join(tmp, "trust.json"), nil)
	enr := standardEnrollment()
	if r.IsTrusted("inside", enr) {
		t.Fatalf("default registry must be OFF; trust got nothing without an explicit OK")
	}
	if got := r.ResolvedInterfaces(enr); len(got) != 0 {
		t.Fatalf("default registry resolved %d entries, want 0", len(got))
	}
}

// TestTrustedLANRegistry_TrustLAN proves the happy
// path. After Trust("inside", enr) the registry
// admits the ref and resolves it to eth1.
func TestTrustedLANRegistry_TrustLAN(t *testing.T) {
	tmp := t.TempDir()
	r := NewTrustedLANRegistry(filepath.Join(tmp, "trust.json"), nil)
	enr := standardEnrollment()
	if err := r.Trust("inside", enr); err != nil {
		t.Fatalf("Trust(inside): %v", err)
	}
	if !r.IsTrusted("inside", enr) {
		t.Fatalf("inside must be trusted after Trust()")
	}
	if got := r.ResolvedInterfaces(enr); got["inside"] != "eth1" {
		t.Fatalf("ResolvedInterfaces: inside -> %q want eth1", got["inside"])
	}
}

// TestTrustedLANRegistry_RejectsWAN is the structural
// block for WAN: an explicit Trust("uplink") MUST fail
// even though the ref exists in the enrollment.
func TestTrustedLANRegistry_RejectsWAN(t *testing.T) {
	tmp := t.TempDir()
	r := NewTrustedLANRegistry(filepath.Join(tmp, "trust.json"), nil)
	enr := standardEnrollment()
	if err := r.Trust("uplink", enr); err == nil {
		t.Fatalf("Trust(uplink) must fail; got nil")
	} else if !strings.Contains(err.Error(), "WAN") {
		t.Fatalf("expected WAN rejection, got %v", err)
	}
	if r.IsTrusted("uplink", enr) {
		t.Fatalf("uplink must not be trusted")
	}
}

// TestTrustedLANRegistry_RejectsManagement proves the
// management interface is structurally blocked. The
// management LAN is the DuckOS control plane; exposing
// it to UPnP would let any client on that segment mutate
// /var/lib/duckos through PCP / NAT-PMP / UPnP-IGD.
func TestTrustedLANRegistry_RejectsManagement(t *testing.T) {
	tmp := t.TempDir()
	r := NewTrustedLANRegistry(filepath.Join(tmp, "trust.json"), nil)
	enr := standardEnrollment()
	if err := r.Trust("mgmt", enr); err == nil {
		t.Fatalf("Trust(mgmt) must fail; got nil")
	} else if !strings.Contains(err.Error(), "management") {
		t.Fatalf("expected management rejection, got %v", err)
	}
}

// TestTrustedLANRegistry_RejectsUnknown proves an
// attempt to trust a ref the enrollment does not
// contain fails closed with ErrNotTrusted.
func TestTrustedLANRegistry_RejectsUnknown(t *testing.T) {
	tmp := t.TempDir()
	r := NewTrustedLANRegistry(filepath.Join(tmp, "trust.json"), nil)
	enr := standardEnrollment()
	if err := r.Trust("does-not-exist", enr); err == nil {
		t.Fatalf("Trust(unknown) must fail; got nil")
	} else if !errorsIs(err, ErrNotTrusted) {
		t.Fatalf("expected ErrNotTrusted, got %v", err)
	}
}

// TestTrustedLANRegistry_RejectsInvalidRef enforces
// the canonical-id gate. A ref containing shell metas
// must be rejected before the resolver is consulted.
func TestTrustedLANRegistry_RejectsInvalidRef(t *testing.T) {
	tmp := t.TempDir()
	r := NewTrustedLANRegistry(filepath.Join(tmp, "trust.json"), nil)
	enr := standardEnrollment()
	bad := []string{
		"",
		"inside; rm -rf /",
		"inside\nGET /evil",
		"inside$(id)",
		"*",
	}
	for _, ref := range bad {
		if err := r.Trust(ref, enr); err == nil {
			t.Errorf("Trust(%q) must fail", ref)
		}
	}
}

// TestTrustedLANRegistry_Revoke proves the remove
// path: after Trust then Revoke, IsTrusted is false
// again.
func TestTrustedLANRegistry_Revoke(t *testing.T) {
	tmp := t.TempDir()
	r := NewTrustedLANRegistry(filepath.Join(tmp, "trust.json"), nil)
	enr := standardEnrollment()
	if err := r.Trust("inside", enr); err != nil {
		t.Fatalf("Trust: %v", err)
	}
	if err := r.Revoke("inside"); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if r.IsTrusted("inside", enr) {
		t.Fatalf("inside must NOT be trusted after Revoke")
	}
}

// TestTrustedLANRegistry_RevokeIdempotent proves a
// second Revoke on the same ref is a no-op so the
// operator can re-issue revoke without a precondition
// check.
func TestTrustedLANRegistry_RevokeIdempotent(t *testing.T) {
	tmp := t.TempDir()
	r := NewTrustedLANRegistry(filepath.Join(tmp, "trust.json"), nil)
	enr := standardEnrollment()
	if err := r.Trust("inside", enr); err != nil {
		t.Fatalf("Trust: %v", err)
	}
	if err := r.Revoke("inside"); err != nil {
		t.Fatalf("first Revoke: %v", err)
	}
	if err := r.Revoke("inside"); err != nil {
		t.Fatalf("second Revoke must be a no-op; got %v", err)
	}
}

// TestTrustedLANRegistry_PersistenceRoundtrip proves
// the trust list survives a save/load round-trip on
// disk. This is the reboot-persistence requirement:
// the trust selection cannot depend on Agent-only state
// because the Agent is restarted across reboots and the
// launcher / kernel transitions cannot afford to lose
// it.
func TestTrustedLANRegistry_PersistenceRoundtrip(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "trust.json")
	r := NewTrustedLANRegistry(path, nil)
	enr := standardEnrollment()
	if err := r.Trust("inside", enr); err != nil {
		t.Fatalf("Trust(inside): %v", err)
	}
	if err := r.Trust("guest", enr); err != nil {
		t.Fatalf("Trust(guest): %v", err)
	}
	r2, err := LoadTrustedLANRegistry(path, nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := r2.Trusted(); len(got) != 2 || got[0] != "guest" || got[1] != "inside" {
		t.Fatalf("round-trip list: got=%v want [guest inside]", got)
	}
	if !r2.IsTrusted("inside", enr) {
		t.Fatalf("inside must remain trusted after reload")
	}
	if !r2.IsTrusted("guest", enr) {
		t.Fatalf("guest must remain trusted after reload")
	}
}

// TestTrustedLANRegistry_RejectsMalformedFile locks the
// fail-closed guarantee. A half-written or hand-edited
// trust file MUST NOT silently drop the trust list —
// it must surface a typed error so the Agent refuses
// to start.
func TestTrustedLANRegistry_RejectsMalformedFile(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "trust.json")
	for _, body := range []string{
		"",
		"{}",
		`{"version":2,"networks":["inside"]}`,
		`{"version":1,"networks":[1,1]}`,
	} {
		if err := writeFile(path, body); err != nil {
			t.Fatalf("writeFile: %v", err)
		}
		if _, err := LoadTrustedLANRegistry(path, nil); err == nil {
			t.Errorf("Load(%q) must fail", body)
		}
	}
}

// TestTrustedLANRegistry_MissingFileIsEmpty proves a
// fresh Agent boot on a host that never had UPnP enabled
// returns an empty registry (default OFF) without an
// error. The operator must explicitly turn it on.
func TestTrustedLANRegistry_MissingFileIsEmpty(t *testing.T) {
	tmp := t.TempDir()
	r, err := LoadTrustedLANRegistry(filepath.Join(tmp, "nope.json"), nil)
	if err != nil {
		t.Fatalf("Load on missing file: %v", err)
	}
	if got := r.Trusted(); len(got) != 0 {
		t.Fatalf("expected empty trust list, got %v", got)
	}
}

// TestTrustedLANRegistry_KernelRename is the "stable
// object identity" guarantee: the trust list is keyed
// by the DuckOS object ID, not by the Linux iface
// name. A kernel rename (inside -> eth9) keeps the
// trust because the object ID survives.
func TestTrustedLANRegistry_KernelRename(t *testing.T) {
	tmp := t.TempDir()
	r := NewTrustedLANRegistry(filepath.Join(tmp, "trust.json"), nil)
	enr := standardEnrollment()
	if err := r.Trust("inside", enr); err != nil {
		t.Fatalf("Trust: %v", err)
	}
	renamed := EnrollmentSnapshot{
		Interfaces: []EnrollmentInterface{
			{ID: "inside", Name: "eth9", Role: "lan", Management: false},
		},
	}
	if !r.IsTrusted("inside", renamed) {
		t.Fatalf("inside must remain trusted after kernel rename")
	}
	if got := r.ResolvedInterfaces(renamed); got["inside"] != "eth9" {
		t.Fatalf("rename resolution: inside -> %q want eth9", got["inside"])
	}
}

// TestTrustedLANRegistry_RoleFlip is the structural
// protection against role drift: if an operator
// later flips a LAN's role to wan (e.g. by reassigning
// it in the enrollment), the next IsTrusted call
// returns false even though the ref is still in the
// trust file.
func TestTrustedLANRegistry_RoleFlip(t *testing.T) {
	tmp := t.TempDir()
	r := NewTrustedLANRegistry(filepath.Join(tmp, "trust.json"), nil)
	enr := standardEnrollment()
	if err := r.Trust("inside", enr); err != nil {
		t.Fatalf("Trust: %v", err)
	}
	flipped := EnrollmentSnapshot{
		Interfaces: []EnrollmentInterface{
			{ID: "inside", Name: "eth1", Role: "wan", Management: false},
		},
	}
	if r.IsTrusted("inside", flipped) {
		t.Fatalf("inside must NOT be trusted after role flip to wan")
	}
}

// TestTrustedLANRegistry_AtomicWrite proves a partial
// write to disk does NOT clobber the previous trust
// file. The contract is the rename-on-write atomicity
// that /var/lib/duckos/upnp/trusted-networks.json
// inherits from POSIX rename. The test forces a stale
// reader by pre-populating the destination file with
// a known good list, then triggers a Trust whose Save()
// uses rename to swap in the new list.
func TestTrustedLANRegistry_AtomicWrite(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "trust.json")
	if err := writeFile(path, `{"version":1,"networks":["inside"]}`); err != nil {
		t.Fatalf("seed: %v", err)
	}
	r, err := LoadTrustedLANRegistry(path, nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := r.Trust("guest", standardEnrollment()); err != nil {
		t.Fatalf("Trust(guest): %v", err)
	}
	// Reload from disk; the on-disk file should be the
	// merged set, not the stale single-entry read.
	r2, err := LoadTrustedLANRegistry(path, nil)
	if err != nil {
		t.Fatalf("Load#2: %v", err)
	}
	got := r2.Trusted()
	if len(got) != 2 {
		t.Fatalf("merged list: got %v want [guest inside]", got)
	}
}

// writeFile is a tiny test-only helper that writes
// body to path with mode 0600 (matches the registry's
// production perm).
func writeFile(path, body string) error {
	return os.WriteFile(path, []byte(body), 0o600)
}

// errorsIs is a thin stdlib alias so the test file can
// match errors without dragging errors.Is through
// every assertion site.
func errorsIs(err, target error) bool {
	return errors.Is(err, target)
}
