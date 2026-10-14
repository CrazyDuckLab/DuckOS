//go:build linux

// Socket-level qualification for the NAT-PMP/PCP listeners.
// Drives a real UDP datagram through listenUDP on an
// ephemeral loopback port and asserts the wire response.
// No privileged operations, no interface mutation: loopback
// only, everything torn down on return.
package frontend

import (
	"encoding/binary"
	"net"
	"net/netip"
	"testing"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/upnp/proto"
)

func loopbackBind(t *testing.T) *udpSocket {
	t.Helper()
	addr := netip.MustParseAddr("127.0.0.1")
	s, _ := newTestServer(t, nil)
	sock, err := listenUDP(t.Context(), s, "lan.test", addr, 0, Bind{})
	if err != nil {
		t.Fatalf("listenUDP: %v", err)
	}
	return sock
}

func TestNATPMPUDPRoundTrip(t *testing.T) {
	sock := loopbackBind(t)
	defer func() {
		sock.cancel()
		_ = sock.conn.Close()
	}()
	local := sock.conn.LocalAddr().(*net.UDPAddr)
	c, err := net.DialUDP("udp4", nil, local)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()
	req := make([]byte, 12)
	req[0] = 0
	req[1] = proto.NATPMPOpUDPPortMapping
	binary.BigEndian.PutUint16(req[2:4], 5000)
	binary.BigEndian.PutUint32(req[6:12], 600)
	if _, err := c.Write(req); err != nil {
		t.Fatalf("write: %v", err)
	}
	_ = c.SetReadDeadline(time.Now().Add(3 * time.Second))
	buf := make([]byte, 64)
	n, err := c.Read(buf)
	if err != nil {
		t.Fatalf("no response: %v", err)
	}
	if n != 16 {
		t.Fatalf("bad response length: %d", n)
	}
	if buf[1] != proto.NATPMPOpUDPPortMapping+128 {
		t.Fatalf("bad response op: %d", buf[1])
	}
	if code := binary.BigEndian.Uint16(buf[2:4]); code != proto.NATPMPResultSuccess {
		t.Fatalf("result code: %d", code)
	}
	if got := binary.BigEndian.Uint16(buf[10:12]); got != 5000 {
		t.Fatalf("external port: %d", got)
	}
}

func TestNATPMPUDPRefusalForBadPacket(t *testing.T) {
	sock := loopbackBind(t)
	defer func() {
		sock.cancel()
		_ = sock.conn.Close()
	}()
	local := sock.conn.LocalAddr().(*net.UDPAddr)
	c, err := net.DialUDP("udp4", nil, local)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()
	// Unsupported version: server answers with the version error.
	bad := []byte{7, 1, 0, 0}
	if _, err := c.Write(bad); err != nil {
		t.Fatalf("write: %v", err)
	}
	_ = c.SetReadDeadline(time.Now().Add(3 * time.Second))
	buf := make([]byte, 64)
	n, err := c.Read(buf)
	if err != nil {
		t.Fatalf("no refusal: %v", err)
	}
	if n < 4 {
		t.Fatalf("short refusal: %d bytes", n)
	}
	if code := binary.BigEndian.Uint16(buf[2:4]); code != proto.NATPMPResultRefused {
		t.Fatalf("refusal code: %d", code)
	}
}
