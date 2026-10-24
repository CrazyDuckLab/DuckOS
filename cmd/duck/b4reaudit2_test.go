// Package main / b4reaudit2_test.go: regressions for the second
// independent re-audit (2026-10-23) — live-node adoption through the
// real CLI, controller-authoritative recovery, fail-closed pending
// handling and secret containment (redirects, stdout, argv).
package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/center"
	"github.com/CrazyDuckLab/DuckOS/internal/persist"
	"github.com/CrazyDuckLab/DuckOS/internal/xnode"
)

func b4rCLIFixture(t *testing.T) (*xnode.Manager, string, string, string, *xnode.NodeIdentity, string, *center.Handler) {
	t.Helper()
	m := xnode.NewManager(nil)
	tok, _, err := m.IssueEnrollmentToken(time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	n, old, err := m.Enroll(tok, xnode.NodeInfo{Name: "b4r-cli"})
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "node")
	identity, err := xnode.OpenNodeIdentity(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := identity.Adopt(old); err != nil {
		t.Fatal(err)
	}
	next, _, err := m.StageRotation(n.ID)
	if err != nil {
		t.Fatal(err)
	}
	h := &center.Handler{Access: b4CLIAuth{}, Prefix: "api/v1/xray/center/", Nodes: m}
	return m, n.ID, old, next, identity, dir, h
}

// b4rArgs builds the secure (F-04) invocation: the current credential
// travels ONLY through stdin (--credential-stdin).
func b4rArgs(id, old, dir, endpoint string) []string {
	return []string{"pull-rotation", "--id", id, "--credential-stdin", "--identity-dir", dir, "--endpoint", endpoint}
}

// b4rRun invokes the real CLI with the current credential piped
// through stdin (protected input).
func b4rRun(args []string, cred string) error {
	r, w, err := os.Pipe()
	if err != nil {
		return err
	}
	if _, err := w.WriteString(cred + "\n"); err != nil {
		w.Close()
		r.Close()
		return err
	}
	w.Close()
	oldStdin := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = oldStdin; r.Close() }()
	return xrayNodesCmd(args)
}

// The audit blocker: the CLI must not report success while a RUNNING
// node server still uses the stale in-memory credential. The server
// is started BEFORE the rotation; the CLI runs against the same
// durable identity store through its own instance.
func TestB401_CLILiveNodeRotationAdoptedByRunningServer(t *testing.T) {
	m, id, old, next, live, dir, h := b4rCLIFixture(t)
	srv := httptest.NewServer(h)
	defer srv.Close()
	node := xnode.NewNodeServer(nil, func([]byte) error { return nil },
		func() xnode.NodeInfo { return xnode.NodeInfo{Name: "live"} })
	node.Identity = live
	tlsNode := httptest.NewTLSServer(node)
	defer tlsNode.Close()
	adapter := xnode.NewNodeHTTPAdapter()
	adapter.HTTPClient = tlsNode.Client()
	if err := b4rRun(b4rArgs(id, old, dir, srv.URL), old); err != nil {
		t.Fatal(err)
	}
	reopened, err := xnode.OpenNodeIdentity(dir)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.Accepts(old) || !reopened.Accepts(next) || m.Authenticate(id, old) || !m.Authenticate(id, next) {
		t.Fatal("durable final state incorrect")
	}
	n := xnode.Node{ID: id, Endpoint: tlsNode.URL}
	if _, err := adapter.Discover(xnode.WithCredential(context.Background(), old), n); err == nil {
		t.Fatal("running node still accepts the old credential after CLI success")
	}
	if _, err := adapter.Discover(xnode.WithCredential(context.Background(), next), n); err != nil {
		t.Fatalf("running node did not adopt the new credential: %v", err)
	}
}

