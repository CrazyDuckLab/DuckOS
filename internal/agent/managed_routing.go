package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sync"

	"github.com/CrazyDuckLab/DuckOS/internal/network"
	"github.com/CrazyDuckLab/DuckOS/internal/safeapply"
	"github.com/CrazyDuckLab/DuckOS/internal/state"
)

// EvidenceCollector is the narrow contract the Agent needs to gather
// management evidence. The production wiring uses a Linux netlink-backed
// collector; tests inject a deterministic synthetic.
type EvidenceCollector interface {
	Collect(ctx context.Context) (network.ManagementEvidence, error)
}

// AdapterFactory builds the safeapply.Adapter the NetworkService uses.
// The production factory wraps network.NewManagedRoutingAdapter; tests
// inject a synthetic that does not need Linux/root.
type AdapterFactory func(deps ManagedRoutingDeps) (safeapply.Adapter, error)

// ManagedRoutingDeps supplies the production Agent with the dependencies it
// needs to wire the real managed routing adapter. Every field is required:
// the Agent must not silently substitute defaults for any of them.
type ManagedRoutingDeps struct {
	Enrollment          network.Enrollment
	BootKernel          network.BootKernel
	Evidence            EvidenceCollector
	BootGuard           func(context.Context) error
	PrivateAgentDir     string
	NetworkStoreDir     string
	OwnerAllowByLink    map[string]bool
	RegisteredNeighbors map[string]bool
	// AdapterFactory is optional. When nil, NewManagedRoutingService uses
	// the production factory that calls network.NewManagedRoutingAdapter.
	AdapterFactory AdapterFactory
}

// NewManagedRoutingService is the R1.4 wiring entry point. It builds the
// real managed routing adapter on top of the second-stage gate, hooks up
// the trusted management verifier, attaches the boot guard and opens a
// durable private network journal. The returned NetworkService is ready to
// serve the API surface; the second-stage gate is run inline so a
// non-ready result is visible from the call site.
func NewManagedRoutingService(deps ManagedRoutingDeps) (*NetworkService, error) {
	if deps.BootKernel == nil {
		return nil, errors.New("managed routing requires an enrolled host kernel")
	}
	if deps.PrivateAgentDir == "" || deps.NetworkStoreDir == "" {
		return nil, errors.New("managed routing requires explicit private agent and network store directories")
	}
	if err := deps.Enrollment.Validate(); err != nil {
		return nil, fmt.Errorf("managed routing enrollment is invalid: %w", err)
	}
	authority := network.NewManagementAuthority(deps.Enrollment)
	verifyManagement := func(ctx context.Context, cfg network.Config) error {
		evidence, err := deps.Evidence.Collect(ctx)
		if err != nil {
			return fmt.Errorf("collect management evidence: %w", err)
		}
		// Merge the optional owner allow list and registered neighbor set
		// into the evidence so the verifier can recognize them.
		if deps.OwnerAllowByLink != nil {
			for i := range evidence.Links {
				evidence.Links[i].Agent = deps.OwnerAllowByLink[evidence.Links[i].Name]
			}
		}
		if deps.RegisteredNeighbors != nil {
			evidence.OwnedNeighbors = make(map[string]bool, len(deps.RegisteredNeighbors))
			for chassis, allowed := range deps.RegisteredNeighbors {
				evidence.OwnedNeighbors[chassis] = allowed
			}
		}
		return network.VerifyManagementOwnership(ctx, authority, cfg, evidence)
	}
	factory := deps.AdapterFactory
	if factory == nil {
		factory = func(d ManagedRoutingDeps) (safeapply.Adapter, error) {
			a, err := network.NewManagedRoutingAdapter(d.PrivateAgentDir, verifyManagement, d.BootGuard)
			if err != nil {
				return nil, err
			}
			// Wire the production adoption LiveLinkAdopter. The
			// netlink-backed adopter is the single source of truth
			// for the adoption boundary on a real host; tests
			// inject a deterministic synthetic through
			// deps.AdapterFactory and so the wire-up here never
			// runs for those. The default/unwired state remains
			// fail-closed: SetLiveLinkAdopter returns nil without
			// admission unless a non-nil adopter is supplied.
			if err := a.SetLiveLinkAdopter(network.NewNetlinkLiveLinkAdopter()); err != nil {
				return nil, err
			}
			return a, nil
		}
	}
	adapter, err := factory(deps)
	if err != nil {
		return nil, err
	}
	validate := func(raw json.RawMessage) error { _, err := network.Decode(raw); return err }
	store, err := state.OpenExisting(deps.NetworkStoreDir, validate)
	if err != nil {
		return nil, err
	}
	controller, err := safeapply.Open(store, adapter)
	if err != nil {
		_ = store.Close()
		return nil, err
	}
	service, err := NewNetworkService(controller)
	if err != nil {
		_ = store.Close()
		return nil, err
	}
	if err := service.StartupVerify(context.Background(), deps.BootKernel, deps.Enrollment); err != nil {
		// Boot-kernel gate failed. Keep the service available so the API
		// surface can report the failure; the second-stage gate stays in
		// recovery_required and refuses to admit any work.
		return service, err
	}
	// Refuse to admit work when the evidence collector is empty at
	// startup. An empty snapshot at construction time means the agent
	// has not yet observed the live kernel; downstream management
	// verification would otherwise pass trivially. Fail-closed here so
	// the API surface can report recovery_required and the operator
	// can investigate before any apply is admitted.
	if err := requireNonEmptyEvidence(context.Background(), deps.Evidence); err != nil {
		service.MarkRecovery(fmt.Errorf("management evidence unavailable at startup: %w", err))
		return service, fmt.Errorf("management evidence unavailable at startup: %w", err)
	}
	// Boot guard is a synchronous check; surface its refusal directly.
	if err := deps.BootGuard(context.Background()); err != nil {
		service.MarkRecovery(fmt.Errorf("boot guard refused startup: %w", err))
		return service, fmt.Errorf("boot guard refused startup: %w", err)
	}
	// The management verifier requires a candidate config; the durable
	// "before" intent is the only one available before any apply. The
	// verifier will refuse if the live evidence disagrees. The placeholder
	// bootstrap snapshot (actor=system, reason=initialize) is exempted —
	// before any apply has occurred there is no candidate to verify; the
	// engine's SeedCurrent replaces the placeholder on the next reconcile.
	beforeRaw, err := store.Snapshot()
	if err != nil {
		service.MarkRecovery(fmt.Errorf("management verifier requires current state: %w", err))
		return service, fmt.Errorf("management verifier requires current state: %w", err)
	}
	if beforeRaw.Current.Actor == "system" && beforeRaw.Current.Reason == "initialize" {
		return service, nil
	}
	cfg, err := network.Decode(beforeRaw.Current.Config)
	if err != nil {
		service.MarkRecovery(fmt.Errorf("management verifier requires a decodable current state: %w", err))
		return service, fmt.Errorf("management verifier requires a decodable current state: %w", err)
	}
	if err := verifyManagement(context.Background(), cfg); err != nil {
		service.MarkRecovery(fmt.Errorf("management verifier refused startup: %w", err))
		return service, fmt.Errorf("management verifier refused startup: %w", err)
	}
	return service, nil
}

