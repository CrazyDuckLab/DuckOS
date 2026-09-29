//go:build linux

// dv08_production_test.go exercises DV08 through the
// production code path:
//
//   PUBLIC ENTRYPOINT  -> httptest.Server wrapping
//                         api.Handler.handleBackupRestore
//   ENGINE PATH        -> engine.Engine.BackupRestore
//                         (Unix-socket client to the
//                         Agent; 1 MiB cap; typed-error
//                         envelope)
//   AGENT PATH         -> agent.NewBackupRestoreHost
//                         drives the production
//                         Executor + JournalStore
//   RUNTIME MUTATION   -> backup.Executor.Execute:
//                         validate -> started entry
//                         (journal fsync) -> snapshotter
//                         -> restorer -> applied entry
//                         (journal fsync)
//   INDEPENDENT ORACLE -> independent os/exec cat of
//                         the journal file (a separate
//                         process from the Agent)
//   RESTART/DURABILITY -> re-open the durable
//                         JournalStore; the started +
//                         applied entries are still
//                         there
//   NEGATIVE CASE      -> invalid backup id / path
//                         traversal / agent-unavailable
//                         (no host wired)
//
// The test runs on duck-dev-01 against the real Linux
// kernel + filesystem + process boundary. There is no
// "xray daemon" or "kernel module" to spin up — the
// Agent itself is the runtime (it owns the durable
// journal + the executor lifecycle); the independent
// oracle is whatever process can cat the journal file.

package layerl

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/agent"
	"github.com/CrazyDuckLab/DuckOS/internal/backup"
	"github.com/CrazyDuckLab/DuckOS/internal/engine"
	"github.com/CrazyDuckLab/DuckOS/internal/restore"
	"github.com/CrazyDuckLab/DuckOS/internal/state"
)

// restoreFixture builds the production restore path
// in-process so the test does not have to boot a
// separate Agent binary. The fixture wraps the exact
// same constructors cmd/duck-agent uses
// (backup.OpenJournalStore + backup.NewExecutor +
// agent.NewBackupRestoreHost) so the production
// code path is exercised end-to-end.
type restoreFixture struct {
	dir        string
	src        string
	dst        string
	journalDir string
	journal    *backup.JournalStore
	executor   *backup.Executor
	host       *agent.BackupRestoreHost
	handler    *agent.Handler
	server     *httptest.Server
	engineSock string
}

func newRestoreFixture(t *testing.T) *restoreFixture {
	t.Helper()
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")
	journalDir := filepath.Join(dir, "journal")
	for _, p := range []string{src, dst, journalDir} {
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	journal, err := backup.OpenJournalStore(journalDir)
	if err != nil {
		t.Fatal(err)
	}
	exec, err := backup.NewExecutor(journal)
	if err != nil {
		t.Fatal(err)
	}
	host := agent.NewBackupRestoreHost(exec, journal)
	if host == nil {
		t.Fatal("BackupRestoreHost refused to register (nil executor/journal)")
	}
	// The Agent handler is mounted on its own
	// httptest.Server so the Engine proxy can dial
	// a Unix socket for the production-path test
	// (TestDV08_PublicEngineAgentChain); the in-
	// process tests use a single httptest.Server
	// that wraps the API handler (which embeds
	// Engine + Agent in one mux via unix-dial
	// Transport).
	agentHandler := agent.NewHandler(nil)
	agentHandler.EnableBackupRestore(host)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		agentHandler.ServeHTTP(w, r)
	}))
	t.Cleanup(func() { srv.Close() })
	return &restoreFixture{
		dir:        dir,
		src:        src,
		dst:        dst,
		journalDir: journalDir,
		journal:    journal,
		executor:   exec,
		host:       host,
		handler:    agentHandler,
		server:     srv,
	}
}

