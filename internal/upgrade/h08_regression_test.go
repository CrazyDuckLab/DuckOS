package upgrade

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestH08HealthReaderDoesNotDeadlock(t *testing.T) {
	s := newFakeStore(1 << 30)
	if err := s.Materialize(ReleaseMeta{ID: "candidate"}, []byte("payload")); err != nil {
		t.Fatal(err)
	}
	o := NewOrchestrator(s, nil, "amd64", nil)
	o.SetState(State{Channel: ChannelStable, Policy: PolicyNotify, Candidate: "candidate"})
	results, err := o.ActivateAndVerify(verificationRunner(func(HealthCheck) error {
		if o.State().Active != "candidate" {
			return errors.New("wrong selection")
		}
		return nil
	}), []HealthCheck{{Name: "reader", Command: "status"}}, time.Second)
	if err != nil || len(results) != 1 || !results[0].Passed {
		t.Fatalf("health reader blocked: %+v %v", results, err)
	}
}

func TestH08FailedFirstActivationClearsDurableSelection(t *testing.T) {
	dir := t.TempDir()
	s, err := NewReleaseStore(dir, 1024)
	if err != nil {
		t.Fatal(err)
	}
	mustMaterialize(t, s, "candidate", []byte("payload"))
	o := NewOrchestrator(s, nil, "amd64", nil)
	o.SetState(State{Channel: ChannelStable, Policy: PolicyNotify, Candidate: "candidate"})
	_, err = o.ActivateAndVerify(verificationRunner(func(HealthCheck) error { return errors.New("unhealthy") }), []HealthCheck{{Name: "runtime", Command: "status"}}, time.Second)
	if err == nil {
		t.Fatal("failed health accepted")
	}
	if st := o.State(); st.Active != "" || st.Candidate != "candidate" {
		t.Fatalf("unsafe state: %+v", st)
	}
	if _, err := os.Lstat(filepath.Join(dir, "active")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("active selection survived: %v", err)
	}
	if _, err := s.ReadPayload("candidate"); err != nil {
		t.Fatal("diagnostic candidate was removed", err)
	}
}
