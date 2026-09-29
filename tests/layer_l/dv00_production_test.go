//go:build linux

// dv00_production_test.go exercises DV00 through the
// production code path:
//
//   PUBLIC HTTP request body
//   -> engine.New(sockPath, env).QoSApply / .QoSRollback
//      (the exact constructor cmd/duck-api uses)
//   -> real net.Listen("unix", sockPath)
//   -> production Agent Handler
//   -> *agent.QoSHost
//   -> *qos.Controller
//   -> *qos.OwnedRunner (real /sbin/tc via ip netns exec <ns>
//      inside a private netns so we never touch eth0)
//   -> independent tc oracle (separate os/exec call,
//      NOT the runner's own bookkeeping)
//
// DV00 production-path contract: PUBLIC -> ENGINE ->
// AGENT -> real tc daemon (kernel qdisc subsystem) ->
// independent tc -s qdisc show oracle -> real mutation,
// real rollback, real invalid-plan, real agent-unavailable,
// real nonexistent-link. No grep/source-only PASS; no
// kernel-route-only test; every observation goes through
// `ip netns exec <ns> /usr/sbin/tc -s qdisc show dev <link>`
// run from a separate os/exec invocation.

package layerl

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/agent"
	"github.com/CrazyDuckLab/DuckOS/internal/engine"
	"github.com/CrazyDuckLab/DuckOS/internal/qos"
)

// dv00Fixture builds the production DV00 path in-process
// using the same constructors cmd/duck-api and
// cmd/duck-agent use. The Agent listens on a real Unix
// socket; the Engine is constructed via engine.New(), the
// exact constructor duck-api uses.
//
// The LinuxRunner is wrapped with a netns Executor so the
// `tc` invocations run inside the disposable netns; the
// runner is otherwise production-shaped (real os/exec,
// real /sbin/tc, real binary, real argv, real stderr).
type dv00Fixture struct {
	dir        string
	netns      string
	sockPath   string
	listener   net.Listener
	host       *agent.QoSHost
	handler    *agent.Handler
	runner     *qos.OwnedRunner
	controller *qos.Controller
	stopped    bool
}

func newDV00Fixture(t *testing.T) *dv00Fixture {
	t.Helper()
	withRoot(t, "ip", "tc")
	if _, err := os.Stat("/sbin/tc"); err != nil {
		if _, err := os.Stat("/usr/sbin/tc"); err != nil {
			t.Skip("/sbin/tc not present")
		}
	}
	dir := t.TempDir()
	suffix := randomSuffix()
	netns := "duckos-dv00-" + suffix
	setupDummyNS(t, netns)

	runner, err := qos.OpenOwnedRunner(filepath.Join(dir, "qos"), &netnsExecutor{netns: netns})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { runner.Close() })
	controller, err := qos.NewController(runner)
	if err != nil {
		t.Fatalf("qos.NewController: %v", err)
	}
	host, err := agent.NewQoSHost(func() (*qos.Controller, error) {
		return controller, nil
	})
	if err != nil {
		t.Fatalf("agent.NewQoSHost: %v", err)
	}
	handler := agent.NewHandler(nil)
	handler.EnableQoS(host)

	sockPath := filepath.Join(dir, "agent.sock")
	listener, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("net.Listen(unix): %v", err)
	}
	go func() { _ = http.Serve(listener, handler) }()
	fixture := &dv00Fixture{
		dir:        dir,
		netns:      netns,
		sockPath:   sockPath,
		listener:   listener,
		host:       host,
		handler:    handler,
		runner:     runner,
		controller: controller,
	}
	t.Cleanup(func() {
		if !fixture.stopped {
			_ = listener.Close()
		}
		_ = os.Remove(sockPath)
	})
	return fixture
}

// stop closes the agent listener so subsequent Engine
// calls see "connection refused" and surface a 503-style
// error. The cleanup order matters: we close the listener
// before the test ends so a follow-up tc oracle cannot
// accidentally re-touch the kernel after the agent is
// gone.
func (f *dv00Fixture) stop(t *testing.T) {
	t.Helper()
	if f.stopped {
		return
	}
	f.stopped = true
	_ = f.listener.Close()
}

// engineFor returns a production Engine pointed at the
// agent Unix socket. Same constructor cmd/duck-api uses.
func (f *dv00Fixture) engineFor(t *testing.T) *engine.Engine {
	t.Helper()
	return engine.New(f.sockPath, "test")
}

