// Package main / closure_test.go: F-04 closure — stronger
// process-level replacements for the historical tests that assumed
// the removed literal credential flags. All invocations use the
// protected stdin contract.
package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/center"
	"github.com/CrazyDuckLab/DuckOS/internal/persist"
	"github.com/CrazyDuckLab/DuckOS/internal/xnode"
)

func TestClosureServerProcessHelper(t *testing.T) {
	role := os.Getenv("DUCK_FINAL_SERVER_ROLE")
	if role == "" {
		return
	}
	dir := os.Getenv("DUCK_FINAL_SERVER_DIR")
	var srv *httptest.Server
	if role == "node" {
		local, err := xnode.OpenNodeIdentity(dir)
		if err != nil {
			t.Fatal(err)
		}
		node := xnode.NewNodeServer(nil, func([]byte) error { return nil },
			func() xnode.NodeInfo { return xnode.NodeInfo{Name: "closure-child"} })
		node.Identity = local
		srv = httptest.NewTLSServer(node)
	} else {
		disk, err := persist.Open[xnode.DurableState](dir, "state.json")
		if err != nil {
			t.Fatal(err)
		}
		m := xnode.NewManager(nil)
		m.LoadState(disk.Get())
		m.SetPersist(func(st xnode.DurableState) (xnode.CommitResult, error) {
			out, e := disk.Update(func(p *xnode.DurableState) { *p = st })
			return xnode.CommitResult{Uncertain: out == persist.CommitUncertain}, e
		})
		if os.Getenv("DUCK_FINAL_STAGE") == "1" {
			if _, _, err = m.StageRotation(os.Getenv("DUCK_FINAL_NODE_ID")); err != nil {
				t.Fatal(err)
			}
		}
		srv = httptest.NewServer(&center.Handler{Access: b4CLIAuth{}, Prefix: "api/v1/xray/center/", Nodes: m})
	}
	meta := struct {
		URL  string
		Cert []byte
	}{URL: srv.URL}
	if role == "node" {
		meta.Cert = srv.Certificate().Raw
	}
	json.NewEncoder(os.Stdout).Encode(meta)
	buf := make([]byte, 1)
	os.Stdin.Read(buf)
	srv.Close()
	os.Exit(0)
}


func TestClosureCLIHelper(t *testing.T) {
	if os.Getenv("DUCK_FINAL_CLI_HELPER") != "1" {
		return
	}
	for i, arg := range os.Args {
		if arg == "--" {
			if err := dispatch(os.Args[i+1:]); err != nil {
				os.Stderr.WriteString(err.Error())
				os.Exit(1)
			}
			os.Exit(0)
		}
	}
	os.Exit(2)
}

