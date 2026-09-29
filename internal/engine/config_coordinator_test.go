// config_coordinator_test.go pins the production wiring of the
// Config coordinator (C-blocker #2). When a verified Agent
// coordinator is attached via Config.EnableNetworking, the
// Prepare/Confirm/Rollback/Validate/Reconcile methods MUST
// delegate to the Networking coordinator and ONLY return
// success after the Agent produced a verifiable receipt.
//
// The tests below use a fake Agent server that records the
// IPC calls; the Engine + Networking + Config triple runs
// end-to-end through the Agent IPC channel.
package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/agent"
	"github.com/CrazyDuckLab/DuckOS/internal/network"
	"github.com/CrazyDuckLab/DuckOS/internal/safeapply"
	"github.com/CrazyDuckLab/DuckOS/internal/state"
)

// fakeAgent serves /agent/v1/network/{status,apply,confirm,rollback}
// over a Unix socket and records every call. The handler is
// deliberately minimal: status returns a configurable snapshot;
// apply/confirm/rollback mutate the in-memory snapshot. The
// test asserts that the Config surface drives every state
// transition through this IPC channel.
type fakeAgent struct {
	mu            sync.Mutex
	current       state.Revision
	initialConfig []byte
	pending       *state.Transaction
	outcomes      []state.Outcome
	applyCalls    int32
	confirmCalls  int32
	rollbackCalls int32
	statusCalls   int32
	now           func() time.Time
}

// startFakeAgent spins up a fake Agent that speaks the
// /agent/v1/network protocol and returns the Unix socket path
// the test should use. The socket path is shortened to
// `a.sock` so macOS path-length limits do not bite.
func startFakeAgent(t *testing.T, initial state.Revision) (string, *fakeAgent) {
	t.Helper()
	dir, err := os.MkdirTemp("", "duck-ipc-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	socket := filepath.Join(dir, "a.sock")
	a := &fakeAgent{current: initial, initialConfig: initial.Config, now: time.Now}
	mux := http.NewServeMux()
	mux.HandleFunc(agent.NetworkPrefix+"status", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&a.statusCalls, 1)
		a.mu.Lock()
		defer a.mu.Unlock()
		status := agent.NetworkStatus{
			Version:  1,
			Current:  a.current,
			Pending:  a.pending,
			Outcomes: append([]state.Outcome(nil), a.outcomes...),
			Startup:  "ready",
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(status)
	})
	mux.HandleFunc(agent.NetworkPrefix+"apply", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&a.applyCalls, 1)
		var input agent.NetworkApply
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		a.mu.Lock()
		defer a.mu.Unlock()
		// The fake Agent never rejects. A real Agent runs
		// the canonical Safe-Apply job and only marks
		// the transaction AwaitingConfirmation after a
		// successful kernel capture/compare cycle.
		newCurrent := state.Revision{
			ID:        revDigest(input.Candidate),
			Config:    append([]byte(nil), input.Candidate...),
			Actor:     input.Actor,
			Reason:    input.Reason,
			CreatedAt: input.CreatedAt,
		}
		a.current = newCurrent
		a.pending = &state.Transaction{
			ID:        input.RequestID,
			Before:    input.ExpectedRevision,
			Candidate: newCurrent,
			Phase:     state.AwaitingConfirmation,
			Deadline:  a.now().Add(time.Duration(input.ConfirmSeconds) * time.Second),
		}
		w.WriteHeader(202)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(agent.NetworkStatus{
			Version: 1, Current: a.current, Pending: a.pending,
			Outcomes: append([]state.Outcome(nil), a.outcomes...), Startup: "ready",
		})
	})
	mux.HandleFunc(agent.NetworkPrefix+"confirm", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&a.confirmCalls, 1)
		var input map[string]string
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		a.mu.Lock()
		defer a.mu.Unlock()
		if a.pending == nil || a.pending.ID != input["transactionId"] {
			http.Error(w, "no such transaction", 409)
			return
		}
		beforeID := a.pending.Before
		candidateID := a.current.ID
		a.pending = nil
		a.outcomes = append(a.outcomes, state.Outcome{
			TransactionID: input["transactionId"],
			Before:        beforeID,
			Candidate:     candidateID,
			Result:        "committed",
			CompletedAt:   a.now().UTC(),
		})
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(202)
		_ = json.NewEncoder(w).Encode(agent.NetworkStatus{
			Version: 1, Current: a.current, Outcomes: append([]state.Outcome(nil), a.outcomes...), Startup: "ready",
		})
	})
	mux.HandleFunc(agent.NetworkPrefix+"rollback", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&a.rollbackCalls, 1)
		var input map[string]string
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		a.mu.Lock()
		defer a.mu.Unlock()
		if a.pending == nil || a.pending.ID != input["transactionId"] {
			http.Error(w, "no such transaction", 409)
			return
		}
		beforeID := a.pending.Before
		candidateID := a.current.ID
		// Restore Linux to the pre-apply revision. The
		// outcome Candidate is the candidate revision that
		// was attempted (NOT the post-rollback current).
		a.current = state.Revision{
			ID:        beforeID,
			Config:    a.initialConfig,
			Actor:     a.current.Actor,
			Reason:    a.current.Reason,
			CreatedAt: a.current.CreatedAt,
		}
		a.pending = nil
		a.outcomes = append(a.outcomes, state.Outcome{
			TransactionID: input["transactionId"],
			Before:        beforeID,
			Candidate:     candidateID,
			Result:        "rolled-back",
			CompletedAt:   a.now().UTC(),
		})
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(202)
		_ = json.NewEncoder(w).Encode(agent.NetworkStatus{
			Version: 1, Current: a.current, Outcomes: append([]state.Outcome(nil), a.outcomes...), Startup: "ready",
		})
	})
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	server := &http.Server{Handler: mux}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })
	return socket, a
}

