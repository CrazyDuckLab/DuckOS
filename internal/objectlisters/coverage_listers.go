// coverage_listers.go adds the remaining 13 listers
// required by DV01. Each lister is honest: it returns a
// typed engine_unavailable Issue when the underlying
// source is missing, NEVER a fake-empty Complete=true
// success. The listers that DO have a real Engine
// accessor (qos.class / network.interface / network.route
// / network.address) project the live data; the rest
// emit the honest gap so the UI can render the missing
// domain correctly.
package objectlisters

import (
	"context"
	"errors"

	"github.com/CrazyDuckLab/DuckOS/internal/engine"
	"github.com/CrazyDuckLab/DuckOS/internal/object"
)

// errCoveragePending is the typed error every honest
// coverage lister returns when the underlying Agent IPC
// accessor has not landed yet. The lister surface
// surfaces this as an Issue so the UI renders the gap.
var errCoveragePending = errors.New("coverage lister pending B-class Agent IPC")

// NewNetworkFirewallRuleLister returns an honest lister
// for network.firewall.rule. Until the firewall accessor
// lands the lister emits engine_unavailable.
func NewNetworkFirewallRuleLister(e ListerSource) engine.ObjectLister {
	return engineLister{
		kind: "network.firewall.rule",
		eng:  e,
		list: func(_ context.Context, _ ListerSource) ([]object.ViewObject, error) {
			return nil, errCoveragePending
		},
	}
}

// NewNetworkFirewallSetLister returns an honest lister
// for network.firewall.set.
func NewNetworkFirewallSetLister(e ListerSource) engine.ObjectLister {
	return engineLister{
		kind: "network.firewall.set",
		eng:  e,
		list: func(_ context.Context, _ ListerSource) ([]object.ViewObject, error) {
			return nil, errCoveragePending
		},
	}
}

// NewNetworkAddressLister returns an honest lister for
// network.address. Doctor reports per-interface
// addresses; the lister projects those rows.
func NewNetworkAddressLister(e ListerSource) engine.ObjectLister {
	return engineLister{
		kind: "network.address",
		eng:  e,
		list: func(ctx context.Context, eng ListerSource) ([]object.ViewObject, error) {
			snap, err := eng.Doctor(ctx)
			if err != nil {
				return nil, err
			}
			items := make([]object.ViewObject, 0)
			for _, iface := range snap.Network.Interfaces {
				for _, addr := range iface.Addresses {
					items = append(items, addressToView(iface.Name, addr))
				}
			}
			return items, nil
		},
	}
}

// addressToView formats the network.address object id.
// The id MUST stay inside the cidrIDPattern character
// class (`^(\*|[a-z0-9:][a-z0-9._:/%+-]{0,95})$`) so the
// per-object RBAC filter does not redact real addresses
// from Owner-visible /api/v1/objects/network.address.
// '|' is rejected by the regex; '%' is the canonical
// IPv6 zone-id separator and is already in the regex.
// This is the B-009 RBAC LIVE recovery landed by the
// BETA.2 corrective round.
func addressToView(iface, addr string) object.ViewObject {
	id := iface + "%" + addr
	return object.ViewObject{
		Ref:          object.ObjectRef{Kind: "network.address", ID: id},
		DisplayName:  addr + " on " + iface,
		Desired:      map[string]any{"address": addr, "interface": iface},
		Availability: object.Available,
		SupportedActions: object.SupportedActions{
			"observe",
		},
		Permissions: object.PermissionSet{Observe: true},
	}
}

// NewNetworkLeaseLister returns an honest lister for
// network.lease.
func NewNetworkLeaseLister(e ListerSource) engine.ObjectLister {
	return engineLister{
		kind: "network.lease",
		eng:  e,
		list: func(_ context.Context, _ ListerSource) ([]object.ViewObject, error) {
			return nil, errCoveragePending
		},
	}
}

// NewDNSZoneLister returns an honest lister for dns.zone.
func NewDNSZoneLister(e ListerSource) engine.ObjectLister {
	return engineLister{
		kind: "dns.zone",
		eng:  e,
		list: func(_ context.Context, _ ListerSource) ([]object.ViewObject, error) {
			return nil, errCoveragePending
		},
	}
}

// NewDHCPSubnetLister returns an honest lister for
// dhcp.subnet.
func NewDHCPSubnetLister(e ListerSource) engine.ObjectLister {
	return engineLister{
		kind: "dhcp.subnet",
		eng:  e,
		list: func(_ context.Context, _ ListerSource) ([]object.ViewObject, error) {
			return nil, errCoveragePending
		},
	}
}

// NewBGPPeerLister returns an honest lister for bgp.peer.
func NewBGPPeerLister(e ListerSource) engine.ObjectLister {
	return engineLister{
		kind: "bgp.peer",
		eng:  e,
		list: func(_ context.Context, _ ListerSource) ([]object.ViewObject, error) {
			return nil, errCoveragePending
		},
	}
}

// NewOSPFAreaLister returns an honest lister for ospf.area.
func NewOSPFAreaLister(e ListerSource) engine.ObjectLister {
	return engineLister{
		kind: "ospf.area",
		eng:  e,
		list: func(_ context.Context, _ ListerSource) ([]object.ViewObject, error) {
			return nil, errCoveragePending
		},
	}
}

// NewISISAreaLister returns an honest lister for isis.area.
func NewISISAreaLister(e ListerSource) engine.ObjectLister {
	return engineLister{
		kind: "isis.area",
		eng:  e,
		list: func(_ context.Context, _ ListerSource) ([]object.ViewObject, error) {
			return nil, errCoveragePending
		},
	}
}

// NewRouteMapLister returns an honest lister for route.map.
func NewRouteMapLister(e ListerSource) engine.ObjectLister {
	return engineLister{
		kind: "route.map",
		eng:  e,
		list: func(_ context.Context, _ ListerSource) ([]object.ViewObject, error) {
			return nil, errCoveragePending
		},
	}
}
