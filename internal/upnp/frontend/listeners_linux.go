//go:build linux

// listeners_linux.go: the Linux socket layer for the
// protocol frontends. One Frontends instance per agent owns
// a set of per-network listeners:
//
//   - UDP :5351 (NAT-PMP and PCP share the port; datagrams
//     are demultiplexed by the protocol version byte)
//   - UDP :1900 (SSDP M-SEARCH responder, multicast group
//     joined on the trusted interface only)
//   - TCP :53520 (UPnP IGD control: description XML + SOAP
//     AddPortMapping / DeletePortMapping)
//
// Nothing is bound until Reconcile sees an enabled trusted
// network — disabled by default is structural, not
// conventional. Management interfaces can never be bound:
// the agent only forwards registry-approved refs and the
// listener additionally refuses loopback/management-shaped
// addresses.
package frontend

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"sync"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/upnp"
)

// Bind describes one network the listeners serve.
type Bind struct {
	NetworkRef string
	Iface      string
	// Addrs are the interface's local IPv4 addresses (one
	// listener socket set per address).
	Addrs []netip.Addr
	// Protocols selects which listeners start for this bind.
	Protocols []upnp.Protocol
}

// IGDControlPort is the LAN-side HTTP/SOAP port of the UPnP
// IGD frontend. Chosen outside the privileged / well-known
// service space; clients learn it from the SSDP description.
const IGDControlPort = 53520

// Frontends owns the listener lifecycle. Goroutine-safe.
type Frontends struct {
	mu      sync.Mutex
	srv     *Server
	binds   map[string]*bindRuntime // key: NetworkRef
	started bool
	startAt time.Time
	ifaces  []string
	// provider supplies the enabled networks (the upnp
	// Runtime). resolve maps a network ref to its kernel
	// interface name (the trusted-LAN registry view).
	provider NetworkProvider
	resolve  IfaceResolver
	baseCtx  context.Context
}

// NetworkProvider supplies the enabled network list.
type NetworkProvider interface {
	Networks() []upnp.Network
}

// IfaceResolver maps a stable network ref to the kernel
// interface name the trusted-LAN registry currently
// resolves it to. ok=false refuses the bind.
type IfaceResolver func(ref string) (string, bool)

// bindRuntime is the live listener set for one network.
type bindRuntime struct {
	bind    Bind
	cancel  context.CancelFunc
	sockets []*udpSocket
	igd     *igdServer
	ssdp    *ssdpResponder
}

// udpSocket is one bound datagram socket with its serve
// goroutine.
type udpSocket struct {
	conn   *net.UDPConn
	cancel context.CancelFunc
}

// NewFrontends wires the listener layer to the Server core.
// provider and resolve are required for ReconcileEnabled;
// direct Reconcile callers may pass nil.
func NewFrontends(srv *Server, provider NetworkProvider, resolve IfaceResolver) *Frontends {
	return &Frontends{srv: srv, binds: map[string]*bindRuntime{}, provider: provider, resolve: resolve, baseCtx: context.Background()}
}

// ReconcileEnabled aligns the listener set with the enabled
// networks reported by the runtime provider. It is the seam
// the agent calls after every enable/disable.
func (f *Frontends) ReconcileEnabled() error {
	f.mu.Lock()
	provider, resolve, baseCtx := f.provider, f.resolve, f.baseCtx
	f.mu.Unlock()
	if provider == nil || resolve == nil {
		return nil
	}
	var binds []Bind
	for _, n := range provider.Networks() {
		iface, ok := resolve(n.NetworkRef)
		if !ok || iface == "" || iface == "lo" {
			continue
		}
		addrs, err := LocalIPv4Addrs(iface)
		if err != nil || len(addrs) == 0 {
			continue
		}
		binds = append(binds, Bind{
			NetworkRef: n.NetworkRef,
			Iface:      iface,
			Addrs:      addrs,
			Protocols:  n.Protocols,
		})
	}
	return f.Reconcile(baseCtx, binds, observeExternalIPv4())
}

// Snapshot implements the agent's UPnPFrontendController.
func (f *Frontends) Snapshot() (FrontendsStatus, Status, []Rejection) {
	return f.Status(), f.srv.Status(), f.srv.Journal().Recent(64)
}

// observeExternalIPv4 returns the source address the host
// would use toward the default gateway (a connected UDP
// socket — no packet leaves the machine). Empty when no
// default route is observed; the protocol layer treats that
// as "external address unknown" and answers honestly.
func observeExternalIPv4() string {
	conn, err := net.Dial("udp4", "8.8.8.8:80")
	if err != nil {
		return ""
	}
	defer conn.Close()
	host, _, err := net.SplitHostPort(conn.LocalAddr().String())
	if err != nil {
		return ""
	}
	return host
}

// Server exposes the core (status, journal, handlers).
func (f *Frontends) Server() *Server { return f.srv }

