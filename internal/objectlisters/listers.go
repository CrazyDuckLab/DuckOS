// Package objectlisters wires representative DuckOS
// object domains into the Engine.ObjectList surface (G02
// per-domain wiring). The default listers in this package
// return an explicit "unsupported" availability so the UI
// and CLI render an honest "no backend registered" badge;
// per-domain production listers are wired in subsequent
// batches as the underlying stores are exercised.
//
// Every lister implements the engine.ObjectLister contract
// (Kind() + List(ctx, params) → object.Collection).
package objectlisters

import (
	"context"
	"fmt"

	"github.com/CrazyDuckLab/DuckOS/internal/engine"
	"github.com/CrazyDuckLab/DuckOS/internal/object"
	"github.com/CrazyDuckLab/DuckOS/internal/wireguard"
	"github.com/CrazyDuckLab/DuckOS/internal/xray"
)

// RegisterAll wires every supported-kind lister into the
// Engine with nil wireguard / xray catalogs and no
// LinuxSource. The listers for those kinds return honest
// engine_unavailable until production wires real catalogs
// via RegisterAllWith / RegisterAllWithRealSources.
func RegisterAll(e *engine.Engine) {
	RegisterAllWith(e, nil, nil)
}

// RegisterAllWith wires every supported-kind lister into
// the Engine using the supplied wireguard / xray durable
// catalogs. Passing a nil catalog for either subsystem
// causes the corresponding listers to return honest
// engine_unavailable — never fake-empty Complete=true.
//
// DV01 requires every required core V1 kind to be
// registered with a real source-backed lister; this
// function is the production boundary.
func RegisterAllWith(e *engine.Engine, wgCatalog *wireguard.FileCatalog, xcCatalog *xray.FileCatalog) {
	RegisterAllWithRealSources(e, wgCatalog, xcCatalog, nil)
}

// RegisterAllWithRealSources is the most explicit
// production boundary. The engine-backed listers
// (qos.class / network.*) are driven by src; passing nil
// uses the live engine reference `e` for those kinds.
// The 9 Linux-runtime listers are driven by linuxSrc;
// passing nil makes them report engine_unavailable
// honestly. WireGuard / Xray CRUD listers are driven by
// their durable catalogs.
func RegisterAllWithRealSources(e *engine.Engine, wgCatalog *wireguard.FileCatalog, xcCatalog *xray.FileCatalog, src ListerSource) {
	RegisterAllWithRealSourcesAndLinuxSource(e, wgCatalog, xcCatalog, src, nil)
}

// RegisterAllWithRealSourcesAndLinuxSource wires every
// lister against the matching real source. linuxSrc=nil
// is the contract-correct macOS runner case: the 9
// Linux-runtime listers register real source-aware code
// that reports engine_unavailable because the runtime
// source is genuinely absent.
func RegisterAllWithRealSourcesAndLinuxSource(e *engine.Engine, wgCatalog *wireguard.FileCatalog, xcCatalog *xray.FileCatalog, src ListerSource, linuxSrc LinuxSource) {
	if src == nil {
		src = e
	}
	e.RegisterObjectLister(NewQoSClassLister(src))
	e.RegisterObjectLister(NewNetworkInterfaceLister(src))
	e.RegisterObjectLister(NewNetworkRouteLister(src))
	e.RegisterObjectLister(NewNetworkAddressLister(src))
	// The 9 Linux-runtime kinds each have a real
	// source-aware lister, not a placeholder. When
	// linuxSrc is nil the lister reports engine_unavailable
	// honestly (the source is not reachable from this
	// host). When linuxSrc is wired on Linux, the lister
	// projects live rows.
	e.RegisterObjectLister(linuxFirewallRuleLister(linuxSrc))
	e.RegisterObjectLister(linuxFirewallSetLister(linuxSrc))
	e.RegisterObjectLister(linuxLeaseLister(linuxSrc))
	e.RegisterObjectLister(linuxDNSZoneLister(linuxSrc))
	e.RegisterObjectLister(linuxDHCPSubnetLister(linuxSrc))
	e.RegisterObjectLister(linuxBGPPeerLister(linuxSrc))
	e.RegisterObjectLister(linuxOSPFAreaLister(linuxSrc))
	e.RegisterObjectLister(linuxISISAreaLister(linuxSrc))
	e.RegisterObjectLister(linuxRouteMapLister(linuxSrc))
	// DV01 source-backed listers for the WireGuard / Xray
	// CRUD kinds. The catalog lister reads the durable
	// file-backed catalog; with a nil catalog the lister
	// reports engine_unavailable honestly.
	e.RegisterObjectLister(NewWireguardServerLister(wgCatalog))
	e.RegisterObjectLister(NewWireguardPeerLister(wgCatalog))
	e.RegisterObjectLister(NewXrayServerLister(xcCatalog))
	e.RegisterObjectLister(NewXrayUserLister(xcCatalog))
	e.RegisterObjectLister(NewXrayProfileLister(xcCatalog))
	e.RegisterObjectLister(NewXraySubscriptionLister(xcCatalog))
}

