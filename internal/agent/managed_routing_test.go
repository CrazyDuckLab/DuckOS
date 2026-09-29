package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"net/netip"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/network"
	"github.com/CrazyDuckLab/DuckOS/internal/safeapply"
	"github.com/CrazyDuckLab/DuckOS/internal/state"
)

type recordingEvidenceCollector struct {
	calls atomic.Int32
	out   network.ManagementEvidence
}

func (r *recordingEvidenceCollector) Collect(_ context.Context) (network.ManagementEvidence, error) {
	r.calls.Add(1)
	return r.out, nil
}

// syntheticAdapter is a test double for safeapply.Adapter that records
// mutations without touching a real kernel. The state of "live" is
// mutable so tests can drive recovery paths.
type syntheticAdapter struct {
	mu   sync.Mutex
	live json.RawMessage
}

func (a *syntheticAdapter) Preflight(context.Context, json.RawMessage, json.RawMessage) error {
	return nil
}
func (a *syntheticAdapter) Apply(_ context.Context, _, next json.RawMessage) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.live = append(json.RawMessage(nil), next...)
	return nil
}
func (a *syntheticAdapter) Verify(_ context.Context, want json.RawMessage) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !bytes.Equal(a.live, want) {
		return errSyntheticStateMismatch
	}
	return nil
}
func (a *syntheticAdapter) Restore(_ context.Context, before, _ json.RawMessage) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.live = append(json.RawMessage(nil), before...)
	return nil
}

var errSyntheticStateMismatch = &syntheticMismatchError{}

type syntheticMismatchError struct{}

func (*syntheticMismatchError) Error() string { return "synthetic state mismatch" }

func managedRoutingDepsFixture(t *testing.T) ManagedRoutingDeps {
	t.Helper()
	return ManagedRoutingDeps{
		Enrollment: network.Enrollment{
			Version: 1, CreatedAt: time.Now().UTC(), FirewallTable: "inet duckos",
			Physical: []network.PhysicalBinding{
				{ID: "uplink", Name: "testwan0", MAC: "02:00:00:00:00:01"},
				{ID: "inside", Name: "testlan0", MAC: "02:00:00:00:00:02"},
			},
			VLANNames: []string{"testguest0"},
		},
		BootKernel:      &emptyBootKernel{tables: []int{}},
		Evidence:        &recordingEvidenceCollector{out: network.ManagementEvidence{}},
		BootGuard:       func(_ context.Context) error { return nil },
		PrivateAgentDir: storeDir(t),
		NetworkStoreDir: storeDir(t),
		OwnerAllowByLink: map[string]bool{
			"testwan0": true, "testlan0": true,
		},
		AdapterFactory: func(ManagedRoutingDeps) (safeapply.Adapter, error) {
			return &syntheticAdapter{live: json.RawMessage(`{"version":1}`)}, nil
		},
	}
}

func openNetworkStoreFor(t *testing.T, dir string) {
	t.Helper()
	raw := json.RawMessage(`{"version":1,"interfaces":[{"id":"inside","name":"testlan0","kind":"physical","role":"lan","mtu":1500,"dhcpClient":"none","management":true,"addresses":["198.51.100.1/24"]}]}`)
	s, err := state.Open(dir, raw, func(raw json.RawMessage) error { _, err := network.Decode(raw); return err })
	if err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
}

func chmod0700(t *testing.T, dir string) {
	t.Helper()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
}

func storeDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	chmod0700(t, dir)
	return dir
}

func TestNewManagedRoutingServiceAdmitsReadyStateWithMatchingEvidence(t *testing.T) {
	deps := managedRoutingDepsFixture(t)
	openNetworkStoreFor(t, deps.NetworkStoreDir)
	deps.Evidence = &recordingEvidenceCollector{out: network.ManagementEvidence{
		Links: []network.ManagementLinkEvidence{
			{Name: "testwan0", Role: "wan", Up: true, Agent: true, HasMAC: true, MTU: 1500},
			{Name: "testlan0", Role: "lan", Up: true, Agent: true, HasMAC: true, MTU: 1500},
		},
		MainAddresses: []network.ManagementAddressEvidence{
			{Link: "testlan0", Addr: netip.MustParsePrefix("198.51.100.1/24"), Agent: true},
		},
		OwnedNeighbors: map[string]bool{},
	}}
	service, err := NewManagedRoutingService(deps)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = service.Close() })
	status, err := service.status()
	if err != nil {
		t.Fatal(err)
	}
	if status.Startup != "ready" {
		t.Fatal("expected ready startup, got", status.Startup)
	}
}

func TestNewManagedRoutingServiceStaysInRecoveryWhenEvidenceDiverges(t *testing.T) {
	deps := managedRoutingDepsFixture(t)
	openNetworkStoreFor(t, deps.NetworkStoreDir)
	deps.Evidence = &recordingEvidenceCollector{out: network.ManagementEvidence{}}
	service, err := NewManagedRoutingService(deps)
	if err == nil {
		t.Fatal("missing evidence was silently accepted")
	}
	if service == nil {
		t.Fatal("service should be returned even on recovery state")
	}
	t.Cleanup(func() { _ = service.Close() })
	status, err := service.status()
	if err != nil {
		t.Fatal(err)
	}
	if status.Startup != "recovery_required" {
		t.Fatal("expected recovery_required, got", status.Startup)
	}
	if status.StartErr == "" {
		t.Fatal("recovery reason was not surfaced")
	}
}

func TestNewManagedRoutingServiceRecoveryKeepsAPIAndClose(t *testing.T) {
	deps := managedRoutingDepsFixture(t)
	openNetworkStoreFor(t, deps.NetworkStoreDir)
	deps.Evidence = &recordingEvidenceCollector{out: network.ManagementEvidence{}}
	service, err := NewManagedRoutingService(deps)
	if err == nil {
		t.Fatal("missing evidence was silently accepted")
	}
	if service == nil {
		t.Fatal("service should be returned even on recovery state")
	}
	if err := service.Close(); err != nil {
		t.Fatal("recovery state service close failed", err)
	}
}

func TestNewManagedRoutingServiceRefusesWithoutEnrollmentOrKernel(t *testing.T) {
	deps := managedRoutingDepsFixture(t)
	deps.BootKernel = nil
	if _, err := NewManagedRoutingService(deps); err == nil {
		t.Fatal("nil kernel was silently accepted")
	}
	deps = managedRoutingDepsFixture(t)
	deps.PrivateAgentDir = ""
	if _, err := NewManagedRoutingService(deps); err == nil {
		t.Fatal("missing private agent dir was silently accepted")
	}
}

func TestNewManagedRoutingServiceRejectsBootGuardFailure(t *testing.T) {
	deps := managedRoutingDepsFixture(t)
	openNetworkStoreFor(t, deps.NetworkStoreDir)
	deps.BootGuard = func(_ context.Context) error { return errBootGuardStub }
	service, err := NewManagedRoutingService(deps)
	if err == nil {
		t.Fatal("boot guard failure was ignored")
	}
	t.Cleanup(func() { service.Close() })
	status, _ := service.status()
	if status.Startup != "recovery_required" {
		t.Fatal("expected recovery_required, got", status.Startup)
	}
}

var errBootGuardStub = &bootGuardError{}

type bootGuardError struct{}

func (*bootGuardError) Error() string { return "boot guard refused (synthetic)" }
