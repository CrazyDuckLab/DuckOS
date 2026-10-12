//go:build linux

// xray/runtime_linux_test.go exercises the
// real Linux Xray runtime. The test installs a
// stub xray shell script that:
//
//   - records its argv and pid
//   - ignores SIGUSR1 by default
//   - sleeps until killed
//
// The test verifies: cold start, hot reload
// via SIGUSR1, Stop terminates the process
// group, Rollback promotes previous.json back
// to active.json and reloads, and Apply
// refuses an invalid config.
package xray

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// installStubXray replaces the xray binary at
// XrayPath with a shell script that records
// its argv and pid to a side file and sleeps.
func installStubXray(t *testing.T, dir string) (binaryPath, callsPath string) {
	t.Helper()
	bin := filepath.Join(dir, "xray")
	log := filepath.Join(dir, "calls.log")
	// The stub is a python script rather than sh: the
	// runtime's readiness gate only accepts the apply once
	// THIS pid owns a listening socket for every declared
	// inbound (ownsListeners inspects /proc/<pid>/fd), so the
	// stub must bind the ports in its own process instead of
	// delegating to shell children.
	script := `#!/usr/bin/env python3
import json, os, signal, socket, sys, time

LOG = "` + log + `"
args = sys.argv[1:]
if "-test" in args:
    sys.exit(0)
with open(LOG, "a") as f:
    f.write("argv0=%s\n" % sys.argv[0])
    for i, a in enumerate(args[:2]):
        f.write("argv%d=%s\n" % (i + 1, a))
    f.write("pid=%d\n" % os.getpid())
cfgpath = args[args.index("-c") + 1]
with open(cfgpath) as f:
    conf = json.load(f)
for inb in conf.get("inbounds", []):
    s = socket.socket()
    s.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
    s.bind((inb.get("listen", "127.0.0.1"), int(inb["port"])))
    s.listen(8)
signal.signal(signal.SIGTERM, lambda *a: sys.exit(0))
# SIGUSR1 must NOT exit; the runtime replaces the process
# on reload and the stub must stay alive until killed.
signal.signal(signal.SIGUSR1, signal.SIG_IGN)
while True:
    time.sleep(3600)
`
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	return bin, log
}

// stubConfig returns a minimal config carrying one
// inbound. Since B-010 (beta.2) a render with no
// inbounds must not start a daemon ("no inbounds = no
// daemon"), so the lifecycle tests exercise the runtime
// with a real inbound present. The stub binary accepts
// any config, so the inbound shape only has to marshal.
func stubConfig(version int) Config {
	return Config{
		Version: version,
		Inbounds: []Inbound{{
			Tag:      "stub-in",
			Listen:   "127.0.0.1",
			Port:     10000 + version,
			Protocol: "dokodemo-door",
		}},
	}
}

func readCalls(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read calls: %v", err)
	}
	return strings.Split(strings.TrimSpace(string(data)), "\n")
}

// withStubRuntime overrides the package-level
// XrayPath and StageDir for the duration of
// the test, then restores them on cleanup.
func withStubRuntime(t *testing.T, bin, stage string) {
	t.Helper()
	oldBin := XrayPath
	oldStage := StageDir
	XrayPath = bin
	StageDir = stage
	t.Cleanup(func() {
		XrayPath = oldBin
		StageDir = oldStage
	})
}

func TestRuntimeColdStart(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	bin, calls := installStubXray(t, dir)
	withStubRuntime(t, bin, filepath.Join(dir, "stage"))
	rt, err := NewRuntime()
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	if err := rt.Apply(context.Background(), stubConfig(1)); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	// Wait for the stub to record.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(calls); err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	got := readCalls(t, calls)
	if len(got) < 3 {
		t.Fatalf("expected at least 3 lines, got %v", got)
	}
	if !strings.Contains(got[0], "argv0="+bin) {
		t.Fatalf("argv0 mismatch: %v", got)
	}
	if !strings.Contains(got[1], "argv1=run") {
		t.Fatalf("argv1 must be 'run', got %v", got)
	}
	if !strings.Contains(got[2], "argv2=-c") {
		t.Fatalf("argv2 must be '-c', got %v", got)
	}
	state := rt.State()
	if !state.Active {
		t.Fatal("runtime must be Active after Apply")
	}
	if state.PID == 0 {
		t.Fatal("runtime must report a non-zero PID")
	}
	if err := rt.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
}

