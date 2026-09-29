//go:build linux

// Package qa: real hostapd non-radio qualification for
// §7 of the Reality Check program.
//
// hostapd 2.10 has no built-in test mode; "-t" is just
// "include timestamps in debug messages", not a dry run.
// We therefore launch hostapd with a context timeout and
// classify the outcome:
//   - non-zero exit with "errors found" -> invalid config
//   - clean exit (e.g. parse error before bring-up) -> invalid
//   - still running after timeout with AP-ENABLED and no
//     "errors found" -> config parsed cleanly
//
// No radio association is performed (HARDWARE-PENDING).
package qa

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/hostapd"
)

// findHostapd returns the path to a real hostapd binary.
func findHostapd(t testing.TB) string {
	t.Helper()
	if p := os.Getenv("HOSTAPD_BIN"); p != "" {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	for _, p := range []string{"/usr/sbin/hostapd", "/usr/local/sbin/hostapd"} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// writeRawHostapdConf writes a hand-rolled hostapd.conf to
// a temp file and returns its path. This bypasses the
// internal/hostapd validator and lets us feed a deliberately
// malformed configuration to the real hostapd binary to
// verify the daemon itself refuses it.
func writeRawHostapdConf(t testing.TB, dir, body string) string {
	t.Helper()
	confPath := filepath.Join(dir, "hostapd-raw.conf")
	if err := os.WriteFile(confPath, []byte(body), 0o600); err != nil {
		t.Fatalf("write raw conf: %v", err)
	}
	return confPath
}

// hostapdOutcome describes the classification of one
// hostapd invocation.
type hostapdOutcome int

const (
	// hostapdAccepted means the daemon parsed the config
	// and reached AP-ENABLED with no "errors found" line.
	hostapdAccepted hostapdOutcome = iota
	// hostapdRejected means the daemon reported parse
	// errors and exited non-zero.
	hostapdRejected
	// hostapdHanged means the daemon neither rejected nor
	// reached AP-ENABLED within the timeout.
	hostapdHanged
)

// runHostapdCheck invokes hostapd against a config file
// with a context timeout and classifies the outcome.
// hostapd 2.10 has no built-in test mode; "-t" is just
// "include timestamps in debug messages", not a dry run.
// We classify:
//   - exit non-zero with "errors found" -> rejected
//   - exit non-zero without it           -> rejected
//   - AP-ENABLED before timeout, no errors -> accepted
//   - timeout without AP-ENABLED          -> hanged (failure)
func runHostapdCheck(t testing.TB, bin, confPath string, timeout time.Duration) (string, hostapdOutcome) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "-d", confPath)
	out, err := cmd.CombinedOutput()
	s := string(out)
	if ctx.Err() == context.DeadlineExceeded {
		if strings.Contains(s, "AP-ENABLED") && !strings.Contains(s, "errors found") {
			return s, hostapdAccepted
		}
		return s, hostapdHanged
	}
	if err != nil {
		// exec.ExitError (real non-zero exit) or other error.
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			t.Logf("hostapd exit: %v", err)
		}
		return s, hostapdRejected
	}
	// Clean exit before timeout: rare but possible.
	if strings.Contains(s, "AP-ENABLED") {
		return s, hostapdAccepted
	}
	return s, hostapdRejected
}

// TestHostapdRenderRealBinaryValidation is §7.1: a
// config produced by the hostapd package's Render is
// fed to a real hostapd 2.10 binary and must reach
// AP-ENABLED without "errors found" in the output.
//
// Driver is forced to "none" because we have no Wi-Fi
// NIC; without it nl80211 would try to bring up wlan0
// and fail. "driver=none" is the standard hostapd
// mechanism for validating a config without hardware,
// and it still exercises every directive we emit
// (security mode, BSSes, channels, PMF, etc.).
func TestHostapdRenderRealBinaryValidation(t *testing.T) {
	bin := findHostapd(t)
	if bin == "" {
		t.Skip("hostapd not present")
	}
	dir := t.TempDir()
	cfg := hostapd.Config{
		BSSes: []hostapd.BSS{
			{
				Name:       "qa-bss",
				Interface:  "wlan0",
				Driver:     "none",
				SSID:       "qa-test",
				Band:       hostapd.Band2G,
				Channel:    6,
				Security:   hostapd.SecWPA2Personal,
				Passphrase: "qa-secret-12345678",
			},
		},
	}
	rendered, err := hostapd.Render(cfg)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	confPath := filepath.Join(dir, "hostapd.conf")
	if err := os.WriteFile(confPath, []byte(rendered), 0o600); err != nil {
		t.Fatalf("write conf: %v", err)
	}
	out, outcome := runHostapdCheck(t, bin, confPath, 4*time.Second)
	t.Logf("hostapd output:\n%s", out)
	switch outcome {
	case hostapdAccepted:
		// ok
	case hostapdRejected:
		t.Fatalf("hostapd rejected the rendered config. output:\n%s", out)
	case hostapdHanged:
		t.Fatalf("hostapd did not reach AP-ENABLED within timeout. output:\n%s", out)
	}
}

// TestHostapdRefusesInvalidWPA is §7.2: a config that
// specifies a security mode without a password (or a
// password that's too short) is refused by hostapd. We
// bypass the internal/hostapd validator on purpose so
// the real daemon sees a deliberately malformed config
// and we can prove the daemon (not just our model) is
// the gatekeeper.
func TestHostapdRefusesInvalidWPA(t *testing.T) {
	bin := findHostapd(t)
	if bin == "" {
		t.Skip("hostapd not present")
	}
	dir := t.TempDir()
	// Hand-roll a WPA2-Personal config with no passphrase.
	// driver=none avoids nl80211 trying to bring up wlan0.
	raw := "interface=wlan0\ndriver=none\nssid=qa-no-pass\nhw_mode=g\nchannel=6\nwpa=2\nwpa_key_mgmt=WPA-PSK\nwpa_pairwise=CCMP\n"
	confPath := writeRawHostapdConf(t, dir, raw)
	out, outcome := runHostapdCheck(t, bin, confPath, 4*time.Second)
	t.Logf("hostapd invalid WPA output:\n%s", out)
	switch outcome {
	case hostapdRejected:
		// expected
	case hostapdAccepted:
		t.Fatalf("expected hostapd to REJECT the missing-password config; output:\n%s", out)
	case hostapdHanged:
		t.Fatalf("hostapd hung instead of rejecting; output:\n%s", out)
	}
	lower := strings.ToLower(out)
	if !strings.Contains(lower, "passphrase") && !strings.Contains(lower, "password") && !strings.Contains(lower, "psk") {
		t.Fatalf("expected hostapd to complain about missing PSK/passphrase; got: %s", out)
	}
}
