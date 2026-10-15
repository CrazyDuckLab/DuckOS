//go:build linux

// pcp_live_test.go: LIVE UDP qualification of the PCP frontend
// against the real kernel through the privileged launcher (RFC 6887
// scenario D). Skipped unless DUCKOS_LIVE_NFT=1, euid 0 and a
// dedicated network namespace; the suite drives the DuckOS-owned
// duckos_upnp table only and cleans it up.
package frontend

import (
	"bytes"
	"encoding/binary"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/nftlaunch"
	"github.com/CrazyDuckLab/DuckOS/internal/upnp"
	"github.com/CrazyDuckLab/DuckOS/internal/upnp/proto"
)

const liveNFTBin = "/usr/sbin/nft"

func skipUnlessLivePCP(t *testing.T) {
	t.Helper()
	// A fresh named netns has lo DOWN; the loopback is the test's
	// trusted client network.
	_ = exec.Command("ip", "link", "set", "lo", "up").Run()
	if os.Getenv("DUCKOS_LIVE_NFT") != "1" {
		t.Skip("live PCP qualification disabled (set DUCKOS_LIVE_NFT=1)")
	}
	self, err := os.Readlink("/proc/self/ns/net")
	init, initErr := os.Readlink("/proc/1/ns/net")
	if err != nil || initErr != nil || self == init {
		t.Fatal("refusing live PCP qualification in host network namespace")
	}
	if os.Geteuid() != 0 {
		t.Skip("live PCP qualification requires root")
	}
	if _, err := os.Stat(liveNFTBin); err != nil {
		t.Skip("nft binary not available")
	}
	startLiveLauncherPCP(t)
}

var (
	pcpLauncherOnce sync.Once
	pcpLauncherSock string
)

