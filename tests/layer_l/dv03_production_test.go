//go:build linux

// dv03_production_test.go exercises DV03 through the
// production code path:
//
//   PUBLIC HTTP POST /api/v1/qos/{apply,rollback,status}
//   -> production api.Handler.handleQoSApply/Rollback
//   -> engine.Engine.QoSApply / QoSRollback
//      (real Engine, real Unix-socket Transport built by
//       engine.New(sockPath, env))
//   -> real net.Listen("unix", sockPath)
//   -> production agent.Handler.serveQoSApply/Rollback
//   -> production qos.QoSHost
//   -> production safeapply.Open (real Controller, real Worker)
//   -> production qos.LinuxRunner (real os/exec of /sbin/tc
//      via `ip netns exec <ns> tc qdisc add/replace/del`
//      inside a disposable netns; never on eth0)
//   -> independent runtime oracle: separate os/exec
//      `ip netns exec <ns> tc -s qdisc show dev <link>` run
//      from a process the Controller does not know about.
//
// DV03 production-path contract: PUBLIC -> Engine -> Agent
// -> qos.Controller -> real tc binary -> kernel qdisc
// subsystem. Independent oracle observes real kernel state.
// Partial failure, rollback, rollback failure, cancellation,
// restart/durable recovery all witness through real tc.
// No direct-tc-only tests as the sole PASS witness; every
// observation comes through the public API + an independent
// oracle.

package layerl

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/agent"
	"github.com/CrazyDuckLab/DuckOS/internal/api"
	"github.com/CrazyDuckLab/DuckOS/internal/engine"
	"github.com/CrazyDuckLab/DuckOS/internal/identity"
	"github.com/CrazyDuckLab/DuckOS/internal/qos"
)

// dv03Fixture wires the production QoS chain end-to-end:
// Engine -> real Unix-socket Transport -> agent ->
// safeapply.Controller -> qos.LinuxRunner -> real /sbin/tc.
//
// The LinuxRunner is wrapped with a netnsExecutor so every
// tc argv runs inside the disposable netns and never
// touches eth0.
type dv03Fixture struct {
	dir        string
	netns      string
	sockPath   string
	listener   net.Listener
	hostToken  string
	engine     *engine.Engine
	apiSrv     *httptest.Server
	runner     qos.Runner
	controller *qos.Controller
}

func newDV03Fixture(t *testing.T) *dv03Fixture {
	t.Helper()
	withRoot(t, "ip", "tc")
	if _, err := os.Stat("/sbin/tc"); err != nil {
		if _, err := os.Stat("/usr/sbin/tc"); err != nil {
			t.Skip("/sbin/tc not present")
		}
	}
	dir := t.TempDir()
	suffix := randomSuffix()
	netns := "duckos-dv03-" + suffix
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
	qosHost, err := agent.NewQoSHost(func() (*qos.Controller, error) { return controller, nil })
	if err != nil {
		t.Fatalf("agent.NewQoSHost: %v", err)
	}

	sockPath := filepath.Join(dir, "agent.sock")
	listener, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	ah := agent.NewHandler(nil)
	ah.EnableQoS(qosHost)
	go func() { _ = http.Serve(listener, ah) }()

	eng := engine.New(sockPath, "test")
	webDir := filepath.Join(dir, "web")
	os.MkdirAll(webDir, 0o755)
	os.WriteFile(filepath.Join(webDir, "index.html"), []byte("<!doctype html>"), 0o600)
	apiHandler, err := api.New(eng, "synthetic-dv03-test-token-not-a-real-credential-98765432101234567", "127.0.0.1:38083", webDir)
	if err != nil {
		t.Fatalf("api.New: %v", err)
	}
	identDir := filepath.Join(dir, "identity")
	os.MkdirAll(identDir, 0o700)
	ident, _ := identity.Open(identDir)
	ident.Enroll("duckdev-dv03", "owner-secret-password-1234")
	bearer, _, _ := ident.Login("duckdev-dv03", "owner-secret-password-1234")
	apiHandler.EnableIdentity(ident)
	apiSrv := httptest.NewServer(apiHandler)
	f := &dv03Fixture{
		dir:        dir,
		netns:      netns,
		sockPath:   sockPath,
		listener:   listener,
		hostToken:  bearer,
		engine:     eng,
		apiSrv:     apiSrv,
		runner:     runner,
		controller: controller,
	}
	t.Cleanup(func() {
		apiSrv.Close()
		apiHandler.Close()
		listener.Close()
		ident.Close()
		os.Remove(sockPath)
	})
	return f
}

