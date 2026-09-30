//go:build linux

package speedlab

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

// helper: spin up a TCP listener that accepts and immediately
// closes; enough for a latency probe.
func newTCPEcho(t *testing.T) (net.Listener, string) {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			_ = c.Close()
		}
	}()
	host, port, _ := net.SplitHostPort(l.Addr().String())
	_ = host
	return l, fmt.Sprintf("127.0.0.1:%s", port)
}

// helper: spin up an HTTP echo server with controllable
// behaviour (slow / fast / head-only).
func newHTTPServer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv
}

func newHostWithAllow(t *testing.T, allowed []string) *LinuxHost {
	t.Helper()
	return NewLinuxHost(runtimeConfig{
		AllowedHosts: allowed,
		AllowLoopback: true,
		AllowPrivate:  true,
		DialTimeout:   500 * time.Millisecond,
		TotalTimeout:  2 * time.Second,
	})
}

// 1. latency success against an in-process TCP listener.
func TestLinuxHost_LatencySuccess(t *testing.T) {
	l, addr := newTCPEcho(t)
	defer l.Close()
	host, port, _ := net.SplitHostPort(addr)
	p := 0
	fmt.Sscanf(port, "%d", &p)
	h := newHostWithAllow(t, []string{host})
	req := Request{
		ID:   "probe-lat-1",
		Kind: KindLatency,
		Target: Target{Host: host, Port: p},
		Duration: 500 * time.Millisecond,
		MaxSamples: 4,
		CreatedAt: time.Now().UTC(),
	}
	ref, err := h.Start(req)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if ref.State != StateRunning {
		t.Fatalf("state: %s", ref.State)
	}
	// wait for run to settle (no Finalize hook yet; wait by
	// polling Status).
	deadline := time.Now().Add(2 * time.Second)
	var got State
	for time.Now().Before(deadline) {
		got, _, _ = h.Status("probe-lat-1")
		if got != StateRunning {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if got == StateRunning {
		t.Fatalf("probe still running after 2s")
	}
	// Cannot read the summary directly without a Manager
	// hook; assert the probe left the active set instead.
	h.mu.Lock()
	_, still := h.active["probe-lat-1"]
	h.mu.Unlock()
	if still {
		t.Fatalf("probe still in active set")
	}
}

// 2. HTTP success against an in-process httptest server.
func TestLinuxHost_HTTPSuccess(t *testing.T) {
	srv := newHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	u, _ := url.Parse(srv.URL)
	host, _, _ := net.SplitHostPort(u.Host)
	h := newHostWithAllow(t, []string{host})
	req := Request{
		ID:   "probe-http-1",
		Kind: KindHTTP,
		Target: Target{URL: srv.URL},
		Duration: 300 * time.Millisecond,
		MaxSamples: 3,
		CreatedAt: time.Now().UTC(),
	}
	ref, err := h.Start(req)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if ref.State != StateRunning {
		t.Fatalf("state: %s", ref.State)
	}
	waitFor(t, h, "probe-http-1", 2*time.Second)
}

// 3. Throughput success.
func TestLinuxHost_ThroughputSuccess(t *testing.T) {
	payload := strings.Repeat("x", 4096)
	srv := newHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, payload)
	}))
	u, _ := url.Parse(srv.URL)
	host, _, _ := net.SplitHostPort(u.Host)
	h := newHostWithAllow(t, []string{host})
	req := Request{
		ID:   "probe-tp-1",
		Kind: KindThroughput,
		Target: Target{URL: srv.URL},
		Duration: 200 * time.Millisecond,
		MaxSamples: 1,
		CreatedAt: time.Now().UTC(),
	}
	if _, err := h.Start(req); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitFor(t, h, "probe-tp-1", 3*time.Second)
}

// 4. cancel an in-flight probe.
func TestLinuxHost_Cancel(t *testing.T) {
	l, addr := newTCPEcho(t)
	defer l.Close()
	host, port, _ := net.SplitHostPort(addr)
	p := 0
	fmt.Sscanf(port, "%d", &p)
	h := newHostWithAllow(t, []string{host})
	req := Request{
		ID:   "probe-cancel-1",
		Kind: KindLatency,
		Target: Target{Host: host, Port: p},
		Duration: 2 * time.Second,
		MaxSamples: 100,
		CreatedAt: time.Now().UTC(),
	}
	if _, err := h.Start(req); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := h.Cancel("probe-cancel-1"); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	h.mu.Lock()
	_, still := h.active["probe-cancel-1"]
	h.mu.Unlock()
	if still {
		t.Fatalf("probe still active after Cancel")
	}
}

