//go:build linux

// cmd/duck-frr-launcher/tracker_test.go: regression
// tests for the FRR lifetime tracker.
//
// These tests verify the identity-gated kill semantics on
// data the tracker manipulates directly (Tracker / Snapshot
// / pidfile glob / pidAlive / pidGone). The full daemon
// wire-protocol integration test (including a fake FRR
// process with -N flag in its cmdline) is run live on the
// lab VM against the real /usr/lib/frr/{zebra,bgpd}; see
// the live qualification script for that path.
//
// PID-reuse coverage: TestIsFRRProcess_RefusesRecycledPID
// uses the test binary itself as the "recycled PID"
// stand-in. Its cmdline is `launcher.test -test.run=...`,
// which is NOT /usr/lib/frr/. The identity gate must
// reject it without signalling.

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIsFRRProcess_RejectsUnrelatedPID(t *testing.T) {
	// /proc/self is this test process. It is not FRR.
	if err := isFRRProcess(os.Getpid(), "zebra", "duckos-test"); err == nil {
		t.Fatalf("isFRRProcess(self): want err, got nil")
	}
}

func TestIsFRRProcess_RejectsDeadPID(t *testing.T) {
	// A clearly-impossible PID; /proc/0 does not exist
	// on any unix and 99999999 is not allocated.
	if err := isFRRProcess(99999999, "zebra", "duckos-test"); err == nil {
		t.Fatalf("isFRRProcess(unallocated): want err, got nil")
	}
}

func TestTerminateAndReap_RefusesRecycledPID(t *testing.T) {
	// The test process is a stand-in for "the PID was
	// recycled to a non-FRRR process." terminateAndReap
	// MUST refuse to signal it.
	if err := terminateAndReap(os.Getpid(), "zebra", "duckos-test"); err == nil {
		t.Fatalf("terminateAndReap(recycled): want err, got nil")
	} else if !errors.Is(err, errIdentityMismatch) && !errors.Is(err, errProcessGone) {
		// Either error class is acceptable: identity
		// mismatch (comm doesn't equal "zebra") OR
		// process gone (if our /proc read fails).
		t.Fatalf("terminateAndReap(recycled): want identity-or-gone err, got %v", err)
	}
	// Test process must still be alive.
	if err := procSignal(pidAlive, os.Getpid()); err != nil {
		t.Fatalf("self signal(0): %v", err)
	}
}

func TestTracker_BasicLookupAndForget(t *testing.T) {
	tr := NewTracker()
	tr.Put(&TrackedFRR{Binary: "zebra", Pathspace: "ps-A", PID: 1001})
	tr.Put(&TrackedFRR{Binary: "bgpd", Pathspace: "ps-A", PID: 1002})
	tr.Put(&TrackedFRR{Binary: "zebra", Pathspace: "ps-B", PID: 1003})

	if e := tr.Lookup("ps-A", "zebra"); e == nil || e.PID != 1001 {
		t.Fatalf("lookup ps-A zebra: got %v", e)
	}
	if e := tr.Lookup("ps-A", "bgpd"); e == nil || e.PID != 1002 {
		t.Fatalf("lookup ps-A bgpd: got %v", e)
	}
	if e := tr.Lookup("ps-B", "zebra"); e == nil || e.PID != 1003 {
		t.Fatalf("lookup ps-B zebra: got %v", e)
	}
	tr.Forget("ps-A", "zebra")
	if e := tr.Lookup("ps-A", "zebra"); e != nil {
		t.Fatalf("lookup ps-A zebra after forget: want nil, got %v", e)
	}
	tr.ForgetPathspace("ps-A")
	if e := tr.Lookup("ps-A", "bgpd"); e != nil {
		t.Fatalf("lookup ps-A bgpd after ForgetPathspace: want nil, got %v", e)
	}
	if e := tr.Lookup("ps-B", "zebra"); e == nil || e.PID != 1003 {
		t.Fatalf("lookup ps-B zebra after ForgetPathspace: got %v", e)
	}
}