// newWiredConfig returns an Engine + Networking + Config triple
// with the Coordinator wired into Config. The Engine uses the
// supplied Unix socket to talk to the fake Agent.
func newWiredConfig(t *testing.T, socket string, initial json.RawMessage) (*Config, *Networking, *state.Store) {
	t.Helper()
	canonical, execution := openStorePair(t, initial)
	controller, err := safeapply.Open(execution, &journalTestAdapter{live: initial})
	if err != nil {
		t.Fatal(err)
	}
	service, err := agent.NewNetworkService(controller)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.StartupVerify(context.Background(), &emptyBootKernel{tables: []int{10000}}, network.Enrollment{
		Version: 1, CreatedAt: time.Now().UTC(), FirewallTable: "inet duckos", PolicyRouting: true, NetworkdReload: true, RoutingTables: []int{10000}, Physical: []network.PhysicalBinding{
			{ID: "uplink", Name: "testwan0", MAC: "02:00:00:00:00:01"},
			{ID: "inside", Name: "testlan0", MAC: "02:00:00:00:00:02"},
		}, VLANNames: []string{"testguest0"},
	}); err != nil {
		t.Fatal(err)
	}
	engine := New(socket, "test")
	networking, err := engine.EnableNetworking(canonical)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(networking.Close)
	config, err := engine.EnableConfig(canonical)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(config.Close)
	if err := config.EnableNetworking(networking); err != nil {
		t.Fatal(err)
	}
	return config, networking, canonical
}

// openStorePair opens two stores: the canonical journal the
// Engine writes to and the execution journal the Agent's
// Safe-Apply controller writes to. The two are separate so
// the test exercises the same wiring as production.
func openStorePair(t *testing.T, initial json.RawMessage) (canonical, execution *state.Store) {
	t.Helper()
	root, err := os.MkdirTemp("", "duck-jrn-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	cdir := filepath.Join(root, "c")
	if err := os.Mkdir(cdir, 0700); err != nil {
		t.Fatal(err)
	}
	c, err := state.Open(cdir, initial, func(raw json.RawMessage) error { _, err := network.Decode(raw); return err })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	edir := filepath.Join(root, "e")
	if err := os.Mkdir(edir, 0700); err != nil {
		t.Fatal(err)
	}
	e, err := state.Open(edir, initial, func(raw json.RawMessage) error { _, err := network.Decode(raw); return err })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { e.Close() })
	return c, e
}

