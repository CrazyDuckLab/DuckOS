// frr/frr.go: V1 FRR real-runtime
// integration. The package owns the typed
// configuration model, the renderer that
// produces an FRR-compatible frr.conf
// fragment, the validator, the lifecycle
// (render / validate / start / reload /
// stop / rollback), the runtime state
// collector, and the typed diagnostics.
//
// DuckOS owns the intent; the runtime
// surfaces the model. The package does not
// shell out; it builds the config text in
// memory and exposes typed entry points
// the privileged Agent uses to drive
// vtysh / frr-reload / etc.
package frr

import (
	"errors"
	"fmt"
	"net/netip"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// Daemon is the FRR daemon kind.
type Daemon string

const (
	DaemonZebra  Daemon = "zebra"
	DaemonBGP    Daemon = "bgpd"
	DaemonOSPF   Daemon = "ospfd"
	DaemonOSPF6  Daemon = "ospfd6"
	DaemonISIS   Daemon = "isisd"
	DaemonRIP    Daemon = "ripd"
	DaemonRIPNG  Daemon = "ripngd"
	DaemonBFD    Daemon = "bfdd"
	DaemonStatic Daemon = "staticd"
)

// Valid reports whether d is known.
func (d Daemon) Valid() bool {
	switch d {
	case DaemonZebra, DaemonBGP, DaemonOSPF, DaemonOSPF6,
		DaemonISIS, DaemonRIP, DaemonRIPNG, DaemonBFD, DaemonStatic:
		return true
	}
	return false
}

// AS is the autonomous-system number.
type AS uint32

// Neighbor is a typed BGP / OSPF neighbor.
type Neighbor struct {
	Address   string `json:"address"`
	RemoteAS  AS     `json:"remoteAS,omitempty"`
	Interface string `json:"interface,omitempty"`
	EBGP      bool   `json:"ebgp,omitempty"`
	Password  string `json:"password,omitempty"`
	// Description is a free-form label.
	Description string `json:"description,omitempty"`
}

// Valid reports whether n is well-formed.
func (n Neighbor) Valid() error {
	if n.Address == "" {
		return errors.New("frr: neighbor address is required")
	}
	if n.RemoteAS > 4294967295 {
		return errors.New("frr: remoteAS out of range")
	}
	return nil
}

// Network is a network to advertise.
type Network struct {
	Prefix string `json:"prefix"`
}

// Valid reports whether n is well-formed.
func (n Network) Valid() error {
	if p, err := netip.ParsePrefix(n.Prefix); err != nil || p != p.Masked() {
		return errors.New("frr: canonical network prefix is required")
	}
	return nil
}

// BGPSession is a complete BGP session.
type BGPSession struct {
	Name        string     `json:"name"`
	LocalAS     AS         `json:"localAS"`
	RouterID    string     `json:"routerID"`
	Networks    []Network  `json:"networks,omitempty"`
	Neighbors   []Neighbor `json:"neighbors,omitempty"`
	IPv4Unicast bool       `json:"ipv4Unicast,omitempty"`
	IPv6Unicast bool       `json:"ipv6Unicast,omitempty"`
	// Communities toggle the acceptance of
	// standard / extended / large communities.
	StandardCommunity    bool `json:"standardCommunity,omitempty"`
	ExtendedCommunity    bool `json:"extendedCommunity,omitempty"`
	LargeCommunity       bool `json:"largeCommunity,omitempty"`
	RouteReflector       bool `json:"routeReflector,omitempty"`
	RouteReflectorClient bool `json:"routeReflectorClient,omitempty"`
	GracefulRestart      bool `json:"gracefulRestart,omitempty"`
	MultipathRelax       bool `json:"multipathRelax,omitempty"`
	AddPath              bool `json:"addPath,omitempty"`
	MaxPrefix            int  `json:"maxPrefix,omitempty"`
	RPKIOut              bool `json:"rpkiOut,omitempty"`
}

var bgpNamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,63}$`)

// Valid reports whether b is well-formed.
func (b BGPSession) Valid() error {
	if !bgpNamePattern.MatchString(b.Name) {
		return errors.New("frr: bgp name is not canonical")
	}
	if b.LocalAS == 0 {
		return errors.New("frr: localAS is required")
	}
	if b.IPv4Unicast == false && b.IPv6Unicast == false {
		return errors.New("frr: at least one address family is required")
	}
	for _, n := range b.Networks {
		if err := n.Valid(); err != nil {
			return err
		}
	}
	for _, n := range b.Neighbors {
		if err := n.Valid(); err != nil {
			return err
		}
	}
	return nil
}

// OSPFArea is an OSPF area.
type OSPFArea struct {
	ID       string    `json:"id"`
	Stub     bool      `json:"stub,omitempty"`
	NSSA     bool      `json:"nssa,omitempty"`
	Networks []Network `json:"networks,omitempty"`
}

// Valid reports whether a is well-formed.
func (a OSPFArea) Valid() error {
	if a.ID == "" {
		return errors.New("frr: ospf area id is required")
	}
	for _, n := range a.Networks {
		if err := n.Valid(); err != nil {
			return err
		}
	}
	return nil
}

// OSPFInstance is an OSPF instance.
type OSPFInstance struct {
	Name         string     `json:"name"`
	RouterID     string     `json:"routerID"`
	Areas        []OSPFArea `json:"areas,omitempty"`
	Passive      []string   `json:"passive,omitempty"`
	Redistribute string     `json:"redistribute,omitempty"`
}

// Valid reports whether o is well-formed.
func (o OSPFInstance) Valid() error {
	if !bgpNamePattern.MatchString(o.Name) {
		return errors.New("frr: ospf name is not canonical")
	}
	if o.RouterID == "" {
		return errors.New("frr: ospf routerID is required")
	}
	for _, a := range o.Areas {
		if err := a.Valid(); err != nil {
			return err
		}
	}
	return nil
}

// Config is the typed FRR configuration.
type Config struct {
	Daemons []Daemon       `json:"daemons"`
	BGP     []BGPSession   `json:"bgp,omitempty"`
	OSPF    []OSPFInstance `json:"ospf,omitempty"`
	LogFile string         `json:"logFile,omitempty"`
}

// Valid reports whether c is well-formed.
func (c Config) Valid() error {
	if len(c.Daemons) == 0 {
		return errors.New("frr: at least one daemon is required")
	}
	for _, d := range c.Daemons {
		if !d.Valid() {
			return errors.New("frr: unknown daemon: " + string(d))
		}
	}
	for _, b := range c.BGP {
		if err := b.Valid(); err != nil {
			return err
		}
	}
	for _, o := range c.OSPF {
		if err := o.Valid(); err != nil {
			return err
		}
	}
	return nil
}

// Render produces the FRR frr.conf text.
// The output is deterministic: the same
// Config always produces the same bytes,
// sorted by section / session / neighbor /
// network.
func Render(c Config) (string, error) {
	if err := c.Valid(); err != nil {
		return "", err
	}
	var sb strings.Builder
	sb.WriteString("! DuckOS-generated frr.conf\n")
	sb.WriteString("! Do not edit by hand.\n")
	// frr defaults traditional: keep legacy syntax
	// (no dataplane / no yang). The runtime is the
	// canonical owner of which daemons run; the
	// frr.conf declares the configuration the
	// daemons load after they start. FRR 10.x
	// auto-discovers the daemons to run from the
	// frr.conf; we do NOT need "zebra=yes" /
	// "bgpd=yes" stubs (those cause parser errors
	// on FRR 10).
	sb.WriteString("frr defaults traditional\n")
	// log file is optional; the runtime's daemon
	// logs already redirect to <sockDir>/{zebra,bgpd}.log.
	if c.LogFile != "" {
		fmt.Fprintf(&sb, "log file %s informational\n", c.LogFile)
	}
	sb.WriteString("!\n")
	// Sort BGP sessions by name for deterministic
	// output (the canonical regression witness).
	bgp := append([]BGPSession(nil), c.BGP...)
	sort.Slice(bgp, func(i, k int) bool { return bgp[i].Name < bgp[k].Name })
	for _, b := range bgp {
		renderBGP(&sb, b)
	}
	ospf := append([]OSPFInstance(nil), c.OSPF...)
	sort.Slice(ospf, func(i, k int) bool { return ospf[i].Name < ospf[k].Name })
	for _, o := range ospf {
		renderOSPF(&sb, o)
	}
	return sb.String(), nil
}

// renderBGP emits a single FRR "router bgp <as>" stanza.
// FRR 10.x rejects the legacy "router bgp <as> <name>"
// form (the session name is a DuckOS-only concept,
// not an FRR router-id), so we emit only the AS.
//
// remote-as belongs to router mode; activate/maximum-prefix belong to
// address-family mode. Mixing them implicitly exits the AF in FRR 10.
func renderBGP(sb *strings.Builder, b BGPSession) {
	fmt.Fprintf(sb, "router bgp %d\n", b.LocalAS)
	if b.RouterID != "" {
		fmt.Fprintf(sb, " bgp router-id %s\n", b.RouterID)
	}
	ns := append([]Neighbor(nil), b.Neighbors...)
	sort.Slice(ns, func(i, k int) bool { return ns[i].Address < ns[k].Address })
	for _, n := range ns {
		fmt.Fprintf(sb, " neighbor %s remote-as %d\n", n.Address, n.RemoteAS)
	}
	if b.IPv4Unicast {
		sb.WriteString(" address-family ipv4 unicast\n")
		for _, n := range b.Networks {
			if p, err := netip.ParsePrefix(n.Prefix); err == nil && p.Addr().Is4() {
				fmt.Fprintf(sb, "  network %s\n", n.Prefix)
			}
		}
		// Sort neighbors by address; FRR requires
		// neighbor X remote-as BEFORE neighbor X
		// activate (parser order matters).
		for _, n := range ns {
			fmt.Fprintf(sb, "  neighbor %s activate\n", n.Address)
		}
		if b.MaxPrefix > 0 {
			for _, n := range ns {
				fmt.Fprintf(sb, "  neighbor %s maximum-prefix %d\n", n.Address, b.MaxPrefix)
			}
		}
		sb.WriteString(" exit-address-family\n")
	}
	if b.IPv6Unicast {
		sb.WriteString(" address-family ipv6 unicast\n")
		for _, n := range b.Networks {
			if p, err := netip.ParsePrefix(n.Prefix); err == nil && p.Addr().Is6() {
				fmt.Fprintf(sb, "  network %s\n", n.Prefix)
			}
		}
		for _, n := range ns {
			fmt.Fprintf(sb, "  neighbor %s activate\n", n.Address)
			if b.MaxPrefix > 0 {
				fmt.Fprintf(sb, "  neighbor %s maximum-prefix %d\n", n.Address, b.MaxPrefix)
			}
		}
		sb.WriteString(" exit-address-family\n")
	}
	// Neighbors at the bgp level (only non-AF
	// settings that FRR 10.x still honours).
	for _, n := range ns {
		if n.Description != "" {
			fmt.Fprintf(sb, " neighbor %s description %s\n", n.Address, n.Description)
		}
		if n.Password != "" {
			fmt.Fprintf(sb, " neighbor %s password %s\n", n.Address, n.Password)
		}
		if n.EBGP {
			fmt.Fprintf(sb, " neighbor %s ebgp-multihop\n", n.Address)
		}
	}
	if b.RouteReflector {
		sb.WriteString(" bgp route-reflector allow-outbound-policy\n")
	}
	if b.RPKIOut {
		sb.WriteString(" bgp rpki server tcp 1.1.1.1 port 323\n")
	}
	sb.WriteString("exit\n!\n")
}

func renderOSPF(sb *strings.Builder, o OSPFInstance) {
	fmt.Fprintf(sb, "router ospf %s\n", o.Name)
	if o.RouterID != "" {
		fmt.Fprintf(sb, " ospf router-id %s\n", o.RouterID)
	}
	as := append([]OSPFArea(nil), o.Areas...)
	sort.Slice(as, func(i, k int) bool { return as[i].ID < as[k].ID })
	for _, a := range as {
		fmt.Fprintf(sb, " area %s\n", a.ID)
		ns := append([]Network(nil), a.Networks...)
		sort.Slice(ns, func(i, k int) bool { return ns[i].Prefix < ns[k].Prefix })
		for _, n := range ns {
			fmt.Fprintf(sb, "  network %s\n", n.Prefix)
		}
		if a.Stub {
			sb.WriteString("  stub\n")
		}
		if a.NSSA {
			sb.WriteString("  nssa\n")
		}
	}
	ps := append([]string(nil), o.Passive...)
	sort.Strings(ps)
	for _, p := range ps {
		fmt.Fprintf(sb, " passive-interface %s\n", p)
	}
	if o.Redistribute != "" {
		fmt.Fprintf(sb, " redistribute %s\n", o.Redistribute)
	}
	sb.WriteString("!\n")
}

// State is the runtime state.
type State struct {
	At         time.Time `json:"at"`
	LastReload time.Time `json:"lastReload"`
	LastError  string    `json:"lastError,omitempty"`
	// SessionsEstablished is the number of
	// BGP / OSPF sessions currently up.
	SessionsEstablished int `json:"sessionsEstablished"`
	// RoutesInstalled is the number of routes
	// in the FIB.
	RoutesInstalled int `json:"routesInstalled"`
}

// LifecycleEvent is one entry in the
// lifecycle history.
type LifecycleEvent struct {
	At     time.Time `json:"at"`
	Action string    `json:"action"`
	Actor  string    `json:"actor"`
	Result string    `json:"result"`
	Detail string    `json:"detail,omitempty"`
}

const MaxLifecycle = 256

// Manager is the FRR lifecycle store.
// Manager is goroutine-safe.
type Manager struct {
	mu       sync.Mutex
	config   Config
	previous Config
	history  []LifecycleEvent
	clock    func() time.Time
	path     string
}

// NewManager returns an empty manager.
func NewManager(clock func() time.Time) *Manager {
	if clock == nil {
		clock = time.Now
	}
	return &Manager{clock: clock}
}

// Set replaces the candidate config. The
// previous config is retained for rollback.
func (m *Manager) Set(c Config) error {
	if err := c.Valid(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.persist(c, m.config); err != nil {
		return err
	}
	m.previous = m.config
	m.config = c
	m.history = append(m.history, LifecycleEvent{At: m.clock().UTC(), Action: "set", Result: "ok"})
	if len(m.history) > MaxLifecycle {
		m.history = m.history[len(m.history)-MaxLifecycle:]
	}
	return nil
}

// Render returns the rendered config text.
func (m *Manager) Render() (string, error) {
	m.mu.Lock()
	c := m.config
	m.mu.Unlock()
	return Render(c)
}

// Rollback restores the previous config.
// The previous config is rotated out so a
// double Rollback returns an error.
func (m *Manager) Rollback(reason string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.previous.Daemons == nil {
		return errors.New("frr: no previous config to roll back to")
	}
	if err := m.persist(m.previous, Config{}); err != nil {
		return err
	}
	m.config = m.previous
	m.previous = Config{}
	m.history = append(m.history, LifecycleEvent{At: m.clock().UTC(), Action: "rollback", Result: "ok", Detail: reason})
	if len(m.history) > MaxLifecycle {
		m.history = m.history[len(m.history)-MaxLifecycle:]
	}
	return nil
}

// History returns a copy of the lifecycle
// history, newest first.
func (m *Manager) History() []LifecycleEvent {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]LifecycleEvent, len(m.history))
	copy(out, m.history)
	sort.SliceStable(out, func(i, k int) bool { return out[i].At.After(out[k].At) })
	return out
}
