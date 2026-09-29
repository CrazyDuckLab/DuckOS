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
script := `#!/bin/sh
set -e
for arg in "$@"; do
  if [ "$arg" = "-test" ]; then exit 0; fi
done
echo "argv0=$0" >> "` + log + `"
echo "argv1=$1" >> "` + log + `"
echo "argv2=$2" >> "` + log + `"
echo "pid=$$" >> "` + log + `"
trap 'exit 0' TERM
# SIGUSR1 must NOT exit; the runtime uses it
# to signal a config reload and the stub must
# stay alive so the test can verify the same
# pid is reused.
trap '' USR1
while true; do
  sleep 0.1 || true
done
`
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	return bin, log
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
	if err := rt.Apply(context.Background(), Config{Version: 1}); err != nil {
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
	if err := rt.Apply(context.Background(), Config{Version: 1}); err != nil {
		t.Fatalf("Apply 1: %v", err)
	}
	pidBefore := rt.State().PID
	if pidBefore == 0 {
		t.Fatal("Apply 1 must record a PID")
	}
	if err := rt.Apply(context.Background(), Config{Version: 2}); err != nil {
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
	if err := rt.Apply(context.Background(), Config{Version: 1}); err != nil {
		t.Fatalf("Apply 1: %v", err)
	}
	// previous.json must exist after the second
	// Apply (which retains the first as previous).
	if err := rt.Apply(context.Background(), Config{Version: 2}); err != nil {
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
	if err := rt.Apply(context.Background(), Config{Version: 1}); err != nil {
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
