//go:build linux

// dv01_l_test.go covers the DV01 L-tier subcases against
// the real Linux fixture:
//
//   - restart_preserves_identity: Engine + Agent cross-
//     process restart preserves canonical object identity
//     and does not resurrect stale observed cache.
//   - disabled_daemon: missing source daemon produces
//     typed degraded / unavailable, never fake empty
//     Complete=true.

package layerl

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/doctor"
	"github.com/CrazyDuckLab/DuckOS/internal/engine"
	"github.com/CrazyDuckLab/DuckOS/internal/object"
	"github.com/CrazyDuckLab/DuckOS/internal/objectlisters"
)

// fakeAgentSocket runs an HTTP handler on a Unix socket
// the production Engine can read from. It serves
// /agent/v1/doctor with a deterministic snapshot.
type fakeAgentSocket struct {
	socketPath string
	listener   net.Listener
	calls      atomic.Int64
	active     int32
}

func newFakeAgentSocket(t *testing.T) *fakeAgentSocket {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.sock")
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	fa := &fakeAgentSocket{socketPath: path, listener: listener}
	fa.active = 1
	mux := http.NewServeMux()
	mux.HandleFunc("/agent/v1/doctor", func(w http.ResponseWriter, r *http.Request) {
		fa.calls.Add(1)
		if atomic.LoadInt32(&fa.active) == 0 {
			http.Error(w, "agent unavailable", http.StatusServiceUnavailable)
			return
		}
		snap := doctor.Snapshot{
			SchemaVersion: doctor.SchemaVersion,
			GeneratedAt:   time.Now().UTC(),
			Environment:   "test",
			Network: doctor.Network{
				Interfaces: []doctor.Interface{
					{
						Name:      "veth-test",
						Index:     42,
						LinkUp:    true,
						MTU:       1500,
						Addresses: []string{"10.99.99.1/24"},
						OperState: "up",
					},
				},
				Routes: []doctor.Route{
					{Destination: "default", Gateway: "10.99.99.254", Device: "veth-test"},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(snap)
	})
	go http.Serve(listener, mux)
	t.Cleanup(func() {
		_ = listener.Close()
		_ = os.Remove(path)
	})
	return fa
}

// TestDV01RestartPreservesIdentity exercises the
// Engine.ObjectList path across two consecutive Engine
// instances pointed at the same fake agent socket.
func TestDV01RestartPreservesIdentity(t *testing.T) {
	fa := newFakeAgentSocket(t)
	first := engine.New(fa.socketPath, "test")
	objectlisters.RegisterAllWithRealSourcesAndLinuxSource(first, nil, nil, nil, nil)
	col1, err := first.ObjectList(context.Background(), "network.interface", object.CollectionParams{})
	if err != nil {
		t.Fatal(err)
	}
	if !col1.Complete || len(col1.Items) == 0 {
		t.Fatalf("first run: Complete=%v, Items=%d", col1.Complete, len(col1.Items))
	}
	second := engine.New(fa.socketPath, "test")
	objectlisters.RegisterAllWithRealSourcesAndLinuxSource(second, nil, nil, nil, nil)
	col2, err := second.ObjectList(context.Background(), "network.interface", object.CollectionParams{})
	if err != nil {
		t.Fatal(err)
	}
	if !col2.Complete || len(col2.Items) == 0 {
		t.Fatalf("second run: Complete=%v, Items=%d", col2.Complete, len(col2.Items))
	}
	if col1.Items[0].Ref.ID != col2.Items[0].Ref.ID {
		t.Fatalf("identity drift across restart: %q vs %q",
			col1.Items[0].Ref.ID, col2.Items[0].Ref.ID)
	}
}

// TestDV01DisabledDaemonProducesTypedUnavailable flips
// the fake agent to inactive; the lister must return
// Complete=false with the engine_unavailable Issue.
func TestDV01DisabledDaemonProducesTypedUnavailable(t *testing.T) {
	fa := newFakeAgentSocket(t)
	atomic.StoreInt32(&fa.active, 0)
	e := engine.New(fa.socketPath, "test")
	objectlisters.RegisterAllWithRealSourcesAndLinuxSource(e, nil, nil, nil, nil)
	col, err := e.ObjectList(context.Background(), "network.interface", object.CollectionParams{})
	if err != nil {
		t.Fatal(err)
	}
	if col.Complete {
		t.Fatal("Complete=true on a disabled daemon is a fake-empty-success violation")
	}
	if col.SnapshotID == "" {
		t.Fatal("SnapshotID must be set even on unavailable")
	}
	found := false
	for _, iss := range col.Issues {
		if iss.ReasonID == "engine_unavailable" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("must carry engine_unavailable Issue, got %+v", col.Issues)
	}
}
