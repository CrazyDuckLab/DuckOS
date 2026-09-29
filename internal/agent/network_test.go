package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/network"
	"github.com/CrazyDuckLab/DuckOS/internal/safeapply"
	"github.com/CrazyDuckLab/DuckOS/internal/state"
)

type syntheticNetworkAdapter struct{ live json.RawMessage }

func (a *syntheticNetworkAdapter) Preflight(context.Context, json.RawMessage, json.RawMessage) error {
	return nil
}
func (a *syntheticNetworkAdapter) Apply(_ context.Context, _, next json.RawMessage) error {
	a.live = append(json.RawMessage(nil), next...)
	return nil
}
func (a *syntheticNetworkAdapter) Verify(_ context.Context, want json.RawMessage) error {
	if !bytes.Equal(a.live, want) {
		return errors.New("synthetic state mismatch")
	}
	return nil
}
func (a *syntheticNetworkAdapter) Restore(_ context.Context, before, _ json.RawMessage) error {
	a.live = append(json.RawMessage(nil), before...)
	return nil
}
func (a *syntheticNetworkAdapter) Capture(context.Context, json.RawMessage, json.RawMessage) (json.RawMessage, error) {
	return json.RawMessage(`{"privateEvidence":"synthetic kernel state"}`), nil
}
func (a *syntheticNetworkAdapter) RestoreCaptured(ctx context.Context, before, next, evidence json.RawMessage) error {
	return a.Restore(ctx, before, next)
}

// emptyBootKernel satisfies the BootKernel contract with an empty live
// snapshot. Tests that journal a minimal `{"version":1}` "before" use it to
// admit the service into the "ready" state.
type emptyBootKernel struct{ tables []int }

func (k *emptyBootKernel) EnrolledTables() []int { return append([]int(nil), k.tables...) }
func (k *emptyBootKernel) CaptureRoutes(context.Context) (network.RouteSnapshot, error) {
	return network.RouteSnapshot{Version: 1, Tables: append([]int{}, k.tables...)}, nil
}
func (k *emptyBootKernel) CaptureRules(context.Context) (network.PolicySnapshot, error) {
	return network.PolicySnapshot{Version: 1}, nil
}

func readyStartup(t *testing.T, n *NetworkService) {
	t.Helper()
	e := network.Enrollment{
		Version: 1, CreatedAt: time.Now().UTC(), FirewallTable: "inet duckos",
		Physical: []network.PhysicalBinding{
			{ID: "uplink", Name: "testwan0", MAC: "02:00:00:00:00:01"},
			{ID: "inside", Name: "testlan0", MAC: "02:00:00:00:00:02"},
		},
		VLANNames: []string{"testguest0"},
	}
	if err := n.StartupVerify(context.Background(), &emptyBootKernel{}, e); err != nil {
		t.Fatal("startup verify refused in test fixture", err)
	}
}

func networkHandler(t *testing.T) (*Handler, *state.Store) {
	t.Helper()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	s, err := state.Open(dir, json.RawMessage(`{"version":1}`), func(raw json.RawMessage) error { _, err := network.Decode(raw); return err })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	c, err := safeapply.Open(s, &syntheticNetworkAdapter{live: json.RawMessage(`{"version":1}`)})
	if err != nil {
		t.Fatal(err)
	}
	n, err := NewNetworkService(c)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { n.Close() })
	readyStartup(t, n)
	h := NewHandler(nil)
	h.EnableNetwork(n)
	return h, s
}

func networkRequest(h *Handler, method, path string, body []byte) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://agent"+path, bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func waitNetwork(t *testing.T, h *Handler, ready func(NetworkStatus) bool) NetworkStatus {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		w := networkRequest(h, "GET", NetworkPrefix+"status", nil)
		var status NetworkStatus
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &status) != nil {
			t.Fatal("status unavailable", w.Code, w.Body.String())
		}
		if strings.Contains(w.Body.String(), "privateEvidence") {
			t.Fatal("private recovery evidence leaked")
		}
		if ready(status) {
			return status
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("network transaction did not converge")
	return NetworkStatus{}
}

