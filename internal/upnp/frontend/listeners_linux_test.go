//go:build linux

// Socket-level qualification for the NAT-PMP/PCP listeners.
// Drives a real UDP datagram through listenUDP on an
// ephemeral port in an isolated namespace and asserts the wire response.
// Loopback-source rejection is tested separately.
package frontend

import (
	"encoding/binary"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/upnp/proto"
)

func loopbackBind(t *testing.T) *udpSocket {
	t.Helper()
	addr := netip.MustParseAddr("192.0.2.1")
	s, _ := newTestServer(t, nil)
	sock, err := listenUDP(t.Context(), s, "lan.test", addr, 0, Bind{Prefixes: []netip.Prefix{netip.MustParsePrefix("192.0.2.0/24")}})
	if err != nil {
		t.Fatalf("listenUDP: %v", err)
	}
	return sock
}

func TestNATPMPUDPRoundTrip(t *testing.T) {
	if !isolatedUDPSocketTest(t) {
		return
	}
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
	binary.BigEndian.PutUint16(req[4:6], 5000)
	binary.BigEndian.PutUint32(req[8:12], 600)
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
	if !isolatedUDPSocketTest(t) {
		return
	}
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
	if code := binary.BigEndian.Uint16(buf[2:4]); code != proto.NATPMPResultUnsupportedVer {
		t.Fatalf("refusal code: %d", code)
	}
}

// Re-exec in a fresh netns: no listener is ever opened on management LAN.
func isolatedUDPSocketTest(t *testing.T) bool {
	t.Helper()
	if os.Getenv("DUCKOS_UDP_TEST_CHILD") == t.Name() {
		self, err := os.Readlink("/proc/self/ns/net")
		init, err2 := os.Readlink("/proc/1/ns/net")
		if err != nil || err2 != nil || self == init {
			t.Fatal("refusing host namespace")
		}
		for _, args := range [][]string{{"link", "set", "lo", "up"}, {"addr", "add", "192.0.2.1/24", "dev", "lo"}} {
			if out, e := exec.Command("ip", args...).CombinedOutput(); e != nil {
				t.Fatalf("setup: %v %s", e, out)
			}
		}
		return true
	}
	if os.Geteuid() != 0 {
		t.Skip("requires root for isolated UDP namespace")
	}
	exe, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	cmd := exec.Command("unshare", "--net", exe, "-test.run=^"+t.Name()+"$", "-test.v")
	cmd.Env = append(os.Environ(), "DUCKOS_UDP_TEST_CHILD="+t.Name())
	out, e := cmd.CombinedOutput()
	if e != nil || !strings.Contains(string(out), "--- PASS:") {
		t.Fatalf("namespace child: %v %s", e, out)
	}
	t.Log(string(out))
	return false
}

func TestNATPMPUDPLoopbackIsRejected(t *testing.T) {
	s, _ := newTestServer(t, nil)
	sock, e := listenUDP(t.Context(), s, "lan.test", netip.MustParseAddr("127.0.0.1"), 0, Bind{})
	if e != nil {
		t.Fatal(e)
	}
	defer sock.cancel()
	defer sock.conn.Close()
	c, e := net.DialUDP("udp4", nil, sock.conn.LocalAddr().(*net.UDPAddr))
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	_, e = c.Write([]byte{0, 0})
	if e != nil {
		t.Fatal(e)
	}
	c.SetReadDeadline(time.Now().Add(150 * time.Millisecond))
	if _, e = c.Read(make([]byte, 64)); !isTimeout(e) {
		t.Fatalf("loopback must be dropped: %v", e)
	}
}
