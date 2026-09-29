package upgrade

import (
	"crypto/ed25519"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAuditDecodeShortInput(t *testing.T) {
	_, v := newTestVerifier(t)
	for n := 0; n < HeaderLen; n++ {
		func() {
			defer func() {
				if recover() != nil {
					t.Errorf("panic at length %d", n)
				}
			}()
			if _, err := Decode(make([]byte, n), v); err == nil {
				t.Errorf("short input accepted: %d", n)
			}
		}()
	}
}

type auditCheckRunner struct{ entered, release chan struct{} }

func (r auditCheckRunner) Run(HealthCheck) error {
	close(r.entered)
	<-r.release
	return nil
}

func TestAuditActivationVerificationIsSerialized(t *testing.T) {
	store := newFakeStore(1 << 30)
	if err := store.Materialize(ReleaseMeta{ID: "candidate"}, []byte("payload")); err != nil {
		t.Fatal(err)
	}
	o := NewOrchestrator(store, nil, "amd64", nil)
	o.state.Candidate = "candidate"
	runner := auditCheckRunner{make(chan struct{}), make(chan struct{})}
	done := make(chan error, 1)
	go func() {
		_, err := o.ActivateAndVerify(runner, []HealthCheck{{Name: "agent", Command: "status"}}, 0)
		done <- err
	}()
	select {
	case <-runner.entered:
	case <-time.After(5 * time.Second):
		close(runner.release)
		t.Fatal("verification did not start")
	}
	unlocked := o.opMu.TryLock()
	if unlocked {
		o.opMu.Unlock()
	}
	close(runner.release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if unlocked {
		t.Fatal("another operation can replace Active during verification")
	}
}
func TestAuditReleaseRejectsDotAndAnchorIDs(t *testing.T) {
	s, err := NewReleaseStore(t.TempDir(), 1024)
	if err != nil {
		t.Fatal(err)
	}
	// Never exercise vulnerable recursive deletion on dot paths.
	for _, id := range []string{".", "..", "active", "previous"} {
		if _, err := s.ReadMeta(id); err == nil {
			t.Errorf("ReadMeta accepted %q", id)
		}
		if err := s.Activate(id); err == nil {
			t.Errorf("Activate accepted %q", id)
		}
	}
}
func TestAuditActivationRequiresIntactRelease(t *testing.T) {
	for _, kind := range []string{"incomplete", "tampered", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			s, err := NewReleaseStore(dir, 1024)
			if err != nil {
				t.Fatal(err)
			}
			mustMaterialize(t, s, "good", []byte("good"))
			if err = s.Activate("good"); err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "incomplete":
				err = os.Mkdir(filepath.Join(dir, "bad"), 0700)
			case "tampered":
				mustMaterialize(t, s, "bad", []byte("original"))
				err = os.WriteFile(filepath.Join(dir, "bad", "payload"), []byte("changed"), 0600)
			case "symlink":
				err = os.Symlink(t.TempDir(), filepath.Join(dir, "bad"))
			}
			if err != nil {
				t.Fatal(err)
			}
			if err = s.Activate("bad"); err == nil {
				t.Fatal("invalid release activated")
			}
			got, err := os.Readlink(filepath.Join(dir, "active"))
			if err != nil || got != "good" {
				t.Fatal("active anchor changed on refusal")
			}
		})
	}
}
func TestAuditPreflightBindsSignedArchitecture(t *testing.T) {
	priv, pub := mustKey(t)
	raw := buildSignedPackage(t, priv, "bad", "v1", "arm64")
	plan := Plan{Channel: ChannelStable, Release: Release{ID: "bad", Version: "v1", Channel: ChannelStable, Arch: "amd64"}, Payload: raw, Operator: "test"}
	if r := NewPreflight(newFakeStore(1<<30), []ed25519.PublicKey{pub}, "amd64", nil).Run(plan); !r.Refused {
		t.Fatal("signed arm64 payload accepted as amd64")
	}
}
func TestAuditMissingHealthRunnerCannotPass(t *testing.T) {
	_, ok := NewExecutor(nil, 0).RunAll([]HealthCheck{{Name: "agent", Command: "status"}})
	if ok {
		t.Error("missing runner claimed successful health")
	}
	if _, ok = NewExecutor(nil, 0).RunAll(nil); ok {
		t.Error("zero checks claimed successful health")
	}
}

func TestAuditInvalidHealthSetupDoesNotActivate(t *testing.T) {
	store := newFakeStore(1 << 30)
	o := NewOrchestrator(store, nil, "amd64", nil)
	o.state.Active = "known-good"
	o.state.Candidate = "candidate"
	if _, err := o.ActivateAndVerify(nil, []HealthCheck{{Name: "agent", Command: "status"}}, 0); err == nil {
		t.Fatal("missing runner accepted")
	}
	if len(store.activated) != 0 || o.State().Active != "known-good" || o.State().Candidate != "candidate" {
		t.Fatal("invalid health setup mutated activation state")
	}
}
