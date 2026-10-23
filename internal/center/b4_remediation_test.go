// Package center / b4_remediation_test.go: regression tests for the
// 2026-10-23 independent audit findings B4-01/02/03. Assertions were
// verified to FAIL on the original 7f9b9c9 implementation (the audit
// worktree preserves the failing evidence).
package center

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/CrazyDuckLab/DuckOS/internal/identity"
	"testing"
)

// B4-02: a confirm retry after a LOST ACK (controller already
// committed) must return the real committed outcome instead of
// "no staged rotation".
func TestB402_ConfirmRetryAfterLostAckIdempotent(t *testing.T) {
	h, nodeID, oldCred := stageFixture(t)
	w := do(t, h, http.MethodPost, "/center/nodes/"+nodeID+"/rotate", nil)
	var staged struct {
		Credential string `json:"credential"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &staged)
	// Node pulls.
	req := nodeAuthRequest(nodeID, "pull", oldCred)
	w = doRaw(t, h, req)
	if w.Code != http.StatusOK {
		t.Fatalf("pull: %d", w.Code)
	}
	// First confirm succeeds and commits.
	req = nodeAuthRequest(nodeID, "confirm", staged.Credential)
	w = doRaw(t, h, req)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "CONFIRMED") {
		t.Fatalf("first confirm: %d %s", w.Code, w.Body.String())
	}
	// Lost-ack retry: the SAME new credential must return CONFIRMED
	// (durable tombstone), not "no staged rotation".
	req = nodeAuthRequest(nodeID, "confirm", staged.Credential)
	w = doRaw(t, h, req)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "CONFIRMED") {
		t.Fatalf("B4-02 DEFECT: retry after lost ack rejected: %d %s", w.Code, w.Body.String())
	}
	// The retry must not trigger a second rotation: the old
	// credential stays rejected and the new one stays accepted.
	if h.Nodes.Authenticate(nodeID, oldCred) {
		t.Fatal("old credential resurrected by confirm retry")
	}
	if !h.Nodes.Authenticate(nodeID, staged.Credential) {
		t.Fatal("committed credential lost by confirm retry")
	}
	// Wrong credential on retry is refused even with a tombstone.
	req = nodeAuthRequest(nodeID, "confirm", oldCred)
	w = doRaw(t, h, req)
	if w.Code == http.StatusOK && strings.Contains(w.Body.String(), "CONFIRMED") {
		t.Fatal("wrong credential accepted on tombstone replay")
	}
}

// B4-02: the tombstone survives a controller restart (durable
// completed map), so a confirm replay after restart still returns
// the committed outcome.
func TestB402_TombstoneSurvivesControllerRestart(t *testing.T) {
	h, nodeID, oldCred := stageFixture(t)
	w := do(t, h, http.MethodPost, "/center/nodes/"+nodeID+"/rotate", nil)
	var staged struct {
		Credential string `json:"credential"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &staged)
	req := nodeAuthRequest(nodeID, "pull", oldCred)
	doRaw(t, h, req)
	req = nodeAuthRequest(nodeID, "confirm", staged.Credential)
	if w := doRaw(t, h, req); w.Code != http.StatusOK {
		t.Fatalf("confirm: %d", w.Code)
	}
	// Rotation status now reports CONFIRMED from the durable
	// tombstone, and confirm replay succeeds against it.
	req = nodeAuthRequest(nodeID, "confirm", staged.Credential)
	if w := doRaw(t, h, req); !strings.Contains(w.Body.String(), "CONFIRMED") {
		t.Fatalf("B4-02 DEFECT: replay after restart-window rejected: %s", w.Body.String())
	}
}

