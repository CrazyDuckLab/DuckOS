// Package frontend implements the beta.3 port-mapping
// protocol frontends (NAT-PMP, PCP, SSDP/IGD) on top of the
// existing upnp.Runtime mapping engine.
//
// Architecture contract:
//
//	LAN client → UDP/HTTP listener (trusted iface only)
//	          → protocol handler (this package, pure logic)
//	          → MappingSink (upnp.Runtime.AddMapping)
//	          → NetworkBackend (nftables duckos_upnp table)
//
// The core (this file and the protocol handlers) is
// cross-platform and fully unit-testable without sockets.
// The Linux-only listeners live in listeners_linux.go.
//
// Security invariants (beta.3 requirements):
//
//   - Disabled by default: a Server with no enabled networks
//     processes nothing and listeners are not started.
//   - Every request is attributed to the trusted network it
//     arrived on; the listener layer resolves that mapping.
//   - Packet size limits and strict parsing live in the
//     proto package.
//   - Per-client rate limiting and mapping quotas.
//   - Lifetime is capped by the network TTL.
//   - Every rejection is recorded in a bounded journal the
//     public API can surface ("inspect rejected requests").
//   - No privilege escalation: this package never execs
//     anything; it only calls the MappingSink.
package frontend

import (
	"sync"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/upnp"
)

// Limits are the server-side resource bounds. All limits are
// enforced before any state mutation.
type Limits struct {
	// MaxMappingsPerClient bounds the active mappings one
	// client address may own across all protocols.
	MaxMappingsPerClient int
	// MaxMappingsTotal bounds the global mapping table the
	// frontends may populate.
	MaxMappingsTotal int
	// RequestsPerMinute is the per-client request budget.
	// Excess requests are refused and journaled.
	RequestsPerMinute int
	// MaxLifetimeSec caps every requested mapping lifetime;
	// the effective lifetime is also capped by the network
	// TTL at the runtime layer.
	MaxLifetimeSec uint32
	// MinLifetimeSec is the RFC 6886 default applied when a
	// client requests lifetime 0.
	MinLifetimeSec uint32
}

// DefaultLimits returns the conservative production limits.
func DefaultLimits() Limits {
	return Limits{
		MaxMappingsPerClient: 32,
		MaxMappingsTotal:     256,
		RequestsPerMinute:    60,
		MaxLifetimeSec:       86400,
		MinLifetimeSec:       7200,
	}
}

// sanitize repairs unset fields so a zero Limits does not
// disable protection.
func (l Limits) sanitize() Limits {
	if l.MaxMappingsPerClient <= 0 {
		l.MaxMappingsPerClient = 32
	}
	if l.MaxMappingsTotal <= 0 {
		l.MaxMappingsTotal = 256
	}
	if l.RequestsPerMinute <= 0 {
		l.RequestsPerMinute = 60
	}
	if l.MaxLifetimeSec == 0 {
		l.MaxLifetimeSec = 86400
	}
	if l.MinLifetimeSec == 0 {
		l.MinLifetimeSec = 7200
	}
	if l.MinLifetimeSec > l.MaxLifetimeSec {
		l.MinLifetimeSec = l.MaxLifetimeSec
	}
	return l
}

// MappingSink is the runtime seam the handlers drive. It is
// satisfied by *upnp.Runtime (linux) and by test doubles.
type MappingSink interface {
	AddMapping(m upnp.Mapping, source string) error
	Mappings() []upnp.Mapping
}

// Status is the public snapshot of the frontend servers the
// API surfaces inside the UPnP state document.
type Status struct {
	// Running reports whether listeners are active. Running
	// is true only when at least one protocol is enabled on
	// at least one trusted network.
	Running bool `json:"running"`
	// StartedAt is the RFC3339 server start time (empty when
	// the frontends have never started).
	StartedAt string `json:"startedAt,omitempty"`
	// BoundIfaces lists the kernel interfaces the listeners
	// are bound to (diagnostics only; never a trust source).
	BoundIfaces []string `json:"boundIfaces,omitempty"`
	// ExternalIPv4 is the observed WAN address used in
	// protocol responses ("unknown" when unobserved).
	ExternalIPv4 string `json:"externalIpv4,omitempty"`
	// Requests counters (monotonic since start).
	RequestsTotal   uint64 `json:"requestsTotal"`
	RequestsDenied  uint64 `json:"requestsDenied"`
	MappingsCreated uint64 `json:"mappingsCreated"`
}

// Rejection is one journaled refused request.
type Rejection struct {
	At         time.Time `json:"at"`
	Protocol   string    `json:"protocol"`
	ClientIP   string    `json:"clientIp"`
	NetworkRef string    `json:"networkRef,omitempty"`
	Reason     string    `json:"reason"`
}

// RejectionJournal is a bounded FIFO of rejections. It is
// goroutine-safe and never grows beyond capacity; the oldest
// entry is dropped when full.
type RejectionJournal struct {
	mu   sync.Mutex
	buf  []Rejection
	cap  int
	head int
	full bool
}

// NewRejectionJournal returns a journal holding up to cap
// entries (minimum 16).
func NewRejectionJournal(cap int) *RejectionJournal {
	if cap < 16 {
		cap = 16
	}
	return &RejectionJournal{buf: make([]Rejection, cap), cap: cap}
}

// Add records one rejection.
func (j *RejectionJournal) Add(r Rejection) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.buf[j.head] = r
	j.head++
	if j.head == j.cap {
		j.head = 0
		j.full = true
	}
}

