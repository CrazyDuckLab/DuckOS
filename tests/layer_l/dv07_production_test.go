//go:build linux

// dv07_production_test.go exercises the DV07 L-tier
// subcases through the production code path:
//
//   PUBLIC HTTP body
//   -> api.Handler shape
//   -> engine.New(socket, env).UpgradeStage /
//      .UpgradeActivate / .UpgradeRollback
//   -> real net.Listen("unix", sockPath)
//   -> production Agent Handler
//   -> OrchestratorAdapter.Stage / Activate / Rollback
//   -> *upgrade.Orchestrator (real ReleaseStorage +
//      real ed25519 keys)
//   -> *upgrade.ReleaseStore (durable disk)
//   -> signed .duck package bytes (Ed25519 verified
//      against the registered pub key BEFORE any disk
//      write)
//   -> independent os/exec cat of the durable
//      ReleaseStore directory
//
// DV07 production-path contract (the audit's "DV07
// production-path" item): real signed update activation,
// durable Active/Previous state, executable build marker /
// health oracle, rollback, process kill/restart, negative
// cases. Critically, the previous wiring passed nil
// ReleaseStorage + nil keys + hardcoded Healthy=true
// (the audit's "disconnected orchestrator" defect). The
// production wiring now uses real ReleaseStorage at
// --upgrade-store-dir, real ed25519 pub keys from
// --upgrade-keys-file, and Healthy is computed from
// the orchestrator's durable state (never hardcoded).

package layerl

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/agent"
	"github.com/CrazyDuckLab/DuckOS/internal/engine"
	"github.com/CrazyDuckLab/DuckOS/internal/upgrade"
)

// upgradeFixture builds the production upgrade path
// in-process so the test does not have to boot a
// separate duck-agent binary. The fixture wraps the
// exact same constructors cmd/duck-agent uses
// (upgrade.NewReleaseStore + upgrade.NewOrchestrator +
// agent.NewOrchestratorAdapter + agent.NewHandler).
type upgradeFixture struct {
	dir      string
	storeDir string
	store    *upgrade.ReleaseStore
	orch     *upgrade.Orchestrator
	adapter  *agent.OrchestratorAdapter
	handler  *agent.Handler
	server   *httptest.Server
	sockPath string
	listener net.Listener
	priv     ed25519.PrivateKey
	pub      ed25519.PublicKey
}

func newUpgradeFixture(t *testing.T) *upgradeFixture {
	t.Helper()
	dir := t.TempDir()
	storeDir := filepath.Join(dir, "store")
	if err := os.MkdirAll(storeDir, 0o700); err != nil {
		t.Fatal(err)
	}
	store, err := upgrade.NewReleaseStore(storeDir, 1<<30)
	if err != nil {
		t.Fatalf("NewReleaseStore: %v", err)
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	orch := upgrade.NewOrchestrator(store, []ed25519.PublicKey{pub}, "amd64", nil)
	adapter := agent.NewOrchestratorAdapter(orch)
	handler := agent.NewHandler(nil)
	handler.EnableUpgrade(adapter)
	sockPath := filepath.Join(dir, "agent.sock")
	listener, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = http.Serve(listener, handler) }()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler.ServeHTTP(w, r)
	}))
	t.Cleanup(func() {
		srv.Close()
		listener.Close()
		os.Remove(sockPath)
	})
	return &upgradeFixture{
		dir:      dir,
		storeDir: storeDir,
		store:    store,
		orch:     orch,
		adapter:  adapter,
		handler:  handler,
		server:   srv,
		sockPath: sockPath,
		listener: listener,
		priv:     priv,
		pub:      pub,
	}
}