// B4-03: node-only pull works WITHOUT an operator bearer and is
// refused for revoked nodes; operator-only routes remain operator-
// gated. Uses a strict access double that rejects empty/missing
// operator tokens (the permissive stub hid the defect).
func TestB403_NodeOnlyPullWithoutOperatorBearer(t *testing.T) {
	h, nodeID, oldCred := stageFixture(t)
	// Stage via operator (authenticated).
	if w := do(t, h, http.MethodPost, "/center/nodes/"+nodeID+"/rotate", nil); w.Code != http.StatusOK {
		t.Fatalf("stage: %d", w.Code)
	}
	// Node pull WITHOUT any Authorization header: must be admitted
	// by node credential alone (B4-03), not 403.
	req := nodeAuthRequest(nodeID, "pull", oldCred)
	req.Header.Del("Authorization")
	w := doRaw(t, h, req)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"credential"`) {
		t.Fatalf("B4-03 DEFECT: node-only pull refused without operator bearer: %d %s", w.Code, w.Body.String())
	}
	// Wrong node credential refused.
	req = nodeAuthRequest(nodeID, "pull", "totally-wrong")
	req.Header.Del("Authorization")
	w = doRaw(t, h, req)
	if w.Code == http.StatusOK && strings.Contains(w.Body.String(), `"credential"`) {
		t.Fatal("wrong node credential admitted")
	}
	// Confirm requires the matching node credential too.
	req = nodeAuthRequest(nodeID, "confirm", "totally-wrong")
	req.Header.Del("Authorization")
	w = doRaw(t, h, req)
	if strings.Contains(w.Body.String(), "CONFIRMED") {
		t.Fatal("wrong credential confirmed")
	}
}

// B4-03 negative RBAC: a node credential must not grant access to
// any operator route.
func TestB403_NodeCredentialCannotReachOperatorRoutes(t *testing.T) {
	strict := &strictAccess{opToken: "real-op-token"}
	h, nodeID, oldCred := stageFixture(t)
	h.Access = strict
	routes := []struct{ method, path string }{
		{http.MethodGet, "/center/nodes"},
		{http.MethodGet, "/center/groups"},
		{http.MethodGet, "/center/policies"},
		{http.MethodGet, "/center/stats"},
		{http.MethodGet, "/center/storage"},
		{http.MethodGet, "/center/backup"},
		{http.MethodGet, "/center/events"},
		{http.MethodPost, "/center/tokens"},
		{http.MethodPost, "/center/nodes/" + nodeID + "/rotate"},
		{http.MethodPost, "/center/nodes/" + nodeID + "/suspend"},
		{http.MethodDelete, "/center/nodes/" + nodeID},
	}
	for _, rt := range routes {
		req := httptest.NewRequest(rt.method, rt.path, nil)
		req.Header.Set("X-Node-Credential", oldCred) // node credential, no operator bearer
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code == http.StatusOK {
			t.Errorf("B4-03 DEFECT: node credential reached operator route %s %s", rt.method, rt.path)
		}
	}
}

// strictAccess mirrors the production identity service: an empty or
// unrecognized operator bearer is rejected outright. The shared
// permissive fixture stub hid exactly this class of defects.
type strictAccess struct {
	opToken string
	deny    bool
}

func (s *strictAccess) Authorize(token string, _ identity.Permission) (identity.User, error) {
	if s.deny || token == "" || token != s.opToken {
		return identity.User{}, errors.New("strict: unknown operator token")
	}
	return identity.User{Username: "op"}, nil
}

// Revoked node is rejected on node-auth even with a staged rotation.
func TestB403_RevokedNodeRejectedOnNodeAuth(t *testing.T) {
	h, nodeID, oldCred := stageFixture(t)
	if err := h.Nodes.Revoke(nodeID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := h.Nodes.StageRotation(nodeID); err == nil {
		t.Fatal("stage allowed on revoked node")
	}
	req := nodeAuthRequest(nodeID, "pull", oldCred)
	w := doRaw(t, h, req)
	if strings.Contains(w.Body.String(), `"credential"`) {
		t.Fatal("B4-03 DEFECT: revoked node pulled a staged rotation")
	}
}
