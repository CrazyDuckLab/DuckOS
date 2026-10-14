//go:build linux

// ssdp_linux.go: the SSDP M-SEARCH responder. Joins the
// 239.255.255.250 multicast group on the trusted interface
// only and answers search requests with the IGD description
// location. No NOTIFY flooding: DuckOS answers queries,
// it never shouts. Responses are delayed by a bounded
// pseudo-random fraction of MX so clients do not synchronize.
package frontend

import (
	"context"
	"errors"
	"math/rand"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/upnp/proto"
)

// ssdpResponder owns the UDP/1900 socket of one trusted
// address.
type ssdpResponder struct {
	conn   *net.UDPConn
	cancel context.CancelFunc
}

func newSSDPResponder(ctx context.Context, srv *Server, networkRef string, addr netip.Addr, iface string) (*ssdpResponder, error) {
	lc := net.ListenConfig{}
	// Join the multicast group scoped to this interface so
	// only M-SEARCHes on the trusted LAN are received. The
	// control hook is Linux-specific and never touches other
	// interfaces.
	ifi, err := net.InterfaceByName(iface)
	if err == nil {
		lc.Control = func(network, address string, c syscall.RawConn) error {
			var sockErr error
			c.Control(func(fd uintptr) {
				sockErr = syscall.SetsockoptIPMreqn(int(fd), syscall.IPPROTO_IP, syscall.IP_ADD_MEMBERSHIP, &syscall.IPMreqn{
					Multiaddr: [4]byte{239, 255, 255, 250},
					Ifindex:   int32(ifi.Index),
				})
			})
			return sockErr
		}
	}
	conn, err := lc.ListenPacket(ctx, "udp4", net.JoinHostPort(addr.String(), strconv.Itoa(proto.SSDPPort)))
	if err != nil {
		return nil, err
	}
	uc, ok := conn.(*net.UDPConn)
	if !ok {
		_ = conn.Close()
		return nil, errors.New("ssdp: unexpected packet conn type")
	}
	cctx, cancel := context.WithCancel(ctx)
	s := &ssdpResponder{conn: uc, cancel: cancel}
	go s.serve(cctx, srv, networkRef, addr)
	return s, nil
}

func (s *ssdpResponder) close() {
	s.cancel()
	_ = s.conn.Close()
}

// serve answers unicast and multicast M-SEARCHes until the
// context ends.
func (s *ssdpResponder) serve(ctx context.Context, srv *Server, networkRef string, addr netip.Addr) {
	buf := make([]byte, proto.SSDPMaxRequest)
	for {
		if ctx.Err() != nil {
			return
		}
		_ = s.conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		n, from, err := s.conn.ReadFromUDP(buf)
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				continue
			}
			return
		}
		if n == 0 || n > proto.SSDPMaxRequest {
			continue
		}
		fromAddr, ok := netip.AddrFromSlice(from.IP.To4())
		if !ok || fromAddr.IsMulticast() {
			continue
		}
		m, err := proto.ParseMSearch(buf[:n:n])
		if err != nil || !m.RespondsTo() {
			continue
		}
		if delay := ssdpDelay(m.MX); delay > 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(delay):
			}
		}
		location := "http://" + addr.String() + ":" + strconv.Itoa(IGDControlPort) + "/igd/description.xml"
		resp := buildMSearchResponse(m.ST, location)
		_, _ = s.conn.WriteToUDP(resp, from)
		_ = srv
		_ = networkRef
	}
}

// ssdpDelay clamps the response delay to [0, MX] seconds and
// hard-caps at 2s so the responder stays responsive.
func ssdpDelay(mx int) time.Duration {
	if mx < 0 {
		mx = 0
	}
	if mx > 2 {
		mx = 2
	}
	if mx == 0 {
		return 0
	}
	return time.Duration(rand.Intn(mx*1000)) * time.Millisecond
}

// buildMSearchResponse renders one SSDP search response.
func buildMSearchResponse(st, location string) []byte {
	var b strings.Builder
	b.WriteString("HTTP/1.1 200 OK\r\n")
	b.WriteString("CACHE-CONTROL: max-age=120\r\n")
	b.WriteString("EXT:\r\n")
	b.WriteString("LOCATION: " + location + "\r\n")
	b.WriteString("SERVER: DuckOS/1.0 UPnP/1.0 DuckOS-beta.3\r\n")
	b.WriteString("ST: " + st + "\r\n")
	b.WriteString("USN: uuid:duckos-upnp-igd::" + st + "\r\n")
	b.WriteString("\r\n")
	return []byte(b.String())
}