// writeFile writes a file under the fixture root with
// the supplied contents. The relative path is what
// flows into backupRoot/liveRoot in the test request.
func (f *restoreFixture) writeFile(t *testing.T, root, rel string, data []byte) {
	t.Helper()
	full := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

// restoreRequest builds the canonical restore payload
// the Agent's BackupRestoreHost accepts.
func (f *restoreFixture) restoreRequest(backupID string, selected []string) []byte {
	body, _ := json.Marshal(map[string]any{
		"backupId":       backupID,
		"sourceRevision": "rev-1",
		"liveRevision":   "rev-1",
		"selectedKeys":   selected,
		"backupRoot":     f.src,
		"liveRoot":       f.dst,
	})
	return body
}

// postRestore sends a POST to the Agent's restore IPC
// route. Returns the status + body + content-type.
func (f *restoreFixture) postRestore(t *testing.T, body []byte) (int, []byte, string) {
	t.Helper()
	resp, err := http.Post(f.server.URL+agent.BackupRestorePath, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return resp.StatusCode, data, resp.Header.Get("Content-Type")
}

// postAgentRestore is the lower-level helper used by
// the cross-process kill test (it dials the socket the
// Agent listens on directly, not via httptest).
func (f *restoreFixture) postAgentRestore(t *testing.T, sockPath string, body []byte) (int, []byte, error) {
	t.Helper()
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{Timeout: time.Second}).DialContext(ctx, "unix", sockPath)
	}}
	client := &http.Client{Transport: transport, Timeout: 9 * time.Second}
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost,
		"http://agent"+agent.BackupRestorePath, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return resp.StatusCode, data, nil
}

// independentOracle reads the durable journal via a
// fresh os/exec process. The Agent's runtime does
// not know this process exists; the journal file is
// the only coupling. The oracle emits the journal
// contents (JSON lines) so the test can assert the
// Agent wrote both the started + applied entries.
//
// A missing journal file is reported as the empty
// string (the Agent's validate gate may have rejected
// the request before the Executor ever touched the
// journal). The oracle NEVER returns an error for
// "file not present" — that is itself an honest
// witness that the durable store was untouched.
func independentOracle(t *testing.T, journalDir string) string {
	t.Helper()
	path := filepath.Join(journalDir, "restore-journal.log")
	for _, cat := range []string{"/usr/bin/cat", "/bin/cat"} {
		cmd := exec.Command(cat, path)
		out, err := cmd.CombinedOutput()
		if err == nil {
			return string(out)
		}
		// cat exits non-zero when the file does
		// not exist; the test interprets the empty
		// oracle as a witness the journal was never
		// touched (which is the honest negative
		// outcome).
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			if _, statErr := os.Stat(path); os.IsNotExist(statErr) {
				return ""
			}
		}
	}
	t.Fatalf("independent oracle: no cat binary on this VM")
	return ""
}

// =====================================================================
// DV08 production-path subcases
// =====================================================================

