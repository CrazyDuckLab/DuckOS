package engine

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/agent"
	"github.com/CrazyDuckLab/DuckOS/internal/network"
	"github.com/CrazyDuckLab/DuckOS/internal/safeapply"
	"github.com/CrazyDuckLab/DuckOS/internal/state"
)

// This fixture exercises real IPC/journals, not Linux mutation semantics.
type journalTestAdapter struct{ live json.RawMessage }

// emptyBootKernel satisfies the BootKernel contract with an empty live
// snapshot, matching the test store's minimal `{"version":1}` journaled
// intent. The engine's IPC tests use it to admit the network service into
// the second-stage "ready" state.
type emptyBootKernel struct{ tables []int }

func (k *emptyBootKernel) EnrolledTables() []int { return append([]int(nil), k.tables...) }
func (k *emptyBootKernel) CaptureRoutes(context.Context) (network.RouteSnapshot, error) {
	return network.RouteSnapshot{Version: 1, Tables: append([]int{}, k.tables...)}, nil
}
func (k *emptyBootKernel) CaptureRules(context.Context) (network.PolicySnapshot, error) {
	return network.PolicySnapshot{Version: 1}, nil
}

func (a *journalTestAdapter) Preflight(context.Context, json.RawMessage, json.RawMessage) error {
	return nil
}
func (a *journalTestAdapter) Apply(_ context.Context, _, next json.RawMessage) error {
	a.live = append(json.RawMessage(nil), next...)
	return nil
}
func (a *journalTestAdapter) Restore(ctx context.Context, before, _ json.RawMessage) error {
	return a.Apply(ctx, nil, before)
}
func (a *journalTestAdapter) Verify(_ context.Context, want json.RawMessage) error {
	if !bytes.Equal(a.live, want) {
		return errors.New("synthetic state mismatch")
	}
	return nil
}

func TestNetworkLostAcknowledgementReconcilesWithoutReplay(t *testing.T) {
	initial := json.RawMessage(`{"version":1}`)
	openStore := func() *state.Store {
		t.Helper()
		dir := t.TempDir()
		if err := os.Chmod(dir, 0700); err != nil {
			t.Fatal(err)
		}
		s, err := state.Open(dir, initial, func(raw json.RawMessage) error { _, err := network.Decode(raw); return err })
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { s.Close() })
		return s
	}
	canonical, execution := openStore(), openStore()
	controller, err := safeapply.Open(execution, &journalTestAdapter{live: initial})
	if err != nil {
		t.Fatal(err)
	}
	service, err := agent.NewNetworkService(controller)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.StartupVerify(context.Background(), &emptyBootKernel{tables: []int{10000}}, network.Enrollment{Version: 1, CreatedAt: time.Now().UTC(), FirewallTable: "inet duckos", PolicyRouting: true, NetworkdReload: true, RoutingTables: []int{10000}, Physical: []network.PhysicalBinding{
		{ID: "uplink", Name: "testwan0", MAC: "02:00:00:00:00:01"},
		{ID: "inside", Name: "testlan0", MAC: "02:00:00:00:00:02"},
	}, VLANNames: []string{"testguest0"}}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { service.Close() })
	handler := agent.NewHandler(nil)
	handler.EnableNetwork(service)
	var submissions atomic.Int32
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == agent.NetworkPrefix+"apply" {
			submissions.Add(1)
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, r)
			// Admit the request, then lose the entire acknowledgement.
			conn, _, err := w.(http.Hijacker).Hijack()
			if err == nil {
				conn.Close()
			}
			return
		}
		handler.ServeHTTP(w, r)
	})}
	// macOS Unix socket paths are limited; testing.T's nested path is too long.
	dir, err := os.MkdirTemp("", "duck-ipc-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	socket := filepath.Join(dir, "a.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	go server.Serve(listener)
	t.Cleanup(func() { server.Close() })
	n, err := New(socket, "test").EnableNetworking(canonical)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(n.Close)
	snapshot, _ := canonical.Snapshot()
	candidate := json.RawMessage(`{"version":1,"interfaces":[{"id":"inside","name":"testlan0","kind":"physical","role":"lan","mtu":1500,"dhcpClient":"none","management":true,"addresses":["198.51.100.1/24"]}]}`)
	tx, err := n.Apply(context.Background(), candidate, snapshot.Current.ID, "owner", "synthetic IPC loss test", 120, "")
	if err != nil {
		t.Fatal(err)
	}
	wait := func(ready func(NetworkState) bool) NetworkState {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			s, err := n.Status(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if ready(s) {
				return s
			}
			time.Sleep(5 * time.Millisecond)
		}
		t.Fatal("journals did not converge")
		return NetworkState{}
	}
	s := wait(func(s NetworkState) bool {
		return s.Agent.Pending != nil && s.Agent.Pending.Phase == state.AwaitingConfirmation
	})
	if s.Current.ID != snapshot.Current.ID || s.Pending.ID != tx.ID || s.Agent.Pending.ID != tx.ID {
		t.Fatal("canonical state committed early or transaction identity diverged")
	}
	if err := n.Confirm(context.Background(), tx.ID); err != nil {
		t.Fatal(err)
	}
	s = wait(func(s NetworkState) bool { return s.Reconciled })
	if s.Current.ID != tx.Candidate.ID || len(s.Outcomes) != 1 || s.Outcomes[0].TransactionID != tx.ID || submissions.Load() != 1 {
		t.Fatal("lost acknowledgement caused replay or missing receipt")
	}
}

