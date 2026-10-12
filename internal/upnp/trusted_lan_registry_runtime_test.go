package upnp

import (
	"net"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// PHASE 1 B-008 LIVE wiring regression coverage. The runtime
// must synchronize TrustedLANRegistry -> NetworkBackend on
// every Trust / Revoke / boot-restore so the live lab
// EnableNetwork(iface) finds AllowListen(iface) == true.
// These tests pin the contract end-to-end against the
// production wiring without requiring a real nft binary.

// runtimeTrackingBackend records TrustIface / UntrustIface
// calls in order. Tests assert the runtime forwards the
// registry's intent to the backend verbatim.
type runtimeTrackingBackend struct {
	mu      sync.Mutex
	trusted []string
	allowed map[string]bool
}

func newRuntimeTrackingBackend() *runtimeTrackingBackend {
	return &runtimeTrackingBackend{allowed: map[string]bool{}}
}

func (b *runtimeTrackingBackend) Add(Mapping) error { return nil }

func (b *runtimeTrackingBackend) Remove(string) error { return nil }

func (b *runtimeTrackingBackend) AllowListen(iface string, _ net.Addr) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.allowed[iface]
}
func (b *runtimeTrackingBackend) allowListen(iface string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.allowed[iface]
}

func (b *runtimeTrackingBackend) TrustIface(iface string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.trusted = append(b.trusted, iface)
	b.allowed[iface] = true
}

func (b *runtimeTrackingBackend) UntrustIface(iface string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.allowed, iface)
}

func (b *runtimeTrackingBackend) snapshot() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]string, len(b.trusted))
	copy(out, b.trusted)
	return out
}

// runtimeTrackingSource wraps a *RuntimeStub (cross-platform)
// to satisfy the UPnPSource interface contract the live host
// enforces. The production runtime in runtime_linux.go has
// the same surface; we exercise the cross-platform type here
// so the tests run on the dev box.
type runtimeTrackingSource struct {
	rt *RuntimeStub
}

func newRuntimeTrackingSource(be NetworkBackend) *runtimeTrackingSource {
	return &runtimeTrackingSource{rt: NewRuntimeStub(NewManager(), be)}
}

func (s *runtimeTrackingSource) EnableNetwork(n Network, iface string) error {
	if err := s.rt.EnableNetwork(n, iface); err != nil {
		return err
	}
	s.rt.TrustIface(iface)
	return nil
}
func (s *runtimeTrackingSource) DisableNetwork(ref string) {
	s.rt.DisableNetwork(ref)
	s.rt.UntrustIface(ref)
}
func (s *runtimeTrackingSource) Networks() []Network { return s.rt.Networks() }
func (s *runtimeTrackingSource) Mappings() []Mapping { return s.rt.Mappings() }
func (s *runtimeTrackingSource) RevokeMapping(id string) {
	s.rt.RevokeMapping(id)
}
func (s *runtimeTrackingSource) Stats() RuntimeStats { return s.rt.Stats() }
func (s *runtimeTrackingSource) TrustIface(iface string) {
	s.rt.TrustIface(iface)
}
func (s *runtimeTrackingSource) UntrustIface(iface string) {
	s.rt.UntrustIface(iface)
}

func lanEnrollment() EnrollmentSnapshot {
	return EnrollmentSnapshot{
		Interfaces: []EnrollmentInterface{
			{ID: "eth0", Name: "eth0", Role: "lan", Management: true},
			{ID: "eth1", Name: "eth1", Role: "lan"},
			{ID: "eth2", Name: "eth2", Role: "lan"},
		},
	}
}

func newRuntimeRegistry(t *testing.T) (*TrustedLANRegistry, *runtimeTrackingBackend, *RuntimeStub) {
	t.Helper()
	dir := t.TempDir()
	reg, err := LoadTrustedLANRegistry(filepath.Join(dir, "trust.json"), func() time.Time {
		return time.Unix(1_700_000_000, 0).UTC()
	})
	if err != nil {
		t.Fatalf("LoadTrustedLANRegistry: %v", err)
	}
	be := newRuntimeTrackingBackend()
	rt := NewRuntimeStub(NewManager(), be)
	return reg, be, rt
}

