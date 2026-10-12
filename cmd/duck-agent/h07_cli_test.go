//go:build linux

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/CrazyDuckLab/DuckOS/internal/agent"
	"github.com/CrazyDuckLab/DuckOS/internal/identity"
	"github.com/CrazyDuckLab/DuckOS/internal/wireguard"
	"github.com/CrazyDuckLab/DuckOS/internal/xray"
)

func TestH07RealCLITunnelPublicParity(t *testing.T) {
	if !remediationIsolatedNetwork(t) {
		return
	}
	// Same private-launcher hygiene as H04: drop any interface
	// a previous crashed run leaked, then isolate the launcher.
	_ = exec.Command("ip", "link", "del", "wg-h07").Run()
	spawnTestLauncher(t)
	dir := t.TempDir()
	cli := filepath.Join(dir, "duck")
	build := exec.Command(filepath.Join(runtime.GOROOT(), "bin", "go"), "build", "-o", cli, "./cmd/duck")
	build.Dir = "../.."
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build duck: %v %s", err, out)
	}
	master := filepath.Join(dir, "master.key")
	if err := os.WriteFile(master, bytes.Repeat([]byte{7}, 32), 0600); err != nil {
		t.Fatal(err)
	}
	vault, err := agent.OpenVault(context.Background(), agent.VaultDeps{Path: filepath.Join(dir, "vault"), MasterFile: master})
	if err != nil {
		t.Fatal(err)
	}
	defer vault.Close()
	private, public, err := wireguard.GenerateServerKey()
	if err != nil {
		t.Fatal(err)
	}
	ref, err := vault.Put(strings.Repeat("a", 32), "wireguard-private", private)
	clear(private)
	if err != nil {
		t.Fatal(err)
	}
	key := strings.Repeat("k", 64)
	access := identity.ForwardedAccess{Secret: key}
	wgHost, err := enableWireguardService(filepath.Join(dir, "wg"), filepath.Join(dir, "conf"), access, vault)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("DUCKOS_XRAY_STAGE", filepath.Join(dir, "xray-runtime"))
	xHost, err := enableXrayService(filepath.Join(dir, "xray-catalog"), access, vault)
	if err != nil {
		t.Fatal(err)
	}
	defer xHost.Runtime.Stop()
	handler := agent.NewHandler(nil)
	handler.EnableWireguard(wgHost)
	handler.EnableXray(xHost)
	srv, token := remediationPublicAPI(t, handler, key)
	tokenFile := filepath.Join(dir, "token")
	if err := os.WriteFile(tokenFile, []byte(token), 0600); err != nil {
		t.Fatal(err)
	}
	run := func(endpoint string, wantError bool, args ...string) []byte {
		t.Helper()
		args = append(args, "--endpoint", endpoint, "--allow-insecure", "--token-file", tokenFile, "--output", "json")
		out, err := exec.Command(cli, args...).CombinedOutput()
		if wantError {
			e, ok := err.(*exec.ExitError)
			if !ok || e.ExitCode() == 0 {
				t.Fatalf("CLI failure exit missing: %v %s", err, out)
			}
		} else {
			if err != nil {
				t.Fatalf("CLI failed: %v %s", err, out)
			}
			if !json.Valid(out) {
				t.Fatalf("CLI output not JSON: %s", out)
			}
		}
		return out
	}
	if out := run(srv.URL, false, "wireguard", "servers", "list"); string(bytes.TrimSpace(out)) != "[]" {
		t.Fatalf("empty array: %s", out)
	}
	write := func(name string, v any) string {
		t.Helper()
		raw, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	server := wireguard.Server{ID: strings.Repeat("1", 32), Name: "h07", Interface: "wg-h07", ListenPort: 51820, Addresses: []string{"10.67.67.1/24"}, PrivateKeyRef: wireguard.KeyRef{VaultID: ref.ID, Revision: ref.Revision, KeyID: ref.KeyID}, PublicKey: public}
	run(srv.URL, false, "wireguard", "servers", "create", "--server", write("server.json", server))
	out := run(srv.URL, false, "wireguard", "servers", "list")
	var listed []wireguard.Server
	if err := json.Unmarshal(out, &listed); err != nil || len(listed) != 1 || listed[0].ID != server.ID {
		t.Fatal("nonempty public collection decode failed")
	}
	peerPrivate, peerPublic, err := wireguard.GenerateServerKey()
	if err != nil {
		t.Fatal(err)
	}
	clear(peerPrivate)
	peer := wireguard.Peer{ID: strings.Repeat("2", 32), ServerID: server.ID, Name: "client", PublicKey: peerPublic, AllowedIPs: []string{"10.67.67.2/32"}, Intent: wireguard.IntentCustom, Enabled: true}
	run(srv.URL, false, "wireguard", "peers", "add", "--peer", write("peer.json", peer))
	if out, err := exec.Command("wg", "show", server.Interface, "peers").CombinedOutput(); err != nil || !bytes.Contains(out, []byte(peerPublic)) {
		t.Fatal("CLI did not reach kernel peer mutation")
	}
	run(srv.URL, false, "wireguard", "peers", "delete", "--peer-id", peer.ID)
	run(srv.URL, false, "wireguard", "servers", "delete", "--server-id", server.ID)
	xserver := xray.Server{ID: strings.Repeat("3", 32), Name: "h07xray", Protocol: xray.ProtocolVLESS, Transport: xray.TransportTCP, Security: xray.SecurityNone, Listen: "127.0.0.1:38543", Port: 38543, Enabled: true}
	run(srv.URL, false, "xray", "servers", "create", "--file", write("xserver.json", xserver))
	run(srv.URL, false, "xray", "servers", "delete", "--id", xserver.ID)
	if err := os.WriteFile(tokenFile, []byte(strings.Repeat("x", 64)), 0600); err != nil {
		t.Fatal(err)
	}
	if out := run(srv.URL, true, "wireguard", "servers", "list"); !bytes.Contains(out, []byte("401")) {
		t.Fatalf("session failure not preserved: %s", out)
	}
	missing, missingToken := remediationPublicAPI(t, agent.NewHandler(nil), key)
	if err := os.WriteFile(tokenFile, []byte(missingToken), 0600); err != nil {
		t.Fatal(err)
	}
	if out := run(missing.URL, true, "wireguard", "servers", "list"); !bytes.Contains(out, []byte("503")) {
		t.Fatalf("backend failure not preserved: %s", out)
	}
}