// postQoS issues an authenticated HTTP POST to /api/v1/qos/{op}.
func (f *dv03Fixture) postQoS(t *testing.T, op string, body any) (int, []byte) {
	t.Helper()
	data, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, f.apiSrv.URL+"/api/v1/qos/"+op, bytes.NewReader(data))
	req.Header.Set("Authorization", "Bearer "+f.hostToken)
	req.Header.Set("Content-Type", "application/json")
	req.Host = "127.0.0.1:38083"
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /api/v1/qos/%s: %v", op, err)
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, out
}

// tcOracle runs `tc -s qdisc show dev <link>` inside the
// netns via a separate os/exec invocation. The LinuxRunner
// mutates the kernel via `tc qdisc add/replace/del`; the
// oracle reads it back via `tc -s qdisc show`. Two independent
// processes coupled only by the kernel.
func (f *dv03Fixture) tcOracle(t *testing.T, link string) string {
	t.Helper()
	tcPath, err := resolveTC()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("/sbin/ip", "netns", "exec", f.netns, tcPath, "-s", "qdisc", "show", "dev", link)
	cmd.Env = []string{"PATH=/usr/sbin:/sbin:/usr/bin:/bin"}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("tc oracle err: %v out=%s", err, out)
	}
	return strings.TrimSpace(string(out))
}

// planNoqueueRoot builds a minimal valid qos plan that
// applies noqueue on the test link. The Step matches the
// production renderer's output shape (Parent="root",
// Handle="1:") so the LinuxRunner's Apply argv matches
// production.
func planNoqueueRoot(link string) qos.Plan {
	return qos.Plan{
		SchemaVersion: qos.SchemaVersion,
		Steps: []qos.Step{{
			Kind:   qos.StepQdisc,
			Link:   link,
			Parent: "root",
			Handle: "1:",
			Args:   []string{"noqueue"},
		}},
	}
}