// Lost acknowledgment AFTER a durable controller commit: the plain
// CLI run reports an error (no false success), the pending state
// keeps both credentials, and an explicit --finalize recovery
// re-confirms against the controller (idempotent tombstone replay)
// before finalizing locally.
func TestB401_RecoveryFinalizeAfterLostAck(t *testing.T) {
	m, id, old, next, _, dir, h := b4rCLIFixture(t)
	confirms := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/confirm") {
			confirms++
			if confirms == 1 {
				// The FIRST confirm commits controller-side but its
				// response is lost (hijacked connection).
				rec := httptest.NewRecorder()
				h.ServeHTTP(rec, r)
				conn, _, err := w.(http.Hijacker).Hijack()
				if err != nil {
					t.Error(err)
					return
				}
				conn.Close()
				return
			}
		}
		h.ServeHTTP(w, r)
	}))
	defer srv.Close()
	if err := b4rRun(b4rArgs(id, old, dir, srv.URL), old); err == nil {
		t.Fatal("false success after lost response")
	}
	local, err := xnode.OpenNodeIdentity(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !local.HasPending() || !local.Accepts(old) || !local.Accepts(next) ||
		!m.Authenticate(id, next) || m.Authenticate(id, old) {
		t.Fatal("lost ack state wrong")
	}
	// A plain rerun must not silently recover anything.
	if err := b4rRun(b4rArgs(id, old, dir, srv.URL), old); err == nil {
		t.Fatal("plain rerun unexpectedly recovered the pending state")
	}
	// Controller-authoritative recovery: the tombstone replay
	// returns CONFIRMED and only then is the local transition
	// finalized.
	recovered := append(b4rArgs(id, old, dir, srv.URL), "--finalize")
	if err := b4rRun(recovered, old); err != nil {
		t.Fatalf("--finalize recovery failed: %v", err)
	}
	reopened, err := xnode.OpenNodeIdentity(dir)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.HasPending() || reopened.Accepts(old) || !reopened.Accepts(next) {
		t.Fatal("recovery did not finalize the local identity")
	}
}

