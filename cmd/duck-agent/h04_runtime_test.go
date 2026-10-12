//go:build linux

package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/agent"
	"github.com/CrazyDuckLab/DuckOS/internal/api"
	"github.com/CrazyDuckLab/DuckOS/internal/engine"
	"github.com/CrazyDuckLab/DuckOS/internal/identity"
	"github.com/CrazyDuckLab/DuckOS/internal/wireguard"
)

func remediationPublicAPI(t *testing.T, handler http.Handler, assertion string, configure ...func(*engine.Engine)) (*httptest.Server, string) {
	t.Helper()
	dir := t.TempDir()
	listener, err := net.Listen("unix", filepath.Join(dir, "agent.sock"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })
	go http.Serve(listener, handler)
	web := filepath.Join(dir, "web")
	if err := os.Mkdir(web, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(web, "index.html"), []byte("test"), 0600); err != nil {
		t.Fatal(err)
	}
	identDir := filepath.Join(dir, "identity")
	if err := os.Mkdir(identDir, 0700); err != nil {
		t.Fatal(err)
	}
	ident, err := identity.Open(identDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ident.Close() })
	if _, err := ident.Enroll("remediation-owner", "test-password-remediation-123"); err != nil {
		t.Fatal(err)
	}
	token, _, err := ident.Login("remediation-owner", "test-password-remediation-123")
	if err != nil {
		t.Fatal(err)
	}
	eng := engine.New(listener.Addr().String(), "test")
	for _, option := range configure {
		option(eng)
	}
	h, err := api.New(eng, strings.Repeat("b", 64), "127.0.0.1:38083", web)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { h.Close() })
	h.EnableIdentity(ident)
	h.EnableAgentAssertionKey(assertion)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Host = "127.0.0.1:38083"
		h.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv, token
}

func remediationIsolatedNetwork(t *testing.T) bool {
	t.Helper()
	if os.Getenv("DUCKOS_REMEDIATION_NETNS") != t.Name() {
		if os.Geteuid() != 0 {
			t.Skip("requires root and disposable netns")
		}
		for _, binary := range []string{"ip", "wg"} {
			if _, err := exec.LookPath(binary); err != nil {
				t.Skip(binary + " required")
			}
		}
		suffix := make([]byte, 6)
		if _, err := rand.Read(suffix); err != nil {
			t.Fatal(err)
		}
		ns := "duck-remed-" + hex.EncodeToString(suffix)
		if out, err := exec.Command("ip", "netns", "add", ns).CombinedOutput(); err != nil {
			t.Fatalf("netns: %v %s", err, out)
		}
		defer exec.Command("ip", "netns", "del", ns).Run()
		if out, err := exec.Command("ip", "-n", ns, "link", "set", "lo", "up").CombinedOutput(); err != nil {
			t.Fatalf("loopback: %v %s", err, out)
		}
		binary, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "ip", "netns", "exec", ns, binary, "-test.run=^"+t.Name()+"$", "-test.v")
		cmd.Env = append(os.Environ(), "DUCKOS_REMEDIATION_NETNS="+t.Name())
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("isolated production test: %v\n%s", err, out)
		} else {
			t.Log(string(out))
		}
		return false
	}
	currentNS, err := os.Readlink("/proc/self/ns/net")
	if err != nil {
		t.Fatal(err)
	}
	hostNS, err := os.Readlink("/proc/1/ns/net")
	if err != nil || currentNS == hostNS {
		t.Fatal("refusing kernel test in host network namespace")
	}
	ifaces, err := net.Interfaces()
	if err != nil {
		t.Fatal(err)
	}
	for _, iface := range ifaces {
		if iface.Name != "lo" {
			t.Fatal("refusing non-empty test network namespace")
		}
	}
	return true
}

// spawnTestLauncher builds and runs a private
// duck-wg-launcher with a fresh owned-state directory and a
// private socket, then points the package-level socket
// override at it. Production tests must never drive the
// host's live launcher: it persists owned interfaces and
// re-creates them (qualified boot recovery), which both
// pollutes the host and resurrects interfaces from failed
// runs across netns boundaries.
func spawnTestLauncher(t *testing.T) {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "duck-wg-launcher")
	build := exec.Command(filepath.Join(runtime.GOROOT(), "bin", "go"), "build", "-o", bin, "./cmd/duck-wg-launcher")
	build.Dir = "../.."
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build duck-wg-launcher: %v %s", err, out)
	}
	work := t.TempDir()
	sock := filepath.Join(work, "wg-launcher.sock")
	cmd := exec.Command(bin,
		"-listen", sock,
		"-state", filepath.Join(work, "state"),
		"-secret", "/run/duckos-agent/secret",
		"-agent-uid", fmt.Sprintf("%d", os.Geteuid()),
	)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start launcher: %v", err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	})
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(sock); err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	old := wireguard.WGLauncherSocket
	wireguard.WGLauncherSocket = func() string { return sock }
	t.Cleanup(func() { wireguard.WGLauncherSocket = old })
}

