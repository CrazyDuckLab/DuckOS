// natpmp.go: the NAT-PMP request handler. Pure logic — the
// Linux listener feeds datagrams in and writes the returned
// response datagram back to the client. Renewals of an
// existing (client, transport, externalPort) tuple replace
// the previous mapping id; conflicting ports are refused.
package frontend

import (
	"encoding/binary"
	"net/netip"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/upnp"
	"github.com/CrazyDuckLab/DuckOS/internal/upnp/proto"
)

// NATPMPResult encodes a handled request outcome.
type NATPMPResult struct {
	// Response is the datagram to send back (nil = drop
	// silently, e.g. unparseable junk from an off-LAN source).
	Response []byte
	// Accepted reports whether a mapping was created or
	// renewed.
	Accepted bool
	// Counted reports whether the denial was already counted
	// by the rate budget.
	Counted bool
}

// HandleNATPMP processes one NAT-PMP datagram that arrived
// on the trusted network n from clientAddr.
func (s *Server) HandleNATPMP(datagram []byte, clientAddr netip.Addr, n upnp.Network) NATPMPResult {
	res := NATPMPResult{}
	if !s.core.admit(clientAddr.String()) {
		res.Counted = true
		// Still answer so the client learns it is refused; a
		// silent drop would trigger client retries.
		res.Response = refuseNATPMP(datagram, proto.NATPMPResultRefused)
		s.core.reject(true, "nat-pmp", clientAddr.String(), n.NetworkRef, "rate limit exceeded")
		return res
	}
	req, err := proto.ParseNATPMPRequest(datagram)
	if err != nil {
		res.Response = refuseNATPMP(datagram, proto.NATPMPResultRefused)
		s.core.reject(false, "nat-pmp", clientAddr.String(), n.NetworkRef, "malformed request: "+err.Error())
		return res
	}
	if !clientAllowed(n, clientAddr.String()) {
		res.Response = refuseNATPMP(datagram, proto.NATPMPResultNotAuthorized)
		s.core.reject(false, "nat-pmp", clientAddr.String(), n.NetworkRef, "client not in allow-list")
		return res
	}
	if req.Op == proto.NATPMPOpPublicAddress {
		res.Response = s.publicAddressResponse()
		return res
	}
	transport := upnp.Transport(req.TransportProtocol())
	sink := s.core.sink
	if sink == nil {
		res.Response = refuseNATPMP(datagram, proto.NATPMPResultNetworkFailure)
		s.core.reject(false, "nat-pmp", clientAddr.String(), n.NetworkRef, "mapping sink unavailable")
		return res
	}
	// Global and per-client capacity.
	mappings := sink.Mappings()
	if len(mappings) >= s.core.limits.MaxMappingsTotal {
		res.Response = refuseNATPMP(datagram, proto.NATPMPResultOutOfResources)
		s.core.reject(false, "nat-pmp", clientAddr.String(), n.NetworkRef, "global mapping limit reached")
		return res
	}
	if sinkClientCount(mappings, clientAddr.String()) >= s.core.limits.MaxMappingsPerClient {
		res.Response = refuseNATPMP(datagram, proto.NATPMPResultOutOfResources)
		s.core.reject(false, "nat-pmp", clientAddr.String(), n.NetworkRef, "per-client mapping quota reached")
		return res
	}
	now := s.core.nowFn()
	extPort := int(req.ExternalPort)
	if extPort == 0 {
		extPort = int(req.InternalPort)
	}
	if !portFree(mappings, transport, extPort, clientAddr.String(), int(req.InternalPort)) {
		res.Response = refuseNATPMP(datagram, proto.NATPMPResultNotAuthorized)
		s.core.reject(false, "nat-pmp", clientAddr.String(), n.NetworkRef, "external port already mapped")
		return res
	}
	lifetime := s.core.effectiveLifetime(req.LifetimeSec)
	m := buildMapping(upnp.ProtoNATPMP, transport, n, clientAddr.String(), int(req.InternalPort), extPort, lifetime, now)
	m.ID = renewalID(mappings, m.ID, m)
	if err := sink.AddMapping(m, "nat-pmp:"+clientAddr.String()); err != nil {
		res.Response = refuseNATPMP(datagram, proto.NATPMPResultRefused)
		s.core.reject(false, "nat-pmp", clientAddr.String(), n.NetworkRef, "rejected by runtime: "+err.Error())
		return res
	}
	s.core.observeSuccess()
	resp := proto.NATPMPResponse{
		Op:           req.Op,
		ResultCode:   proto.NATPMPResultSuccess,
		EpochSec:     s.epoch(now),
		InternalPort: req.InternalPort,
		ExternalPort: uint16(extPort),
		LifetimeSec:  lifetime,
	}
	res.Response = resp.EncodeMapping()
	res.Accepted = true
	return res
}

