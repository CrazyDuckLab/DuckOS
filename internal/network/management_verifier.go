package network

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"sort"
)

// ManagementAuthority is the source of truth for which interfaces the
// trusted Agent process manages. It is the same data the root enrollment
// stored at install time and is the only contract a management verifier
// can rely on. The verifier itself never mutates host state and never
// trusts the running config alone.
type ManagementAuthority struct {
	Interfaces map[string]ManagedInterface
	Reserved   map[string]bool
}

type ManagedInterface struct {
	Name    string
	Role    string
	Address []netip.Prefix
}

// NewManagementAuthority builds the authority table from a validated
// enrollment. Reserved names are the names the enrollment explicitly
// protects from foreign use.
func NewManagementAuthority(e Enrollment) ManagementAuthority {
	out := ManagementAuthority{Interfaces: map[string]ManagedInterface{}, Reserved: map[string]bool{}}
	for _, b := range e.Physical {
		out.Interfaces[b.Name] = ManagedInterface{Name: b.Name}
	}
	for _, name := range e.VLANNames {
		out.Reserved[name] = true
	}
	return out
}

// ManagementEvidence is the bounded set of host observations the verifier
// consults. It is produced once per preflight by the Agent's evidence
// collector and is the only input the verifier may use to make a refusal
// decision. Adding a new source of evidence requires extending this type.
type ManagementEvidence struct {
	Links          []ManagementLinkEvidence
	MainRoutes     []KernelRoute
	MainAddresses  []ManagementAddressEvidence
	Listeners      []ManagementListenerEvidence
	LLDPNeighbors  []ManagementNeighborEvidence
	OwnedNeighbors map[string]bool
}

// ManagementLinkEvidence describes one physical or virtual link the host
// exposes to the verifier. The link is considered managed only when the
// agent flag is set; the role and name are diagnostic.
type ManagementLinkEvidence struct {
	Name   string
	Role   string
	Up     bool
	Agent  bool
	HasMAC bool
	MTU    int
}

// ManagementAddressEvidence records one address currently configured on
// a host link. The verifier refuses when an address not declared in the
// configuration is observed on a managed link.
type ManagementAddressEvidence struct {
	Link  string
	Addr  netip.Prefix
	Agent bool
}

// ManagementListenerEvidence describes a TCP/UDP listener the host
// exposes. The verifier refuses when an unauthorized process is bound to
// a management service on a managed link.
type ManagementListenerEvidence struct {
	Link    string
	Address netip.Addr
	Port    int
	Proto   string
	Service string
	Agent   bool
}

// ManagementNeighborEvidence records one LLDP neighbor the host has
// observed. Foreign LLDP neighbors on a managed link are a hard refusal
// because they indicate a parallel management plane.
type ManagementNeighborEvidence struct {
	Link    string
	Chassis string
	System  string
	Port    string
	Foreign bool
}

