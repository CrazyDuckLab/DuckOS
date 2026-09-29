// G04 readiness IPC torture regression. The Engine must
// prefer the Agent IPC path and fall back to the local
// store only when the IPC channel is unreachable. A nil
// Engine must remain benign.
package engine

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/chaos"
)

func TestReadiness_NilEngineIsBenign(t *testing.T) {
	var e *Engine
	snap, err := e.Readiness(context.Background())
	if err != nil {
		t.Fatalf("nil engine readiness: %v", err)
	}
	if snap.Version != 0 {
		t.Fatalf("nil engine must return zero snapshot, got %+v", snap)
	}
}

func TestReadiness_IPCFallbackToLocal(t *testing.T) {
	e := New("/tmp/readiness-no-agent.sock", "test")
	// No SetReadiness + no agent -> IPC fails + no local store -> error
	if _, err := e.Readiness(context.Background()); err == nil {
		t.Fatalf("expected error when no IPC and no local store")
	}
	// Local store set, IPC still fails -> fallback succeeds
	store := chaos.NewReadinessStore()
	store.Set(chaos.FeatureReadiness{Feature: "vpn.wireguard", State: chaos.ReadySupported, Note: "kernel present"})
	e.SetReadiness(store)
	snap, err := e.Readiness(context.Background())
	if err != nil {
		t.Fatalf("Readiness fallback: %v", err)
	}
	if len(snap.Features) != 1 || snap.Features[0].Feature != "vpn.wireguard" {
		t.Fatalf("fallback row drift: %+v", snap)
	}
}

func TestReadiness_IPCPreferredOverLocal(t *testing.T) {
	// Local store has "old" feature; IPC returns "new".
	store := chaos.NewReadinessStore()
	store.Set(chaos.FeatureReadiness{Feature: "old", State: chaos.ReadySupported, Note: "local"})
	// Mock IPC by injecting a transport.
	e := New("/tmp/readiness-ipc.sock", "test")
	e.SetReadiness(store)
	calls := atomic.Int32{}
	e.client = &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		body := `{"version":1,"updatedAt":"2026-01-01T00:00:00Z","features":[{"feature":"new","state":"supported","note":"from agent","updatedAt":"2026-01-01T00:00:00Z"}]}`
		return &http.Response{
			StatusCode: 200,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(body)),
		}, nil
	})}
	snap, err := e.Readiness(context.Background())
	if err != nil {
		t.Fatalf("Readiness IPC: %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("IPC must be called once, got %d", calls.Load())
	}
	if len(snap.Features) != 1 || snap.Features[0].Feature != "new" {
		t.Fatalf("IPC must win over local store, got %+v", snap.Features)
	}
}

func TestReadiness_IPCNon200SurfacesError(t *testing.T) {
	e := New("/tmp/readiness-bad.sock", "test")
	e.client = &http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 503,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"error":"agent_busy"}`)),
		}, nil
	})}
	if _, err := e.Readiness(context.Background()); err == nil {
		t.Fatalf("expected error when IPC returns 503")
	}
}

func TestReadiness_IPCDecodeFailureSurfacesError(t *testing.T) {
	e := New("/tmp/readiness-decode.sock", "test")
	e.client = &http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 200,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{`)),
		}, nil
	})}
	if _, err := e.Readiness(context.Background()); err == nil {
		t.Fatalf("expected decode error")
	}
}

func TestReadiness_ZeroVersionEnvelopeRejected(t *testing.T) {
	e := New("/tmp/readiness-zero.sock", "test")
	e.client = &http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 200,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"version":0,"features":[]}`)),
		}, nil
	})}
	if _, err := e.Readiness(context.Background()); err == nil {
		t.Fatalf("expected version=0 to be rejected")
	}
}

func TestReadiness_LocalStoreCountersAreAccurate(t *testing.T) {
	e := New("/tmp/readiness-counters.sock", "test")
	store := chaos.NewReadinessStore()
	store.Set(chaos.FeatureReadiness{Feature: "a", State: chaos.ReadySupported})
	store.Set(chaos.FeatureReadiness{Feature: "b", State: chaos.ReadyHWPending})
	store.Set(chaos.FeatureReadiness{Feature: "c", State: chaos.ReadyDegraded})
	store.Set(chaos.FeatureReadiness{Feature: "d", State: chaos.ReadyCredPending})
	store.Set(chaos.FeatureReadiness{Feature: "e", State: chaos.ReadyExperimental})
	store.Set(chaos.FeatureReadiness{Feature: "f", State: chaos.ReadyUnsupported})
	e.SetReadiness(store)
	snap, err := e.Readiness(context.Background())
	if err != nil {
		t.Fatalf("Readiness: %v", err)
	}
	if snap.Counters.Total != 6 {
		t.Fatalf("total drift: %+v", snap.Counters)
	}
	if snap.Counters.Supported != 1 || snap.Counters.HardwarePending != 1 || snap.Counters.Degraded != 1 || snap.Counters.CredentialPending != 1 || snap.Counters.Experimental != 1 || snap.Counters.Unsupported != 1 {
		t.Fatalf("counter drift: %+v", snap.Counters)
	}
}

func TestReadiness_RoundTripJSON(t *testing.T) {
	e := New("/tmp/readiness-roundtrip.sock", "test")
	store := chaos.NewReadinessStore()
	store.Set(chaos.FeatureReadiness{Feature: "x", State: chaos.ReadySupported, Note: "ok"})
	e.SetReadiness(store)
	snap, err := e.Readiness(context.Background())
	if err != nil {
		t.Fatalf("Readiness: %v", err)
	}
	b, err := json.Marshal(snap)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back ReadinessSnapshot
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(back.Features) != 1 || back.Features[0].Feature != "x" {
		t.Fatalf("round-trip drift: %+v", back)
	}
}

func TestReadiness_TransportError(t *testing.T) {
	e := New("/tmp/readiness-transport.sock", "test")
	e.client = &http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("boom")
	})}
	if _, err := e.Readiness(context.Background()); err == nil {
		t.Fatalf("expected transport error")
	}
}

func TestReadiness_RowsAreSorted(t *testing.T) {
	e := New("/tmp/readiness-sort.sock", "test")
	store := chaos.NewReadinessStore()
	store.Set(chaos.FeatureReadiness{Feature: "zeta", State: chaos.ReadySupported})
	store.Set(chaos.FeatureReadiness{Feature: "alpha", State: chaos.ReadySupported})
	store.Set(chaos.FeatureReadiness{Feature: "mu", State: chaos.ReadySupported})
	e.SetReadiness(store)
	snap, err := e.Readiness(context.Background())
	if err != nil {
		t.Fatalf("Readiness: %v", err)
	}
	if len(snap.Features) != 3 {
		t.Fatalf("feature count drift: %d", len(snap.Features))
	}
	if snap.Features[0].Feature != "alpha" || snap.Features[1].Feature != "mu" || snap.Features[2].Feature != "zeta" {
		t.Fatalf("rows not sorted: %+v", snap.Features)
	}
}

// silence unused-import warning for time.
var _ = time.Second
