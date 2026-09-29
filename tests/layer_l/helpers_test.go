//go:build linux

// helpers_test.go provides the shared helpers used by
// every dvXX_l_test.go file in this package. Build tag
// matches the L-tier test files.

package layerl

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"
)

// timeNow returns UTC now for fixture timestamps.
func timeNow() time.Time { return time.Now().UTC() }

// testCtx returns a fresh background context.
func testCtx() context.Context { return context.Background() }

// run is a thin wrapper around exec.Command that returns
// combined output + error.
func run(t interface {
	Helper()
}, name string, args ...string) (string, error) {
	t.Helper()
	c := exec.Command(name, args...)
	out, err := c.CombinedOutput()
	return string(out), err
}

// randomSuffix returns 8 hex chars; suitable for unique
// fixture names without colliding.
func randomSuffix() string {
	var b [4]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// sha256File returns the hex sha256 of path.
func sha256File(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:]), nil
}

// buildSignedBinary writes a synthetic signed binary
// fixture under dir. The signature is the sha256 of the
// payload; the test asserts the orchestrator accepts
// the signature.
func buildSignedBinary(t *testing.T, dir, name string) string {
	t.Helper()
	payload := make([]byte, 4096)
	for i := range payload {
		payload[i] = byte(i & 0xFF)
	}
	h := sha256.Sum256(payload)
	sig := hex.EncodeToString(h[:])
	bin := filepathJoin(dir, name)
	if err := osWriteFile(bin, append([]byte("# duckos L-tier signed binary\n# sig="+sig+"\n"), payload...)); err != nil {
		t.Fatal(err)
	}
	_ = os.Chmod(bin, 0o755)
	return bin
}

// filepathJoin is a tiny wrapper for tests that need
// it without polluting many files.
func filepathJoin(a, b string) string { return a + "/" + b }

// osWriteFile writes data to path with 0600 mode.
func osWriteFile(path string, data []byte) error { return os.WriteFile(path, data, 0o600) }

// readFileImpl reads path.
func readFileImpl(path string) ([]byte, error) { return os.ReadFile(path) }

// writeFileImpl writes data to path.
func writeFileImpl(path string, data []byte) error { return os.WriteFile(path, data, 0o600) }

// osRemove removes path.
func osRemove(path string) error { return os.Remove(path) }

// filepathExists reports whether path exists.
func filepathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// waitForFile polls path every 100ms until it contains
// needle, or returns an error after timeout seconds.
func waitForFile(path, needle string, timeoutSeconds int) error {
	deadline := time.Now().Add(time.Duration(timeoutSeconds) * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(path)
		if err == nil && bytes.Contains(data, []byte(needle)) {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("waitForFile: %s not found in %s within %ds",
		needle, path, timeoutSeconds)
}
