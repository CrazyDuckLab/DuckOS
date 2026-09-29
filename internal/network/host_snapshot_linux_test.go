//go:build linux

package network

import (
	"context"
	"net"
	"net/netip"
	"testing"
)

func TestCaptureActualLinuxHostWithoutMutation(t *testing.T) {
	links, err := net.Interfaces()
	if err != nil {
		t.Fatal(err)
	}
	for _, link := range links {
		if !linkName.MatchString(link.Name) || !macAddress.MatchString(link.HardwareAddr.String()) || link.Flags&net.FlagLoopback != 0 || link.MTU < 1280 || link.MTU > 9000 {
			continue
		}
		addresses, err := link.Addrs()
		if err != nil {
			t.Fatal(err)
		}
		for _, address := range addresses {
			p, err := netip.ParsePrefix(address.String())
			if err != nil || !usable(p.Addr()) {
				continue
			}
			c := Config{Version: 1, Interfaces: []Interface{{ID: "observed-link", Name: link.Name, Role: "lan", Kind: "physical", MTU: link.MTU, Management: true, DHCPClient: "none", Addresses: []string{p.String()}}}}
			raw, err := CaptureHost(context.Background(), c, c)
			if err != nil {
				t.Fatal(err)
			}
			snapshot, err := DecodeHostSnapshot(raw, c, c)
			if err != nil {
				t.Fatal(err)
			}
			if len(snapshot.Links) != 1 || snapshot.Links[0].Index != link.Index || snapshot.Links[0].MTU != link.MTU {
				t.Fatal("captured link differs from actual Linux observation")
			}
			return
		}
	}
	t.Skip("no compatible addressed Ethernet interface on this Linux test host")
}
