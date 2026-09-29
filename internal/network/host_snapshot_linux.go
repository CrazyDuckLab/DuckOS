//go:build linux

package network

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"sort"
	"strings"
	"time"
)

// CaptureHost reads actual Linux link and forwarding state. Bindings originate
// in independently validated configuration; there is no caller-selected path.
func CaptureHost(ctx context.Context, before, next Config) (json.RawMessage, error) {
	names, err := PhysicalBindings(before, next)
	if err != nil {
		return nil, err
	}
	s := HostSnapshot{Version: 1, CapturedAt: time.Now().UTC(), Links: []HostLink{}}
	s.IPv6Forwarding, err = forwardingValue("/proc/sys/net/ipv6/conf/all/forwarding")
	if err != nil {
		return nil, err
	}
	for _, name := range names {
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		link, e := net.InterfaceByName(name)
		if e != nil {
			return nil, errors.New("bound physical interface unavailable")
		}
		l := HostLink{Name: name, Index: link.Index, MAC: link.HardwareAddr.String(), MTU: link.MTU, Up: link.Flags&net.FlagUp != 0, Addresses: []string{}}
		if l.IPv4Forwarding, e = forwardingValue("/proc/sys/net/ipv4/conf/" + name + "/forwarding"); e != nil {
			return nil, e
		}
		if l.IPv6Forwarding, e = forwardingValue("/proc/sys/net/ipv6/conf/" + name + "/forwarding"); e != nil {
			return nil, e
		}
		addresses, e := link.Addrs()
		if e != nil {
			return nil, errors.New("could not capture interface addresses")
		}
		for _, address := range addresses {
			l.Addresses = append(l.Addresses, address.String())
		}
		sort.Strings(l.Addresses)
		s.Links = append(s.Links, l)
	}
	raw, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}
	if _, err = DecodeHostSnapshot(raw, before, next); err != nil {
		return nil, err
	}
	return raw, nil
}
func forwardingValue(path string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, errors.New("forwarding state unavailable")
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, 8))
	if err != nil {
		return false, errors.New("forwarding state read failed")
	}
	switch strings.TrimSpace(string(b)) {
	case "0":
		return false, nil
	case "1":
		return true, nil
	default:
		return false, errors.New("unsupported forwarding state")
	}
}
