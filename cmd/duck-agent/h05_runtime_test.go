//go:build linux

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/agent"
	"github.com/CrazyDuckLab/DuckOS/internal/identity"
	"github.com/CrazyDuckLab/DuckOS/internal/xray"
)

func TestH05ProductionLastUserRevocation(t *testing.T) {
	if !remediationIsolatedNetwork(t) {
		return
	}
	xrayBinary := os.Getenv("DUCKOS_XRAY")
	if xrayBinary == "" {
		xrayBinary = xray.XrayPath
	}
	if _, err := os.Stat(xrayBinary); err != nil {
		t.Fatal("installed native Xray binary required")
	}
	dir := t.TempDir()
	stage := filepath.Join(dir, "runtime")
	t.Setenv("DUCKOS_XRAY_STAGE", stage)
	catalog := filepath.Join(dir, "catalog")
	key := strings.Repeat("k", 64)
	access := identity.ForwardedAccess{Secret: key}
	host, err := enableXrayService(catalog, access)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { host.Runtime.Stop() }()
	ah := agent.NewHandler(nil)
	ah.EnableXray(host)
	srv, token := remediationPublicAPI(t, ah, key)
	call := func(method, path string, body any, bearer string, want int) {
		t.Helper()
		var raw []byte
		if body != nil {
			raw, err = json.Marshal(body)
			if err != nil {
				t.Fatal(err)
			}
		}
		req, err := http.NewRequest(method, srv.URL+"/api/v1/xray/"+path, bytes.NewReader(raw))
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
		out, _ := io.ReadAll(res.Body)
		if res.StatusCode != want {
			t.Fatalf("%s %s: HTTP %d want %d: %s", method, path, res.StatusCode, want, out)
		}
	}
	port := 38541
	server := xray.Server{ID: strings.Repeat("1", 32), Name: "h05", Protocol: xray.ProtocolVLESS, Transport: xray.TransportTCP, Security: xray.SecurityNone, Listen: fmt.Sprintf("127.0.0.1:%d", port), Port: port, Enabled: true}
	user := xray.User{ID: strings.Repeat("2", 32), ServerID: server.ID, Name: "client", UUID: "01234567-89ab-cdef-0123-456789abcdef", Enabled: true}
	call("POST", "servers", server, "", 401)
	call("POST", "servers", server, token, 201)
	call("POST", "users", user, token, 201)
	oracle := func(enabled bool) {
		t.Helper()
		conn, err := net.DialTimeout("tcp", server.Listen, 300*time.Millisecond)
		if conn != nil {
			conn.Close()
		}
		if (err == nil) != enabled {
			t.Fatalf("independent listener oracle enabled=%v err=%v", enabled, err)
		}
		raw, err := os.ReadFile(filepath.Join(stage, "active.json"))
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(raw, []byte(user.UUID)) != enabled {
			t.Fatal("active configuration retains incorrect credential state")
		}
	}
	oracle(true)
	restart := func() {
		t.Helper()
		if err := host.Runtime.Stop(); err != nil {
			t.Fatal(err)
		}
		host, err = enableXrayService(catalog, access)
		if err != nil {
			t.Fatal(err)
		}
		ah.EnableXray(host)
	}
	restart()
	oracle(true)
	// Native parser rejection must leave the running daemon unchanged.
	pid := host.Runtime.State().PID
	err = host.Runtime.Apply(context.Background(), xray.Config{Version: 1, Inbounds: []xray.Inbound{{Tag: "invalid", Listen: "127.0.0.1", Port: port, Protocol: "invalid-h05"}}})
	if err == nil || host.Runtime.State().PID != pid {
		t.Fatal("native validation failed to protect active runtime")
	}
	oracle(true)
	// An occupied port is valid JSON/native syntax, but not a healthy runtime.
	occupied, err := net.Listen("tcp", "127.0.0.1:38542")
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()
	bad := server
	bad.Listen = "127.0.0.1:38542"
	bad.Port = 38542
	call("PUT", "servers/"+server.ID, bad, token, 503)
	oracle(true)
	call("DELETE", "users/"+user.ID, nil, token, 200)
	oracle(false)
	restart()
	oracle(false)
	cat, err := xray.OpenFileCatalog(catalog)
	if err != nil {
		t.Fatal(err)
	}
	users, err := cat.ListUsers(context.Background(), server.ID)
	if err != nil || len(users) != 0 {
		t.Fatal("revoked user resurrected in durable catalog")
	}
}
