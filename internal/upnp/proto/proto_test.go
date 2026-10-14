// Cross-platform tests for the protocol wire formats. Every
// security-relevant parser property is exercised here so the
// suite runs green on any build host.
package proto

import (
	"bytes"
	"encoding/binary"
	"net/netip"
	"strings"
	"testing"
)

func TestParseNATPMPMappingRequest(t *testing.T) {
	buf := make([]byte, 12)
	buf[0] = 0
	buf[1] = NATPMPOpUDPPortMapping
	binary.BigEndian.PutUint16(buf[2:4], 5000)
	binary.BigEndian.PutUint16(buf[4:6], 0)
	binary.BigEndian.PutUint32(buf[6:12], 3600)
	req, err := ParseNATPMPRequest(buf)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if req.InternalPort != 5000 || req.ExternalPort != 0 || req.LifetimeSec != 3600 {
		t.Fatalf("fields: %+v", req)
	}
	if req.TransportProtocol() != "udp" {
		t.Fatalf("transport: %q", req.TransportProtocol())
	}
}

func TestParseNATPMPRefusals(t *testing.T) {
	cases := [][]byte{
		nil,
		{},
		bytes.Repeat([]byte{0}, 13),
		{1, NATPMPOpUDPPortMapping, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}, // bad version
		{0, 99},                       // unknown opcode
		{0, NATPMPOpPublicAddress, 0}, // malformed address req
		{0, NATPMPOpUDPPortMapping, 0, 0, 0, 0, 0, 0, 0}, // truncated mapping
	}
	for i, buf := range cases {
		if _, err := ParseNATPMPRequest(buf); err == nil {
			t.Fatalf("case %d: expected refusal", i)
		}
	}
}

func encodePCPMap(nonce [12]byte, protoNum byte, intPort, extPort uint16, extIP netip.Addr, lifetime uint32, opts ...[]byte) []byte {
	buf := make([]byte, PCPMinRequest+PCPMapPayloadSize)
	buf[0] = PCPVersion
	buf[1] = PCPOpMap
	binary.BigEndian.PutUint32(buf[4:8], lifetime)
	client := netip.MustParseAddr("192.168.10.50")
	a := client.As16()
	copy(buf[8:24], a[:])
	p := buf[PCPMinRequest:]
	copy(p[0:12], nonce[:])
	p[12] = protoNum
	binary.BigEndian.PutUint16(p[16:18], intPort)
	binary.BigEndian.PutUint16(p[18:20], extPort)
	if extIP.IsValid() {
		e := extIP.As16()
		copy(p[20:36], e[:])
	}
	binary.BigEndian.PutUint32(p[32:36], lifetime)
	for _, o := range opts {
		buf = append(buf, o...)
	}
	return buf
}

func TestParsePCPMapRequest(t *testing.T) {
	var nonce [12]byte
	for i := range nonce {
		nonce[i] = byte(i)
	}
	buf := encodePCPMap(nonce, 17, 5000, 5000, netip.Addr{}, 1800)
	req, err := ParsePCPRequest(buf)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if req.ProtocolNum != 17 || req.InternalPort != 5000 || req.TransportProtocol() != "udp" {
		t.Fatalf("fields: %+v", req)
	}
	if req.MapNonce != nonce {
		t.Fatal("nonce mismatch")
	}
}

func TestParsePCPRefusals(t *testing.T) {
	// THIRD_PARTY option (code 1, 0 words).
	thirdParty := []byte{PCPOptThirdParty, 1, 0, 0, 0, 0}
	buf := encodePCPMap([12]byte{}, 6, 80, 80, netip.Addr{}, 600, thirdParty)
	req, err := ParsePCPRequest(buf)
	if err != nil {
		t.Fatalf("parse with option: %v", err)
	}
	if code, ok := req.HasUnsupportedOption(); !ok || code != PCPOptThirdParty {
		t.Fatalf("THIRD_PARTY not detected: %v %v", code, ok)
	}
	// PEER opcode must be refused at parse level.
	peer := encodePCPMap([12]byte{}, 17, 80, 80, netip.Addr{}, 600)
	peer[1] = PCPOpPeer
	if _, err := ParsePCPRequest(peer); err == nil {
		t.Fatal("PEER opcode accepted")
	}
	// Truncated and oversized packets.
	if _, err := ParsePCPRequest(buf[:20]); err == nil {
		t.Fatal("truncated accepted")
	}
	if _, err := ParsePCPRequest(bytes.Repeat([]byte{2, 2}, 600)); err == nil {
		t.Fatal("oversized accepted")
	}
	// Malformed option chain (truncated header).
	bad := encodePCPMap([12]byte{}, 17, 80, 80, netip.Addr{}, 600)
	bad = append(bad, 3)
	if _, err := ParsePCPRequest(bad); err == nil {
		t.Fatal("truncated option accepted")
	}
}

func TestParseMSearch(t *testing.T) {
	req := "M-SEARCH * HTTP/1.1\r\nHOST: 239.255.255.250:1900\r\nMAN: \"ssdp:discover\"\r\nMX: 2\r\nST: urn:schemas-upnp-org:service:WANIPConnection:1\r\n\r\n"
	m, err := ParseMSearch([]byte(req))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if m.ST != STWANIPConn || m.MX != 2 || !m.RespondsTo() {
		t.Fatalf("fields: %+v", m)
	}
	// ssdp:all responds; an alien ST does not.
	m, err = ParseMSearch([]byte(strings.Replace(req, STWANIPConn, "ssdp:all", 1)))
	if err != nil || !m.RespondsTo() {
		t.Fatalf("ssdp:all: %v", err)
	}
	m, err = ParseMSearch([]byte(strings.Replace(req, STWANIPConn, "urn:evil:device", 1)))
	if err != nil || m.RespondsTo() {
		t.Fatal("alien ST responded")
	}
	// Not an M-SEARCH.
	if _, err := ParseMSearch([]byte("GET / HTTP/1.1\r\nST: ssdp:all\r\n\r\n")); err == nil {
		t.Fatal("GET accepted")
	}
	// Missing ST.
	if _, err := ParseMSearch([]byte("M-SEARCH * HTTP/1.1\r\nMX: 1\r\n\r\n")); err == nil {
		t.Fatal("missing ST accepted")
	}
	// MX clamped.
	m, err = ParseMSearch([]byte(strings.Replace(req, "MX: 2", "MX: 99", 1)))
	if err != nil || m.MX != 5 {
		t.Fatalf("MX clamp: %v %+v", err, m)
	}
	// Oversized datagram.
	if _, err := ParseMSearch(make([]byte, SSDPMaxRequest+1)); err == nil {
		t.Fatal("oversized accepted")
	}
}