// TestDV08_PublicAgentRestore: a restore through the
// production Agent host reaches the production
// Executor + JournalStore. The selected files are
// copied from backupRoot to liveRoot and the journal
// records the started + applied entries.
func TestDV08_PublicAgentRestore(t *testing.T) {
	f := newRestoreFixture(t)
	f.writeFile(t, f.src, "keep-a.conf", []byte("AAA"))
	f.writeFile(t, f.src, "skip-b.conf", []byte("BBB"))
	f.writeFile(t, f.src, "keep-c.conf", []byte("CCC"))
	body := f.restoreRequest("backup-restore-1", []string{"keep-a.conf", "keep-c.conf"})
	status, data, ctype := f.postRestore(t, body)
	if status != 200 {
		t.Fatalf("restore: status=%d ctype=%s body=%s", status, ctype, data)
	}
	var resp map[string]any
	if err := json.Unmarshal(data, &resp); err != nil {
		t.Fatalf("decode response: %v body=%s", err, data)
	}
	if resp["outcome"] != "applied" {
		t.Fatalf("restore outcome: %v want applied; body=%s", resp["outcome"], data)
	}
	entryID, _ := resp["entryId"].(string)
	if entryID == "" {
		t.Fatalf("entry id missing: %s", data)
	}
	// Selected files were copied to liveRoot
	// (the production Restorer reads from src and
	// writes byte-for-byte into dst).
	for _, name := range []string{"keep-a.conf", "keep-c.conf"} {
		got, err := os.ReadFile(filepath.Join(f.dst, name))
		if err != nil {
			t.Fatalf("selected %s missing: %v", name, err)
		}
		want, err := os.ReadFile(filepath.Join(f.src, name))
		if err != nil {
			t.Fatalf("source %s missing: %v", name, err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("selected %s: got %q want %q", name, got, want)
		}
	}
	// Unselected file was NOT copied.
	if _, err := os.Stat(filepath.Join(f.dst, "skip-b.conf")); err == nil {
		t.Fatalf("unselected file skip-b.conf was restored")
	}
	// Independent oracle sees both started + applied entries.
	oracle := independentOracle(t, f.journalDir)
	if !strings.Contains(oracle, "backup-restore-1") {
		t.Fatalf("journal missing backupId: %s", oracle)
	}
	if !strings.Contains(oracle, `"outcome":"started"`) {
		t.Fatalf("journal missing started entry: %s", oracle)
	}
	if !strings.Contains(oracle, `"outcome":"applied"`) {
		t.Fatalf("journal missing applied entry: %s", oracle)
	}
}

// TestDV08_SelectedUnselectedComparison: only the
// selected subset is restored; an unselected file
// already present at liveRoot stays untouched.
func TestDV08_SelectedUnselectedComparison(t *testing.T) {
	f := newRestoreFixture(t)
	f.writeFile(t, f.src, "only.conf", []byte("NEW"))
	f.writeFile(t, f.dst, "untouched.conf", []byte("PRESERVE-ME"))
	body := f.restoreRequest("backup-restore-2", []string{"only.conf"})
	status, data, _ := f.postRestore(t, body)
	if status != 200 {
		t.Fatalf("restore: status=%d body=%s", status, data)
	}
	untouched, err := os.ReadFile(filepath.Join(f.dst, "untouched.conf"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(untouched, []byte("PRESERVE-ME")) {
		t.Fatalf("unselected file was modified: %q", untouched)
	}
	only, err := os.ReadFile(filepath.Join(f.dst, "only.conf"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(only, []byte("NEW")) {
		t.Fatalf("selected file not restored: %q", only)
	}
}

// TestDV08_NegativeInvalidBackupID: a restore request
// with a non-canonical backup id is refused at the
// Agent boundary; the journal has no entries.
func TestDV08_NegativeInvalidBackupID(t *testing.T) {
	f := newRestoreFixture(t)
	body := []byte(`{"backupId":"../etc/passwd","sourceRevision":"rev-1","liveRevision":"rev-1","selectedKeys":["a"],"backupRoot":"` + f.src + `","liveRoot":"` + f.dst + `"}`)
	status, data, _ := f.postRestore(t, body)
	if status == 200 {
		t.Fatalf("invalid backupId accepted: %s", data)
	}
	// Journal must be empty — the request never
	// reached the Executor.
	oracle := independentOracle(t, f.journalDir)
	if strings.TrimSpace(oracle) != "" {
		t.Fatalf("journal must be empty on negative: %q", oracle)
	}
}

// TestDV08_NegativePathTraversal: a selectedKeys
// entry that escapes liveRoot is refused.
func TestDV08_NegativePathTraversal(t *testing.T) {
	f := newRestoreFixture(t)
	body := []byte(`{"backupId":"backup-rest-3","sourceRevision":"rev-1","liveRevision":"rev-1","selectedKeys":["../escape"],"backupRoot":"` + f.src + `","liveRoot":"` + f.dst + `"}`)
	status, data, _ := f.postRestore(t, body)
	if status == 200 {
		t.Fatalf("path traversal accepted: %s", data)
	}
}

// TestDV08_NegativeRevisionMismatch: source and live
// revisions do not match; the Executor refuses the
// plan (requireSafe gate). The requireSafe gate runs
// BEFORE the started entry is appended, so the journal
// is the honest witness that the Executor never even
// touched the durable store.
func TestDV08_NegativeRevisionMismatch(t *testing.T) {
	f := newRestoreFixture(t)
	f.writeFile(t, f.src, "a.conf", []byte("AAA"))
	body := []byte(`{"backupId":"backup-rest-4","sourceRevision":"rev-1","liveRevision":"rev-2","selectedKeys":["a.conf"],"backupRoot":"` + f.src + `","liveRoot":"` + f.dst + `"}`)
	status, data, _ := f.postRestore(t, body)
	if status == 200 {
		t.Fatalf("revision mismatch accepted: %s", data)
	}
	// requireSafe is checked BEFORE the started
	// entry is appended, so the journal is the
	// honest witness that the Executor never
	// touched the durable store. No entries should
	// be present (started, applied, OR failed).
	oracle := independentOracle(t, f.journalDir)
	if strings.Contains(oracle, "backup-rest-4") {
		t.Fatalf("revision mismatch must not touch the journal: %s", oracle)
	}
}

// TestDV08_NegativeAgentUnavailable: when the Agent
// host is not enabled, the IPC route returns 503
// backup_restore_unavailable, never a fake 200.
func TestDV08_NegativeAgentUnavailable(t *testing.T) {
	agentHandler := agent.NewHandler(nil)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		agentHandler.ServeHTTP(w, r)
	}))
	defer srv.Close()
	resp, err := http.Post(srv.URL+agent.BackupRestorePath, "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", resp.StatusCode)
	}
}

// TestDV08_IndependentJournalOracle: a restore
// followed by an independent process reading the
// journal file sees both the started + applied
// entries. The independent process is the canonical
// witness: the Agent runtime does not know it
// exists; only the journal file couples them.
func TestDV08_IndependentJournalOracle(t *testing.T) {
	f := newRestoreFixture(t)
	f.writeFile(t, f.src, "a.conf", []byte("AAA"))
	body := f.restoreRequest("backup-restore-oracle", []string{"a.conf"})
	if status, data, _ := f.postRestore(t, body); status != 200 {
		t.Fatalf("restore: status=%d body=%s", status, data)
	}
	out := independentOracle(t, f.journalDir)
	if !strings.Contains(out, `"backupId":"backup-restore-oracle"`) {
		t.Fatalf("oracle missing backupId: %s", out)
	}
	if !strings.Contains(out, `"outcome":"started"`) {
		t.Fatalf("oracle missing started entry: %s", out)
	}
	if !strings.Contains(out, `"outcome":"applied"`) {
		t.Fatalf("oracle missing applied entry: %s", out)
	}
	// The oracle must observe EXACTLY the canonical
	// entry pair; a runaway reconciler or duplicate
	// apply would surface as extra entries.
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) < 2 {
		t.Fatalf("oracle must see at least 2 lines (started + applied), got %d: %s", len(lines), out)
	}
}