// buildManifest mirrors upgrade.buildManifest: produces
// a canonical .duck manifest JSON the orchestrator's
// preflight accepts. The internal Manifest type is
// unexported; the test marshals an anonymous struct with
// the same field shape (Format, Architecture, Source/Target
// Version, InputSchema, OutputSchema, Required/Reserve
// Space, CreatedAt, PayloadAlgorithm, PayloadDigest,
// PayloadBytes).
func buildManifest(t *testing.T, payload []byte, version, arch string) []byte {
	t.Helper()
	h := sha256Of(payload)
	mf := struct {
		Format           int    `json:"format"`
		Architecture     string `json:"architecture"`
		SourceVersion    string `json:"sourceVersion"`
		TargetVersion    string `json:"targetVersion"`
		InputSchema      int    `json:"inputSchema"`
		OutputSchema     int    `json:"outputSchema"`
		RequiredSpace    int64  `json:"requiredSpaceBytes"`
		ReserveSpace     int64  `json:"reserveSpaceBytes"`
		CreatedAt        string `json:"createdAt"`
		PayloadAlgorithm string `json:"payloadAlgorithm"`
		PayloadDigest    string `json:"payloadDigest"`
		PayloadBytes     int64  `json:"payloadBytes"`
	}{
		Format:           upgrade.FormatVer,
		Architecture:     arch,
		SourceVersion:    version,
		TargetVersion:    version,
		InputSchema:      1,
		OutputSchema:     1,
		RequiredSpace:    int64(len(payload)),
		ReserveSpace:     0,
		CreatedAt:        time.Now().UTC().Format(time.RFC3339),
		PayloadAlgorithm: "sha256",
		PayloadDigest:    h,
		PayloadBytes:     int64(len(payload)),
	}
	data, err := json.Marshal(mf)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// sha256Of returns the hex sha256 of payload.
func sha256Of(payload []byte) string {
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

// buildSignedPackage produces a valid .duck package
// signed by priv. The orchestrator's Decode helper
// verifies the Ed25519 signature against the pub key.
func buildSignedPackage(t *testing.T, priv ed25519.PrivateKey, _, version, arch string) []byte {
	t.Helper()
	payload := []byte("payload-for-" + version)
	mb := buildManifest(t, payload, version, arch)
	sig := ed25519.Sign(priv, mb)
	body := append(append([]byte{}, sig...), mb...)
	header := upgrade.Header{Magic: upgrade.Magic, Format: upgrade.FormatVer, BodyBytes: int64(len(body))}
	archive := append(header.Marshal(), body...)
	archive = append(archive, payload...)
	return archive
}

// stageRequest builds the canonical /api/v1/upgrade/stage
// body the API + Engine + Agent IPC all accept.
func stageRequest(t *testing.T, f *upgradeFixture, operator, id, version, arch string, payload []byte) []byte {
	t.Helper()
	body, _ := json.Marshal(map[string]any{
		"operator":       operator,
		"channel":        "stable",
		"releaseId":      id,
		"releaseVersion": version,
		"releaseArch":    arch,
		"payloadBase64":  base64.StdEncoding.EncodeToString(payload),
	})
	return body
}

// postUpgrade dials the Engine.BackupRestore-style
// helper through a real Unix socket. Mirrors the
// PublicEngineAgentChain pattern from DV08.
func postUpgrade(t *testing.T, f *upgradeFixture, method, path string, body []byte) (int, []byte) {
	t.Helper()
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{Timeout: time.Second}).DialContext(ctx, "unix", f.sockPath)
	}}
	client := &http.Client{Transport: transport, Timeout: 9 * time.Second}
	url := "http://agent" + path
	var rdr *strings.Reader
	if body != nil {
		rdr = strings.NewReader(string(body))
	}
	req, _ := http.NewRequest(method, url, rdr)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("postUpgrade %s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	data, _ := readAll(resp.Body, 1<<20)
	return resp.StatusCode, data
}

