// Package xraycore performs genuine runtime qualification against
// the actual pinned Xray-core binary (26.3.27).
//
// Every test in this package SKIPS when the binary is absent, so
// CI and non-lab hosts stay green; where it runs, the results are
// real execution evidence, not mocks: `xray run -test` validation,
// listener startup, client connections through a live proxy,
// traffic forwarding to a local target, stats collection through
// `xray api statsquery`, authentication refusals, and invalid-
// config rejection.
//
// Point XRAY_CENTER_XRAY_BIN at the binary to enable. On the
// development host the checked-out toolchain lives at
// .tools/xray-26.3.27/xray (darwin/arm64 build of the SAME pinned
// version; the linux deployment binaries carry per-arch sha256
// pins in scripts/stage_xray.sh).
package xraycore

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/xray"
)

const pinnedVersion = "26.3.27"

func findBinary(t *testing.T) string {
	t.Helper()
	if v := os.Getenv("XRAY_CENTER_XRAY_BIN"); v != "" {
		if _, err := os.Stat(v); err == nil {
			return v
		}
		t.Skipf("XRAY_CENTER_XRAY_BIN=%s does not exist", v)
	}
	candidates := []string{
		filepath.Join("..", "..", ".tools", "xray-"+pinnedVersion, "xray"),
		filepath.Join("..", "..", "..", ".tools", "xray-"+pinnedVersion, "xray"),
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	t.Skip("xray binary not available; runtime qualification skipped (unit-only host)")
	return ""
}

func xrayVersion(t *testing.T, bin string) string {
	t.Helper()
	out, err := exec.Command(bin, "version").Output()
	if err != nil {
		t.Fatalf("xray version: %v", err)
	}
	return string(out)
}

// xrayTest runs `xray run -test -c <file>` and returns the error,
// with bounded output for diagnostics.
func xrayTest(t *testing.T, bin string, cfg []byte) error {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "candidate.json")
	if err := os.WriteFile(path, cfg, 0o600); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, bin, "run", "-test", "-c", path).CombinedOutput()
	if err != nil {
		msg := string(out)
		if len(msg) > 2000 {
			msg = msg[:2000]
		}
		return fmt.Errorf("xray -test rejected: %v: %s", err, msg)
	}
	return nil
}

// matrixCase is one protocol/transport/security combination under
// qualification.
type matrixCase struct {
	name    string
	server  xray.Server
	user    xray.User
	reality bool
	tls     bool
}

func matrixCases() []matrixCase {
	uid := func(n string) string { return n }
	u := "11111111-1111-1111-1111-111111111111"
	mk := func(id, name string, proto xray.Protocol, transport xray.Transport, sec xray.Security) matrixCase {
		port := 10000 + len(id)%20000
		c := matrixCase{
			name: id,
			server: xray.Server{
				ID: uid(pad32(id)), Name: name, Protocol: proto,
				Listen: fmt.Sprintf("127.0.0.1:%d", port), Port: port,
				Transport: transport, Security: sec, Enabled: true,
			},
			user: xray.User{ID: uid(pad32(id + "-u")), ServerID: uid(pad32(id)), Name: name + "-u",
				UUID: u, Password: "test-password", Email: name + "@example.com", Enabled: true},
		}
		if proto == xray.ProtocolShadowsocks {
			c.server.SSMethod = "aes-256-gcm"
		}
		return c
	}
	protocols := []xray.Protocol{xray.ProtocolVLESS, xray.ProtocolVMess, xray.ProtocolTrojan, xray.ProtocolShadowsocks}
	transports := []xray.Transport{xray.TransportTCP, xray.TransportWeb, xray.TransportGRPC,
		xray.TransportXHTTP, xray.TransportHTTPUpgrade, xray.TransportKCP}
	var out []matrixCase
	for _, p := range protocols {
		for _, tr := range transports {
			out = append(out, mk(fmt.Sprintf("%s-%s-none", p, tr), fmt.Sprintf("%s%s", p, tr), p, tr, xray.SecurityNone))
		}
	}
	// REALITY: vless over tcp and xhttp.
	for _, tr := range []xray.Transport{xray.TransportTCP, xray.TransportXHTTP} {
		c := mk(fmt.Sprintf("vless-%s-reality", tr), "reality"+string(tr), xray.ProtocolVLESS, tr, xray.SecurityReality)
		c.reality = true
		out = append(out, c)
	}
	// TLS: vless tcp/ws with a generated certificate.
	for _, tr := range []xray.Transport{xray.TransportTCP, xray.TransportWeb} {
		c := mk(fmt.Sprintf("vless-%s-tls", tr), "tls"+string(tr), xray.ProtocolVLESS, tr, xray.SecurityTLS)
		c.tls = true
		out = append(out, c)
	}
	// SOCKS / HTTP inbounds.
	out = append(out, mk("socks-none", "socks", xray.ProtocolSOCKS, xray.TransportTCP, xray.SecurityNone))
	out = append(out, mk("http-none", "http", xray.ProtocolHTTP, xray.TransportTCP, xray.SecurityNone))
	return out
}

