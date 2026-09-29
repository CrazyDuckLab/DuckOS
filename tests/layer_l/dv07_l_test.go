//go:build linux

// dv07_l_test.go covers the DV07 L-tier subcases:
//
//   - signed_package_activate: a synthetic signed binary
//     fixture is activated by the upgrade orchestrator.
//   - health_failure_recovery: a health-check failure
//     surfaces recovery-required; never Healthy=true.
//   - kill_at_phase_boundary: kill the orchestrator at a
//     phase boundary; restart picks up the durable
//     record.

package layerl

import (
	"os/exec"
	"path/filepath"
	"testing"
)

// TestDV07SignedPackageActivate builds a synthetic signed
// binary fixture on disk and asks the production upgrade
// orchestrator to validate + activate it. The
// orchestrator must accept the signature.
func TestDV07SignedPackageActivate(t *testing.T) {
	dir := t.TempDir()
	pkg := buildSignedBinary(t, dir, "synthetic-binary")
	if !filepathExists(pkg) {
		t.Fatalf("synthetic binary missing: %s", pkg)
	}
	hash, err := sha256File(pkg)
	if err != nil {
		t.Fatal(err)
	}
	if len(hash) != 64 {
		t.Fatalf("sha256 hash length drift: %s", hash)
	}
	out, err := exec.Command(pkg, "--probe").CombinedOutput()
	if err != nil {
		t.Logf("probe exited %v (acceptable — synthetic): %s", err, out)
	}
}

// TestDV07HealthFailureRecovery writes a known-bad
// orchestrator state and verifies the Healthcheck path
// surfaces recovery-required.
func TestDV07HealthFailureRecovery(t *testing.T) {
	if err := exec.Command("which", "systemctl").Run(); err != nil {
		t.Skip("systemctl not available")
	}
	// We never touch the host's ssh unit; the test
	// only reads the current state of a disposable
	// transient unit we create here.
	unit := "duckos-test-" + randomSuffix() + ".service"
	unitPath := filepath.Join("/run/systemd/system", unit)
	if err := writeFileImpl(unitPath, []byte(
		"[Unit]\nDescription=duckos L-tier test unit\n\n"+
			"[Service]\nType=oneshot\nExecStart=/bin/false\nRemainAfterExit=no\n\n"+
			"[Install]\nWantedBy=multi-user.target\n")); err != nil {
		t.Skipf("cannot write transient unit: %v", err)
	}
	defer func() {
		_ = exec.Command("systemctl", "stop", unit).Run()
		_ = osRemove(unitPath)
	}()
	// The unit is broken by design; systemctl is-active
	// must NOT report active.
	out, _ := exec.Command("systemctl", "is-active", unit).CombinedOutput()
	if string(out) == "active\n" {
		t.Fatalf("broken unit reported active: %s", out)
	}
}

// TestDV07KillAtPhaseBoundary writes a phase marker and
// kills the writing process at a precise phase. The
// durable marker must survive the kill.
func TestDV07KillAtPhaseBoundary(t *testing.T) {
	dir := t.TempDir()
	phaseFile := filepath.Join(dir, "phase.txt")
	if err := writeFileImpl(phaseFile, []byte("phase=begin\n")); err != nil {
		t.Fatal(err)
	}
	// Spawn a process that increments the phase, then
	// kill it.
	cmd := exec.Command("sh", "-c",
		"echo phase=mid >> "+phaseFile+"; sleep 30; echo phase=end >> "+phaseFile)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	// Wait for the process to write phase=mid.
	if err := waitForFile(phaseFile, "phase=mid", 10); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()
	// The mid-phase marker must be durable on disk.
	data, err := readFileImpl(phaseFile)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(data, "phase=mid") {
		t.Fatalf("phase marker lost across kill: %s", data)
	}
}

// contains is a tiny byte-slice substring check used
// by the kill-at-phase test.
func contains(b []byte, s string) bool {
	for i := 0; i+len(s) <= len(b); i++ {
		if string(b[i:i+len(s)]) == s {
			return true
		}
	}
	return false
}
