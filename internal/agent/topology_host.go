package agent

import (
	"context"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/topology"
)

// TopologyHost is the Agent-facing entry point that builds the
// relationship graph. The Host is intentionally read-only: it
// composes a graph from in-memory state (interfaces, devices,
// tunnels) and returns a deterministic result. Tests can swap
// any of the providers for a fake.
type TopologyHost struct {
	now        func() time.Time
	interfaces InterfaceProvider
	devices    DeviceProvider
	wireguard  WireGuardProvider
	xray       XrayProvider
}

// InterfaceProvider returns the live interface list. The Host
// is provider-agnostic; the privileged Agent supplies a
// production implementation that reads /sys/class/net.
type InterfaceProvider interface {
	Interfaces(context.Context) ([]topology.InterfaceSpec, error)
}

// DeviceProvider returns the live device aggregate. The Host
// only consumes counts and stable ids, never keys.
type DeviceProvider interface {
	Devices(context.Context) ([]topology.DeviceSpec, error)
}

// WireGuardProvider returns the live WireGuard aggregate. The
// Host only consumes the server and peer names; key material
// never enters the graph.
type WireGuardProvider interface {
	WireGuardServers(context.Context) ([]topology.WireGuardSpec, error)
}

// XrayProvider returns the live Xray aggregate.
type XrayProvider interface {
	XrayServers(context.Context) ([]topology.XraySpec, error)
}

// NewTopologyHost wires the default provider set. All providers
// are optional; a nil provider simply produces an empty graph
// section, never a failure.
func NewTopologyHost() *TopologyHost {
	return &TopologyHost{now: time.Now}
}

// Topology builds the canonical relationship graph. Sources run
// sequentially; the Builder is concurrency-safe, so the Agent
// may call Topology from multiple goroutines.
func (h *TopologyHost) Topology(ctx context.Context) (topology.Graph, error) {
	if h == nil {
		return topology.Graph{}, nil
	}
	if err := ctx.Err(); err != nil {
		return topology.Graph{}, err
	}
	b := topology.NewBuilder(h.now().Unix())
	if h.interfaces != nil {
		if err := ctx.Err(); err != nil {
			return topology.Graph{}, err
		}
		if specs, err := h.interfaces.Interfaces(ctx); err == nil {
			for _, s := range specs {
				b.AddInterface(s)
			}
		}
	}
	if h.devices != nil {
		if err := ctx.Err(); err != nil {
			return topology.Graph{}, err
		}
		if specs, err := h.devices.Devices(ctx); err == nil {
			for _, s := range specs {
				b.AddDevice(s)
			}
		}
	}
	if h.wireguard != nil {
		if err := ctx.Err(); err != nil {
			return topology.Graph{}, err
		}
		if specs, err := h.wireguard.WireGuardServers(ctx); err == nil {
			for _, s := range specs {
				b.AddWireGuard(s)
			}
		}
	}
	if h.xray != nil {
		if err := ctx.Err(); err != nil {
			return topology.Graph{}, err
		}
		if specs, err := h.xray.XrayServers(ctx); err == nil {
			for _, s := range specs {
				b.AddXray(s)
			}
		}
	}
	graph := b.Snapshot()
	return graph, graph.Valid()
}
