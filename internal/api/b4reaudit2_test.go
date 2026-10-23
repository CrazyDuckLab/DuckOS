// Package api / b4reaudit2_test.go: production node-auth admission
// regressions for the second independent re-audit (2026-10-23).
// These tests exercise the REAL request chain — public API → Engine
// → Agent XrayHost → Xray API → Center — not just the Center
// handler fixture. (The package only builds where internal/agent
// builds: Linux.)
package api

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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

// The public API must not reject a legitimate node-only rotation
// request before the node credential can even be checked: the
// request has to reach the Engine (the Center performs the actual
// credential verification).
func TestB403_PublicNodeOnlyAdmissionReachesEngine(t *testing.T) {
	e := &testEngine{}
	h := handler(t, e)
	r := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8080/api/v1/xray/center/node-auth/real-node/pull", nil)
	r.Host = "127.0.0.1:8080"
	r.Header.Set("X-Node-Credential", "synthetic-valid-node-credential")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code == 401 && e.xrayCalls == 0 {
		t.Fatalf("public API rejects before node credential can be checked: status=%d engineCalls=%d", w.Code, e.xrayCalls)
	}
}

func b4rChain(t *testing.T) (*Handler, *xnode.Manager) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<!doctype html><title>b4r</title>"), 0o600); err != nil {
		t.Fatal(err)
	}
	nodeMgr := xnode.NewManager(nil)
	centerHandler := &center.Handler{
		Access:       identity.ForwardedAccess{Secret: testAssertionKey},
		Prefix:       "center/",
		Nodes:        nodeMgr,
		NodeAuthRate: nodeAuthLimiterForTest(),
	}
	cat, err := xray.OpenFileCatalog(filepath.Join(dir, "xray"))
	if err != nil {
		t.Fatal(err)
	}
	access := identity.ForwardedAccess{Secret: testAssertionKey}
	xrayAPI := &xray.API{Access: access, Catalog: cat, Prefix: "/xray/v1/", Center: centerHandler}
	host := agent.NewXrayHost(xrayAPI, access, nil)

	sockPath := filepath.Join(dir, "agent.sock")
	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatal(err)
	}
	agentSrv := &http.Server{Handler: host, ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second}
	go func() { _ = agentSrv.Serve(ln) }()
	t.Cleanup(func() { _ = agentSrv.Close() })

	eng := engine.New(sockPath, "test")
	h, err := New(eng, testToken, "127.0.0.1:8080", dir)
	if err != nil {
		t.Fatal(err)
	}
	h.EnableAgentAssertionKey(testAssertionKey)
	t.Cleanup(func() { _ = h.Close() })
	return h, nodeMgr
}

func nodeAuthLimiterForTest() func(string) bool {
	var mu sync.Mutex
	hits := map[string]int{}
	return func(peer string) bool {
		mu.Lock()
		defer mu.Unlock()
		hits[peer]++
		return hits[peer] <= 60
	}
}

// The FULL production chain admits a legitimate node-only rotation
// without an operator bearer, binds the credential to the exact node
// id, and rejects wrong credentials / other methods / operator
// routes for node principals.
func TestB403_FullChainNodeAuthThroughEngineAgentCenter(t *testing.T) {
	h, nodeMgr := b4rChain(t)
	tok, _, err := nodeMgr.IssueEnrollmentToken(time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	n, old, err := nodeMgr.Enroll(tok, xnode.NodeInfo{Name: "chain-node"})
	if err != nil {
		t.Fatal(err)
	}
	next, _, err := nodeMgr.StageRotation(n.ID)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(h)
	defer srv.Close()

	post := func(path, cred string) *http.Response {
		req, err := http.NewRequest(http.MethodPost, srv.URL+path, nil)
		req.Host = "127.0.0.1:8080"
		if err != nil {
			t.Fatal(err)
		}
		if cred != "" {
			req.Header.Set("X-Node-Credential", cred)
		}
		resp, err := srv.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}

	// Positive: node-only pull through API → Engine → Agent → Center.
	resp := post("/api/v1/xray/center/node-auth/"+n.ID+"/pull", old)
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("node-only pull through the full chain: status=%d body=%s", resp.StatusCode, body)
	}
	var out struct {
		Credential string `json:"credential"`
		State      string `json:"state"`
	}
	if err := json.Unmarshal(body, &out); err != nil || out.Credential != next {
		t.Fatalf("unexpected pull response: %s", body)
	}
	// Positive: confirm with the staged credential.
	resp = post("/api/v1/xray/center/node-auth/"+n.ID+"/confirm", next)
	body, _ = io.ReadAll(io.LimitReader(resp.Body, 4096))
	resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(string(body), "CONFIRMED") {
		t.Fatalf("node-only confirm through the full chain: status=%d body=%s", resp.StatusCode, body)
	}
	if nodeMgr.Authenticate(n.ID, old) || !nodeMgr.Authenticate(n.ID, next) {
		t.Fatal("controller credential state wrong after full-chain rotation")
	}

	// Negative: wrong credential.
	resp = post("/api/v1/xray/center/node-auth/"+n.ID+"/pull", "wrong-credential")
	resp.Body.Close()
	if resp.StatusCode < 400 {
		t.Fatalf("wrong credential admitted: %d", resp.StatusCode)
	}
	// Negative: credential bound to the exact node id.
	tok2, _, err := nodeMgr.IssueEnrollmentToken(time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	n2, otherCred, err := nodeMgr.Enroll(tok2, xnode.NodeInfo{Name: "other-node"})
	if err != nil {
		t.Fatal(err)
	}
	resp = post("/api/v1/xray/center/node-auth/"+n.ID+"/pull", otherCred)
	resp.Body.Close()
	if resp.StatusCode < 400 {
		t.Fatalf("other node's credential admitted: %d", resp.StatusCode)
	}
	_ = n2
	// Negative: non-POST methods on node-auth.
	req, err := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/xray/center/node-auth/"+n.ID+"/pull", nil)
	req.Host = "127.0.0.1:8080"
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Node-Credential", next)
	resp, err = srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("GET node-auth: %d", resp.StatusCode)
	}
	// Negative: non-canonical paths never reach the node-auth route.
	for _, p := range []string{
		"/api/v1/xray/center/node-auth/" + n.ID + "/pull/extra",
		"/api/v1/xray/center/node-auth//pull",
		"/api/v1/xray/center/node-authx/" + n.ID + "/pull",
	} {
		resp = post(p, next)
		resp.Body.Close()
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			t.Fatalf("non-canonical node-auth path admitted: %s %d", p, resp.StatusCode)
		}
	}
	// Negative: a node credential grants no operator privileges.
	for _, p := range []string{
		"/api/v1/xray/center/nodes",
		"/api/v1/xray/center/nodes/" + n.ID + "/rotate",
		"/api/v1/xray/center/tokens",
	} {
		req, err := http.NewRequest(http.MethodPost, srv.URL+p, strings.NewReader("{}"))
		req.Host = "127.0.0.1:8080"
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("X-Node-Credential", next)
		req.Header.Set("Authorization", "Bearer "+next)
		resp, err = srv.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			t.Fatalf("node principal reached operator route %s: %d", p, resp.StatusCode)
		}
	}
}
