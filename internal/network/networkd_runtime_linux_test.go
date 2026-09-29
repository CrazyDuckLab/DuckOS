//go:build linux

package network

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// Entire daemon, bus, files and links are disposable. Never use the host bus or
// installed networkd. Parent must explicitly opt into this root-only witness.
func TestQualifiedNetworkdIsolatedRuntime(t *testing.T) {
	if os.Geteuid() != 0 || os.Getenv("DUCKOS_NETWORKD_WITNESS") != "1" {
		t.Skip("requires isolated root networkd witness")
	}
	if os.Getenv("DUCKOS_NETWORKD_CHILD") != "1" {
		cmd := exec.Command("/usr/bin/unshare", "--mount", "--net", "--pid", "--fork", "--mount-proc", os.Args[0], "-test.run=^TestQualifiedNetworkdIsolatedRuntime$", "-test.v", "-test.timeout=120s")
		cmd.Env = append(os.Environ(), "DUCKOS_NETWORKD_CHILD=1")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("isolated witness: %v\n%s", err, out)
		}
		t.Log(string(out))
		return
	}
	run := func(name string, args ...string) []byte {
		t.Helper()
		out, err := exec.Command(name, args...).CombinedOutput()
		if err != nil {
			t.Fatalf("%s %v: %v %s", name, args, err, out)
		}
		return out
	}
	run("/usr/bin/mount", "--make-rprivate", "/")
	// A private read-only sysfs tells networkd to initialize links without a
	// host udev daemon. Never remount the host's sysfs instance.
	run("/usr/bin/mount", "-t", "sysfs", "-o", "ro", "sysfs", "/sys")
	// Mask complete roots instead of creating any directory on the host.
	for _, path := range []string{"/run", "/etc/systemd", "/usr/local/lib", "/usr/lib/systemd/network"} {
		run("/usr/bin/mount", "-t", "tmpfs", "-o", "mode=0755", "tmpfs", path)
	}
	for _, path := range []string{"/run/dbus", "/run/systemd/system", "/run/systemd/network", "/etc/systemd/network", "/usr/local/lib/systemd/network"} {
		if err := os.MkdirAll(path, 0755); err != nil {
			t.Fatal(err)
		}
	}
	run("/usr/sbin/ip", "link", "set", "lo", "up")
	run("/usr/sbin/ip", "netns", "add", "witness-gateway")
	run("/usr/sbin/ip", "link", "add", "eth0", "type", "veth", "peer", "name", "gateway")
	run("/usr/sbin/ip", "link", "set", "gateway", "netns", "witness-gateway")
	run("/usr/sbin/ip", "netns", "exec", "witness-gateway", "ip", "addr", "add", "192.0.2.1/24", "dev", "gateway")
	run("/usr/sbin/ip", "netns", "exec", "witness-gateway", "ip", "link", "set", "gateway", "up")
	run("/usr/sbin/ip", "link", "add", "eth1", "type", "dummy")
	for _, name := range []string{"eth0", "eth1"} {
		run("/usr/sbin/ip", "link", "set", name, "up")
	}
	run("/usr/sbin/ip", "addr", "add", "192.0.2.10/24", "dev", "eth0")
	run("/usr/sbin/ip", "route", "add", "default", "via", "192.0.2.1", "dev", "eth0")
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()
	start := func(path string, args ...string) (*exec.Cmd, *bytes.Buffer) {
		t.Helper()
		cmd := exec.CommandContext(ctx, path, args...)
		log := new(bytes.Buffer)
		cmd.Stdout = log
		cmd.Stderr = log
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		return cmd, log
	}
	bus, buslog := start("/usr/bin/dbus-daemon", "--system", "--nopidfile", "--nofork", "--address=unix:path=/run/dbus/system_bus_socket")
	defer func() {
		bus.Process.Kill()
		bus.Wait()
		if t.Failed() {
			t.Log(buslog.String())
		}
	}()
	for i := 0; i < 100; i++ {
		if _, err := os.Stat("/run/dbus/system_bus_socket"); err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	c := Config{Version: 1, Interfaces: []Interface{{ID: "lab", Name: "eth1", Kind: "physical", Role: "lan", MTU: 1500, DHCPClient: "none", Management: true, Addresses: []string{"198.51.100.1/24"}}}}
	d, err := OpenArtifactDirectory("/etc/systemd/network")
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	p, err := Render(c)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Replace(nil, p.Networkd); err != nil {
		t.Fatal(err)
	}
	daemon, logs := start("/usr/lib/systemd/systemd-networkd")
	defer func() {
		daemon.Process.Kill()
		daemon.Wait()
		if t.Failed() {
			for _, line := range strings.Split(logs.String(), "\n") {
				if strings.Contains(line, "Bus ") || strings.Contains(line, "Failed") || strings.Contains(line, "bus-api") || strings.Contains(line, "denied") {
					t.Log(line)
				}
			}
		}
	}()
	wait := func(check func() error) {
		t.Helper()
		deadline := time.Now().Add(15 * time.Second)
		for {
			err := check()
			if err == nil {
				return
			}
			if time.Now().After(deadline) {
				t.Fatal(err)
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
	wait(func() error { return verifyStaticHost(ctx, c) })
	var inventory []NetworkdLink
	wait(func() error { var err error; inventory, err = ReadNetworkdInventory(ctx); return err })
	t.Logf("baseline inventory: %+v", inventory)
	linkRaw, err := CaptureHost(ctx, c, c)
	if err != nil {
		t.Fatal(err)
	}
	links, err := DecodeHostSnapshot(linkRaw, c, c)
	if err != nil {
		t.Fatal(err)
	}
	e := Enrollment{Version: 1, CreatedAt: time.Now(), FirewallTable: "inet duckos", RoutingTables: []int{10000}, PolicyRouting: true, NetworkdReload: true, Physical: []PhysicalBinding{{ID: "lab", Name: "eth1", MAC: links.Links[0].MAC}}}
	external, err := net.InterfaceByName("eth0")
	if err != nil {
		t.Fatal(err)
	}
	e.Physical = append(e.Physical, PhysicalBinding{ID: "external", Name: "eth0", MAC: external.HardwareAddr.String(), ExternallyManaged: true})
	h, err := NewNetworkdReloadHost(e)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	foreign := func() []byte {
		out := append(run("/usr/sbin/ip", "-j", "addr", "show", "dev", "eth0"), run("/usr/sbin/ip", "-j", "route", "show", "table", "all", "dev", "eth0")...)
		return append(out, run("/usr/sbin/ip", "-j", "-6", "route", "show", "table", "all", "dev", "eth0")...)
	}
	run("/usr/bin/ping", "-n", "-c", "3", "-i", "0.1", "-W", "1", "192.0.2.1")
	// Wait for actual DAD completion, not a guessed sleep duration.
	wait(func() error {
		out := run("/usr/sbin/ip", "-j", "addr", "show", "dev", "eth0")
		if strings.Contains(string(out), "tentative") || !strings.Contains(string(out), "fe80:") {
			return errors.New("external initial DAD pending")
		}
		return nil
	})
	original := foreign()
	eth0, err := net.InterfaceByName("eth0")
	if err != nil {
		t.Fatal(err)
	}
	eth1, err := net.InterfaceByName("eth1")
	if err != nil {
		t.Fatal(err)
	}
	events := watchNetworkEvents(t)
	defer func() {
		if t.Failed() {
			t.Logf("failure netlink counts: %v; original=%s current=%s", events(), original, foreign())
		}
	}()
	ping, pinglog := start("/usr/bin/ping", "-n", "-i", "0.05", "-W", "1", "192.0.2.1")
	defer func() { ping.Process.Kill(); ping.Wait() }()
	if err := CheckNetworkdOwnership(c, c, inventory); err == nil {
		t.Fatal("dummy must not pass physical enrollment admission")
	}
	const foreignProfile = "/etc/systemd/network/20-foreign.network"
	if err := os.WriteFile(foreignProfile, []byte("[Match]\nName=eth0\n[Network]\nDHCP=yes\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := h.PreflightScoped(ctx, c, c); err == nil {
		t.Fatal("foreign matching profile admitted")
	}
	if err := os.Remove(foreignProfile); err != nil {
		t.Fatal(err)
	}
	// Exercise the unmocked production refresh primitive beneath that separate
	// admission gate. Do not falsify inventory Kind to impersonate physical NICs.
	apply := func(before, next Config) {
		t.Helper()
		if err := h.PreflightScoped(ctx, before, next); err != nil {
			t.Fatal(err)
		}
		pb, _ := Render(before)
		pn, _ := Render(next)
		ids := renderedBodyChanged(pb.Networkd, pn.Networkd)
		if err := d.Replace(filterNetworkdByIDs(pb.Networkd, ids), filterNetworkdByIDs(pn.Networkd, ids)); err != nil {
			t.Fatal(err)
		}
		if err := h.refreshQualified(ctx, before, next, ids); err != nil {
			t.Fatal(err)
		}
		if err := d.Check(pn.Networkd, pn.Networkd); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(original, foreign()) {
			t.Fatal("eth0 changed")
		}
	}
	copyConfig := func(c Config) Config {
		raw, _ := json.Marshal(c)
		var out Config
		json.Unmarshal(raw, &out)
		return out
	}
	address := copyConfig(c)
	address.Interfaces[0].Addresses = []string{"198.51.100.2/24"}
	apply(c, address)
	if err := verifyStaticHost(ctx, address); err != nil {
		t.Fatal(err)
	}
	routed := copyConfig(address)
	routed.Routes = []Route{{ID: "private", Interface: "lab", Destination: "198.18.0.0/24", Table: 10000}}
	apply(address, routed)
	routeHost, err := NewPrivateRouteHost(e)
	if err != nil {
		t.Fatal(err)
	}
	expectedRoutes, err := PrivateRoutesForConfig(routed, e.RoutingTables)
	if err != nil {
		t.Fatal(err)
	}
	if err := routeHost.Verify(ctx, expectedRoutes); err != nil {
		t.Fatal(err)
	}
	if out := run("/usr/sbin/ip", "-j", "route", "show", "table", "10000"); !strings.Contains(string(out), "198.18.0.0/24") {
		t.Fatalf("route not applied: %s", out)
	}
	// Restart the actual daemon, then restore through a fresh host instance.
	daemon.Process.Kill()
	daemon.Wait()
	// Remove owned runtime state while the daemon is stopped: startup must
	// reconstruct it from durable candidate artifacts, not merely observe it.
	run("/usr/sbin/ip", "route", "del", "198.18.0.0/24", "table", "10000", "dev", "eth1")
	run("/usr/sbin/ip", "addr", "del", "198.51.100.2/24", "dev", "eth1")
	daemon, logs = start("/usr/lib/systemd/systemd-networkd")
	wait(func() error {
		if err := verifyStaticHost(ctx, routed); err != nil {
			return err
		}
		if err := routeHost.Verify(ctx, expectedRoutes); err != nil {
			return err
		}
		out, err := exec.Command("/usr/sbin/ip", "-j", "route", "show", "table", "10000").Output()
		if err != nil {
			return err
		}
		if !strings.Contains(string(out), "198.18.0.0/24") {
			return errors.New("startup route not converged")
		}
		return nil
	})
	h.Close()
	h, err = NewNetworkdReloadHost(e)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	pn, _ := Render(routed)
	pb, _ := Render(c)
	ids := renderedBodyChanged(pn.Networkd, pb.Networkd)
	if err := d.Restore(pb.Networkd, pn.Networkd); err != nil {
		t.Fatal(err)
	}
	if err := h.refreshQualified(ctx, c, c, ids); err != nil {
		t.Fatal(err)
	}
	if err := verifyStaticHost(ctx, c); err != nil {
		t.Fatal(err)
	}
	beforeRoutes, err := PrivateRoutesForConfig(c, e.RoutingTables)
	if err != nil {
		t.Fatal(err)
	}
	if err := routeHost.Verify(ctx, beforeRoutes); err != nil {
		t.Fatal(err)
	}
	if out := run("/usr/sbin/ip", "-j", "route", "show", "table", "all", "dev", "eth1"); strings.Contains(string(out), "198.18.0.0/24") {
		t.Fatalf("rollback left route: %s", out)
	}
	if !bytes.Equal(original, foreign()) {
		t.Fatal("restart/rollback changed eth0")
	}
	if err := d.Check(pb.Networkd, pb.Networkd); err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond)
	ping.Process.Signal(os.Interrupt)
	if err := ping.Wait(); err != nil {
		t.Fatalf("connectivity failed: %v %s", err, pinglog.String())
	}
	if !strings.Contains(pinglog.String(), ", 0% packet loss") {
		t.Fatalf("connectivity loss: %s", pinglog.String())
	}
	counts := events()
	if counts[eth0.Index] != 0 || counts[eth1.Index] == 0 {
		t.Fatalf("netlink events: eth0=%d eth1=%d", counts[eth0.Index], counts[eth1.Index])
	}
	t.Logf("real kernel witness: eth0 events=0, eth1 events=%d; %s", counts[eth1.Index], pinglog.String())
}
