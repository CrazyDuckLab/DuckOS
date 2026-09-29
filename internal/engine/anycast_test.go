// anycast_test.go pins the Engine.Anycast IPC wiring
// (C-blocker #9). The Engine forwards /agent/v1/anycast
// to the privileged Agent; a 200 with a canonical
// envelope becomes an AnycastStatus. A non-200 OR a
// schema-mismatched response becomes a structured
// 'Agent unavailable' error so the UI can render the
// gap.
package engine

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/agent"
	"github.com/CrazyDuckLab/DuckOS/internal/anycast"
)

// TempDir is a tiny alias to keep the test file
// self-contained even on platforms where the
// filepath.TempDir signature differs.
func TempDir(prefix string) (string, error) { return os.MkdirTemp("", prefix) }

// anycastFakeServer is a tiny stub Agent that serves
// /agent/v1/anycast. The test asserts the Engine decodes
// the response and never invents availability.
func anycastFakeServer(t *testing.T, status int, payload []byte, hdr http.Header) string {
	t.Helper()
	dir, err := TempDir("duck-ipc-")
	if err != nil {
		t.Fatal(err)
	}
	socket := filepath.Join(dir, "a.sock")
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	mux := http.NewServeMux()
	mux.HandleFunc(agent.AnycastPath, func(w http.ResponseWriter, _ *http.Request) {
		for k, v := range hdr {
			w.Header()[k] = v
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write(payload)
	})
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	go func() { _ = http.Serve(listener, mux) }()
	return socket
}

// TestEngineAnycastDecodesAgentResponse proves the
// happy path: a 200 with a canonical AnycastStatus
// envelope is decoded into the typed Engine response.
func TestEngineAnycastDecodesAgentResponse(t *testing.T) {
	want := agent.AnycastStatus{
		Version:   1,
		UpdatedAt: time.Unix(1, 0).UTC(),
		Evidence: []anycast.Evidence{
			{
				VIP: anycast.VIPIdentifier{Name: "primary", IP: "192.0.2.1"},
				Desired: &anycast.DesiredRecord{
					VIP:       anycast.VIPIdentifier{Name: "primary", IP: "192.0.2.1"},
					State:     anycast.HysteresisAnnounce,
					UpdatedAt: time.Unix(1, 0).UTC(),
				},
			},
		},
	}
	data, _ := json.Marshal(want)
	socket := anycastFakeServer(t, http.StatusOK, data, nil)
	eng := New(socket, "test")
	got, err := eng.Anycast(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != 1 {
		t.Fatalf("version: %d, want 1", got.Version)
	}
	if len(got.Evidence) != 1 {
		t.Fatalf("evidence: %d, want 1", len(got.Evidence))
	}
}

// TestEngineAnycastRefusesAgentError proves the Engine
// propagates Agent unavailability as a structured error;
// it never invents availability on a non-2xx response.
func TestEngineAnycastRefusesAgentError(t *testing.T) {
	socket := anycastFakeServer(t, http.StatusServiceUnavailable, []byte("{}"), nil)
	eng := New(socket, "test")
	if _, err := eng.Anycast(context.Background()); err == nil {
		t.Fatal("non-2xx Agent response must fail closed")
	}
}

// TestEngineAnycastRefusesSchemaMismatch proves the
// Engine enforces the version contract; a schema
// mismatch becomes a structured error so the UI can
// render the gap.
func TestEngineAnycastRefusesSchemaMismatch(t *testing.T) {
	data, _ := json.Marshal(agent.AnycastStatus{Version: 99})
	socket := anycastFakeServer(t, http.StatusOK, data, nil)
	eng := New(socket, "test")
	if _, err := eng.Anycast(context.Background()); err == nil {
		t.Fatal("schema-mismatched Agent response must fail closed")
	}
}

// silence unused-import warning when the helper is
// unused after a future edit.
var _ = http.MethodGet