func startLiveLauncherPCP(t *testing.T) {
	t.Helper()
	pcpLauncherOnce.Do(func() {
		dir, mkErr := os.MkdirTemp("", "duckos-pcp-launcher")
		if mkErr != nil {
			t.Fatalf("launcher dir: %v", mkErr)
		}
		sock := filepath.Join(dir, "nft-launcher.sock")
		ln, err := net.Listen("unix", sock)
		if err != nil {
			t.Fatalf("launcher listen: %v", err)
		}
		if err := os.Chmod(sock, 0o660); err != nil {
			t.Fatalf("launcher chmod: %v", err)
		}
		go func() { _ = nftlaunch.ServeListener(ln, liveNFTBin, uint32(os.Getuid())) }()
		pcpLauncherSock = sock
		// Redirect EVERY client in this process (including the
		// production backend) to the test launcher. The installed
		// /run socket on a lab host must never be reachable from a
		// namespaced qualification run.
		prev := nftlaunch.SocketPath
		nftlaunch.SocketPath = func() string { return pcpLauncherSock }
		t.Cleanup(func() { nftlaunch.SocketPath = prev })
	})
	deadline := time.Now().Add(5 * time.Second)
	c := nftlaunch.NewClient()
	c.SetSocketFn(func() string { return pcpLauncherSock })
	for !c.Available() {
		if time.Now().After(deadline) {
			t.Fatal("in-process nft launcher never became available")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// rfcMapRequestLive is the RFC 6887 wire MAP request, transcribed
// independently of the DuckOS encoder (see proto/pcp_rfc6887_test.go).
func rfcMapRequestLive(protocol byte, internalPort, externalPort uint16, lifetime uint32, nonce [12]byte) []byte {
	buf := make([]byte, proto.PCPMinRequest+36)
	buf[0] = 2
	buf[1] = 2
	binary.BigEndian.PutUint32(buf[4:8], lifetime)
	client := netip.MustParseAddr("127.0.0.1").As16()
	copy(buf[8:24], client[:])
	p := buf[proto.PCPMinRequest:]
	copy(p[0:12], nonce[:])
	p[12] = protocol
	binary.BigEndian.PutUint16(p[16:18], internalPort)
	binary.BigEndian.PutUint16(p[18:20], externalPort)
	return buf
}

func cleanupUpnpTable(t *testing.T) {
	t.Helper()
	_ = exec.Command(liveNFTBin, "delete", "table", "inet", "duckos_upnp").Run()
	t.Cleanup(func() { _ = exec.Command(liveNFTBin, "delete", "table", "inet", "duckos_upnp").Run() })
}

func livePCPStack(t *testing.T) (*Server, *upnp.Runtime) {
	t.Helper()
	dir := t.TempDir()
	be, err := upnp.NewNftablesBackendTrusted(dir, []string{"lo"})
	if err != nil {
		t.Fatalf("backend: %v", err)
	}
	rt := upnp.NewRuntime(upnp.NewManager(), be)
	// A live runtime keeps ticking and re-rendering the shared
	// duckos_upnp table; it must not outlive its test.
	t.Cleanup(rt.Close)
	if err := rt.EnableNetwork(upnp.Network{
		NetworkRef: "lan",
		Protocols:  []upnp.Protocol{upnp.ProtoPCP},
		TTLSec:     3600,
	}, "lo"); err != nil {
		t.Fatalf("enable network: %v", err)
	}
	limits := DefaultLimits()
	limits.MaxLifetimeSec = 3600
	limits.MinLifetimeSec = 2 // the expiry test needs short grants
	// The expiry sweep only runs once the runtime is started.
	rt.Start(t.Context())
	return NewServer(limits, nil, rt), rt
}

// serveUDP echoes datagrams through HandlePCP over a real socket.
func serveUDP(t *testing.T, srv *Server) string {
	t.Helper()
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	go func() {
		buf := make([]byte, 2048)
		for {
			n, from, err := conn.ReadFromUDP(buf)
			if err != nil {
				return
			}
			addr, _ := netip.AddrFromSlice(from.IP)
			res := srv.HandlePCP(append([]byte(nil), buf[:n]...), addr, upnp.Network{NetworkRef: "lan"})
			if len(res.Response) > 0 {
				_, _ = conn.WriteToUDP(res.Response, from)
			}
		}
	}()
	return conn.LocalAddr().String()
}

func udpExchange(t *testing.T, addr string, packet []byte) []byte {
	t.Helper()
	client, err := net.Dial("udp4", addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()
	if _, err := client.Write(packet); err != nil {
		t.Fatalf("write: %v", err)
	}
	_ = client.SetReadDeadline(time.Now().Add(3 * time.Second))
	out := make([]byte, 2048)
	n, err := client.Read(out)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	return out[:n]
}

// TestLivePCPUDPMapAndReadback: real UDP request → validated mapping
// → launcher → kernel DNAT rule → RFC-conformant response → removal.
func TestLivePCPUDPMapAndReadback(t *testing.T) {
	skipUnlessLivePCP(t)
	cleanupUpnpTable(t)
	srv, rt := livePCPStack(t)
	addr := serveUDP(t, srv)

	var nonce [12]byte
	copy(nonce[:], []byte("livenonce0001"))
	req := rfcMapRequestLive(6, 5000, 5000, 600, nonce)
	resp := udpExchange(t, addr, req)

	if len(resp) != 64 {
		t.Fatalf("MAP response must be 64 bytes (28 header + 36 payload), got %d", len(resp))
	}
	if resp[0] != 2 || resp[1] != proto.PCPOpMap|proto.PCPOpcodeResponseBit {
		t.Fatalf("response header: % x", resp[:2])
	}
	if resp[3] != proto.PCPResultSuccess {
		t.Fatalf("result code %d: % x", resp[3], resp)
	}
	if got := binary.BigEndian.Uint16(resp[46:48]); got != 5000 {
		t.Fatalf("external port: %d", got)
	}

	// Kernel readback through the launcher: the DNAT rule must exist.
	client := nftlaunch.NewClient()
	client.SetSocketFn(func() string { return pcpLauncherSock })
	out, err := client.ListTableJSON("inet", "duckos_upnp")
	if err != nil {
		t.Fatalf("kernel readback: %v", err)
	}
	if !bytes.Contains(out, []byte("5000")) {
		t.Fatalf("kernel state must contain the mapping: %s", out)
	}

	// Renewal: the same nonce renews instead of duplicating.
	req2 := rfcMapRequestLive(6, 5000, 5000, 600, nonce)
	if resp2 := udpExchange(t, addr, req2); resp2[3] != proto.PCPResultSuccess {
		t.Fatalf("renewal refused: %d", resp2[3])
	}
	mappings := rt.Mappings()
	if len(mappings) != 1 {
		t.Fatalf("renewal duplicated mappings: %d", len(mappings))
	}

	// Deletion: explicit remove clears the kernel rule.
	if err := rt.RemoveMapping(mappings[0].ID); err != nil {
		t.Fatalf("remove: %v", err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		out, err = client.ListTableJSON("inet", "duckos_upnp")
		if err == nil && !bytes.Contains(out, []byte("dnat")) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("mapping never left the kernel: %s", out)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// TestLivePCPUDPExpiryRemoves: a short-lived mapping disappears from
// the kernel when the runtime expiry tick fires.
func TestLivePCPUDPExpiryRemoves(t *testing.T) {
	skipUnlessLivePCP(t)
	cleanupUpnpTable(t)
	srv, rt := livePCPStack(t)
	addr := serveUDP(t, srv)

	var nonce [12]byte
	copy(nonce[:], []byte("livenonce0002"))
	// Minimum lifetime the server grants for requested 0.
	req := rfcMapRequestLive(17, 5353, 5353, 2, nonce)
	resp := udpExchange(t, addr, req)
	if resp[3] != proto.PCPResultSuccess {
		t.Fatalf("short mapping refused: %d", resp[3])
	}
	t.Logf("post-exchange: mappings=%d", len(rt.Mappings()))
	time.Sleep(4 * time.Second)
	t.Logf("after 4s: mappings=%d", len(rt.Mappings()))

	client := nftlaunch.NewClient()
	client.SetSocketFn(func() string { return pcpLauncherSock })
	deadline := time.Now().Add(90 * time.Second)
	i := 0
	for {
		i++
		out, err := client.ListTableJSON("inet", "duckos_upnp")
		// Removed means "no rule left": either the table exists
		// without the mapping or it is entirely absent.
		if err == nil && !bytes.Contains(out, []byte("5353")) {
			return
		}
		if err != nil && strings.Contains(err.Error(), "No such file") {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("expired mapping still in the kernel: %s", out)
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// TestLivePCPUDPRefusals: unsupported option, PEER opcode and
// foreign-table attempts never touch the kernel.
func TestLivePCPUDPRefusals(t *testing.T) {
	skipUnlessLivePCP(t)
	cleanupUpnpTable(t)
	srv, _ := livePCPStack(t)
	addr := serveUDP(t, srv)

	// THIRD_PARTY option must be refused.
	thirdParty := append(rfcMapRequestLive(6, 4000, 4000, 600, [12]byte{9}), 0x01, 0x00)
	if resp := udpExchange(t, addr, thirdParty); resp[3] != proto.PCPResultUnsuppOption {
		t.Fatalf("THIRD_PARTY must yield unsupp-option, got %d", resp[3])
	}

	// PEER opcode must be refused as unsupported.
	peer := rfcMapRequestLive(6, 4000, 4000, 600, [12]byte{8})
	peer[1] = proto.PCPOpPeer
	if resp := udpExchange(t, addr, peer); resp[3] != proto.PCPResultUnsuppOpcode && resp[3] != proto.PCPResultMalformedRequest {
		t.Fatalf("PEER must be refused, got %d", resp[3])
	}

	// Ownership conflict: same external port, different client port
	// tuple already mapped above.
	first := rfcMapRequestLive(6, 4100, 4100, 600, [12]byte{7})
	if resp := udpExchange(t, addr, first); resp[3] != proto.PCPResultSuccess {
		t.Fatalf("first mapping refused: %d", resp[3])
	}
	second := rfcMapRequestLive(6, 4200, 4100, 600, [12]byte{6})
	if resp := udpExchange(t, addr, second); resp[3] == proto.PCPResultSuccess {
		t.Fatal("external port conflict must not succeed")
	}

	client := nftlaunch.NewClient()
	client.SetSocketFn(func() string { return pcpLauncherSock })
	out, err := client.ListTableJSON("inet", "duckos_upnp")
	if err != nil && !strings.Contains(err.Error(), "No such file") {
		t.Fatal(err)
	}
	if bytes.Contains(out, []byte("4200")) {
		t.Fatal("refused mapping must not reach the kernel")
	}
}
