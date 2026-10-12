//go:build linux

// Package systemd inspects the unit files DuckOS ships
// and asserts the runtime directory ownership model is
// internally consistent. Three units
//
//	duck-agent.service
//	duck-frr-launcher.service
//	duck-wg-launcher.service
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
// ship in an artifact.
//
// Invariant: exactly ONE unit declares
// RuntimeDirectory=duckos-agent (the canonical owner).
// Multiple units claiming the same RuntimeDirectory
// causes systemd to wipe the dir inode on every
// restart, breaking duck-api's IPC path to
// /agent/v1/*. The Agent is the canonical owner of
// /run/duckos-agent; the FRR + WG launchers bind into
// the Agent's already-created dir and have After=
// dependencies on duck-agent.service so the Agent has
// already created the dir before they start.
func TestRuntimeDirectoryOwnerModelShippedUnits(t *testing.T) {
	root := shippedUnitsRoot(t)
	units := []string{
		"duck-agent.service",
		"duck-frr-launcher.service",
		"duck-wg-launcher.service",
	}
	var owners []string
	var users []string
	for _, u := range units {
		path := filepath.Join(root, u)
		if _, err := os.Stat(path); err != nil {
			t.Skipf("shipped unit %s missing: %v", path, err)
		}
		dirs := runtimeDirectories(path)
		var hasDuckosAgent bool
		for _, d := range dirs {
			if d == "duckos-agent" {
				hasDuckosAgent = true
			}
		}
		if hasDuckosAgent {
			owners = append(owners, u)
			group, _ := unitKey(t, path, "Group")
			if group != "duckos" {
				t.Fatalf("%s declares Group=%q for the canonical RuntimeDirectory=duckos-agent parent but the group must be duckos so the Agent and the FRR/WG launcher sockets are reachable by duck-api.", path, group)
			}
			mode, _ := unitKey(t, path, "RuntimeDirectoryMode")
			if mode != "0750" {
				t.Fatalf("%s declares RuntimeDirectoryMode=%q for the canonical RuntimeDirectory=duckos-agent parent but must be 0750 so duckos can enter the dir while other groups cannot.", path, mode)
			}
		} else {
			users = append(users, u)
		}
	}
	if len(owners) != 1 {
		t.Fatalf("expected exactly ONE unit to declare RuntimeDirectory=duckos-agent (canonical owner), got %d: %v. Multiple owners cause systemd to wipe the dir inode on every restart, breaking /agent/v1/* proxy from duck-api.", len(owners), owners)
	}
	if filepath.Base(owners[0]) != "duck-agent.service" {
		t.Fatalf("canonical RuntimeDirectory=duckos-agent owner must be duck-agent.service (creates the dir at startup), got %s", owners[0])
	}
	for _, u := range users {
		path := filepath.Join(root, u)
		dirs := runtimeDirectories(path)
		var hasDuckosAgent bool
		for _, d := range dirs {
			if d == "duckos-agent" {
				hasDuckosAgent = true
			}
		}
		if hasDuckosAgent {
			t.Fatalf("%s must NOT declare RuntimeDirectory=duckos-agent; only duck-agent.service is the canonical owner. Multiple owners wipe the dir inode on restart.", path)
		}
	}
}