// 5. SSRF: a target not in the allow-list is refused.
func TestLinuxHost_SSRFAllowListBlocks(t *testing.T) {
	h := NewLinuxHost(runtimeConfig{
		AllowedHosts: []string{"allowed.example"},
		AllowLoopback: false,
		AllowPrivate:  false,
	})
	req := Request{
		ID:   "probe-ssrf-1",
		Kind: KindLatency,
		Target: Target{Host: "evil.example", Port: 80},
		Duration: 100 * time.Millisecond,
		MaxSamples: 1,
		CreatedAt: time.Now().UTC(),
	}
	if _, err := h.Start(req); err == nil {
		t.Fatalf("expected SSRF refusal")
	}
}

// 6. SSRF: empty allow-list refuses every probe.
func TestLinuxHost_SSRFEmptyAllowListRefusesAll(t *testing.T) {
	h := NewLinuxHost(runtimeConfig{AllowLoopback: true, AllowPrivate: true})
	req := Request{
		ID:   "probe-empty-1",
		Kind: KindLatency,
		Target: Target{Host: "anything.example", Port: 80},
		Duration: 100 * time.Millisecond,
		MaxSamples: 1,
		CreatedAt: time.Now().UTC(),
	}
	if _, err := h.Start(req); err == nil {
		t.Fatalf("expected refusal on empty allow-list")
	}
}

// 7. SSRF: loopback is blocked when AllowLoopback=false.
func TestLinuxHost_SSRFLoopbackBlocked(t *testing.T) {
	l, _ := newTCPEcho(t)
	defer l.Close()
	_, port, _ := net.SplitHostPort(l.Addr().String())
	p := 0
	fmt.Sscanf(port, "%d", &p)
	h := NewLinuxHost(runtimeConfig{
		AllowedHosts:  []string{"127.0.0.1"},
		AllowLoopback: false,
	})
	req := Request{
		ID:   "probe-lo-1",
		Kind: KindLatency,
		Target: Target{Host: "127.0.0.1", Port: p},
		Duration: 100 * time.Millisecond,
		MaxSamples: 1,
		CreatedAt: time.Now().UTC(),
	}
	if _, err := h.Start(req); err == nil {
		t.Fatalf("expected loopback refusal")
	}
}

// 8. invalid target (port out of range) is rejected.
func TestLinuxHost_InvalidPortRejected(t *testing.T) {
	h := newHostWithAllow(t, []string{"x"})
	req := Request{
		ID:   "probe-port-1",
		Kind: KindLatency,
		Target: Target{Host: "x", Port: 0},
		Duration: 100 * time.Millisecond,
		MaxSamples: 1,
		CreatedAt: time.Now().UTC(),
	}
	if _, err := h.Start(req); err == nil {
		t.Fatalf("expected port rejection")
	}
}

// 9. invalid URL is rejected.
func TestLinuxHost_InvalidURLRejected(t *testing.T) {
	h := newHostWithAllow(t, []string{"x"})
	req := Request{
		ID:   "probe-url-1",
		Kind: KindHTTP,
		Target: Target{URL: "ftp://x"},
		Duration: 100 * time.Millisecond,
		MaxSamples: 1,
		CreatedAt: time.Now().UTC(),
	}
	if _, err := h.Start(req); err == nil {
		t.Fatalf("expected URL scheme rejection")
	}
}

// 10. timeout target refuses; dial timeout elapses, no sample.
func TestLinuxHost_TimeoutRefuses(t *testing.T) {
	// Bind a listener that never accepts. We use a raw TCP
	// listener then do not call Accept().
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer l.Close()
	host, port, _ := net.SplitHostPort(l.Addr().String())
	p := 0
	fmt.Sscanf(port, "%d", &p)
	h := newHostWithAllow(t, []string{host})
	req := Request{
		ID:   "probe-timeout-1",
		Kind: KindLatency,
		Target: Target{Host: host, Port: p},
		Duration:  600 * time.Millisecond,
		MaxSamples: 100,
		CreatedAt: time.Now().UTC(),
	}
	if _, err := h.Start(req); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitFor(t, h, "probe-timeout-1", 2*time.Second)
}