// pad32 derives a deterministic 32-hex id from a label.
func pad32(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])[:32]
}

// TestRuntime_ConfigValidationMatrix validates every supported
// protocol/transport/security combination with `xray run -test`.
func TestRuntime_ConfigValidationMatrix(t *testing.T) {
	bin := findBinary(t)
	if !strings.Contains(xrayVersion(t, bin), pinnedVersion) {
		t.Fatalf("binary is not the pinned %s", pinnedVersion)
	}
	kp, err := xray.GenerateRealityKeypair()
	if err != nil {
		t.Fatal(err)
	}
	sid, _ := xray.GenerateShortID()
	resolver := stubResolverFor(kp.PrivateKey, sid)
	var failed []string
	for _, c := range matrixCases() {
		c := c
		t.Run(c.name, func(t *testing.T) {
			if c.reality {
				c.server.RealityDest = "yahoo.com:443"
				c.server.RealitySNI = []string{"yahoo.com"}
				c.server.Fingerprint = "chrome"
				c.server.RealityPublicKey = kp.PublicKey
				c.server.RealityKeyRef = &xray.KeyRef{VaultID: pad32("v"), Revision: pad32("r"), KeyID: pad32("priv")}
				c.server.ShortIDRef = &xray.KeyRef{VaultID: pad32("v"), Revision: pad32("r"), KeyID: pad32("sid")}
			}
			if c.tls {
				c.server.TLSCertRef = &xray.KeyRef{VaultID: pad32("v"), Revision: pad32("r"), KeyID: pad32("crt")}
				c.server.TLSKeyRef = &xray.KeyRef{VaultID: pad32("v"), Revision: pad32("r"), KeyID: pad32("key")}
			}
			if err := c.server.Validate(); err != nil {
				t.Fatalf("model rejected the case: %v", err)
			}
			stage := t.TempDir()
			oldStage := xray.StageDir
			xray.StageDir = stage
			defer func() { xray.StageDir = oldStage }()
			rendered, rerr := renderForTest([]xray.Server{c.server}, []xray.User{c.user})
			if rerr != nil {
				t.Fatalf("render: %v", rerr)
			}
			if _, err := xray.Materialize(context.Background(), rendered, stage,
				[]xray.Server{c.server}, []xray.User{c.user}, resolver); err != nil {
				t.Fatalf("materialize: %v", err)
			}
			if err := xrayTest(t, bin, mustJSON(t, rendered)); err != nil {
				failed = append(failed, c.name)
				t.Errorf("%v", err)
			}
		})
	}
	if len(failed) > 0 {
		t.Errorf("combos rejected by xray -test: %v", failed)
	}
}

// renderForTest renders one inbound with its user.
func renderForTest(servers []xray.Server, users []xray.User) (xray.Config, error) {
	return xray.RenderConfig(servers, users, nil, nil)
}

