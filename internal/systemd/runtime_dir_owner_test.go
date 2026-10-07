//go:build linux

// Package systemd inspects the unit files DuckOS ships
// and asserts the runtime directory ownership model is
// internally consistent. Three units
//
//   duck-agent.service
//   duck-frr-launcher.service
//   duck-wg-launcher.service
//
// share /run/duckos-agent as their RuntimeDirectory.
// systemd applies RuntimeDirectory=Mode and the unit's
// Group= to that directory on each unit start. Conflicting
// User/Group/Mode declarations cause the parent dir to
// be re-owned on restart, breaking duck-api's IPC
// connection to /agent/v1/*.
package systemd

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// shippedUnitsRoot finds the absolute path to
// packaging/systemd in the repo. The test runs in two
// modes:
//
//   - In-tree on the build host (developer CI): the
//     source tree is mounted; runtime.Caller locates
//     the test file and we ascend to packaging/systemd.
//   - Cross-compiled test binary run on a Linux test
//     host: the test binary is shipped with the source
//     tree via scp or a build pipeline; the caller sets
//     DUCKOS_PACKAGING_ROOT to its absolute path.
func shippedUnitsRoot(t *testing.T) string {
	t.Helper()
	if v := os.Getenv("DUCKOS_PACKAGING_ROOT"); v != "" {
		if st, err := os.Stat(v); err == nil && st.IsDir() {
			return v
		}
		t.Skipf("DUCKOS_PACKAGING_ROOT=%s is not a directory", v)
		return ""
	}
	_, here, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("runtime.Caller failed")
	}
	dir := filepath.Dir(here)
	for i := 0; i < 8; i++ {
		candidate := filepath.Join(dir, "packaging", "systemd")
		if st, err := os.Stat(candidate); err == nil && st.IsDir() {
			return candidate
		}
		dir = filepath.Dir(dir)
	}
	t.Skipf("could not locate packaging/systemd in any ancestor of %s and DUCKOS_PACKAGING_ROOT is unset", here)
	return ""
}

// unitKey extracts a single setting from a unit file.
// The line must be `Key=Value` (no continuation, no
// quoted spans).
func unitKey(t *testing.T, unit, key string) (string, bool) {
	t.Helper()
	data, err := os.ReadFile(unit)
	if err != nil {
		return "", false
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, key+"=") {
			return strings.TrimPrefix(line, key+"="), true
		}
	}
	return "", false
}

// runtimeDirectories declared on the unit. systemd
// allows `RuntimeDirectory=foo bar baz` which expands to
// /run/foo /run/bar /run/baz. We treat the whole RHS as
// space-separated.
func runtimeDirectories(unit string) []string {
	data, err := os.ReadFile(unit)
	if err != nil {
		return nil
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "RuntimeDirectory=") {
			v := strings.TrimPrefix(line, "RuntimeDirectory=")
			return strings.Fields(v)
		}
	}
	return nil
}

// TestRuntimeDirectoryOwnerModelShippedUnits checks the
// ACTUAL files under packaging/systemd/ in the repo.
// This is the gate that catches regressions before they
// ship in an artifact. The test only verifies the three
// duck units that share /run/duckos-agent.
func TestRuntimeDirectoryOwnerModelShippedUnits(t *testing.T) {
	root := shippedUnitsRoot(t)
	units := []string{
		"duck-agent.service",
		"duck-frr-launcher.service",
		"duck-wg-launcher.service",
	}
	type sig struct {
		group, mode string
		has         bool
	}
	signatures := map[string]sig{}
	for _, u := range units {
		path := filepath.Join(root, u)
		if _, err := os.Stat(path); err != nil {
			t.Skipf("shipped unit %s missing: %v", path, err)
		}
		dirs := runtimeDirectories(path)
		var has bool
		for _, d := range dirs {
			if d == "duckos-agent" {
				has = true
			}
		}
		if !has {
			continue
		}
		group, _ := unitKey(t, path, "Group")
		mode, _ := unitKey(t, path, "RuntimeDirectoryMode")
		signatures[u] = sig{group: group, mode: mode, has: true}
	}
	if len(signatures) != 3 {
		t.Fatalf("expected all three units to declare RuntimeDirectory=duckos-agent, got: %v", signatures)
	}
	for u, s := range signatures {
		if s.group != "duckos" {
			t.Fatalf("%s declares Group=%q but the RuntimeDirectory=duckos-agent parent requires Group=duckos so the launcher-created sockets are reachable by duck-api.", u, s.group)
		}
		if s.mode != "0750" {
			t.Fatalf("%s declares RuntimeDirectoryMode=%q but must match duck-agent.service (0750) so no unit re-chmods the shared parent dir.", u, s.mode)
		}
	}
}