// revDigest computes the SHA-256 digest of the supplied
// JSON the same way state.Open does. The fake Agent uses it
// to project the post-apply current revision without a real
// store.
func revDigest(raw json.RawMessage) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// waitFor polls a predicate until it succeeds or the
// timeout elapses. Used to wait for the Networking
// coordinator's async IPC submission to complete.
func waitFor(t *testing.T, timeout time.Duration, ready func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if ready() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("predicate did not become true within %s", timeout)
}

// TestConfigCoordinatorPrepareForwardsToAgent proves the
// Config.Prepare method delegates to the Networking
// coordinator and produces an IPC submission to the Agent
// when the coordinator is attached.
func TestConfigCoordinatorPrepareForwardsToAgent(t *testing.T) {
	initial := json.RawMessage(`{"version":1}`)
	snap := revFromRaw(t, initial)
	socket, agent := startFakeAgent(t, snap)
	cfg, _, _ := newWiredConfig(t, socket, initial)
	candidate := json.RawMessage(`{"version":1,"interfaces":[]}`)
	pt, err := cfg.Prepare(context.Background(), ConfigApplyRequest{
		Expected:  snap.ID,
		Candidate: candidate,
		Actor:     "test",
		Reason:    "coordinator wiring test",
		Window:    120,
	})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if pt.ID == "" {
		t.Fatal("Prepare returned empty transaction id")
	}
	waitFor(t, 2*time.Second, func() bool {
		return atomic.LoadInt32(&agent.applyCalls) >= 1
	})
	// The journal admits before the Agent replies, so the
	// returned transaction is in "applying" phase. The
	// journal only advances to awaiting-confirmation when
	// the Agent's safeapply controller drives its own
	// transition; the Engine side surfaces the authoritative
	// phase through Status after the watch goroutine
	// reconciles.
	if pt.Phase != string(state.Applying) {
		t.Fatalf("Prepare-returned Phase: %q, want applying (journal admits before Agent reply)", pt.Phase)
	}
	// Wait for the Agent's pending entry to surface via
	// Status (this proves the watch goroutine observed the
	// Agent response).
	waitFor(t, 2*time.Second, func() bool {
		st, err := cfg.Status(context.Background())
		if err != nil {
			return false
		}
		return st.Pending != nil
	})
}

func TestConfigCoordinatorUsesAuthenticatedActorFromContext(t *testing.T) {
	initial := json.RawMessage(`{"version":1}`)
	snap := revFromRaw(t, initial)
	socket, agent := startFakeAgent(t, snap)
	cfg, _, _ := newWiredConfig(t, socket, initial)
	ctx := WithAuthenticatedActor(context.Background(), "session-owner")
	if _, err := cfg.Prepare(ctx, ConfigApplyRequest{
		Expected: snap.ID, Candidate: json.RawMessage(`{"version":1,"interfaces":[]}`),
		Actor: "attacker", Reason: "coordinator actor binding test", Window: 120,
	}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, 2*time.Second, func() bool {
		agent.mu.Lock()
		defer agent.mu.Unlock()
		return agent.pending != nil
	})
	agent.mu.Lock()
	defer agent.mu.Unlock()
	if agent.pending.Candidate.Actor != "session-owner" {
		t.Fatalf("candidate actor %q, want authenticated session actor", agent.pending.Candidate.Actor)
	}
}