func TestNetworkStatusRejectsTamperedRevisionAndReceipt(t *testing.T) {
	config := json.RawMessage(`{"version":1}`)
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	s, err := state.Open(dir, config, func(raw json.RawMessage) error { _, err := network.Decode(raw); return err })
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	snapshot, _ := s.Snapshot()
	for name, mutate := range map[string]func(*agent.NetworkStatus){
		"digest":   func(s *agent.NetworkStatus) { s.Current.ID = "invalid" },
		"metadata": func(s *agent.NetworkStatus) { s.Current.Actor = "" },
		"receipt": func(s *agent.NetworkStatus) {
			s.Outcomes = []state.Outcome{{TransactionID: "invalid", Result: "committed"}}
		},
		"private evidence": func(s *agent.NetworkStatus) {
			s.Pending = &state.Transaction{Recovery: json.RawMessage(`{"private":true}`)}
		},
	} {
		t.Run(name, func(t *testing.T) {
			status := agent.NetworkStatus{Version: 1, Current: snapshot.Current}
			mutate(&status)
			data, _ := json.Marshal(status)
			n := Networking{client: &http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(bytes.NewReader(data))}, nil
			})}}
			if _, err := n.remoteStatus(context.Background()); err == nil {
				t.Fatal("malformed Agent status accepted")
			}
		})
	}
}