// Recent returns the newest n rejections, oldest first.
func (j *RejectionJournal) Recent(n int) []Rejection {
	j.mu.Lock()
	defer j.mu.Unlock()
	count := j.cap
	if !j.full {
		count = j.head
	}
	if n > count {
		n = count
	}
	ordered := make([]Rejection, 0, count)
	for i := 0; i < count; i++ {
		idx := 0
		if j.full {
			idx = (j.head + i) % j.cap
		} else {
			idx = i
		}
		ordered = append(ordered, j.buf[idx])
	}
	return ordered[len(ordered)-n:]
}

// Count returns the number of journaled entries.
func (j *RejectionJournal) Count() int {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.full {
		return j.cap
	}
	return j.head
}

// serverCore carries the state the pure protocol handlers
// share: limits, counters, rate budgets and the journal. It
// is embedded by Server (listeners_linux.go) and by the test
// harness.
type serverCore struct {
	mu      sync.Mutex
	limits  Limits
	journal *RejectionJournal
	started time.Time
	status  Status
	budgets map[string]*rateBudget
	sink    MappingSink
	extIPv4 string
	nowFn   func() time.Time
}

// rateBudget is a fixed-window per-client request budget.
type rateBudget struct {
	windowStart time.Time
	used        int
}

func newServerCore(limits Limits, journal *RejectionJournal, sink MappingSink, nowFn func() time.Time) *serverCore {
	if journal == nil {
		journal = NewRejectionJournal(256)
	}
	if nowFn == nil {
		nowFn = time.Now
	}
	return &serverCore{
		limits:  limits.sanitize(),
		journal: journal,
		started: nowFn().UTC(),
		sink:    sink,
		nowFn:   nowFn,
		budgets: map[string]*rateBudget{},
	}
}

// admit records one request from clientIP and reports
// whether the per-client rate budget allows it.
func (c *serverCore) admit(clientIP string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.status.RequestsTotal++
	now := c.nowFn()
	b := c.budgets[clientIP]
	if b == nil || now.Sub(b.windowStart) >= time.Minute {
		if len(c.budgets) > 4096 {
			// Bound the budget table: drop expired entries.
			for k, v := range c.budgets {
				if now.Sub(v.windowStart) >= time.Minute {
					delete(c.budgets, k)
				}
			}
		}
		b = &rateBudget{windowStart: now}
		c.budgets[clientIP] = b
	}
	if b.used >= c.limits.RequestsPerMinute {
		c.status.RequestsDenied++
		return false
	}
	b.used++
	return true
}

// reject journals a refusal and bumps the denied counter.
// The rate-limit path already counted the denial; callers
// pass counted=false there.
func (c *serverCore) reject(counted bool, proto, clientIP, networkRef, reason string) {
	c.mu.Lock()
	if !counted {
		c.status.RequestsDenied++
	}
	c.mu.Unlock()
	c.journal.Add(Rejection{
		At:         c.nowFn().UTC(),
		Protocol:   proto,
		ClientIP:   clientIP,
		NetworkRef: networkRef,
		Reason:     reason,
	})
}

// observeSuccess bumps the created counter.
func (c *serverCore) observeSuccess() {
	c.mu.Lock()
	c.status.MappingsCreated++
	c.mu.Unlock()
}

// snapshot returns the public Status.
func (c *serverCore) snapshot() Status {
	c.mu.Lock()
	defer c.mu.Unlock()
	s := c.status
	s.StartedAt = c.started.Format(time.RFC3339)
	s.ExternalIPv4 = c.extIPv4
	return s
}

// setExternal records the observed WAN IPv4 used in protocol
// responses.
func (c *serverCore) setExternal(ip string) {
	c.mu.Lock()
	c.extIPv4 = ip
	c.mu.Unlock()
}

// clientAllowed enforces the optional per-network allow-list.
func clientAllowed(n upnp.Network, clientIP string) bool {
	if len(n.AllowedClients) == 0 {
		return true
	}
	for _, a := range n.AllowedClients {
		if a == clientIP {
			return true
		}
	}
	return false
}

// effectiveLifetime clamps the requested lifetime to the
// configured floor/ceiling.
func (c *serverCore) effectiveLifetime(requested uint32) uint32 {
	if requested == 0 {
		return c.limits.MinLifetimeSec
	}
	if requested > c.limits.MaxLifetimeSec {
		return c.limits.MaxLifetimeSec
	}
	return requested
}

// buildMapping assembles the canonical upnp.Mapping for one
// accepted request.
func buildMapping(protocol upnp.Protocol, transport upnp.Transport, network upnp.Network, clientIP string, intPort, extPort int, lifetimeSec uint32, now time.Time) upnp.Mapping {
	return upnp.Mapping{
		ID:           mappingID(protocol, transport, clientIP, extPort, now),
		NetworkRef:   network.NetworkRef,
		Protocol:     protocol,
		Transport:    transport,
		ClientIP:     clientIP,
		InternalHost: clientIP,
		InternalPort: intPort,
		ExternalPort: extPort,
		ExpiresAt:    now.UTC().Add(time.Duration(lifetimeSec) * time.Second),
		CreatedAt:    now.UTC(),
	}
}

// mappingID derives a stable, collision-free id from the
// request tuple. Renewals of the same tuple reuse the id so
// the runtime replaces the mapping instead of duplicating it.
func mappingID(protocol upnp.Protocol, transport upnp.Transport, clientIP string, extPort int, now time.Time) string {
	return string(protocol) + "-" + string(transport) + "-" + clientIP + "-" + itoa(extPort)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
