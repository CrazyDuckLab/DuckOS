// measure/measure.go: G20 measurement quality / history.
//
// A measurement is one timestamped sample of a network
// quantity (latency, jitter, loss, throughput). The Store
// keeps a bounded history per metric so the UI can render
// "last 10 minutes of WAN latency" without re-probing.
//
// Every measurement carries:
//   - Quality: a normalised 0..1 score the producer
//     attached (e.g. percentile confidence, sample
//     distribution width). 1.0 = trusted, 0.0 = suspect.
//   - Method: how the sample was collected (probe,
//     passive capture, external telemetry).
//
// The store never grows unbounded. Per-metric retention
// is bounded by MaxSamples (default 512). Per-metric age
// is bounded by MaxAge (default 24h). The GCExpired
// method removes stale metrics on demand.
package measure

import (
	"errors"
	"regexp"
	"sort"
	"sync"
	"time"
)

// SchemaVersion is the on-disk schema version.
const SchemaVersion = 1

// MaxSamples is the per-metric retention ceiling.
const MaxSamples = 512

// MaxAge is the per-sample age ceiling.
const MaxAge = 24 * time.Hour

// Method enumerates the canonical measurement methods.
type Method string

const (
	MethodProbe    Method = "probe"
	MethodPassive  Method = "passive"
	MethodExternal Method = "external"
)

// Valid reports whether m is a known method.
func (m Method) Valid() bool {
	switch m {
	case MethodProbe, MethodPassive, MethodExternal:
		return true
	}
	return false
}

// Sample is one timestamped measurement.
type Sample struct {
	// Metric is the canonical metric id (e.g. "wan.latency",
	// "dns.lookup-time"). Dot-separated namespaces are
	// allowed.
	Metric string `json:"metric"`
	// Value is the measured quantity in the unit
	// declared on the sample.
	Value float64 `json:"value"`
	// Unit is the measurement unit ("ms", "pct", "pps").
	Unit string `json:"unit"`
	// Quality is the producer confidence score [0, 1].
	Quality float64   `json:"quality"`
	Method  Method    `json:"method"`
	At      time.Time `json:"at"`
	// Source is the subsystem that produced the sample.
	Source string `json:"source,omitempty"`
	// Tags are free-form labels for grouping.
	Tags []string `json:"tags,omitempty"`
}

var metricPattern = regexp.MustCompile(`^[a-z][a-z0-9._-]{2,63}$`)

// Validate reports whether s is internally consistent.
func (s Sample) Validate() error {
	if !metricPattern.MatchString(s.Metric) {
		return errors.New("measure: metric is not canonical")
	}
	if !s.Method.Valid() {
		return errors.New("measure: method is not canonical")
	}
	if s.Quality < 0 || s.Quality > 1 {
		return errors.New("measure: quality out of [0,1]")
	}
	if s.At.IsZero() {
		return errors.New("measure: at is zero")
	}
	if len(s.Unit) > 16 {
		return errors.New("measure: unit too long")
	}
	if len(s.Source) > 32 {
		return errors.New("measure: source too long")
	}
	for _, t := range s.Tags {
		if len(t) > 64 {
			return errors.New("measure: tag too long")
		}
	}
	return nil
}

// Store is the goroutine-safe bounded measurement store.
type Store struct {
	mu      sync.RWMutex
	samples map[string][]Sample
	maxPer  int
	maxAge  time.Duration
}

// NewStore returns an empty store.
func NewStore() *Store {
	return &Store{
		samples: map[string][]Sample{},
		maxPer:  MaxSamples,
		maxAge:  MaxAge,
	}
}

// SetLimits overrides the retention caps.
func (s *Store) SetLimits(maxSamples int, maxAge time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if maxSamples > 0 {
		s.maxPer = maxSamples
	}
	if maxAge > 0 {
		s.maxAge = maxAge
	}
}

// Append inserts a sample. The per-metric ring is trimmed
// to maxSamples.
func (s *Store) Append(sample Sample) error {
	if err := sample.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	ring := append(s.samples[sample.Metric], sample)
	if s.maxPer > 0 && len(ring) > s.maxPer {
		ring = ring[len(ring)-s.maxPer:]
	}
	s.samples[sample.Metric] = ring
	return nil
}

// Latest returns the newest sample for the supplied
// metric.
func (s *Store) Latest(metric string) (Sample, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ring := s.samples[metric]
	if len(ring) == 0 {
		return Sample{}, false
	}
	return ring[len(ring)-1], true
}

// History returns the bounded ring for the supplied
// metric, newest first. The slice is a copy.
func (s *Store) History(metric string) []Sample {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ring := s.samples[metric]
	out := make([]Sample, len(ring))
	for i, sample := range ring {
		out[len(ring)-1-i] = sample
	}
	return out
}

// AllMetrics returns every observed metric id sorted.
func (s *Store) AllMetrics() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, 0, len(s.samples))
	for k := range s.samples {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Counters summarises the store.
type Counters struct {
	Metrics int `json:"metrics"`
	Samples int `json:"samples"`
}

// Counters returns the current counts.
func (s *Store) Counters() Counters {
	s.mu.RLock()
	defer s.mu.RUnlock()
	total := 0
	for _, ring := range s.samples {
		total += len(ring)
	}
	return Counters{Metrics: len(s.samples), Samples: total}
}

// GCExpired removes samples older than the configured
// age ceiling. Returns the number of samples removed.
func (s *Store) GCExpired() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	removed := 0
	for k, ring := range s.samples {
		kept := ring[:0]
		for _, sample := range ring {
			if now.Sub(sample.At) <= s.maxAge {
				kept = append(kept, sample)
			} else {
				removed++
			}
		}
		if len(kept) == 0 {
			delete(s.samples, k)
		} else {
			s.samples[k] = kept
		}
	}
	return removed
}

// QualitySummary returns min/max/avg quality across the
// most recent N samples of the supplied metric. N <= 0
// means "all samples".
func (s *Store) QualitySummary(metric string, lastN int) (QualityStats, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ring := s.samples[metric]
	if len(ring) == 0 {
		return QualityStats{}, false
	}
	if lastN <= 0 || lastN > len(ring) {
		lastN = len(ring)
	}
	start := len(ring) - lastN
	min := 2.0
	max := -1.0
	sum := 0.0
	for _, sample := range ring[start:] {
		if sample.Quality < min {
			min = sample.Quality
		}
		if sample.Quality > max {
			max = sample.Quality
		}
		sum += sample.Quality
	}
	return QualityStats{
		Count:   lastN,
		Min:     min,
		Max:     max,
		Average: sum / float64(lastN),
	}, true
}

// QualityStats summarises quality across N samples.
type QualityStats struct {
	Count   int     `json:"count"`
	Min     float64 `json:"min"`
	Max     float64 `json:"max"`
	Average float64 `json:"average"`
}
