//go:build linux

// udp_linux.go: the NAT-PMP/PCP datagram loop. NAT-PMP
// (version 0) and PCP (version 2) share UDP port 5351; the
// first byte of each datagram is the protocol version and
// demultiplexes cleanly. Read timeouts keep the loop
// cancelable; the socket is never rebound implicitly.
package frontend

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/upnp"
	"github.com/CrazyDuckLab/DuckOS/internal/upnp/proto"
)

// listenUDP binds one NAT-PMP/PCP socket on addr:port and
// serves it until ctx or the returned cancel fires.
func listenUDP(ctx context.Context, srv *Server, networkRef string, addr netip.Addr, port int, bind Bind) (*udpSocket, error) {
	laddr := net.UDPAddrFromAddrPort(netip.AddrPortFrom(addr, uint16(port)))
	conn, err := net.ListenUDP("udp4", laddr)
	if err != nil {
		return nil, err
	}
	cctx, cancel := context.WithCancel(ctx)
	s := &udpSocket{conn: conn, cancel: cancel, bind: bind}
	go s.serve(cctx, srv, networkRef, addr)
	return s, nil
}

// serve reads datagrams until the context ends. Buffers are
// stack-bounded to the largest protocol maximum (PCP: 1100).
func (s *udpSocket) serve(ctx context.Context, srv *Server, networkRef string, localAddr netip.Addr) {
	buf := make([]byte, proto.PCPMaxRequest)
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
			if isTimeout(err) {
				continue
			}
			return
		}
		if n == 0 || n > proto.PCPMaxRequest {
			continue
		}
		fromAddr, ok := netip.AddrFromSlice(from.IP.To4())
		if !ok {
			continue
		}
		// Refuse traffic that did not originate on the local
		// L3 segment shape: loopback and multicast sources are
		// never valid mapping clients.
		if fromAddr.IsLoopback() || fromAddr.IsMulticast() {
			continue
		}
		// Cross-subnet guard: the source must live on one of
		// the bind's on-link prefixes. A routed packet from a
		// foreign subnet is refused and journaled.
		if !s.bind.InPrefixes(fromAddr) {
			srv.Journal().Add(Rejection{
				At:         time.Now().UTC(),
				Protocol:   "udp",
				ClientIP:   fromAddr.String(),
				NetworkRef: networkRef,
				Reason:     "cross-subnet source refused",
			})
			continue
		}
		packet := buf[:n:n]
		s.dispatch(srv, networkRef, from, packet)
	}
}

// dispatch demultiplexes by version byte and writes the
// response back to the source address.
func (s *udpSocket) dispatch(srv *Server, networkRef string, from *net.UDPAddr, packet []byte) {
	fromAddr, ok := netip.AddrFromSlice(from.IP.To4())
	if !ok {
		return
	}
	var resp []byte
	switch packet[0] {
	case proto.NATPMPVersion:
		res := srv.HandleNATPMP(packet, fromAddr, upnp.Network{NetworkRef: networkRef})
		resp = res.Response
	case proto.PCPVersion:
		res := srv.HandlePCP(packet, fromAddr, upnp.Network{NetworkRef: networkRef})
		resp = res.Response
	default:
		return
	}
	if resp == nil {
		return
	}
	_, _ = s.conn.WriteToUDP(resp, from)
}

func isTimeout(err error) bool {
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
}
