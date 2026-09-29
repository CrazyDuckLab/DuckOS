package upgrade

import (
	"crypto/ed25519"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func durableFixture(t *testing.T) (*Orchestrator, *ReleaseStore, []ed25519.PublicKey) {
	t.Helper()
	s, err := NewReleaseStore(t.TempDir(), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	priv, pub := mustKey(t)
	keys := []ed25519.PublicKey{pub}
	o, err := OpenDurableOrchestrator(s, keys, "amd64")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = o.Close() })
	raw := buildSignedPackage(t, priv, "candidate", "v1", "amd64")
	if err := o.Stage(Plan{Channel: ChannelStable, Release: Release{ID: "candidate", Version: "v1", Channel: ChannelStable, Arch: "amd64"}, Payload: raw, Operator: "owner"}); err != nil {
		t.Fatal(err)
	}
	return o, s, keys
}

func TestH08DurableCandidateAndPolicy(t *testing.T) {
	o, s, keys := durableFixture(t)
	if err := o.SetPolicy(PolicyDownload, "owner"); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenDurableOrchestrator(s, keys, "amd64"); err == nil {
		t.Fatal("second writer admitted")
	}
	if err := o.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenDurableOrchestrator(s, keys, "amd64")
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	st := reopened.State()
	if st.Candidate != "candidate" || st.Policy != PolicyDownload || st.RecoveryRequired {
		t.Fatalf("lost durable state: %+v", st)
	}
	if _, err := reopened.Activate(); err == nil {
		t.Fatal("plain durable activation bypassed health")
	}
}

func TestH08DurableFailedFirstActivation(t *testing.T) {
	o, s, keys := durableFixture(t)
	_, err := o.ActivateAndVerify(verificationRunner(func(HealthCheck) error { return errors.New("unhealthy") }), []HealthCheck{{Name: "runtime", Command: "status"}}, time.Second)
	if err == nil {
		t.Fatal("failed health accepted")
	}
	if err := o.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenDurableOrchestrator(s, keys, "amd64")
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	st := reopened.State()
	if st.Active != "" || st.Candidate != "candidate" || st.RecoveryRequired {
		t.Fatalf("failed first activation resurrected: %+v", st)
	}
}

func TestH08CorruptWorkflowFailsClosed(t *testing.T) {
	o, s, keys := durableFixture(t)
	_ = o.Close()
	if err := os.WriteFile(filepath.Join(s.dir, "workflow.json"), []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenDurableOrchestrator(s, keys, "amd64"); !errors.Is(err, ErrRecoveryRequired) {
		t.Fatalf("corrupt journal admitted: %v", err)
	}
	got, _ := os.ReadFile(filepath.Join(s.dir, "workflow.json"))
	if string(got) != "{" {
		t.Fatal("corrupt evidence overwritten")
	}
}

func TestH08UnjournaledSelectionFailsClosed(t *testing.T) {
	o, s, keys := durableFixture(t)
	_ = o.Close()
	if err := s.Activate("candidate"); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenDurableOrchestrator(s, keys, "amd64"); !errors.Is(err, ErrRecoveryRequired) {
		t.Fatalf("unjournaled selection accepted: %v", err)
	}
	if err := o.SetPolicy(PolicyInstall, "owner"); !errors.Is(err, ErrRecoveryRequired) {
		t.Fatalf("closed writer admitted: %v", err)
	}
}

func TestH08WorkflowCrashHelper(t *testing.T) {
	dir := os.Getenv("DUCKOS_H08_WORKFLOW_CRASH")
	if dir == "" {
		t.Skip("subprocess helper")
	}
	s, err := NewReleaseStore(dir, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	o, err := OpenDurableOrchestrator(s, nil, "amd64")
	if err != nil {
		t.Fatal(err)
	}
	defer o.Close()
	_, err = o.ActivateAndVerify(verificationRunner(func(HealthCheck) error {
		if err := os.WriteFile(filepath.Join(dir, "health-entered"), []byte("ready"), 0600); err != nil {
			return err
		}
		select {}
	}), []HealthCheck{{Name: "runtime", Command: "status"}}, time.Minute)
	t.Fatalf("helper survived activation: %v", err)
}

func TestH08InterruptedWorkflowRequiresRuntimeReconciliation(t *testing.T) {
	o, s, keys := durableFixture(t)
	_ = o.Close()
	cmd := exec.Command(os.Args[0], "-test.run=^TestH08WorkflowCrashHelper$", "-test.timeout=30s")
	cmd.Env = append(os.Environ(), "DUCKOS_H08_WORKFLOW_CRASH="+s.dir)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	waited := false
	defer func() {
		if !waited {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	}()
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := os.Stat(filepath.Join(s.dir, "health-entered")); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("child never reached health phase")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if target, err := os.Readlink(filepath.Join(s.dir, "active")); err != nil || target != "candidate" {
		t.Fatalf("activation effect absent: %s %v", target, err)
	}
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()
	waited = true
	reopened, err := OpenDurableOrchestrator(s, keys, "amd64")
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if !reopened.State().RecoveryRequired {
		t.Fatal("interrupted activation silently accepted")
	}
	if err := reopened.SetPolicy(PolicyInstall, "owner"); !errors.Is(err, ErrRecoveryRequired) {
		t.Fatalf("mutation admitted before recovery: %v", err)
	}
}
