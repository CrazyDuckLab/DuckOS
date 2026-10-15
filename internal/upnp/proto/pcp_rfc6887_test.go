// pcp_rfc6887_test.go: RFC-derived fixtures, INDEPENDENT of the
// DuckOS encoder. Every byte below is transcribed from RFC 6887
// section 7 (common header), section 11.1 (MAP) and section 8
// (result codes), so a renderer regression cannot hide here.
package proto

import (
	"bytes"
	"encoding/binary"
	"net/netip"
	"testing"
)

// rfcMapRequest builds a MAP request byte-for-byte per RFC 6887
// section 11.1: 24-byte header (lifetime in the header) + 36-byte
// MAP payload (nonce 12, protocol 1, reserved 3, ports 2+2, external
// IP 16).
func rfcMapRequest(protocol byte, internalPort, externalPort uint16, externalIP [4]byte, lifetime uint32) []byte {
	buf := make([]byte, PCPMinRequest+36)
	buf[0] = 2 // version
	buf[1] = 2 // opcode MAP (request: R bit clear)
	binary.BigEndian.PutUint32(buf[4:8], lifetime)
	client := netip.MustParseAddr("192.168.10.50").As16()
	copy(buf[8:24], client[:])
	p := buf[PCPMinRequest:]
	copy(p[0:12], []byte("rfc6887nonce"))
	p[12] = protocol
	binary.BigEndian.PutUint16(p[16:18], internalPort)
	binary.BigEndian.PutUint16(p[18:20], externalPort)
	copy(p[20:24], externalIP[:])
	return buf
}

func TestRFC6887MapRequestParses(t *testing.T) {
	req, err := ParsePCPRequest(rfcMapRequest(6, 5000, 5000, [4]byte{0, 0, 0, 0}, 3600))
	if err != nil {
		t.Fatalf("RFC-conformant MAP request must parse: %v", err)
	}
	if req.Op != PCPOpMap || req.ProtocolNum != 6 || req.TransportProtocol() != "tcp" {
		t.Fatalf("wrong decode: %+v", req)
	}
	if req.InternalPort != 5000 || req.ExternalPort != 5000 {
		t.Fatalf("wrong ports: %+v", req)
	}
	if req.LifetimeSec != 3600 {
		t.Fatalf("lifetime must come from the fixed header, got %d", req.LifetimeSec)
	}
	if string(req.MapNonce[:]) != "rfc6887nonce" {
		t.Fatalf("nonce corrupted: %q", req.MapNonce)
	}
	if !req.ExtIP.IsUnspecified() {
		t.Fatalf("zero external IP must decode as unspecified, got %s", req.ExtIP)
	}
}

// TestRFC6887LifetimeIsHeaderOnly: the audit's MAP-layout finding.
// Garbage in bytes 60..64 (option area) must never be read as a
// lifetime.
func TestRFC6887LifetimeIsHeaderOnly(t *testing.T) {
	bad := rfcMapRequest(17, 53, 53, [4]byte{}, 120)
	bad = append(bad, 0xde, 0x00) // one well-formed unknown option
	req, err := ParsePCPRequest(bad)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if req.LifetimeSec != 120 {
		t.Fatalf("lifetime leaked from option area: %d", req.LifetimeSec)
	}
}

func TestRFC6887AnnounceRequest(t *testing.T) {
	buf := make([]byte, 24)
	buf[0] = 2
	buf[1] = 1
	buf[4] = 0
	client := netip.MustParseAddr("192.168.10.50").As16()
	copy(buf[8:24], client[:])
	req, err := ParsePCPRequest(buf)
	if err != nil || req.Op != PCPOpAnnounce {
		t.Fatalf("announce must parse: %v", err)
	}
	// Any payload after the 24-byte announce header is malformed.
	if _, err := ParsePCPRequest(append(append([]byte{}, buf...), 0, 0, 0, 0)); err == nil {
		t.Fatal("announce with payload must be refused")
	}
}