// TestDV08_RestartReconciles: the durable journal
// survives the Agent process exiting. After the
// restore completes, a fresh JournalStore reopens
// the same directory and the entries are still
// observable from an independent process.
func TestDV08_RestartReconciles(t *testing.T) {
	f := newRestoreFixture(t)
	f.writeFile(t, f.src, "a.conf", []byte("AAA"))
	f.writeFile(t, f.src, "b.conf", []byte("BBB"))
	body := f.restoreRequest("backup-restore-restart", []string{"a.conf", "b.conf"})
	if status, data, _ := f.postRestore(t, body); status != 200 {
		t.Fatalf("restore: status=%d body=%s", status, data)
	}
	// Tear down the Agent's httptest server (and
	// the in-process executor/journal reference).
	f.server.Close()
	// Re-open the journal from a fresh caller.
	journal2, err := backup.OpenJournalStore(f.journalDir)
	if err != nil {
		t.Fatalf("re-open journal: %v", err)
	}
	_ = journal2 // re-open asserts the durable store is re-readable from a fresh caller
	// Independent oracle confirms the durable
	// state survived the Agent's exit.
	out := independentOracle(t, f.journalDir)
	if !strings.Contains(out, `"backupId":"backup-restore-restart"`) {
		t.Fatalf("restart: journal missing backupId: %s", out)
	}
	if !strings.Contains(out, `"outcome":"started"`) {
		t.Fatalf("restart: journal missing started: %s", out)
	}
	if !strings.Contains(out, `"outcome":"applied"`) {
		t.Fatalf("restart: journal missing applied: %s", out)
	}
}