// TestBootstrapRealignsJournalWithAgentOverRolledBackNoise
// pins the divergence fix: the engine's journal may carry
// rolled-back outcomes from rejected apply attempts while the
// Agent's Current has been initialized from the live kernel
// with a different empty placeholder. The engine must
// reconcile its journal Current to the Agent's Current on
// startup so the next Prepare succeeds; the previous bootstrap
// gate (no Outcomes allowed) refused to re-seed and the first
// Prepare from the API always lost with HTTP 409
// revision_conflict.
func TestBootstrapRealignsJournalWithAgentOverRolledBackNoise(t *testing.T) {
	canonicalDir := t.TempDir()
	if err := os.Chmod(canonicalDir, 0700); err != nil {
		t.Fatal(err)
	}
	// Canonical journal starts with the engine's own
	// {"version":1} placeholder, exactly like duck-api does at
	// startup. The SHA-256 of `{"version":1}` is what the
	// witness catalog calls the "engine placeholder" identity.
	placeholder := json.RawMessage(`{"version":1}`)
	canonical, err := state.Open(canonicalDir, placeholder, func(raw json.RawMessage) error {
		_, e := network.Decode(raw)
		return e
	})
	if err != nil {
		t.Fatal(err)
	}
	defer canonical.Close()

	// Simulate the agent's first-boot baseline config — the
	// real cause of the divergence the witness caught. Its
	// SHA-256 is a different identity.
	agentBaseline := json.RawMessage(`{"dns":null,"firewall":null,"interfaces":null,"routes":null,"version":1}`)
	agentRevision, err := canonical.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	agentStatus := agent.NetworkStatus{
		Version: 1,
		Current: state.Revision{
			ID:        mustDigest(t, agentBaseline),
			Config:    agentBaseline,
			Actor:     "agent",
			Reason:    "live kernel capture",
			CreatedAt: time.Now().UTC(),
		},
	}
	if agentStatus.Current.ID == agentRevision.Current.ID {
		t.Fatal("test fixture does not establish a divergence between engine and agent Current")
	}

	// Roll-back noise: a rejected Prepare/Transition/
	// RolledBack cycle deposits a "rolled-back" outcome in the
	// canonical journal without advancing Current.
	tx, err := canonical.PrepareRequest(agentRevision.Current.ID, "", json.RawMessage(`{"version":1,"interfaces":[{"id":"x","name":"x0","kind":"physical","role":"lan","mtu":1500,"dhcpClient":"none","management":true,"addresses":["198.51.100.1/24"]}]}`), "witness", "fresh-install first apply", nil)
	if err != nil {
		t.Fatalf("seed rolled-back outcome: %v", err)
	}
	if err := canonical.Transition(tx.ID, state.RollingBack, time.Time{}); err != nil {
		t.Fatal(err)
	}
	if err := canonical.RolledBack(tx.ID); err != nil {
		t.Fatal(err)
	}
	preSeed, _ := canonical.Snapshot()
	if len(preSeed.Outcomes) != 1 || preSeed.Outcomes[0].Result != "rolled-back" {
		t.Fatalf("rolled-back outcome missing: %+v", preSeed.Outcomes)
	}

	// Wire a fake Agent transport so bootstrap can call
	// remoteStatus; it returns the Agent's divergent Current.
	statusBytes, _ := json.Marshal(agentStatus)
	n := &Networking{client: &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != agent.NetworkPrefix+"status" {
			return nil, errors.New("unexpected agent call: " + r.URL.Path)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(bytes.NewReader(statusBytes))}, nil
	})}, done: make(chan struct{})}
	// Run bootstrap directly. EnableNetworking is not safe
	// here (it spawns goroutines that expect a real Agent).
	go n.bootstrap(canonical, preSeed)
	<-n.done

	after, err := canonical.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if after.Current.ID != agentStatus.Current.ID {
		t.Fatalf("bootstrap did not realign: journal.Current=%s agent.Current=%s", after.Current.ID, agentStatus.Current.ID)
	}
	if len(after.History) != 0 {
		t.Fatalf("bootstrap must not create committed history on a fresh journal: %d", len(after.History))
	}
	if len(after.Outcomes) != 1 || after.Outcomes[0].Result != "rolled-back" {
		t.Fatalf("rolled-back outcome lost during re-seed: %+v", after.Outcomes)
	}
	if after.Pending != nil {
		t.Fatalf("bootstrap must not introduce a pending tx: %+v", after.Pending)
	}
}