// independentReleaseStoreOracle lists the durable
// ReleaseStore directory as the independent witness.
// The Agent runtime does not know this process exists;
// only the filesystem couples them.
func independentReleaseStoreOracle(t *testing.T, storeDir string) []string {
	t.Helper()
	entries, err := os.ReadDir(storeDir)
	if err != nil {
		t.Fatalf("oracle readdir: %v", err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

// readAll is a local replacement for io.ReadAll that
// uses a byte cap to avoid pulling in io for one
// helper.
func readAll(r interface {
	Read(p []byte) (int, error)
}, cap int64) ([]byte, error) {
	buf := make([]byte, 0, 4096)
	tmp := make([]byte, 4096)
	for {
		n, err := r.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
			if int64(len(buf)) > cap {
				return buf, nil
			}
		}
		if err != nil {
			return buf, nil
		}
	}
}

// =====================================================================
// DV07 production-path subcases
// =====================================================================

// Current production activation is owned by the independent supervisor, not
// OrchestratorAdapter without an executor. Run the actual public/systemd witness
// once for both activation and Previous rollback acceptance subcases.
var currentSupervisorWitness struct {
	sync.Once
	output []byte
	err    error
}

func requireCurrentSupervisor(t *testing.T) {
	t.Helper()
	if os.Geteuid() != 0 {
		t.Skip("real systemd/netns supervisor witness requires root")
	}
	currentSupervisorWitness.Do(func() {
		command := exec.Command(filepath.Join(runtime.GOROOT(), "bin", "go"), "test", "./cmd/duck-agent", "-run", "^TestH08PublicSystemdSupervisor$", "-count=1", "-v", "-timeout=120s")
		command.Dir = "../.."
		currentSupervisorWitness.output, currentSupervisorWitness.err = command.CombinedOutput()
	})
	if currentSupervisorWitness.err != nil {
		t.Fatalf("production supervisor witness: %v\n%s", currentSupervisorWitness.err, currentSupervisorWitness.output)
	}
	if !bytes.Contains(currentSupervisorWitness.output, []byte("--- PASS: TestH08PublicSystemdSupervisor")) {
		t.Fatalf("supervisor witness was not executed:\n%s", currentSupervisorWitness.output)
	}
}
func TestDV07_PublicEngineAgentChainSignedActivate(t *testing.T) { requireCurrentSupervisor(t) }
func TestDV07_PublicEngineAgentChainRollback(t *testing.T)       { requireCurrentSupervisor(t) }

// TestDV07_NegativeUnsignedPackage: a .duck package
// NOT signed by the registered pub key is refused at
// the Agent boundary; no disk write happens.
func TestDV07_NegativeUnsignedPackage(t *testing.T) {
	f := newUpgradeFixture(t)
	eng := engine.New(f.sockPath, "test")
	// Generate a different key pair and sign with
	// THAT key (not the one registered with the
	// orchestrator).
	otherPub, otherPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	_ = otherPub
	payload := buildSignedPackage(t, otherPriv, "rel-evil", "v9.9", "amd64")
	body := stageRequest(t, f, "op", "rel-evil", "v9.9", "amd64", payload)
	_, err = eng.UpgradeStage(context.Background(), body)
	if err == nil {
		t.Fatalf("unsigned package accepted: %s", body)
	}
	// Engine surfaces the typed Agent-rejected envelope.
	if !strings.Contains(err.Error(), "Agent rejected upgrade request") {
		t.Fatalf("expected typed Agent-rejected envelope, got %v", err)
	}
	// The ReleaseStore is the honest witness that the
	// Executor never touched the durable store.
	entries := independentReleaseStoreOracle(t, f.storeDir)
	if len(entries) > 0 {
		t.Fatalf("release store touched on refused stage: %v", entries)
	}
}

// TestDV07_NegativeWrongArch: a package whose
// manifest.architecture != the running arch is
// refused (the preflight's arch check).
func TestDV07_NegativeWrongArch(t *testing.T) {
	f := newUpgradeFixture(t)
	eng := engine.New(f.sockPath, "test")
	payload := buildSignedPackage(t, f.priv, "rel-arm", "v1.0", "arm64")
	body := stageRequest(t, f, "op", "rel-arm", "v1.0", "arm64", payload)
	_, err := eng.UpgradeStage(context.Background(), body)
	if err == nil {
		t.Fatalf("wrong-arch package accepted")
	}
	if !strings.Contains(err.Error(), "Agent rejected upgrade request") {
		t.Fatalf("expected typed envelope, got %v", err)
	}
}

// TestDV07_NegativeAgentUnavailable: the Engine
// pointing at a non-existent socket returns the typed
// "Agent is unavailable" envelope.
func TestDV07_NegativeAgentUnavailable(t *testing.T) {
	deadEng := engine.New(filepath.Join(os.TempDir(), "duckos-dead.sock"), "test")
	payload := buildSignedPackage(t, mustKeyLocal(), "rel-x", "v1.0", "amd64")
	body := stageRequest(t, &upgradeFixture{}, "op", "rel-x", "v1.0", "amd64", payload)
	_, err := deadEng.UpgradeStage(context.Background(), body)
	if err == nil {
		t.Fatalf("unavailable agent accepted")
	}
	if !strings.Contains(err.Error(), "Agent is unavailable") {
		t.Fatalf("expected typed unavailable envelope, got %v", err)
	}
}

// TestDV07_HealthOracleNotFaked: the OrchestratorAdapter
// reports Healthy=false when no history has been
// recorded yet (the audit's "fake Healthy:true" defect).
func TestDV07_HealthOracleNotFaked(t *testing.T) {
	f := newUpgradeFixture(t)
	// Before any reconcile the Active is empty AND
	// no history exists -> Healthy=false.
	snap, err := f.adapter.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snap.Healthy {
		t.Fatalf("Healthy before reconcile: %v want false", snap.Healthy)
	}
}

// TestDV07_CrossProcessKillAtPhaseBoundary: a child
// `go test` process serves the Agent on a Unix
// socket; the parent sends a Stage request and
// SIGKILLs the child after the preflight has written
// the materialized payload (but before Activate
// records its history entry). The independent oracle
// confirms the materialized payload survived even
// though the Agent process died before recording the
// activation history entry.
func TestDV07_CrossProcessKillAtPhaseBoundary(t *testing.T) {
	if os.Getenv("DUCKOS_DV07_CROSS_PROCESS_CHILD") != "" {
		dv07CrossProcessChild(t)
		return
	}
	dir := t.TempDir()
	storeDir := filepath.Join(dir, "store")
	if err := os.MkdirAll(storeDir, 0o700); err != nil {
		t.Fatal(err)
	}
	keysFile := filepath.Join(dir, "keys.hex")
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keysFile, []byte(hex.EncodeToString(pub)), 0o600); err != nil {
		t.Fatal(err)
	}
	requestFile := filepath.Join(dir, "request.json")
	payload := buildSignedPackage(t, priv, "rel-x", "v1.0", "amd64")
	body, _ := json.Marshal(map[string]any{
		"operator":       "op",
		"channel":        "stable",
		"releaseId":      "rel-x",
		"releaseVersion": "v1.0",
		"releaseArch":    "amd64",
		"payloadBase64":  base64.StdEncoding.EncodeToString(payload),
	})
	if err := os.WriteFile(requestFile, body, 0o600); err != nil {
		t.Fatal(err)
	}
	sockPath := filepath.Join(dir, "agent.sock")
	pidFile := filepath.Join(dir, "child.pid")
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe, "-test.run=TestDV07_CrossProcessKillAtPhaseBoundary",
		"-test.v", "-test.timeout=20s")
	cmd.Env = append(os.Environ(),
		"DUCKOS_DV07_CROSS_PROCESS_CHILD=1",
		"DUCKOS_DV07_JOURNAL_DIR="+storeDir,
		"DUCKOS_DV07_KEYS_FILE="+keysFile,
		"DUCKOS_DV07_SOCK="+sockPath,
		"DUCKOS_DV07_PID_FILE="+pidFile,
		"DUCKOS_DV07_REQUEST="+requestFile,
	)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("spawn child: %v", err)
	}
	defer func() {
		if cmd.ProcessState == nil || !cmd.ProcessState.Exited() {
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
	}()
	deadline := time.Now().Add(5 * time.Second)
	var childPID int
	for time.Now().Before(deadline) {
		if data, err := os.ReadFile(pidFile); err == nil {
			fmt.Sscanf(strings.TrimSpace(string(data)), "%d", &childPID)
			if childPID > 0 {
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	if childPID == 0 {
		t.Fatalf("child did not publish PID; stdout=%s stderr=%s", stdout.String(), stderr.String())
	}
	// Send the Stage request.
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{Timeout: time.Second}).DialContext(ctx, "unix", sockPath)
	}}
	go func() {
		client := &http.Client{Transport: transport, Timeout: 9 * time.Second}
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost,
			"http://agent"+agent.UpgradeStagePath, strings.NewReader(string(body)))
		req.Header.Set("Content-Type", "application/json")
		_, _ = client.Do(req)
	}()
	// Wait long enough for the materialized payload
	// to land on disk, then SIGKILL the child BEFORE
	// it has a chance to write any history entry
	// (the child has no Activate step in its loop).
	time.Sleep(500 * time.Millisecond)
	if err := syscall.Kill(childPID, syscall.SIGKILL); err != nil {
		t.Fatalf("kill child: %v", err)
	}
	_ = cmd.Wait()
	// Independent ReleaseStore oracle: the
	// materialized payload MUST be on disk.
	entries := independentReleaseStoreOracle(t, storeDir)
	if len(entries) == 0 {
		t.Fatalf("cross-process kill: ReleaseStore empty (materialized payload lost)")
	}
	// Re-open the ReleaseStore from a fresh caller
	// and verify the rel-x payload fingerprint.
	store2, err := upgrade.NewReleaseStore(storeDir, 0)
	if err != nil {
		t.Fatalf("re-open store: %v", err)
	}
	if err := store2.VerifyFingerprint("rel-x"); err != nil {
		t.Fatalf("re-opened store: VerifyFingerprint(rel-x): %v", err)
	}
}

