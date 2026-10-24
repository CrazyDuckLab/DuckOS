// Package main / b4_cli_rotation_test.go: B4-01 regression through
// the ACTUAL CLI entrypoint (xrayNodesCmd), separate durable
// controller and node stores, and a real TLS node server — ported
// from the independent audit with harness adaptation (the
// --identity-dir/--endpoint flags; all assertions preserved).
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/center"
	"github.com/CrazyDuckLab/DuckOS/internal/identity"
	"github.com/CrazyDuckLab/DuckOS/internal/persist"
	"github.com/CrazyDuckLab/DuckOS/internal/xnode"
)

type b4CLIAuth struct{}

func (b4CLIAuth) Authorize(token string, _ identity.Permission) (identity.User, error) {
	if token != "b4-synthetic-operator-credential" {
		return identity.User{}, errors.New("invalid operator")
	}
	return identity.User{Username: "audit-operator"}, nil
}

// The audit blocker, re-expressed against the remediated CLI: a real
// xrayNodesCmd pull-rotation run must durably adopt and finalize the
// credential on the node, so after restarts of both sides the OLD
// credential is rejected and the NEW accepted everywhere.
func TestB401_CLIRotationDurableEndToEnd(t *testing.T) {
	root := t.TempDir()
	// Controller with durable state.
	disk, err := persist.Open[xnode.DurableState](filepath.Join(root, "controller"), "state.json")
	if err != nil {
		t.Fatal(err)
	}
	m := xnode.NewManager(nil)
	m.SetPersist(func(st xnode.DurableState) (xnode.CommitResult, error) {
		out, err := disk.Update(func(d *xnode.DurableState) { *d = st })
		return xnode.CommitResult{Uncertain: out == persist.CommitUncertain}, err
	})
	tok, _, err := m.IssueEnrollmentToken(time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	node, old, err := m.Enroll(tok, xnode.NodeInfo{Name: "cli-audit-node"})
	if err != nil {
		t.Fatal(err)
	}
	// Node role: its own durable identity store.
	nodeDir := filepath.Join(root, "node")
	nodeIdentity, err := xnode.OpenNodeIdentity(nodeDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := nodeIdentity.Adopt(old); err != nil {
		t.Fatal(err)
	}
	// Stage via the center handler mounted exactly as production.
	h := &center.Handler{
		Access: b4CLIAuth{},
		Prefix: "api/v1/xray/center/",
		Nodes:  m,
	}
	srv := httptest.NewServer(h)
	defer srv.Close()
	// Stage rotation through the real HTTP API; capture the staged
	// credential from the response (the CLI prints it exactly once
	// and persists it durably as pending).
	staged := b4Stage(t, srv.URL, node.ID, "b4-synthetic-operator-credential")
	// Operator token file (accepted for CLI symmetry).
	tokenFile := filepath.Join(root, "op-token")
	if err := os.WriteFile(tokenFile, []byte("b4-synthetic-operator-credential"), 0o600); err != nil {
		t.Fatal(err)
	}
	// THE ACTUAL CLI ENTRYPOINT: pull (old credential via protected
	// stdin, F-04 contract) → durable pending → confirm → local
	// finalize.
	stdinR, stdinW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stdinW.WriteString(old + "\n"); err != nil {
		t.Fatal(err)
	}
	stdinW.Close()
	oldStdin := os.Stdin
	os.Stdin = stdinR
	cmdErr := xrayNodesCmd([]string{
		"pull-rotation",
		"--id", node.ID,
		"--credential-stdin",
		"--endpoint", srv.URL,
		"--token-file", tokenFile,
		"--identity-dir", nodeDir,
	})
	os.Stdin = oldStdin
	stdinR.Close()
	if cmdErr != nil {
		t.Fatalf("CLI pull-rotation failed: %v", cmdErr)
	}
	// Restart the controller from durable state.
	restartedController := xnode.NewManager(nil)
	restartedController.LoadState(disk.Get())
	// Restart the node identity from its durable store.
	restartedNode, err := xnode.OpenNodeIdentity(nodeDir)
	if err != nil {
		t.Fatal(err)
	}
	// Real TLS node server for wire-level checks.
	server := xnode.NewNodeServer(nil, func([]byte) error { return nil },
		func() xnode.NodeInfo { return xnode.NodeInfo{Name: "restarted-node"} })
	server.Identity = restartedNode
	tlsNode := httptest.NewTLSServer(server)
	defer tlsNode.Close()
	node.Endpoint = tlsNode.URL
	adapter := xnode.NewNodeHTTPAdapter()
	adapter.HTTPClient = tlsNode.Client()
	// Old credential rejected over the node TLS wire.
	if _, err := adapter.Discover(
		xnode.WithCredential(context.Background(), old), node); err == nil {
		t.Error("BLOCKER CHECK: old credential accepted over node TLS after rotation")
	}
	// New credential accepted over the node TLS wire.
	if _, err := adapter.Discover(
		xnode.WithCredential(context.Background(), staged), node); err != nil {
		t.Errorf("new credential rejected over node TLS: %v", err)
	}
	// THE AUDIT BLOCKER ASSERTION (preserved): must NOT hold.
	if !restartedController.Authenticate(node.ID, old) &&
		restartedController.Authenticate(node.ID, staged) &&
		restartedNode.Accepts(old) && !restartedNode.Accepts(staged) {
		t.Fatal("BLOCKER: real CLI returned success; durable controller rotated, restarted node still has only old credential")
	}
	// Remediated contract: old rejected everywhere.
	if restartedController.Authenticate(node.ID, old) {
		t.Fatal("controller accepted the old credential")
	}
	if restartedNode.Accepts(old) {
		t.Fatal("B4-01 DEFECT: node still accepts the old credential after CLI success")
	}
}

// b4Stage stages a rotation through the real HTTP API and returns
// the staged credential.
func b4Stage(t *testing.T, baseURL, nodeID, opToken string) string {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost,
		baseURL+"/api/v1/xray/center/nodes/"+nodeID+"/rotate", bytes.NewReader([]byte("{}")))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+opToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("stage: %d %s", resp.StatusCode, body)
	}
	var out struct {
		Credential string `json:"credential"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatal(err)
	}
	return out.Credential
}