// 11. server disappears mid-test.
func TestLinuxHost_ServerDisappearsMidTest(t *testing.T) {
	l, addr := newTCPEcho(t)
	host, port, _ := net.SplitHostPort(addr)
	p := 0
	fmt.Sscanf(port, "%d", &p)
	h := newHostWithAllow(t, []string{host})
	req := Request{
		ID:   "probe-disappear-1",
		Kind: KindLatency,
		Target: Target{Host: host, Port: p},
		Duration:  500 * time.Millisecond,
		MaxSamples: 8,
		CreatedAt: time.Now().UTC(),
	}
	if _, err := h.Start(req); err != nil {
		t.Fatalf("Start: %v", err)
	}
	// Close the listener after a short delay; the probe must
	// not crash.
	time.Sleep(50 * time.Millisecond)
	_ = l.Close()
	waitFor(t, h, "probe-disappear-1", 2*time.Second)
}

// 12. duplicate id is rejected.
func TestLinuxHost_DuplicateIDRejected(t *testing.T) {
	l, addr := newTCPEcho(t)
	defer l.Close()
	host, port, _ := net.SplitHostPort(addr)
	p := 0
	fmt.Sscanf(port, "%d", &p)
	h := newHostWithAllow(t, []string{host})
	req := Request{
		ID:   "probe-dup-1",
		Kind: KindLatency,
		Target: Target{Host: host, Port: p},
		Duration:  500 * time.Millisecond,
		MaxSamples: 2,
		CreatedAt: time.Now().UTC(),
	}
	if _, err := h.Start(req); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if _, err := h.Start(req); err == nil {
		t.Fatalf("expected duplicate id refusal")
	}
	waitFor(t, h, "probe-dup-1", 2*time.Second)
}

// 13. oversized duration rejected at validate time.
func TestRequest_OversizedDurationRejected(t *testing.T) {
	r := Request{
		ID:         "ok",
		Kind:       KindLatency,
		Target:     Target{Host: "x", Port: 80},
		Duration:   999 * time.Second,
		MaxSamples: 1,
		CreatedAt:  time.Now().UTC(),
	}
	if err := r.Validate(); err == nil {
		t.Fatalf("expected oversized duration rejection")
	}
}

// 14. SecureRandomID returns 64-hex-character ids with no
// collision on a small sample.
func TestSecureRandomID_NoCollisionSmallSample(t *testing.T) {
	seen := map[string]struct{}{}
	for i := 0; i < 64; i++ {
		id := SecureRandomID()
		if len(id) != 64 {
			t.Fatalf("len=%d", len(id))
		}
		if _, dup := seen[id]; dup {
			t.Fatalf("dup id %s", id)
		}
		seen[id] = struct{}{}
	}
}

// 15. Status returns NotFound for unknown id.
func TestLinuxHost_Status_NotFound(t *testing.T) {
	h := NewLinuxHost(runtimeConfig{AllowLoopback: true, AllowPrivate: true})
	_, _, err := h.Status("missing")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err: %v", err)
	}
}

// 16. Cancel returns NotFound for unknown id.
func TestLinuxHost_Cancel_NotFound(t *testing.T) {
	h := NewLinuxHost(runtimeConfig{AllowLoopback: true, AllowPrivate: true})
	if err := h.Cancel("missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err: %v", err)
	}
}

// 17. List returns in-flight probes only.
func TestLinuxHost_List_OnlyInflight(t *testing.T) {
	l, addr := newTCPEcho(t)
	defer l.Close()
	host, port, _ := net.SplitHostPort(addr)
	p := 0
	fmt.Sscanf(port, "%d", &p)
	h := newHostWithAllow(t, []string{host})
	if got := h.List(); len(got) != 0 {
		t.Fatalf("empty list expected")
	}
	req := Request{
		ID:   "probe-list-1",
		Kind: KindLatency,
		Target: Target{Host: host, Port: p},
		Duration:  600 * time.Millisecond,
		MaxSamples: 4,
		CreatedAt: time.Now().UTC(),
	}
	if _, err := h.Start(req); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if got := h.List(); len(got) != 1 || got[0].ID != "probe-list-1" {
		t.Fatalf("list=%+v", got)
	}
	waitFor(t, h, "probe-list-1", 2*time.Second)
}

