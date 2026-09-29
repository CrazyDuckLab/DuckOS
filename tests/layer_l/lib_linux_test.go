//go:build linux

// Layer-L tests exercise the L-tier subcases from the
// backend acceptance manifest against the real Linux
// fixture (netns/veth/tc/wg/xray/frr/flock). The tests
// never touch the management interface eth0, never
// flush host nftables, and never modify the default
// route. Each test uses private temp dirs / netns and
// is fully torn down at the end via t.Cleanup.

package layerl

import (
	"context"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// withRoot returns a context that requires root + ip(8)
// + the named tools. The test is skipped if any of them
// is unavailable — this matches the contract's
// environment-aware skip rule.
func withRoot(t *testing.T, tools ...string) {
	t.Helper()
	if os.Getuid() != 0 {
		t.Skip("layer-L tests require root; run with sudo")
	}
	for _, tool := range tools {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s(8) is not available: %v", tool, err)
		}
	}
}

// netnsRun creates an isolated namespace and runs fn in
// it. The namespace is torn down at the end of the test.
func netnsRun(t *testing.T, name string, fn func(ctx context.Context) error) {
	t.Helper()
	withRoot(t, "ip")
	cleanup := exec.Command("ip", "netns", "del", name)
	_ = cleanup.Run()
	create := exec.Command("ip", "netns", "add", name)
	if out, err := create.CombinedOutput(); err != nil {
		t.Fatalf("ip netns add %s: %v (%s)", name, err, out)
	}
	t.Cleanup(func() {
		_ = exec.Command("ip", "netns", "del", name).Run()
	})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	if err := fn(ctx); err != nil {
		t.Fatal(err)
	}
}

// nsExec runs ip <args...> inside the named namespace
// and returns combined output + exit code.
func nsExec(t *testing.T, ns string, args ...string) (string, error) {
	t.Helper()
	args = append([]string{"netns", "exec", ns}, args...)
	cmd := exec.Command("ip", args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// tcQdisc runs tc qdisc show dev <iface> inside the
// namespace and returns the parsed stdout.
func tcQdisc(t *testing.T, ns, iface string) string {
	t.Helper()
	out, err := nsExec(t, ns, "tc", "qdisc", "show", "dev", iface)
	if err != nil {
		t.Logf("tc qdisc show dev %s: %v (%s)", iface, err, out)
		return ""
	}
	return strings.TrimSpace(out)
}

// writeFile writes data to path inside ns.
func writeFile(t *testing.T, ns, path string, data []byte, mode os.FileMode) {
	t.Helper()
	if ns == "" {
		if err := os.WriteFile(path, data, mode); err != nil {
			t.Fatal(err)
		}
		return
	}
	cmd := exec.Command("ip", "netns", "exec", ns, "sh", "-c",
		"cat > "+shellQuote(path))
	cmd.Stdin = strings.NewReader(string(data))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("write %s in %s: %v (%s)", path, ns, err, out)
	}
	cmd = exec.Command("ip", "netns", "exec", ns, "chmod",
		octalMode(mode), path)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("chmod %s in %s: %v (%s)", path, ns, err, out)
	}
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}

func octalMode(m os.FileMode) string {
	return "0" + strings.TrimLeft(string([]byte{
		byte('0' + (m >> 6 & 7)),
		byte('0' + (m >> 3 & 7)),
		byte('0' + (m & 7)),
	}), "0")
}

// freePort returns a free loopback port. The caller is
// responsible for closing whatever binds it.
func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// requireXray locates the xray binary or skips.
func requireXray(t *testing.T) string {
	t.Helper()
	path := os.Getenv("XRAY_BIN")
	if path == "" {
		path = "/tmp/duckos-test/xray/xray"
	}
	if _, err := os.Stat(path); err != nil {
		t.Skipf("xray binary not at %s: %v", path, err)
	}
	return path
}

// waitFor polls fn every interval until it returns nil
// or timeout elapses.
func waitFor(t *testing.T, timeout, interval time.Duration, fn func() error) error {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if err := fn(); err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return errors.New("waitFor timeout")
		}
		time.Sleep(interval)
	}
}

// tempPath returns a path under the system temp dir
// that the test fully owns and cleans up.
func tempPath(t *testing.T, name string) string {
	t.Helper()
	dir := filepath.Join(os.TempDir(), "duckos-layer-l", name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}
