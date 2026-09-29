//go:build linux

// bounded_resources_test.go verifies the
// bounded-resource contract for the V1
// integration candidate: log ring, log
// subscriber cap, capture model bounds, and
// capture manager concurrency cap. The test
// is the integration-level contract for the
// "every queue is bounded" requirement: a
// runaway producer must never grow memory or
// file handles past documented caps, and a
// missed cleanup must never block the next
// operation.
//
// The test exercises real logger + capture
// manager composition. It does not start a
// real tcpdump; it uses a fake host that
// echoes the request lifecycle.
package integration

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/capture"
	"github.com/CrazyDuckLab/DuckOS/internal/logging"
)

func TestLogRingCapsAtConfiguredCapacity(t *testing.T) {
	const cap = 32
	l := logging.New(logging.Config{RingCapacity: cap})
	l.SetLevel(logging.Global, logging.Trace)
	for i := 0; i < cap*4; i++ {
		l.Log("v", logging.SevInfo, fmt.Sprintf("entry %d", i), nil)
	}
	recs := l.Snapshot()
	if got := len(recs); got != cap {
		t.Fatalf("ring length = %d, want %d", got, cap)
	}
	// Newest records are kept. The first record
	// must be at index cap*3 (i.e. the oldest
	// survivor of the latest cap entries).
	if !strings.Contains(recs[0].Message, fmt.Sprintf("entry %d", cap*3)) {
		t.Fatalf("oldest survivor = %q, want entry %d", recs[0].Message, cap*3)
	}
	if !strings.Contains(recs[cap-1].Message, fmt.Sprintf("entry %d", cap*4-1)) {
		t.Fatalf("newest record = %q, want entry %d", recs[cap-1].Message, cap*4-1)
	}
	emitted, dropped := l.Counts()
	if dropped == 0 {
		t.Fatal("dropped counter must be > 0 after ring overflow")
	}
	if emitted != uint64(cap*4) {
		t.Fatalf("emitted = %d, want %d", emitted, cap*4)
	}
}

func TestLogSubscriberLimitIsBounded(t *testing.T) {
	const max = 8
	l := logging.New(logging.Config{MaxSubscribers: max})
	subs := make([]<-chan logging.Record, 0, max+1)
	for i := 0; i < max; i++ {
		ch, err := l.Subscribe()
		if err != nil {
			t.Fatalf("Subscribe %d: %v", i, err)
		}
		subs = append(subs, ch)
	}
	if _, err := l.Subscribe(); err == nil {
		t.Fatal("Subscribe past limit must return error")
	}
	for _, ch := range subs {
		l.Unsubscribe(ch)
	}
	if _, err := l.Subscribe(); err != nil {
		t.Fatalf("Unsubscribe then Subscribe must succeed: %v", err)
	}
}

func TestLogSlowSubscriberDoesNotBlockProducer(t *testing.T) {
	// The contract is: a slow subscriber's
	// channel fills up; the producer uses a
	// non-blocking send and increments the
	// dropped counter; subsequent records still
	// reach the ring.
	l := logging.New(logging.Config{RingCapacity: 256})
	ch, err := l.Subscribe()
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer l.Unsubscribe(ch)
	// Drain nothing: subscriber is full.
	for i := 0; i < 1024; i++ {
		l.Log("v", logging.SevInfo, fmt.Sprintf("e%d", i), nil)
	}
	emitted, dropped := l.Counts()
	if dropped == 0 {
		t.Fatal("expected dropped > 0 when subscriber is full")
	}
	if emitted != 1024 {
		t.Fatalf("emitted = %d, want 1024", emitted)
	}
	if got := len(l.Snapshot()); got != 256 {
		t.Fatalf("ring len = %d, want 256", got)
	}
}

func TestLogCloseStopsAcceptingNewRecords(t *testing.T) {
	l := logging.New(logging.DefaultConfig())
	l.SetLevel(logging.Global, logging.Trace)
	l.Close()
	l.Log("v", logging.SevInfo, "after close", nil)
	emitted, _ := l.Counts()
	if emitted != 0 {
		t.Fatalf("emitted after Close = %d, want 0", emitted)
	}
}

type fakeCaptureHost struct {
	mu          sync.Mutex
	started     int
	concurrent  int32
	maxObserved int32
	finished    int
	cancelled   int
	stopped     chan struct{}
}

func newFakeHost(stopped chan struct{}) *fakeCaptureHost {
	return &fakeCaptureHost{stopped: stopped}
}

func (h *fakeCaptureHost) Start(req capture.Request) (capture.ActiveRef, error) {
	h.mu.Lock()
	h.started++
	cur := atomic.AddInt32(&h.concurrent, 1)
	if cur > atomic.LoadInt32(&h.maxObserved) {
		atomic.StoreInt32(&h.maxObserved, cur)
	}
	h.mu.Unlock()
	go func() {
		<-h.stopped
		atomic.AddInt32(&h.concurrent, -1)
		h.mu.Lock()
		h.finished++
		h.mu.Unlock()
	}()
	return capture.ActiveRef{ID: req.ID, State: capture.StateRunning, StartedAt: time.Now().UTC()}, nil
}

