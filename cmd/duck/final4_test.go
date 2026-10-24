// Package main / final4_test.go: regressions for the final
// independent audit (2026-10-24) — F-01 live-TLS identity read fault
// after a real CLI rotation, and F-04 process-argument
// confidentiality of the current node credential.
package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/CrazyDuckLab/DuckOS/internal/xnode"
)

func TestFinalCLIHelper(t *testing.T) {
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

// F-01: after an ACTUAL CLI rotation completed, a read fault on the
// authoritative identity document must fail the prestarted live TLS
// node closed — the old credential (already rejected by the
// controller) must not be resurrected from the stale in-memory
// cache.
func TestFinalCLISuccessThenIdentityReadFault(t *testing.T) {
	m, id, old, next, live, dir, h := b4rCLIFixture(t)
	server := xnode.NewNodeServer(nil, func([]byte) error { return nil },
		func() xnode.NodeInfo { return xnode.NodeInfo{Name: "prestarted"} })
	server.Identity = live
	tls := httptest.NewTLSServer(server)
	defer tls.Close()
	adapter := xnode.NewNodeHTTPAdapter()
	adapter.HTTPClient = tls.Client()
	node := xnode.Node{ID: id, Endpoint: tls.URL}
	controller := httptest.NewServer(h)
	defer controller.Close()
	if err := xrayNodesCmd(b4rArgs(id, old, dir, controller.URL)); err != nil {
		t.Fatal(err)
	}
	if m.Authenticate(id, old) || !m.Authenticate(id, next) {
		t.Fatal("controller not committed")
	}
	path := filepath.Join(dir, "node-identity.json")
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(path, 0o600)
	if _, err := adapter.Discover(xnode.WithCredential(context.Background(), old), node); err == nil {
		t.Fatal("actual CLI returned success; controller rejects old, but prestarted live TLS node accepts old when identity reload fails")
	}
	// Recovery: once the document is readable again, the live node
	// authenticates against the authoritative state.
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.Discover(xnode.WithCredential(context.Background(), next), node); err != nil {
		t.Fatalf("no automatic recovery after the fault cleared: %v", err)
	}
	if _, err := adapter.Discover(xnode.WithCredential(context.Background(), old), node); err == nil {
		t.Fatal("old credential accepted after recovery")
	}
}

// F-04: the current node credential supplied through the legacy
// --credential form must be scrubbed from the actual running CLI
// process arguments before any network I/O.
func TestFinalCurrentCredentialProcessArguments(t *testing.T) {
	_, id, old, _, _, dir, h := b4rCLIFixture(t)
	entered := make(chan struct{})
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/pull") {
			close(entered)
			<-release
		}
		h.ServeHTTP(w, r)
	}))
	defer srv.Close()
	cmd := exec.Command(os.Args[0], "-test.run=^TestFinalCLIHelper$", "--", "xray", "nodes", "pull-rotation", "--id", id, "--identity-dir", dir, "--endpoint", srv.URL, "--credential", old)
	cmd.Env = append(os.Environ(), "DUCK_FINAL_CLI_HELPER=1")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	<-entered
	out, psErr := exec.Command("/bin/ps", "-p", strconv.Itoa(cmd.Process.Pid), "-o", "args=").Output()
	close(release)
	runErr := cmd.Wait()
	if psErr != nil {
		t.Fatal(psErr)
	}
	if runErr != nil {
		t.Fatal(runErr)
	}
	if strings.Contains(string(out), old) {
		t.Fatal("current node credential is readable in actual running CLI process arguments (value suppressed)")
	}
}

// F-04: the protected stdin form keeps the current credential out of
// argv entirely and works end to end.
func TestFinalCurrentCredentialFromStdin(t *testing.T) {
	_, id, old, next, _, dir, h := b4rCLIFixture(t)
	srv := httptest.NewServer(h)
	defer srv.Close()
	cmd := exec.Command(os.Args[0], "-test.run=^TestFinalCLIHelper$", "--", "xray", "nodes", "pull-rotation", "--id", id, "--identity-dir", dir, "--endpoint", srv.URL, "--credential-stdin")
	cmd.Env = append(os.Environ(), "DUCK_FINAL_CLI_HELPER=1")
	cmd.Stdin = strings.NewReader(old + "\n")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("stdin credential flow failed: %v %s", err, out)
	}
	for _, arg := range cmd.Args {
		if strings.Contains(arg, old) {
			t.Fatal("current credential in argv with the protected stdin form")
		}
	}
	if strings.Contains(string(out), next) {
		t.Fatal("new credential in output")
	}
	reopened, err := xnode.OpenNodeIdentity(dir)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.Accepts(old) || !reopened.Accepts(next) || reopened.HasPending() {
		t.Fatal("stdin flow did not complete the rotation")
	}
	// Positional secrets are rejected without being echoed back.
	cmd = exec.Command(os.Args[0], "-test.run=^TestFinalCLIHelper$", "--", "xray", "nodes", "pull-rotation", "--id", id, "--identity-dir", dir, "--endpoint", srv.URL, old)
	cmd.Env = append(os.Environ(), "DUCK_FINAL_CLI_HELPER=1")
	out, err = cmd.CombinedOutput()
	if err == nil {
		t.Fatal("positional credential accepted")
	}
	if strings.Contains(string(out), old) {
		t.Fatal("rejected positional credential echoed in the error output")
	}
}
