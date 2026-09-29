//go:build linux

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/restore"
)

func TestH09InstalledRestoreService(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("isolated systemd mount namespace requires root")
	}
	root, err := os.MkdirTemp("/var/lib", "duckos-h09-service-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(root) })
	for _, p := range []string{"usr/libexec/duckos", "var/lib/duckos/restore-source/b1", "var/lib/duckos/restore-live", "var/lib/duckos-restore", "run/duckos-restore"} {
		if err := os.MkdirAll(filepath.Join(root, p), 0700); err != nil {
			t.Fatal(err)
		}
	}
	binary := filepath.Join(root, "usr/libexec/duckos/duck-restore")
	build := exec.Command(filepath.Join(runtime.GOROOT(), "bin/go"), "build", "-o", binary, ".")
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("service build: %v %s", err, out)
	}
	source := filepath.Join(root, "var/lib/duckos/restore-source/b1/selected")
	live := filepath.Join(root, "var/lib/duckos/restore-live/selected")
	if err := os.WriteFile(source, []byte("after"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(live, []byte("before"), 0640); err != nil {
		t.Fatal(err)
	}
	unitBytes, err := os.ReadFile("../../packaging/systemd/duck-restore.service")
	if err != nil {
		t.Fatal(err)
	}
	// Use the production executable, sandbox and exact fixed paths in an
	// isolated mount namespace. Fixture directories are provisioned explicitly;
	// disable systemd's host State/RuntimeDirectory creation for this test only.
	unit := strings.ReplaceAll(string(unitBytes), "StateDirectory=duckos-restore", "StateDirectory=")
	unit = strings.ReplaceAll(unit, "RuntimeDirectory=duckos-restore", "RuntimeDirectory=")
	// Bind only the production managed roots to disposable backing directories.
	// Keep the production write allowlist and every sandbox restriction intact.
	unit = strings.Replace(unit, "ExecStart=/usr/libexec/duckos/duck-restore", "ExecStart="+binary, 1)
	for _, p := range []string{"/var/lib/duckos/restore-live", "/var/lib/duckos-restore", "/run/duckos-restore"} {
		unit = strings.Replace(unit, "[Install]", "BindPaths="+root+p+":"+p+"\n\n[Install]", 1)
	}
	unit = strings.Replace(unit, "[Install]", "BindReadOnlyPaths="+root+"/var/lib/duckos/restore-source:/var/lib/duckos/restore-source\n\n[Install]", 1)
	name := fmt.Sprintf("duckos-h09-%d-%d.service", os.Getpid(), time.Now().UnixNano())
	unitPath := filepath.Join("/run/systemd/system", name)
	f, err := os.OpenFile(unitPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.WriteString(unit); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Close()
	t.Cleanup(func() {
		exec.Command("/usr/bin/systemctl", "stop", name).Run()
		os.Remove(unitPath)
		exec.Command("/usr/bin/systemctl", "daemon-reload").Run()
	})
	t.Cleanup(func() {
		if t.Failed() {
			out, _ := exec.Command("/usr/bin/journalctl", "-u", name, "--no-pager", "-n", "20").CombinedOutput()
			t.Logf("isolated service journal: %s", out)
		}
	})
	command := func(args ...string) {
		t.Helper()
		if out, err := exec.Command("/usr/bin/systemctl", args...).CombinedOutput(); err != nil {
			t.Fatalf("systemd fixture: %v %s", err, out)
		}
	}
	command("daemon-reload")
	command("start", name)
	socket := filepath.Join(root, "run/duckos-restore/restore.sock")
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", socket)
	}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second}
	request := restore.Request{ID: strings.Repeat("1", 32), Expected: strings.Repeat("2", 64), Backup: "b1", Keys: []string{"selected"}}
	raw, _ := json.Marshal(request)
	call := func() {
		t.Helper()
		deadline := time.Now().Add(10 * time.Second)
		for {
			response, err := client.Post("http://restore/v1/restore", "application/json", bytes.NewReader(raw))
			if err == nil {
				var result restore.Result
				decodeErr := json.NewDecoder(response.Body).Decode(&result)
				response.Body.Close()
				if response.StatusCode != 200 || decodeErr != nil || result.Outcome != "applied" {
					t.Fatalf("installed service outcome: %d %+v %v", response.StatusCode, result, decodeErr)
				}
				return
			}
			if time.Now().After(deadline) {
				t.Fatal("installed service socket unavailable", err)
			}
			time.Sleep(25 * time.Millisecond)
		}
	}
	call()
	if got, err := exec.Command("/bin/cat", live).Output(); err != nil || string(got) != "after" {
		t.Fatal("independent live-file oracle failed", err)
	}
	command("kill", "--kill-whom=main", "--signal=SIGKILL", name)
	command("restart", name)
	call() // Durable receipt after process restart, not a second restore.
	if err := os.WriteFile(source, []byte("changed after commit"), 0600); err != nil {
		t.Fatal(err)
	}
	call()
	if got, _ := os.ReadFile(live); string(got) != "after" {
		t.Fatal("restart replay performed a new mutation")
	}
}