// TestRFC6887ResponseWireFormat pins the encoded response bytes
// against the RFC layout: R bit set, epoch at 8..12, client address
// at 12..28, MAP payload at 28..64, no lifetime in the payload.
func TestRFC6887ResponseWireFormat(t *testing.T) {
	resp := PCPResponse{
		Op:           PCPOpMap,
		ResultCode:   PCPResultSuccess,
		LifetimeSec:  7200,
		EpochSec:     111111,
		InternalIP:   netip.MustParseAddr("192.168.10.50"),
		MapNonce:     [12]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12},
		ProtocolNum:  17,
		InternalPort: 5000,
		ExternalPort: 6000,
		ExtIP:        netip.MustParseAddr("203.0.113.7"),
	}
	out := resp.Encode()
	if len(out) != 64 {
		t.Fatalf("MAP response must be 28+36=64 bytes, got %d", len(out))
	}
	if out[0] != 2 {
		t.Fatalf("version: %d", out[0])
	}
	if out[1] != PCPOpMap|PCPOpcodeResponseBit {
		t.Fatalf("response opcode must carry the R bit: %02x", out[1])
	}
	if out[3] != PCPResultSuccess {
		t.Fatalf("result code: %d", out[3])
	}
	if got := binary.BigEndian.Uint32(out[4:8]); got != 7200 {
		t.Fatalf("granted lifetime at header 4..8: %d", got)
	}
	if got := binary.BigEndian.Uint32(out[8:12]); got != 111111 {
		t.Fatalf("epoch at header 8..12: %d", got)
	}
	client := netip.MustParseAddr("192.168.10.50").As4()
	if !bytes.Equal(out[12:16], client[:]) || !bytes.Equal(out[16:28], make([]byte, 12)) {
		t.Fatalf("client address must occupy 12..28 (IPv4-mapped), got % x", out[12:28])
	}
	if !bytes.Equal(out[28:40], []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}) {
		t.Fatalf("nonce must start at byte 28, got % x", out[28:40])
	}
	if out[40] != 17 {
		t.Fatalf("protocol at payload byte 12 (abs 40): %d", out[40])
	}
	if got := binary.BigEndian.Uint16(out[44:46]); got != 5000 {
		t.Fatalf("internal port at abs 44: %d", got)
	}
	if got := binary.BigEndian.Uint16(out[46:48]); got != 6000 {
		t.Fatalf("external port at abs 46: %d", got)
	}
	ext := netip.MustParseAddr("203.0.113.7").As4()
	if !bytes.Equal(out[48:52], ext[:]) {
		t.Fatalf("external IP at abs 48: % x", out[48:52])
	}

	// ANNOUNCE response: 28 bytes, header only.
	ann := PCPResponse{Op: PCPOpAnnounce, ResultCode: PCPResultSuccess, EpochSec: 42,
		InternalIP: netip.MustParseAddr("192.168.10.50")}.Encode()
	if len(ann) != 28 {
		t.Fatalf("announce response must be 28 bytes, got %d", len(ann))
	}
	if ann[1] != PCPOpAnnounce|PCPOpcodeResponseBit {
		t.Fatalf("announce response opcode: %02x", ann[1])
	}
	if got := binary.BigEndian.Uint32(ann[8:12]); got != 42 {
		t.Fatalf("announce epoch: %d", got)
	}
}

// TestRFC6887MalformedRequests: denial of malformed wire input.
func TestRFC6887MalformedRequests(t *testing.T) {
	short := rfcMapRequest(6, 1, 1, [4]byte{}, 60)[:30]
	if _, err := ParsePCPRequest(short); err == nil {
		t.Fatal("truncated MAP payload must be refused")
	}
	peer := rfcMapRequest(6, 1, 1, [4]byte{}, 60)
	peer[1] = PCPOpPeer
	if _, err := ParsePCPRequest(peer); err == nil {
		t.Fatal("PEER must be refused")
	}
	ver := rfcMapRequest(6, 1, 1, [4]byte{}, 60)
	ver[0] = 1
	if _, err := ParsePCPRequest(ver); err == nil {
		t.Fatal("version 1 must be refused")
	}
	badOpt := append(rfcMapRequest(6, 1, 1, [4]byte{}, 60), 0x01, 0x00) // THIRD_PARTY
	if req, err := ParsePCPRequest(badOpt); err != nil {
		t.Fatalf("option parsing must succeed before policy: %v", err)
	} else if _, found := req.HasUnsupportedOption(); !found {
		t.Fatal("THIRD_PARTY must be reported unsupported")
	}
	reservedOpt := append(rfcMapRequest(6, 1, 1, [4]byte{}, 60), 0x00, 0x00)
	if _, err := ParsePCPRequest(reservedOpt); err == nil {
		t.Fatal("reserved option code 0 must be refused")
	}
	oversized := make([]byte, PCPMaxRequest+1)
	if _, err := ParsePCPRequest(oversized); err == nil {
		t.Fatal("oversized datagram must be refused")
	}
}
