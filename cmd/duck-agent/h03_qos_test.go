//go:build linux

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/CrazyDuckLab/DuckOS/internal/agent"
	"github.com/CrazyDuckLab/DuckOS/internal/qos"
)

func h03Plan() qos.Plan {
	return qos.Plan{SchemaVersion: qos.SchemaVersion, Steps: []qos.Step{{Kind: qos.StepQdisc, Link: "dummy0", Parent: "root", Handle: "1:", Args: []string{"fq_codel", "limit", "1000"}}}}
}

func TestH03ProductionOwnershipAndCrashRecovery(t *testing.T) {
	if !remediationIsolatedNetwork(t) {
		return
	}
	command := func(binary string, args ...string) []byte {
		t.Helper()
		out, err := exec.Command(binary, args...).CombinedOutput()
		if err != nil {
			t.Fatalf("isolated command: %v %s", err, out)
		}
		return out
	}
	command("ip", "link", "add", "dummy0", "type", "dummy")
	command("ip", "link", "set", "dummy0", "up")
	command("tc", "qdisc", "add", "dev", "dummy0", "root", "handle", "77:", "pfifo", "limit", "2345")
	dir := t.TempDir()
	host, err := enableQoSService(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if host != nil {
			host.Close()
		}
	}()
	ah := agent.NewHandler(nil)
	ah.EnableQoS(host)
	srv, token := remediationPublicAPI(t, ah, strings.Repeat("k", 64))
	call := func(path string, plan *qos.Plan, want int) []byte {
		t.Helper()
		var raw []byte
		if plan != nil {
			raw, _ = json.Marshal(plan)
		}
		method := "GET"
		if plan != nil || path == "/rollback" {
			method = "POST"
		}
		req, err := http.NewRequest(method, srv.URL+"/api/v1/qos"+path, bytes.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		out, _ := io.ReadAll(res.Body)
		if res.StatusCode != want {
			t.Fatalf("QoS %s: HTTP %d want %d: %s", path, res.StatusCode, want, out)
		}
		return out
	}
	p := h03Plan()
	call("/apply", &p, 503)
	oracle := string(command("tc", "-j", "qdisc", "show", "dev", "dummy0"))
	if !strings.Contains(oracle, "77:") || !strings.Contains(oracle, "2345") {
		t.Fatal("external pfifo was modified")
	}
	var status qos.Status
	_ = json.Unmarshal(call("", nil, 200), &status)
	if status.HasShapers != 0 {
		t.Fatal("failed runtime apply committed desired state")
	}
	// Remove only the fixture-owned external qdisc, then exercise real apply.
	command("tc", "qdisc", "del", "dev", "dummy0", "root")
	partial := h03Plan()
	partial.Steps = append(partial.Steps, qos.Step{Kind: qos.StepClass, Link: "dummy0", Parent: "1:", Handle: "1:1", Args: []string{"invalid-h03"}})
	call("/apply", &partial, 503)
	if out := string(command("tc", "-j", "qdisc", "show", "dev", "dummy0")); strings.Contains(out, "fq_codel") {
		t.Fatal("partial apply did not restore pre-state")
	}
	call("/apply", &p, 200)
	if out := string(command("tc", "-j", "qdisc", "show", "dev", "dummy0")); !strings.Contains(out, "fq_codel") {
		t.Fatal("public apply missing live qdisc")
	}
	if err := host.Close(); err != nil {
		t.Fatal(err)
	}
	host, err = enableQoSService(dir)
	if err != nil {
		t.Fatal(err)
	}
	ah.EnableQoS(host)
	call("/rollback", nil, 200)
	_ = json.Unmarshal(call("", nil, 200), &status)
	if status.HasShapers != 0 {
		t.Fatal("rollback retained synthetic active topology")
	}
	if out := string(command("tc", "-j", "qdisc", "show", "dev", "dummy0")); strings.Contains(out, "fq_codel") {
		t.Fatal("durable rollback did not remove owned qdisc")
	}
	if err := host.Close(); err != nil {
		t.Fatal(err)
	}
	host = nil
	// A separate process is killed after tc succeeded but before the after
	// witness is saved. Restart must preserve that uncertainty, not delete it.
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	child := exec.Command(binary, "-test.run=^TestH03CrashHelper$")
	child.Env = append(os.Environ(), "DUCKOS_H03_CRASH_DIR="+dir)
	if err := child.Run(); err == nil {
		t.Fatal("crash helper unexpectedly completed")
	}
	if _, err := os.Stat(filepath.Join(dir, "mutated")); err != nil {
		t.Fatal("child did not reach real runtime mutation")
	}
	host, err = enableQoSService(dir)
	if err != nil {
		t.Fatal(err)
	}
	ah.EnableQoS(host)
	if out := call("", nil, 200); !bytes.Contains(out, []byte("recovery_required")) {
		t.Fatal("restart hid uncertain runtime")
	}
	call("/apply", &p, 503)
	if out := string(command("tc", "-j", "qdisc", "show", "dev", "dummy0")); !strings.Contains(out, "fq_codel") {
		t.Fatal("uncertain qdisc was blindly removed")
	}
}

type h03CrashExecutor struct {
	*qos.ExecRunner
	marker string
}

func (e h03CrashExecutor) Run(ctx context.Context, args []string) error {
	if err := e.ExecRunner.Run(ctx, args); err != nil {
		return err
	}
	if len(args) > 2 && args[1] == "qdisc" && args[2] == "add" {
		if err := os.WriteFile(e.marker, []byte("mutated"), 0600); err != nil {
			return err
		}
		_ = syscall.Kill(os.Getpid(), syscall.SIGKILL)
	}
	return nil
}
func TestH03CrashHelper(t *testing.T) {
	dir := os.Getenv("DUCKOS_H03_CRASH_DIR")
	if dir == "" {
		t.Skip("subprocess helper")
	}
	self, err := os.Readlink("/proc/self/ns/net")
	if err != nil {
		t.Fatal(err)
	}
	host, err := os.Readlink("/proc/1/ns/net")
	if err != nil || self == host {
		t.Fatal("refusing host namespace")
	}
	r, err := qos.OpenOwnedRunner(dir, h03CrashExecutor{ExecRunner: qos.NewExecRunner("/sbin/tc"), marker: filepath.Join(dir, "mutated")})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	c, err := qos.NewController(r)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Apply(context.Background(), h03Plan()); err != nil {
		t.Fatal(err)
	}
	t.Fatal("crash witness not triggered")
}