// Process-level lost-ACK recovery with the protected stdin contract
// (replacement for the historical TestFinalCLIProcessLostAckAndRestart,
// which invoked the CLI through the removed --credential flag).
func TestClosureCLIProcessLostAckStdin(t *testing.T) {
	m, id, old, next, live, dir, h := b4rCLIFixture(t)
	registry := filepath.Join(t.TempDir(), "registry")
	disk, err := persist.Open[xnode.DurableState](registry, "state.json")
	if err != nil {
		t.Fatal(err)
	}
	m.SetPersist(func(st xnode.DurableState) (xnode.CommitResult, error) {
		out, e := disk.Update(func(p *xnode.DurableState) { *p = st })
		return xnode.CommitResult{Uncertain: out == persist.CommitUncertain}, e
	})
	ns := xnode.NewNodeServer(nil, func([]byte) error { return nil },
		func() xnode.NodeInfo { return xnode.NodeInfo{Name: "closure"} })
	ns.Identity = live
	tls := httptest.NewTLSServer(ns)
	defer tls.Close()
	adapter := xnode.NewNodeHTTPAdapter()
	adapter.HTTPClient = tls.Client()
	node := xnode.Node{ID: id, Endpoint: tls.URL}
	var mu sync.Mutex
	drop := true
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if drop && strings.HasSuffix(r.URL.Path, "/confirm") {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, r)
			conn, _, e := w.(http.Hijacker).Hijack()
			if e != nil {
				t.Error(e)
				return
			}
			conn.Close()
			return
		}
		h.ServeHTTP(w, r)
	}))
	defer srv.Close()
	run := func(recover bool) ([]byte, error) {
		args := []string{"-test.run=^TestClosureCLIHelper$", "--", "xray", "nodes", "pull-rotation", "--id", id, "--identity-dir", dir, "--endpoint", srv.URL}
		if recover {
			args = append(args, "--finalize")
		} else {
			args = append(args, "--credential-stdin")
		}
		cmd := exec.Command(os.Args[0], args...)
		cmd.Env = append(os.Environ(), "DUCK_FINAL_CLI_HELPER=1")
		if !recover {
			cmd.Stdin = strings.NewReader(old + "\n")
		}
		return cmd.CombinedOutput()
	}
	out, err := run(false)
	if err == nil {
		t.Fatalf("false success on lost ack: %s", out)
	}
	if strings.Contains(string(out), next) {
		t.Fatal("secret disclosed")
	}
	reopened, e := persist.Open[xnode.DurableState](registry, "state.json")
	if e != nil {
		t.Fatal(e)
	}
	fresh := xnode.NewManager(nil)
	fresh.LoadState(reopened.Get())
	mu.Lock()
	h.Nodes = fresh
	drop = false
	mu.Unlock()
	out, err = run(true)
	if err != nil {
		t.Fatalf("recovery after controller reopen: %v %s", err, out)
	}
	t.Logf("recovery output: %s", out)
	if strings.Contains(string(out), next) || !strings.Contains(string(out), "rotation complete") {
		t.Fatal("invalid output")
	}
	if fresh.Authenticate(id, old) || !fresh.Authenticate(id, next) {
		t.Fatal("controller auth inconsistent")
	}
	if _, err = adapter.Discover(xnode.WithCredential(context.Background(), old), node); err == nil {
		t.Fatal("live old accepted")
	}
	if _, err = adapter.Discover(xnode.WithCredential(context.Background(), next), node); err != nil {
		t.Fatal(err)
	}
	restarted, e := xnode.OpenNodeIdentity(dir)
	if e != nil {
		t.Fatal(e)
	}
	if restarted.Accepts(old) || !restarted.Accepts(next) || restarted.HasPending() {
		t.Fatal("node restart inconsistent")
	}
}

