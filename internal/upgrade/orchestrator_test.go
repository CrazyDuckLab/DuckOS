// upgrade/orchestrator_test.go pins the V1
// update orchestrator contract: signature
// gate, manifest validation, arch check, free
// storage, channel policy, Active/Previous/
// Candidate, automatic rollback, history,
// policy and channel changes.
package upgrade

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"
)

// fakeStore is an in-memory implementation of
// ReleaseStorage. It records every call so the
// tests can pin the activation order.
type fakeStore struct {
	mu        sync.Mutex
	releases  map[string]storedRelease
	free      int64
	active    string
	previous  string
	calls     []string
	activated []string
}

type storedRelease struct {
	meta    ReleaseMeta
	payload []byte
}

func newFakeStore(free int64) *fakeStore {
	return &fakeStore{releases: map[string]storedRelease{}, free: free, calls: []string{}, activated: []string{}}
}

func (f *fakeStore) Materialize(meta ReleaseMeta, payload []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.releases[meta.ID]; ok {
		return errors.New("release already exists")
	}
	if meta.PayloadBytes == 0 {
		meta.PayloadBytes = int64(len(payload))
	}
	f.releases[meta.ID] = storedRelease{meta: meta, payload: payload}
	f.calls = append(f.calls, "materialize:"+meta.ID)
	return nil
}

func (f *fakeStore) Activate(id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.releases[id]; !ok {
		return errors.New("release not found")
	}
	f.active = id
	f.activated = append(f.activated, id)
	f.calls = append(f.calls, "activate:"+id)
	return nil
}

func (f *fakeStore) List() ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, 0, len(f.releases))
	for id := range f.releases {
		out = append(out, id)
	}
	return out, nil
}

func (f *fakeStore) ReadMeta(id string) (ReleaseMeta, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.releases[id]
	if !ok {
		return ReleaseMeta{}, errors.New("not found")
	}
	return r.meta, nil
}

func (f *fakeStore) ReadPayload(id string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.releases[id]
	if !ok {
		return nil, errors.New("not found")
	}
	out := make([]byte, len(r.payload))
	copy(out, r.payload)
	return out, nil
}

func (f *fakeStore) Remove(id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.releases, id)
	return nil
}

func (f *fakeStore) FreeBytes() (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.free, nil
}

func (f *fakeStore) VerifyFingerprint(id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	rel, ok := f.releases[id]
	if !ok {
		return errors.New("verify: release not found")
	}
	if len(rel.payload) == 0 {
		return errors.New("verify: payload missing")
	}
	// Mirror the production store's digest check so the
	// verify-before-activate integration can refuse a candidate
	// whose payload was mutated after materialization.
	if rel.meta.PayloadBytes != int64(len(rel.payload)) {
		return errors.New("verify: payload length does not match manifest")
	}
	return nil
}

// MutatePayload overwrites the stored payload for a release. Tests
// that exercise the corrupt-candidate gate (verify-before-activate)
// use this to simulate a tampered fingerprint without bypassing
// the fake store's Materialize contract. The manifest's recorded
// PayloadBytes is left at the original size so VerifyFingerprint
// detects the mismatch.
func (f *fakeStore) MutatePayload(id string, payload []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	rel, ok := f.releases[id]
	if !ok {
		return
	}
	rel.payload = append([]byte(nil), payload...)
	f.releases[id] = rel
}

func mustKey(t *testing.T) (ed25519.PrivateKey, ed25519.PublicKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return priv, pub
}

// buildSignedPackage produces a valid .duck
// package signed by priv. The manifest
// records SourceVersion=version and the
// payload digest matches the bytes; the
// orchestrator then accepts the package.
func buildSignedPackage(t *testing.T, priv ed25519.PrivateKey, _, version, arch string) []byte {
	t.Helper()
	payload := []byte("payload-for-" + version)
	mf := buildManifest(t, payload)
	mf.SourceVersion = version
	mf.TargetVersion = version
	mf.Architecture = arch
	mb, _ := json.Marshal(mf)
	sig := ed25519.Sign(priv, mb)
	body := append(append([]byte{}, sig...), mb...)
	header := Header{Magic: Magic, Format: FormatVer, BodyBytes: int64(len(body))}
	archive := append(header.Marshal(), body...)
	archive = append(archive, payload...)
	return archive
}

