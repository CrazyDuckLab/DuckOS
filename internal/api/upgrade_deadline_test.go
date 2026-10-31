package api

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type slowUpgradeEngine struct{ testEngine }

func (*slowUpgradeEngine) UpgradeStage(context.Context, []byte) ([]byte, error) {
	time.Sleep(80 * time.Millisecond)
	return []byte(`{"version":1,"result":"staged"}`), nil
}
func TestAuthorizedStageExtendsOnlyItsBoundedServerDeadline(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<!doctype html>"), 0600); err != nil {
		t.Fatal(err)
	}
	h, err := New(&slowUpgradeEngine{}, testToken, "127.0.0.1:8080", dir)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	h.EnableAgentAssertionKey(testAssertionKey)
	server := httptest.NewUnstartedServer(h)
	server.Config.WriteTimeout = 15 * time.Millisecond
	server.Start()
	defer server.Close()
	req, _ := http.NewRequest("POST", server.URL+"/api/v1/upgrade/stage", strings.NewReader(`{}`))
	req.Host = "127.0.0.1:8080"
	req.Header.Set("Authorization", "Bearer "+testToken)
	req.Header.Set("Content-Type", "application/json")
	response, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("authorized full-package stage lost response to short global deadline: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		b, _ := io.ReadAll(response.Body)
		t.Fatalf("HTTP %d: %s", response.StatusCode, b)
	}
	// Authentication still runs before the stage-specific deadline/body handling.
	bad, _ := http.NewRequest("POST", server.URL+"/api/v1/upgrade/stage", strings.NewReader(`{}`))
	bad.Host = "127.0.0.1:8080"
	bad.Header.Set("Content-Type", "application/json")
	res, err := server.Client().Do(bad)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != 401 {
		t.Fatalf("unauthenticated stage HTTP %d", res.StatusCode)
	}
}
