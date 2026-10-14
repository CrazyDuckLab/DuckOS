// Package proto implements the wire formats for the beta.3
// port-mapping protocol frontends: NAT-PMP (RFC 6886), PCP
// (RFC 6887) and SSDP (UPnP discovery).
//
// The decoders are deliberately paranoid: every read is
// bounds-checked, packets above the RFC maximum are refused
// before parsing, and no field is trusted without range
// validation. The types are plain data so the servers can
// be tested without sockets on any build target.
package proto

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// NATPMPVersion is the NAT-PMP protocol version (RFC 6886).
const NATPMPVersion = 0

// NAT-PMP opcodes (client -> server).
const (
	NATPMPOpUDPPortMapping = 1
	NATPMPOpTCPPortMapping = 2
	NATPMPOpPublicAddress  = 128
)

// NAT-PMP result codes (server -> client).
const (
	NATPMPResultSuccess        = 0
	NATPMPResultUnsupportedVer = 1
	NATPMPResultNotAuthorized  = 2
	NATPMPResultNetworkFailure = 3
	NATPMPResultOutOfResources = 4
	NATPMPResultUnsupportedOp  = 5
	NATPMPResultRefused        = 6
)

// NATPMPMaxRequest is the largest request the server will
// parse. A port-mapping request is 12 bytes; a public-address
// request is 2. Anything larger is malformed by definition.
const NATPMPMaxRequest = 12

// NATPMPRequest is one decoded NAT-PMP request.
type NATPMPRequest struct {
	Version byte
	Op      byte
	// InternalPort / ExternalPort / Lifetime are only
	// meaningful for mapping requests.
	InternalPort uint16
	ExternalPort uint16
	LifetimeSec  uint32
}

// TransportProtocol reports "udp" or "tcp" for mapping
// requests, or "" for non-mapping opcodes.
func (r NATPMPRequest) TransportProtocol() string {
	switch r.Op {
	case NATPMPOpUDPPortMapping:
		return "udp"
	case NATPMPOpTCPPortMapping:
		return "tcp"
	}
	return ""
}

// ParseNATPMPRequest decodes one NAT-PMP request. It refuses
// nil buffers, oversized packets, unknown versions and
// unknown opcodes. The src port of the UDP datagram carries
// the internal port per RFC 6886 §3.2 — the server passes
// the received InternalPort from the packet (clients set it
// to their source port) and the server should verify equality
// at the transport layer.
func ParseNATPMPRequest(buf []byte) (NATPMPRequest, error) {
	if len(buf) == 0 {
		return NATPMPRequest{}, errors.New("natpmp: empty request")
	}
	if len(buf) > NATPMPMaxRequest {
		return NATPMPRequest{}, fmt.Errorf("natpmp: request too large (%d bytes)", len(buf))
	}
	req := NATPMPRequest{Version: buf[0]}
	if req.Version != NATPMPVersion {
		return NATPMPRequest{}, fmt.Errorf("natpmp: unsupported version %d", req.Version)
	}
	switch req.Op = buf[1]; req.Op {
	case NATPMPOpUDPPortMapping, NATPMPOpTCPPortMapping:
		if len(buf) < 12 {
			return NATPMPRequest{}, errors.New("natpmp: truncated mapping request")
		}
		req.InternalPort = binary.BigEndian.Uint16(buf[2:4])
		req.ExternalPort = binary.BigEndian.Uint16(buf[4:6])
		req.LifetimeSec = binary.BigEndian.Uint32(buf[6:12])
	case NATPMPOpPublicAddress:
		if len(buf) != 2 {
			return NATPMPRequest{}, errors.New("natpmp: malformed public-address request")
		}
	default:
		return NATPMPRequest{}, fmt.Errorf("natpmp: unsupported opcode %d", req.Op)
	}
	return req, nil
}

// NATPMPResponse is one encoded NAT-PMP response.
type NATPMPResponse struct {
	Op           byte
	ResultCode   uint16
	EpochSec     uint32
	InternalPort uint16
	ExternalPort uint16
	LifetimeSec  uint32
	// ExternalIPv4 is only set for public-address responses.
	ExternalIPv4 [4]byte
}

// EncodeMapping serialises a port-mapping response
// (8-byte header + 10-byte mapping payload).
func (r NATPMPResponse) EncodeMapping() []byte {
	out := make([]byte, 16)
	out[0] = NATPMPVersion
	out[1] = r.Op + 128
	binary.BigEndian.PutUint16(out[2:4], r.ResultCode)
	binary.BigEndian.PutUint32(out[4:8], r.EpochSec)
	binary.BigEndian.PutUint16(out[8:10], r.InternalPort)
	binary.BigEndian.PutUint16(out[10:12], r.ExternalPort)
	binary.BigEndian.PutUint32(out[12:16], r.LifetimeSec)
	return out
}

// EncodePublicAddress serialises a public-address response.
func (r NATPMPResponse) EncodePublicAddress() []byte {
	out := make([]byte, 12)
	out[0] = NATPMPVersion
	out[1] = r.Op + 128
	binary.BigEndian.PutUint16(out[2:4], r.ResultCode)
	binary.BigEndian.PutUint32(out[4:8], r.EpochSec)
	copy(out[8:12], r.ExternalIPv4[:])
	return out
}