// tcOracle runs /usr/sbin/tc -s qdisc show dev <link>
// inside the netns via a separate os/exec invocation.
// This is the independent runtime witness: the LinuxRunner
// does not know this process exists; only the kernel
// couples them.
func (f *dv00Fixture) tcOracle(t *testing.T, link string) string {
	t.Helper()
	tcPath := "/sbin/tc"
	if _, err := os.Stat(tcPath); err != nil {
		tcPath = "/usr/sbin/tc"
	}
	cmd := exec.Command("/sbin/ip", "netns", "exec", f.netns, tcPath, "-s", "qdisc", "show", "dev", link)
	cmd.Env = []string{"PATH=/usr/sbin:/sbin:/usr/bin:/bin", "LANG=C", "LC_ALL=C"}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("tc oracle err: %v out=%s", err, out)
	}
	return strings.TrimSpace(string(out))
}

// planPfifo builds a minimal valid plan that installs
// a `pfifo` root qdisc on the given link. The Step's
// Parent="root" and Handle="1:" match the production
// renderer's output (see internal/qos/render.go:renderShaperQdisc)
// so the LinuxRunner's rollback teardown (s.Parent == "root")
// fires correctly.
//
// pfifo is preferred over noqueue for the test because
// the kernel auto-installs `noqueue` on a dummy interface
// when it is brought up; an `add` of noqueue would fail
// with "RTNETLINK answers: File exists".
func planPfifo(link string) qos.Plan {
	return qos.Plan{
		SchemaVersion: qos.SchemaVersion,
		Steps: []qos.Step{{
			Kind:   qos.StepQdisc,
			Link:   link,
			Parent: "root",
			Handle: "1:",
			Args:   []string{"pfifo", "limit", "1000"},
		}},
	}
}

// TestDV00ProductionPathRealMutation is the canonical
// DV00 production-path witness. The full chain runs:
//
//	POST-style Engine.QoSApply(plan)
//	-> http over real Unix socket
//	-> Agent Handler.ServeQoSApply
//	-> QoSHost.Apply
//	-> Controller.Apply
//	-> LinuxRunner.Apply ("ip netns exec <ns> tc qdisc add dev dummy0 root pfifo")
//	-> kernel qdisc subsystem on dummy0 inside <ns>
//	-> independent tc oracle observes "pfifo" qdisc
//
// A successful API call with no observable kernel
// mutation is a fake-success failure.
func TestDV00ProductionPathRealMutation(t *testing.T) {
	f := newDV00Fixture(t)
	eng := f.engineFor(t)

	before := f.tcOracle(t, "dummy0")
	if strings.Contains(before, "pfifo") {
		t.Fatalf("dummy0 already has pfifo before apply: %s", before)
	}

	out, err := eng.QoSApply(context.Background(), planPfifo("dummy0"))
	if err != nil {
		t.Fatalf("Engine.QoSApply: %v", err)
	}
	if out.Applied != 1 {
		t.Fatalf("ApplyOutcome.Applied=%d, want 1 (out=%+v)", out.Applied, out)
	}

	after := f.tcOracle(t, "dummy0")
	if !strings.Contains(after, "pfifo") {
		t.Fatalf("kernel oracle did not see pfifo qdisc: %s", after)
	}
	if before == after {
		t.Fatalf("before/after oracle identical: %s", after)
	}
}

// TestDV00ProductionPathRollback confirms the rollback
// path actually reverts the kernel state. Apply installs
// pfifo; rollback must remove it so the oracle
// observes a fresh interface again.
func TestDV00ProductionPathRollback(t *testing.T) {
	f := newDV00Fixture(t)
	eng := f.engineFor(t)

	if _, err := eng.QoSApply(context.Background(), planPfifo("dummy0")); err != nil {
		t.Fatalf("apply: %v", err)
	}
	mid := f.tcOracle(t, "dummy0")
	if !strings.Contains(mid, "pfifo") {
		t.Fatalf("apply did not install pfifo: %s", mid)
	}

	if _, err := eng.QoSRollback(context.Background()); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	after := f.tcOracle(t, "dummy0")
	if strings.Contains(after, "pfifo") {
		t.Fatalf("rollback left pfifo in place: %s", after)
	}
}

// TestDV00NegativeInvalidPlan asserts the Engine refuses
// a plan whose schemaVersion does not match the model's.
// The Engine must surface a typed error and the kernel
// oracle must remain unchanged — no fake success.
func TestDV00NegativeInvalidPlan(t *testing.T) {
	f := newDV00Fixture(t)
	eng := f.engineFor(t)

	before := f.tcOracle(t, "dummy0")
	bad := qos.Plan{
		SchemaVersion: "0", // wrong version
		Steps: []qos.Step{{
			Kind: qos.StepQdisc,
			Link: "dummy0",
			Args: []string{"root", "pfifo"},
		}},
	}
	if _, err := eng.QoSApply(context.Background(), bad); err == nil {
		t.Fatalf("Engine accepted plan with wrong schemaVersion (fake success)")
	}
	after := f.tcOracle(t, "dummy0")
	if before != after {
		t.Fatalf("invalid plan mutated kernel state: before=%q after=%q", before, after)
	}
}

