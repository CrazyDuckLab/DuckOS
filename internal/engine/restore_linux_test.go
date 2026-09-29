//go:build linux

package engine

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/agent"
	"github.com/CrazyDuckLab/DuckOS/internal/restore"
	"github.com/CrazyDuckLab/DuckOS/internal/state"
)

func TestH09EngineStartupResolvesDurableAdmissionWithoutNewApply(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"source/b1", "live", "service", "engine"} {
		if err := os.MkdirAll(filepath.Join(dir, p), 0700); err != nil {
			t.Fatal(err)
		}
	}
	os.WriteFile(filepath.Join(dir, "source/b1/a"), []byte("new"), 0600)
	os.WriteFile(filepath.Join(dir, "live/a"), []byte("old"), 0600)
	service, err := restore.Open(filepath.Join(dir, "source"), filepath.Join(dir, "live"), filepath.Join(dir, "service"))
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	listener, err := agent.Listen(filepath.Join(dir, "restore.sock"), uint32(os.Geteuid()))
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	server := &http.Server{Handler: service}
	defer server.Close()
	go server.Serve(listener)
	handler := agent.NewHandler(nil)
	handler.EnableBackupRestore(agent.NewRestoreProxy(listener.Addr().String()))
	agentListener, err := agent.Listen(filepath.Join(dir, "agent.sock"), uint32(os.Geteuid()))
	if err != nil {
		t.Fatal(err)
	}
	defer agentListener.Close()
	agentServer := &http.Server{Handler: handler}
	defer agentServer.Close()
	go agentServer.Serve(agentListener)
	validate := func(json.RawMessage) error { return nil }
	store, err := state.Open(filepath.Join(dir, "engine"), []byte(`{"version":1}`), validate)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, _ := store.Snapshot()
	request := restore.Request{ID: strings.Repeat("1", 32), Expected: snapshot.Current.ID, Backup: "b1", Keys: []string{"a"}}
	raw, _ := json.Marshal(request)
	if err := store.BeginRestore(request.ID, request.Expected, request.Digest(), raw); err != nil {
		t.Fatal(err)
	}
	store.Close()
	store, err = state.OpenExisting(filepath.Join(dir, "engine"), validate)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	engine := New(agentListener.Addr().String(), "test")
	config, err := engine.EnableConfig(store)
	if err != nil {
		t.Fatal(err)
	}
	defer config.Close()
	deadline := time.Now().Add(5 * time.Second)
	for {
		snapshot, err = store.Snapshot()
		if err != nil {
			t.Fatal(err)
		}
		if snapshot.Restore == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("startup reservation not reconciled")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "live/a")); string(b) != "old" {
		t.Fatal("startup unexpectedly started unapplied restore")
	}
	result, err := service.Execute(context.Background(), request)
	if err != nil || result.Outcome != "rolled-back" {
		t.Fatal("late request was not durably tombstoned", result, err)
	}
}
