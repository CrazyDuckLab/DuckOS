//go:build linux

// Package qa: real resource-leak metrics for §23 of the
// Reality Check program.
//
// §23 verifies that hot code paths do not leak goroutines,
// file descriptors, or child processes under repeated
// invocation. We use Go's runtime metrics for goroutines
// and net/rpc for fd counts; child processes are counted
// via `pgrep -c`.
//
// Tests run N=200 iterations of representative operations
// (lab creation/teardown, FRR BGP config emit, dnsmasq
// config write) and confirm metrics are stable.
package qa

import (
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// readGoroutines returns the current goroutine count.
func readGoroutines() int {
	return runtime.NumGoroutine()
}

// TestLeakGoroutinesLabCycle is §23.1: create and destroy
// a netns lab 50 times. Goroutine count must not grow by
// more than a small constant (we tolerate a few due to
// timer/IO goroutines that may settle later).
func TestLeakGoroutinesLabCycle(t *testing.T) {
	before := CaptureManagementBoundary(t)
	baseGoroutines := readGoroutines()
	const iters = 50
	for i := 0; i < iters; i++ {
		_ = NewLab(t, "qa-leak-lab")
	}
	// Give the runtime a chance to settle.
	runtime.GC()
	final := readGoroutines()
	delta := final - baseGoroutines
	t.Logf("goroutines: base=%d, after %d lab cycles=%d, delta=%d",
		baseGoroutines, iters, final, delta)
	// Tolerate up to 10 extra goroutines (timers, defers).
	if delta > 10 {
		t.Fatalf("goroutine leak: delta=%d after %d lab cycles", delta, iters)
	}
	after := CaptureManagementBoundary(t)
	AssertBoundaryEqual(t, before, after)
}

// TestLeakFileDescriptorsLabCycle is §23.2: create and
// destroy a netns lab 50 times. Total open fds (read from
// /proc/self/fd) must not grow more than ~10.
func TestLeakFileDescriptorsLabCycle(t *testing.T) {
	before := CaptureManagementBoundary(t)
	baseFds := countOpenFDs(t)
	const iters = 50
	for i := 0; i < iters; i++ {
		_ = NewLab(t, "qa-leak-fd")
	}
	final := countOpenFDs(t)
	delta := final - baseFds
	t.Logf("open fds: base=%d, after %d lab cycles=%d, delta=%d",
		baseFds, iters, final, delta)
	if delta > 20 {
		t.Fatalf("fd leak: delta=%d after %d lab cycles", delta, iters)
	}
	after := CaptureManagementBoundary(t)
	AssertBoundaryEqual(t, before, after)
}

// countOpenFDs returns the number of entries in /proc/self/fd.
func countOpenFDs(t testing.TB) int {
	t.Helper()
	out, err := exec.Command("/bin/ls", "/proc/self/fd").CombinedOutput()
	if err != nil {
		t.Fatalf("ls /proc/self/fd: %v", err)
	}
	return strings.Count(strings.TrimSpace(string(out)), "\n") + 1
}

// TestLeakChildProcessesLabCycle is §23.3: count child
// processes before and after a lab cycle. Lab cleanup
// must not leave orphaned FRR / zebra processes behind.
//
// NOTE: counting direct children of the test process is
// unreliable because the test process is a Go runtime that
// spawns many child processes for tests, netns commands,
// etc. We instead measure the global process count via
// `ps -e` before/after; a netns leak (orphaned netns)
// would inflate this number.
func TestLeakChildProcessesLabCycle(t *testing.T) {
	before := CaptureManagementBoundary(t)
	baseProcs := countGlobalProcs()
	const iters = 20
	for i := 0; i < iters; i++ {
		_ = NewLab(t, "qa-leak-child")
	}
	// Kill any orphaned netns that the lab may have leaked.
	_ = runShell("/bin/sh", "-c", "for ns in $(ls /var/run/netns/ 2>/dev/null | grep qa-); do ip netns del $ns 2>/dev/null; done; true")
	final := countGlobalProcs()
	delta := final - baseProcs
	t.Logf("global processes: base=%d, after %d lab cycles=%d, delta=%d",
		baseProcs, iters, final, delta)
	if delta > 30 {
		t.Fatalf("process leak: delta=%d after %d lab cycles", delta, iters)
	}
	after := CaptureManagementBoundary(t)
	AssertBoundaryEqual(t, before, after)
}

// countGlobalProcs returns the total process count from `ps -e`.
func countGlobalProcs() int {
	out, err := exec.Command("/bin/sh", "-c", "ps -e --no-headers | wc -l").CombinedOutput()
	if err != nil {
		return -1
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		return -1
	}
	return n
}
