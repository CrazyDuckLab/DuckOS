// Package speedlab owns the bounded latency and throughput probe
// the V1 contract requires. A probe is a short, focused
// measurement: the operator picks a target (a host, a port, a
// service), a duration and a sample cap, and the Agent returns
// a summary that is safe to publish and easy to interpret.
//
// The package is split into a portable model layer and a
// platform-specific collector. The model layer enforces the
// bound contract and the redacted summary shape; the collector
// runs the actual measurement on a Linux host. The non-Linux
// collector returns an explicit refusal.
package speedlab

import (
	"errors"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

const SchemaVersion = "1"

// Hard caps. A Speed Lab probe is intentionally cheap: at most
// 30 s, at most 1024 samples, and the resulting summary must
// fit inside a 64 KiB envelope. The probe never opens a raw
// socket; it relies on Go's stdlib resolvers, the regular
// net.Dialer and an HTTP client.
const (
	MaxDuration    = 30 * time.Second
	MaxSamples     = 1024
	MaxBytes       = 64 << 10
	MaxConcurrent  = 2
	MinDuration    = 1 * time.Second
	MaxTargetBytes = 128
	MaxLabelBytes  = 64
	MaxRequestID   = 64
	MaxRTTMicros   = 60 * 1000 * 1000 // 60 s ceiling per sample
)

// Kind enumerates the canonical probe kinds. Unknown kinds
// are rejected at validation time.
type Kind string

const (
	KindLatency    Kind = "latency"    // TCP RTT (handshake)
	KindHTTP       Kind = "http"       // HTTP HEAD RTT
	KindThroughput Kind = "throughput" // sustained HTTP GET throughput
)

// Validate returns the canonical list of legal probe kinds.
func isValidKind(k Kind) bool {
	switch k {
	case KindLatency, KindHTTP, KindThroughput:
		return true
	}
	return false
}

// Target is the operator-facing destination. The Target is a
// host name, IP or URL: the Agent normalises it into a host
// and a port. The Label is a human-friendly alias that ends
// up in the summary; the package redacts the raw target if
// the Label is non-empty.
type Target struct {
	Host  string `json:"host,omitempty"`
	Port  int    `json:"port,omitempty"`
	URL   string `json:"url,omitempty"`
	Label string `json:"label,omitempty"`
}

// Request is the operator-facing input.
type Request struct {
	ID         string        `json:"id"`
	Kind       Kind          `json:"kind"`
	Target     Target        `json:"target"`
	Duration   time.Duration `json:"duration"`
	MaxSamples int           `json:"maxSamples"`
	Operator   string        `json:"operator,omitempty"`
	Reason     string        `json:"reason,omitempty"`
	CreatedAt  time.Time     `json:"createdAt"`
}

// Validate rejects any Request that exceeds the policy cap or
// contains characters that could break out of the audited
// argument vector. The probe is initiated by the Agent, not by
// a shell, so the validation only needs to reject obviously
// bad input.
func (r *Request) Validate() error {
	if r == nil {
		return errors.New("speedlab: request is nil")
	}
	if !idPattern.MatchString(r.ID) {
		return errors.New("speedlab: id is not canonical")
	}
	if !isValidKind(r.Kind) {
		return errors.New("speedlab: kind is not canonical")
	}
	if err := r.Target.validate(); err != nil {
		return err
	}
	if r.Duration < MinDuration || r.Duration > MaxDuration {
		return errors.New("speedlab: duration out of bounds")
	}
	if r.MaxSamples <= 0 || r.MaxSamples > MaxSamples {
		return errors.New("speedlab: maxSamples out of bounds")
	}
	if r.Operator != "" && (len(r.Operator) > 64 || !userPattern.MatchString(r.Operator)) {
		return errors.New("speedlab: operator is not canonical")
	}
	if r.Reason != "" && len(r.Reason) > 256 {
		return errors.New("speedlab: reason exceeds size cap")
	}
	if r.CreatedAt.IsZero() {
		return errors.New("speedlab: createdAt is required")
	}
	return nil
}

var (
	idPattern   = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,63}$`)
	userPattern = regexp.MustCompile(`^[a-z][a-z0-9._@-]{0,63}$`)
	hostPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)
)

func (t *Target) validate() error {
	if t == nil {
		return errors.New("speedlab: target is nil")
	}
	if t.Label != "" && len(t.Label) > MaxLabelBytes {
		return errors.New("speedlab: target label exceeds size cap")
	}
	if t.URL != "" {
		if len(t.URL) > MaxTargetBytes {
			return errors.New("speedlab: target url exceeds size cap")
		}
		if !strings.HasPrefix(t.URL, "http://") && !strings.HasPrefix(t.URL, "https://") {
			return errors.New("speedlab: target url must use http or https")
		}
		return nil
	}
	if t.Host == "" {
		return errors.New("speedlab: target host is required")
	}
	if len(t.Host) > MaxTargetBytes {
		return errors.New("speedlab: target host exceeds size cap")
	}
	if !hostPattern.MatchString(t.Host) {
		return errors.New("speedlab: target host is not canonical")
	}
	if t.Port < 0 || t.Port > 65535 {
		return errors.New("speedlab: target port out of range")
	}
	return nil
}

// Summary is the redacted result of a probe. The raw RTT and
// throughput samples are reduced to percentiles; the raw
// sample list never crosses the API boundary.
type Summary struct {
	ID         string        `json:"id"`
	Kind       Kind          `json:"kind"`
	Target     Target        `json:"target"`
	StartedAt  time.Time     `json:"startedAt"`
	FinishedAt time.Time     `json:"finishedAt"`
	Samples    int           `json:"samples"`
	Duration   time.Duration `json:"duration"`
	Latency    LatencyStats  `json:"latency,omitempty"`
	Throughput ThroughStats  `json:"throughput,omitempty"`
	Status     string        `json:"status"`
	Error      string        `json:"error,omitempty"`
}

// LatencyStats summarises a latency probe. Percentiles are
// computed in microseconds; Min/Max are reported in the same
// unit. The Count is the number of successful samples.
type LatencyStats struct {
	Count int   `json:"count"`
	MinUS int64 `json:"minMicros"`
	AvgUS int64 `json:"avgMicros"`
	MaxUS int64 `json:"maxMicros"`
	P50US int64 `json:"p50Micros"`
	P95US int64 `json:"p95Micros"`
	P99US int64 `json:"p99Micros"`
}

// ThroughStats summarises a throughput probe. Bytes is the
// total payload delivered; BPS is the per-second average.
type ThroughStats struct {
	Bytes   int64 `json:"bytes"`
	Seconds int64 `json:"seconds"`
	BPS     int64 `json:"bytesPerSecond"`
}

// State is the lifecycle of a probe.
type State string

const (
	StatePending State = "pending"
	StateRunning State = "running"
	StateDone    State = "done"
	StateFailed  State = "failed"
)

// Probe is the in-memory state a Host keeps for a single probe.
type Probe struct {
	Request Request
	State   State
	Summary Summary
}

// ActiveRef is what the API returns to a caller that started a
// probe. The Reference can be used to poll status or fetch the
// final summary.
type ActiveRef struct {
	ID        string    `json:"id"`
	State     State     `json:"state"`
	StartedAt time.Time `json:"startedAt,omitempty"`
}

// Host is the contract a platform implementation must satisfy.
// The reference implementation lives in speedlab_linux.go; the
// non-Linux implementation refuses with an explicit reason.
type Host interface {
	Start(Request) (ActiveRef, error)
	Status(id string) (State, Summary, error)
	Cancel(id string) error
	List() []ActiveRef
}

// Errors the Host returns. The API maps each to a stable
// HTTP code; the CLI prints the message.
var (
	ErrBusy        = errors.New("speedlab: too many concurrent probes")
	ErrNotFound    = errors.New("speedlab: not found")
	ErrUnsupported = errors.New("speedlab: not supported on this platform")
)

// Manager owns the in-flight set of Probes and enforces the
// concurrent cap. Manager is goroutine-safe.
type Manager struct {
	mu     sync.Mutex
	host   Host
	active map[string]Probe
}

// NewManager wires a Host to the Manager. A nil Host produces
// a Manager that returns ErrUnsupported for every operation.
func NewManager(host Host) *Manager {
	if host == nil {
		host = &unsupportedHost{}
	}
	return &Manager{host: host, active: map[string]Probe{}}
}

// Start validates the Request, registers a pending probe and
// delegates to the Host. The call is non-blocking: the Host
// owns the actual lifecycle.
func (m *Manager) Start(req Request) (ActiveRef, error) {
	if err := req.Validate(); err != nil {
		return ActiveRef{}, err
	}
	m.mu.Lock()
	if len(m.active) >= MaxConcurrent {
		m.mu.Unlock()
		return ActiveRef{}, ErrBusy
	}
	now := time.Now().UTC()
	if req.CreatedAt.IsZero() {
		req.CreatedAt = now
	}
	probe := Probe{Request: req, State: StatePending, Summary: Summary{ID: req.ID, Kind: req.Kind, Target: req.Target, Status: string(StatePending)}}
	m.active[req.ID] = probe
	m.mu.Unlock()
	ref, err := m.host.Start(req)
	if err != nil {
		m.mu.Lock()
		delete(m.active, req.ID)
		m.mu.Unlock()
		return ActiveRef{}, err
	}
	m.mu.Lock()
	probe.State = StateRunning
	probe.Summary.StartedAt = ref.StartedAt
	probe.Summary.Status = string(StateRunning)
	m.active[req.ID] = probe
	m.mu.Unlock()
	return ref, nil
}

// Status returns the current State and Summary of an active
// probe. A probe that is no longer in the active set is
// reported through the Host (which owns the persistent
// summary).
func (m *Manager) Status(id string) (State, Summary, error) {
	m.mu.Lock()
	probe, ok := m.active[id]
	m.mu.Unlock()
	if !ok {
		return m.host.Status(id)
	}
	return probe.State, probe.Summary, nil
}

// Cancel stops an in-flight probe.
func (m *Manager) Cancel(id string) error {
	if err := m.host.Cancel(id); err != nil {
		return err
	}
	m.mu.Lock()
	if probe, ok := m.active[id]; ok {
		probe.State = StateFailed
		probe.Summary.Status = string(StateFailed)
		probe.Summary.Error = "cancelled"
		probe.Summary.FinishedAt = time.Now().UTC()
		delete(m.active, id)
	}
	m.mu.Unlock()
	return nil
}

// List returns the ActiveRef of every in-flight probe. The
// returned slice is a copy.
func (m *Manager) List() []ActiveRef {
	m.mu.Lock()
	out := make([]ActiveRef, 0, len(m.active))
	for _, p := range m.active {
		out = append(out, ActiveRef{ID: p.Request.ID, State: p.State, StartedAt: p.Summary.StartedAt})
	}
	m.mu.Unlock()
	return out
}

// Finalize is called by the Host when a probe completes. The
// Manager updates the local state, removes the probe from the
// in-flight set and returns the final summary to the caller.
func (m *Manager) Finalize(id string, summary Summary) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.active[id]; !ok {
		return
	}
	summary.Status = string(StateDone)
	summary.FinishedAt = time.Now().UTC()
	delete(m.active, id)
}

type unsupportedHost struct{}

func (unsupportedHost) Start(Request) (ActiveRef, error) { return ActiveRef{}, ErrUnsupported }
func (unsupportedHost) Status(string) (State, Summary, error) {
	return StateFailed, Summary{}, ErrUnsupported
}
func (unsupportedHost) Cancel(string) error { return ErrUnsupported }
func (unsupportedHost) List() []ActiveRef   { return nil }

// ReduceLatency computes a LatencyStats from a list of RTT
// samples in microseconds. The function never panics on empty
// or out-of-range input.
func ReduceLatency(samples []int64) LatencyStats {
	if len(samples) == 0 {
		return LatencyStats{}
	}
	clean := make([]int64, 0, len(samples))
	var sum int64
	for _, s := range samples {
		if s < 0 || s > MaxRTTMicros {
			continue
		}
		clean = append(clean, s)
		sum += s
	}
	if len(clean) == 0 {
		return LatencyStats{}
	}
	sort.Slice(clean, func(i, j int) bool { return clean[i] < clean[j] })
	return LatencyStats{
		Count: len(clean),
		MinUS: clean[0],
		AvgUS: sum / int64(len(clean)),
		MaxUS: clean[len(clean)-1],
		P50US: percentile(clean, 50),
		P95US: percentile(clean, 95),
		P99US: percentile(clean, 99),
	}
}

// percentile returns the value at the p-th percentile of an
// already-sorted slice using nearest-rank.
func percentile(sorted []int64, p int) int64 {
	if len(sorted) == 0 {
		return 0
	}
	if p <= 0 {
		return sorted[0]
	}
	if p >= 100 {
		return sorted[len(sorted)-1]
	}
	idx := (p * (len(sorted) - 1)) / 100
	return sorted[idx]
}
