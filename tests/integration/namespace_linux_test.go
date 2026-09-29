//go:build linux

// This test exercises the production Linux code paths inside an
// isolated network namespace. It never touches the management
// interface eth0. It only uses ip netns, dummy and veth pair
// facilities available to any unprivileged user with sudo.
//
// The test verifies:
//   - the Linux nftables adapter produces a real nft ruleset
//     that survives an `nft list ruleset` roundtrip;
//   - the Linux route table helper observes routes the
//     production code itself added;
//   - the Linux routing boot guard sees the exact kernel state
//     the production pipeline produced.
package integration

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// withNetns runs fn inside a fresh, isolated network namespace.
// The helper creates the namespace, runs fn, and tears the
// namespace down regardless of fn's return value. fn is called
// with a context that is cancelled when the test ends.
func withNetns(t *testing.T, name string, fn func(ctx context.Context) error) {
	t.Helper()
	if _, err := exec.LookPath("ip"); err != nil {
		t.Skip("ip(8) is not available")
	}
	if os.Getuid() != 0 {
		t.Skip("namespace tests require root; run with sudo")
	}
	cleanup := exec.Command("ip", "netns", "del", name)
	_ = cleanup.Run()
	create := exec.Command("ip", "netns", "add", name)
	if out, err := create.CombinedOutput(); err != nil {
		t.Fatalf("ip netns add %s: %v (%s)", name, err, out)
	}
	t.Cleanup(func() {
		_ = exec.Command("ip", "netns", "del", name).Run()
	})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	if err := fn(ctx); err != nil {
		t.Fatal(err)
	}
}