// TestDV00AgentUnavailableDoesNotFakeSuccess kills the
// agent listener and asserts the Engine surfaces a typed
// error (NOT a fake ApplyOutcome). The kernel oracle must
// remain unchanged — this is the canonical "agent down,
// runtime did its job" witness.
func TestDV00AgentUnavailableDoesNotFakeSuccess(t *testing.T) {
	f := newDV00Fixture(t)
	eng := f.engineFor(t)

	before := f.tcOracle(t, "dummy0")
	f.stop(t)

	out, err := eng.QoSApply(context.Background(), planPfifo("dummy0"))
	if err == nil {
		t.Fatalf("Engine returned success after agent stop (fake success): out=%+v", out)
	}
	if out.Applied != 0 {
		t.Fatalf("Engine.ApplyOutcome.Applied=%d after agent stop, want 0 (fake success)", out.Applied)
	}

	// Re-open listener briefly to run oracle without the
	// agent (oracle is independent of the agent).
	after := f.tcOracle(t, "dummy0")
	if before != after {
		t.Fatalf("agent-unavailable path mutated kernel state: before=%q after=%q", before, after)
	}
}

// TestDV00NonexistentLinkNoFakeSuccess asserts a plan that
// targets a non-existent interface fails the Runner and
// the kernel oracle observes no mutation. A nil error
// here would mean the Controller faked success.
func TestDV00NonexistentLinkNoFakeSuccess(t *testing.T) {
	f := newDV00Fixture(t)
	eng := f.engineFor(t)

	before := f.tcOracle(t, "dummy0")
	bad := planPfifo("nosuch0")
	out, err := eng.QoSApply(context.Background(), bad)
	if err == nil {
		t.Fatalf("Engine accepted plan against non-existent link (fake success): out=%+v", out)
	}
	if out.Applied != 0 {
		t.Fatalf("ApplyOutcome.Applied=%d on nonexistent link, want 0", out.Applied)
	}
	after := f.tcOracle(t, "dummy0")
	if before != after {
		t.Fatalf("nonexistent-link path mutated dummy0: before=%q after=%q", before, after)
	}
}

// netnsExecutor is the production-shaped Executor that
// runs every argv inside the disposable netns. The
// contract matches qos.Executor exactly: argv[0] is the
// logical binary name ("tc"), argv[1:] are its real
// arguments. The real /sbin/tc (or /usr/sbin/tc) is
// invoked via `ip netns exec <ns>` so the kernel
// mutation lands inside the netns and never on eth0.
type netnsExecutor struct {
	netns string
}

func (n *netnsExecutor) Run(ctx context.Context, argv []string) error {
	if len(argv) == 0 {
		return nil
	}
	tcPath, err := resolveTC()
	if err != nil {
		return err
	}
	args := append([]string{"netns", "exec", n.netns, tcPath}, argv[1:]...)
	cmd := exec.CommandContext(ctx, "/sbin/ip", args...)
	cmd.Env = []string{"PATH=/usr/sbin:/sbin:/usr/bin:/bin", "LANG=C", "LC_ALL=C"}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.Stdout = &bytes.Buffer{}
	if err := cmd.Run(); err != nil {
		return &exitErr{argv: argv, stderr: stderr.String(), err: err}
	}
	return nil
}

// exitErr preserves argv + stderr for the QoS Controller's
// error wrapping. The Controller surfaces this string back
// to the API; the oracle path keeps the original argv
// shape so a test can inspect what failed.
type exitErr struct {
	argv   []string
	stderr string
	err    error
}

func (e *exitErr) Error() string {
	return "netns-exec " + e.argv[0] + ": " + e.err.Error() + ": " + e.stderr
}
func (e *exitErr) Unwrap() error { return e.err }

func resolveTC() (string, error) {
	for _, p := range []string{"/sbin/tc", "/usr/sbin/tc"} {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", &os.PathError{Op: "stat", Path: "tc", Err: os.ErrNotExist}
}

// silenceUnusedTime keeps `time` imported even though the
// oracle is intentionally a one-shot subprocess.
var _ = time.Second

func (n *netnsExecutor) Observe(ctx context.Context, link string) (json.RawMessage, error) {
	state := map[string][]map[string]any{}
	for _, kind := range []string{"qdisc", "class", "filter"} {
		out, err := exec.CommandContext(ctx, "/sbin/ip", "netns", "exec", n.netns, "/sbin/tc", "-j", kind, "show", "dev", link).Output()
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
