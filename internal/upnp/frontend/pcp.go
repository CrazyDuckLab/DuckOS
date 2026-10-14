// pcp.go: the PCP request handler (RFC 6887 subset).
// ANNOUNCE is answered with the epoch; MAP is validated
// against the same limits as NAT-PMP. PEER requests and
// unsupported options are refused before any state changes.
package frontend

import (
	"net/netip"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/upnp"
	"github.com/CrazyDuckLab/DuckOS/internal/upnp/proto"
)

// PCPResult is one handled PCP datagram outcome.
type PCPResult struct {
	Response []byte
	Accepted bool
	Counted  bool
}

// HandlePCP processes one PCP datagram that arrived on the
// trusted network n from clientAddr.
func (s *Server) HandlePCP(datagram []byte, clientAddr netip.Addr, n upnp.Network) PCPResult {
	res := PCPResult{}
	if !s.core.admit(clientAddr.String()) {
		res.Counted = true
		res.Response = s.pcpRefuse(datagram, clientAddr, proto.PCPResultNotAuthorized)
		s.core.reject(true, "pcp", clientAddr.String(), n.NetworkRef, "rate limit exceeded")
		return res
	}
	req, err := proto.ParsePCPRequest(datagram)
	if err != nil {
		res.Response = s.pcpRefuse(datagram, clientAddr, proto.PCPResultMalformedRequest)
		s.core.reject(false, "pcp", clientAddr.String(), n.NetworkRef, "malformed request: "+err.Error())
		return res
	}
	// RFC 6887 §11.3: the header client address must equal the
	// packet source; otherwise the request is unauthenticated
	// third-party signalling and is refused.
	if req.InternalIP.IsValid() && req.InternalIP != clientAddr {
		res.Response = s.pcpRefuse(datagram, clientAddr, proto.PCPResultAddressMismatch)
		s.core.reject(false, "pcp", clientAddr.String(), n.NetworkRef, "header address differs from packet source")
		return res
	}
	if req.Op == proto.PCPOpAnnounce {
		now := s.core.nowFn()
		resp := proto.PCPResponse{
			Op:         proto.PCPOpAnnounce,
			ResultCode: proto.PCPResultSuccess,
			EpochSec:   s.epoch(now),
			InternalIP: clientAddr,
		}
		res.Response = resp.Encode()
		return res
	}
	// MAP opcode from here on.
	if code, ok := req.HasUnsupportedOption(); ok {
		res.Response = s.pcpRefuse(datagram, clientAddr, proto.PCPResultUnsuppOption)
		s.core.reject(false, "pcp", clientAddr.String(), n.NetworkRef, "unsupported option code")
		_ = code
		return res
	}
	transport := upnp.Transport(req.TransportProtocol())
	if transport == "" {
		res.Response = s.pcpRefuse(datagram, clientAddr, proto.PCPResultUnsupportedProto)
		s.core.reject(false, "pcp", clientAddr.String(), n.NetworkRef, "unsupported transport protocol number")
		return res
	}
	if !clientAllowed(n, clientAddr.String()) {
		res.Response = s.pcpRefuse(datagram, clientAddr, proto.PCPResultNotAuthorized)
		s.core.reject(false, "pcp", clientAddr.String(), n.NetworkRef, "client not in allow-list")
		return res
	}
	sink := s.core.sink
	if sink == nil {
		res.Response = s.pcpRefuse(datagram, clientAddr, proto.PCPResultNetworkFailure)
		s.core.reject(false, "pcp", clientAddr.String(), n.NetworkRef, "mapping sink unavailable")
		return res
	}
	mappings := sink.Mappings()
	if len(mappings) >= s.core.limits.MaxMappingsTotal {
		res.Response = s.pcpRefuse(datagram, clientAddr, proto.PCPResultNoResources)
		s.core.reject(false, "pcp", clientAddr.String(), n.NetworkRef, "global mapping limit reached")
		return res
	}
	if sinkClientCount(mappings, clientAddr.String()) >= s.core.limits.MaxMappingsPerClient {
		res.Response = s.pcpRefuse(datagram, clientAddr, proto.PCPResultNoResources)
		s.core.reject(false, "pcp", clientAddr.String(), n.NetworkRef, "per-client mapping quota reached")
		return res
	}
	// Suggested external IP: zero (any) or an exact match of
	// the observed external address; anything else is the
	// cannot-provide-external conflict per RFC 6887 §11.1.
	if ext := req.ExtIP; ext.IsValid() && !ext.IsUnspecified() {
		if observed := s.observedExternal(); !observed.IsValid() || observed != ext {
			res.Response = s.pcpRefuse(datagram, clientAddr, proto.PCPResultCannotProvideExt)
			s.core.reject(false, "pcp", clientAddr.String(), n.NetworkRef, "suggested external address not available")
			return res
		}
	}
	extPort := int(req.ExternalPort)
	if extPort == 0 {
		extPort = int(req.InternalPort)
	}
	if !portFree(mappings, transport, extPort, clientAddr.String(), int(req.InternalPort)) {
		res.Response = s.pcpRefuse(datagram, clientAddr, proto.PCPResultNotAuthorized)
		s.core.reject(false, "pcp", clientAddr.String(), n.NetworkRef, "external port already mapped")
		return res
	}
	lifetime := s.core.effectiveLifetime(req.LifetimeSec)
	now := s.core.nowFn()
	m := buildMapping(upnp.ProtoPCP, transport, n, clientAddr.String(), int(req.InternalPort), extPort, lifetime, now)
	m.ID = renewalID(mappings, m.ID, m)
	if err := sink.AddMapping(m, "pcp:"+clientAddr.String()); err != nil {
		res.Response = s.pcpRefuse(datagram, clientAddr, proto.PCPResultNotAuthorized)
		s.core.reject(false, "pcp", clientAddr.String(), n.NetworkRef, "rejected by runtime: "+err.Error())
		return res
	}
	s.core.observeSuccess()
	resp := proto.PCPResponse{
		Op:           proto.PCPOpMap,
		ResultCode:   proto.PCPResultSuccess,
		LifetimeSec:  lifetime,
		EpochSec:     s.epoch(now),
		InternalIP:   clientAddr,
		MapNonce:     req.MapNonce,
		ProtocolNum:  req.ProtocolNum,
		InternalPort: req.InternalPort,
		ExternalPort: uint16(extPort),
	}
	res.Response = resp.Encode()
	res.Accepted = true
	return res
}

