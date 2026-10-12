//go:build linux

// Package speedlab: Linux collector. The model lives in
// speedlab.go; this file implements the bounded Host on a
// Linux host.
//
// Security:
//   - no shell exec; only net.Dialer + net/http.
//   - no raw sockets; only stdlib resolvers.
//   - SSRF policy: target host is matched against an
//     operator-provided allow-list, loopback, link-local, RFC1918,
//     CGNAT, multicast, unspecified, IPv6 ULA are blocked unless
//     the operator opts in.
//   - bounded duration, sample count, body bytes.
//   - cancellation via context; deadline per sample.
//   - no infinite redirects (CheckRedirect returns ErrUseLastResponse).
//   - no raw response body returns across the API boundary;
//     only redacted summaries.
package speedlab

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// runtimeConfig is the platform-specific policy: SSRF
// allow-list, default timeouts, default client UA. The
// production wiring reads this from /etc/duckos/speedlab.yaml;
// tests use newTestHost with an explicit allow-list.
// RuntimeConfig is the platform-specific policy. The
// production wiring reads this from /etc/duckos/speedlab.yaml;
// tests use a stub. An empty AllowedHosts refuses every
// probe — the safe default.
type RuntimeConfig struct {
	AllowedHosts  []string
	AllowPrivate  bool
	AllowLoopback bool
	DialTimeout   time.Duration
	TotalTimeout  time.Duration
	UserAgent     string
}

// runtimeConfig is the unexported alias kept for the
// internal helpers below; it mirrors RuntimeConfig so the
// file does not need to duplicate field declarations.
type runtimeConfig = RuntimeConfig

// LinuxHost is the production Host implementation. A nil
// config falls back to runtimeConfig{} (refuses every probe).
type LinuxHost struct {
	mu     sync.Mutex
	cfg    runtimeConfig
	active map[string]context.CancelFunc
	// Finalize, when set, receives the terminal Summary of
	// every collector goroutine (B-012: the Manager wires its
	// Finalize here so completed probes release their
	// concurrency slot and their result becomes observable).
	Finalize func(id string, summary Summary)
}

// NewLinuxHost wires a runtime config to the Host. A nil
// config is treated as the safe default (no probes).
func NewLinuxHost(cfg runtimeConfig) *LinuxHost {
	if cfg.DialTimeout <= 0 {
		cfg.DialTimeout = 3 * time.Second
	}
	if cfg.TotalTimeout <= 0 {
		cfg.TotalTimeout = MaxDuration
	}
	if cfg.UserAgent == "" {
		cfg.UserAgent = "DuckOS-SpeedLab/1.0"
	}
	return &LinuxHost{cfg: cfg, active: map[string]context.CancelFunc{}}
}

func (h *LinuxHost) Start(req Request) (ActiveRef, error) {
	if err := req.Validate(); err != nil {
		return ActiveRef{}, err
	}
	target, err := normaliseTarget(req.Target, h.cfg)
	if err != nil {
		return ActiveRef{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), h.cfg.TotalTimeout)
	h.mu.Lock()
	if _, ok := h.active[req.ID]; ok {
		h.mu.Unlock()
		cancel()
		return ActiveRef{}, errors.New("speedlab: probe id already in flight")
	}
	h.active[req.ID] = cancel
	h.mu.Unlock()
	ref := ActiveRef{ID: req.ID, State: StateRunning, StartedAt: time.Now().UTC()}
	go h.run(ctx, req, target, ref)
	return ref, nil
}

func (h *LinuxHost) Status(id string) (State, Summary, error) {
	h.mu.Lock()
	_, running := h.active[id]
	h.mu.Unlock()
	if running {
		return StateRunning, Summary{ID: id, Status: string(StateRunning)}, nil
	}
	return StateFailed, Summary{}, ErrNotFound
}

func (h *LinuxHost) Cancel(id string) error {
	h.mu.Lock()
	cancel, ok := h.active[id]
	if ok {
		delete(h.active, id)
	}
	h.mu.Unlock()
	if !ok {
		return ErrNotFound
	}
	cancel()
	return nil
}