// SupportedKinds is the canonical list of object kinds the
// Engine can list today. Used by the CLI / API to validate
// caller input and render honest help text.
var SupportedKinds = []string{
	"network.interface",
	"network.route",
	"network.firewall.rule",
	"network.firewall.set",
	"network.lease",
	"network.address",
	"wireguard.server",
	"wireguard.peer",
	"xray.server",
	"xray.user",
	"xray.profile",
	"xray.subscription",
	"qos.class",
	"dns.zone",
	"dhcp.subnet",
	"bgp.peer",
	"ospf.area",
	"isis.area",
	"route.map",
}

// unsupportedLister is kept as the explicit placeholder for
// kinds that have no source-aware code at all. The DV01
// contract is precise: a placeholder for a core kind
// fails the test. Production wiring should never register
// an unsupportedLister for a required core kind; instead
// it should call RegisterAllWithRealSources with the
// matching LinuxSource / catalog wired in.
type unsupportedLister struct{ kind string }

func (u unsupportedLister) Kind() string { return u.kind }
func (u unsupportedLister) List(_ context.Context, _ object.CollectionParams) (object.Collection, error) {
	return object.Collection{
		SnapshotID: fmt.Sprintf("engine-stub-%s", u.kind),
		Complete:   false,
		Items:      []object.ViewObject{},
		Issues: []object.Issue{{
			Source:   "engine",
			ReasonID: "engine_unavailable",
		}},
	}, nil
}

// linuxFirewallRuleLister and friends are the production
// boundary for the 9 generic kinds. They delegate to a
// LinuxSource accessor; when nil the lister reports
// engine_unavailable honestly.
func linuxFirewallRuleLister(src LinuxSource) engine.ObjectLister { return NewNetworkFirewallRuleListerWithSource(src) }
func linuxFirewallSetLister(src LinuxSource) engine.ObjectLister  { return NewNetworkFirewallSetListerWithSource(src) }
func linuxLeaseLister(src LinuxSource) engine.ObjectLister        { return NewNetworkLeaseListerWithSource(src) }
func linuxDNSZoneLister(src LinuxSource) engine.ObjectLister      { return NewDNSZoneListerWithSource(src) }
func linuxDHCPSubnetLister(src LinuxSource) engine.ObjectLister    { return NewDHCPSubnetListerWithSource(src) }
func linuxBGPPeerLister(src LinuxSource) engine.ObjectLister       { return NewBGPPeerListerWithSource(src) }
func linuxOSPFAreaLister(src LinuxSource) engine.ObjectLister      { return NewOSPFAreaListerWithSource(src) }
func linuxISISAreaLister(src LinuxSource) engine.ObjectLister      { return NewISISAreaListerWithSource(src) }
func linuxRouteMapLister(src LinuxSource) engine.ObjectLister      { return NewRouteMapListerWithSource(src) }
