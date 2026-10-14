// server.go: the cross-platform Server shell around the
// shared core. The Linux listeners (listeners_linux.go) bind
// sockets; this file owns construction, external-address
// observation and the public status surface so tests on any
// build target can drive the full protocol path.
package frontend

// Server is the protocol frontend core. It is created per
// agent; listeners attach to it on Linux. Disabled by
// default: with no listeners started nothing is processed
// and no sockets are open.
type Server struct {
	core *serverCore
}

// NewServer wires the frontend core to a MappingSink (the
// upnp.Runtime) with the given limits and rejection journal.
// Both may be nil: limits fall back to DefaultLimits and the
// journal to a 256-entry ring.
func NewServer(limits Limits, journal *RejectionJournal, sink MappingSink) *Server {
	return &Server{core: newServerCore(limits, journal, sink, nil)}
}

// SetExternal records the observed WAN IPv4 address used in
// NAT-PMP public-address responses and PCP conflict checks.
func (s *Server) SetExternal(ip string) { s.core.setExternal(ip) }

// Status returns the public frontend status snapshot.
func (s *Server) Status() Status { return s.core.snapshot() }

// Journal returns the rejection journal.
func (s *Server) Journal() *RejectionJournal { return s.core.journal }

// Limits returns the effective (sanitized) limits.
func (s *Server) Limits() Limits { return s.core.limits }

// ExternalAddresser is the optional seam for the observed
// external address provider (e.g. the WAN manager snapshot).
type ExternalAddresser interface {
	ExternalIPv4() string
}