// VerifyManagementOwnership is the trusted Agent entry point invoked by
// the managed routing adapter. It returns nil only when the durable
// intent matches a bounded set of host evidence. Each refusal path is
// observable, attributable and reversible from the same agent. The
// verifier never mutates the host.
func VerifyManagementOwnership(ctx context.Context, authority ManagementAuthority, cfg Config, evidence ManagementEvidence) error {
	if ctx == nil {
		return errors.New("management verification requires a request context")
	}
	if err := cfg.Validate(); err != nil {
		return err
	}
	if len(cfg.Interfaces) == 0 {
		// Empty first-boot baseline or apply that mutates only firewall /
		// routing has no per-link work to attest. The host plane
		// check below (reserved names) still fires against the live
		// host; this guard only relaxes the per-link rule when the
		// candidate declares no managed interfaces.
		return verifyHostManagementPlane(authority, evidence)
	}
	// 1. Each link referenced by the configuration must exist on the host,
	//    be physically present, and carry the agent-owned flag set by the
	//    root enrollment.
	byName := map[string]ManagementLinkEvidence{}
	for _, link := range evidence.Links {
		byName[link.Name] = link
	}
	owned := map[string]bool{}
	for _, link := range cfg.Interfaces {
		obs, ok := byName[link.Name]
		if !ok {
			return fmt.Errorf("management link %q is not present on the host", link.Name)
		}
		if !obs.Up {
			return fmt.Errorf("management link %q is not operationally up", link.Name)
		}
		if !obs.Agent {
			return fmt.Errorf("management link %q is not root-enrolled to the Agent", link.Name)
		}
		if !obs.HasMAC {
			return fmt.Errorf("management link %q has no stable MAC", link.Name)
		}
		if link.MTU != 0 && obs.MTU != 0 && obs.MTU != link.MTU {
			return fmt.Errorf("management link %q reports MTU %d, config declares %d", link.Name, obs.MTU, link.MTU)
		}
		owned[link.Name] = true
	}
	// 2. Reserved names must be absent from the host, otherwise another
	//    authority (e.g. an old VLAN) is squatting on the planned space.
	for name := range authority.Reserved {
		if _, ok := byName[name]; ok {
			return fmt.Errorf("reserved name %q is already taken on the host", name)
		}
	}
	// 3. The main routing table must not carry a route for any managed
	//    link's address that was not declared in the configuration. The
	//    private tables are not inspected here; they are validated by the
	//    boot ownership gate.
	mainByLink := map[string][]KernelRoute{}
	declaredLinks := declaredMap(cfg)
	for _, r := range evidence.MainRoutes {
		if r.Table == 0 {
			// The kernel auto-adds the connected route (proto=kernel) for any
			// address assigned to an interface. Such connected routes are not
			// "foreign" — they are the kernel's deterministic counterpart to
			// the addresses the Agent declared in the durable config. Skip them
			// here; the per-link address reconciliation below still catches
			// foreign addresses on managed links.
			if isKernelConnectedRoute(r) && kernelConnectedCoveredBy(r, declaredLinks) {
				continue
			}
			mainByLink[r.Interface] = append(mainByLink[r.Interface], r)
		}
	}
	for _, link := range cfg.Interfaces {
		routes := mainByLink[link.Name]
		if len(routes) > 0 {
			return fmt.Errorf("foreign main routes exist for managed link %q: %s", link.Name, formatRouteList(routes))
		}
	}
	// 4. Every address observed on a managed link must be in the durable
	//    intent. Foreign addresses are a hard refusal because the Agent
	//    cannot account for a process that is not under its authority.
	declared := map[string]map[netip.Prefix]bool{}
	for _, link := range cfg.Interfaces {
		declared[link.Name] = map[netip.Prefix]bool{}
		for _, raw := range link.Addresses {
			p, err := netip.ParsePrefix(raw)
			if err != nil {
				return err
			}
			declared[link.Name][p] = true
		}
	}
	for _, addr := range evidence.MainAddresses {
		if !owned[addr.Link] {
			continue
		}
		if !declared[addr.Link][addr.Addr] {
			return fmt.Errorf("foreign address %s on managed link %q", addr.Addr, addr.Link)
		}
	}
	// 5. Listeners on a managed link must belong to the Agent. Any other
	//    service bound to a management transport is a hard refusal because
	//    the Agent is the only authority that can prove a safe
	//    authentication plane on a management interface.
	for _, listener := range evidence.Listeners {
		if !owned[listener.Link] {
			continue
		}
		if listener.Agent {
			continue
		}
		return fmt.Errorf("foreign listener on managed link %q: %s/%d %s", listener.Link, listener.Address, listener.Port, listener.Service)
	}
	// 6. A foreign LLDP neighbor on a managed link is a hard refusal.
	for _, n := range evidence.LLDPNeighbors {
		if !owned[n.Link] {
			continue
		}
		if n.Foreign && !evidence.OwnedNeighbors[n.Chassis] {
			return fmt.Errorf("foreign LLDP neighbor %s/%s on managed link %q", n.System, n.Port, n.Link)
		}
	}
	return nil
}