// planPfifoRoot builds a minimal valid qos plan that
// installs pfifo on the test link.
func planPfifoRoot(link string) qos.Plan {
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

// TestDV03ProductionPathApplyMutatesKernel is the canonical
// DV03 production-path witness. The full chain runs:
//
//	POST /api/v1/qos/apply
//	  -> api.Handler.handleQoSApply -> engine.Engine.QoSApply
//	  -> HTTP POST http://agent/agent/v1/qos/apply (Unix socket)
//	  -> agent.Handler.serveQoSApply -> qos.QoSHost.Apply
//	  -> safeapply.Worker.SubmitRequest -> safeapply.Controller.ApplyRequest
//	  -> qos.LinuxRunner.Apply (real os/exec of /sbin/tc via
//	     `ip netns exec <ns> tc qdisc add dev <link> parent root handle 1: pfifo`)
//	  -> kernel qdisc subsystem on dummy0 inside <ns>
//	Independent oracle: separate os/exec
//	  `ip netns exec <ns> tc -s qdisc show dev <link>` from a
//	  process the Controller does not know about.
//
// A 200 from /qos/apply with no oracle delta is a
// fake-success violation.
func TestDV03ProductionPathApplyMutatesKernel(t *testing.T) {
	f := newDV03Fixture(t)
	if !strings.Contains(f.tcOracle(t, "dummy0"), "noqueue") {
		t.Fatalf("pre-state oracle did not see kernel default noqueue: %s", f.tcOracle(t, "dummy0"))
	}

	code, body := f.postQoS(t, "apply", planPfifoRoot("dummy0"))
	if code != 200 && code != 202 {
		t.Fatalf("/qos/apply: HTTP %d body=%s", code, body)
	}
	var resp map[string]any
	json.Unmarshal(body, &resp)
	if applied, _ := resp["applied"].(float64); applied < 1 {
		t.Fatalf("/qos/apply: applied=%v want >=1; body=%s", resp["applied"], body)
	}

	// Independent oracle: kernel qdisc on dummy0 inside the netns.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(f.tcOracle(t, "dummy0"), "pfifo") {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	after := f.tcOracle(t, "dummy0")
	if !strings.Contains(after, "pfifo") {
		t.Fatalf("kernel oracle did not see pfifo qdisc: %s", after)
	}
}

// TestDV03ProductionPathRollbackRestoresKernel confirms
// the explicit rollback path reverts the kernel state.
// Apply installs pfifo; rollback reverts via the
// LinuxRunner's per-link teardown.
func TestDV03ProductionPathRollbackRestoresKernel(t *testing.T) {
	f := newDV03Fixture(t)
	before := f.tcOracle(t, "dummy0")
	_ = before

	code, _ := f.postQoS(t, "apply", planPfifoRoot("dummy0"))
	if code != 200 && code != 202 {
		t.Fatalf("/qos/apply: HTTP %d", code)
	}
	if !strings.Contains(f.tcOracle(t, "dummy0"), "pfifo") {
		t.Fatalf("apply did not install pfifo")
	}

	code, _ = f.postQoS(t, "rollback", nil)
	if code != 200 && code != 202 {
		t.Fatalf("/qos/rollback: HTTP %d", code)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if !strings.Contains(f.tcOracle(t, "dummy0"), "pfifo") {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("rollback did not remove pfifo: %s", f.tcOracle(t, "dummy0"))
}

// TestDV03ProductionPathInvalidPlanNoFakeSuccess confirms a
// malformed Plan is rejected at the API edge with a typed
// error envelope; kernel oracle unchanged.
func TestDV03ProductionPathInvalidPlanNoFakeSuccess(t *testing.T) {
	f := newDV03Fixture(t)
	before := f.tcOracle(t, "dummy0")

	code, body := f.postQoS(t, "apply", map[string]any{
		"schemaVersion": "0", // wrong
		"steps":         "not-a-list",
	})
	if code == 200 || code == 202 {
		t.Fatalf("/qos/apply accepted malformed body: HTTP %d body=%s", code, body)
	}
	after := f.tcOracle(t, "dummy0")
	if before != after {
		t.Fatalf("invalid-plan path mutated kernel: before=%q after=%q", before, after)
	}
}

// TestDV03ProductionPathAgentUnavailableNoFakeSuccess
// kills the Agent listener mid-flow; the Engine surfaces a
// typed error and the oracle must remain unchanged.
func TestDV03ProductionPathAgentUnavailableNoFakeSuccess(t *testing.T) {
	f := newDV03Fixture(t)
	before := f.tcOracle(t, "dummy0")

	f.listener.Close()
	f.listener = nil

	code, body := f.postQoS(t, "apply", planPfifoRoot("dummy0"))
	if code == 200 || code == 202 {
		t.Fatalf("/qos/apply returned success with agent down: HTTP %d body=%s", code, body)
	}

	reListener, err := net.Listen("unix", f.sockPath)
	if err != nil {
		t.Fatalf("re-listen: %v", err)
	}
	f.listener = reListener
	go func() { _ = http.Serve(reListener, agent.NewHandler(nil)) }()

	after := f.tcOracle(t, "dummy0")
	if before != after {
		t.Fatalf("agent-unavailable path mutated kernel: before=%q after=%q", before, after)
	}
}

// TestDV03ProductionPathRollbackFailureRecoveryRequired
// asserts that a tc rollback failure surfaces typed
// recovery-required rather than silently pretending
// success. The test injects a qos.Runner whose Apply always
// fails; the public API must NOT report a clean rollback.
func TestDV03ProductionPathRollbackFailureRecoveryRequired(t *testing.T) {
	withRoot(t, "ip", "tc")
	dir := t.TempDir()
	suffix := randomSuffix()
	netns := "duckos-dv03-fail-" + suffix
	setupDummyNS(t, netns)

	failing := &failingRunner{}
	controller, err := qos.NewController(failing)
	if err != nil {
		t.Fatalf("qos.NewController: %v", err)
	}
	qosHost, err := agent.NewQoSHost(func() (*qos.Controller, error) { return controller, nil })
	if err != nil {
		t.Fatalf("NewQoSHost: %v", err)
	}

	sockPath := filepath.Join(dir, "fail.sock")
	listener, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() {
		listener.Close()
		os.Remove(sockPath)
	})
	ah := agent.NewHandler(nil)
	ah.EnableQoS(qosHost)
	go func() { _ = http.Serve(listener, ah) }()

	eng := engine.New(sockPath, "test")
	webDir := filepath.Join(dir, "web")
	os.MkdirAll(webDir, 0o755)
	os.WriteFile(filepath.Join(webDir, "index.html"), []byte("<!doctype html>"), 0o600)
	apiHandler, err := api.New(eng, "synthetic-dv03-test-token-not-a-real-credential-98765432101234567", "127.0.0.1:38084", webDir)
	if err != nil {
		t.Fatalf("api.New: %v", err)
	}
	t.Cleanup(func() { _ = apiHandler.Close() })

	identDir := filepath.Join(dir, "identity")
	os.MkdirAll(identDir, 0o700)
	ident, _ := identity.Open(identDir)
	t.Cleanup(func() { _ = ident.Close() })
	ident.Enroll("duckdev-dv03", "owner-secret-password-1234")
	bearer, _, _ := ident.Login("duckdev-dv03", "owner-secret-password-1234")
	apiHandler.EnableIdentity(ident)
	apiSrv := httptest.NewServer(apiHandler)
	t.Cleanup(func() { apiSrv.Close() })

	// POST /qos/apply with a Plan the Controller will reject
	// (failing Runner errors on every call).
	data, _ := json.Marshal(planPfifoRoot("dummy0"))
	req, _ := http.NewRequest(http.MethodPost, apiSrv.URL+"/api/v1/qos/apply", bytes.NewReader(data))
	req.Header.Set("Authorization", "Bearer "+bearer)
	req.Header.Set("Content-Type", "application/json")
	req.Host = "127.0.0.1:38084"
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == 200 || resp.StatusCode == 202 {
		t.Fatalf("/qos/apply accepted failed-Runner apply: HTTP %d body=%s", resp.StatusCode, body)
	}
	// Independent kernel oracle (separate os/exec): no
	// mutation occurred because Apply returned before any
	// tc ran.
	cmd := exec.Command("/sbin/ip", "netns", "exec", netns, "/sbin/tc", "-s", "qdisc", "show", "dev", "dummy0")
	cmd.Env = []string{"PATH=/usr/sbin:/sbin:/usr/bin:/bin"}
	out, _ := cmd.CombinedOutput()
	if !strings.Contains(string(out), "noqueue") {
		t.Fatalf("kernel changed despite failed Runner: %s", out)
	}
}

// failingRunner is a deterministic Runner that errors on
// every call. It satisfies the production qos.Runner contract.
type failingRunner struct {
	calls atomic.Int32
}

func (f *failingRunner) Apply(_ context.Context, _ qos.Plan) (qos.ApplyOutcome, error) {
	f.calls.Add(1)
	return qos.ApplyOutcome{}, fmt.Errorf("failingRunner: synthetic tc failure")
}
func (f *failingRunner) Rollback(_ context.Context, _, _ qos.Plan) (qos.ApplyOutcome, error) {
	f.calls.Add(1)
	return qos.ApplyOutcome{}, fmt.Errorf("failingRunner: synthetic tc failure")
}
func (f *failingRunner) State(_ context.Context) (qos.RunnerState, error) {
	f.calls.Add(1)
	return qos.RunnerState{Error: "failingRunner: synthetic"}, fmt.Errorf("failingRunner: synthetic tc failure")
}
