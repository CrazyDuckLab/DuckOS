//go:build linux

package network

import (
	"context"
	"net"
	"net/netip"
	"os/exec"
	"testing"
	"time"
)

func TestRoutedKernelNativeApplyRecover(t *testing.T) {
	requireIsolatedNetworkNamespace(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	call := func(args ...string) error {
		commandCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		cmd := exec.CommandContext(commandCtx, "/usr/sbin/ip", args...)
		cmd.Env = []string{"PATH=/usr/sbin:/usr/bin", "LANG=C", "LC_ALL=C"}
		cmd.WaitDelay = time.Second
		return cmd.Run()
	}
	c, _ := staticTrafficFixture()
	c.Interfaces[0].Name, c.Interfaces[1].Name = "duckrk0", "duckrk1"
	c.Interfaces[0].Addresses = append(c.Interfaces[0].Addresses, "2001:db8:2::2/64")
	e := Enrollment{Version: 1, CreatedAt: time.Now(), FirewallTable: "inet duckos", RoutingTables: []int{10000, 10001}, PolicyRouting: true}
	for _, i := range c.Interfaces {
		if err := call("link", "add", i.Name, "type", "dummy"); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := exec.CommandContext(cleanupCtx, "/usr/sbin/ip", "link", "delete", i.Name).Run(); err != nil {
				t.Error("test link cleanup failed", err)
			}
		})
		if err := call("link", "set", i.Name, "up"); err != nil {
			t.Fatal(err)
		}
		for _, address := range i.Addresses {
			args := []string{"address", "add", address, "dev", i.Name}
			if netip.MustParsePrefix(address).Addr().Is6() {
				args = append(args, "nodad")
			}
			if err := call(args...); err != nil {
				t.Fatal(err)
			}
		}
		link, err := net.InterfaceByName(i.Name)
		if err != nil {
			t.Fatal(err)
		}
		e.Physical = append(e.Physical, PhysicalBinding{ID: i.ID, Name: i.Name, MAC: link.HardwareAddr.String()})
	}
	c.Routes = append(c.Routes,
		Route{ID: "private-v4", Interface: "uplink", Destination: "0.0.0.0/0", Gateway: "192.0.2.1", Table: 10000},
		Route{ID: "private-v6", Interface: "uplink", Destination: "::/0", Gateway: "2001:db8:2::1", Table: 10000},
		Route{ID: "direct-v4", Interface: "inside", Destination: "203.0.113.0/24", Table: 10001},
		Route{ID: "direct-v6", Interface: "inside", Destination: "2001:db8:3::/64", Table: 10001})
	c.PolicyRules = []PolicyRule{{ID: "private-v4-policy", Family: "ipv4", Priority: 20000, Table: 10000, From: "198.51.100.0/24"}}
	k, err := NewRoutedKernel(e)
	if err != nil {
		t.Fatal(err)
	}
	before, err := k.firewall.Capture(ctx)
	if err != nil || len(before.Objects) != 0 {
		t.Fatal("test requires absent owned table", err)
	}
	raw, err := k.Capture(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		if _, err := k.Restore(cleanupCtx, c, raw); err != nil {
			t.Error("test routing cleanup failed", err)
		}
		if err := k.firewall.Restore(cleanupCtx, before); err != nil {
			t.Error("test firewall cleanup failed", err)
		}
	})
	guard, err := k.Apply(ctx, c, raw)
	if err != nil || verifyForwardGuard(guard) != nil {
		t.Fatal("native coordinated apply", err)
	}
	// New instance models loss of all admission/capture memory after a crash.
	recovered, err := NewRoutedKernel(e)
	if err != nil {
		t.Fatal(err)
	}
	guard, err = recovered.Restore(ctx, c, raw)
	if err != nil || verifyForwardGuard(guard) != nil {
		t.Fatal("native coordinated recovery", err)
	}
}