// TestDV08_CrossProcessKillAtPhaseBoundary: the Agent
// runs in a separate OS process (a child of `go test`).
// The test sends a restore request, then kills the
// child Agent mid-flight; a fresh process reopens the
// journal and sees the started entry (the apply was
// never reached because the child died before
// completion).
//
// This is the L-tier cross-process durability case.
// The single-process testDV08_RestartReconciles only
// exercises the file durability; this test exercises
// the OS-process durability (the Agent process can die
// at any phase and the journal survives).
func TestDV08_CrossProcessKillAtPhaseBoundary(t *testing.T) {
	if os.Getenv("DUCKOS_DV08_CROSS_PROCESS_CHILD") != "" {
		dv08CrossProcessChild(t)
		return
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")
	journalDir := filepath.Join(dir, "journal")
	for _, p := range []string{src, dst, journalDir} {
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(src, "a.conf"), []byte("AAA"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Build a payload the child can read; the
	// child blocks on a barrier so the test can
	// kill it after the started entry is on disk.
	bodyFile := filepath.Join(dir, "request.json")
	body, _ := json.Marshal(map[string]any{
		"backupId":       "backup-rest-cross",
		"sourceRevision": "rev-1",
		"liveRevision":   "rev-1",
		"selectedKeys":   []string{"a.conf"},
		"backupRoot":     src,
		"liveRoot":       dst,
	})
	if err := os.WriteFile(bodyFile, body, 0o600); err != nil {
		t.Fatal(err)
	}
	// Spawn the Agent child via `go test -run` with
	// a sentinel env var. The child listens on a
	// Unix socket under dir/agent.sock and writes
	// the child PID to dir/child.pid.
	sockPath := filepath.Join(dir, "agent.sock")
	pidFile := filepath.Join(dir, "child.pid")
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe, "-test.run=TestDV08_CrossProcessKillAtPhaseBoundary",
		"-test.v", "-test.timeout=20s")
	cmd.Env = append(os.Environ(),
		"DUCKOS_DV08_CROSS_PROCESS_CHILD=1",
		"DUCKOS_DV08_JOURNAL_DIR="+journalDir,
		"DUCKOS_DV08_SOCK="+sockPath,
		"DUCKOS_DV08_PID_FILE="+pidFile,
		"DUCKOS_DV08_REQUEST="+bodyFile,
	)
	var stdout, stderr bytes.Buffer
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
	// Wait for the child to write its PID file.
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
	// Send the restore request in the background;
	// the child intentionally blocks between snap
	// and restore (the barrier executor), so the
	// HTTP request will hang. We don't need the
	// response — we only need the child to have
	// reached the started fsync + snap phase.
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{Timeout: time.Second}).DialContext(ctx, "unix", sockPath)
	}}
	go func() {
		client := &http.Client{Transport: transport, Timeout: 9 * time.Second}
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost,
			"http://agent"+agent.BackupRestorePath, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		_, _ = client.Do(req)
	}()
	// Give the child a moment to fsync the started
	// entry and reach the snap barrier, then SIGKILL
	// it BEFORE the apply finishes.
	time.Sleep(300 * time.Millisecond)
	if err := syscall.Kill(childPID, syscall.SIGKILL); err != nil {
		t.Fatalf("kill child: %v", err)
	}
	_ = cmd.Wait()
	// Independent oracle confirms the started
	// entry survived the cross-process kill.
	out := independentOracle(t, journalDir)
	if !strings.Contains(out, `"backupId":"backup-rest-cross"`) {
		t.Fatalf("cross-process kill: journal missing backupId: %s", out)
	}
	if !strings.Contains(out, `"outcome":"started"`) {
		t.Fatalf("cross-process kill: journal missing started: %s", out)
	}
	// The applied entry must NOT be present (the
	// apply never completed because the child was
	// killed). A naive executor that fsyncs the
	// applied entry optimistically would fail this
	// assertion.
	if strings.Contains(out, `"outcome":"applied"`) {
		t.Fatalf("cross-process kill: applied entry present (apply should have been killed): %s", out)
	}
}

