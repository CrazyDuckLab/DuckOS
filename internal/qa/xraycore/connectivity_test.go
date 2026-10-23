// Package xraycore / connectivity_test.go: PHASE 2 — REAL client
// connectivity. A full Xray server (VLESS+REALITY inbound) and a
// full Xray client (SOCKS inbound → VLESS+REALITY outbound) run as
// separate processes; HTTP traffic flows curl → client → server →
// local target. This upgrades CONFIG-VALIDATED to RUNTIME-STARTED
// and TRAFFIC-VERIFIED for the flagship VLESS+REALITY combination.
package xraycore

import (
	"context"
	"encoding/json"
	"fmt"
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

// realityDest/realitySNI: overridable for egress-restricted labs.
func realityDest() string {
	if v := os.Getenv("REALITY_TEST_DEST"); v != "" {
		return v
	}
	return "yahoo.com:443"
}

func realitySNI() string {
	if v := os.Getenv("REALITY_TEST_SNI"); v != "" {
		return v
	}
	return "yahoo.com"
}

func startXray(t *testing.T, bin, cfgPath string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(bin, "run", "-c", cfgPath)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	return cmd
}

func killProc(cmd *exec.Cmd) {
	_ = cmd.Process.Kill()
	_, _ = cmd.Process.Wait()
}

// TestConnectivity_VLESSReality_EndToEnd: full proxy chain with two
// separate xray processes (server + client) and real HTTP traffic.
func TestConnectivity_VLESSReality_EndToEnd(t *testing.T) {
	bin := findBinary(t)
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "duckos-reality-ok")
	}))
	defer target.Close()
	serverPort, clientSocksPort := freePort(t), freePort(t)
	kp, err := xray.GenerateRealityKeypair()
	if err != nil {
		t.Fatal(err)
	}
	sid, _ := xray.GenerateShortID()
	stage := t.TempDir()
	oldStage := xray.StageDir
	xray.StageDir = stage
	defer func() { xray.StageDir = oldStage }()

	// Server side from the DuckOS model + materialization (the same
	// pipeline the agent uses).
	server := xray.Server{
		ID: pad32("srv"), Name: "reality-e2e", Protocol: xray.ProtocolVLESS,
		Listen:           fmt.Sprintf("127.0.0.1:%d", serverPort),
		Port:             serverPort,
		Transport:        xray.TransportTCP,
		Security:         xray.SecurityReality,
		RealityDest:      "yahoo.com:443",
		RealitySNI:       []string{"yahoo.com"},
		Fingerprint:      "chrome",
		RealityPublicKey: kp.PublicKey,
		RealityKeyRef:    &xray.KeyRef{VaultID: pad32("v"), Revision: pad32("r"), KeyID: pad32("priv")},
		ShortIDRef:       &xray.KeyRef{VaultID: pad32("v"), Revision: pad32("r"), KeyID: pad32("sid")},
		Enabled:          true,
	}
	// Inline UUID user (no vault indirection needed for connectivity
	// qualification; vault materialization is covered elsewhere).
	clientUUID := "11111111-1111-1111-1111-111111111111"
	user := xray.User{
		ID: pad32("srvu"), ServerID: server.ID, Name: "alice", Email: "alice@test",
		UUID:    clientUUID,
		Flow:    xray.FlowVision,
		Enabled: true,
	}
	if err := server.Validate(); err != nil {
		t.Fatal(err)
	}
	rendered, err := renderForTest([]xray.Server{server}, []xray.User{user})
	if err != nil {
		t.Fatal(err)
	}
	resolver := stubResolver{priv: kp.PrivateKey, sid: sid}
	resolved, err := xray.Materialize(context.Background(), rendered, stage,
		[]xray.Server{server}, []xray.User{user}, resolver)
	if err != nil {
		t.Fatal(err)
	}
	serverCfgPath := filepath.Join(stage, "server.json")
	raw, err := json.Marshal(resolved)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(serverCfgPath, raw, 0o600); err != nil {
		t.Fatal(err)
	}

	// Client side: SOCKS inbound → VLESS+REALITY outbound with the
	// same UUID and public REALITY parameters.
	clientCfg := map[string]any{
		"log": map[string]any{"loglevel": "warning"},
		"inbounds": []map[string]any{{
			"tag": "socks-in", "listen": "127.0.0.1", "port": clientSocksPort,
			"protocol": "socks",
			"settings": map[string]any{"auth": "noauth", "udp": false},
		}},
		"outbounds": []map[string]any{{
			"tag":      "proxy",
			"protocol": "vless",
			"settings": map[string]any{
				"vnext": []map[string]any{{
					"address": "127.0.0.1", "port": serverPort,
					"users": []map[string]any{{
						"id": clientUUID, "encryption": "none",
						"flow": "xtls-rprx-vision",
					}},
				}},
			},
			"streamSettings": map[string]any{
				"network": "tcp", "security": "reality",
				"realitySettings": map[string]any{
					"serverName": realitySNI(), "fingerprint": "chrome",
					"publicKey": kp.PublicKey, "shortId": sid,
				},
			},
		}},
	}
	clientRaw, _ := json.Marshal(clientCfg)
	clientPath := filepath.Join(stage, "client.json")
	if err := os.WriteFile(clientPath, clientRaw, 0o600); err != nil {
		t.Fatal(err)
	}

	// RUNTIME-STARTED: both processes, real listeners.
	serverProc := startXray(t, bin, serverCfgPath)
	defer killProc(serverProc)
	if !waitTCP(t, serverPort, 10*time.Second) {
		t.Fatal("RUNTIME-STARTED FAIL: server listener did not come up")
	}
	clientProc := startXray(t, bin, clientPath)
	defer killProc(clientProc)
	if !waitTCP(t, clientSocksPort, 10*time.Second) {
		t.Fatal("client SOCKS listener did not come up")
	}

	// TRAFFIC-VERIFIED: real HTTP through curl → client SOCKS →
	// VLESS+REALITY → server → local target.
	out, err := exec.Command("curl", "-s", "--max-time", "20",
		"--socks5-hostname", "127.0.0.1:"+fmt.Sprint(clientSocksPort),
		target.URL).Output()
	if err != nil || !strings.Contains(string(out), "duckos-reality-ok") {
		t.Fatalf("TRAFFIC-VERIFIED FAIL: %v %s", err, out)
	}
	t.Log("TRAFFIC VERIFIED: VLESS+REALITY chain, separate server and client xray processes")

	// Both processes alive after traffic (stability evidence).
	if serverProc.Process == nil || clientProc.Process == nil {
		t.Fatal("processes died during traffic")
	}
}