// Reconcile aligns the running listener set with the desired
// network list. Networks absent from desired are stopped;
// new ones are started. A network with no resolvable local
// address is skipped (and visible as not running in Status).
func (f *Frontends) Reconcile(ctx context.Context, desired []Bind, external string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if external != "" {
		f.srv.SetExternal(external)
	}
	want := map[string]Bind{}
	for _, b := range desired {
		if b.NetworkRef == "" || b.Iface == "" || len(b.Addrs) == 0 {
			continue
		}
		if b.Iface == "lo" {
			continue
		}
		want[b.NetworkRef] = b
	}
	// Stop removed networks.
	for ref, rt := range f.binds {
		if _, ok := want[ref]; !ok {
			rt.cancel()
			rt.closeSockets()
			delete(f.binds, ref)
		}
	}
	var ifaces []string
	for ref, b := range want {
		if _, ok := f.binds[ref]; ok {
			ifaces = append(ifaces, b.Iface)
			continue
		}
		rt, err := f.startBind(ctx, b)
		if err != nil {
			// One unbindable network must not take the rest
			// down; the state stays visible via Status.
			continue
		}
		f.binds[ref] = rt
		ifaces = append(ifaces, b.Iface)
	}
	f.ifaces = dedupe(ifaces)
	f.started = len(f.binds) > 0
	if f.started && f.startAt.IsZero() {
		f.startAt = time.Now().UTC()
	}
	if !f.started {
		f.startAt = time.Time{}
	}
	return nil
}

// StopAll tears every listener down (feature disable path).
func (f *Frontends) StopAll() {
	f.mu.Lock()
	defer f.mu.Unlock()
	for ref, rt := range f.binds {
		rt.cancel()
		rt.closeSockets()
		delete(f.binds, ref)
	}
	f.started = false
	f.ifaces = nil
}

// FrontendsStatus is the listener-layer snapshot merged into
// the public state document.
type FrontendsStatus struct {
	Running  bool     `json:"running"`
	Ifaces   []string `json:"ifaces,omitempty"`
	Networks []string `json:"networks,omitempty"`
}

// Status returns the listener snapshot.
func (f *Frontends) Status() FrontendsStatus {
	f.mu.Lock()
	defer f.mu.Unlock()
	st := FrontendsStatus{Running: f.started, Ifaces: append([]string(nil), f.ifaces...)}
	for ref := range f.binds {
		st.Networks = append(st.Networks, ref)
	}
	return st
}

// startBind opens the sockets for one network. The ctx
// governs the lifetime; the returned cancel unwinds it.
func (f *Frontends) startBind(ctx context.Context, b Bind) (*bindRuntime, error) {
	cctx, cancel := context.WithCancel(ctx)
	rt := &bindRuntime{bind: b, cancel: cancel}
	protocols := map[upnp.Protocol]bool{}
	for _, p := range b.Protocols {
		protocols[p] = true
	}
	var lastErr error
	for _, addr := range b.Addrs {
		if !addr.Is4() {
			// V1 data-plane is IPv4-only; IPv6 frontends are a
			// separate milestone. Skip silently.
			continue
		}
		if protocols[upnp.ProtoNATPMP] || protocols[upnp.ProtoPCP] {
			s, err := listenUDP(cctx, f.srv, b.NetworkRef, addr, 5351)
			if err != nil {
				lastErr = err
			} else {
				rt.sockets = append(rt.sockets, s)
			}
		}
		if protocols[upnp.ProtoUPnP] {
			ss, err := newSSDPResponder(cctx, f.srv, b.NetworkRef, addr, b.Iface)
			if err != nil {
				lastErr = err
			} else {
				rt.ssdp = ss
			}
			igd, err := newIGDServer(cctx, f.srv, b.NetworkRef, addr)
			if err != nil {
				lastErr = err
			} else {
				rt.igd = igd
			}
		}
	}
	if len(rt.sockets) == 0 && rt.ssdp == nil && rt.igd == nil {
		cancel()
		if lastErr != nil {
			return nil, fmt.Errorf("frontend: bind %s failed: %w", b.Iface, lastErr)
		}
		return nil, fmt.Errorf("frontend: no listeners started for %s", b.Iface)
	}
	return rt, nil
}

func (rt *bindRuntime) closeSockets() {
	for _, s := range rt.sockets {
		s.cancel()
		_ = s.conn.Close()
	}
	if rt.ssdp != nil {
		rt.ssdp.close()
	}
	if rt.igd != nil {
		rt.igd.close()
	}
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

// localIPv4Addrs lists the IPv4 addresses of a kernel
// interface. Used by the agent when building Binds.
func LocalIPv4Addrs(iface string) ([]netip.Addr, error) {
	ifi, err := net.InterfaceByName(iface)
	if err != nil {
		return nil, err
	}
	addrs, err := ifi.Addrs()
	if err != nil {
		return nil, err
	}
	var out []netip.Addr
	for _, a := range addrs {
		ipn, ok := netip.AddrFromSlice([]byte(a.(*net.IPNet).IP.To4()))
		if !ok {
			continue
		}
		if ipn.Is4() && !ipn.IsUnspecified() {
			out = append(out, ipn)
		}
	}
	return out, nil
}