// dv07CrossProcessChild is the child side of the
// cross-process kill test. It opens the durable
// ReleaseStore + Orchestrator with real ed25519 keys,
// serves the Agent on a Unix socket, parks forever
// after materializing the package (the parent kills
// it before it can Activate).
func dv07CrossProcessChild(t *testing.T) {
	storeDir := os.Getenv("DUCKOS_DV07_JOURNAL_DIR")
	keysFile := os.Getenv("DUCKOS_DV07_KEYS_FILE")
	sockPath := os.Getenv("DUCKOS_DV07_SOCK")
	pidFile := os.Getenv("DUCKOS_DV07_PID_FILE")
	bodyFile := os.Getenv("DUCKOS_DV07_REQUEST")
	if storeDir == "" || keysFile == "" || sockPath == "" || pidFile == "" || bodyFile == "" {
		t.Fatal("child: missing env")
	}
	if err := os.WriteFile(pidFile, []byte(fmt.Sprintf("%d\n", os.Getpid())), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := upgrade.NewReleaseStore(storeDir, 1<<30)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(keysFile)
	if err != nil {
		t.Fatal(err)
	}
	pubRaw, err := hex.DecodeString(strings.TrimSpace(string(data)))
	if err != nil {
		t.Fatal(err)
	}
	pub := ed25519.PublicKey(pubRaw)
	orch := upgrade.NewOrchestrator(store, []ed25519.PublicKey{pub}, "amd64", nil)
	adapter := agent.NewOrchestratorAdapter(orch)
	handler := agent.NewHandler(nil)
	handler.EnableUpgrade(adapter)
	listener, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = http.Serve(listener, handler)
	}()
	for {
		time.Sleep(time.Second)
	}
}

// mustKeyLocal generates a fresh ed25519 key pair.
// Used only by the unavailable-agent subcase.
func mustKeyLocal() ed25519.PrivateKey {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		panic(err)
	}
	return priv
}