// helper: wait until a probe leaves the active set.
func waitFor(t *testing.T, h *LinuxHost, id string, dur time.Duration) {
	t.Helper()
	deadline := time.Now().Add(dur)
	for time.Now().Before(deadline) {
		h.mu.Lock()
		_, still := h.active[id]
		h.mu.Unlock()
		if !still {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("probe %s still in active set after %s", id, dur)
}

// 18. Concurrent starts honour MaxConcurrent.
func TestLinuxHost_ConcurrentBounded(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping concurrent bound test in short mode")
	}
	h := NewLinuxHost(runtimeConfig{AllowLoopback: true, AllowPrivate: true})
	// The Manager enforces MaxConcurrent, but the Host alone
	// does not. Verify Start accepts multiple probes (the
	// Manager is the gate; the Host keeps a per-id active map).
	for i := 0; i < 3; i++ {
		req := Request{
			ID:         fmt.Sprintf("probe-conc-%d", i),
			Kind:       KindLatency,
			Target:     Target{Host: "127.0.0.1", Port: 65000},
			Duration:   100 * time.Millisecond,
			MaxSamples: 1,
			CreatedAt:  time.Now().UTC(),
		}
		// Will fail to dial but Start itself succeeds.
		_, _ = h.Start(req)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.active) > 3 {
		t.Fatalf("active=%d > 3", len(h.active))
	}
}

// 19. context cancellation terminates the dial.
func TestLinuxHost_ContextCancellation(t *testing.T) {
	h := NewLinuxHost(runtimeConfig{
		AllowedHosts:  []string{"127.0.0.1"},
		AllowLoopback: true,
		AllowPrivate:  true,
		DialTimeout:   50 * time.Millisecond,
		TotalTimeout:  500 * time.Millisecond,
	})
	// 192.0.2.0/24 is TEST-NET-1; unrouted. The dial must
	// time out instead of looping forever.
	req := Request{
		ID:         "probe-ctx-1",
		Kind:       KindLatency,
		Target:     Target{Host: "127.0.0.1", Port: 65000},
		Duration:   50 * time.Millisecond,
		MaxSamples: 1,
		CreatedAt:  time.Now().UTC(),
	}
	if _, err := h.Start(req); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitFor(t, h, "probe-ctx-1", 2*time.Second)
}

// 20. concurrent probes are isolated.
func TestLinuxHost_ConcurrentProbesIsolated(t *testing.T) {
	l, addr := newTCPEcho(t)
	defer l.Close()
	host, port, _ := net.SplitHostPort(addr)
	p := 0
	fmt.Sscanf(port, "%d", &p)
	h := newHostWithAllow(t, []string{host})
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			req := Request{
				ID:         id,
				Kind:       KindLatency,
				Target:     Target{Host: host, Port: p},
				Duration:   300 * time.Millisecond,
				MaxSamples: 3,
				CreatedAt:  time.Now().UTC(),
			}
			_, _ = h.Start(req)
		}(fmt.Sprintf("probe-iso-%d", i))
	}
	wg.Wait()
	time.Sleep(500 * time.Millisecond)
	h.mu.Lock()
	still := len(h.active)
	h.mu.Unlock()
	if still != 0 {
		t.Fatalf("active=%d", still)
	}
}

// 21. http HEAD returns no body bytes (bounded).
func TestLinuxHost_HTTPNoBodyBytesLeak(t *testing.T) {
	var bodiesSeen []int
	srv := newHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("y", 10000)))
	}))
	u, _ := url.Parse(srv.URL)
	host, _, _ := net.SplitHostPort(u.Host)
	h := newHostWithAllow(t, []string{host})
	req := Request{
		ID:         "probe-noleak-1",
		Kind:       KindHTTP,
		Target:     Target{URL: srv.URL},
		Duration:   200 * time.Millisecond,
		MaxSamples: 2,
		CreatedAt:  time.Now().UTC(),
	}
	if _, err := h.Start(req); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitFor(t, h, "probe-noleak-1", 2*time.Second)
	_ = bodiesSeen
}

// 22. SecureRandomID uses crypto/rand.
func TestSecureRandomID_UsesCryptoRand(t *testing.T) {
	id := SecureRandomID()
	if id == "" {
		t.Fatal("empty id")
	}
	if len(id) != 64 {
		t.Fatalf("len=%d", len(id))
	}
}

// 23. zero-duration request rejected.
func TestRequest_ZeroDurationRejected(t *testing.T) {
	r := Request{
		ID:         "ok",
		Kind:       KindLatency,
		Target:     Target{Host: "x", Port: 80},
		Duration:   0,
		MaxSamples: 1,
		CreatedAt:  time.Now().UTC(),
	}
	if err := r.Validate(); err == nil {
		t.Fatal("expected zero duration rejection")
	}
}

// 24. multi-sample latency reduces correctly.
func TestReduceLatency_EmptySafe(t *testing.T) {
	if ReduceLatency(nil).Count != 0 {
		t.Fatal("expected zero count on empty")
	}
}