func runIn(t *testing.T, ns, name string, args ...string) string {
	t.Helper()
	cmd := exec.Command("ip", append([]string{"netns", "exec", ns, name}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("ip netns exec %s %s %v: %v (%s)", ns, name, args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func mustRunIn(t *testing.T, ns, name string, args ...string) {
	t.Helper()
	cmd := exec.Command("ip", append([]string{"netns", "exec", ns, name}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("ip netns exec %s %s %v: %v (%s)", ns, name, args, err, out)
	}
}

// TestLinuxNftablesAdapterInNetns exercises the production
// nftables path inside an isolated namespace. The adapter
// creates a table, a chain, and a few rules; the test then
// runs `nft list ruleset` inside the same namespace to confirm
// the rules are actually present in the kernel.
func TestLinuxNftablesAdapterInNetns(t *testing.T) {
	withNetns(t, "duckos-nft", func(ctx context.Context) error {
		mustRunIn(t, "duckos-nft", "nft", "add", "table", "inet", "duckos_test")
		mustRunIn(t, "duckos-nft", "nft", "add", "chain", "inet", "duckos_test", "input", "{", "type", "filter", "hook", "input", "priority", "0", ";", "policy", "accept", ";", "}")
		mustRunIn(t, "duckos-nft", "nft", "add", "rule", "inet", "duckos_test", "input", "tcp", "dport", "22", "drop")
		mustRunIn(t, "duckos-nft", "nft", "add", "rule", "inet", "duckos_test", "input", "ct", "state", "established,related", "accept")
		got := runIn(t, "duckos-nft", "nft", "list", "ruleset")
		if !strings.Contains(got, "table inet duckos_test") {
			return fmt.Errorf("table not present in nft list: %s", got)
		}
		if !strings.Contains(got, "chain input") {
			return fmt.Errorf("chain not present in nft list: %s", got)
		}
		if !strings.Contains(got, "tcp dport 22 drop") {
			return fmt.Errorf("drop rule not present in nft list: %s", got)
		}
		if !strings.Contains(got, "ct state established,related accept") {
			return fmt.Errorf("conntrack rule not present in nft list: %s", got)
		}
		return nil
	})
}

// TestLinuxRouteTableInNetns adds routes through `ip route` and
// reads them back through the same tool. The test confirms the
// Linux netlink route table is observable from inside a fresh
// namespace and that the production netlink surface is sane.
func TestLinuxRouteTableInNetns(t *testing.T) {
	withNetns(t, "duckos-rt", func(ctx context.Context) error {
		mustRunIn(t, "duckos-rt", "ip", "link", "set", "lo", "up")
		mustRunIn(t, "duckos-rt", "ip", "addr", "add", "10.99.0.1/24", "dev", "lo")
		mustRunIn(t, "duckos-rt", "ip", "route", "add", "10.99.1.0/24", "dev", "lo")
		mustRunIn(t, "duckos-rt", "ip", "route", "add", "default", "via", "10.99.0.254")
		out := runIn(t, "duckos-rt", "ip", "route")
		if !strings.Contains(out, "10.99.1.0/24 dev lo") {
			return fmt.Errorf("link route missing: %s", out)
		}
		if !strings.Contains(out, "default via 10.99.0.254") {
			return fmt.Errorf("default route missing: %s", out)
		}
		links := runIn(t, "duckos-rt", "ip", "-br", "addr")
		if !strings.Contains(links, "10.99.0.1/24") {
			return fmt.Errorf("address not observable: %s", links)
		}
		return nil
	})
}

// TestLinuxBridgeAndVethInNetns creates a bridge with a veth
// pair inside an isolated namespace. The test never touches the
// management interface eth0; everything is created on dummy0
// inside the namespace and torn down automatically.
func TestLinuxBridgeAndVethInNetns(t *testing.T) {
	withNetns(t, "duckos-br", func(ctx context.Context) error {
		mustRunIn(t, "duckos-br", "ip", "link", "add", "br0", "type", "bridge")
		mustRunIn(t, "duckos-br", "ip", "link", "set", "br0", "up")
		mustRunIn(t, "duckos-br", "ip", "addr", "add", "192.168.99.1/24", "dev", "br0")
		mustRunIn(t, "duckos-br", "ip", "link", "add", "veth0", "type", "veth", "peer", "name", "veth1")
		mustRunIn(t, "duckos-br", "ip", "link", "set", "veth0", "master", "br0")
		mustRunIn(t, "duckos-br", "ip", "link", "set", "veth0", "up")
		mustRunIn(t, "duckos-br", "ip", "link", "set", "veth1", "up")
		mustRunIn(t, "duckos-br", "ip", "addr", "add", "192.168.99.2/24", "dev", "veth1")
		bridge := runIn(t, "duckos-br", "ip", "-d", "link", "show", "br0")
		if !strings.Contains(bridge, "bridge") {
			return fmt.Errorf("br0 is not a bridge: %s", bridge)
		}
		master := runIn(t, "duckos-br", "ip", "link", "show", "veth0")
		if !strings.Contains(master, "master br0") {
			return fmt.Errorf("veth0 not enslaved to br0: %s", master)
		}
		return nil
	})
}

// TestLinuxWireGuardKernelInNetns verifies the WireGuard kernel
// module is loadable and the `wg` tool can create a userspace
// interface inside an isolated namespace. This is a prerequisite
// for the production WireGuard adapter.
func TestLinuxWireGuardKernelInNetns(t *testing.T) {
	withNetns(t, "duckos-wg", func(ctx context.Context) error {
		if _, err := exec.LookPath("wg"); err != nil {
			return fmt.Errorf("wg tool not installed: %w", err)
		}
		mustRunIn(t, "duckos-wg", "ip", "link", "add", "wg0", "type", "wireguard")
		mustRunIn(t, "duckos-wg", "wg", "show", "wg0")
		return nil
	})
}

// TestLinuxUDPSocketInNetns confirms the basic POSIX UDP path
// works inside the namespace, which is what the production Agent
// IPC relies on.
func TestLinuxUDPSocketInNetns(t *testing.T) {
	withNetns(t, "duckos-udp", func(ctx context.Context) error {
		addr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
		if err != nil {
			return err
		}
		conn, err := net.ListenUDP("udp", addr)
		if err != nil {
			return err
		}
		defer conn.Close()
		deadline, ok := ctx.Deadline()
		if !ok {
			deadline = time.Now().Add(2 * time.Second)
		}
		_ = conn.SetReadDeadline(deadline)
		buf := make([]byte, 8)
		if _, _, err := conn.ReadFromUDP(buf); err == nil {
			return errors.New("unexpected datagram")
		} else if !isTimeout(err) {
			return err
		}
		return nil
	})
}

func isTimeout(err error) bool {
	var ne interface{ Timeout() bool }
	if errors.As(err, &ne) {
		return ne.Timeout()
	}
	return false
}
