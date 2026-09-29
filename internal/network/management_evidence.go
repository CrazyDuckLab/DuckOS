package network

import "context"

// NetlinkSnapshotter is the linux-only source of link and address
// observations. Non-Linux hosts return an explicit refusal.
type NetlinkSnapshotter interface {
	SnapshotLinks() ([]ManagementLinkEvidence, []ManagementAddressEvidence, error)
	MainRoutes() ([]KernelRoute, error)
}

// ProcNetReader is the linux-only source of TCP/UDP listener observations.
type ProcNetReader interface {
	Listeners() ([]ManagementListenerEvidence, error)
}

// LLDPProbe is an optional LLDP neighbor source. It is allowed to be nil
// on hosts where the probe is not installed.
type LLDPProbe interface {
	Neighbors() ([]ManagementNeighborEvidence, error)
}

// ManagementEvidenceCollector is the production Agent's evidence collector.
// The Linux build wires it to netlink and proc-net; the non-Linux build
// refuses to operate. Callers should treat the collector as opaque and
// always go through the wire-up helpers in internal/agent.
type ManagementEvidenceCollector struct {
	Netlink    NetlinkSnapshotter
	ProcNet    ProcNetReader
	LLDPProbe  LLDPProbe
	OwnerAllow map[string]bool
}

// Collect returns the bounded evidence set. The implementation lives in a
// per-platform file so non-Linux callers never accidentally exercise the
// production path.
func (c ManagementEvidenceCollector) Collect(_ context.Context) (ManagementEvidence, error) {
	return collectManagementEvidence(c)
}
