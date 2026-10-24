// Package api / final4_test.go: F-02 trusted ingress peer identity
// over the API→Engine→Agent→Center chain, plus the canonical
// methods/RBAC matrix, for the final independent audit (2026-10-24).
package api

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/agent"
	"github.com/CrazyDuckLab/DuckOS/internal/center"
	"github.com/CrazyDuckLab/DuckOS/internal/engine"
	"github.com/CrazyDuckLab/DuckOS/internal/identity"
	"github.com/CrazyDuckLab/DuckOS/internal/xnode"
	"github.com/CrazyDuckLab/DuckOS/internal/xray"
)

// Spy observes the production peer key at the Center; the rest of
// the path uses the actual API, Engine, Unix HTTP transport,
// XrayHost and Center.
func finalChain(t *testing.T) (*Handler, *xnode.Manager, *[]string, *sync.Mutex) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("audit"), 0o600); err != nil {
		t.Fatal(err)
	}
	m := xnode.NewManager(nil)
	peers := []string{}
	mu := &sync.Mutex{}
	hits := map[string]int{}
	access := identity.ForwardedAccess{Secret: testAssertionKey}
	ch := &center.Handler{Access: access, Prefix: "center/", Nodes: m, NodeAuthRate: func(peer string) bool {
		mu.Lock()
		defer mu.Unlock()
		peers = append(peers, peer)
		hits[peer]++
		return hits[peer] <= 60
	}}
	cat, err := xray.OpenFileCatalog(filepath.Join(dir, "catalog"))
	if err != nil {
		t.Fatal(err)
	}
	host := agent.NewXrayHost(&xray.API{Access: access, Catalog: cat, Prefix: "/xray/v1/", Center: ch}, access, nil)
	sock := filepath.Join(dir, "agent.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: host, ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second}
	go srv.Serve(ln)
	t.Cleanup(func() { srv.Close() })
	h, err := New(engine.New(sock, "audit"), testToken, "127.0.0.1:8080", dir)
	if err != nil {
		t.Fatal(err)
	}
	h.EnableAgentAssertionKey(testAssertionKey)
	t.Cleanup(func() { h.Close() })
	return h, m, &peers, mu
}

func finalRequest(h *Handler, method, path, cred, peer string) int {
	r := httptest.NewRequest(method, "http://127.0.0.1:8080"+path, nil)
	r.Host = "127.0.0.1:8080"
	r.RemoteAddr = peer
	r.Header.Set("X-Node-Credential", cred)
	// Spoofable client-supplied peer headers must never become the
	// rate-limit key.
	r.Header.Set("X-Forwarded-For", "198.51.100.77")
	r.Header.Set("X-Real-IP", "198.51.100.78")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w.Code
}

func TestFinalProductionPeerIdentityAndLimiterIsolation(t *testing.T) {
	h, m, peers, mu := finalChain(t)
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
	path := "/api/v1/xray/center/node-auth/" + n.ID + "/pull"
	for i := 0; i < 60; i++ {
		code := finalRequest(h, "POST", path, "wrong", fmt.Sprintf("192.0.2.1:%d", 10000+i))
		if code == 429 {
			t.Fatalf("limited before 60: %d", i)
		}
	}
	attacker := finalRequest(h, "POST", path, "wrong", "192.0.2.1:30000")
	victim := finalRequest(h, "POST", path, old, "192.0.2.2:40000")
	mu.Lock()
	seen := append([]string(nil), (*peers)...)
	mu.Unlock()
	t.Logf("attacker request 61=%d, first legitimate request from other IP=%d, center peer keys=%q..%q", attacker, victim, seen[0], seen[len(seen)-1])
	if attacker != 429 {
		t.Error("port churn bypassed rate limit")
	}
	if victim != 200 {
		t.Fatalf("one unauthenticated peer exhausted another peer's production node-auth budget: %d", victim)
	}
	if seen[0] != "192.0.2.1" || seen[len(seen)-1] != "192.0.2.2" {
		t.Fatal("original source host lost over IPC")
	}
}

func TestFinalProductionCanonicalMethodsAndOperatorIsolation(t *testing.T) {
	h, m, _, _ := finalChain(t)
	tok, _, err := m.IssueEnrollmentToken(time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	n, cred, err := m.Enroll(tok, xnode.NodeInfo{Name: "matrix"})
	if err != nil {
		t.Fatal(err)
	}
	prefix := "/api/v1/xray/center/"
	for _, method := range []string{"GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "OPTIONS", "TRACE"} {
		for _, tail := range []string{"nodes", "nodes/" + n.ID + "/rotate", "groups", "policies", "stats", "storage", "tokens", "preview/x", "backup", "events", "diagnostics", "delivery-validate", "node-authx/" + n.ID + "/pull", "node-auth//pull", "node-auth/" + n.ID + "/pull/extra", "node-auth/../nodes", "node-auth/" + n.ID + "/%70ull"} {
			code := finalRequest(h, method, prefix+tail, cred, "192.0.2.10:10000")
			if code >= 200 && code < 300 {
				t.Errorf("node credential authorized %s %s: %d", method, tail, code)
			}
		}
		if method != "POST" {
			code := finalRequest(h, method, prefix+"node-auth/"+n.ID+"/pull", cred, "192.0.2.10:10000")
			if code != 405 {
				t.Errorf("method %s=%d", method, code)
			}
		}
	}
}