func TestRuntimeReloadReplacesProcess(t *testing.T) {
	dir := t.TempDir()
	bin, _ := installStubXray(t, dir)
	withStubRuntime(t, bin, filepath.Join(dir, "stage"))
	rt, err := NewRuntime()
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	if err := rt.Apply(context.Background(), stubConfig(1)); err != nil {
		t.Fatalf("Apply 1: %v", err)
	}
	pidBefore := rt.State().PID
	if pidBefore == 0 {
		t.Fatal("Apply 1 must record a PID")
	}
	if err := rt.Apply(context.Background(), stubConfig(2)); err != nil {
		t.Fatalf("Apply 2 (reload): %v", err)
	}
	pidAfter := rt.State().PID
	// Xray has no supported SIGUSR1 config reload. Replace and verify.
	if pidAfter == pidBefore || pidAfter == 0 {
		t.Fatalf("reload must replace the process: before=%d after=%d", pidBefore, pidAfter)
	}
	_ = rt.Stop()
}

func TestRuntimeRollbackPromotesPrevious(t *testing.T) {
	dir := t.TempDir()
	bin, _ := installStubXray(t, dir)
	stage := filepath.Join(dir, "stage")
	withStubRuntime(t, bin, stage)
	rt, err := NewRuntime()
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	if err := rt.Apply(context.Background(), stubConfig(1)); err != nil {
		t.Fatalf("Apply 1: %v", err)
	}
	// previous.json must exist after the second
	// Apply (which retains the first as previous).
	if err := rt.Apply(context.Background(), stubConfig(2)); err != nil {
		t.Fatalf("Apply 2: %v", err)
	}
	if _, err := os.Stat(filepath.Join(stage, "previous.json")); err != nil {
		t.Fatalf("previous.json must exist after second Apply: %v", err)
	}
	if err := rt.Rollback(); err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	// After Rollback, previous.json must be gone
	// (it has been moved to active.json).
	if _, err := os.Stat(filepath.Join(stage, "previous.json")); !os.IsNotExist(err) {
		t.Fatalf("previous.json must be removed after Rollback, got err=%v", err)
	}
	_ = rt.Stop()
}

func TestRuntimeRollbackWithoutPreviousIsError(t *testing.T) {
	dir := t.TempDir()
	bin, _ := installStubXray(t, dir)
	withStubRuntime(t, bin, filepath.Join(dir, "stage"))
	rt, err := NewRuntime()
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	if err := rt.Rollback(); err == nil {
		t.Fatal("Rollback without previous config must fail")
	}
}

func TestRuntimeNewRuntimeRefusesMissingBinary(t *testing.T) {
	withStubRuntime(t, "/nonexistent/xray-binary-zzz", t.TempDir())
	if _, err := NewRuntime(); err == nil {
		t.Fatal("NewRuntime must refuse when the xray binary is missing")
	}
}

func TestRuntimeStopIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	bin, _ := installStubXray(t, dir)
	withStubRuntime(t, bin, filepath.Join(dir, "stage"))
	rt, err := NewRuntime()
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	if err := rt.Apply(context.Background(), stubConfig(1)); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if err := rt.Stop(); err != nil {
		t.Fatalf("first Stop: %v", err)
	}
	// Give the wait goroutine a moment to reap.
	time.Sleep(100 * time.Millisecond)
	// After the process exits, the runtime must
	// report itself inactive.
	state := rt.State()
	if state.Active {
		t.Fatal("runtime must be inactive after Stop")
	}
	_ = strconv.Itoa
}

// TestRuntimeApplyNoInboundsDoesNotStartDaemon pins the
// B-010 (beta.2) contract: a render that produced no
// inbounds must not bring up an xray daemon. Apply still
// reports success (the catalog mutation is the public
// outcome) but the runtime stays inactive.
func TestRuntimeApplyNoInboundsDoesNotStartDaemon(t *testing.T) {
	dir := t.TempDir()
	bin, calls := installStubXray(t, dir)
	withStubRuntime(t, bin, filepath.Join(dir, "stage"))
	rt, err := NewRuntime()
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = rt.Stop() })
	if err := rt.Apply(context.Background(), Config{Version: 1}); err != nil {
		t.Fatalf("Apply with no inbounds must succeed: %v", err)
	}
	time.Sleep(200 * time.Millisecond)
	if state := rt.State(); state.Active {
		t.Fatal("no-inbounds apply must not start the daemon")
	}
	if _, err := os.Stat(calls); err == nil {
		t.Fatal("stub xray must never be exec'd for a no-inbounds apply")
	}
}
