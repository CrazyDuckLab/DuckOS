//go:build linux

package network

import (
	"context"
	"net"
	"os/exec"
	"testing"
	"time"
)

func TestPrivateRouteNativeApplyRestore(t *testing.T) {
	requireIsolatedNetworkNamespace(t)
	call := func(args ...string) error {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "/usr/sbin/ip", args...)
		cmd.Env = []string{"PATH=/usr/sbin:/usr/bin", "LANG=C", "LC_ALL=C"}
		cmd.WaitDelay = time.Second
		return cmd.Run()
	}
	if err := call("link", "add", "duckroute0", "type", "dummy"); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := call("link", "delete", "duckroute0"); err != nil {
			t.Error("failed to remove test-owned link", err)
		}
	}()
	if err := call("link", "set", "duckroute0", "up"); err != nil {
		t.Fatal(err)
	}
	if err := call("address", "add", "192.0.2.2/24", "dev", "duckroute0"); err != nil {
		t.Fatal(err)
	}
	link, err := net.InterfaceByName("duckroute0")
	if err != nil {
		t.Fatal(err)
	}
	h, err := NewPrivateRouteHost(Enrollment{Version: 1, CreatedAt: time.Now(), FirewallTable: "inet duckos", RoutingTables: []int{10002}, Physical: []PhysicalBinding{{ID: "test-uplink", Name: link.Name, MAC: link.HardwareAddr.String()}}})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	empty, err := h.Capture(ctx)
	if err != nil || len(empty.Routes) != 0 {
		t.Fatal("test requires empty private table", err)
	}
	before, err := normalizeRouteSnapshot([]int{10002}, []KernelRoute{
		{Family: "ipv4", Table: 10002, Destination: "0.0.0.0/0", Interface: "duckroute0", Gateway: "192.0.2.1", Metric: 100, OnLink: true},
		{Family: "ipv6", Table: 10002, Destination: "2001:db8:10::/64", Interface: "duckroute0", Metric: 1024},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := h.Apply(ctx, empty, before); err != nil {
		t.Fatal(err)
	}
	candidate, _ := normalizeRouteSnapshot(before.Tables, before.Routes)
	for i := range candidate.Routes {
		if candidate.Routes[i].Family == "ipv4" {
			candidate.Routes[i].Gateway = "192.0.2.3"
		}
	}
	if err := h.Apply(ctx, before, candidate); err != nil {
		t.Fatal(err)
	}
	if err := h.Restore(ctx, before, candidate); err != nil {
		t.Fatal(err)
	}
	if err := h.Restore(ctx, empty, before); err != nil {
		t.Fatal(err)
	}
	// This checks route objects and restoration, not gateway reachability.
}
