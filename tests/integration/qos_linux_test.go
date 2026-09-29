//go:build linux

// qos_linux_test.go exercises the real Linux tc path
// in an isolated network namespace. The test creates
// a veth pair, brings both ends up, applies a small
// qos.Plan through the production LinuxRunner, and
// verifies the kernel's queueing state. The
// management interface eth0 is never touched.
//
// The test is the closest the qualification host
// gets to the production QoS/SQM path without a
// full installed DuckOS boot environment.
package integration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/qos"
)

// tcBinaryPath is the iproute2 binary the production
// LinuxRunner uses. The test reads the same path so
// the qualifying binary matches the one the Agent
// would use.
const tcBinaryPath = "/sbin/tc"

// netnsRunner is a qos.Executor that runs every
// command inside the named network namespace. The
// test constructs the runner with a fixed namespace
// and the production binary; the LinuxRunner
// delegates to the netnsRunner as if it were the
// real /sbin/tc.
//
// The runner holds a mutex because the underlying
// LinuxRunner serialises Apply and Rollback; the
// tests run sequentially per t.Run.
type netnsRunner struct {
	binary string
	ns     string
	mu     sync.Mutex
}

func (n *netnsRunner) Run(ctx context.Context, argv []string) error {
	if len(argv) == 0 {
		return errors.New("netns runner: empty argv")
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	args := append([]string{"netns", "exec", n.ns, n.binary}, argv[1:]...)
	cmd := exec.CommandContext(ctx, "ip", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("netns runner: ip %v: %w: %s", args, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// TestQoSLinuxApplyInNetns creates two namespaces
// connected by a veth pair, applies a small
// qos.Plan with a CAKE shaper on one link and
// inspects the kernel state. The test is the
// Linux qualification of the QoS foundation.
func TestQoSLinuxApplyInNetns(t *testing.T) {
	if _, err := exec.LookPath(tcBinaryPath); err != nil {
		t.Skipf("%s not available: %v", tcBinaryPath, err)
	}
	if _, err := exec.LookPath("ip"); err != nil {
		t.Skip("ip(8) is not available")
	}
	nsA := uniqueName("qosA")
	nsB := uniqueName("qosB")
	veth := uniqueName("vqos")
	addrA := "10.99.0.1/30"
	addrB := "10.99.0.2/30"
	mustRunNetns(t, nsA)
	mustRunNetns(t, nsB)
	t.Cleanup(func() {
		_ = exec.Command("ip", "netns", "del", nsA).Run()
		_ = exec.Command("ip", "netns", "del", nsB).Run()
	})
	mustRunRoot(t, "ip", "link", "add", veth, "type", "veth", "peer", "name", veth+"B")
	mustRunRoot(t, "ip", "link", "set", veth, "netns", nsA)
	mustRunRoot(t, "ip", "link", "set", veth+"B", "netns", nsB)
	mustRunIn(t, nsA, "ip", "addr", "add", addrA, "dev", veth)
	mustRunIn(t, nsB, "ip", "addr", "add", addrB, "dev", veth+"B")
	mustRunIn(t, nsA, "ip", "link", "set", veth, "up")
	mustRunIn(t, nsB, "ip", "link", "set", veth+"B", "up")
	mustRunIn(t, nsA, "ip", "link", "set", "lo", "up")
	mustRunIn(t, nsB, "ip", "link", "set", "lo", "up")

	// Build a small plan with a CAKE shaper on the
	// A side of the veth pair. The shaper caps the
	// rate at 10 mbit which is well below the
	// veth mtu ceiling.
	sh := qos.Shaper{ID: veth, Link: veth, Direction: qos.DirectionEgress, Algorithm: qos.AlgorithmCAKE, RateKbps: 10_000, BurstKbyte: 256, Priority: qos.PriorityVoice}
	tp, err := qos.Build([]qos.Shaper{sh}, nil, nil, time.Unix(1, 0))
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	plan, err := qos.Render(tp)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	// Run the production LinuxRunner against a
	// namespace-scoped Executor. The runner itself
	// is the production code; only the Executor is
	// overridden to route every call through
	// `ip netns exec`.
	runner, err := qos.OpenOwnedRunner(t.TempDir(), &netnsRunner{binary: tcBinaryPath, ns: nsA})
	if err != nil {
		t.Fatal(err)
	}
	defer runner.Close()
	if _, err := runner.Apply(context.Background(), plan); err != nil {
		t.Fatalf("apply in netns: %v", err)
	}
	got := runIn(t, nsA, tcBinaryPath, "-s", "qdisc", "show")
	if !strings.Contains(got, "qdisc cake") {
		t.Fatalf("expected kernel to show cake qdisc, got: %s", got)
	}
	if !strings.Contains(got, "dev "+veth) {
		t.Fatalf("expected kernel to show qdisc on dev %s, got: %s", veth, got)
	}
	if _, err := runner.Rollback(context.Background(), plan, qos.Plan{SchemaVersion: qos.SchemaVersion}); err != nil {
		t.Fatalf("rollback in netns: %v", err)
	}
	got2 := runIn(t, nsA, tcBinaryPath, "-s", "qdisc", "show")
	if strings.Contains(got2, "qdisc cake") {
		t.Fatalf("expected kernel to no longer show cake qdisc, got: %s", got2)
	}
}

// TestQoSLinuxFQCoDelOnEgress exercises the fq_codel
// path. fq_codel on egress is rate-shaped; the test
// verifies the kernel sees a fq_codel qdisc on the
// link root.
func TestQoSLinuxFQCoDelOnEgress(t *testing.T) {
	if _, err := exec.LookPath(tcBinaryPath); err != nil {
		t.Skipf("%s not available: %v", tcBinaryPath, err)
	}
	if _, err := exec.LookPath("ip"); err != nil {
		t.Skip("ip(8) is not available")
	}
	nsA := uniqueName("qosC")
	nsB := uniqueName("qosD")
	veth := uniqueName("vqosc")
	addrA := "10.99.1.1/30"
	addrB := "10.99.1.2/30"
	mustRunNetns(t, nsA)
	mustRunNetns(t, nsB)
	t.Cleanup(func() {
		_ = exec.Command("ip", "netns", "del", nsA).Run()
		_ = exec.Command("ip", "netns", "del", nsB).Run()
	})
	mustRunRoot(t, "ip", "link", "add", veth, "type", "veth", "peer", "name", veth+"B")
	mustRunRoot(t, "ip", "link", "set", veth, "netns", nsA)
	mustRunRoot(t, "ip", "link", "set", veth+"B", "netns", nsB)
	mustRunIn(t, nsA, "ip", "addr", "add", addrA, "dev", veth)
	mustRunIn(t, nsB, "ip", "addr", "add", addrB, "dev", veth+"B")
	mustRunIn(t, nsA, "ip", "link", "set", veth, "up")
	mustRunIn(t, nsB, "ip", "link", "set", veth+"B", "up")
	mustRunIn(t, nsA, "ip", "link", "set", "lo", "up")
	mustRunIn(t, nsB, "ip", "link", "set", "lo", "up")

	sh := qos.Shaper{ID: veth, Link: veth, Direction: qos.DirectionEgress, Algorithm: qos.AlgorithmFQCoDel, RateKbps: 5_000, BurstKbyte: 128}
	tp, err := qos.Build([]qos.Shaper{sh}, nil, nil, time.Unix(1, 0))
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	plan, err := qos.Render(tp)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	runner, err := qos.OpenOwnedRunner(t.TempDir(), &netnsRunner{binary: tcBinaryPath, ns: nsA})
	if err != nil {
		t.Fatal(err)
	}
	defer runner.Close()
	if _, err := runner.Apply(context.Background(), plan); err != nil {
		t.Fatalf("apply: %v", err)
	}
	got := runIn(t, nsA, tcBinaryPath, "-s", "qdisc", "show")
	if !strings.Contains(got, "qdisc fq_codel") {
		t.Fatalf("expected kernel to show fq_codel qdisc, got: %s", got)
	}
	if _, err := runner.Rollback(context.Background(), plan, qos.Plan{SchemaVersion: qos.SchemaVersion}); err != nil {
		t.Fatalf("rollback: %v", err)
	}
}

// mustRunNetns creates a network namespace. The
// helper is local to the qos test so it does not
// depend on the firewall test surface.
func mustRunNetns(t *testing.T, name string) {
	t.Helper()
	_ = exec.Command("ip", "netns", "del", name).Run()
	if out, err := exec.Command("ip", "netns", "add", name).CombinedOutput(); err != nil {
		t.Fatalf("ip netns add %s: %v (%s)", name, err, out)
	}
}

// mustRunRoot runs a command in the root
// namespace. The helper is the same shape as
// mustRunIn but bypasses `ip netns exec` so the
// empty namespace case does not generate an
// "Invalid netns name" error.
func mustRunRoot(t *testing.T, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %v: %v (%s)", name, args, err, out)
	}
}

// uniqueName returns a namespace name unique to
// the test. The kernel does not require globally
// unique names, but tests that run in parallel
// benefit from a counter.
var qosNSCounter int

func uniqueName(prefix string) string {
	qosNSCounter++
	return fmt.Sprintf("%s%d", prefix, qosNSCounter)
}

func (n *netnsRunner) Observe(ctx context.Context, link string) (json.RawMessage, error) {
	state := map[string][]map[string]any{}
	for _, kind := range []string{"qdisc", "class", "filter"} {
		out, err := exec.CommandContext(ctx, "/sbin/ip", "netns", "exec", n.ns, n.binary, "-j", kind, "show", "dev", link).Output()
		if err != nil {
			return nil, err
		}
		var rows []map[string]any
		if err := json.Unmarshal(out, &rows); err != nil {
			return nil, err
		}
		for _, row := range rows {
			delete(row, "refcnt")
			delete(row, "stats")
			delete(row, "stats2")
		}
		state[kind] = rows
	}
	return json.Marshal(state)
}
