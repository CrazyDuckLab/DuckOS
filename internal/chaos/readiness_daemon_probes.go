// chaos/readiness_daemon_probes.go: G04 daemon-detection
// probes. Each probe classifies one feature by inspecting
// the local system for the required daemon, binary, kernel
// module or configuration directory. Probes are non-mutating
// and bounded; they may exec `which`, read /proc and
// /sys/class/net, and stat /run.
//
// Probes are Linux-aware: on non-Linux hosts they
// immediately return ReadyUnsupported with a stable
// reason. Production callers wrap the recorder with a
// build-tag gate so non-Linux callers never enter this
// file.
package chaos

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// ProbeTimeout is the per-probe ceiling used by daemon
// detection. Kept short because every probe is bounded
// to a few exec calls.
const ProbeTimeout = 1500 * time.Millisecond

// DaemonProbe classifies a feature by checking for a
// required binary on PATH and (optionally) a state file in
// /run. The probe never executes the daemon itself.
type DaemonProbe struct {
	FeatureKey   string
	Binary       string
	StateFile    string // optional: /run/<...>
	VersionArgs  []string
	MinVersionOK func(found string) bool // optional; nil = any version counts
}

// Feature returns the configured feature key.
func (d DaemonProbe) Feature() string { return d.FeatureKey }

// Probe runs the daemon detection. State transitions:
//   - binary missing -> ReadyHWPending with reason
//   - state file missing -> ReadyDegraded
//   - version parse fails -> ReadyExperimental
//   - otherwise -> ReadySupported with version evidence
func (d DaemonProbe) Probe(ctx context.Context) (Readiness, string, error) {
	if runtime.GOOS != "linux" {
		return ReadyUnsupported, "non-linux host", nil
	}
	if d.FeatureKey == "" || d.Binary == "" {
		return "", "", errors.New("chaos: daemon probe missing feature key or binary")
	}
	path, err := exec.LookPath(d.Binary)
	if err != nil {
		return ReadyHWPending, fmt.Sprintf("%s binary not on PATH", d.Binary), nil
	}
	if d.StateFile != "" {
		if _, err := os.Stat(d.StateFile); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return ReadyDegraded, fmt.Sprintf("%s present but %s not running", d.Binary, d.StateFile), nil
			}
			return ReadyDegraded, fmt.Sprintf("%s state unreadable: %v", d.StateFile, err), nil
		}
	}
	if len(d.VersionArgs) == 0 {
		return ReadySupported, fmt.Sprintf("%s available at %s", d.Binary, path), nil
	}
	cmd := exec.CommandContext(ctx, d.Binary, d.VersionArgs...)
	cmd.Args[0] = path
	out, err := cmd.Output()
	if err != nil {
		return ReadyExperimental, fmt.Sprintf("%s invocation failed: %v", d.Binary, err), nil
	}
	ver := firstNonEmptyLine(string(out))
	if ver == "" {
		return ReadyExperimental, fmt.Sprintf("%s produced no version output", d.Binary), nil
	}
	if d.MinVersionOK != nil && !d.MinVersionOK(ver) {
		return ReadyExperimental, fmt.Sprintf("%s version %q below minimum", d.Binary, ver), nil
	}
	return ReadySupported, fmt.Sprintf("%s %s", d.Binary, ver), nil
}

// KernelModuleProbe classifies a feature based on the
// presence of a Linux kernel module in /sys/module.
type KernelModuleProbe struct {
	FeatureKey string
	ModuleName string
}

// Feature returns the configured feature key.
func (k KernelModuleProbe) Feature() string { return k.FeatureKey }

// Probe checks /sys/module/<name> existence.
func (k KernelModuleProbe) Probe(_ context.Context) (Readiness, string, error) {
	if runtime.GOOS != "linux" {
		return ReadyUnsupported, "non-linux host", nil
	}
	if k.FeatureKey == "" || k.ModuleName == "" {
		return "", "", errors.New("chaos: kernel module probe missing feature or module name")
	}
	entry := filepath.Join("/sys/module", k.ModuleName)
	if _, err := os.Stat(entry); err == nil {
		return ReadySupported, fmt.Sprintf("kernel module %s present", k.ModuleName), nil
	}
	if _, err := os.Stat("/sys/module"); err != nil {
		return ReadyHWPending, "/sys/module not mounted", nil
	}
	return ReadyHWPending, fmt.Sprintf("kernel module %s not loaded", k.ModuleName), nil
}

// ExecutableProbe is the simplest possible probe: it
// checks whether a binary exists on PATH. It is used for
// Xray and similar out-of-tree binaries.
type ExecutableProbe struct {
	FeatureKey string
	Binary     string
	Reason     string // note prefix when missing
}

// Feature returns the configured feature key.
func (e ExecutableProbe) Feature() string { return e.FeatureKey }

// Probe resolves the binary.
func (e ExecutableProbe) Probe(_ context.Context) (Readiness, string, error) {
	if runtime.GOOS != "linux" {
		return ReadyUnsupported, "non-linux host", nil
	}
	if e.FeatureKey == "" || e.Binary == "" {
		return "", "", errors.New("chaos: executable probe missing feature or binary")
	}
	if _, err := exec.LookPath(e.Binary); err != nil {
		reason := e.Reason
		if reason == "" {
			reason = fmt.Sprintf("%s binary not on PATH", e.Binary)
		}
		return ReadyCredPending, reason, nil
	}
	return ReadySupported, fmt.Sprintf("%s available", e.Binary), nil
}

// PathProbe classifies a feature based on a filesystem
// path. Used for configuration directories.
type PathProbe struct {
	FeatureKey string
	Path       string
	WantDir    bool
	Label      string
}

// Feature returns the configured feature key.
func (p PathProbe) Feature() string { return p.FeatureKey }

// Probe stats the configured path.
func (p PathProbe) Probe(_ context.Context) (Readiness, string, error) {
	if runtime.GOOS != "linux" {
		return ReadyUnsupported, "non-linux host", nil
	}
	if p.FeatureKey == "" || p.Path == "" {
		return "", "", errors.New("chaos: path probe missing feature or path")
	}
	info, err := os.Stat(p.Path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ReadyHWPending, fmt.Sprintf("%s not present", p.LabelOrPath()), nil
		}
		return ReadyDegraded, fmt.Sprintf("%s unreadable: %v", p.LabelOrPath(), err), nil
	}
	if p.WantDir && !info.IsDir() {
		return ReadyDegraded, fmt.Sprintf("%s exists but is not a directory", p.LabelOrPath()), nil
	}
	if !p.WantDir && info.IsDir() {
		return ReadyDegraded, fmt.Sprintf("%s is a directory, expected file", p.LabelOrPath()), nil
	}
	return ReadySupported, fmt.Sprintf("%s present", p.LabelOrPath()), nil
}

// LabelOrPath returns the human label or the raw path.
func (p PathProbe) LabelOrPath() string {
	if p.Label != "" {
		return p.Label
	}
	return p.Path
}

func firstNonEmptyLine(s string) string {
	for _, line := range strings.Split(strings.TrimSpace(s), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return ""
}