// pcpRefuse builds a refusal echoing the request opcode. An
// unparseable datagram is dropped (nil) — echoing an unknown
// opcode would confuse clients.
func (s *Server) pcpRefuse(datagram []byte, clientAddr netip.Addr, code byte) []byte {
	if len(datagram) < 2 {
		return nil
	}
	req, err := proto.ParsePCPRequest(datagram)
	if err != nil {
		// Header-only echo when the payload is what failed.
		if len(datagram) >= proto.PCPMinRequest && datagram[0] == proto.PCPVersion {
			req = proto.PCPRequest{Op: datagram[1], InternalIP: clientAddr}
			if datagram[1] == proto.PCPOpMap && len(datagram) >= proto.PCPMinRequest+proto.PCPMapPayloadSize {
				copy(req.MapNonce[:], datagram[proto.PCPMinRequest:proto.PCPMinRequest+12])
			}
		} else {
			return nil
		}
	}
	resp := proto.PCPResponse{
		Op:         req.Op,
		ResultCode: code,
		EpochSec:   s.epoch(s.core.nowFn()),
		InternalIP: clientAddr,
		MapNonce:   req.MapNonce,
	}
	return resp.Encode()
}

// observedExternal returns the observed WAN address as
// netip.Addr (invalid when unobserved).
func (s *Server) observedExternal() netip.Addr {
	s.core.mu.Lock()
	ip := s.core.extIPv4
	s.core.mu.Unlock()
	if ip == "" {
		return netip.Addr{}
	}
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return netip.Addr{}
	}
	return addr
}

// ServerUptimeSec exposes the epoch for diagnostics.
func (s *Server) ServerUptimeSec() uint32 {
	return s.epoch(s.core.nowFn())
}

// NowAt is a test seam: the server's notion of now.
func (s *Server) NowAt() time.Time { return s.core.nowFn() }