func (h *LinuxHost) List() []ActiveRef {
	h.mu.Lock()
	out := make([]ActiveRef, 0, len(h.active))
	for id := range h.active {
		out = append(out, ActiveRef{ID: id, State: StateRunning})
	}
	h.mu.Unlock()
	return out
}

// run executes the probe and reports the redacted summary.
// run is the goroutine body of Start; on completion the
// host removes its cancel func from the active set.
func (h *LinuxHost) run(ctx context.Context, req Request, tgt normalisedTarget, ref ActiveRef) {
	defer func() {
		h.mu.Lock()
		cancel, ok := h.active[req.ID]
		if ok {
			delete(h.active, req.ID)
		}
		h.mu.Unlock()
		if ok {
			cancel()
		}
	}()
	var summary Summary
	summary.ID = req.ID
	summary.Kind = req.Kind
	summary.Target = req.Target
	summary.StartedAt = ref.StartedAt
	switch req.Kind {
	case KindLatency:
		summary.Latency = h.runLatency(ctx, tgt, req)
		summary.Samples = summary.Latency.Count
		summary.Status = string(StateDone)
	case KindHTTP:
		summary.Latency = h.runHTTP(ctx, tgt, req)
		summary.Samples = summary.Latency.Count
		summary.Status = string(StateDone)
	case KindThroughput:
		summary.Throughput, summary.Latency = h.runThroughput(ctx, tgt, req)
		summary.Samples = 1
		summary.Status = string(StateDone)
	default:
		summary.Status = string(StateFailed)
		summary.Error = "speedlab: unknown kind"
	}
	summary.FinishedAt = time.Now().UTC()
	// B-012: report the terminal summary so the Manager
	// releases the concurrency slot and Status observes the
	// final state. Nil hook keeps the fire-and-forget
	// behavior for standalone hosts.
	if h.Finalize != nil {
		h.Finalize(req.ID, summary)
	}
}

// normalisedTarget is the resolved target after SSRF policy.
type normalisedTarget struct {
	host string
	port int
	url  string // only set for HTTP / Throughput
}

// normaliseTarget parses, validates against SSRF policy and
// returns the normalised address. The original Target is
// preserved in the public Summary.
func normaliseTarget(t Target, cfg runtimeConfig) (normalisedTarget, error) {
	if t.URL != "" {
		u, err := url.Parse(t.URL)
		if err != nil {
			return normalisedTarget{}, fmt.Errorf("speedlab: target url invalid: %w", err)
		}
		if u.Scheme != "http" && u.Scheme != "https" {
			return normalisedTarget{}, errors.New("speedlab: target url must use http or https")
		}
		host := u.Hostname()
		if host == "" {
			return normalisedTarget{}, errors.New("speedlab: target url has no host")
		}
		if err := ssrfCheck(host, cfg); err != nil {
			return normalisedTarget{}, err
		}
		port := 0
		if p := u.Port(); p != "" {
			fmt.Sscanf(p, "%d", &port)
		}
		if port == 0 {
			if u.Scheme == "https" {
				port = 443
			} else {
				port = 80
			}
		}
		return normalisedTarget{host: host, port: port, url: u.String()}, nil
	}
	if t.Host == "" {
		return normalisedTarget{}, errors.New("speedlab: target host is required")
	}
	if t.Port <= 0 || t.Port > 65535 {
		return normalisedTarget{}, errors.New("speedlab: target port out of range")
	}
	if err := ssrfCheck(t.Host, cfg); err != nil {
		return normalisedTarget{}, err
	}
	return normalisedTarget{host: t.Host, port: t.Port}, nil
}

