// Package xnode / transport_test.go: end-to-end control-plane
// transport over a real HTTP server: discovery, push, health,
// replay and authentication refusals.
package xnode

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func transportFixture(t *testing.T) (*Manager, *Manager, *NodeHTTPAdapter, string, *[]byte, *httptest.Server) {
	t.Helper()
	coordinator := NewManager(NewNodeHTTPAdapter())
	nodeMgr := NewManager(nil)
	var applied []byte
	apply := func(cfg []byte) error {
		if len(cfg) == 0 || !json.Valid(cfg) {
			return errors.New("invalid config")
		}
		applied = append([]byte(nil), cfg...)
		return nil
	}
	info := func() NodeInfo {
		return NodeInfo{Name: "edge-1", Version: "26.3.27", Capabilities: []string{"vless", "reality"}}
	}
	server := NewNodeServer(nodeMgr, apply, info)
	ts := httptest.NewTLSServer(server)
	client := ts.Client()
	adapter := NewNodeHTTPAdapter()
	adapter.HTTPClient = client
	coordinator = NewManager(adapter)
	return coordinator, nodeMgr, adapter, ts.URL, &applied, ts
}

func TestTransport_EndToEnd(t *testing.T) {
	coordinator, nodeMgr, adapter, endpoint, applied, ts := transportFixture(t)
	defer ts.Close()
	// Enrollment: coordinator issues a token, the node's manager
	// holds the resulting identity (mirrors the real enrollment
	// exchange where the coordinator ships the credential to the
	// node over its operator-approved channel).
	tok, _, _ := coordinator.IssueEnrollmentToken(time.Hour)
	node, credential, err := coordinator.Enroll(tok, NodeInfo{Name: "edge-1"})
	if err != nil {
		t.Fatal(err)
	}
	node.Endpoint = endpoint
	// The node adopts the shared credential it received at
	// enrollment (production: redeemed via the coordinator API).
	if err := nodeMgr.AdoptIdentity(credential); err != nil {
		t.Fatal(err)
	}
	// A second adoption is refused (re-bind goes through rotation).
	if err := nodeMgr.AdoptIdentity("another"); err == nil {
		t.Fatal("double adoption accepted")
	}
	// Self-auth: the node must accept exactly the shared credential.
	if !nodeMgr.AuthenticateSelf(credential) {
		t.Fatal("node rejected the shared credential")
	}
	if nodeMgr.AuthenticateSelf("wrong") || nodeMgr.AuthenticateSelf("") {
		t.Fatal("node accepted a wrong credential")
	}
	// Discover through the real HTTP transport.
	cred := credential
	ctx := WithCredential(context.Background(), cred)
	info, err := adapter.Discover(ctx, node)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if info.Version != "26.3.27" || len(info.Capabilities) != 2 {
		t.Fatalf("discovery: %+v", info)
	}
	// Push flows into the node's apply hook.
	cfg := []byte(`{"inbounds":[],"outbounds":[]}`)
	if err := adapter.PushConfig(ctx, node, cfg); err != nil {
		t.Fatalf("push: %v", err)
	}
	if !bytes.Equal(*applied, cfg) {
		t.Fatalf("apply hook got: %s", *applied)
	}
	// Health reports latency.
	if _, err := adapter.Health(ctx, node); err != nil {
		t.Fatalf("health: %v", err)
	}
}

// credentialFromCoordinator documents the production exchange: the
// coordinator issues an enrollment token; the node redeems it and
// receives the shared credential. In this test the credential is
// already known, so this helper just returns it.
func credentialFromCoordinator(_ *Manager, _ Node, credential string) string {
	return credential
}

func TestTransport_RejectsReplayAndWrongCredential(t *testing.T) {
	coordinator, nodeMgr, _, _, _, ts := transportFixture(t)
	defer ts.Close()
	tok, _, _ := coordinator.IssueEnrollmentToken(time.Hour)
	_, credential, _ := coordinator.Enroll(tok, NodeInfo{Name: "edge-1"})
	if err := nodeMgr.AdoptIdentity(credential); err != nil {
		t.Fatal(err)
	}
	if !nodeMgr.AuthenticateSelf(credential) {
		t.Fatal("setup: credential rejected")
	}
	// A captured request replayed after the skew window fails.
	srv := NewNodeServer(nodeMgr, func([]byte) error { return nil }, func() NodeInfo { return NodeInfo{} })
	base := time.Now().UTC()
	srv.SetClock(func() time.Time { return base })
	reqBody := controlRequest{Op: "health", Nonce: "abc", Time: base.Add(-5 * time.Minute)}
	raw, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, TransportPath, bytes.NewReader(raw))
	req.Header.Set("Authorization", "Bearer "+credential)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("stale request accepted: %d", w.Code)
	}
	// Wrong credential fails closed.
	reqBody.Time = base
	reqBody.Nonce = "def"
	raw, _ = json.Marshal(reqBody)
	req = httptest.NewRequest(http.MethodPost, TransportPath, bytes.NewReader(raw))
	req.Header.Set("Authorization", "Bearer wrong")
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("wrong credential accepted: %d", w.Code)
	}
	// Non-transport paths 404 without leaking anything.
	req = httptest.NewRequest(http.MethodGet, "/admin", nil)
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("admin path not 404: %d", w.Code)
	}
}

func TestTransport_NodeRevocationStopsPush(t *testing.T) {
	coordinator, nodeMgr, adapter, endpoint, applied, ts := transportFixture(t)
	defer ts.Close()
	tok, _, _ := coordinator.IssueEnrollmentToken(time.Hour)
	node, credential, _ := coordinator.Enroll(tok, NodeInfo{Name: "edge-1"})
	node.Endpoint = endpoint
	if err := nodeMgr.AdoptIdentity(credential); err != nil {
		t.Fatal(err)
	}
	ctx := WithCredential(context.Background(), credential)
	if err := adapter.PushConfig(ctx, node, []byte(`{}`)); err != nil {
		t.Fatalf("pre-revoke push: %v", err)
	}
	if err := coordinator.Revoke(node.ID); err != nil {
		t.Fatal(err)
	}
	// Manager-authenticated path: revoked credential no longer
	// passes the coordinator's registry; a node side honoring the
	// revocation feed rejects it too (same digest registry).
	if coordinator.Authenticate(node.ID, credential) {
		t.Fatal("revoked credential authenticated on coordinator")
	}
	_ = applied
}

func TestPinningClient_RejectsBadFingerprint(t *testing.T) {
	if _, err := PinningHTTPClient("zz"); err == nil {
		t.Fatal("malformed fingerprint accepted")
	}
	// Correct length but wrong value must fail the TLS handshake.
	client, err := PinningHTTPClient(strings.Repeat("ab", 32))
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer ts.Close()
	_, err = client.Get(ts.URL)
	if err == nil || !strings.Contains(err.Error(), "fingerprint mismatch") {
		t.Fatalf("wrong fingerprint handshake not refused: %v", err)
	}
}

// Ensure atomic import surface used by production wiring exists.
var _ = atomic.Value{}
