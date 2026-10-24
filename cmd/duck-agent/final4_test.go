// Package main / final4_test.go: F-02 production peer-isolation
// regression for the final independent audit (2026-10-24). Uses the
// ACTUAL production nodeAuthLimiter, a real public TCP listener and
// the real Engine Unix IPC client.
package main

import (
	"github.com/CrazyDuckLab/DuckOS/internal/agent"
	publicapi "github.com/CrazyDuckLab/DuckOS/internal/api"
	"github.com/CrazyDuckLab/DuckOS/internal/center"
	"github.com/CrazyDuckLab/DuckOS/internal/engine"
	"github.com/CrazyDuckLab/DuckOS/internal/identity"
	"github.com/CrazyDuckLab/DuckOS/internal/xnode"
	"github.com/CrazyDuckLab/DuckOS/internal/xray"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// Real public TCP connections from two local source IPs, the actual
// production nodeAuthLimiter(), and the real Engine Unix IPC client.
func TestFinalActualProductionLimiterTwoTCPPeers(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<!doctype html><title>audit</title>"), 0o600); err != nil {
		t.Fatal(err)
	}
	secret := "synthetic-test-assertion-secret-01234567890123456789"
	access := identity.ForwardedAccess{Secret: secret}
	m := xnode.NewManager(nil)
	tok, _, err := m.IssueEnrollmentToken(time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	n, old, err := m.Enroll(tok, xnode.NodeInfo{Name: "victim"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = m.StageRotation(n.ID); err != nil {
		t.Fatal(err)
	}
	limit := nodeAuthLimiter()
	var mu sync.Mutex
	keys := []string{}
	sources := []string{}
	ch := &center.Handler{Access: access, Prefix: "center/", Nodes: m, NodeAuthRate: func(peer string) bool {
		mu.Lock()
		keys = append(keys, peer)
		mu.Unlock()
		return limit(peer)
	}}
	cat, err := xray.OpenFileCatalog(filepath.Join(dir, "catalog"))
	if err != nil {
		t.Fatal(err)
	}
	host := agent.NewXrayHost(&xray.API{Access: access, Catalog: cat, Prefix: "/xray/v1/", Center: ch}, access, nil)
	sock := filepath.Join(dir, "a.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.Handle(agent.XrayPath+"/", host)
	ipc := &http.Server{Handler: mux}
	go ipc.Serve(ln)
	defer ipc.Close()
	h, err := publicapi.New(engine.New(sock, "audit"), "synthetic-operator-token-01234567890123456789", "127.0.0.1:8080", dir)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	h.EnableAgentAssertionKey(secret)
	public := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		sources = append(sources, r.RemoteAddr)
		mu.Unlock()
		h.ServeHTTP(w, r)
	}))
	defer public.Close()
	call := func(source, cred string) int {
		tr := &http.Transport{Proxy: nil, DisableKeepAlives: true, DialContext: (&net.Dialer{LocalAddr: &net.TCPAddr{IP: net.ParseIP(source)}, Timeout: time.Second}).DialContext}
		defer tr.CloseIdleConnections()
		client := &http.Client{Transport: tr, Timeout: 3 * time.Second}
		req, err := http.NewRequest("POST", public.URL+"/api/v1/xray/center/node-auth/"+n.ID+"/pull", nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Host = "127.0.0.1:8080"
		req.Header.Set("X-Node-Credential", cred)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	for i := 0; i < 60; i++ {
		if code := call("127.0.0.1", "wrong"); code != 403 {
			t.Fatalf("wrong attempt %d returned %d", i+1, code)
		}
	}
	over := call("127.0.0.1", "wrong")
	victim := call("127.0.0.2", old)
	mu.Lock()
	first, last := sources[0], sources[len(sources)-1]
	kfirst, klast := keys[0], keys[len(keys)-1]
	mu.Unlock()
	t.Logf("actual TCP sources=%s and %s; actual production limiter keys=%q and %q; attacker 61=%d; legitimate other-IP first=%d", first, last, kfirst, klast, over, victim)
	if over != 429 {
		t.Error("source port churn bypassed limiter")
	}
	if victim != 200 {
		t.Fatalf("unauthenticated TCP peer blocks a different legitimate peer through actual production limiter: %d", victim)
	}
	if kfirst == klast || kfirst == "@" || klast == "@" {
		t.Fatalf("production limiter lost per-peer isolation: keys=%q..%q", kfirst, klast)
	}
}
