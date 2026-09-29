package integration

import (
	"context"
	"encoding/json"
	"github.com/CrazyDuckLab/DuckOS/internal/agent"
	"github.com/CrazyDuckLab/DuckOS/internal/api"
	"github.com/CrazyDuckLab/DuckOS/internal/engine"
	"github.com/CrazyDuckLab/DuckOS/internal/model"
	"github.com/CrazyDuckLab/DuckOS/internal/observe"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

// privateTempDir creates a temp directory that is owned by the
// test user and is not writable by group or other. t.TempDir()
// creates a directory with perm 0775 on this Go version, which
// the Agent refuses because the group write bit is set. The
// helper restores the per-temp-dir contract by chmod-ing the
// result to 0700.
func privateTempDir(t *testing.T) string {
	t.Helper()
	d := t.TempDir()
	if err := os.Chmod(d, 0o700); err != nil {
		t.Fatal(err)
	}
	return d
}

// This test observes the real runner's Linux state and real kernel peer UID.
// Missing optional systemd/nft facilities stay explicit issues; fixtures are not used.
func TestLinuxAPIEngineAgentRealHost(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("requires real Linux; Mac cross-compilation is not runtime evidence")
	}
	dir := privateTempDir(t)
	socket := dir + "/agent.sock"
	l, err := agent.Listen(socket, uint32(os.Getuid()))
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: agent.NewHandler(&observe.Collector{Environment: "test"}), ReadHeaderTimeout: time.Second}
	go func() { _ = server.Serve(l) }()
	defer server.Close()
	e := engine.New(socket, "test")
	data, err := e.Overview(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	o, err := model.DecodeOverview(data, "test", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	name, _ := os.Hostname()
	if o.System.Hostname != name || o.System.UptimeSeconds <= 0 || o.Memory.TotalBytes == 0 || len(o.Interfaces) == 0 {
		t.Fatal("actual Linux facts were not observed")
	}
	web := t.TempDir()
	if err := os.WriteFile(web+"/index.html", []byte("<title>TEST asset</title>"), 0600); err != nil {
		t.Fatal(err)
	}
	const token = "synthetic-integration-token-never-installed-123456789"
	h, err := api.New(e, token, "127.0.0.1:8080", web)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	r := httptest.NewRequest("GET", "http://127.0.0.1:8080/api/v1/overview", nil)
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	var viaAPI model.Overview
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &viaAPI) != nil || viaAPI.System.Hostname != name {
		t.Fatalf("API path failed status%d", w.Code)
	}
	t.Logf("Real Linux read integration passed (%s); optional observation issues=%d; not systemd installer/root capability qualification", runtime.GOARCH, len(o.Issues))
}

func TestLinuxPeerUIDDenialAndSocketPermissions(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("SO_PEERCRED is Linux-only")
	}
	dir := privateTempDir(t)
	socket := dir + "/denied.sock"
	l, err := agent.Listen(socket, uint32(os.Getuid()+1))
	if err != nil {
		t.Fatal(err)
	}
	s := &http.Server{Handler: agent.NewHandler(&observe.Collector{Environment: "test"}), ReadHeaderTimeout: time.Second}
	go func() { _ = s.Serve(l) }()
	defer s.Close()
	info, err := os.Stat(socket)
	if err != nil || info.Mode().Perm() != 0660 {
		t.Fatalf("socket mode %v %v", info, err)
	}
	e := engine.New(socket, "test")
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := e.Overview(ctx); err == nil {
		t.Fatal("wrong UID accepted")
	}
	file := dir + "/not-a-socket"
	if err := os.WriteFile(file, []byte("preserve"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := agent.Listen(file, uint32(os.Getuid())); err == nil {
		t.Fatal("non-socket clobbered")
	}
	b, _ := os.ReadFile(file)
	if strings.TrimSpace(string(b)) != "preserve" {
		t.Fatal("file changed")
	}
	t.Log("Kernel rejected UID", strconv.Itoa(os.Getuid()), "at the Agent boundary")
}