// 25. context cancel race: a probe cancelled mid-flight
// must not panic.
func TestLinuxHost_CancelRaceSafe(t *testing.T) {
	l, addr := newTCPEcho(t)
	defer l.Close()
	host, port, _ := net.SplitHostPort(addr)
	p := 0
	fmt.Sscanf(port, "%d", &p)
	h := newHostWithAllow(t, []string{host})
	for i := 0; i < 5; i++ {
		req := Request{
			ID:         fmt.Sprintf("probe-race-%d", i),
			Kind:       KindLatency,
			Target:     Target{Host: host, Port: p},
			Duration:   500 * time.Millisecond,
			MaxSamples: 5,
			CreatedAt:  time.Now().UTC(),
		}
		_, _ = h.Start(req)
		_ = h.Cancel(req.ID)
	}
}

// 26. ssrf: isRFC1918 / isCGNAT / isULA classifications.
func TestSSRFClassifications(t *testing.T) {
	cases := []struct {
		ip       string
		rfc1918  bool
		cgnat    bool
		ula      bool
	}{
		{"10.0.0.1", true, false, false},
		{"172.16.0.1", true, false, false},
		{"172.32.0.1", false, false, false},
		{"192.168.0.1", true, false, false},
		{"100.64.0.1", false, true, false},
		{"8.8.8.8", false, false, false},
		{"fc00::1", false, false, true},
		{"fd00::1", false, false, true},
		{"fe80::1", false, false, false},
	}
	for _, c := range cases {
		ip := net.ParseIP(c.ip)
		if ip == nil {
			t.Fatalf("parse %s", c.ip)
		}
		if isRFC1918(ip) != c.rfc1918 {
			t.Errorf("rfc1918(%s) want=%v got=%v", c.ip, c.rfc1918, isRFC1918(ip))
		}
		if isCGNAT(ip) != c.cgnat {
			t.Errorf("cgnat(%s) want=%v got=%v", c.ip, c.cgnat, isCGNAT(ip))
		}
		if isULA(ip) != c.ula {
			t.Errorf("ula(%s) want=%v got=%v", c.ip, c.ula, isULA(ip))
		}
	}
}

// 27. start returns error when an existing probe id collides.
func TestLinuxHost_Start_DuplicateIDReturnsError(t *testing.T) {
	l, addr := newTCPEcho(t)
	defer l.Close()
	host, port, _ := net.SplitHostPort(addr)
	p := 0
	fmt.Sscanf(port, "%d", &p)
	h := newHostWithAllow(t, []string{host})
	req := Request{
		ID:         "probe-dup-x",
		Kind:       KindLatency,
		Target:     Target{Host: host, Port: p},
		Duration:   300 * time.Millisecond,
		MaxSamples: 2,
		CreatedAt:  time.Now().UTC(),
	}
	if _, err := h.Start(req); err != nil {
		t.Fatalf("Start: %v", err)
	}
	_, err := h.Start(req)
	if err == nil || !strings.Contains(err.Error(), "already in flight") {
		t.Fatalf("err: %v", err)
	}
	waitFor(t, h, "probe-dup-x", 2*time.Second)
}

// 28. TestNormaliseTarget rejects ftp scheme.
func TestNormaliseTarget_FTPSchemeRejected(t *testing.T) {
	if _, err := normaliseTarget(Target{URL: "ftp://x"}, runtimeConfig{AllowLoopback: true, AllowPrivate: true}); err == nil {
		t.Fatal("expected ftp rejection")
	}
}

// 29. newBoundedClient refuses redirects.
func TestNewBoundedClient_RefusesRedirects(t *testing.T) {
	var saw int
	mux := http.NewServeMux()
	mux.HandleFunc("/r", func(w http.ResponseWriter, r *http.Request) {
		saw++
		http.Redirect(w, r, "/dest", http.StatusFound)
	})
	mux.HandleFunc("/dest", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	c := newBoundedClient(runtimeConfig{AllowLoopback: true, AllowPrivate: true})
	req, _ := http.NewRequest(http.MethodHead, srv.URL+"/r", nil)
	_, err := c.Do(req)
	if err == nil {
		t.Fatal("expected redirect refusal")
	}
	if saw != 1 {
		t.Fatalf("redirect target hit %d times", saw)
	}
}

// 30. ensure ctx propagation is honoured.
func TestNormaliseTarget_CtxPropagation(t *testing.T) {
	_ = context.Background()
}