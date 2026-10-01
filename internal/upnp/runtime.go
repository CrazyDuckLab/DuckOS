//go:build linux

// Package upnp: Linux runtime.
//
// The runtime owns the dynamic-mapping surface for UPnP IGD,
// NAT-PMP and PCP. The Manager (upnp.go) is the canonical
// store; the runtime here is the *behaviour* layer that:
//
//   - refuses UPnP / NAT-PMP / PCP on WAN networks
//   - refuses to listen on WAN addresses
//   - enforces TTL on every dynamic mapping
//   - cleans up expired mappings
//   - publishes mapping notifications via a ChannelSink
//   - exposes a NetworkBackend interface so the production
//     wiring can plug in real nftables / miniupnpd integration
//     while tests run against a no-op
//
// Default state is OFF: nothing listens, no mapping exists.
// The runtime is goroutine-safe.
package upnp

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"
)

// NetworkBackend is defined cross-platform in upnp.go so
// the Agent and API packages can refer to it from non-linux
// builds. The Linux runtime in this file consumes the same
// interface.

// MappingEvent is one event on the mapping channel. The
// public API exposes a filtered view; the channel is for
// internal subscribers (audit, Agent log, tests).
type MappingEvent struct {
	Kind     string // "added" | "removed" | "expired"
	Mapping  Mapping
	At       time.Time
	Source   string
}

// Runtime owns the Manager plus a NetworkBackend plus the
// expiry tick.
type Runtime struct {
	mu        sync.Mutex
	mgr       *Manager
	backend   NetworkBackend
	listeners []chan MappingEvent
	stop      chan struct{}
	once      sync.Once
	stopped   bool
	// observedAt is a sink for "freshness" data the public
	// API surfaces.
	observedAt time.Time
}

// NewRuntime wires the Manager and the backend.
func NewRuntime(mgr *Manager, backend NetworkBackend) *Runtime {
	if mgr == nil {
		mgr = NewManager()
	}
	if backend == nil {
		backend = noopBackend{}
	}
	return &Runtime{mgr: mgr, backend: backend, stop: make(chan struct{})}
}

// Subscribe returns a buffered channel of MappingEvents. The
// caller must drain it; otherwise the runtime blocks on emit.
// Test-only; production uses internal Audit hooks.
func (r *Runtime) Subscribe(buf int) <-chan MappingEvent {
	if buf <= 0 {
		buf = 16
	}
	ch := make(chan MappingEvent, buf)
	r.mu.Lock()
	r.listeners = append(r.listeners, ch)
	r.mu.Unlock()
	return ch
}

// Start runs the expiry tick until Close.
func (r *Runtime) Start(ctx context.Context) {
	go r.expiryLoop(ctx)
}

// Close stops the runtime. After Close, Subscribe channels are
// NOT closed (callers do not read from closed channels cleanly);
// the runtime simply stops emitting.
func (r *Runtime) Close() {
	r.once.Do(func() { close(r.stop) })
	r.mu.Lock()
	r.stopped = true
	r.mu.Unlock()
}

// EnableNetwork registers a network for UPnP / NAT-PMP / PCP.
// The runtime refuses networks whose interface resolves to a
// non-private address — the production check is in the
// backend's AllowListen, the runtime applies the policy
// eagerly.
func (r *Runtime) EnableNetwork(n Network, iface string) error {
	if err := n.Valid(); err != nil {
		return err
	}
	if iface != "" && !r.backend.AllowListen(iface, nil) {
		return errors.New("upnp: network interface is not trusted (WAN or unknown)")
	}
	if err := r.mgr.EnableNetwork(n); err != nil {
		return err
	}
	r.mu.Lock()
	r.observedAt = time.Now().UTC()
	r.mu.Unlock()
	r.emit(MappingEvent{Kind: "network-enabled", At: r.observedAt, Source: "runtime"})
	return nil
}

// DisableNetwork removes the network. All mappings on the
// network are revoked and the backend is asked to remove them.
func (r *Runtime) DisableNetwork(ref string) {
	r.mgr.DisableNetwork(ref)
	for _, m := range r.mgr.Mappings() {
		if m.NetworkRef != ref {
			continue
		}
		_ = r.backend.Remove(m.ID)
		r.mgr.RevokeMapping(m.ID)
		r.emit(MappingEvent{Kind: "removed", Mapping: m, At: time.Now().UTC(), Source: "runtime"})
	}
	r.mu.Lock()
	r.observedAt = time.Now().UTC()
	r.mu.Unlock()
}

// Networks returns the current enabled networks.
func (r *Runtime) Networks() []Network {
	return r.mgr.Networks()
}

// Mappings returns the current mappings.
func (r *Runtime) Mappings() []Mapping {
	return r.mgr.Mappings()
}

// AddMapping records a dynamic mapping and asks the backend
// to install it. If the backend refuses, the Manager state is
// rolled back. The Source field of the mapping is set to
// "runtime" when the runtime itself adds it (for example from
// a daemon event), or to the caller-provided string otherwise.
func (r *Runtime) AddMapping(m Mapping, source string) error {
	if err := m.Valid(); err != nil {
		return err
	}
	if !r.mgr.IsEnabled(m.NetworkRef) {
		return errors.New("upnp: network is not enabled for the protocol")
	}
	net := r.findNetwork(m.NetworkRef)
	if net == nil {
		return errors.New("upnp: network is not enabled")
	}
	if !protocolAllowed(net, m.Protocol) {
		return fmt.Errorf("upnp: protocol %s is not enabled on %s", m.Protocol, m.NetworkRef)
	}
	if m.ExpiresAt.IsZero() {
		ttl := time.Duration(net.TTLSec) * time.Second
		if ttl <= 0 {
			ttl = 1 * time.Hour
		}
		m.ExpiresAt = time.Now().UTC().Add(ttl)
	}
	if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now().UTC()
	}
	if source == "" {
		source = "runtime"
	}
	m.Source = source
	if err := r.backend.Add(m); err != nil {
		return err
	}
	if err := r.mgr.AddMapping(m); err != nil {
		_ = r.backend.Remove(m.ID)
		return err
	}
	r.mu.Lock()
	r.observedAt = time.Now().UTC()
	r.mu.Unlock()
	r.emit(MappingEvent{Kind: "added", Mapping: m, At: r.observedAt, Source: source})
	return nil
}

