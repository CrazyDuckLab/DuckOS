// pcp.go: PCP (RFC 6887) wire format — strict decoder and
// encoder. Only the ANNOUNCE and MAP opcodes are supported;
// PEER and everything else is refused. The THIRD_PARTY and
// PREFER_FAILURE options are rejected so an untrusted client
// can never open a port for a neighbour or force conflict
// behaviour.
package proto

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net/netip"
)

// PCPVersion is the PCP protocol version (RFC 6887).
const PCPVersion = 2

// PCP opcodes.
const (
	PCPOpAnnounce = 1
	PCPOpMap      = 2
	PCPOpPeer     = 3
)

// PCP result codes.
const (
	PCPResultSuccess          = 0
	PCPResultUnsuppVersion    = 1
	PCPResultNotAuthorized    = 2
	PCPResultMalformedRequest = 3
	PCPResultUnsuppOpcode     = 4
	PCPResultUnsuppOption     = 5
	PCPResultMalformedOption  = 6
	PCPResultNetworkFailure   = 7
	PCPResultNoResources      = 8
	PCPResultUnsupportedProto = 9
	PCPResultAddressMismatch  = 10
	PCPResultCannotProvideExt = 11
)

// PCP option codes we explicitly evaluate.
const (
	PCPOptThirdParty    = 1
	PCPOptPreferFail    = 2
	PCPOptFilter        = 3
	PCPOptPortSetOption = 128 // RFC 7753
)

// PCPMinRequest is the fixed 24-byte PCP header.
const PCPMinRequest = 24

// PCPMaxRequest is the RFC 6887 maximum: 1100 octets.
const PCPMaxRequest = 1100

// PCPMapPayloadSize is the fixed MAP opcode payload size:
// 12 nonce + 4 protocol/reserved + 2+2 ports + 16 address + 4 lifetime.
const PCPMapPayloadSize = 40

// PCPRequest is one decoded PCP request.
type PCPRequest struct {
	Version byte
	Op      byte
	// LifetimeSec is the requested mapping lifetime.
	LifetimeSec uint32
	// InternalIP is the client address copied from the fixed
	// header. The server MUST verify it equals the UDP source
	// address (RFC 6887 §11.3); a mismatch is refused before
	// any state changes.
	InternalIP netip.Addr
	// MAP payload fields.
	MapNonce     [12]byte
	ProtocolNum  byte
	InternalPort uint16
	ExternalPort uint16
	// ExtIP is the Suggested External IP address field. A
	// non-zero address that does not match the server's
	// external address is a conflict; zero means "any".
	ExtIP netip.Addr
	// Options carries the raw option bytes (already
	// bounds-validated). The server evaluates them.
	Options []PCPOption
}

// TransportProtocol returns "udp", "tcp" or "" per the MAP
// protocol field.
func (r PCPRequest) TransportProtocol() string {
	switch r.ProtocolNum {
	case 17:
		return "udp"
	case 6:
		return "tcp"
	}
	return ""
}

// HasUnsupportedOption reports whether the request carries an
// option the DuckOS frontend must refuse (THIRD_PARTY,
// PREFER_FAILURE, FILTER, PORT_SET).
func (r PCPRequest) HasUnsupportedOption() (byte, bool) {
	for _, o := range r.Options {
		switch o.Code {
		case PCPOptThirdParty, PCPOptPreferFail, PCPOptFilter, PCPOptPortSetOption:
			return o.Code, true
		}
	}
	return 0, false
}

// PCPOption is one decoded TLV option.
type PCPOption struct {
	Code byte
	Data []byte
}