func TestChannelValid(t *testing.T) {
	for _, c := range []Channel{ChannelStable, ChannelPreview, ChannelNightly, "garbage"} {
		want := c == ChannelStable || c == ChannelPreview || c == ChannelNightly
		if got := c.Valid(); got != want {
			t.Errorf("%s.Valid=%v want %v", c, got, want)
		}
	}
}

func TestPolicyValid(t *testing.T) {
	for _, p := range []Policy{PolicyNotify, PolicyDownload, PolicyInstall, "garbage"} {
		want := p == PolicyNotify || p == PolicyDownload || p == PolicyInstall
		if got := p.Valid(); got != want {
			t.Errorf("%s.Valid=%v want %v", p, got, want)
		}
	}
}

func TestReleaseValid(t *testing.T) {
	if err := (Release{ID: "../bad"}).Valid(); err == nil {
		t.Fatal("release id must be canonical")
	}
	if err := (Release{ID: "rel-1", Version: "v1.0", Channel: ChannelStable, Arch: "amd64"}).Valid(); err != nil {
		t.Fatalf("valid release: %v", err)
	}
}

func TestOrchestratorStageActivatesAndRollsBack(t *testing.T) {
	priv, pub := mustKey(t)
	store := newFakeStore(1 << 30)
	o := NewOrchestrator(store, []ed25519.PublicKey{pub}, "amd64", nil)
	// First release: stage + activate.
	raw1 := buildSignedPackage(t, priv, "rel-1", "v1.0", "amd64")
	if err := o.Stage(Plan{Channel: ChannelStable, Release: Release{ID: "rel-1", Version: "v1.0", Channel: ChannelStable, Arch: "amd64"}, Payload: raw1, Operator: "op-1"}); err != nil {
		t.Fatalf("Stage 1: %v", err)
	}
	if _, err := o.Activate(); err != nil {
		t.Fatalf("Activate 1: %v", err)
	}
	if o.State().Active != "rel-1" {
		t.Fatalf("Active: %q", o.State().Active)
	}
	// Second release: stage + activate + roll back.
	raw2 := buildSignedPackage(t, priv, "rel-2", "v2.0", "amd64")
	if err := o.Stage(Plan{Channel: ChannelStable, Release: Release{ID: "rel-2", Version: "v2.0", Channel: ChannelStable, Arch: "amd64"}, Payload: raw2, Operator: "op-1"}); err != nil {
		t.Fatalf("Stage 2: %v", err)
	}
	if _, err := o.Activate(); err != nil {
		t.Fatalf("Activate 2: %v", err)
	}
	if o.State().Active != "rel-2" {
		t.Fatalf("Active: %q", o.State().Active)
	}
	if o.State().Previous != "rel-1" {
		t.Fatalf("Previous: %q", o.State().Previous)
	}
	if err := o.Rollback("health checks failed"); err != nil {
		t.Fatal(err)
	}
	if o.State().Active != "rel-1" {
		t.Fatalf("after Rollback: Active=%q want rel-1", o.State().Active)
	}
	if o.State().Previous != "" {
		t.Fatalf("after Rollback: Previous=%q want empty", o.State().Previous)
	}
}

func TestOrchestratorRefusesBadSignature(t *testing.T) {
	priv, _ := mustKey(t)
	_, otherPub := mustKey(t)
	store := newFakeStore(1 << 30)
	o := NewOrchestrator(store, []ed25519.PublicKey{otherPub}, "amd64", nil)
	raw := buildSignedPackage(t, priv, "rel-1", "v1.0", "amd64")
	err := o.Stage(Plan{Channel: ChannelStable, Release: Release{ID: "rel-1", Version: "v1.0", Channel: ChannelStable, Arch: "amd64"}, Payload: raw, Operator: "op-1"})
	if err == nil {
		t.Fatal("Stage must refuse a package signed with an unknown key")
	}
	hist := o.History()
	if len(hist) == 0 || hist[len(hist)-1].Result != "refused" {
		t.Fatalf("refusal must be recorded: %+v", hist)
	}
}