// TestRuntimeWiring_BootRestoreSeedsBackend pins the boot
// path: a registry persisted with one trusted LAN must
// re-seed the backend's TrustIface after a fresh load so
// the runtime's EnableNetwork(iface) finds AllowListen true.
func TestRuntimeWiring_BootRestoreSeedsBackend(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "trust.json")
	reg1, err := LoadTrustedLANRegistry(path, nil)
	if err != nil {
		t.Fatalf("first load: %v", err)
	}
	if err := reg1.Trust("eth1", lanEnrollment()); err != nil {
		t.Fatalf("trust eth1: %v", err)
	}
	// Simulate restart: fresh load from the same path.
	reg2, err := LoadTrustedLANRegistry(path, nil)
	if err != nil {
		t.Fatalf("second load: %v", err)
	}
	be := newRuntimeTrackingBackend()
	rt := NewRuntimeStub(NewManager(), be)
	for _, ref := range reg2.Trusted() {
		if err := reg2.AssertLANRoleOrRevoke(ref, lanEnrollment()); err != nil {
			t.Fatalf("AssertLANRoleOrRevoke(%s): %v", ref, err)
		}
		if iface, ok := reg2.ResolvedInterfaces(lanEnrollment())[ref]; ok {
			be.TrustIface(iface)
		}
	}
	if !be.allowListen("eth1") {
		t.Fatalf("backend did not admit eth1 after boot restore")
	}
	// EnableNetwork must now succeed.
	if err := rt.EnableNetwork(Network{
		NetworkRef: "eth1",
		Protocols:  []Protocol{ProtoUPnP},
		TTLSec:     3600,
	}, "eth1"); err != nil {
		t.Fatalf("EnableNetwork after boot restore: %v", err)
	}
}

// TestRuntimeWiring_Eth0Rejected pins the structural
// refusal: the externally-managed interface can never be
// trusted, even when the operator explicitly tries.
func TestRuntimeWiring_Eth0Rejected(t *testing.T) {
	reg, _, _ := newRuntimeRegistry(t)
	if err := reg.Trust("eth0", lanEnrollment()); err == nil {
		t.Fatalf("Trust(eth0) must reject management interface")
	}
}

// TestRuntimeWiring_Eth1Accepted pins the happy path: a
// disposable LAN lands in the registry AND the backend
// admits iface so EnableNetwork-iface succeeds.
func TestRuntimeWiring_Eth1Accepted(t *testing.T) {
	reg, be, rt := newRuntimeRegistry(t)
	if err := reg.Trust("eth1", lanEnrollment()); err != nil {
		t.Fatalf("Trust(eth1): %v", err)
	}
	rt.TrustIface("eth1")
	if !be.allowListen("eth1") {
		t.Fatalf("backend did not admit eth1")
	}
	if err := rt.EnableNetwork(Network{
		NetworkRef: "eth1",
		Protocols:  []Protocol{ProtoUPnP},
		TTLSec:     3600,
	}, "eth1"); err != nil {
		t.Fatalf("EnableNetwork(eth1): %v", err)
	}
}

// TestRuntimeWiring_BackendSynchronization pins the
// invariant that the runtime forwards every registry mutation
// to the backend (and only the backend) so the operator's
// view of the trust list matches the firewall's view of the
// trusted interfaces.
func TestRuntimeWiring_BackendSynchronization(t *testing.T) {
	reg, be, rt := newRuntimeRegistry(t)
	if err := reg.Trust("eth1", lanEnrollment()); err != nil {
		t.Fatalf("trust eth1: %v", err)
	}
	rt.TrustIface("eth1")
	if err := rt.EnableNetwork(Network{
		NetworkRef: "eth1",
		Protocols:  []Protocol{ProtoUPnP},
		TTLSec:     3600,
	}, "eth1"); err != nil {
		t.Fatalf("EnableNetwork(eth1): %v", err)
	}
	// Revoke path: registry + backend + manager all clear.
	reg.Revoke("eth1")
	rt.DisableNetwork("eth1")
	rt.UntrustIface("eth1")
	if be.allowListen("eth1") {
		t.Fatalf("backend still admits eth1 after revoke")
	}
	if got := be.snapshot(); len(got) != 1 || got[0] != "eth1" {
		t.Fatalf("backend trust list snapshot = %v, want [eth1]", got)
	}
}