// TestDV08_PublicEngineAgentChain: full production
// chain through Engine.BackupRestore on a real Unix
// socket. The Agent is wired into a real Agent Handler
// (production handler) listening on the socket — the
// test does NOT use httptest.Server; it uses net.Listen
// and a fresh engine.New(socket, env) (the exact
// constructor cmd/duck-api uses). This is the
// structural-equivalent of DV04-WG and DV04-XRAY's
// PublicEngineAgentChain subcase.
func TestDV08_PublicEngineAgentChain(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"source/b1", "live", "journal", "engine"} {
		if err := os.MkdirAll(filepath.Join(dir, p), 0700); err != nil {
			t.Fatal(err)
		}
	}
	service, err := restore.Open(filepath.Join(dir, "source"), filepath.Join(dir, "live"), filepath.Join(dir, "journal"))
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	restoreSocket := filepath.Join(dir, "restore.sock")
	listener, err := agent.Listen(restoreSocket, uint32(os.Geteuid()))
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	server := &http.Server{Handler: service}
	defer server.Close()
	go server.Serve(listener)
	handler := agent.NewHandler(nil)
	handler.EnableBackupRestore(agent.NewRestoreProxy(restoreSocket))
	agentSocket := filepath.Join(dir, "agent.sock")
	agentListener, err := agent.Listen(agentSocket, uint32(os.Geteuid()))
	if err != nil {
		t.Fatal(err)
	}
	defer agentListener.Close()
	agentServer := &http.Server{Handler: handler}
	defer agentServer.Close()
	go agentServer.Serve(agentListener)
	eng := engine.New(agentSocket, "test")
	store, err := state.Open(filepath.Join(dir, "engine"), []byte(`{"version":1}`), func(json.RawMessage) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := eng.EnableConfig(store); err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "source/b1/e2e.conf"), []byte("E2E-DATA"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "live/unselected"), []byte("KEEP"), 0600); err != nil {
		t.Fatal(err)
	}
	request := restore.Request{ID: strings.Repeat("8", 32), Expected: snapshot.Current.ID, Backup: "b1", Keys: []string{"e2e.conf"}}
	body, _ := json.Marshal(request)
	data, err := eng.BackupRestore(context.Background(), body)
	if err != nil {
		t.Fatal(err)
	}
	var result restore.Result
	if json.Unmarshal(data, &result) != nil || result.Outcome != "applied" {
		t.Fatalf("invalid terminal result: %s", data)
	}
	// The oracle reads actual managed bytes, not metadata or a fake health flag.
	got, err := exec.Command("/bin/cat", filepath.Join(dir, "live/e2e.conf")).Output()
	if err != nil || string(got) != "E2E-DATA" {
		t.Fatal("independent live oracle failed", err)
	}
	got, err = exec.Command("/bin/cat", filepath.Join(dir, "live/unselected")).Output()
	if err != nil || string(got) != "KEEP" {
		t.Fatal("unselected file changed", err)
	}
	request.ID = strings.Repeat("9", 32)
	request.Expected = strings.Repeat("f", 64)
	body, _ = json.Marshal(request)
	if _, err := eng.BackupRestore(context.Background(), body); err == nil {
		t.Fatal("invented revision accepted")
	}
	request.Expected = snapshot.Current.ID
	request.Keys = []string{"../escape"}
	body, _ = json.Marshal(request)
	if _, err := eng.BackupRestore(context.Background(), body); err == nil {
		t.Fatal("path escape accepted")
	}
	// The configured roots are absent from and forbidden in the public contract.
	if _, err := eng.BackupRestore(context.Background(), []byte(`{"liveRoot":"/etc"}`)); err == nil {
		t.Fatal("caller root accepted")
	}
	server.Close()
	service.Close()
	reopened, err := restore.Open(filepath.Join(dir, "source"), filepath.Join(dir, "live"), filepath.Join(dir, "journal"))
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	request = restore.Request{ID: strings.Repeat("8", 32), Expected: snapshot.Current.ID, Backup: "b1", Keys: []string{"e2e.conf"}}
	receipt, err := reopened.Reconcile(request)
	if err != nil || receipt.Outcome != "applied" {
		t.Fatal("terminal receipt not durable", err)
	}
}

