// Package center / staged_rotation_test.go: Milestone 1 regression —
// the staged rotation flow never strands a node, and node-auth
// endpoints accept only the right credential for the right node.
package center

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/CrazyDuckLab/DuckOS/internal/xnode"
)

func stageFixture(t *testing.T) (*Handler, string, string) {
	t.Helper()
	h, _ := fixture(t)
	w := do(t, h, http.MethodPost, "/center/nodes/enroll-token", nil)
	var issued struct {
		Token string `json:"token"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &issued)
	node, credential, err := h.Nodes.Enroll(issued.Token, xnode.NodeInfo{Name: "edge-s"})
	if err != nil {
		t.Fatal(err)
	}
	return h, node.ID, credential
}

// The full staged lifecycle over HTTP: stage → node pull (old cred)
// → node confirm (new cred) → old cred rejected, new accepted.
func TestStagedRotation_FullLifecycle(t *testing.T) {
	h, nodeID, oldCred := stageFixture(t)

	// Stage via UI endpoint.
	w := do(t, h, http.MethodPost, "/center/nodes/"+nodeID+"/rotate", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("stage: %d %s", w.Code, w.Body.String())
	}
	var staged struct {
		Credential string `json:"credential"`
		State      string `json:"state"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &staged)
	if staged.Credential == "" || staged.State != "PENDING" {
		t.Fatalf("stage payload: %+v", staged)
	}
	// State is PENDING and old credential still authenticates.
	if got := h.Nodes.RotationStatus(nodeID); got != xnode.RotationPending {
		t.Fatalf("state %s", got)
	}
	if !h.Nodes.Authenticate(nodeID, oldCred) {
		t.Fatal("old credential invalidated at stage time (strand bug)")
	}

	// Node pulls with its CURRENT credential via node-auth.
	req := nodeAuthRequest(nodeID, "pull", oldCred)
	w = doRaw(t, h, req)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), staged.Credential) {
		t.Fatalf("pull: %d %s", w.Code, w.Body.String())
	}
	if got := h.Nodes.RotationStatus(nodeID); got != xnode.RotationDelivered {
		t.Fatalf("post-pull state %s", got)
	}

	// Node confirms with the NEW credential; only now does the
	// controller commit.
	req = nodeAuthRequest(nodeID, "confirm", staged.Credential)
	w = doRaw(t, h, req)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "CONFIRMED") {
		t.Fatalf("confirm: %d %s", w.Code, w.Body.String())
	}
	if h.Nodes.Authenticate(nodeID, oldCred) {
		t.Fatal("old credential accepted after confirmed rotation")
	}
	if !h.Nodes.Authenticate(nodeID, staged.Credential) {
		t.Fatal("new credential rejected after confirmed rotation")
	}
}

// Security: the node credential can only drive its OWN staged
// rotation; wrong-node, wrong-credential and pre-stage attempts are
// refused; cancel aborts safely.
func TestStagedRotation_SecurityBoundaries(t *testing.T) {
	h, nodeA, credA := stageFixture(t)
	_, nodeB, credB := stageFixture(t)

	// Stage only A.
	if _, _, err := h.Nodes.StageRotation(nodeA); err != nil {
		t.Fatal(err)
	}
	// B's credential cannot pull A's staged credential.
	if _, _, err := h.Nodes.PullStagedRotation(nodeA, credB); err == nil {
		t.Fatal("wrong-node credential pulled A's staged rotation")
	}
	// Confirm without staging on B is refused.
	if _, err := h.Nodes.ConfirmStagedRotation(nodeB, credB); err == nil {
		t.Fatal("confirm without staging accepted")
	}
	// Pull on A works, then cancel: both sides keep old credential.
	if _, _, err := h.Nodes.PullStagedRotation(nodeA, credA); err != nil {
		t.Fatal(err)
	}
	if err := h.Nodes.CancelStagedRotation(nodeA); err != nil {
		t.Fatal(err)
	}
	if h.Nodes.RotationStatus(nodeA) != xnode.RotationNone {
		t.Fatal("cancel did not clear the staged rotation")
	}
	// Confirm after cancel is refused and nothing changed.
	if _, err := h.Nodes.ConfirmStagedRotation(nodeA, credA); err == nil {
		t.Fatal("confirm after cancel accepted")
	}
	if !h.Nodes.Authenticate(nodeA, credA) {
		t.Fatal("cancelled rotation invalidated the old credential")
	}
}

func nodeAuthRequest(nodeID, op, credential string) *http.Request {
	req := nodeAuthReqBuild("/center/node-auth/"+nodeID+"/"+op, credential)
	return req
}

func nodeAuthReqBuild(path, credential string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, path, nil)
	req.Header.Set("Authorization", "Bearer ops-token")
	req.Header.Set("X-Node-Credential", credential)
	req.RemoteAddr = "127.0.0.1:12345"
	return req
}

func doRaw(t *testing.T, h *Handler, req *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}
