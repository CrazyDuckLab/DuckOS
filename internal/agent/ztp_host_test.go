package agent

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/ztp"
)

func TestZTPHostIssueRedeemReplayThroughIPC(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	m, err := ztp.OpenManager(dir, []byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	host := NewZTPHost(m)
	host.SetTimeout(2 * time.Second)

	issueReq := httptest.NewRequest(http.MethodPost, ZTPIssuePath, strings.NewReader(`{"subject":"node-1","node":"node-1","validitySeconds":60}`))
	issueRec := httptest.NewRecorder()
	host.ServeZTP(issueRec, issueReq)
	if issueRec.Code != 200 {
		t.Fatalf("issue status %d body %s", issueRec.Code, issueRec.Body.String())
	}
	var issued ZTPIssueResponse
	if err := json.Unmarshal(issueRec.Body.Bytes(), &issued); err != nil {
		t.Fatal(err)
	}
	if issued.Token.MAC == "" || issued.Token.Subject != "node-1" {
		t.Fatalf("bad issued token: %#v", issued.Token)
	}

	redeemBody, _ := json.Marshal(ZTPRedeemRequest{
		Subject:      issued.Token.Subject,
		Scope:        issued.Token.Scope,
		Node:         issued.Token.Node,
		IssuedAt:     issued.Token.IssuedAt,
		ExpiresAt:    issued.Token.ExpiresAt,
		MAC:          issued.Token.MAC,
		ObservedNode: "node-1",
	})
	redeemReq := httptest.NewRequest(http.MethodPost, ZTPPath, strings.NewReader(string(redeemBody)))
	redeemRec := httptest.NewRecorder()
	host.ServeZTP(redeemRec, redeemReq)
	if redeemRec.Code != 200 {
		t.Fatalf("redeem status %d body %s", redeemRec.Code, redeemRec.Body.String())
	}
	replayReq := httptest.NewRequest(http.MethodPost, ZTPPath, strings.NewReader(string(redeemBody)))
	replayRec := httptest.NewRecorder()
	host.ServeZTP(replayRec, replayReq)
	if replayRec.Code != http.StatusConflict {
		t.Fatalf("replay status %d body %s", replayRec.Code, replayRec.Body.String())
	}
}