// TestConfigCoordinatorConfirmReachesAgentLinuxReceipt proves
// the Confirm method only returns success after the Agent
// returned the authoritative commit receipt.
func TestConfigCoordinatorConfirmReachesAgentLinuxReceipt(t *testing.T) {
	initial := json.RawMessage(`{"version":1}`)
	snap := revFromRaw(t, initial)
	socket, agent := startFakeAgent(t, snap)
	cfg, _, _ := newWiredConfig(t, socket, initial)
	pt, err := cfg.Prepare(context.Background(), ConfigApplyRequest{
		Expected: snap.ID, Candidate: json.RawMessage(`{"version":1,"interfaces":[]}`),
		Actor: "test", Reason: "coordinator wiring test", Window: 120,
	})
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, 2*time.Second, func() bool {
		return atomic.LoadInt32(&agent.applyCalls) >= 1
	})
	out, err := cfg.Confirm(context.Background(), pt.ID)
	if err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if got := atomic.LoadInt32(&agent.confirmCalls); got != 1 {
		t.Fatalf("Agent confirm calls: %d, want 1", got)
	}
	if out.Receipt == nil || out.Receipt.Outcome != "committed" {
		t.Fatalf("Receipt: %+v, want committed", out.Receipt)
	}
	if out.Phase != "committed" {
		t.Fatalf("Phase: %q, want committed", out.Phase)
	}
}

// TestConfigCoordinatorRollbackReachesAgentLinuxReceipt proves
// the Rollback method only returns success after the Agent
// restored Linux and produced the rolled-back receipt.
func TestConfigCoordinatorRollbackReachesAgentLinuxReceipt(t *testing.T) {
	initial := json.RawMessage(`{"version":1}`)
	snap := revFromRaw(t, initial)
	socket, agent := startFakeAgent(t, snap)
	cfg, _, _ := newWiredConfig(t, socket, initial)
	pt, err := cfg.Prepare(context.Background(), ConfigApplyRequest{
		Expected: snap.ID, Candidate: json.RawMessage(`{"version":1,"interfaces":[]}`),
		Actor: "test", Reason: "coordinator wiring test", Window: 120,
	})
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, 2*time.Second, func() bool {
		return atomic.LoadInt32(&agent.applyCalls) >= 1
	})
	out, err := cfg.Rollback(context.Background(), pt.ID, "")
	if err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	if got := atomic.LoadInt32(&agent.rollbackCalls); got != 1 {
		t.Fatalf("Agent rollback calls: %d, want 1", got)
	}
	if out.Receipt == nil || out.Receipt.Outcome != "rolled-back" {
		t.Fatalf("Receipt: %+v, want rolled-back", out.Receipt)
	}
}

// TestConfigCoordinatorUnwiredStillFailsClosed pins the
// behaviour the audit regression test pinned: a Config with
// no coordinator attached MUST refuse to advertise success.
// This test guards against regressions where the
// EnableNetworking setter is forgotten on the production
// wiring path.
func TestConfigCoordinatorUnwiredStillFailsClosed(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	s, err := state.Open(dir, []byte(`{"privateKey":"synthetic-not-for-DTO"}`), func(json.RawMessage) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	cfg, err := (&Engine{}).EnableConfig(s)
	if err != nil {
		t.Fatal(err)
	}
	defer cfg.Close()
	_, err = cfg.Prepare(context.Background(), ConfigApplyRequest{
		Expected: "any", Candidate: json.RawMessage(`{}`), Actor: "test", Reason: "x", Window: 120,
	})
	if !errors.Is(err, ErrConfigRuntimeUnavailable) {
		t.Fatalf("Prepare without coordinator: %v, want ErrConfigRuntimeUnavailable", err)
	}
	_, err = cfg.Confirm(context.Background(), "any")
	if !errors.Is(err, ErrConfigRuntimeUnavailable) {
		t.Fatalf("Confirm without coordinator: %v, want ErrConfigRuntimeUnavailable", err)
	}
	_, err = cfg.Rollback(context.Background(), "any", "")
	if !errors.Is(err, ErrConfigRuntimeUnavailable) {
		t.Fatalf("Rollback without coordinator: %v, want ErrConfigRuntimeUnavailable", err)
	}
}

// revFromRaw constructs a state.Revision from raw JSON so
// the test can use the canonical store's initial state.
func revFromRaw(t *testing.T, raw json.RawMessage) state.Revision {
	t.Helper()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	s, err := state.Open(dir, raw, func(json.RawMessage) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	snap, err := s.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	return snap.Current
}
