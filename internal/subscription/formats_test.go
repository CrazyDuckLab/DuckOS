// Package subscription / formats_test.go: verified output formats —
// base64 round-trips, Clash YAML safety and structure, JSON export,
// refusal of unsupported or silently-broken entries.
package subscription

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func sampleProxies() []Proxy {
	return []Proxy{
		{
			Name: "nl-1", Protocol: "vless", Host: "203.0.113.10", Port: 443,
			UUID: "11111111-1111-1111-1111-111111111111", Flow: "xtls-rprx-vision",
			Transport: "tcp", Security: "reality", SNI: "yahoo.com",
			Fingerprint: "chrome", PublicKey: "PUBKEY", ShortID: "0123456789abcdef",
		},
		{
			Name: "de-1", Protocol: "vmess", Host: "203.0.113.11", Port: 8443,
			UUID:      "22222222-2222-2222-2222-222222222222",
			Transport: "ws", Security: "tls", SNI: "cdn.example.com",
			Path: "/wss", HeaderHost: "cdn.example.com",
		},
		{
			Name: "se-1", Protocol: "trojan", Host: "203.0.113.12", Port: 443,
			Password: "hunter2+pass word", Transport: "grpc", Security: "tls",
			ServiceName: "duckos-se",
		},
		{
			Name: "fi-1", Protocol: "ss", Host: "203.0.113.13", Port: 8388,
			Cipher: "2022-blake3-aes-256-gcm", Password: "PSKVALUE",
			Transport: "tcp", Security: "none",
		},
	}
}

func TestRenderBase64RoundTrip(t *testing.T) {
	out, err := Render(FormatBase64, sampleProxies())
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := base64.StdEncoding.DecodeString(string(out))
	if err != nil {
		t.Fatalf("payload is not base64: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(decoded)), "\n")
	if len(lines) != 4 {
		t.Fatalf("line count %d: %s", len(lines), decoded)
	}
	if !strings.HasPrefix(lines[0], "vless://") ||
		!strings.Contains(lines[0], "security=reality") ||
		!strings.Contains(lines[0], "pbk=PUBKEY") ||
		!strings.Contains(lines[0], "flow=xtls-rprx-vision") {
		t.Fatalf("vless line: %s", lines[0])
	}
	if !strings.HasPrefix(lines[1], "vmess://") {
		t.Fatalf("vmess line: %s", lines[1])
	}
	if !strings.HasPrefix(lines[2], "trojan://") {
		t.Fatalf("trojan line: %s", lines[2])
	}
	// Password with space and plus must arrive percent-decoded intact.
	if !strings.Contains(lines[2], "hunter2%2Bpass%20word") {
		t.Fatalf("trojan credential escaping: %s", lines[2])
	}
	if !strings.HasPrefix(lines[3], "ss://") {
		t.Fatalf("ss line: %s", lines[3])
	}
}

func TestRenderClashYAMLStructure(t *testing.T) {
	out, err := Render(FormatClash, sampleProxies())
	if err != nil {
		t.Fatal(err)
	}
	text := string(out)
	for _, want := range []string{
		"proxies:",
		`- name: "nl-1"`,
		`type: "vless"`,
		`flow: "xtls-rprx-vision"`,
		"reality-opts:",
		"public-key: \"PUBKEY\"",
		"short-id: \"0123456789abcdef\"",
		`type: "vmess"`,
		`type: "trojan"`,
		`password: "hunter2+pass word"`,
		`type: "ss"`,
		"cipher: \"2022-blake3-aes-256-gcm\"",
		"proxy-groups:",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("clash output missing %q", want)
		}
	}
	// ws-opts nested mapping present for the vmess proxy.
	if !strings.Contains(text, "ws-opts:") || !strings.Contains(text, "Host: \"cdn.example.com\"") {
		t.Errorf("clash ws-opts missing")
	}
}

func TestRenderClashYAMLInjectionSafety(t *testing.T) {
	evil := []Proxy{{
		Name:     "x\"\n  type: evil-injected\ny: \"",
		Protocol: "ss", Host: "203.0.113.9", Port: 8388,
		Cipher: "aes-256-gcm", Password: "p\nassword",
		Transport: "tcp", Security: "none",
	}}
	out, err := Render(FormatClash, evil)
	if err != nil {
		t.Fatal(err)
	}
	// The hostile content must stay inside a single escaped scalar:
	// no real line may carry an injected mapping key.
	for _, line := range strings.Split(string(out), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "type: evil-injected" || strings.HasPrefix(trimmed, "y:") {
			t.Fatalf("YAML injection escaped the scalar context:\n%s", out)
		}
	}
}

func TestRenderJSONExport(t *testing.T) {
	out, err := Render(FormatJSON, sampleProxies())
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Version int `json:"version"`
		Proxies []struct {
			Name string `json:"name"`
			Port int    `json:"port"`
		} `json:"proxies"`
	}
	if err := json.Unmarshal(out, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Version != 1 || len(doc.Proxies) != 4 {
		t.Fatalf("json doc: %+v", doc)
	}
}

func TestRenderRefusals(t *testing.T) {
	if _, err := Render(Format("yaml2"), sampleProxies()); err == nil {
		t.Fatal("unknown format accepted")
	}
	dup := sampleProxies()
	dup[1].Name = dup[0].Name
	if _, err := Render(FormatClash, dup); err == nil {
		t.Fatal("duplicate names accepted")
	}
	broken := []Proxy{{Name: "x", Protocol: "vless", Host: "h", Port: 443, Transport: "tcp", Security: "none"}}
	if _, err := Render(FormatBase64, broken); err == nil {
		t.Fatal("proxy without uuid accepted")
	}
	empty := []Proxy{}
	if _, err := Render(FormatPlain, empty); err == nil {
		t.Fatal("empty proxy set accepted")
	}
}

func TestDeliveryFormatParam(t *testing.T) {
	store := NewTokenStore()
	clashPayload := []byte("proxies: []\n")
	h := NewDeliveryHandler(store, func(subID string) ([]byte, error) {
		return []byte("YmFzZTY0"), nil
	})
	h.RenderFormat = func(subID string, format Format) ([]byte, error) {
		if format == FormatClash {
			return clashPayload, nil
		}
		return nil, errorsNew("bad format")
	}
	tok, _, _ := store.Issue("live-sub", time.Hour)
	// Default stays base64 content type.
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/sub/"+tok, nil))
	if w.Code != http.StatusOK || w.Header().Get("Content-Type") != "text/plain; charset=utf-8" {
		t.Fatalf("default format: %d %s", w.Code, w.Header().Get("Content-Type"))
	}
	// Known format routes to RenderFormat.
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/sub/"+tok+"?format=clash", nil))
	if w.Code != http.StatusOK || w.Header().Get("Content-Type") != "text/yaml; charset=utf-8" {
		t.Fatalf("clash format: %d %s", w.Code, w.Header().Get("Content-Type"))
	}
	// Unknown format is the uniform 404.
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/sub/"+tok+"?format=powershell", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("unknown format: %d", w.Code)
	}
}