func TestH04ProductionVaultAndKernelRestart(t *testing.T) {
	if !remediationIsolatedNetwork(t) {
		return
	}
	// The test drives a private launcher against the real
	// kernel inside the netns. A leftover interface from a
	// crashed run must never mask the create path.
	_ = exec.Command("ip", "link", "del", "wg-h04").Run()
	spawnTestLauncher(t)
	dir := t.TempDir()
	master := filepath.Join(dir, "master.key")
	if err := os.WriteFile(master, bytes.Repeat([]byte{7}, 32), 0600); err != nil {
		t.Fatal(err)
	}
	deps := agent.VaultDeps{Path: filepath.Join(dir, "vault"), MasterFile: master}
	secrets, err := agent.OpenVault(context.Background(), deps)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { secrets.Close() }()
	private, public, err := wireguard.GenerateServerKey()
	if err != nil {
		t.Fatal(err)
	}
	privateText := base64.StdEncoding.EncodeToString(private)
	ref, err := secrets.Put(strings.Repeat("a", 32), "wireguard-private", private)
	clear(private)
	if err != nil {
		t.Fatal(err)
	}
	key := strings.Repeat("k", 64)
	access := identity.ForwardedAccess{Secret: key}
	catalog, configs := filepath.Join(dir, "catalog"), filepath.Join(dir, "configs")
	if h, err := enableWireguardService(catalog, configs, access, nil); err == nil || h != nil {
		t.Fatal("missing vault accepted")
	}
	host, err := enableWireguardService(catalog, configs, access, secrets)
	if err != nil {
		t.Fatal(err)
	}
	ah := agent.NewHandler(nil)
	ah.EnableWireguard(host)
	srv, token := remediationPublicAPI(t, ah, key)
	call := func(method, path string, body any, bearer string, want int) []byte {
		t.Helper()
		var raw []byte
		if body != nil {
			raw, err = json.Marshal(body)
			if err != nil {
				t.Fatal(err)
			}
		}
		req, err := http.NewRequest(method, srv.URL+"/api/v1/wireguard/"+path, bytes.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		if bearer != "" {
			req.Header.Set("Authorization", "Bearer "+bearer)
		}
		req.Header.Set("Content-Type", "application/json")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		out, err := io.ReadAll(res.Body)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(out, []byte(privateText)) {
			t.Fatal("private key leaked")
		}
		if res.StatusCode != want {
			t.Fatalf("%s %s: HTTP %d want %d: %s", method, path, res.StatusCode, want, out)
		}
		return out
	}
	server := wireguard.Server{ID: strings.Repeat("1", 32), Name: "h04", Interface: "wg-h04", ListenPort: 51820, Addresses: []string{"10.66.66.1/24"}, PrivateKeyRef: wireguard.KeyRef{VaultID: ref.ID, Revision: ref.Revision, KeyID: ref.KeyID}, PublicKey: public}
	call("POST", "servers", server, "", 401)
	call("POST", "servers", server, token, 201)
	peerKey, peerPublic, err := wireguard.GenerateServerKey()
	if err != nil {
		t.Fatal(err)
	}
	clear(peerKey)
	peer := wireguard.Peer{ID: strings.Repeat("2", 32), ServerID: server.ID, Name: "client", PublicKey: peerPublic, AllowedIPs: []string{"10.66.66.2/32", "10.66.67.0/24"}, Intent: wireguard.IntentCustom, Enabled: true}
	call("POST", "peers", peer, token, 201)
	oracle := func(want bool) {
		t.Helper()
		out, err := exec.Command("wg", "show", server.Interface, "allowed-ips").CombinedOutput()
		if err != nil {
			t.Fatalf("wg oracle: %v %s", err, out)
		}
		found := strings.Contains(string(out), peerPublic) && strings.Contains(string(out), "10.66.66.2/32") && strings.Contains(string(out), "10.66.67.0/24")
		if found != want {
			t.Fatalf("peer runtime want=%v: %s", want, out)
		}
	}
	oracle(true)
	call("GET", "servers/"+server.ID+"/export", nil, "", 401)
	export := call("GET", "servers/"+server.ID+"/export", nil, token, 200)
	if bytes.Contains(export, []byte("PrivateKey =")) {
		t.Fatal("unredacted server export")
	}
	restart := func() {
		t.Helper()
		if err := secrets.Close(); err != nil {
			t.Fatal(err)
		}
		secrets, err = agent.OpenVault(context.Background(), deps)
		if err != nil {
			t.Fatal(err)
		}
		host, err = enableWireguardService(catalog, configs, access, secrets)
		if err != nil {
			t.Fatal(err)
		}
		ah.EnableWireguard(host)
	}
	if out, err := exec.Command("ip", "link", "del", server.Interface).CombinedOutput(); err != nil {
		t.Fatalf("isolated reset: %v %s", err, out)
	}
	restart()
	oracle(true)
	peer.Enabled = false
	call("PUT", "peers/"+peer.ID, peer, token, 200)
	oracle(false)
	restart()
	oracle(false)
	call("DELETE", "peers/"+peer.ID, nil, token, 200)
	restart()
	oracle(false)
	call("DELETE", "servers/"+server.ID, nil, token, 200)
	restart()
	if err := exec.Command("ip", "link", "show", server.Interface).Run(); err == nil {
		t.Fatal("deleted interface resurrected")
	}
}