// ParsePCPRequest decodes one PCP request. ANNOUNCE uses the
// bare 24-byte header; MAP adds the 36-byte payload. Options
// are parsed and length-validated but not trusted.
func ParsePCPRequest(buf []byte) (PCPRequest, error) {
	if len(buf) < PCPMinRequest {
		return PCPRequest{}, errors.New("pcp: request shorter than fixed header")
	}
	if len(buf) > PCPMaxRequest {
		return PCPRequest{}, fmt.Errorf("pcp: request too large (%d bytes)", len(buf))
	}
	req := PCPRequest{Version: buf[0], Op: buf[1]}
	if req.Version != PCPVersion {
		return PCPRequest{}, fmt.Errorf("pcp: unsupported version %d", req.Version)
	}
	req.LifetimeSec = binary.BigEndian.Uint32(buf[4:8])
	ip, ok := netip.AddrFromSlice(buf[8:24])
	if !ok {
		return PCPRequest{}, errors.New("pcp: malformed client address")
	}
	req.InternalIP = ip.Unmap()
	switch req.Op {
	case PCPOpAnnounce:
		if len(buf) != PCPMinRequest {
			return PCPRequest{}, errors.New("pcp: announce carries unexpected payload")
		}
		return req, nil
	case PCPOpMap:
		if len(buf) < PCPMinRequest+PCPMapPayloadSize {
			return PCPRequest{}, errors.New("pcp: truncated MAP payload")
		}
		p := buf[PCPMinRequest:]
		copy(req.MapNonce[:], p[0:12])
		req.ProtocolNum = p[12]
		req.InternalPort = binary.BigEndian.Uint16(p[16:18])
		req.ExternalPort = binary.BigEndian.Uint16(p[18:20])
		if ext, ok := netip.AddrFromSlice(p[20:36]); ok {
			req.ExtIP = ext.Unmap()
		}
		req.LifetimeSec = binary.BigEndian.Uint32(p[36:40])
		rest := p[PCPMapPayloadSize:]
		opts, err := parsePCPOptions(rest)
		if err != nil {
			return PCPRequest{}, err
		}
		req.Options = opts
		return req, nil
	default:
		return PCPRequest{}, fmt.Errorf("pcp: unsupported opcode %d", req.Op)
	}
}

// parsePCPOptions walks the TLV option chain with strict
// bounds: each option is 2 bytes header + 4*n bytes data.
func parsePCPOptions(buf []byte) ([]PCPOption, error) {
	var out []PCPOption
	for len(buf) > 0 {
		if len(buf) < 2 {
			return nil, errors.New("pcp: truncated option header")
		}
		code := buf[0]
		words := int(buf[1])
		size := 2 + words*4
		if len(buf) < size {
			return nil, errors.New("pcp: option data exceeds packet")
		}
		data := buf[2:size]
		// Reserved option code 0 is malformed per RFC 6887.
		if code == 0 {
			return nil, errors.New("pcp: reserved option code 0")
		}
		out = append(out, PCPOption{Code: code, Data: append([]byte(nil), data...)})
		buf = buf[size:]
	}
	return out, nil
}

// PCPResponse is one encoded PCP response. The server echoes
// the incoming opcode and the MAP nonce/lifetime.
type PCPResponse struct {
	Op           byte
	ResultCode   byte
	LifetimeSec  uint32
	EpochSec     uint32
	InternalIP   netip.Addr
	MapNonce     [12]byte
	ProtocolNum  byte
	InternalPort uint16
	ExternalPort uint16
	ExtIP        netip.Addr
}

// Encode serialises the response for ANNOUNCE or MAP.
func (r PCPResponse) Encode() []byte {
	out := make([]byte, PCPMinRequest+PCPMapPayloadSize)
	out[0] = PCPVersion
	out[1] = r.Op
	out[2] = 0 // reserved
	out[3] = r.ResultCode
	binary.BigEndian.PutUint32(out[4:8], r.LifetimeSec)
	binary.BigEndian.PutUint32(out[8:12], r.EpochSec)
	if r.InternalIP.Is4() {
		{
			a4 := r.InternalIP.As4()
			copy(out[12:16], a4[:])
		}
		// remaining 12 bytes are zero (IPv4-mapped form)
	} else if r.InternalIP.IsValid() {
		{
			a16 := r.InternalIP.As16()
			copy(out[12:28], a16[:])
		}
	}
	if r.Op != PCPOpAnnounce {
		p := out[PCPMinRequest:]
		copy(p[0:12], r.MapNonce[:])
		p[12] = r.ProtocolNum
		binary.BigEndian.PutUint16(p[16:18], r.InternalPort)
		binary.BigEndian.PutUint16(p[18:20], r.ExternalPort)
		if r.ExtIP.Is4() {
			{
				e4 := r.ExtIP.As4()
				copy(p[20:24], e4[:])
			}
		} else if r.ExtIP.IsValid() {
			{
				e16 := r.ExtIP.As16()
				copy(p[20:36], e16[:])
			}
		}
		binary.BigEndian.PutUint32(p[36:40], r.LifetimeSec)
	}
	return out
}
