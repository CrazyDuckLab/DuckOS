// chaos/readiness_probes.go: G04 agent write side.
//
// The Agent composition root instantiates a
// ReadinessRecorder, registers probes for every feature it
// can classify, and runs the recorder at startup. Probes
// are pure (no agent dependencies), deterministic, and
// time-bounded. The Recorder writes the results into the
// shared *ReadinessStore that the API exposes through the
// Engine.
//
// Honest classification is the rule: if a probe cannot
// prove the feature is "supported" with real evidence, the
// row is recorded as "hardware-pending", "credential-pending",
// "experimental", or "unsupported" with a stable reason.
// Probes never fabricate success.
package chaos

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"
)

// ReadinessProbe is a single deterministic classification
// rule. Implementations must not mutate system state and
// must respect ctx cancellation. The feature name is the
// canonical key (e.g., "network.firewall", "vpn.wireguard").
type ReadinessProbe interface {
	// Feature returns the canonical feature key. Must be
	// non-empty and stable across calls.
	Feature() string
	// Probe runs the classification. It must return a
	// valid Readiness state and a stable Note string.
	// The recorder validates the state via Readiness.Valid.
	Probe(ctx context.Context) (state Readiness, note string, err error)
}

// ProbeFunc adapts a plain function to ReadinessProbe for
// short-lived test probes.
type ProbeFunc struct {
	Name string
	Run  func(context.Context) (Readiness, string, error)
}

// Feature returns the configured feature key.
func (p ProbeFunc) Feature() string { return p.Name }

// Probe invokes the wrapped function.
func (p ProbeFunc) Probe(ctx context.Context) (Readiness, string, error) {
	if p.Run == nil {
		return "", "", errors.New("chaos: probe function is nil")
	}
	return p.Run(ctx)
}

// ReadinessRecorder collects probes and writes their
// classifications into a ReadinessStore. The recorder is
// the G04 agent write side: the Engine only reads.
type ReadinessRecorder struct {
	mu       sync.Mutex
	store    *ReadinessStore
	probes   []ReadinessProbe
	timeout  time.Duration
	maxNotes int
}

// NewReadinessRecorder wraps store. A nil store is replaced
// with an empty store so callers may pass nil during early
// boot. Default timeout is 2 seconds per probe.
func NewReadinessRecorder(store *ReadinessStore) *ReadinessRecorder {
	if store == nil {
		store = NewReadinessStore()
	}
	return &ReadinessRecorder{store: store, timeout: 2 * time.Second, maxNotes: 256}
}

// SetMaxNotes overrides the per-note truncation ceiling.
// A non-positive value resets to the 256 default.
func (r *ReadinessRecorder) SetMaxNotes(n int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if n <= 0 {
		n = 256
	}
	r.maxNotes = n
}

// SetTimeout overrides the per-probe timeout. A non-positive
// value resets to the 2s default.
func (r *ReadinessRecorder) SetTimeout(d time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if d <= 0 {
		d = 2 * time.Second
	}
	r.timeout = d
}

// Register adds a probe to the recorder. Duplicate feature
// names replace the earlier registration so callers can
// override test stubs in the composition root.
func (r *ReadinessRecorder) Register(p ReadinessProbe) {
	if p == nil || p.Feature() == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	replaced := false
	for i, existing := range r.probes {
		if existing.Feature() == p.Feature() {
			r.probes[i] = p
			replaced = true
			break
		}
	}
	if !replaced {
		r.probes = append(r.probes, p)
	}
}

// Features returns the registered feature names sorted.
func (r *ReadinessRecorder) Features() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, 0, len(r.probes))
	for _, p := range r.probes {
		out = append(out, p.Feature())
	}
	sort.Strings(out)
	return out
}

// Store returns the underlying readiness store. Callers
// must treat the store as read-only.
func (r *ReadinessRecorder) Store() *ReadinessStore {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.store
}

// Run executes every registered probe and writes the
// classification into the store. Probes that fail or panic
// are recorded as "degraded" with the failure reason.
func (r *ReadinessRecorder) Run(ctx context.Context) error {
	r.mu.Lock()
	probes := make([]ReadinessProbe, len(r.probes))
	copy(probes, r.probes)
	timeout := r.timeout
	r.mu.Unlock()

	for _, p := range probes {
		if p == nil {
			continue
		}
		probeCtx, cancel := context.WithTimeout(ctx, timeout)
		state, note, err := safeProbe(probeCtx, p)
		cancel()
		if err != nil {
			state = ReadyDegraded
			note = truncateNote(fmt.Sprintf("probe failed: %v", err), r.maxNotes)
		} else if !state.Valid() {
			state = ReadyUnsupported
			note = truncateNote(fmt.Sprintf("probe returned invalid state: %q", state), r.maxNotes)
		} else {
			note = truncateNote(note, r.maxNotes)
		}
		r.store.Set(FeatureReadiness{
			Feature: p.Feature(),
			State:   state,
			Note:    note,
		})
	}
	return nil
}

// safeProbe invokes the probe inside a recover guard so a
// panicking probe cannot crash the Agent.
func safeProbe(ctx context.Context, p ReadinessProbe) (state Readiness, note string, err error) {
	defer func() {
		if rec := recover(); rec != nil {
			state = ""
			note = ""
			err = fmt.Errorf("chaos: probe %q panicked: %v", p.Feature(), rec)
		}
	}()
	return p.Probe(ctx)
}

func truncateNote(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max]
}
