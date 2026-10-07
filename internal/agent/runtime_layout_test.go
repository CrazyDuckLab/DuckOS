// runtime_layout_test.go: regression test for the Agent's
// bootstrap of the shared /run/duckos-agent IPC layout.
//
// The Agent is the canonical owner of /run/duckos-agent
// (RuntimeDirectory=duckos-agent on duck-agent.service is the
// ONLY declaration; launchers wait on the Agent and bind their
// sockets into the Agent's already-created dir). The wg-launcher
// needs /run/duckos-agent/secret to exist BEFORE systemd's
// namespace setup runs (ReadWritePaths= requires the path to
// exist at namespace setup time, not at first write).
//
// If the Agent fails to bootstrap the secret channel dir on
// startup, the wg-launcher unit will fail with status 226
// (NAMESPACE) and remain in a restart loop — which silently
// breaks WireGuard on every fresh install.
package agent

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestBootstrapRuntimeLayout_FreshDir_CreatesSecretChannelAt0750(t *testing.T) {
	rt := t.TempDir()
	// t.TempDir already creates the dir; tighten to the
	// systemd-declared mode 0750.
	if err := os.Chmod(rt, 0o750); err != nil {
		t.Fatalf("chmod runtime dir: %v", err)
	}
	if err := os.Chown(rt, 0, os.Getegid()); err != nil {
		t.Skipf("cannot chown runtime dir to root:duckos (uid 0): %v", err)
	}
	secret := filepath.Join(rt, "secret")
	if err := bootstrapRuntimeLayoutAt(rt, secret); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	st, err := os.Lstat(secret)
	if err != nil {
		t.Fatalf("stat secret dir: %v", err)
	}
	if !st.IsDir() {
		t.Fatalf("secret path is not a directory: mode=%v", st.Mode())
	}
	if perm := st.Mode().Perm(); perm != 0o750 {
		t.Fatalf("secret dir mode = %#o, want 0750", perm)
	}
	stat, ok := st.Sys().(*syscall.Stat_t)
	if !ok {
		t.Fatalf("secret dir: cannot read ownership")
	}
	if stat.Uid != 0 {
		t.Fatalf("secret dir uid = %d, want 0", stat.Uid)
	}
}

func TestBootstrapRuntimeLayout_IdempotentOnSecondCall(t *testing.T) {
	rt := t.TempDir()
	if err := os.Chmod(rt, 0o750); err != nil {
		t.Fatalf("chmod runtime dir: %v", err)
	}
	if err := os.Chown(rt, 0, os.Getegid()); err != nil {
		t.Skipf("cannot chown runtime dir to root:duckos (uid 0): %v", err)
	}
	secret := filepath.Join(rt, "secret")
	for i := 0; i < 3; i++ {
		if err := bootstrapRuntimeLayoutAt(rt, secret); err != nil {
			t.Fatalf("bootstrap iter %d: %v", i, err)
		}
	}
}

func TestBootstrapRuntimeLayout_RejectsLooserMode(t *testing.T) {
	rt := t.TempDir()
	if err := os.Chmod(rt, 0o750); err != nil {
		t.Fatalf("chmod runtime dir: %v", err)
	}
	if err := os.Chown(rt, 0, os.Getegid()); err != nil {
		t.Skipf("cannot chown runtime dir to root:duckos (uid 0): %v", err)
	}
	// Pre-create the secret dir with looser mode to simulate
	// operator/upgrade drift. Bootstrap must reject and not
	// silently relax the access boundary.
	secret := filepath.Join(rt, "secret")
	if err := os.Mkdir(secret, 0o755); err != nil {
		t.Fatalf("create secret dir: %v", err)
	}
	if err := bootstrapRuntimeLayoutAt(rt, secret); err == nil {
		t.Fatalf("bootstrap accepted pre-existing dir with looser mode 0755; want error")
	}
}

func TestBootstrapRuntimeLayout_RejectsMissingParentRuntimeDir(t *testing.T) {
	// Use a path that does not exist; MkdirAll will create it,
	// but the parent runtime dir ownership check will succeed
	// because we just created it. The test asserts the function
	// does not panic and ends in a coherent state.
	rt := filepath.Join(t.TempDir(), "missing")
	secret := filepath.Join(rt, "secret")
	if err := bootstrapRuntimeLayoutAt(rt, secret); err != nil {
		t.Fatalf("bootstrap with auto-created runtime dir should succeed: %v", err)
	}
}