// Recovery must fail CLOSED when the controller cannot verify the
// outcome: local finalization without proven controller commit would
// strand the node. This supersedes the re-audit's offline-finalize
// helper assertion (TestReauditFinalizeFlag), which codified the
// unsafe no-network finalization the re-audit itself flagged as a
// latent defect.
func TestB401_RecoveryFailsClosedWithoutController(t *testing.T) {
	m, id, old, next, _, dir, _ := b4rCLIFixture(t)
	local, err := xnode.OpenNodeIdentity(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := local.BeginTransition(next); err != nil {
		t.Fatal(err)
	}
	args := append(b4rArgs(id, old, dir, "http://127.0.0.1:1"), "--finalize")
	if err := b4rRun(args, old); err == nil {
		t.Fatal("offline finalize succeeded without controller proof")
	}
	reopened, err := xnode.OpenNodeIdentity(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !reopened.HasPending() || !reopened.Accepts(old) || !reopened.Accepts(next) {
		t.Fatal("fail-closed recovery must preserve the recoverable pending state")
	}
	if m.Authenticate(id, next) {
		t.Fatal("controller must remain uncommitted")
	}
}

// A pending write failure must abort BEFORE the controller confirm:
// no durable pending means no confirmation.
func TestB401_PendingWriteFailureAbortsBeforeConfirm(t *testing.T) {
	m, id, old, next, _, dir, h := b4rCLIFixture(t)
	srv := httptest.NewServer(h)
	defer srv.Close()
	if err := os.Mkdir(filepath.Join(dir, "node-identity.json.tmp"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := b4rRun(b4rArgs(id, old, dir, srv.URL), old); err == nil {
		t.Fatal("false success on pending persistence failure")
	}
	if !m.Authenticate(id, old) || m.Authenticate(id, next) {
		t.Fatal("confirmed before durable pending")
	}
}

// An uncertain local finalization (directory fsync failure after the
// rename) is reported as an error — never as success — while the
// controller commit stands and the state stays recoverable.
func TestB401_LocalFinalizeUncertaintyReported(t *testing.T) {
	m, id, old, next, _, dir, h := b4rCLIFixture(t)
	srv := httptest.NewServer(h)
	defer srv.Close()
	calls := 0
	restore := persist.SyncDirForTest(func(string) error {
		calls++
		if calls == 2 {
			return errors.New("injected finalization directory fsync EIO")
		}
		return nil
	})
	defer restore()
	if err := b4rRun(b4rArgs(id, old, dir, srv.URL), old); err == nil {
		t.Fatal("false success on uncertain local finalize")
	}
	if !m.Authenticate(id, next) {
		t.Fatal("controller did not commit")
	}
	// The uncertain commit most plausibly landed (post-rename): the
	// local identity is already on the new credential, and a rerun
	// with the old credential fails closed instead of resurrecting
	// anything.
	reopened, err := xnode.OpenNodeIdentity(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !reopened.Accepts(next) {
		t.Fatal("uncertain finalization lost the new credential locally")
	}
	if err := b4rRun(b4rArgs(id, old, dir, srv.URL), old); err == nil {
		t.Fatal("rerun with the stale credential must not silently succeed")
	}
}

// A failed confirm BEFORE the controller commit leaves the same
// local pending shape as a lost ack — the CLI must not finalize
// either of them locally.
func TestB401_ConfirmUnavailableLeavesRecoverablePending(t *testing.T) {
	m, id, old, next, _, dir, h := b4rCLIFixture(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/confirm") {
			http.Error(w, "injected unavailable", http.StatusServiceUnavailable)
			return
		}
		h.ServeHTTP(w, r)
	}))
	defer srv.Close()
	if err := b4rRun(b4rArgs(id, old, dir, srv.URL), old); err == nil {
		t.Fatal("false success")
	}
	local, err := xnode.OpenNodeIdentity(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !local.HasPending() || !local.Accepts(old) || !local.Accepts(next) ||
		!m.Authenticate(id, old) || m.Authenticate(id, next) {
		t.Fatal("failed confirm state incorrect")
	}
}

// Secret containment: a node credential must never follow a
// redirect — not even from a trusted HTTPS controller to another
// origin over plaintext HTTP.
func TestSecrets_RedirectDoesNotDiscloseNodeCredential(t *testing.T) {
	received := ""
	sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received = r.Header.Get("X-Node-Credential")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"state":"CONFIRMED"}`))
	}))
	defer sink.Close()
	controller := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, sink.URL+"/collect", http.StatusTemporaryRedirect)
	}))
	defer controller.Close()
	oldTransport := http.DefaultTransport
	http.DefaultTransport = controller.Client().Transport
	defer func() { http.DefaultTransport = oldTransport }()
	_, _, _ = nodeAuthDirectFull(controller.URL, "synthetic-node", "confirm", "synthetic-node-credential-redirect-canary")
	if received != "" {
		t.Fatal("credential header followed verified HTTPS controller redirect to another origin over plaintext HTTP")
	}
}

// Secret containment: the CLI must not print the new credential to
// stdout (the durable local identity store is the only place it
// lives during the transition).
func TestSecrets_CLIRotatesWithoutPrintingCredential(t *testing.T) {
	_, id, old, next, _, dir, h := b4rCLIFixture(t)
	srv := httptest.NewServer(h)
	defer srv.Close()
	stdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	cmdErr := b4rRun(b4rArgs(id, old, dir, srv.URL), old)
	w.Close()
	os.Stdout = stdout
	out, _ := io.ReadAll(r)
	if cmdErr != nil {
		t.Fatal(cmdErr)
	}
	if strings.Contains(string(out), next) {
		t.Fatalf("new credential leaked to stdout: %q", string(out))
	}
	if !strings.Contains(string(out), "rotation complete") {
		t.Fatalf("unexpected CLI output: %q", string(out))
	}
}

// Ordering proof over a verified TLS controller: the durable pending
// state exists BEFORE the confirm request arrives.
func TestB401_TrustedTLSControllerPendingBeforeConfirm(t *testing.T) {
	m, id, old, next, _, dir, h := b4rCLIFixture(t)
	seenPending := make(chan bool, 1)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/confirm") {
			state, err := xnode.OpenNodeIdentity(dir)
			if err != nil {
				t.Error(err)
			} else {
				seenPending <- state.HasPending() && state.Accepts(next)
			}
		}
		h.ServeHTTP(w, r)
	}))
	defer srv.Close()
	transport := http.DefaultTransport
	http.DefaultTransport = srv.Client().Transport
	defer func() { http.DefaultTransport = transport }()
	if err := b4rRun(b4rArgs(id, old, dir, srv.URL), old); err != nil {
		t.Fatal(err)
	}
	if !<-seenPending || m.Authenticate(id, old) || !m.Authenticate(id, next) {
		t.Fatal("ordering or controller state wrong")
	}
}