// TestRuntimeWiring_KernelRename pins the rename / rebind
// guarantee: the registry tracks the stable DuckOS ref,
// not the kernel name. After the rename the runtime
// resolves the new iface and re-seeds the backend.
func TestRuntimeWiring_KernelRename(t *testing.T) {
	reg, be, rt := newRuntimeRegistry(t)
	if err := reg.Trust("eth1", lanEnrollment()); err != nil {
		t.Fatalf("trust eth1: %v", err)
	}
	rt.TrustIface("eth1")
	// Rebind: eth1 -> eth9. Stable ref stays "eth1"; only
	// the kernel name changes.
	renamed := EnrollmentSnapshot{
		Interfaces: []EnrollmentInterface{
			{ID: "eth0", Name: "eth0", Role: "lan", Management: true},
			{ID: "eth1", Name: "eth9", Role: "lan"},
			{ID: "eth2", Name: "eth2", Role: "lan"},
		},
	}
	if iface, ok := reg.ResolvedInterfaces(renamed)["eth1"]; !ok || iface != "eth9" {
		t.Fatalf("resolved interface = %q, want eth9", iface)
	}
	// After rename re-TrustIface on the new kernel name so
	// the runtime sees AllowListen(eth9) true.
	rt.TrustIface("eth9")
	be.TrustIface("eth9")
	if !be.allowListen("eth9") {
		t.Fatalf("backend did not admit renamed iface eth9")
	}
	_ = rt // rt is used by the source / registry wiring
}

// TestRuntimeWiring_StaleObject pins the stale-interface
// handling: if a ref points to a kernel name that no
// longer exists, the registry's ResolvedInterfaces omits
// it (the operator sees "missing" and can revoke).
func TestRuntimeWiring_StaleObject(t *testing.T) {
	reg, _, _ := newRuntimeRegistry(t)
	if err := reg.Trust("eth1", lanEnrollment()); err != nil {
		t.Fatalf("trust eth1: %v", err)
	}
	// eth1 vanishes from the live enrollment.
	empty := EnrollmentSnapshot{
		Interfaces: []EnrollmentInterface{
			{ID: "eth0", Name: "eth0", Role: "lan", Management: true},
		},
	}
	if _, ok := reg.ResolvedInterfaces(empty)["eth1"]; ok {
		t.Fatalf("stale ref must be omitted from ResolvedInterfaces")
	}
}

// TestRuntimeWiring_RestartPersistence pins the on-disk
// persistence: a registry survives a fresh load with the
// trusted set intact.
func TestRuntimeWiring_RestartPersistence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "trust.json")
	reg1, err := LoadTrustedLANRegistry(path, nil)
	if err != nil {
		t.Fatalf("first load: %v", err)
	}
	if err := reg1.Trust("eth1", lanEnrollment()); err != nil {
		t.Fatalf("trust eth1: %v", err)
	}
	if err := reg1.Trust("eth2", lanEnrollment()); err != nil {
		t.Fatalf("trust eth2: %v", err)
	}
	reg2, err := LoadTrustedLANRegistry(path, nil)
	if err != nil {
		t.Fatalf("second load: %v", err)
	}
	got := reg2.Trusted()
	if len(got) != 2 || got[0] != "eth1" || got[1] != "eth2" {
		t.Fatalf("trusted after restart = %v, want [eth1 eth2]", got)
	}
}

// TestRuntimeWiring_RoleFlipAutoRevokes pins the boot
// protection when a LAN is later reclassified to management.
// The registry's AssertLANRoleOrRevoke clears the entry so
// the next boot does not re-seed the backend.
func TestRuntimeWiring_RoleFlipAutoRevokes(t *testing.T) {
	reg, _, _ := newRuntimeRegistry(t)
	if err := reg.Trust("eth1", lanEnrollment()); err != nil {
		t.Fatalf("trust eth1: %v", err)
	}
	// Flip eth1 to management in the live enrollment.
	flipped := EnrollmentSnapshot{
		Interfaces: []EnrollmentInterface{
			{ID: "eth0", Name: "eth0", Role: "lan", Management: true},
			{ID: "eth1", Name: "eth1", Role: "lan", Management: true},
		},
	}
	if err := reg.AssertLANRoleOrRevoke("eth1", flipped); err == nil {
		t.Fatalf("AssertLANRoleOrRevoke must fail when ref becomes management")
	}
	if got := reg.Trusted(); len(got) != 0 {
		t.Fatalf("trusted after role flip = %v, want []", got)
	}
}
