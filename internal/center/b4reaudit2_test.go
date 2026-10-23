// Package center / b4reaudit2_test.go: regressions for the second
// independent re-audit (2026-10-23) — production prefix node-auth
// routing, strict method/path RBAC, suspended identities, single
// limiter admission and principal isolation.
package center

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/CrazyDuckLab/DuckOS/internal/identity"
)

// b4rStrictAccess rejects every token except the synthetic operator
// bearer (unlike the permissive production stub this keeps empty
// tokens out, which is what exposed the original B4-03 defect).
type b4rStrictAccess struct{}

func (b4rStrictAccess) Authorize(token string, _ identity.Permission) (identity.User, error) {
	if token != "ops-token" {
		return identity.User{}, errors.New("invalid operator bearer")
	}
	return identity.User{Username: "op", Role: identity.NetworkAdmin}, nil
}

// The production mount (/xray/v1/center/node-auth/...) must reach
// the node-auth dispatch WITHOUT an operator bearer; the bearer
// presence must not change the outcome either.
func TestB403_ProductionPrefixNodeOnlyPull(t *testing.T) {
	h, id, old := stageFixture(t)
	h.Access = b4rStrictAccess{}
	if _, _, err := h.Nodes.StageRotation(id); err != nil {
		t.Fatal(err)
	}
	for _, bearer := range []string{"", "ops-token"} {
		r := httptest.NewRequest(http.MethodPost, "/xray/v1/center/node-auth/"+id+"/pull", nil)
		r.Header.Set("X-Node-Credential", old)
		if bearer != "" {
			r.Header.Set("Authorization", "Bearer "+bearer)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != http.StatusOK {
			t.Errorf("production prefix bearerPresent=%v status=%d body=%s", bearer != "", w.Code, w.Body.String())
		}
	}
}

// Strict negative matrix: node credentials (and issued subscription
// tokens) must never reach operator operations through any method,
// and node-auth itself is POST-only with a canonical two-segment
// path.
func TestB403_NodeAuthMethodsAndRBACMatrix(t *testing.T) {
	h, id, old := stageFixture(t)
	h.Access = b4rStrictAccess{}
	next, _, err := h.Nodes.StageRotation(id)
	if err != nil {
		t.Fatal(err)
	}
	_ = next
	for _, method := range []string{"GET", "HEAD", "PUT", "PATCH", "DELETE", "OPTIONS", "TRACE", "POST"} {
		for _, suffix := range []string{
			"nodes", "groups", "policies", "stats", "storage", "tokens",
			"preview/x", "backup", "events", "diagnostics", "delivery-validate",
			"nodes/" + id + "/rotate",
			"node-authx/" + id + "/pull",
			"node-auth/" + id + "/pull/extra",
			"node-auth/../nodes",
			"node-auth//pull",
		} {
			for _, cred := range []string{old, "synthetic-subscription-token"} {
				r := httptest.NewRequest(method, "/center/"+suffix, nil)
				r.Header.Set("X-Node-Credential", cred)
				r.Header.Set("Authorization", "Bearer "+cred)
				w := httptest.NewRecorder()
				h.ServeHTTP(w, r)
				if w.Code >= 200 && w.Code < 300 {
					t.Errorf("unexpected access %s %s", method, suffix)
				}
			}
		}
	}
	for _, op := range []string{"pull", "confirm"} {
		for _, method := range []string{"GET", "HEAD", "PUT", "PATCH", "DELETE", "OPTIONS", "TRACE"} {
			r := httptest.NewRequest(method, fmt.Sprintf("/center/node-auth/%s/%s", id, op), nil)
			r.Header.Set("X-Node-Credential", old)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != http.StatusMethodNotAllowed {
				t.Errorf("method %s %s: %d", method, op, w.Code)
			}
		}
		for _, cred := range []string{"wrong", "synthetic-subscription-token"} {
			r := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/center/node-auth/%s/%s", id, op), nil)
			r.Header.Set("X-Node-Credential", cred)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code >= 200 && w.Code < 300 {
				t.Errorf("wrong principal %s succeeded", op)
			}
		}
	}
}

// A suspended node gets no successful pull or confirm over HTTP.
func TestB403_SuspendedNodeRejectedOverHTTP(t *testing.T) {
	h, id, old := stageFixture(t)
	h.Access = b4rStrictAccess{}
	next, _, err := h.Nodes.StageRotation(id)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.Nodes.Suspend(id); err != nil {
		t.Fatal(err)
	}
	for op, cred := range map[string]string{"pull": old, "confirm": next} {
		r := httptest.NewRequest(http.MethodPost, "/center/node-auth/"+id+"/"+op, nil)
		r.Header.Set("X-Node-Credential", cred)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code >= 200 && w.Code < 300 {
			t.Errorf("suspended %s succeeded: %d", op, w.Code)
		}
	}
}

// Exactly ONE limiter admission per accepted request (the pre-gate
// dispatch must not charge again inside the handler).
func TestB403_SingleLimiterAdmission(t *testing.T) {
	h, id, old := stageFixture(t)
	if _, _, err := h.Nodes.StageRotation(id); err != nil {
		t.Fatal(err)
	}
	calls := 0
	h.NodeAuthRate = func(string) bool { calls++; return true }
	r := httptest.NewRequest(http.MethodPost, "/center/node-auth/"+id+"/pull", nil)
	r.Header.Set("X-Node-Credential", old)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if calls != 1 {
		t.Fatalf("one request consumed limiter %d times", calls)
	}
}

// An actually issued subscription token must not authorize any
// center operation, including the node-auth routes.
func TestB403_IssuedSubscriptionTokenCannotAuthorize(t *testing.T) {
	h, id, _ := stageFixture(t)
	h.Access = b4rStrictAccess{}
	if _, _, err := h.Nodes.StageRotation(id); err != nil {
		t.Fatal(err)
	}
	token, _, err := h.Tokens.Issue("g1", 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		"nodes", "groups", "storage", "tokens",
		"node-auth/" + id + "/pull", "node-auth/" + id + "/confirm",
	} {
		for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete} {
			r := httptest.NewRequest(method, "/center/"+path, nil)
			r.Header.Set("X-Node-Credential", token)
			r.Header.Set("Authorization", "Bearer "+token)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code >= 200 && w.Code < 300 {
				t.Errorf("issued subscription token accessed %s %s", method, path)
			}
		}
	}
}
