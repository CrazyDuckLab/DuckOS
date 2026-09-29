//go:build linux

package network

import (
	"encoding/binary"
	"errors"
	"syscall"
	"testing"
)

// Bind synchronously before any operation. The kernel queues multicast events;
// overflow/truncation is a failed witness, never interpreted as zero events.
func watchNetworkEvents(t *testing.T) func() map[int]int {
	t.Helper()
	fd, err := syscall.Socket(syscall.AF_NETLINK, syscall.SOCK_RAW|syscall.SOCK_CLOEXEC|syscall.SOCK_NONBLOCK, syscall.NETLINK_ROUTE)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { syscall.Close(fd) })
	if err := syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_RCVBUF, 1<<20); err != nil {
		t.Fatal(err)
	}
	// LINK, IPv4/IPv6 IFADDR, IPv4/IPv6 ROUTE.
	if err := syscall.Bind(fd, &syscall.SockaddrNetlink{Family: syscall.AF_NETLINK, Groups: 1 | 0x10 | 0x40 | 0x100 | 0x400}); err != nil {
		t.Fatal(err)
	}
	return func() map[int]int {
		t.Helper()
		counts := map[int]int{}
		buf := make([]byte, 1<<20)
		for {
			n, _, flags, from, err := syscall.Recvmsg(fd, buf, nil, 0)
			if errors.Is(err, syscall.EAGAIN) {
				return counts
			}
			if err != nil || flags&syscall.MSG_TRUNC != 0 {
				t.Fatalf("netlink witness lost events: %v flags=%d", err, flags)
			}
			peer, ok := from.(*syscall.SockaddrNetlink)
			if !ok || peer.Pid != 0 {
				t.Fatal("non-kernel event source")
			}
			messages, err := syscall.ParseNetlinkMessage(buf[:n])
			if err != nil {
				t.Fatal(err)
			}
			for _, m := range messages {
				var index int
				switch m.Header.Type {
				case syscall.NLMSG_ERROR, syscall.NLMSG_OVERRUN:
					t.Fatal("netlink event delivery failed")
				case syscall.RTM_NEWLINK, syscall.RTM_DELLINK:
					if len(m.Data) < 16 {
						t.Fatal("short link event")
					}
					index = int(binary.NativeEndian.Uint32(m.Data[4:8]))
				case syscall.RTM_NEWADDR, syscall.RTM_DELADDR:
					if len(m.Data) < 8 {
						t.Fatal("short address event")
					}
					index = int(binary.NativeEndian.Uint32(m.Data[4:8]))
				case syscall.RTM_NEWROUTE, syscall.RTM_DELROUTE:
					attrs, err := syscall.ParseNetlinkRouteAttr(&m)
					if err != nil {
						t.Fatal(err)
					}
					for _, a := range attrs {
						if a.Attr.Type == syscall.RTA_MULTIPATH {
							t.Fatal("unexpected multipath event; witness cannot classify it")
						}
						if a.Attr.Type == syscall.RTA_OIF {
							if len(a.Value) != 4 {
								t.Fatal("invalid route output index")
							}
							index = int(binary.NativeEndian.Uint32(a.Value))
						}
					}
					if index == 0 {
						t.Fatal("route event without output link; witness cannot classify it")
					}
				}
				if index != 0 {
					counts[index]++
				}
			}
		}
	}
}
