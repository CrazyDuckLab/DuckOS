//go:build linux

package upgrade

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
	"time"
)

// processRuntime exercises actual native executable startup and an independent
// live socket oracle. This fixture is not systemd service-level acceptance.
type processRuntime struct {
	root, socket string
	cmd          *exec.Cmd
}

func (p *processRuntime) Stop(context.Context) error {
	if p.cmd != nil {
		_ = p.cmd.Process.Kill()
		_ = p.cmd.Wait()
		p.cmd = nil
	}
	_ = os.Remove(p.socket)
	return nil
}
func (p *processRuntime) Restart(ctx context.Context) error {
	_ = p.Stop(ctx)
	p.cmd = exec.Command(filepath.Join(p.root, "current", "bin", "duck-api"), "--socket", p.socket)
	if err := p.cmd.Start(); err != nil {
		p.cmd = nil
		return err
	}
	return nil
}
func (p *processRuntime) Health(ctx context.Context, b BuildIdentity) error {
	ctx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", p.socket)
	}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport}
	for {
		req, _ := http.NewRequestWithContext(ctx, "GET", "http://runtime/", nil)
		res, err := client.Do(req)
		if err == nil {
			var got BuildIdentity
			decodeErr := json.NewDecoder(res.Body).Decode(&got)
			res.Body.Close()
			if res.StatusCode == 200 && decodeErr == nil && got.Version == b.Version && got.Commit == b.Commit {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return errors.New("fixture health failed")
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func nativeCandidate(t *testing.T, priv ed25519.PrivateKey, version string, unhealthy bool) Plan {
	t.Helper()
	file := filepath.Join(t.TempDir(), "release")
	flags := "-X main.version=" + version
	if unhealthy {
		flags += " -X main.unhealthy=true"
	}
	cmd := exec.Command("go", "build", "-ldflags", flags, "-o", file, "./testdata/release")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("native fixture build: %v %s", err, out)
	}
	binary, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	build, _ := json.Marshal(BuildIdentity{Version: version, Commit: "1111111111111111111111111111111111111111", Target: "linux/" + runtime.GOARCH})
	var payload bytes.Buffer
	archive := tar.NewWriter(&payload)
	for _, entry := range []struct {
		name string
		data []byte
	}{{"bin/duck-agent", binary}, {"bin/duck-api", binary}, {"BUILD-INFO.json", build}} {
		if err := archive.WriteHeader(&tar.Header{Name: entry.name, Mode: 0755, Size: int64(len(entry.data))}); err != nil {
			t.Fatal(err)
		}
		if _, err := archive.Write(entry.data); err != nil {
			t.Fatal(err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	return signedArchive(t, priv, version, payload.Bytes())
}
func signedArchive(t *testing.T, priv ed25519.PrivateKey, version string, payload []byte) Plan {
	t.Helper()
	digest := sha256.Sum256(payload)
	m := Manifest{Format: FormatVer, Architecture: runtime.GOARCH, SourceVersion: version, TargetVersion: version, InputSchema: 1, OutputSchema: 1, PayloadAlgorithm: "sha256", PayloadDigest: hex.EncodeToString(digest[:]), PayloadBytes: int64(len(payload))}
	manifest, _ := json.Marshal(m)
	body := append(ed25519.Sign(priv, manifest), manifest...)
	raw := append((Header{Magic: Magic, Format: FormatVer, BodyBytes: int64(len(body))}).Marshal(), body...)
	raw = append(raw, payload...)
	return Plan{Channel: ChannelStable, Release: Release{ID: version, Version: version, Arch: runtime.GOARCH, Channel: ChannelStable}, Payload: raw, Operator: "owner"}
}

func TestH08SupervisorNativeExecutionRollbackAndReopen(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("native Linux candidate")
	}
	priv, pub := mustKey(t)
	keys := []ed25519.PublicKey{pub}
	dir := t.TempDir()
	store, err := NewReleaseStore(filepath.Join(dir, "store"), 64<<20)
	if err != nil {
		t.Fatal(err)
	}
	r := &processRuntime{root: filepath.Join(dir, "installed"), socket: filepath.Join(dir, "runtime.sock")}
	defer r.Stop(context.Background())
	s, err := OpenSupervisor(store, keys, runtime.GOARCH, r.root, r)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if s != nil {
			_ = s.Close()
		}
	}()
	bad := nativeCandidate(t, priv, "vbad", true)
	if err := s.Stage(bad); err != nil {
		t.Fatal(err)
	}
	if err := s.Activate(context.Background()); err == nil {
		t.Fatal("unhealthy first candidate accepted")
	}
	if s.State().Active != "" {
		t.Fatal("false first Active")
	}
	if _, err := os.Lstat(filepath.Join(r.root, "current")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("failed first selection remains")
	}
	good := nativeCandidate(t, priv, "v1", false)
	if err := s.Stage(good); err != nil {
		t.Fatal(err)
	}
	if err := s.Activate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if s.State().Active != "v1" {
		t.Fatal("valid native release not active")
	}
	bad2 := nativeCandidate(t, priv, "vbad2", true)
	if err := s.Stage(bad2); err != nil {
		t.Fatal(err)
	}
	if err := s.Activate(context.Background()); err == nil {
		t.Fatal("failed health accepted")
	}
	if s.State().Active != "v1" || s.State().RecoveryRequired {
		t.Fatal("previous runtime not recovered")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	_ = r.Stop(context.Background())
	s, err = OpenSupervisor(store, keys, runtime.GOARCH, r.root, r)
	if err != nil {
		t.Fatal(err)
	}
	if s.State().Active != "v1" || s.State().RecoveryRequired {
		t.Fatal("startup reconciliation failed")
	}
}

func TestH08CandidateRejectsEscapeAndInvalidExecutable(t *testing.T) {
	priv, pub := mustKey(t)
	dir := t.TempDir()
	store, err := NewReleaseStore(filepath.Join(dir, "store"), 64<<20)
	if err != nil {
		t.Fatal(err)
	}
	r := &processRuntime{root: filepath.Join(dir, "installed"), socket: filepath.Join(dir, "runtime.sock")}
	s, err := OpenSupervisor(store, []ed25519.PublicKey{pub}, runtime.GOARCH, r.root, r)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, name := range []string{"../escaped", "/escaped", "bin/duck-agent"} {
		var payload bytes.Buffer
		archive := tar.NewWriter(&payload)
		_ = archive.WriteHeader(&tar.Header{Name: name, Mode: 0755, Size: 3})
		_, _ = archive.Write([]byte("bad"))
		_ = archive.Close()
		if err := s.Stage(signedArchive(t, priv, "bad", payload.Bytes())); err == nil {
			t.Fatalf("accepted %q", name)
		}
		if s.State().Active != "" || s.State().Candidate != "" {
			t.Fatal("rejected input changed workflow")
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "escaped")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("escaped release root")
	}
	plan := nativeCandidate(t, priv, "unsigned", false)
	plan.Payload[len(plan.Payload)-1] ^= 1
	if err := s.Stage(plan); err == nil {
		t.Fatal("tampered signed payload accepted")
	}
}

type crashRuntime struct {
	processRuntime
	marker string
	phase  string
}

func (r *crashRuntime) Restart(ctx context.Context) error {
	if r.phase == "before-start" {
		return r.block()
	}
	if err := r.processRuntime.Restart(ctx); err != nil {
		return err
	}
	if r.phase == "after-health" {
		return nil
	}
	return r.block()
}

func (r *crashRuntime) Health(ctx context.Context, build BuildIdentity) error {
	if err := r.processRuntime.Health(ctx, build); err != nil {
		return err
	}
	return r.block()
}

func (r *crashRuntime) block() error {
	if err := os.WriteFile(r.marker, []byte("started"), 0600); err != nil {
		return err
	}
	select {}
}

func TestH08SupervisorCrashHelper(t *testing.T) {
	dir := os.Getenv("DUCKOS_H08_SUPERVISOR_CRASH")
	if dir == "" {
		t.Skip("subprocess helper")
	}
	key, err := hex.DecodeString(os.Getenv("DUCKOS_H08_PUBLIC_KEY"))
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewReleaseStore(filepath.Join(dir, "store"), 64<<20)
	if err != nil {
		t.Fatal(err)
	}
	r := &processRuntime{root: filepath.Join(dir, "installed"), socket: filepath.Join(dir, "runtime.sock")}
	s, err := OpenSupervisor(store, []ed25519.PublicKey{key}, runtime.GOARCH, r.root, r)
	if err != nil {
		t.Fatal(err)
	}
	// Reconciliation has completed; fault only the subsequent activation.
	s.runtime = &crashRuntime{processRuntime: *r, marker: filepath.Join(dir, "phase-marker"), phase: os.Getenv("DUCKOS_H08_CRASH_PHASE")}
	if err := s.Activate(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Fatal("crash helper returned")
}

func TestH08SupervisorRecoversKilledActivation(t *testing.T) {
	for _, phase := range []string{"before-start", "after-start", "after-health"} {
		t.Run(phase, func(t *testing.T) { testH08KilledActivation(t, phase) })
	}
}

func testH08KilledActivation(t *testing.T, phase string) {
	priv, pub := mustKey(t)
	dir := t.TempDir()
	store, err := NewReleaseStore(filepath.Join(dir, "store"), 64<<20)
	if err != nil {
		t.Fatal(err)
	}
	r := &processRuntime{root: filepath.Join(dir, "installed"), socket: filepath.Join(dir, "runtime.sock")}
	defer r.Stop(context.Background())
	s, err := OpenSupervisor(store, []ed25519.PublicKey{pub}, runtime.GOARCH, r.root, r)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Stage(nativeCandidate(t, priv, "v1", false)); err != nil {
		t.Fatal(err)
	}
	if err := s.Activate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := s.Stage(nativeCandidate(t, priv, "v2", false)); err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	_ = r.Stop(context.Background())
	cmd := exec.Command(os.Args[0], "-test.run=^TestH08SupervisorCrashHelper$", "-test.timeout=30s")
	cmd.Env = append(os.Environ(), "DUCKOS_H08_SUPERVISOR_CRASH="+dir, "DUCKOS_H08_PUBLIC_KEY="+hex.EncodeToString(pub), "DUCKOS_H08_CRASH_PHASE="+phase)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	waited := false
	defer func() {
		if !waited {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			_ = cmd.Wait()
		}
	}()
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := os.Stat(filepath.Join(dir, "phase-marker")); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("activation boundary not reached")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if selected, err := os.Readlink(filepath.Join(r.root, "current")); err != nil || selected != "releases/v2" {
		t.Fatalf("candidate not provisionally selected: %s %v", selected, err)
	}
	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()
	waited = true
	s, err = OpenSupervisor(store, []ed25519.PublicKey{pub}, runtime.GOARCH, r.root, r)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if s.State().Active != "v1" || s.State().RecoveryRequired || s.Pending() {
		t.Fatalf("failed crash recovery: %+v", s.State())
	}
	if err := r.Health(context.Background(), BuildIdentity{Version: "v1", Commit: "1111111111111111111111111111111111111111"}); err != nil {
		t.Fatal(err)
	}
}

func TestH08EnrollSignedInstalledBaseline(t *testing.T) {
	priv, pub := mustKey(t)
	keys := []ed25519.PublicKey{pub}
	dir := t.TempDir()
	store, err := NewReleaseStore(filepath.Join(dir, "store"), 64<<20)
	if err != nil {
		t.Fatal(err)
	}
	r := &processRuntime{root: filepath.Join(dir, "installed"), socket: filepath.Join(dir, "runtime.sock")}
	defer r.Stop(context.Background())
	s, err := OpenSupervisor(store, keys, runtime.GOARCH, r.root, r)
	if err != nil {
		t.Fatal(err)
	}
	plan := nativeCandidate(t, priv, "v1", false)
	if err := s.Stage(plan); err != nil {
		t.Fatal(err)
	}
	if err := s.selectRelease("v1"); err != nil {
		t.Fatal(err)
	}
	if err := r.Restart(context.Background()); err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	if err := EnrollInstalledRelease(store, keys, runtime.GOARCH, r.root, r, plan.Payload); err != nil {
		t.Fatal(err)
	}
	s, err = OpenSupervisor(store, keys, runtime.GOARCH, r.root, r)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if st := s.State(); st.Active != "v1" || st.Candidate != "" || st.RecoveryRequired {
		t.Fatalf("baseline not enrolled: %+v", st)
	}
}

type cancellationRuntime struct {
	ReleaseRuntime
	cancel context.CancelFunc
}

func (r cancellationRuntime) Health(ctx context.Context, b BuildIdentity) error {
	err := r.ReleaseRuntime.Health(ctx, b)
	r.cancel()
	return err
}

type rollbackFailureRuntime struct {
	ReleaseRuntime
	starts int
}

func (r *rollbackFailureRuntime) Restart(ctx context.Context) error {
	r.starts++
	if r.starts > 1 {
		return errors.New("rollback fixture failure")
	}
	return r.ReleaseRuntime.Restart(ctx)
}

func TestH08DeadlineRollbackAndRecoveryFailure(t *testing.T) {
	priv, pub := mustKey(t)
	keys := []ed25519.PublicKey{pub}
	dir := t.TempDir()
	store, err := NewReleaseStore(filepath.Join(dir, "store"), 64<<20)
	if err != nil {
		t.Fatal(err)
	}
	r := &processRuntime{root: filepath.Join(dir, "installed"), socket: filepath.Join(dir, "runtime.sock")}
	defer r.Stop(context.Background())
	s, err := OpenSupervisor(store, keys, runtime.GOARCH, r.root, r)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if s != nil {
			_ = s.Close()
		}
	}()
	if err := s.Stage(nativeCandidate(t, priv, "v1", false)); err != nil {
		t.Fatal(err)
	}
	if err := s.Activate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := s.Stage(nativeCandidate(t, priv, "v2", false)); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.runtime = cancellationRuntime{ReleaseRuntime: r, cancel: cancel}
	if err := s.Activate(ctx); err == nil {
		t.Fatal("late health overrode canceled activation")
	}
	if s.State().Active != "v1" || s.State().RecoveryRequired {
		t.Fatal("fresh rollback context was not used")
	}
	s.runtime = &rollbackFailureRuntime{ReleaseRuntime: r}
	if err := s.Stage(nativeCandidate(t, priv, "vbad", true)); err != nil {
		t.Fatal(err)
	}
	if err := s.Activate(context.Background()); err == nil {
		t.Fatal("rollback failure accepted")
	}
	if !s.State().RecoveryRequired || !s.Pending() {
		t.Fatal("uncertain recovery not durable")
	}
	_ = s.Close()
	s, err = OpenSupervisor(store, keys, runtime.GOARCH, r.root, r)
	if err != nil {
		t.Fatal(err)
	}
	if s.State().Active != "v1" || s.State().RecoveryRequired || s.Pending() {
		t.Fatal("startup did not complete interrupted rollback")
	}
}

func TestH08JournalFailureBeforeSelectionHasNoLiveEffects(t *testing.T) {
	priv, pub := mustKey(t)
	dir := t.TempDir()
	store, err := NewReleaseStore(filepath.Join(dir, "store"), 64<<20)
	if err != nil {
		t.Fatal(err)
	}
	r := &processRuntime{root: filepath.Join(dir, "installed"), socket: filepath.Join(dir, "runtime.sock")}
	s, err := OpenSupervisor(store, []ed25519.PublicKey{pub}, runtime.GOARCH, r.root, r)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Stage(nativeCandidate(t, priv, "v1", false)); err != nil {
		t.Fatal(err)
	}
	// Replace only the fixture journal pathname with a directory to force the
	// durable rename to fail even when the regression runs as root.
	journal := filepath.Join(store.dir, "workflow.json")
	if err := os.Rename(journal, journal+".saved"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(journal, 0700); err != nil {
		t.Fatal(err)
	}
	if err := s.Activate(context.Background()); err == nil {
		t.Fatal("journal failure accepted")
	}
	if r.cmd != nil {
		t.Fatal("runtime started without durable intent")
	}
	if _, err := os.Lstat(filepath.Join(r.root, "current")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("selection changed without durable intent")
	}
}

func TestH08AcceptedActivationSurvivesRequestCancellation(t *testing.T) {
	priv, pub := mustKey(t)
	dir := t.TempDir()
	store, err := NewReleaseStore(filepath.Join(dir, "store"), 64<<20)
	if err != nil {
		t.Fatal(err)
	}
	r := &processRuntime{root: filepath.Join(dir, "installed"), socket: filepath.Join(dir, "runtime.sock")}
	defer r.Stop(context.Background())
	s, err := OpenSupervisor(store, []ed25519.PublicKey{pub}, runtime.GOARCH, r.root, r)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Stage(nativeCandidate(t, priv, "v1", false)); err != nil {
		t.Fatal(err)
	}
	h := &SupervisorHTTP{Supervisor: s}
	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest("POST", "/v1/activate", nil).WithContext(ctx)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	cancel()
	if w.Code != 200 || !bytes.Contains(w.Body.Bytes(), []byte(`"accepted"`)) {
		t.Fatalf("not accepted: %s", w.Body.String())
	}
	deadline := time.Now().Add(10 * time.Second)
	for !h.operation.TryLock() {
		if time.Now().After(deadline) {
			t.Fatal("accepted operation did not terminate")
		}
		time.Sleep(10 * time.Millisecond)
	}
	h.operation.Unlock()
	if s.State().Active != "v1" || s.Pending() || s.State().RecoveryRequired {
		t.Fatal("request disconnect canceled supervisor-owned operation")
	}
	if err := r.Health(context.Background(), BuildIdentity{Version: "v1", Commit: "1111111111111111111111111111111111111111"}); err != nil {
		t.Fatal(err)
	}
}
