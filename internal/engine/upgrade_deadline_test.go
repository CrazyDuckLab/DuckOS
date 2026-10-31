package engine

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestUpgradeStageDeadlineDoesNotChangeOtherRequests(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(80 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":1,"result":"staged"}`))
	}))
	defer server.Close()
	e := New("/synthetic-unused.sock", "test")
	tr := e.client.Transport.(*http.Transport)
	tr.DialContext = func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "tcp", server.Listener.Addr().String())
	}
	tr.ResponseHeaderTimeout = 10 * time.Millisecond
	e.client.Timeout = 20 * time.Millisecond
	if _, err := e.UpgradeStage(context.Background(), []byte(`{}`)); err != nil {
		t.Fatalf("full-package stage inherited short read-operation timeout: %v", err)
	}
	if e.client.Timeout != 20*time.Millisecond || tr.ResponseHeaderTimeout != 10*time.Millisecond {
		t.Fatal("shared read-only transport was changed")
	}
	if _, err := e.UpgradeActivate(context.Background(), []byte(`{}`)); err == nil {
		t.Fatal("non-stage request lost its existing timeout")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	if _, err := e.UpgradeStage(ctx, []byte(`{}`)); err == nil {
		t.Fatal("stage ignored caller cancellation")
	}
}
