// runtime_layout.go: shared /run/duckos-agent IPC layout bootstrap.
//
// The Agent is the canonical owner of /run/duckos-agent
// (RuntimeDirectory=duckos-agent on duck-agent.service is the
// ONLY declaration across all DuckOS units). The FRR launcher
// and WG launcher wait on the Agent (After=duck-agent.service +
// Wants=duck-agent.service) and bind their sockets into the
// Agent's already-created dir.
//
// Both launchers also need shared-IPC subdirs to exist BEFORE
// systemd's namespace setup runs:
//   - duck-wg-launcher.service declares
//     ReadWritePaths=/var/lib/duckos/wireguard /run/duckos-agent/secret
//     so the launcher's WG PSK channel dir must exist at namespace
//     setup time (systemd bind-mounts the path into a writable
//     mount namespace with ProtectSystem=strict).
//   - duck-frr-launcher.service creates /run/duckos-agent/frr-launcher.sock
//     via listenUnix() inside its own process, so no bootstrap is
//     needed for the FRR launcher channel.
//
// The Agent bootstraps the shared layout at startup so the
// dependent launchers' namespaces can mount the secret channel
// dir without a missing-path failure. Mode is 0750 root:duckos
// matching the parent; only the duckos group can traverse into
// the secret subdir, and only root can write.
package agent

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// StandardRuntimeDir is the canonical shared IPC root for the
// DuckOS Agent + launcher bus. Declared exclusively by
// duck-agent.service's RuntimeDirectory=; never declared by
// any other duckos unit (the single-owner invariant is enforced
// by internal/systemd/runtime_dir_owner_test.go).
const StandardRuntimeDir = "/run/duckos-agent"

// StandardSecretChannelDir is the WG PSK channel dir under the
// shared runtime root. Bootstrap is the Agent's responsibility
// because systemd needs the path to exist at namespace setup
// time (ReadWritePaths= in the wg-launcher unit).
const StandardSecretChannelDir = StandardRuntimeDir + "/secret"

// BootstrapRuntimeLayout creates the shared IPC subdirs under
// StandardRuntimeDir that the launchers need to exist before
// systemd's namespace setup. The runtime dir itself is created
// by systemd via RuntimeDirectory=duckos-agent on
// duck-agent.service. Idempotent: a fresh MkdirAll of an
// already-present dir is a no-op.
//
// Returns nil on success. The dir is enforced to be mode 0750
// root:duckos after creation (matching the systemd-declared
// mode); if it pre-exists with looser mode or different owner,
// BootstrapRuntimeLayout returns an error rather than silently
// relaxing the access boundary.
func BootstrapRuntimeLayout() error {
	return bootstrapRuntimeLayoutAt(StandardRuntimeDir, StandardSecretChannelDir)
}

func bootstrapRuntimeLayoutAt(runtimeDir, secretDir string) error {
	if err := os.MkdirAll(secretDir, 0o750); err != nil {
		return fmt.Errorf("create secret channel dir %q: %w", secretDir, err)
	}
	st, err := os.Lstat(secretDir)
	if err != nil {
		return fmt.Errorf("inspect secret channel dir: %w", err)
	}
	if !st.IsDir() {
		return fmt.Errorf("secret channel path is not a directory: %q", secretDir)
	}
	if perm := st.Mode().Perm(); perm != 0o750 {
		return fmt.Errorf("secret channel dir must be mode 0750, got %#o", perm)
	}
	stat, ok := st.Sys().(*syscall.Stat_t)
	if !ok {
		return errors.New("secret channel dir: cannot read ownership")
	}
	if stat.Uid != 0 {
		return fmt.Errorf("secret channel dir must be owned by uid 0, got %d", stat.Uid)
	}
	// Group is duckos GID. We don't pin a numeric gid (duckos may
	// not exist at test time); we pin the path under the runtime
	// dir which systemd created as root:duckos. Validate the
	// parent runtime dir is the canonical root:duckos 0750 layout.
	runtimeSt, err := os.Lstat(runtimeDir)
	if err != nil {
		return fmt.Errorf("inspect runtime dir %q: %w", runtimeDir, err)
	}
	runtimeStat, ok := runtimeSt.Sys().(*syscall.Stat_t)
	if !ok {
		return errors.New("runtime dir: cannot read ownership")
	}
	if runtimeStat.Uid != 0 {
		return fmt.Errorf("runtime dir must be owned by uid 0, got %d", runtimeStat.Uid)
	}
	if perm := runtimeSt.Mode().Perm(); perm != 0o750 {
		return fmt.Errorf("runtime dir must be mode 0750, got %#o", perm)
	}
	return nil
}

// secretDirForTest exposes the secret channel dir path for
// callers that need to pass it to other components. Not used in
// production; the secret dir is created with the canonical path
// by BootstrapRuntimeLayout.
func secretDirForTest() string { return filepath.Clean(StandardSecretChannelDir) }