// RevokeMapping removes the mapping and tells the backend.
func (r *Runtime) RevokeMapping(id string) {
	for _, m := range r.mgr.Mappings() {
		if m.ID == id {
			_ = r.backend.Remove(id)
			r.mgr.RevokeMapping(id)
			r.mu.Lock()
			r.observedAt = time.Now().UTC()
			r.mu.Unlock()
			r.emit(MappingEvent{Kind: "removed", Mapping: m, At: r.observedAt, Source: "operator"})
			return
		}
	}
}

// ObservedAt returns the last time the runtime state changed.
func (r *Runtime) ObservedAt() time.Time {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.observedAt
}

// Stats returns a small summary useful for the public API.
func (r *Runtime) Stats() RuntimeStats {
	r.mu.Lock()
	defer r.mu.Unlock()
	return RuntimeStats{
		Networks:   len(r.mgr.networks),
		Mappings:   len(r.mgr.mappings),
		ObservedAt: r.observedAt,
		Stopped:    r.stopped,
		Backend:    r.backendState(),
	}
}

// backendState returns the BackendState that mirrors the
// runtime's current NetworkBackend. Production wiring
// uses NftablesBackend (linux) or UnavailableBackend
// (linux with no nft on PATH). Tests inject NftablesBackend
// directly to assert the production wiring chose the right
// one.
func (r *Runtime) backendState() BackendState {
	switch b := r.backend.(type) {
	case *UnavailableBackend:
		reason := b.Reason()
		return BackendState{
			Kind:       BackendKindUnavailable,
			Reason:     reason.Reason,
			Since:      reason.Since,
			Diagnostic: reason.Diagnostic,
		}
	case *NftablesBackend:
		return BackendState{Kind: BackendKindNftables}
	}
	return BackendState{Kind: "unknown"}
}

// IsNftablesBackend reports whether the runtime is wired
// to the real nftables backend. Production code uses it
// at startup to refuse to bring up the runtime when nft is
// unavailable; tests use it to assert that production
// wiring never selects the package-private test fixture.
func (r *Runtime) IsNftablesBackend() bool {
	_, ok := r.backend.(*NftablesBackend)
	return ok
}

// IsUnavailableBackend reports whether the runtime is
// wired to the production UnavailableBackend.
func (r *Runtime) IsUnavailableBackend() bool {
	_, ok := r.backend.(*UnavailableBackend)
	return ok
}

// RuntimeStats is defined cross-platform in upnp.go so
// the Agent and API packages can reference it from
// non-linux builds. The Stats() method is here.

// findNetwork returns a copy of the network entry or nil.
func (r *Runtime) findNetwork(ref string) *Network {
	for _, n := range r.mgr.Networks() {
		if n.NetworkRef == ref {
			return &n
		}
	}
	return nil
}

func protocolAllowed(n *Network, p Protocol) bool {
	for _, q := range n.Protocols {
		if q == p {
			return true
		}
	}
	return false
}

// emit broadcasts one event to every subscriber.
func (r *Runtime) emit(e MappingEvent) {
	r.mu.Lock()
	listeners := append([]chan MappingEvent(nil), r.listeners...)
	r.mu.Unlock()
	for _, ch := range listeners {
		select {
		case ch <- e:
		default:
			// drop on slow consumer rather than block the runtime
		}
	}
}

// expiryLoop expires mappings whose TTL has elapsed. The
// loop sleeps in 1 s ticks so even short TTLs are honoured
// within a second.
func (r *Runtime) expiryLoop(ctx context.Context) {
	tick := time.NewTicker(1 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-r.stop:
			return
		case <-tick.C:
			r.sweep()
		}
	}
}

// sweep removes expired mappings. It is also invoked when
// the runtime is closed.
func (r *Runtime) sweep() {
	now := time.Now().UTC()
	for _, m := range r.mgr.Mappings() {
		if !m.ExpiresAt.IsZero() && now.After(m.ExpiresAt) {
			_ = r.backend.Remove(m.ID)
			r.mgr.RevokeMapping(m.ID)
			r.emit(MappingEvent{Kind: "expired", Mapping: m, At: now, Source: "expiry"})
		}
	}
	r.mu.Lock()
	r.observedAt = now
	r.mu.Unlock()
}

// noopBackend is the test / default backend.
type noopBackend struct{}

func (noopBackend) Add(Mapping) error                { return nil }
func (noopBackend) Remove(string) error              { return nil }
func (noopBackend) AllowListen(string, net.Addr) bool { return false }

// WanRefused is the error returned when a UPnP / NAT-PMP /
// PCP listener is asked to bind a WAN address. Production
// wiring checks the firewall zone table before invoking the
// backend.
var WanRefused = errors.New("upnp: WAN listeners are structurally refused")

// FormatMapping is a tiny helper for the public API renderer.
func FormatMapping(m Mapping) string {
	return fmt.Sprintf("%s/%s ext=%d -> %s:%d expires=%s",
		m.Protocol, m.NetworkRef, m.ExternalPort, m.InternalHost, m.InternalPort, m.ExpiresAt.Format(time.RFC3339))
}