package qa

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/CrazyDuckLab/DuckOS/internal/support"
)

var _ = json.Marshal

// TestSupportBundleNoCanaryLeak seeds one canary per
// critical subsystem (vault, xray, wireguard, acme, radius,
// api-token) into a representative support bundle and
// confirms the default redaction set strips every canary
// value and marker from each file marked Redact=true.
//
// This is the §16/§42 baseline: a unit-test of the
// redaction contract, run on every CI, on Darwin, and on
// duck-dev-01. It is the cheap, deterministic layer; a
// real lab canary grep is layered on top of this in §49.
func TestSupportBundleNoCanaryLeak(t *testing.T) {
	vault := NewCanary("vault")
	xray := NewCanary("xray")
	wg := NewCanary("wireguard")
	acme := NewCanary("acme")
	radius := NewCanary("radius")
	api := NewCanary("api-token")
	canaries := []Canary{vault, xray, wg, acme, radius, api}

	bb := support.NewBundleBuilder("host-qa", "v1.0.0-qa", support.NewRedactor(support.DefaultPatterns()), nil)
	bb.Add(support.File{Path: "/etc/duckos/vault/keys.conf", Redact: true,
		Body: []byte("vault master=" + vault.Value + "\ncipher=chacha20poly1305")})
	bb.Add(support.File{Path: "/etc/xray/config.json", Redact: true,
		Body: []byte("{\"token\":\"" + xray.Value + "\",\"port\":443}")})
	bb.Add(support.File{Path: "/etc/wireguard/wg0.conf", Redact: true,
		Body: []byte("[Interface]\nPrivateKey = " + wg.Value + "\nAddress = 10.0.0.1/24")})
	bb.Add(support.File{Path: "/var/lib/duckos/acme/account.json", Redact: true,
		Body: []byte("{\"contact\":\"mailto:qa@duckos\",\"token\":\"" + acme.Value + "\"}")})
	bb.Add(support.File{Path: "/etc/freeradius/clients.conf", Redact: true,
		Body: []byte("secret = " + radius.Value + "\nshortname = duckos")})
	bb.Add(support.File{Path: "/etc/duckos/api/tokens.json", Redact: true,
		Body: []byte("[{\"id\":\"admin\",\"token\":\"" + api.Value + "\"}]")})
	bb.Add(support.File{Path: "/etc/duckos/version.txt", Redact: false,
		Body: []byte("duckos v1.0.0-qa")})

	bundle := bb.Build()

	if len(bundle.Files) != 7 {
		t.Fatalf("expected 7 files, got %d", len(bundle.Files))
	}

	for _, f := range bundle.Files {
		body := string(f.Body)
		// None of the redacted files may contain a canary
		// value or marker. Plain files also must not, in
		// this test, because we never put canaries there.
		if ContainsLeak(body, canaries...) {
			t.Fatalf("LEAK in %s:\n%s", f.Path, body)
		}
	}

	// Plain file is preserved verbatim.
	for _, f := range bundle.Files {
		if !f.Redact && !strings.Contains(string(f.Body), "duckos v1.0.0-qa") {
			t.Fatalf("plain file mangled: %s -> %q", f.Path, f.Body)
		}
	}

	// Bundle JSON itself must not leak.
	jb, err := json.Marshal(bundle)
	if err != nil {
		t.Fatalf("bundle json: %v", err)
	}
	if ContainsLeak(string(jb), canaries...) {
		t.Fatalf("LEAK in bundle JSON: %s", jb)
	}
}

// TestSupportBundleRedactionPreservesPlainContent is a
// negative control: non-secret data must survive redaction
// untouched, so the bundle remains useful for diagnostics.
func TestSupportBundleRedactionPreservesPlainContent(t *testing.T) {
	bb := support.NewBundleBuilder("h", "v", support.NewRedactor(support.DefaultPatterns()), nil)
	body := "version: duckos v1.0.0\nuptime: 12345s\ninterface: eth0 172.18.4.149/16"
	bb.Add(support.File{Path: "/status.txt", Redact: true, Body: []byte(body)})
	out := bb.Build()
	if !strings.Contains(string(out.Files[0].Body), "eth0 172.18.4.149/16") {
		t.Fatalf("plain content over-redacted: %q", out.Files[0].Body)
	}
}

// TestSupportBundleRedactsRealWorldSecretPrefixes seeds
// realistic secret formats (sk_live_, ghp_, AIza, AWS) and
// asserts that no plaintext prefix or value remains in the
// bundle.
//
// This test is expected to FAIL on the current code, which
// only redacts the secret value and not the issuer prefix.
// The failure documents finding M-1 in the QA report:
// the default redaction pattern set does not handle the
// common "issuer-prefix + secret" shape that nearly every
// modern credential uses. Once the patterns are extended
// (or a context-aware redaction pass is added), the test
// should pass and stay passing as a regression guard.
func TestSupportBundleRedactsRealWorldSecretPrefixes(t *testing.T) {
	cases := []struct {
		name, body string
	}{
		{"stripe-live", "STRIPE_SECRET_KEY=sk_live_" + strings.Repeat("a", 40)},
		{"github-pat", "GITHUB_TOKEN=ghp_" + strings.Repeat("B", 36)},
		{"google-api", "GOOGLE_API_KEY=AIza" + strings.Repeat("c", 35)},
		{"aws-secret", "AWS_SECRET_ACCESS_KEY=wJalrXUtnFEMI/K7MDENG/bPxRfiCYz" + strings.Repeat("Z", 20)},
		{"slack-bot", "SLACK_BOT_TOKEN=xoxb-" + strings.Repeat("0", 10) + "-" + strings.Repeat("1", 10) + "-" + strings.Repeat("2", 20)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bb := support.NewBundleBuilder("h", "v", support.NewRedactor(support.DefaultPatterns()), nil)
			bb.Add(support.File{Path: "/env", Redact: true, Body: []byte(tc.body)})
			out := bb.Build()
			body := string(out.Files[0].Body)
			if strings.Contains(body, tc.body) {
				t.Fatalf("real-world secret not redacted: %q -> %q", tc.body, body)
			}
		})
	}
}