// publicAddressResponse answers a public-address request. A
// missing observation is an honest network-failure answer,
// never a fabricated address.
func (s *Server) publicAddressResponse() []byte {
	s.core.mu.Lock()
	ip := s.core.extIPv4
	s.core.mu.Unlock()
	resp := proto.NATPMPResponse{Op: proto.NATPMPOpPublicAddress, EpochSec: s.epoch(time.Now())}
	if ip == "" {
		resp.ResultCode = proto.NATPMPResultNetworkFailure
	} else {
		addr, err := netip.ParseAddr(ip)
		if err != nil || !addr.Is4() {
			resp.ResultCode = proto.NATPMPResultNetworkFailure
		} else {
			a4 := addr.As4()
			copy(resp.ExternalIPv4[:], a4[:])
		}
	}
	return resp.EncodePublicAddress()
}

// epoch returns seconds since frontend start.
func (s *Server) epoch(now time.Time) uint32 {
	d := now.Sub(s.core.started)
	if d < 0 {
		d = 0
	}
	return uint32(d / time.Second)
}

// portFree reports whether extPort is available on transport.
// The owner of an existing identical tuple (renewal) passes.
func portFree(mappings []upnp.Mapping, transport upnp.Transport, extPort int, clientIP string, intPort int) bool {
	for _, m := range mappings {
		if m.ExternalPort != extPort {
			continue
		}
		if sameTrans(m.Transport, transport) {
			// Occupied on this transport: allowed only as an
			// exact renewal by the same client to the same port.
			if m.ClientIP == clientIP && m.InternalPort == intPort {
				continue
			}
			return false
		}
	}
	return true
}

// renewalID keeps the previous mapping id for an exact
// renewal tuple so the runtime replaces rather than
// duplicates it.
func renewalID(mappings []upnp.Mapping, candidateID string, m upnp.Mapping) string {
	for _, existing := range mappings {
		if existing.ClientIP == m.ClientIP &&
			externalPortsEqual(existing, m) &&
			sameTrans(existing.Transport, m.Transport) &&
			existing.InternalPort == m.InternalPort &&
			existing.Protocol == m.Protocol &&
			existing.NetworkRef == m.NetworkRef {
			return existing.ID
		}
	}
	return candidateID
}

func externalPortsEqual(a, b upnp.Mapping) bool { return a.ExternalPort == b.ExternalPort }

func sameTrans(a, b upnp.Transport) bool {
	if a == "" {
		a = upnp.TransportTCP
	}
	if b == "" {
		b = upnp.TransportTCP
	}
	return a == b
}

func sinkClientCount(mappings []upnp.Mapping, clientIP string) int {
	n := 0
	for _, m := range mappings {
		if m.ClientIP == clientIP {
			n++
		}
	}
	return n
}

// refuseNATPMP builds a best-effort refusal echoing the
// opcode when the header is parseable.
func refuseNATPMP(datagram []byte, code uint16) []byte {
	out := make([]byte, 8)
	out[0] = proto.NATPMPVersion
	if len(datagram) >= 2 {
		op := datagram[1]
		switch op {
		case proto.NATPMPOpUDPPortMapping, proto.NATPMPOpTCPPortMapping, proto.NATPMPOpPublicAddress:
			out[1] = op + 128
		}
	}
	binary.BigEndian.PutUint16(out[2:4], code)
	return out
}