func (h *fakeCaptureHost) Status(id string) (capture.State, capture.Result, error) {
	return capture.StateRunning, capture.Result{ID: id, Link: "eth0", Status: string(capture.StateRunning)}, nil
}

func (h *fakeCaptureHost) Cancel(id string) error {
	h.mu.Lock()
	h.cancelled++
	h.mu.Unlock()
	atomic.AddInt32(&h.concurrent, -1)
	return nil
}

func (h *fakeCaptureHost) List() []capture.ActiveRef { return nil }

func makeCaptureRequest(t *testing.T, id string) capture.Request {
	t.Helper()
	return capture.Request{
		ID:        id,
		Link:      "eth0",
		Filter:    "tcp port 80",
		Duration:  2 * time.Second,
		MaxBytes:  1 << 20,
		Operator:  "operator-1",
		CreatedAt: time.Now().UTC(),
		Reason:    "bounded-resource-test",
	}
}

func TestCaptureManagerRejectsBeyondMaxConcurrent(t *testing.T) {
	stopped := make(chan struct{})
	defer close(stopped)
	mgr := capture.NewManager(newFakeHost(stopped))
	for i := 0; i < capture.MaxConcurrent; i++ {
		req := makeCaptureRequest(t, fmt.Sprintf("cap-%d", i))
		if _, err := mgr.Start(req); err != nil {
			t.Fatalf("Start %d: %v", i, err)
		}
	}
	// Next one is rejected with ErrBusy.
	req := makeCaptureRequest(t, "cap-overflow")
	if _, err := mgr.Start(req); err == nil {
		t.Fatal("Start past MaxConcurrent must fail")
	}
}

func TestCaptureManagerRejectsInvalidRequest(t *testing.T) {
	stopped := make(chan struct{})
	defer close(stopped)
	mgr := capture.NewManager(newFakeHost(stopped))
	// Filter with shell metacharacter must be rejected.
	bad := capture.Request{
		ID:        "cap-bad-1",
		Link:      "eth0",
		Filter:    "tcp port 80; rm -rf /",
		Duration:  time.Second,
		MaxBytes:  1 << 20,
		CreatedAt: time.Now().UTC(),
	}
	if _, err := mgr.Start(bad); err == nil {
		t.Fatal("Start with unsafe filter must fail")
	}
	// Duration out of bounds.
	badDur := makeCaptureRequest(t, "cap-bad-2")
	badDur.Duration = 10 * time.Minute
	if _, err := mgr.Start(badDur); err == nil {
		t.Fatal("Start with duration > MaxDuration must fail")
	}
	// MaxBytes out of bounds.
	badBytes := makeCaptureRequest(t, "cap-bad-3")
	badBytes.MaxBytes = 1 << 30
	if _, err := mgr.Start(badBytes); err == nil {
		t.Fatal("Start with maxBytes > MaxBytes must fail")
	}
	// Link too long.
	badLink := makeCaptureRequest(t, "cap-bad-4")
	badLink.Link = strings.Repeat("a", capture.MaxInterfaceLen+1)
	if _, err := mgr.Start(badLink); err == nil {
		t.Fatal("Start with overlong link must fail")
	}
}

func TestCaptureManagerCancelRemovesFromActiveSet(t *testing.T) {
	stopped := make(chan struct{})
	defer close(stopped)
	host := newFakeHost(stopped)
	mgr := capture.NewManager(host)
	req := makeCaptureRequest(t, "cap-cancel-1")
	if _, err := mgr.Start(req); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := mgr.Cancel("cap-cancel-1"); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	// After cancel, the slot is free again.
	again := makeCaptureRequest(t, "cap-cancel-2")
	if _, err := mgr.Start(again); err != nil {
		t.Fatalf("Start after cancel: %v", err)
	}
}

func TestLogRingBoundedUnderConcurrentProducers(t *testing.T) {
	const ring = 128
	const producers = 16
	const perProducer = 1024
	l := logging.New(logging.Config{RingCapacity: ring})
	l.SetLevel(logging.Global, logging.Trace)
	var wg sync.WaitGroup
	for p := 0; p < producers; p++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < perProducer; i++ {
				l.Log("v", logging.SevInfo, fmt.Sprintf("p%d-i%d", id, i), nil)
			}
		}(p)
	}
	wg.Wait()
	if got := len(l.Snapshot()); got > ring {
		t.Fatalf("ring grew to %d, must stay <= %d", got, ring)
	}
	emitted, dropped := l.Counts()
	if emitted != uint64(producers*perProducer) {
		t.Fatalf("emitted = %d, want %d", emitted, producers*perProducer)
	}
	if dropped == 0 {
		t.Fatal("dropped must be > 0 after concurrent overflow")
	}
}