// requireNonEmptyEvidence confirms the evidence collector can produce a
// non-empty snapshot before any apply is admitted. An empty snapshot
// means the agent has not yet observed the live kernel; downstream
// management verification would pass trivially and admit requests
// against an unverified system.
func requireNonEmptyEvidence(ctx context.Context, collector EvidenceCollector) error {
	if collector == nil {
		return errors.New("evidence collector is not configured")
	}
	evidence, err := collector.Collect(ctx)
	if err != nil {
		return fmt.Errorf("collect management evidence: %w", err)
	}
	if len(evidence.Links) == 0 && len(evidence.MainAddresses) == 0 && len(evidence.OwnedNeighbors) == 0 {
		return errors.New("evidence snapshot is empty; live kernel not observed")
	}
	return nil
}

// ManagementEvidenceAdapter wires a network.ManagementEvidenceCollector into
// the network service so a single Collector instance is reused across calls.
// The adapter is concurrency-safe and bounded by the underlying collector.
type ManagementEvidenceAdapter struct {
	collector network.ManagementEvidenceCollector
	mu        sync.Mutex
}

func (a *ManagementEvidenceAdapter) Collect(ctx context.Context) (network.ManagementEvidence, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.collector.Collect(ctx)
}

// StandardNetworkPaths is the production layout for the Agent's private
// state. The constants are exported so tests can mirror the same
// directory shape.
const (
	StandardPrivateAgentDir = "/var/lib/duckos-agent"
	StandardNetworkStoreDir = "/var/lib/duckos-agent/network"
)

// ResolveNetworkStateDir returns the on-disk path of the private Agent's
// network journal. Callers must not assume it exists.
func ResolveNetworkStateDir() string {
	return filepath.Join(StandardPrivateAgentDir, "network")
}