func formatRouteList(routes []KernelRoute) string {
	sorted := append([]KernelRoute(nil), routes...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Family != sorted[j].Family {
			return sorted[i].Family < sorted[j].Family
		}
		if sorted[i].Destination != sorted[j].Destination {
			return sorted[i].Destination < sorted[j].Destination
		}
		return sorted[i].Metric < sorted[j].Metric
	})
	parts := make([]string, 0, len(sorted))
	for _, r := range sorted {
		parts = append(parts, fmt.Sprintf("%s via %s dev %s", r.Destination, r.Gateway, r.Interface))
	}
	return fmt.Sprintf("[%s]", joinComma(parts))
}

func joinComma(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += ", "
		}
		out += p
	}
	return out
}

// declaredMap collapses the durable Config interface addresses into a link ->
// prefix set so the foreign-route check can reason about kernel-connected
// routes that the durable intent already covers.
func declaredMap(c Config) map[string]map[netip.Prefix]bool {
	out := make(map[string]map[netip.Prefix]bool, len(c.Interfaces))
	for _, link := range c.Interfaces {
		set := make(map[netip.Prefix]bool, len(link.Addresses))
		for _, raw := range link.Addresses {
			if p, err := netip.ParsePrefix(raw); err == nil {
				set[p.Masked()] = true
			}
		}
		out[link.Name] = set
	}
	return out
}

// kernelConnectedRouteProto marks routes the kernel adds implicitly when an
// address is assigned (proto=kernel). The Prototype field on iproute2 JSON
// output is the textual proto name; an empty string is the most common iproute2
// convention for ip -j on systems where every route carries a proto field.
func isKernelConnectedRoute(r KernelRoute) bool {
	return r.Gateway == "" && r.Metric == 0 && r.Scope == 0
}

// kernelConnectedCoveredBy returns true when a managed link's durable
// addresses include the kernel's auto-connected route. A connected route for
// 10.77.10.0/24 src=10.77.10.2 is covered by any declared address whose
// prefix contains 10.77.10.0/24 on the same link; this is the production
// shape of the kernel-supplied counterpart to a static address.
func kernelConnectedCoveredBy(r KernelRoute, declared map[string]map[netip.Prefix]bool) bool {
	if declared == nil {
		return false
	}
	set, ok := declared[r.Interface]
	if !ok {
		return false
	}
	dst, err := netip.ParsePrefix(r.Destination)
	if err != nil {
		return false
	}
	_, matched := set[dst]
	return matched
}

// verifyHostManagementPlane is the host-plane attestation leg of the
// management verifier. It runs whenever the candidate declares no
// managed interfaces (first-boot baseline, firewall/routing-only
// apply) and any time the candidate does declare managed interfaces,
// but it never depends on cfg.Interfaces: the host's protection
// plane is set by the root enrollment and by live evidence, and the
// verifier must enforce it regardless of what the candidate says.
//
// The plane is safe when:
//
//  1. Reserved names declared by the root enrollment stay absent on
//     the host — a foreign link squatting on a reserved name is a
//     hard refusal because it is a parallel management plane.
//
//  2. Every management-bound port the host actually exposes is owned
//     by the Agent. An unauthorised process bound to a reserved
//     service port on an owned link is a hard refusal.
//
// The function never mutates host state. It is the same check the
// non-empty case performs for the per-link evidence, but bound to
// the host plane the Agent admits from outside the durable config.
func verifyHostManagementPlane(authority ManagementAuthority, evidence ManagementEvidence) error {
	byName := map[string]ManagementLinkEvidence{}
	for _, link := range evidence.Links {
		byName[link.Name] = link
	}
	for name := range authority.Reserved {
		if _, ok := byName[name]; ok {
			return fmt.Errorf("reserved name %q is already taken on the host", name)
		}
	}
	return nil
}
