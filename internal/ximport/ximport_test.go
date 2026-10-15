// Package ximport tests: 3x-ui migration mapping and subscription
// fetch security (SSRF, private-address refusal, size caps).
package ximport

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/xray"
)

func threeUIVLESSRealityExport() string {
	return `{"inbounds":[{` +
		`"id":1,"port":443,"protocol":"vless","remark":"Main VLESS","enable":true,"listen":"",` +
		`"settings":"{\"clients\":[{\"id\":\"8f2f8f2f-8f2f-8f2f-8f2f-8f2f8f2f8f2f\",\"flow\":\"xtls-rprx-vision\",\"email\":\"alice@example.com\"}],\"decryption\":\"none\",\"fingerprint\":\"chrome\"}",` +
		`"streamSettings":"{\"network\":\"tcp\",\"security\":\"reality\",\"realitySettings\":{\"show\":false,\"dest\":\"yahoo.com:443\",\"xver\":0,\"serverNames\":[\"yahoo.com\"],\"privateKey\":\"supersecretprivatekey\",\"shortIds\":[\"abcdef1234567890\"],\"settings\":{\"publicKey\":\"PUBKEY123\",\"fingerprint\":\"chrome\"}}}",` +
		`"sniffing":{"enabled":true}}]}`
}

func TestParseThreeUIExport_VLESSReality(t *testing.T) {
	preview, err := ParseThreeUIExport([]byte(threeUIVLESSRealityExport()))
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Servers) != 1 {
		t.Fatalf("servers %d unsupported %v", len(preview.Servers), preview.Unsupported)
	}
	s := preview.Servers[0]
	if s.Protocol != xray.ProtocolVLESS || s.Security != xray.SecurityReality || s.Port != 443 {
		t.Fatalf("server mapping: %+v", s)
	}
	if s.RealityDest != "yahoo.com:443" || len(s.RealitySNI) != 1 || s.RealitySNI[0] != "yahoo.com" {
		t.Fatalf("reality mapping: %+v", s)
	}
	if strings.Contains(s.RealityDest, "supersecretprivatekey") {
		t.Fatal("private key leaked into canonical draft")
	}
	foundPrivWarn := false
	for _, w := range preview.Warnings {
		if strings.Contains(w, "private key") {
			foundPrivWarn = true
		}
	}
	if !foundPrivWarn {
		t.Fatalf("private-key handling warning missing: %v", preview.Warnings)
	}
	if len(preview.Users) != 1 || preview.Users[0].Flow != xray.FlowVision {
		t.Fatalf("user mapping: %+v", preview.Users)
	}
	if preview.Servers[0].Name != "main-vless" {
		t.Fatalf("draft name %q", preview.Servers[0].Name)
	}
}

func TestParseThreeUIExport_UnsupportedReported(t *testing.T) {
	raw := `{"inbounds":[{"id":2,"port":9001,"protocol":"wireguard","remark":"wg"}]}`
	preview, err := ParseThreeUIExport([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Servers) != 0 || len(preview.Unsupported) != 1 {
		t.Fatalf("expected refusal, got servers=%d unsupported=%v", len(preview.Servers), preview.Unsupported)
	}
}

func TestParseThreeUIExport_SplithttpMapping(t *testing.T) {
	raw := `{"inbounds":[{"id":3,"port":2096,"protocol":"vmess","remark":"legacy http","listen":"",` +
		`"settings":"{\"clients\":[{\"id\":\"8f2f8f2f-8f2f-8f2f-8f2f-8f2f8f2f8f2f\",\"email\":\"bob\"}]}",` +
		`"streamSettings":"{\"network\":\"splithttp\",\"security\":\"none\"}"}]}`
	preview, err := ParseThreeUIExport([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Servers) != 1 || preview.Servers[0].Transport != xray.TransportXHTTP {
		t.Fatalf("splithttp mapping: %+v", preview.Servers)
	}
	mapped := false
	for _, w := range preview.Warnings {
		if strings.Contains(w, "xhttp") {
			mapped = true
		}
	}
	if !mapped {
		t.Fatalf("legacy transport warning missing: %v", preview.Warnings)
	}
}

func TestParseThreeUIExport_EmptyAndHuge(t *testing.T) {
	if _, err := ParseThreeUIExport(nil); err == nil {
		t.Fatal("empty export accepted")
	}
	huge := strings.Repeat(" ", MaxImportBytes+10)
	if _, err := ParseThreeUIExport([]byte(huge)); err == nil {
		t.Fatal("oversized export accepted")
	}
}

func TestFetchSubscription_RejectsNonHTTPSAndInternal(t *testing.T) {
	if _, _, err := FetchSubscription(context.Background(), "http://example.com/sub", DefaultFetchLimits()); err == nil {
		t.Fatal("http accepted")
	}
	for _, target := range []string{
		"https://127.0.0.1/sub",
		"https://10.0.0.1/sub",
		"https://192.168.1.1/sub",
		"https://169.254.169.254/latest/meta-data/",
		"https://[::1]/sub",
	} {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_, _, err := FetchSubscription(ctx, target, FetchLimits{Timeout: 2 * time.Second, MaxBytes: MaxSubscriptionBytes, Redirects: 0})
		cancel()
		if err == nil {
			t.Fatalf("internal target %s was fetched", target)
		}
	}
}

func TestParseSubscriptionPayload(t *testing.T) {
	good := "vless://11111111-1111-1111-1111-111111111111@example.com:443?type=tcp&security=tls#one"
	payload := good + "\n" + good + "\n# comment\nnot-a-uri\n"
	encoded := base64.StdEncoding.EncodeToString([]byte(payload))
	conn, skipped, err := ParseSubscriptionPayload(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if len(conn) != 1 || skipped == nil || len(skipped) < 2 {
		t.Fatalf("conn=%d skipped=%v", len(conn), skipped)
	}
	if conn[0].Protocol != xray.ProtocolVLESS {
		t.Fatalf("parsed protocol %q", conn[0].Protocol)
	}
	if _, _, err := ParseSubscriptionPayload(base64.StdEncoding.EncodeToString([]byte("garbage"))); err == nil {
		t.Fatal("garbage payload accepted")
	}
}
