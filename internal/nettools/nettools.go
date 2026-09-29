// nettools/nettools.go: V1 contextual
// network-tools surface. The package owns
// the typed contracts for ping, traceroute,
// TCP connect probe, DNS query, route
// lookup, and live path capture. Every
// probe accepts an explicit network context
// (source interface, source IP, VRF, mark,
// routing table).
//
// The runtime is a thin mapping to
// net/icmp, net.Dial, and the existing
// internal/capture Linux runtime. The
// package itself never spawns processes.
package nettools

import (
	"errors"
	"regexp"
	"time"
)

// NetworkContext is the per-probe network
// context. Empty fields mean "use the
// system default".
type NetworkContext struct {
	SourceIP    string        `json:"sourceIp,omitempty"`
	SourceIface string        `json:"sourceIface,omitempty"`
	VRF         string        `json:"vrf,omitempty"`
	Table       int           `json:"table,omitempty"`
	Mark        int           `json:"mark,omitempty"`
	Timeout     time.Duration `json:"timeout,omitempty"`
}

// Valid reports whether c is well-formed.
func (c NetworkContext) Valid() error {
	if c.Table < 0 {
		return errors.New("nettools: routing table must be non-negative")
	}
	if c.Mark < 0 {
		return errors.New("nettools: mark must be non-negative")
	}
	if c.Timeout < 0 {
		return errors.New("nettools: timeout must be non-negative")
	}
	return nil
}

// PingRequest is the input to a ping probe.
type PingRequest struct {
	Target       string         `json:"target"`
	Count        int            `json:"count,omitempty"`
	Payload      int            `json:"payload,omitempty"`
	DontFragment bool           `json:"dontFragment,omitempty"`
	Context      NetworkContext `json:"context"`
}

// PingResult is the per-probe result.
type PingResult struct {
	Target     string    `json:"target"`
	Sent       int       `json:"sent"`
	Received   int       `json:"received"`
	Loss       float64   `json:"loss"`
	RTTMs      []float64 `json:"rttMs,omitempty"`
	AvgRTTMs   float64   `json:"avgRttMs,omitempty"`
	ResolvedIP string    `json:"resolvedIp,omitempty"`
	Error      string    `json:"error,omitempty"`
}

var targetPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:/-]{0,253}$`)

// Valid reports whether p is well-formed.
func (p PingRequest) Valid() error {
	if !targetPattern.MatchString(p.Target) {
		return errors.New("nettools: target is not canonical")
	}
	if p.Count < 0 || p.Count > 64 {
		return errors.New("nettools: ping count must be 0..64")
	}
	if p.Payload < 0 || p.Payload > 65535 {
		return errors.New("nettools: payload must be 0..65535")
	}
	return p.Context.Valid()
}

// TracerouteRequest is the input to a
// traceroute.
type TracerouteRequest struct {
	Target  string         `json:"target"`
	MaxHops int            `json:"maxHops,omitempty"`
	Context NetworkContext `json:"context"`
}

// Hop is a single traceroute hop.
type Hop struct {
	Index   int     `json:"index"`
	Address string  `json:"address,omitempty"`
	Host    string  `json:"host,omitempty"`
	RTTMs   float64 `json:"rttMs"`
	Timeout bool    `json:"timeout,omitempty"`
}

// TracerouteResult is the per-probe result.
type TracerouteResult struct {
	Target  string `json:"target"`
	Hops    []Hop  `json:"hops"`
	Reached bool   `json:"reached"`
	Error   string `json:"error,omitempty"`
}

// Valid reports whether t is well-formed.
func (t TracerouteRequest) Valid() error {
	if !targetPattern.MatchString(t.Target) {
		return errors.New("nettools: target is not canonical")
	}
	if t.MaxHops < 0 || t.MaxHops > 64 {
		return errors.New("nettools: maxHops must be 0..64")
	}
	return t.Context.Valid()
}

// TCPProbeRequest is the input to a TCP
// connect probe.
type TCPProbeRequest struct {
	Host    string         `json:"host"`
	Port    int            `json:"port"`
	Context NetworkContext `json:"context"`
}

// TCPProbeResult is the per-probe result.
type TCPProbeResult struct {
	Host    string  `json:"host"`
	Port    int     `json:"port"`
	Open    bool    `json:"open"`
	Elapsed float64 `json:"elapsedMs"`
	Error   string  `json:"error,omitempty"`
}

// Valid reports whether r is well-formed.
func (r TCPProbeRequest) Valid() error {
	if !targetPattern.MatchString(r.Host) {
		return errors.New("nettools: host is not canonical")
	}
	if r.Port < 1 || r.Port > 65535 {
		return errors.New("nettools: port out of range")
	}
	return r.Context.Valid()
}

// DNSQueryRequest is the input to a DNS
// query.
type DNSQueryRequest struct {
	Name     string         `json:"name"`
	Type     string         `json:"type,omitempty"` // A, AAAA, etc.
	Resolver string         `json:"resolver,omitempty"`
	Context  NetworkContext `json:"context"`
}

// DNSQueryResult is the per-query result.
type DNSQueryResult struct {
	Name     string   `json:"name"`
	Records  []string `json:"records,omitempty"`
	Resolver string   `json:"resolver,omitempty"`
	Elapsed  float64  `json:"elapsedMs"`
	Error    string   `json:"error,omitempty"`
}

// Valid reports whether q is well-formed.
func (q DNSQueryRequest) Valid() error {
	if q.Name == "" {
		return errors.New("nettools: name is required")
	}
	if q.Type != "" {
		switch q.Type {
		case "A", "AAAA", "CNAME", "MX", "TXT", "NS", "PTR", "SRV":
		default:
			return errors.New("nettools: query type is unknown")
		}
	}
	return q.Context.Valid()
}

// RouteLookupRequest is the input to a
// route-lookup simulation.
type RouteLookupRequest struct {
	Src     string         `json:"src,omitempty"`
	Dst     string         `json:"dst"`
	VRF     string         `json:"vrf,omitempty"`
	Table   int            `json:"table,omitempty"`
	Mark    int            `json:"mark,omitempty"`
	Context NetworkContext `json:"context"`
}

// RouteLookupResult is the simulation
// output. The "Selected" fields describe
// the chosen rule, route, and egress
// interface; the Reason is the human-
// readable explanation.
type RouteLookupResult struct {
	Src           string `json:"src,omitempty"`
	Dst           string `json:"dst"`
	VRF           string `json:"vrf,omitempty"`
	SelectedRule  string `json:"selectedRule,omitempty"`
	SelectedRoute string `json:"selectedRoute,omitempty"`
	NextHop       string `json:"nextHop,omitempty"`
	Egress        string `json:"egress,omitempty"`
	Reason        string `json:"reason"`
}

// Valid reports whether r is well-formed.
func (r RouteLookupRequest) Valid() error {
	if r.Dst == "" {
		return errors.New("nettools: dst is required")
	}
	if r.Mark < 0 {
		return errors.New("nettools: mark must be non-negative")
	}
	if r.Table < 0 {
		return errors.New("nettools: table must be non-negative")
	}
	return r.Context.Valid()
}

// LiveCaptureRequest is the input to a
// bounded live capture. The package
// delegates to the existing capture
// runtime; it does not run tcpdump itself.
type LiveCaptureRequest struct {
	Interface string         `json:"interface"`
	Filter    string         `json:"filter,omitempty"`
	Duration  time.Duration  `json:"duration"`
	Bytes     int            `json:"bytes,omitempty"`
	Context   NetworkContext `json:"context"`
}

// Valid reports whether c is well-formed.
func (c LiveCaptureRequest) Valid() error {
	if c.Interface == "" {
		return errors.New("nettools: interface is required")
	}
	if c.Duration <= 0 || c.Duration > 5*time.Minute {
		return errors.New("nettools: duration must be 0..5m")
	}
	if c.Bytes < 0 || c.Bytes > 1<<26 {
		return errors.New("nettools: bytes must be 0..64MiB")
	}
	return c.Context.Valid()
}