func mustJSON(t *testing.T, cfg xray.Config) []byte {
	t.Helper()
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// selfSignedDER generates a throwaway ECDSA P-256 TLS pair.
func selfSignedDER() ([]byte, []byte, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "duckos-test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, nil, err
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, nil, err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	return certPEM, keyPEM, nil
}

// stubResolverFor resolves the fixed test key ids.
type stubResolver struct{ priv, sid string }

func (s stubResolver) Resolve(_ context.Context, ref xray.KeyRef) ([]byte, error) {
	// The production resolver returns RAW bytes: the REALITY
	// private key as 32 raw bytes (the materializer applies the
	// URL-safe base64 itself). The stub mirrors that contract.
	switch {
	case ref.KeyID == pad32("priv"):
		raw, err := base64.RawURLEncoding.DecodeString(s.priv)
		if err != nil {
			return nil, err
		}
		return raw, nil
	case ref.KeyID == pad32("sid"):
		return hex.DecodeString(s.sid)
	case ref.KeyID == pad32("crt"), ref.KeyID == pad32("key"):
		cert, key, err := selfSignedDER()
		if err != nil {
			return nil, err
		}
		if ref.KeyID == pad32("crt") {
			return cert, nil
		}
		return key, nil
	}
	return nil, errors.New("unknown key")
}

func stubResolverFor(priv, sid string) stubResolver { return stubResolver{priv: priv, sid: sid} }

// TestRuntime_SOCKSLoopTraffic runs a live xray with a SOCKS inbound
// and forwards real HTTP traffic through it to a local target, then
// observes the traffic through `xray api statsquery`.
func TestRuntime_SOCKSLoopTraffic(t *testing.T) {
	bin := findBinary(t)
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "duckos-loop-ok")
	}))
	defer target.Close()
	port := freePort(t)
	apiPort := freePort(t)
	cfg := map[string]any{
		"log":    map[string]any{"loglevel": "warning"},
		"stats":  map[string]any{},
		"policy": map[string]any{"levels": map[string]any{"0": map[string]any{"statsUserUplink": true, "statsUserDownlink": true, "statsInboundUplink": true, "statsInboundDownlink": true}}},
		"api":    map[string]any{"tag": "api", "services": []string{"StatsService"}},
		"inbounds": []map[string]any{
			{"tag": "in-socks", "listen": "127.0.0.1", "port": port, "protocol": "socks",
				"settings": map[string]any{"auth": "password", "udp": false,
					"accounts": []map[string]string{{"user": "bob", "pass": "loop-secret"}}}},
			{"tag": "api", "listen": "127.0.0.1", "port": apiPort, "protocol": "dokodemo-door",
				"settings": map[string]any{"address": "127.0.0.1"}},
		},
		"outbounds": []map[string]any{{"tag": "direct", "protocol": "freedom"}},
		"routing": map[string]any{"rules": []map[string]any{
			{"type": "field", "inboundTag": []string{"api"}, "outboundTag": "api"},
		}},
	}
	raw, _ := json.Marshal(cfg)
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "loop.json")
	if err := os.WriteFile(cfgPath, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bin, "run", "-c", cfgPath)
	cmd.Stdout, cmd.Stderr = nil, nil
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}()
	if !waitTCP(t, port, 10*time.Second) {
		t.Fatal("xray socks listener did not come up")
	}
	// Forward real traffic through the proxy via curl.
	out, err := exec.Command("curl", "-s", "--max-time", "10",
		"--socks5-hostname", "bob:loop-secret@127.0.0.1:"+fmt.Sprint(port), target.URL).Output()
	if err != nil || !strings.Contains(string(out), "duckos-loop-ok") {
		t.Fatalf("proxied request failed: %v %s", err, out)
	}
	// Observe the traffic through the stats service.
	observed := false
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		q, err := exec.Command(bin, "api", "statsquery", "--server=127.0.0.1:"+fmt.Sprint(apiPort)).Output()
		if err == nil {
			snap, perr := xray.ParseStatsQueryOutput(q)
			if perr == nil {
				// Xray 26.3.27 (verified live): per-USER counters
				// are reported with values; per-inbound counters
				// stay absent under policy.levels settings. Quota
				// enforcement keys on user counters, so does the
				// assertion.
				for _, c := range snap.Counters {
					if strings.HasPrefix(c.Name, "user>>>bob>>>traffic>>>") && c.Value > 0 {
						observed = true
					}
				}
			}
		}
		if observed {
			break
		}
		time.Sleep(300 * time.Millisecond)
	}
	if !observed {
		t.Fatal("traffic was forwarded but the stats service did not observe it")
	}
}

// TestRuntime_AuthReject proves a wrong SOCKS credential cannot
// establish a session.
func TestRuntime_AuthReject(t *testing.T) {
	bin := findBinary(t)
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer target.Close()
	port := freePort(t)
	cfg := map[string]any{
		"log": map[string]any{"loglevel": "warning"},
		"inbounds": []map[string]any{
			{"tag": "in-socks", "listen": "127.0.0.1", "port": port, "protocol": "socks",
				"settings": map[string]any{"auth": "password", "udp": false,
					"accounts": []map[string]string{{"user": "bob", "pass": "right"}}}},
		},
		"outbounds": []map[string]any{{"tag": "direct", "protocol": "freedom"}},
	}
	raw, _ := json.Marshal(cfg)
	cfgPath := filepath.Join(t.TempDir(), "auth.json")
	_ = os.WriteFile(cfgPath, raw, 0o600)
	cmd := exec.Command(bin, "run", "-c", cfgPath)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill(); _, _ = cmd.Process.Wait() }()
	if !waitTCP(t, port, 10*time.Second) {
		t.Fatal("listener did not come up")
	}
	out, err := exec.Command("curl", "-s", "--max-time", "8",
		"--socks5-hostname", "bob:wrong@127.0.0.1:"+fmt.Sprint(port), target.URL).Output()
	if err == nil {
		t.Fatalf("wrong credential was accepted: %s", out)
	}
}

// TestRuntime_InvalidConfigRejected pins the rejection path.
func TestRuntime_InvalidConfigRejected(t *testing.T) {
	bin := findBinary(t)
	bad := map[string]any{"inbounds": []map[string]any{{"port": "not-a-port"}}}
	raw, _ := json.Marshal(bad)
	if err := xrayTest(t, bin, raw); err == nil {
		t.Fatal("invalid config accepted by xray -test")
	}
}

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func waitTCP(t *testing.T, port int, timeout time.Duration) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		c, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 500*time.Millisecond)
		if err == nil {
			_ = c.Close()
			return true
		}
		time.Sleep(150 * time.Millisecond)
	}
	return false
}
