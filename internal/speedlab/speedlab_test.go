package speedlab

import (
	"strings"
	"testing"
	"time"
)

func newRequest(t *testing.T) Request {
	t.Helper()
	return Request{
		ID:         "probe-1",
		Kind:       KindLatency,
		Target:     Target{Host: "1.1.1.1", Port: 443, Label: "cloudflare"},
		Duration:   5 * time.Second,
		MaxSamples: 32,
		Operator:   "admin",
		CreatedAt:  time.Now().UTC(),
	}
}

func TestRequestValid(t *testing.T) {
	r := newRequest(t)
	if err := r.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestRequestRejectsBadID(t *testing.T) {
	r := newRequest(t)
	r.ID = "id with space"
	if err := r.Validate(); err == nil {
		t.Fatal("invalid id accepted")
	}
}

func TestRequestRejectsBadKind(t *testing.T) {
	r := newRequest(t)
	r.Kind = "nmap"
	if err := r.Validate(); err == nil {
		t.Fatal("invalid kind accepted")
	}
}

func TestRequestRejectsZeroDuration(t *testing.T) {
	r := newRequest(t)
	r.Duration = 0
	if err := r.Validate(); err == nil {
		t.Fatal("zero duration accepted")
	}
}

func TestRequestRejectsLongDuration(t *testing.T) {
	r := newRequest(t)
	r.Duration = MaxDuration + time.Second
	if err := r.Validate(); err == nil {
		t.Fatal("oversized duration accepted")
	}
}

func TestRequestRejectsShortDuration(t *testing.T) {
	r := newRequest(t)
	r.Duration = 500 * time.Millisecond
	if err := r.Validate(); err == nil {
		t.Fatal("under-minimum duration accepted")
	}
}

func TestRequestRejectsBadMaxSamples(t *testing.T) {
	r := newRequest(t)
	r.MaxSamples = MaxSamples + 1
	if err := r.Validate(); err == nil {
		t.Fatal("oversized max samples accepted")
	}
	r.MaxSamples = 0
	if err := r.Validate(); err == nil {
		t.Fatal("zero max samples accepted")
	}
}

func TestRequestRejectsBadOperator(t *testing.T) {
	r := newRequest(t)
	r.Operator = "root; rm -rf /"
	if err := r.Validate(); err == nil {
		t.Fatal("unsafe operator accepted")
	}
}

func TestRequestRejectsBadReason(t *testing.T) {
	r := newRequest(t)
	r.Reason = strings.Repeat("x", 257)
	if err := r.Validate(); err == nil {
		t.Fatal("oversized reason accepted")
	}
}

func TestRequestRejectsBadTarget(t *testing.T) {
	r := newRequest(t)
	r.Target = Target{Host: "host; rm -rf /"}
	if err := r.Validate(); err == nil {
		t.Fatal("unsafe host accepted")
	}
}

func TestRequestAcceptsURL(t *testing.T) {
	r := newRequest(t)
	r.Target = Target{URL: "https://example.com/healthz", Label: "self"}
	if err := r.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestRequestRejectsBadURL(t *testing.T) {
	r := newRequest(t)
	r.Target = Target{URL: "ftp://example.com"}
	if err := r.Validate(); err == nil {
		t.Fatal("non-http url accepted")
	}
}

func TestRequestRejectsOversizedURL(t *testing.T) {
	r := newRequest(t)
	r.Target = Target{URL: "https://" + strings.Repeat("a", MaxTargetBytes)}
	if err := r.Validate(); err == nil {
		t.Fatal("oversized url accepted")
	}
}

func TestRequestRejectsBadPort(t *testing.T) {
	r := newRequest(t)
	r.Target.Port = 70000
	if err := r.Validate(); err == nil {
		t.Fatal("oversized port accepted")
	}
}

func TestReduceLatencyEmpty(t *testing.T) {
	if got := ReduceLatency(nil); got != (LatencyStats{}) {
		t.Fatalf("expected empty stats, got %+v", got)
	}
}

func TestReduceLatencyPercentiles(t *testing.T) {
	samples := []int64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	got := ReduceLatency(samples)
	if got.Count != 10 {
		t.Fatalf("expected 10 samples, got %d", got.Count)
	}
	if got.MinUS != 1 {
		t.Fatalf("expected min 1, got %d", got.MinUS)
	}
	if got.MaxUS != 10 {
		t.Fatalf("expected max 10, got %d", got.MaxUS)
	}
	if got.AvgUS != 5 {
		t.Fatalf("expected avg 5, got %d", got.AvgUS)
	}
	if got.P50US < 4 || got.P50US > 6 {
		t.Fatalf("p50 out of range: %d", got.P50US)
	}
}

func TestReduceLatencyFiltersOutOfRange(t *testing.T) {
	samples := []int64{1, -10, 2, MaxRTTMicros + 1, 3}
	got := ReduceLatency(samples)
	if got.Count != 3 {
		t.Fatalf("expected 3 valid samples, got %d", got.Count)
	}
}

func TestManagerStarts(t *testing.T) {
	m := NewManager(&fakeHost{})
	r := newRequest(t)
	ref, err := m.Start(r)
	if err != nil {
		t.Fatal(err)
	}
	if ref.ID != r.ID {
		t.Fatalf("id mismatch: %q vs %q", ref.ID, r.ID)
	}
}

func TestManagerEnforcesCap(t *testing.T) {
	m := NewManager(&fakeHost{})
	for i := 0; i < MaxConcurrent; i++ {
		r := newRequest(t)
		r.ID = "probe-" + itoa(i)
		if _, err := m.Start(r); err != nil {
			t.Fatal(err)
		}
	}
	r := newRequest(t)
	r.ID = "probe-overflow"
	if _, err := m.Start(r); err != ErrBusy {
		t.Fatalf("expected ErrBusy, got %v", err)
	}
}

func TestManagerCancel(t *testing.T) {
	m := NewManager(&fakeHost{})
	r := newRequest(t)
	if _, err := m.Start(r); err != nil {
		t.Fatal(err)
	}
	if err := m.Cancel(r.ID); err != nil {
		t.Fatal(err)
	}
	if list := m.List(); len(list) != 0 {
		t.Fatalf("expected empty after cancel, got %d", len(list))
	}
}

func TestManagerList(t *testing.T) {
	m := NewManager(&fakeHost{})
	for i := 0; i < MaxConcurrent; i++ {
		r := newRequest(t)
		r.ID = "probe-" + itoa(i)
		if _, err := m.Start(r); err != nil {
			t.Fatal(err)
		}
	}
	if list := m.List(); len(list) != MaxConcurrent {
		t.Fatalf("expected %d probes, got %d", MaxConcurrent, len(list))
	}
}

func TestUnsupportedHost(t *testing.T) {
	m := NewManager(nil)
	r := newRequest(t)
	if _, err := m.Start(r); err != ErrUnsupported {
		t.Fatalf("expected ErrUnsupported, got %v", err)
	}
}

func TestManagerFinalizeRemoves(t *testing.T) {
	m := NewManager(&fakeHost{})
	r := newRequest(t)
	if _, err := m.Start(r); err != nil {
		t.Fatal(err)
	}
	m.Finalize(r.ID, Summary{ID: r.ID, Kind: r.Kind, Target: r.Target})
	if list := m.List(); len(list) != 0 {
		t.Fatalf("expected empty after finalize, got %d", len(list))
	}
}

type fakeHost struct{}

func (f *fakeHost) Start(r Request) (ActiveRef, error) {
	return ActiveRef{ID: r.ID, State: StateRunning, StartedAt: time.Now().UTC()}, nil
}
func (f *fakeHost) Status(id string) (State, Summary, error) {
	return StateDone, Summary{ID: id, Status: string(StateDone)}, nil
}
func (f *fakeHost) Cancel(string) error { return nil }
func (f *fakeHost) List() []ActiveRef   { return nil }

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	const digits = "0123456789"
	if i < 0 {
		i = -i
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = digits[i%10]
		i /= 10
	}
	return string(buf[pos:])
}