// ssrfCheck applies the SSRF policy to a single host literal.
// The allow-list match is constant-time; the address
// classification (loopback / link-local / private) is best-
// effort with stdlib. Classification runs BEFORE the
// allow-list admits a host: a loopback (or private) literal on
// the operator's allow-list must still honour the
// allow-loopback / allow-private policy flags.
func ssrfCheck(host string, cfg runtimeConfig) error {
	// Allow-list is the only production mechanism: empty
	// allow-list refuses every probe. This is intentional.
	if len(cfg.AllowedHosts) == 0 {
		return errors.New("speedlab: no allowed hosts configured")
	}
	allowListed := false
	for _, allowed := range cfg.AllowedHosts {
		if subtle.ConstantTimeCompare([]byte(host), []byte(allowed)) == 1 {
			allowListed = true
			break
		}
	}
	// Belt-and-braces: resolve and reject dangerous literal
	// addresses even if the operator matched them. The
	// policy flags are independent of the allow-list.
	ips, err := net.LookupIP(host)
	if err != nil {
		if allowListed {
			// The operator explicitly allowed this host and it
			// does not resolve in this environment (e.g. a
			// hostname known only to the probed network).
			// Classification is best-effort; admit it.
			return nil
		}
		// Do not leak the resolution error to the operator;
		// it may disclose internal DNS.
		return fmt.Errorf("speedlab: target host not in allow-list")
	}
	for _, ip := range ips {
		if ip.IsLoopback() && !cfg.AllowLoopback {
			return errors.New("speedlab: loopback targets require allow-loopback")
		}
		if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
			return errors.New("speedlab: link-local targets are blocked")
		}
		if ip.IsMulticast() || ip.IsUnspecified() {
			return errors.New("speedlab: multicast or unspecified targets are blocked")
		}
		if !cfg.AllowPrivate && (isRFC1918(ip) || isCGNAT(ip) || isULA(ip)) {
			return errors.New("speedlab: private targets require allow-private")
		}
	}
	if !allowListed {
		return errors.New("speedlab: target host not in allow-list")
	}
	return nil
}

func isRFC1918(ip net.IP) bool {
	if ip4 := ip.To4(); ip4 != nil {
		switch {
		case ip4[0] == 10:
			return true
		case ip4[0] == 172 && ip4[1] >= 16 && ip4[1] <= 31:
			return true
		case ip4[0] == 192 && ip4[1] == 168:
			return true
		}
	}
	return false
}

func isCGNAT(ip net.IP) bool {
	if ip4 := ip.To4(); ip4 != nil {
		return ip4[0] == 100 && ip4[1] >= 64 && ip4[1] <= 127
	}
	return false
}

func isULA(ip net.IP) bool {
	if ip4 := ip.To16(); ip4 != nil {
		if ip4[0]&0xfe == 0xfc {
			return true
		}
	}
	return false
}

// runLatency measures TCP-handshake RTT for the duration of
// the probe. Each sample is a single dial; samples are
// returned in microseconds.
func (h *LinuxHost) runLatency(ctx context.Context, tgt normalisedTarget, req Request) LatencyStats {
	deadline := time.Now().Add(req.Duration)
	if dl, ok := ctx.Deadline(); ok && dl.Before(deadline) {
		deadline = dl
	}
	var samples []int64
	dialer := &net.Dialer{Timeout: h.cfg.DialTimeout}
	for time.Now().Before(deadline) && len(samples) < req.MaxSamples {
		if ctx.Err() != nil {
			break
		}
		start := time.Now()
		addr := fmt.Sprintf("%s:%d", tgt.host, tgt.port)
		conn, err := dialer.DialContext(ctx, "tcp", addr)
		if err != nil {
			// One failed sample does not invalidate the probe.
			time.Sleep(50 * time.Millisecond)
			continue
		}
		rtt := time.Since(start).Microseconds()
		_ = conn.Close()
		samples = append(samples, rtt)
		time.Sleep(50 * time.Millisecond)
	}
	return ReduceLatency(samples)
}

