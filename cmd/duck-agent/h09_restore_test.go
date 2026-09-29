//go:build linux

package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CrazyDuckLab/DuckOS/internal/agent"
	"github.com/CrazyDuckLab/DuckOS/internal/engine"
	"github.com/CrazyDuckLab/DuckOS/internal/restore"
	"github.com/CrazyDuckLab/DuckOS/internal/state"
)

func TestH09PublicEngineAgentRestoreService(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"source/b1", "live", "journal", "engine"} {
		if err := os.MkdirAll(filepath.Join(dir, p), 0700); err != nil {
			t.Fatal(err)
		}
	}
	for _, k := range []string{"selected", "untouched"} {
		if err := os.WriteFile(filepath.Join(dir, "source/b1", k), []byte("new"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "live", k), []byte("old"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	service, err := restore.Open(filepath.Join(dir, "source"), filepath.Join(dir, "live"), filepath.Join(dir, "journal"))
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	socket := filepath.Join(dir, "restore.sock")
	listener, err := agent.Listen(socket, uint32(os.Geteuid()))
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	server := &http.Server{Handler: service}
	defer server.Close()
	go server.Serve(listener)
	journal, err := state.Open(filepath.Join(dir, "engine"), []byte(`{"version":1}`), func(json.RawMessage) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	defer journal.Close()
	snapshot, err := journal.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	handler := agent.NewHandler(nil)
	handler.EnableBackupRestore(agent.NewRestoreProxy(socket))
	api, token := remediationPublicAPI(t, handler, strings.Repeat("s", 64), func(e *engine.Engine) {
		if _, err := e.EnableConfig(journal); err != nil {
			t.Fatal(err)
		}
	})
	call := func(body any, auth string) int {
		t.Helper()
		raw, _ := json.Marshal(body)
		req, _ := http.NewRequest("POST", api.URL+"/api/v1/backup/restore", bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		if auth != "" {
			req.Header.Set("Authorization", "Bearer "+auth)
		}
		response, err := api.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		b, _ := io.ReadAll(response.Body)
		t.Logf("restore response %d %s", response.StatusCode, b)
		return response.StatusCode
	}
	request := restore.Request{ID: strings.Repeat("1", 32), Expected: strings.Repeat("f", 64), Backup: "b1", Keys: []string{"selected"}}
	if code := call(request, token); code != 409 {
		t.Fatalf("invented revision: %d", code)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "live/selected")); string(b) != "old" {
		t.Fatal("invented revision mutated filesystem")
	}
	request.Expected = snapshot.Current.ID
	if code := call(request, ""); code == 200 {
		t.Fatal("unauthenticated restore")
	}
	if code := call(map[string]any{"requestId": request.ID, "expectedRevision": request.Expected, "backupId": "b1", "selectedKeys": request.Keys, "liveRoot": dir}, token); code == 200 {
		t.Fatal("caller root accepted")
	}
	if code := call(request, token); code != 200 {
		t.Fatalf("valid public restore: %d", code)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "live/selected")); string(b) != "new" {
		t.Fatal("independent selected-file oracle failed")
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "live/untouched")); string(b) != "old" {
		t.Fatal("unselected file changed")
	}
	snapshot, err = journal.Snapshot()
	if err != nil || snapshot.Restore != nil {
		t.Fatal("Engine reservation not released")
	}
}