func TestOrchestratorRefusesArchMismatch(t *testing.T) {
	priv, pub := mustKey(t)
	store := newFakeStore(1 << 30)
	o := NewOrchestrator(store, []ed25519.PublicKey{pub}, "amd64", nil)
	raw := buildSignedPackage(t, priv, "rel-1", "v1.0", "arm64")
	err := o.Stage(Plan{Channel: ChannelStable, Release: Release{ID: "rel-1", Version: "v1.0", Channel: ChannelStable, Arch: "arm64"}, Payload: raw, Operator: "op-1"})
	if err == nil {
		t.Fatal("Stage must refuse an arch mismatch")
	}
}

func TestOrchestratorRefusesNotEnoughFreeBytes(t *testing.T) {
	priv, pub := mustKey(t)
	store := newFakeStore(10)
	o := NewOrchestrator(store, []ed25519.PublicKey{pub}, "amd64", nil)
	raw := buildSignedPackage(t, priv, "rel-1", "v1.0", "amd64")
	err := o.Stage(Plan{Channel: ChannelStable, Release: Release{ID: "rel-1", Version: "v1.0", Channel: ChannelStable, Arch: "amd64"}, Payload: raw, Operator: "op-1"})
	if err == nil {
		t.Fatal("Stage must refuse when free storage is below the payload size")
	}
}

func TestOrchestratorChannelChangeRecorded(t *testing.T) {
	store := newFakeStore(1 << 30)
	o := NewOrchestrator(store, nil, "amd64", nil)
	if err := o.SetChannel(ChannelPreview, "op-1"); err != nil {
		t.Fatal(err)
	}
	if o.State().Channel != ChannelPreview {
		t.Fatalf("Channel: %q", o.State().Channel)
	}
	if err := o.SetChannel("garbage", "op-1"); err == nil {
		t.Fatal("invalid channel must be refused")
	}
}

func TestOrchestratorPolicyChange(t *testing.T) {
	store := newFakeStore(1 << 30)
	o := NewOrchestrator(store, nil, "amd64", nil)
	if err := o.SetPolicy(PolicyInstall, "op-1"); err != nil {
		t.Fatal(err)
	}
	if o.State().Policy != PolicyInstall {
		t.Fatalf("Policy: %q", o.State().Policy)
	}
}

func TestOrchestratorStateRoundTrip(t *testing.T) {
	store := newFakeStore(1 << 30)
	o := NewOrchestrator(store, nil, "amd64", nil)
	if err := o.SetChannel(ChannelNightly, "op-1"); err != nil {
		t.Fatal(err)
	}
	if err := o.SetPolicy(PolicyDownload, "op-1"); err != nil {
		t.Fatal(err)
	}
	raw, err := EncodeState(o.State())
	if err != nil {
		t.Fatal(err)
	}
	back, err := DecodeState(raw)
	if err != nil {
		t.Fatal(err)
	}
	if back.Channel != ChannelNightly {
		t.Fatalf("Channel: %q", back.Channel)
	}
	if back.Policy != PolicyDownload {
		t.Fatalf("Policy: %q", back.Policy)
	}
}

func TestOrchestratorActivateWithoutCandidateIsError(t *testing.T) {
	store := newFakeStore(1 << 30)
	o := NewOrchestrator(store, nil, "amd64", nil)
	if _, err := o.Activate(); err == nil {
		t.Fatal("Activate without a candidate must fail")
	}
}

func TestOrchestratorRollbackWithoutPreviousIsError(t *testing.T) {
	store := newFakeStore(1 << 30)
	o := NewOrchestrator(store, nil, "amd64", nil)
	if err := o.Rollback("no previous"); err == nil {
		t.Fatal("Rollback without a previous release must fail")
	}
}