func TestSnapshot_DeepCopy(t *testing.T) {
	tr := NewTracker()
	tr.Put(&TrackedFRR{Binary: "zebra", Pathspace: "ps-A", PID: 1001, Cmdline: "first"})
	snap := tr.Snapshot()
	if len(snap) != 1 {
		t.Fatalf("snapshot: want 1, got %d", len(snap))
	}
	snap[0].Cmdline = "mutated"
	e := tr.Lookup("ps-A", "zebra")
	if e == nil || e.Cmdline != "first" {
		t.Fatalf("tracker leaked snapshot mutation; got Cmdline=%q", e.Cmdline)
	}
}

func TestPidfileGlob_FindsPidfiles(t *testing.T) {
	tmp := t.TempDir()
	// Layout: <tmp>/<ps>/zebra.pid  (zebra only)
	for _, ps := range []string{"alpha", "beta"} {
		if err := os.MkdirAll(filepath.Join(tmp, ps), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(tmp, ps, "zebra.pid"), []byte("12345\n"), 0o644); err != nil {
			t.Fatalf("write zebra: %v", err)
		}
	}
	// Add an unrelated file: should NOT be picked up.
	if err := os.WriteFile(filepath.Join(tmp, "alpha", "other.txt"), []byte("nope"), 0o644); err != nil {
		t.Fatalf("write other: %v", err)
	}
	files, err := pidfileGlob(tmp, "zebra")
	if err != nil {
		t.Fatalf("pidfileGlob: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("pidfileGlob: want 2, got %d (%v)", len(files), files)
	}
	// No bgpd files exist.
	files, err = pidfileGlob(tmp, "bgpd")
	if err != nil {
		t.Fatalf("pidfileGlob bgpd: %v", err)
	}
	if len(files) != 0 {
		t.Fatalf("pidfileGlob bgpd: want 0, got %d", len(files))
	}
}

func TestPidfileGlob_MissingRootIsEmpty(t *testing.T) {
	files, err := pidfileGlob("/definitely/not/here", "zebra")
	if err != nil {
		t.Fatalf("pidfileGlob missing root: %v", err)
	}
	if files != nil {
		t.Fatalf("pidfileGlob missing root: want nil, got %v", files)
	}
}

func TestFormatList_Stable(t *testing.T) {
	entries := []*TrackedFRR{
		{Binary: "zebra", Pathspace: "ps-A", PID: 1001},
		{Binary: "bgpd", Pathspace: "ps-A", PID: 1002},
	}
	out := string(formatList(entries))
	if !strings.HasPrefix(out, "OK\n") {
		t.Fatalf("formatList must start with OK\\n: %q", out)
	}
	if !strings.Contains(out, "zebra ps-A 1001") {
		t.Fatalf("formatList missing zebra: %q", out)
	}
	if !strings.Contains(out, "bgpd ps-A 1002") {
		t.Fatalf("formatList missing bgpd: %q", out)
	}
}

func TestParseSTOPArg_OneOrTwo(t *testing.T) {
	ps, bin, ok := parseSTOPArg([]string{"ps-A"})
	if !ok || ps != "ps-A" || bin != "" {
		t.Fatalf("parseSTOPArg 1-arg: got ps=%q bin=%q ok=%v", ps, bin, ok)
	}
	ps, bin, ok = parseSTOPArg([]string{"ps-A", "zebra"})
	if !ok || ps != "ps-A" || bin != "zebra" {
		t.Fatalf("parseSTOPArg 2-arg: got ps=%q bin=%q ok=%v", ps, bin, ok)
	}
	_, _, ok = parseSTOPArg(nil)
	if ok {
		t.Fatalf("parseSTOPArg nil: want ok=false")
	}
	_, _, ok = parseSTOPArg([]string{})
	if ok {
		t.Fatalf("parseSTOPArg empty: want ok=false")
	}
}

// procSignal wraps syscall.Signal(0) so the test can
// pass the pidAlive function as a callback.
func procSignal(fn func(int) error, pid int) error {
	return fn(pid)
}

// silence unused-import warnings for fmt if some tests
// do not reference it directly.
var _ = fmt.Sprintf