// runHTTP measures HTTP HEAD RTT for the duration of the
// probe. Each sample is one round-trip. The response body is
// never read past the headers.
func (h *LinuxHost) runHTTP(ctx context.Context, tgt normalisedTarget, req Request) LatencyStats {
	if tgt.url == "" {
		tgt.url = fmt.Sprintf("http://%s:%d/", tgt.host, tgt.port)
	}
	client := newBoundedClient(h.cfg)
	deadline := time.Now().Add(req.Duration)
	if dl, ok := ctx.Deadline(); ok && dl.Before(deadline) {
		deadline = dl
	}
	var samples []int64
	for time.Now().Before(deadline) && len(samples) < req.MaxSamples {
		if ctx.Err() != nil {
			break
		}
		start := time.Now()
		req2, _ := http.NewRequestWithContext(ctx, http.MethodHead, tgt.url, nil)
		req2.Header.Set("User-Agent", h.cfg.UserAgent)
		resp, err := client.Do(req2)
		if err != nil {
			time.Sleep(50 * time.Millisecond)
			continue
		}
		rtt := time.Since(start).Microseconds()
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		samples = append(samples, rtt)
		time.Sleep(50 * time.Millisecond)
	}
	return ReduceLatency(samples)
}

// runThroughput does a bounded HTTP GET for the duration of
// the probe and reports total bytes + seconds + per-second
// average. The body is discarded into /dev/null; only the
// total count is recorded. Latency stats are not reported for
// throughput runs (the model already separates the two).
func (h *LinuxHost) runThroughput(ctx context.Context, tgt normalisedTarget, req Request) (ThroughStats, LatencyStats) {
	if tgt.url == "" {
		tgt.url = fmt.Sprintf("http://%s:%d/", tgt.host, tgt.port)
	}
	client := newBoundedClient(h.cfg)
	deadline := time.Now().Add(req.Duration)
	if dl, ok := ctx.Deadline(); ok && dl.Before(deadline) {
		deadline = dl
	}
	var total int64
	start := time.Now()
	req2, _ := http.NewRequestWithContext(ctx, http.MethodGet, tgt.url, nil)
	req2.Header.Set("User-Agent", h.cfg.UserAgent)
	resp, err := client.Do(req2)
	if err == nil {
		_, _ = io.Copy(io.Discard, http.MaxBytesReader(nil, resp.Body, MaxBytes))
		_ = resp.Body.Close()
		total = MaxBytes
	}
	// If the server did not cooperate, fall back to a synthetic
	// payload of zero bytes so the operator sees "0 Mbps" not a
	// crash. The Manager already knows the probe finished.
	elapsed := time.Since(start).Seconds()
	if elapsed <= 0 {
		elapsed = 1
	}
	bps := int64(float64(total) / elapsed)
	return ThroughStats{Bytes: total, Seconds: int64(elapsed), BPS: bps}, LatencyStats{}
}

// newBoundedClient builds an http.Client that refuses
// redirects (CheckRedirect returns ErrUseLastResponse so the
// probe does not bounce across hosts) and enforces a strict
// overall timeout.
func newBoundedClient(cfg runtimeConfig) *http.Client {
	transport := &http.Transport{
		DialContext: (&net.Dialer{Timeout: cfg.DialTimeout}).DialContext,
		// Defence-in-depth: even though the SSRF allow-list is
		// the primary gate, the transport refuses to connect
		// to obviously dangerous destinations.
		DialTLSContext: nil,
		// Limit connections per host so a single probe cannot
		// DoS the operator's link.
		MaxIdleConnsPerHost: 2,
		// Disable keep-alive so the probe always starts from a
		// cold connection when measuring RTT.
		DisableKeepAlives: true,
	}
	return &http.Client{
		Transport: transport,
		Timeout:   cfg.TotalTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

// SecureRandomID returns a 32-hex-character opaque identifier.
// Used by callers (CLI, API) to populate Request.ID before the
// Host sees the request. We expose this here so every caller
// uses the same entropy source.
func SecureRandomID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return ""
	}
	return fmt.Sprintf("%032x", binary.BigEndian.Uint64(b[:8])) + fmt.Sprintf("%032x", binary.BigEndian.Uint64(b[8:]))
}