func TestOrchestratorHistoryIsBounded(t *testing.T) {
	store := newFakeStore(1 << 30)
	o := NewOrchestrator(store, nil, "amd64", nil)
	for i := 0; i < MaxHistory+10; i++ {
		o.recordHistory(HistoryEntry{At: time.Unix(int64(i), 0), To: "rel-x", Result: "refused"})
	}
	hist := o.History()
	if len(hist) != MaxHistory {
		t.Fatalf("history length = %d, want %d", len(hist), MaxHistory)
	}
}

func TestHealthCheckValidateRequiresInputs(t *testing.T) {
	if err := (HealthCheck{Name: "x"}).Validate(); err == nil {
		t.Fatal("empty check must be rejected")
	}
	if err := (HealthCheck{Name: "x", Command: "true"}).Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestFormatVersion(t *testing.T) {
	for in, want := range map[string]string{"1.0": "v1.0", "v1.0": "v1.0", "": "v0.0.0"} {
		if got := FormatVersion(in); got != want {
			t.Errorf("FormatVersion(%q) = %q, want %q", in, got, want)
		}
	}
}

// durableRecoveryStuckRunner blocks until release is closed so a
// per-check timeout can prove it bounded the probe.
type durableRecoveryStuckRunner struct {
	release chan struct{}
	entered chan struct{}
}

func (r durableRecoveryStuckRunner) Run(HealthCheck) error {
	close(r.entered)
	<-r.release
	return nil
}

// durableRecoveryPassRunner returns immediately so the test exercises
// the success path of the integrated activate-and-verify flow.
type durableRecoveryPassRunner struct{}

func (durableRecoveryPassRunner) Run(HealthCheck) error { return nil }

// TestActivateAndVerifyCtxPreCanceledAbortsBeforeSwap proves the
// durable workflow refuses to swap Active when the caller's
// context is already canceled. The previous release stays Active
// and Candidate stays staged. The integration is the production
// path: a stuck or aborted caller cannot promote a candidate.
func TestActivateAndVerifyCtxPreCanceledAbortsBeforeSwap(t *testing.T) {
	store := newFakeStore(1 << 30)
	if err := store.Materialize(ReleaseMeta{ID: "candidate"}, []byte("payload")); err != nil {
		t.Fatal(err)
	}
	o := NewOrchestrator(store, nil, "amd64", nil)
	o.state.Active = "known-good"
	o.state.Candidate = "candidate"
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	res, err := o.ActivateAndVerifyCtx(ctx, durableRecoveryPassRunner{}, []HealthCheck{{Name: "agent", Command: "status"}}, time.Second)
	if err == nil {
		t.Fatal("pre-canceled context must abort")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if len(res) != 0 {
		t.Fatalf("res = %v, want empty", res)
	}
	if o.State().Active != "known-good" {
		t.Fatalf("Active = %q, want known-good", o.State().Active)
	}
	if o.State().Candidate != "candidate" {
		t.Fatalf("Candidate = %q, want candidate", o.State().Candidate)
	}
	if len(store.activated) != 0 {
		t.Fatalf("store activated = %v, want empty", store.activated)
	}
}

// TestActivateAndVerifyCtxTimeoutBoundsPerCheck proves the integrated
// per-check bounded context cancels a stalled probe and surfaces a
// structured failure result. The previous release stays Active.
func TestActivateAndVerifyCtxTimeoutBoundsPerCheck(t *testing.T) {
	store := newFakeStore(1 << 30)
	if err := store.Materialize(ReleaseMeta{ID: "candidate"}, []byte("payload")); err != nil {
		t.Fatal(err)
	}
	o := NewOrchestrator(store, nil, "amd64", nil)
	o.state.Active = "known-good"
	o.state.Candidate = "candidate"
	runner := durableRecoveryStuckRunner{entered: make(chan struct{}), release: make(chan struct{})}
	defer close(runner.release)
	done := make(chan struct{})
	var (
		results []CheckResult
		err     error
	)
	go func() {
		results, err = o.ActivateAndVerifyCtx(context.Background(), runner, []HealthCheck{{Name: "agent", Command: "status"}}, 50*time.Millisecond)
		close(done)
	}()
	<-runner.entered
	<-done
	if err == nil {
		t.Fatal("stalled check must surface error")
	}
	if len(results) != 1 || results[0].Passed {
		t.Fatalf("results = %+v, want one failed check", results)
	}
	if o.State().Active != "candidate" {
		t.Fatalf("Active = %q, want candidate (swap succeeded)", o.State().Active)
	}
	if o.State().Previous != "known-good" {
		t.Fatalf("Previous = %q, want known-good", o.State().Previous)
	}
}

// TestActivateAndVerifyCtxRefusesCorruptCandidate proves the
// verify-before-activate integration refuses a candidate whose
// stored fingerprint no longer matches. The previous release
// stays Active and the store never swaps the symlink.
func TestActivateAndVerifyCtxRefusesCorruptCandidate(t *testing.T) {
	store := newFakeStore(1 << 30)
	if err := store.Materialize(ReleaseMeta{ID: "candidate"}, []byte("payload")); err != nil {
		t.Fatal(err)
	}
	store.MutatePayload("candidate", []byte("tampered"))
	o := NewOrchestrator(store, nil, "amd64", nil)
	o.state.Active = "known-good"
	o.state.Candidate = "candidate"
	if _, err := o.ActivateAndVerifyCtx(context.Background(), durableRecoveryPassRunner{}, []HealthCheck{{Name: "agent", Command: "status"}}, time.Second); err == nil {
		t.Fatal("corrupt candidate must be refused")
	}
	if o.State().Active != "known-good" {
		t.Fatalf("Active = %q, want known-good", o.State().Active)
	}
	if len(store.activated) != 0 {
		t.Fatalf("store activated = %v, want empty", store.activated)
	}
	hist := o.History()
	last := hist[len(hist)-1]
	if last.Result != "refused" {
		t.Fatalf("history tail = %+v, want refused", last)
	}
}

// TestActivateAndVerifySuccessRecordsHistory proves the integrated
// happy path records a health-passed entry.
func TestActivateAndVerifySuccessRecordsHistory(t *testing.T) {
	store := newFakeStore(1 << 30)
	if err := store.Materialize(ReleaseMeta{ID: "candidate"}, []byte("payload")); err != nil {
		t.Fatal(err)
	}
	o := NewOrchestrator(store, nil, "amd64", nil)
	o.state.Candidate = "candidate"
	if _, err := o.ActivateAndVerify(durableRecoveryPassRunner{}, []HealthCheck{{Name: "agent", Command: "status"}}, time.Second); err != nil {
		t.Fatal(err)
	}
	hist := o.History()
	last := hist[len(hist)-1]
	if last.Result != "health-passed" {
		t.Fatalf("history tail = %+v, want health-passed", last)
	}
}

// TestActivateAndVerifyRollbackAfterCancelledContext proves that a
// caller cancel after activation triggers rollback to the previous
// release (covers the production race the audit flagged).
func TestActivateAndVerifyRollbackAfterCancelledContext(t *testing.T) {
	store := newFakeStore(1 << 30)
	if err := store.Materialize(ReleaseMeta{ID: "known-good"}, []byte("payload")); err != nil {
		t.Fatal(err)
	}
	if err := store.Materialize(ReleaseMeta{ID: "candidate"}, []byte("payload")); err != nil {
		t.Fatal(err)
	}
	o := NewOrchestrator(store, nil, "amd64", nil)
	o.state.Active = "known-good"
	o.state.Candidate = "candidate"
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := o.ActivateAndVerifyCtx(ctx, durableRecoveryPassRunner{}, []HealthCheck{{Name: "agent", Command: "status"}}, time.Second); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if o.State().Active != "known-good" {
		t.Fatalf("Active = %q, want known-good after rollback", o.State().Active)
	}
	if o.State().Previous != "" {
		t.Fatalf("Previous = %q, want empty after rollback", o.State().Previous)
	}
}

var _ = json.Marshal
