//go:build linux

package main

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/agent"
	"github.com/CrazyDuckLab/DuckOS/internal/healthauth"
	"github.com/CrazyDuckLab/DuckOS/internal/identity"
	"github.com/CrazyDuckLab/DuckOS/internal/upgrade"
)

func TestH08PublicSystemdSupervisor(t *testing.T) {
	if !remediationIsolatedNetwork(t) {
		return
	}
	command := func(args ...string) string {
		t.Helper()
		out, err := exec.Command("/usr/bin/systemctl", args...).CombinedOutput()
		if err != nil {
			t.Fatalf("isolated systemd fixture %v: %v %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	units := []string{"duck-agent.service", "duck-api.service"}
	for _, unit := range units {
		if command("show", "--property=LoadState", "--value", unit) != "not-found" {
			t.Fatal("refusing to replace existing DuckOS service")
		}
	}
	dir, err := os.MkdirTemp("/tmp", "h08-systemd-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	root := filepath.Join(dir, "installed")
	tokenFile := filepath.Join(dir, "token")
	tokenValue := strings.Repeat("t", 64)
	if err := os.WriteFile(tokenFile, []byte(tokenValue), 0600); err != nil {
		t.Fatal(err)
	}
	healthKey := filepath.Join(dir, "health.key")
	healthPub := filepath.Join(dir, "health.pub")
	if err := healthauth.Provision(healthKey, healthPub); err != nil {
		t.Fatal(err)
	}
	identities := filepath.Join(dir, "identity")
	if err := os.Mkdir(identities, 0700); err != nil {
		t.Fatal(err)
	}
	auth, err := identity.Open(identities)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := auth.Enroll("owner", "Synthetic enrolled update witness 2026!"); err != nil {
		t.Fatal(err)
	}
	auth.Close()
	web := filepath.Join(dir, "web")
	if err := os.Mkdir(web, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(web, "index.html"), []byte("<!doctype html><title>Health regression fixture</title>"), 0600); err != nil {
		t.Fatal(err)
	}
	var created []string
	defer func() {
		if t.Failed() {
			for _, unit := range created {
				out, _ := exec.Command("/usr/bin/journalctl", "-u", unit, "--no-pager", "-n", "15").CombinedOutput()
				t.Logf("fixture %s: %s", unit, out)
			}
		}
		for _, unit := range created {
			_ = exec.Command("/usr/bin/systemctl", "stop", unit).Run()
			_ = os.Remove(filepath.Join("/run/systemd/system", unit))
		}
		_ = exec.Command("/usr/bin/systemctl", "daemon-reload").Run()
	}()
	for _, unit := range units {
		name := strings.TrimSuffix(unit, ".service")
		args := "--production-handler --socket " + filepath.Join(dir, "agent.sock")
		if name == "duck-api" {
			args = "--listen 127.0.0.1:8080 --token-file " + tokenFile + " --agent-socket " + filepath.Join(dir, "agent.sock") + " --identity-dir " + identities + " --health-public-key " + healthPub + " --web-dir " + web
		}
		content := fmt.Sprintf("[Unit]\nDescription=Isolated H08 regression fixture\n[Service]\nType=simple\nExecStart=%s/current/bin/%s %s\nNetworkNamespacePath=/proc/%d/ns/net\n", root, name, args, os.Getpid())
		file, err := os.OpenFile(filepath.Join("/run/systemd/system", unit), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
		if err != nil {
			t.Fatal(err)
		}
		created = append(created, unit)
		_, writeErr := file.WriteString(content)
		closeErr := file.Close()
		if writeErr != nil || closeErr != nil {
			t.Fatal("unit fixture write failed")
		}
	}
	command("daemon-reload")
	priv, pub, err := upgrade.GenerateSigningKey()
	if err != nil {
		t.Fatal(err)
	}
	store, err := upgrade.NewReleaseStore(filepath.Join(dir, "store"), 64<<20)
	if err != nil {
		t.Fatal(err)
	}
	supervisor, err := upgrade.OpenSupervisor(store, []ed25519.PublicKey{pub}, runtime.GOARCH, root, &upgrade.SystemdRuntime{Root: root, HealthKeyFile: healthKey, HealthTimeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer supervisor.Close()
	updaterSocket := filepath.Join(dir, "updater.sock")
	listener, err := agent.Listen(updaterSocket, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	server := &http.Server{Handler: &upgrade.SupervisorHTTP{Supervisor: supervisor}}
	defer server.Close()
	go server.Serve(listener)
	h := agent.NewHandler(nil)
	h.EnableUpgrade(agent.NewUpdaterProxy(updaterSocket))
	api, session := remediationPublicAPI(t, h, strings.Repeat("h", 64))
	call := func(path string, body any) []byte {
		t.Helper()
		raw, _ := json.Marshal(body)
		req, _ := http.NewRequest("POST", api.URL+"/api/v1/upgrade/"+path, bytes.NewReader(raw))
		req.Header.Set("Authorization", "Bearer "+session)
		req.Header.Set("Content-Type", "application/json")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		out, _ := io.ReadAll(res.Body)
		if res.StatusCode != 200 {
			t.Fatalf("public %s: %d %s", path, res.StatusCode, out)
		}
		return out
	}
	stage := func(version string, unhealthy bool) {
		t.Helper()
		binaryPath := filepath.Join(dir, "native-"+version)
		// Keep the signed fixture compact, as production release builds do;
		// race-instrumented JSON transport still uses the production deadlines.
		flags := "-s -w -X main.version=" + version
		if unhealthy {
			flags += " -X main.unhealthy=true"
		}
		cmd := exec.Command(filepath.Join(runtime.GOROOT(), "bin", "go"), "build", "-ldflags", flags, "-o", binaryPath, "../../internal/upgrade/testdata/release")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("build: %v %s", err, out)
		}
		binary, err := os.ReadFile(binaryPath)
		if err != nil {
			t.Fatal(err)
		}
		build, _ := json.Marshal(upgrade.BuildIdentity{Version: version, Commit: strings.Repeat("1", 40), Target: "linux/" + runtime.GOARCH})
		var payload bytes.Buffer
		archive := tar.NewWriter(&payload)
		for _, entry := range []struct {
			name string
			data []byte
		}{{"bin/duck-agent", binary}, {"bin/duck-api", binary}, {"BUILD-INFO.json", build}} {
			if err := archive.WriteHeader(&tar.Header{Name: entry.name, Mode: 0755, Size: int64(len(entry.data))}); err != nil {
				t.Fatal(err)
			}
			if _, err := archive.Write(entry.data); err != nil {
				t.Fatal(err)
			}
		}
		if err := archive.Close(); err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(payload.Bytes())
		manifest, _ := json.Marshal(upgrade.Manifest{Format: upgrade.FormatVer, Architecture: runtime.GOARCH, SourceVersion: version, TargetVersion: version, InputSchema: 1, OutputSchema: 1, PayloadAlgorithm: "sha256", PayloadDigest: hex.EncodeToString(digest[:]), PayloadBytes: int64(payload.Len())})
		signed := append(ed25519.Sign(priv, manifest), manifest...)
		raw := append((upgrade.Header{Magic: upgrade.Magic, Format: upgrade.FormatVer, BodyBytes: int64(len(signed))}).Marshal(), signed...)
		raw = append(raw, payload.Bytes()...)
		call("stage", map[string]any{"operator": "owner", "channel": "stable", "releaseId": version, "releaseVersion": version, "releaseArch": runtime.GOARCH, "payloadBase64": base64.StdEncoding.EncodeToString(raw)})
	}
	wait := func(result, active string) {
		t.Helper()
		deadline := time.Now().Add(15 * time.Second)
		for {
			st := supervisor.State()
			if len(st.History) > 0 && st.History[len(st.History)-1].Result == result && st.Active == active && !supervisor.Pending() {
				if st.Active != active || st.RecoveryRequired {
					t.Fatalf("bad result: %+v", st)
				}
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("operation did not finish: %+v", st)
			}
			time.Sleep(25 * time.Millisecond)
		}
	}
	stage("vfirstbad", true)
	call("activate", map[string]any{})
	wait("activation-failed", "")
	stage("v1", false)
	call("activate", map[string]any{})
	wait("health-passed", "v1")
	stage("vbad", true)
	call("activate", map[string]any{})
	wait("activation-failed", "v1")
	stage("v2", false)
	call("activate", map[string]any{})
	wait("health-passed", "v2")
	call("rollback", map[string]any{"reason": "regression"})
	wait("health-passed", "v1")
	if err := (&upgrade.SystemdRuntime{Root: root, HealthKeyFile: healthKey, HealthTimeout: time.Second}).Health(context.Background(), upgrade.BuildIdentity{Version: "v1", Commit: strings.Repeat("1", 40)}); err != nil {
		t.Fatal(err)
	}

	server.Close()
	if err := supervisor.Close(); err != nil {
		t.Fatal(err)
	}
	recovered, err := upgrade.OpenSupervisor(store, []ed25519.PublicKey{pub}, runtime.GOARCH, root, &upgrade.SystemdRuntime{Root: root, HealthKeyFile: healthKey, HealthTimeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("enrolled Previous startup reconciliation: %v", err)
	}
	defer recovered.Close()
	if recovered.State().Active != "v1" || recovered.State().RecoveryRequired {
		t.Fatal("startup lost healthy Previous")
	}
}