// TestBootstrapPreservesCommittedHistory is the safety
// counterpart: a journal that has received a committed Apply
// must never be re-aligned by bootstrap, even if the Agent
// reports a divergent Current (e.g. stale netlink cache).
func TestBootstrapPreservesCommittedHistory(t *testing.T) {
	canonicalDir := t.TempDir()
	if err := os.Chmod(canonicalDir, 0700); err != nil {
		t.Fatal(err)
	}
	canonical, err := state.Open(canonicalDir, json.RawMessage(`{"version":1}`), func(raw json.RawMessage) error {
		_, e := network.Decode(raw)
		return e
	})
	if err != nil {
		t.Fatal(err)
	}
	defer canonical.Close()
	// Commit one Apply so the journal has History.
	committed := json.RawMessage(`{"version":1,"interfaces":[{"id":"inside","name":"inside0","kind":"physical","role":"lan","mtu":1500,"dhcpClient":"none","management":true,"addresses":["198.51.100.1/24"]}]}`)
	snap, _ := canonical.Snapshot()
	tx, err := canonical.Prepare(snap.Current.ID, committed, "owner", "synthetic history")
	if err != nil {
		t.Fatal(err)
	}
	if err := canonical.Transition(tx.ID, state.Applying, time.Time{}); err != nil {
		t.Fatal(err)
	}
	if err := canonical.Transition(tx.ID, state.AwaitingConfirmation, time.Now().Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := canonical.Commit(tx.ID); err != nil {
		t.Fatal(err)
	}
	committedSnap, _ := canonical.Snapshot()

	// Pretend the Agent now reports a different Current (e.g.
	// out-of-band capture).
	divergent := json.RawMessage(`{"version":1,"interfaces":[]}`)
	agentStatus := agent.NetworkStatus{
		Version: 1,
		Current: state.Revision{
			ID:        mustDigest(t, divergent),
			Config:    divergent,
			Actor:     "agent",
			Reason:    "stale cache",
			CreatedAt: time.Now().UTC(),
		},
	}
	statusBytes, _ := json.Marshal(agentStatus)
	n := &Networking{client: &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(bytes.NewReader(statusBytes))}, nil
	})}, done: make(chan struct{})}
	go n.bootstrap(canonical, committedSnap)
	<-n.done

	after, _ := canonical.Snapshot()
	if after.Current.ID != committedSnap.Current.ID {
		t.Fatalf("bootstrap overwrote committed history: %s -> %s", committedSnap.Current.ID, after.Current.ID)
	}
	if len(after.History) == 0 {
		t.Fatalf("committed history lost: %+v", after.History)
	}
}

// mustDigest computes the SHA-256 of the supplied JSON payload
// using the same canonicalisation the state store applies, so
// test fixtures match what the engine would observe on the
// live host.
func mustDigest(t *testing.T, raw json.RawMessage) string {
	t.Helper()
	var v any
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	if err := d.Decode(&v); err != nil {
		t.Fatal(err)
	}
	canonical, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:])
}

// TestNextPhaseTowardsWalksApplyToRecover pins the multi-step
// transition path the live qualification caught: the Agent walks
// Applying → RollingBack → RecoveryRequired inside its rollback
// goroutine, faster than the Engine's reconcile poll can keep
// up. The Engine's reconcile must step the canonical journal
// through RollingBack before it can advance to RecoveryRequired,
// because store.Transition only accepts single-step edges.
func TestNextPhaseTowardsWalksApplyToRecover(t *testing.T) {
	cases := []struct {
		name string
		from state.Phase
		to   state.Phase
		want state.Phase
	}{
		{"apply→awaiting", state.Applying, state.AwaitingConfirmation, state.AwaitingConfirmation},
		{"apply→rolling-back", state.Applying, state.RollingBack, state.RollingBack},
		{"apply→recovery-required", state.Applying, state.RecoveryRequired, state.RollingBack},
		{"rolling-back→recovery-required", state.RollingBack, state.RecoveryRequired, state.RecoveryRequired},
		{"rolling-back→awaiting not legal", state.RollingBack, state.AwaitingConfirmation, ""},
		{"already-at-target", state.Applying, state.Applying, ""},
		{"recover→any not legal", state.RecoveryRequired, state.RollingBack, ""},
		{"prepared→recovery-required", state.Prepared, state.RecoveryRequired, state.RollingBack},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := nextPhaseTowards(tc.from, tc.to)
			if got != tc.want {
				t.Fatalf("nextPhaseTowards(%s,%s) = %s, want %s", tc.from, tc.to, got, tc.want)
			}
		})
	}
}