// Process-level double server restart around a real CLI rotation with
// the protected stdin contract (replacement for the historical
// TestFinalBothServerProcessesRestartAfterCLI).
func TestClosureBothServerProcessesRestartStdin(t *testing.T) {
	root := t.TempDir()
	ctrlDir := filepath.Join(root, "controller")
	nodeDir := filepath.Join(root, "node")
	disk, err := persist.Open[xnode.DurableState](ctrlDir, "state.json")
	if err != nil {
		t.Fatal(err)
	}
	m := xnode.NewManager(nil)
	m.SetPersist(func(st xnode.DurableState) (xnode.CommitResult, error) {
		out, e := disk.Update(func(d *xnode.DurableState) { *d = st })
		return xnode.CommitResult{Uncertain: out == persist.CommitUncertain}, e
	})
	tok, _, err := m.IssueEnrollmentToken(time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	n, old, err := m.Enroll(tok, xnode.NodeInfo{Name: "closure-restart"})
	if err != nil {
		t.Fatal(err)
	}
	local, err := xnode.OpenNodeIdentity(nodeDir)
	if err != nil {
		t.Fatal(err)
	}
	if err = local.Adopt(old); err != nil {
		t.Fatal(err)
	}
	start := func(role, dir, stage string) (string, *http.Client, func()) {
		cmd := exec.Command(os.Args[0], "-test.run=^TestClosureServerProcessHelper$")
		cmd.Env = append(os.Environ(), "DUCK_FINAL_SERVER_ROLE="+role, "DUCK_FINAL_SERVER_DIR="+dir, "DUCK_FINAL_STAGE="+stage, "DUCK_FINAL_NODE_ID="+n.ID)
		in, e := cmd.StdinPipe()
		if e != nil {
			t.Fatal(e)
		}
		out, e := cmd.StdoutPipe()
		if e != nil {
			t.Fatal(e)
		}
		if e = cmd.Start(); e != nil {
			t.Fatal(e)
		}
		reader := bufio.NewReader(out)
		line, e := reader.ReadBytes('\n')
		if e != nil {
			t.Fatal(e)
		}
		var meta struct {
			URL  string
			Cert []byte
		}
		if e = json.Unmarshal(line, &meta); e != nil {
			t.Fatal(e)
		}
		client := &http.Client{}
		if role == "node" {
			cert, e := x509.ParseCertificate(meta.Cert)
			if e != nil {
				t.Fatal(e)
			}
			roots := x509.NewCertPool()
			roots.AddCert(cert)
			client.Transport = &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots}}
		}
		var once sync.Once
		stop := func() {
			once.Do(func() {
				in.Close()
				if e := cmd.Wait(); e != nil {
					t.Error(e)
				}
				client.CloseIdleConnections()
			})
		}
		t.Cleanup(stop)
		return meta.URL, client, stop
	}
	nodeURL, nodeClient, stopNode := start("node", nodeDir, "0")
	ctrlURL, _, stopCtrl := start("controller", ctrlDir, "1")
	next, err := nodeAuthDirect(ctrlURL, n.ID, "pull", old)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestClosureCLIHelper$", "--", "xray", "nodes", "pull-rotation", "--id", n.ID, "--identity-dir", nodeDir, "--endpoint", ctrlURL, "--credential-stdin")
	cmd.Env = append(os.Environ(), "DUCK_FINAL_CLI_HELPER=1")
	cmd.Stdin = strings.NewReader(old + "\n")
	if out, e := cmd.CombinedOutput(); e != nil {
		t.Fatalf("CLI failed: %v %s", e, out)
	}
	verifyNode := func(url string, client *http.Client) {
		adapter := xnode.NewNodeHTTPAdapter()
		adapter.HTTPClient = client
		node := xnode.Node{ID: n.ID, Endpoint: url}
		if _, e := adapter.Discover(xnode.WithCredential(context.Background(), old), node); e == nil {
			t.Fatal("old node credential accepted")
		}
		if _, e := adapter.Discover(xnode.WithCredential(context.Background(), next), node); e != nil {
			t.Fatal(e)
		}
	}
	verifyNode(nodeURL, nodeClient)
	stopNode()
	stopCtrl()
	nodeURL, nodeClient, _ = start("node", nodeDir, "0")
	ctrlURL, _, _ = start("controller", ctrlDir, "0")
	verifyNode(nodeURL, nodeClient)
	if state, e := nodeAuthDirectState(ctrlURL, n.ID, "confirm", next); e != nil || state != "CONFIRMED" {
		t.Fatalf("fresh controller process lost durable completion: %s %v", state, e)
	}
	if _, e := nodeAuthDirectState(ctrlURL, n.ID, "confirm", old); e == nil {
		t.Fatal("fresh controller accepted old confirmation")
	}
	reopened, e := persist.Open[xnode.DurableState](ctrlDir, "state.json")
	if e != nil {
		t.Fatal(e)
	}
	fresh := xnode.NewManager(nil)
	fresh.LoadState(reopened.Get())
	if fresh.Authenticate(n.ID, old) || !fresh.Authenticate(n.ID, next) {
		t.Fatal("durable registry mismatch")
	}
}

// In-process equivalent of the historical
// TestFinalUnavailableControllerThenRecovery with the stdin contract:
// a failed confirm preserves the recoverable pending state, and
// recovery succeeds once the controller becomes available again.
func TestClosureUnavailableControllerThenRecoveryStdin(t *testing.T) {
	m, id, old, next, _, dir, h := b4rCLIFixture(t)
	var mu sync.Mutex
	unavailable := true
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		deny := unavailable && strings.HasSuffix(r.URL.Path, "/confirm")
		mu.Unlock()
		if deny {
			http.Error(w, "closure unavailable", 503)
			return
		}
		h.ServeHTTP(w, r)
	}))
	defer srv.Close()
	if err := b4rRun(b4rArgs(id, old, dir, srv.URL), old); err == nil {
		t.Fatal("false success while unavailable")
	}
	pending, err := xnode.OpenNodeIdentity(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !pending.HasPending() || !pending.Accepts(old) || m.Authenticate(id, next) {
		t.Fatal("pending lost or controller committed while unavailable")
	}
	mu.Lock()
	unavailable = false
	mu.Unlock()
	if err = b4rRun(append(b4rArgs(id, old, dir, srv.URL), "--finalize"), old); err != nil {
		t.Fatal(err)
	}
	final, err := xnode.OpenNodeIdentity(dir)
	if err != nil {
		t.Fatal(err)
	}
	if final.HasPending() || final.Accepts(old) || !final.Accepts(next) {
		t.Fatal("restored controller did not recover")
	}
}
