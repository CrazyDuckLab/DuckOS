// H11 update recovery torture regression.
package upgrade

import (
	"context"
	"errors"
	"testing"
	"time"
)

type stubStore struct {
	fingerprints map[string]error
}

func (s *stubStore) Materialize(_ ReleaseMeta, _ []byte) error { return nil }
func (s *stubStore) Activate(id string) error                  { return nil }
func (s *stubStore) List() ([]string, error)                   { return nil, nil }
func (s *stubStore) ReadMeta(_ string) (ReleaseMeta, error)    { return ReleaseMeta{}, nil }
func (s *stubStore) ReadPayload(_ string) ([]byte, error)      { return nil, nil }
func (s *stubStore) Remove(_ string) error                     { return nil }
func (s *stubStore) FreeBytes() (int64, error)                 { return 0, nil }
func (s *stubStore) VerifyFingerprint(id string) error {
	if err, ok := s.fingerprints[id]; ok {
		return err
	}
	return nil
}

func TestVerifyCandidate_RejectsEmptyID(t *testing.T) {
	if err := VerifyCandidate(context.Background(), &stubStore{}, ""); err == nil {
		t.Fatalf("empty id must be rejected")
	}
}

func TestVerifyCandidate_RejectsNilStore(t *testing.T) {
	if err := VerifyCandidate(context.Background(), nil, "rel-1"); err == nil {
		t.Fatalf("nil store must be rejected")
	}
}

func TestVerifyCandidate_RejectsNilContext(t *testing.T) {
	if err := VerifyCandidate(nil, &stubStore{}, "rel-1"); err == nil {
		t.Fatalf("nil context must be rejected")
	}
}

func TestVerifyCandidate_WrapsFingerprintError(t *testing.T) {
	store := &stubStore{fingerprints: map[string]error{
		"rel-bad": errors.New("checksum mismatch"),
	}}
	err := VerifyCandidate(context.Background(), store, "rel-bad")
	if err == nil {
		t.Fatalf("expected wrapped error")
	}
	if !errors.Is(err, ErrCorruptCandidate) {
		t.Fatalf("error must be ErrCorruptCandidate, got %v", err)
	}
}

func TestVerifyCandidate_SucceedsOnCleanFingerprint(t *testing.T) {
	store := &stubStore{fingerprints: map[string]error{}}
	if err := VerifyCandidate(context.Background(), store, "rel-good"); err != nil {
		t.Fatalf("clean fingerprint must succeed, got %v", err)
	}
}

func TestLoadState_RejectsNilOrchestrator(t *testing.T) {
	if err := LoadState(nil, &stubStore{}, State{Channel: ChannelStable, Policy: PolicyNotify, Active: "x"}); err == nil {
		t.Fatalf("nil orchestrator must be rejected")
	}
}

func TestLoadState_RejectsNilStore(t *testing.T) {
	o := NewOrchestrator(&stubStore{}, nil, "amd64", time.Now)
	if err := LoadState(o, nil, State{Channel: ChannelStable, Policy: PolicyNotify, Active: "x"}); err == nil {
		t.Fatalf("nil store must be rejected")
	}
}

func TestLoadState_RejectsInvalidState(t *testing.T) {
	o := NewOrchestrator(&stubStore{}, nil, "amd64", time.Now)
	if err := LoadState(o, &stubStore{}, State{}); err == nil {
		t.Fatalf("empty state must be rejected")
	}
}

func TestLoadState_DetectsMissingActive(t *testing.T) {
	o := NewOrchestrator(&stubStore{}, nil, "amd64", time.Now)
	store := &stubStore{fingerprints: map[string]error{
		"active-missing": errors.New("not on disk"),
	}}
	err := LoadState(o, store, State{
		Channel:   ChannelStable,
		Policy:    PolicyNotify,
		Active:    "active-missing",
		UpdatedAt: time.Now().UTC(),
	})
	if !errors.Is(err, ErrInconsistentState) {
		t.Fatalf("missing active must yield ErrInconsistentState, got %v", err)
	}
}

func TestLoadState_AcceptsConsistentState(t *testing.T) {
	o := NewOrchestrator(&stubStore{}, nil, "amd64", time.Now)
	store := &stubStore{fingerprints: map[string]error{}}
	state := State{
		Channel:   ChannelStable,
		Policy:    PolicyNotify,
		Active:    "rel-1",
		Previous:  "rel-2",
		Candidate: "rel-3",
		UpdatedAt: time.Now().UTC(),
	}
	if err := LoadState(o, store, state); err != nil {
		t.Fatalf("consistent state must load, got %v", err)
	}
	if o.State().Active != "rel-1" {
		t.Fatalf("active must be loaded")
	}
}

func TestStateValid(t *testing.T) {
	if err := (State{}).Valid(); err == nil {
		t.Fatalf("empty state must be invalid")
	}
	if err := (State{Channel: ChannelStable, Policy: PolicyNotify, Active: "a", UpdatedAt: time.Now().UTC()}).Valid(); err != nil {
		t.Fatalf("valid state must pass: %v", err)
	}
}

type recoveryRunner struct {
	out error
}

func (r *recoveryRunner) Run(_ HealthCheck) error { return r.out }

func TestCancellableHealthCheck_RejectsNilContext(t *testing.T) {
	res := CancellableHealthCheck(nil, &recoveryRunner{}, HealthCheck{Name: "test", Command: "echo ok"})
	if res.Passed || res.Error == "" {
		t.Fatalf("nil context must yield failure")
	}
}

func TestCancellableHealthCheck_RejectsInvalidCheck(t *testing.T) {
	res := CancellableHealthCheck(context.Background(), &recoveryRunner{}, HealthCheck{Name: "BAD!", Command: "echo ok"})
	if res.Passed || res.Error == "" {
		t.Fatalf("invalid check must yield failure")
	}
}

func TestCancellableHealthCheck_Success(t *testing.T) {
	res := CancellableHealthCheck(context.Background(), &recoveryRunner{}, HealthCheck{Name: "ok", Command: "echo ok"})
	if !res.Passed {
		t.Fatalf("clean run must succeed, got %+v", res)
	}
}

func TestCancellableHealthCheck_RunnerError(t *testing.T) {
	res := CancellableHealthCheck(context.Background(), &recoveryRunner{out: errors.New("boom")}, HealthCheck{Name: "bad", Command: "echo ok"})
	if res.Passed {
		t.Fatalf("runner error must yield failure")
	}
}

func TestCancellableHealthCheck_CancelMidway(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	// Slow runner: block until cancellation.
	slow := &slowRunner{}
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()
	res := CancellableHealthCheck(ctx, slow, HealthCheck{Name: "slow", Command: "sleep 5"})
	if res.Passed {
		t.Fatalf("canceled check must yield failure, got %+v", res)
	}
	if res.Error == "" {
		t.Fatalf("canceled check must surface an error")
	}
}

// slowRunner blocks on a channel until ctx is canceled.
type slowRunner struct{ ch chan struct{} }

func (r *slowRunner) Run(_ HealthCheck) error {
	r.ch <- struct{}{}
	return errors.New("should not reach here before cancel")
}