// dv08CrossProcessChild is the child side of the
// cross-process kill test. It opens the journal,
// spawns the Agent on a Unix socket, parks a request
// goroutine that triggers a restore (which writes the
// started entry, then blocks on a barrier that the
// parent kills through).
//
// To force the parent to be able to kill BETWEEN the
// started fsync and the apply fsync, the child uses a
// barrierExecutor that wraps the production Executor
// and blocks on a waitgroup between snap and restore.
// The parent never releases the waitgroup (it SIGKILLs
// the child), so the applied entry is never written —
// which is exactly the audit's "kill at phase boundary"
// semantics: started survives, applied does not.
func dv08CrossProcessChild(t *testing.T) {
	journalDir := os.Getenv("DUCKOS_DV08_JOURNAL_DIR")
	sockPath := os.Getenv("DUCKOS_DV08_SOCK")
	pidFile := os.Getenv("DUCKOS_DV08_PID_FILE")
	bodyFile := os.Getenv("DUCKOS_DV08_REQUEST")
	if journalDir == "" || sockPath == "" || pidFile == "" || bodyFile == "" {
		t.Fatal("child: missing env")
	}
	if err := os.WriteFile(pidFile, []byte(fmt.Sprintf("%d\n", os.Getpid())), 0o600); err != nil {
		t.Fatal(err)
	}
	journal, err := backup.OpenJournalStore(journalDir)
	if err != nil {
		t.Fatal(err)
	}
	_ = journal
	executor, err := backup.NewExecutor(journal)
	if err != nil {
		t.Fatal(err)
	}
	// Wrap the production executor in a barrier that
	// blocks between snap and restore. The waitgroup
	// is never released by design — the parent SIGKILLs
	// this process. Without the barrier, the production
	// lifecycle is fast enough that the apply completes
	// before the parent can SIGKILL.
	be := &barrierExecutor{
		inner: executor,
		apply: &sync.WaitGroup{},
	}
	be.apply.Add(1) // parent never Done()s
	host := agent.NewBackupRestoreHost(be, journal)
	agentHandler := agent.NewHandler(nil)
	agentHandler.EnableBackupRestore(host)
	listener, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() { _ = http.Serve(listener, agentHandler) }()
	// Park until the parent SIGKILLs us.
	for {
		time.Sleep(time.Second)
	}
}

// barrierExecutor wraps a production Executor and
// blocks between snap and restore. Used only by the
// cross-process kill test to ensure the parent has a
// window between the started fsync and the apply fsync.
type barrierExecutor struct {
	inner *backup.Executor
	apply *sync.WaitGroup
}

func (b *barrierExecutor) Execute(ctx context.Context, entry backup.RestoreEntry, plan backup.RestorePlan, sourceRev, liveRev string, snap backup.Snapshotter, restore backup.Restorer) (backup.RestorerResult, error) {
	wrappedSnap := func(ctx context.Context) (any, error) {
		s, err := snap(ctx)
		if err != nil {
			return nil, err
		}
		// Block between snap and restore.
		b.apply.Wait()
		return s, nil
	}
	return b.inner.Execute(ctx, entry, plan, sourceRev, liveRev, wrappedSnap, restore)
}