func TestNetworkIPCApplyConfirmAndReplayBoundary(t *testing.T) {
	h, s := networkHandler(t)
	snap, _ := s.Snapshot()
	input := NetworkApply{RequestID: "0123456789abcdef0123456789abcdef", ExpectedRevision: snap.Current.ID, Actor: "owner", Reason: "synthetic IPC test", ConfirmSeconds: 120, CreatedAt: time.Now().UTC(), Candidate: json.RawMessage(`{"version":1,"interfaces":[{"id":"inside","name":"testlan0","kind":"physical","role":"lan","mtu":1500,"dhcpClient":"none","management":true,"addresses":["198.51.100.1/24"]}]}`)}
	body, _ := json.Marshal(input)
	w := networkRequest(h, "POST", NetworkPrefix+"apply", body)
	if w.Code != 202 {
		t.Fatal(w.Code, w.Body.String())
	}
	status := waitNetwork(t, h, func(s NetworkStatus) bool { return s.Pending != nil && s.Pending.Phase == state.AwaitingConfirmation })
	if status.Pending.ID != input.RequestID || status.Current.ID != input.ExpectedRevision {
		t.Fatal("lost shared identity or committed early")
	}
	confirm, _ := json.Marshal(map[string]string{"transactionId": input.RequestID})
	if w = networkRequest(h, "POST", NetworkPrefix+"confirm", confirm); w.Code != 202 {
		t.Fatal(w.Code, w.Body.String())
	}
	status = waitNetwork(t, h, func(s NetworkStatus) bool { return s.Pending == nil && len(s.Outcomes) == 1 })
	if status.Outcomes[0].Result != "committed" || status.Outcomes[0].TransactionID != input.RequestID {
		t.Fatal("missing durable outcome")
	}
	if w = networkRequest(h, "POST", NetworkPrefix+"apply", body); w.Code != 409 {
		t.Fatal("replayed transaction admitted", w.Code)
	}
}

func TestNetworkIPCDisabledAndInvalidRequests(t *testing.T) {
	if w := networkRequest(NewHandler(nil), "GET", NetworkPrefix+"status", nil); w.Code != 503 {
		t.Fatal("networking enabled without enrollment")
	}
	h, _ := networkHandler(t)
	for _, tc := range []struct {
		method, path, body string
		want               int
	}{
		{"POST", NetworkPrefix + "apply", `{"command":"arbitrary"}`, 400},
		{"POST", NetworkPrefix + "confirm", `{"transactionId":"../../etc"}`, 400},
		{"POST", NetworkPrefix + "apply", `{} {}`, 400},
		{"POST", NetworkPrefix + "apply", strings.Repeat("x", state.MaxConfigBytes+4097), 400},
		{"GET", NetworkPrefix + "status?token=x", "", 400},
		{"GET", NetworkPrefix + "apply", "", 405},
	} {
		if w := networkRequest(h, tc.method, tc.path, []byte(tc.body)); w.Code != tc.want {
			t.Fatal(tc.path, w.Code, tc.want)
		}
	}
	stale := NetworkApply{RequestID: "0123456789abcdef0123456789abcdef", ConfirmSeconds: 120, CreatedAt: time.Now().Add(-2 * time.Minute), Candidate: json.RawMessage(`{"version":1}`)}
	body, _ := json.Marshal(stale)
	if w := networkRequest(h, "POST", NetworkPrefix+"apply", body); w.Code != 409 {
		t.Fatal("stale mutation request admitted", w.Code)
	}
}

func TestNetworkAdmissionCancellationPersistsBeforeDelayedApply(t *testing.T) {
	h, s := networkHandler(t)
	snapshot, _ := s.Snapshot()
	input := NetworkApply{RequestID: "abcdef0123456789abcdef0123456789", ExpectedRevision: snapshot.Current.ID, Actor: "owner", Reason: "synthetic cancellation", ConfirmSeconds: 120, CreatedAt: time.Now().UTC(), Candidate: json.RawMessage(`{"version":1,"interfaces":[{"id":"inside","name":"testlan0","kind":"physical","role":"lan","mtu":1500,"dhcpClient":"none","management":true,"addresses":["198.51.100.1/24"]}]}`)}
	body, _ := json.Marshal(input)
	for range 2 {
		if w := networkRequest(h, "POST", NetworkPrefix+"cancel-admission", body); w.Code != 200 {
			t.Fatal("idempotent cancellation failed", w.Code, w.Body.String())
		}
	}
	if w := networkRequest(h, "POST", NetworkPrefix+"apply", body); w.Code != 409 {
		t.Fatal("delayed apply bypassed tombstone", w.Code)
	}
	after, _ := s.Snapshot()
	if after.Pending != nil || after.Current.ID != snapshot.Current.ID || len(after.Outcomes) != 1 || after.Outcomes[0].TransactionID != input.RequestID || after.Outcomes[0].Result != "rolled-back" {
		t.Fatal("cancellation changed canonical state or lost receipt")
	}
}

// divergentBootKernel reports a private route that is not in the journaled
// intent. It is used to drive the second-stage gate into recovery_required.
type divergentBootKernel struct{}

func (k *divergentBootKernel) EnrolledTables() []int { return nil }
func (k *divergentBootKernel) CaptureRoutes(context.Context) (network.RouteSnapshot, error) {
	return network.RouteSnapshot{Version: 1, Tables: []int{10000}, Routes: []network.KernelRoute{
		{Family: "ipv4", Table: 10000, Destination: "10.0.0.0/8", Interface: "testwan0", Metric: 50},
	}}, nil
}
func (k *divergentBootKernel) CaptureRules(context.Context) (network.PolicySnapshot, error) {
	return network.PolicySnapshot{Version: 1}, nil
}

