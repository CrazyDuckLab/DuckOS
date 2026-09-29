//go:build linux

package layerl

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/engine"
	"github.com/CrazyDuckLab/DuckOS/internal/network"
	"github.com/CrazyDuckLab/DuckOS/internal/state"
)

func TestH02CompletedCLIReplaySurvivesRestart(t *testing.T) {
	f := newDV02Fixture(t)
	defer f.networking.Close()
	defer f.config.Close()
	cli := filepath.Join(t.TempDir(), "duck")
	build := exec.Command(filepath.Join(runtime.GOROOT(), "bin", "go"), "build", "-o", cli, "./cmd/duck")
	build.Dir = "../.."
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build CLI: %v: %s", err, out)
	}
	// Match the configured host boundary without bypassing authentication.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Host = "127.0.0.1:38081"
		f.apiHandler.ServeHTTP(w, r)
	}))
	defer srv.Close()
	token, candidate := filepath.Join(t.TempDir(), "token"), filepath.Join(t.TempDir(), "candidate.json")
	if err := os.WriteFile(token, []byte(f.hostToken), 0600); err != nil {
		t.Fatal(err)
	}
	body := engine.ConfigApplyRequest{Expected: f.currentRevID(t), RequestID: "abcdef0123456789abcdef0123456789", Candidate: mustJSON(t, dv02CandidateConfig("10.99.99.42/24")), Actor: "spoofed-owner", Reason: "h02-replay", Window: 300}
	if err := os.WriteFile(candidate, body.Candidate, 0600); err != nil {
		t.Fatal(err)
	}
	run := func() map[string]any {
		t.Helper()
		cmd := exec.Command(cli, "config", "apply", "--endpoint", srv.URL, "--allow-insecure", "--token-file", token, "--candidate", candidate, "--expected-revision", body.Expected, "--request-id", body.RequestID, "--actor", body.Actor, "--reason", body.Reason, "--confirm-seconds", "300", "--output", "json")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("CLI apply: %v: %s", err, out)
		}
		var result map[string]any
		if err := json.Unmarshal(out, &result); err != nil {
			t.Fatalf("CLI JSON: %v: %s", err, out)
		}
		return result
	}
	run()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if p := f.execStateJSON(t).Pending; p != nil && p.Phase == state.AwaitingConfirmation {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if code, raw := f.postConfig(t, "confirm", body.RequestID, nil); code != 200 {
		t.Fatalf("confirm %d: %s", code, raw)
	}
	first := run()
	if code, raw := f.postConfig(t, "apply", "", body); code != http.StatusOK {
		t.Fatalf("completed replay HTTP %d: %s", code, raw)
	}
	if first["phase"] != "committed" || first["remainingMs"] != float64(0) || first["completedAt"] == nil {
		t.Fatalf("not terminal: %v", first)
	}
	snap := f.stateJSON(t)
	if snap.Current.Actor != "duckdev-dv02" || len(snap.Completed) != 1 || snap.Completed[0].Candidate.Actor != snap.Current.Actor {
		t.Fatalf("session actor not durable: %q", snap.Current.Actor)
	}
	f.networking.Close()
	f.config.Close()
	if err := f.canonical.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := state.Open(f.canonicalDir, snap.Current.Config, func(raw json.RawMessage) error { _, err := network.Decode(raw); return err })
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	cfg, err := f.engine.EnableConfig(reopened)
	if err != nil {
		t.Fatal(err)
	}
	defer cfg.Close()
	net, err := f.engine.EnableNetworking(reopened)
	if err != nil {
		t.Fatal(err)
	}
	defer net.Close()
	cfg.EnableNetworking(net)
	if after := run(); !reflect.DeepEqual(first, after) {
		t.Fatalf("replay changed after restart: before=%v after=%v", first, after)
	}
	changed := body
	changed.Expected = snap.Current.ID
	if code, raw := f.postConfig(t, "apply", "", changed); code != 409 {
		t.Fatalf("changed expected: %d %s", code, raw)
	}
	changed = body
	changed.Candidate = mustJSON(t, dv02CandidateConfig("10.99.99.43/24"))
	if code, raw := f.postConfig(t, "apply", "", changed); code != 409 {
		t.Fatalf("changed payload: %d %s", code, raw)
	}
	if got := f.addrOracle(t, "dummy0"); strings.Count(got, "10.99.99.42") != 1 || strings.Contains(got, "10.99.99.43") {
		t.Fatalf("unexpected runtime: %s", got)
	}
}
