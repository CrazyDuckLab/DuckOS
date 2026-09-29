package main

import (
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/CrazyDuckLab/DuckOS/internal/agent"
	"github.com/CrazyDuckLab/DuckOS/internal/object"
)

func TestAgentLinuxSourceFetchesOverUnixSocket(t *testing.T) {
	dir := t.TempDir()
	sock := filepath.Join(dir, "agent.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	mux := http.NewServeMux()
	mux.HandleFunc(agent.LinuxObjectsPathPrefix+"network.firewall.rule", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("method %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		total := 1
		_ = json.NewEncoder(w).Encode(object.Collection{
			SnapshotID: "snap-1",
			Complete:   true,
			Total:      &total,
			Items: []object.ViewObject{{
				Ref:          object.ObjectRef{Kind: "network.firewall.rule", ID: "rule-1"},
				DisplayName:  "rule-1",
				Availability: object.Available,
			}},
		})
	})
	srv := &http.Server{Handler: mux}
	go func() { _ = srv.Serve(ln) }()
	defer srv.Close()
	defer os.Remove(sock)

	src := newAgentLinuxSource(sock)
	rows, err := src.FirewallRules(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Ref.ID != "rule-1" {
		t.Fatalf("unexpected rows: %#v", rows)
	}
}