func TestNetworkStartupVerifyRefusesWorkUntilReady(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	s, err := state.Open(dir, json.RawMessage(`{"version":1}`), func(raw json.RawMessage) error { _, err := network.Decode(raw); return err })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	c, err := safeapply.Open(s, &syntheticNetworkAdapter{live: json.RawMessage(`{"version":1}`)})
	if err != nil {
		t.Fatal(err)
	}
	n, err := NewNetworkService(c)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { n.Close() })
	h := NewHandler(nil)
	h.EnableNetwork(n)

	// Apply must be refused while the service is still pending startup.
	input := NetworkApply{RequestID: "feedfacefeedfacefeedfacefeedface", ConfirmSeconds: 120, CreatedAt: time.Now().UTC(), Candidate: json.RawMessage(`{"version":1}`)}
	body, _ := json.Marshal(input)
	if w := networkRequest(h, "POST", NetworkPrefix+"apply", body); w.Code != 503 {
		t.Fatal("apply admitted before startup verify", w.Code, w.Body.String())
	}
	status := waitNetwork(t, h, func(s NetworkStatus) bool { return s.Startup != "" })
	if status.Startup != "pending" {
		t.Fatal("status did not report pending startup", status.Startup)
	}
}

func TestNetworkStartupVerifyTransitionsToRecoveryRequiredOnDrift(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	s, err := state.Open(dir, json.RawMessage(`{"version":1}`), func(raw json.RawMessage) error { _, err := network.Decode(raw); return err })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	c, err := safeapply.Open(s, &syntheticNetworkAdapter{live: json.RawMessage(`{"version":1}`)})
	if err != nil {
		t.Fatal(err)
	}
	n, err := NewNetworkService(c)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { n.Close() })
	e := network.Enrollment{Version: 1, CreatedAt: time.Now().UTC(), FirewallTable: "inet duckos",
		Physical: []network.PhysicalBinding{
			{ID: "uplink", Name: "testwan0", MAC: "02:00:00:00:00:01"},
			{ID: "inside", Name: "testlan0", MAC: "02:00:00:00:00:02"},
		}, VLANNames: []string{"testguest0"}, RoutingTables: []int{10000}, PolicyRouting: true, NetworkdReload: true,
	}
	if err := n.StartupVerify(context.Background(), &divergentBootKernel{}, e); err == nil {
		t.Fatal("startup verify accepted divergent kernel state")
	}
	h := NewHandler(nil)
	h.EnableNetwork(n)
	input := NetworkApply{RequestID: "beadbeadbeadbeadbeadbeadbeadbead", ConfirmSeconds: 120, CreatedAt: time.Now().UTC(), Candidate: json.RawMessage(`{"version":1}`)}
	body, _ := json.Marshal(input)
	if w := networkRequest(h, "POST", NetworkPrefix+"apply", body); w.Code != 503 {
		t.Fatal("apply admitted while recovery required", w.Code, w.Body.String())
	}
	status := waitNetwork(t, h, func(s NetworkStatus) bool { return s.Startup == "recovery_required" })
	if status.Startup != "recovery_required" || status.StartErr == "" {
		t.Fatal("status did not surface recovery state", status.Startup, status.StartErr)
	}
}

func TestNetworkStartupVerifyAdmitsWorkOnce(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	s, err := state.Open(dir, json.RawMessage(`{"version":1}`), func(raw json.RawMessage) error { _, err := network.Decode(raw); return err })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	c, err := safeapply.Open(s, &syntheticNetworkAdapter{live: json.RawMessage(`{"version":1}`)})
	if err != nil {
		t.Fatal(err)
	}
	n, err := NewNetworkService(c)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { n.Close() })
	e := network.Enrollment{Version: 1, CreatedAt: time.Now().UTC(), FirewallTable: "inet duckos",
		Physical: []network.PhysicalBinding{
			{ID: "uplink", Name: "testwan0", MAC: "02:00:00:00:00:01"},
			{ID: "inside", Name: "testlan0", MAC: "02:00:00:00:00:02"},
		}, VLANNames: []string{"testguest0"}, PolicyRouting: true, NetworkdReload: true,
	}
	if err := n.StartupVerify(context.Background(), &emptyBootKernel{}, e); err != nil {
		t.Fatal("startup verify refused empty kernel", err)
	}
	// Re-running verify on a ready service is idempotent.
	if err := n.StartupVerify(context.Background(), &emptyBootKernel{}, e); err != nil {
		t.Fatal("startup verify not idempotent", err)
	}
	// Nil kernel is an explicit refusal, never a fallback to "ready".
	if err := n.StartupVerify(context.Background(), nil, e); err == nil {
		t.Fatal("nil kernel silently accepted on already-ready service")
	}
}
