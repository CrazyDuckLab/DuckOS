package agent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type slowStageObserver struct{ fakeUpgradeObs }

func (*slowStageObserver) Stage(_ context.Context, w http.ResponseWriter, _ *http.Request) {
	time.Sleep(80 * time.Millisecond)
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"version":1,"result":"staged"}`))
}
func (*slowStageObserver) Activate(context.Context, http.ResponseWriter, *http.Request) {}
func (*slowStageObserver) Rollback(context.Context, http.ResponseWriter, *http.Request) {}
func TestTrustedStageExtendsAgentResponseDeadline(t *testing.T) {
	h := NewHandler(nil)
	h.EnableUpgrade(&slowStageObserver{})
	server := httptest.NewUnstartedServer(http.HandlerFunc(h.serveUpgradeStage))
	server.Config.WriteTimeout = 15 * time.Millisecond
	server.Start()
	defer server.Close()
	res, err := server.Client().Post(server.URL, "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatalf("complete signed stage lost Agent response to short lifecycle deadline: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("HTTP %d", res.StatusCode)
	}
}
