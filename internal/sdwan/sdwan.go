// sdwan/sdwan.go: V1 SD-WAN policy layer.
// The package owns the typed contract for
// per-application path selection, the
// criteria (latency / jitter / loss /
// reachability / cost / preference /
// tunnel / source VRF / destination),
// the path scoring, the brownout
// detection, the failover / failback with
// hysteresis and hold-down, and the
// per-policy health.
//
// The runtime is a thin mapping over the
// existing internal/multiwan + ipsla
// packages.
package sdwan

import (
	"errors"
	"regexp"
	"sort"
	"sync"
)

// Criteria is the path-scoring dimension.
type Criteria string

const (
	CriterionLatency      Criteria = "latency"
	CriterionJitter       Criteria = "jitter"
	CriterionLoss         Criteria = "loss"
	CriterionReachability Criteria = "reachability"
	CriterionCost         Criteria = "cost"
	CriterionPreference   Criteria = "preference"
)

// Valid reports whether c is known.
func (c Criteria) Valid() bool {
	switch c {
	case CriterionLatency, CriterionJitter, CriterionLoss,
		CriterionReachability, CriterionCost, CriterionPreference:
		return true
	}
	return false
}

// Weight is the per-criterion weight.
type Weight struct {
	Criteria Criteria `json:"criteria"`
	Weight   int      `json:"weight"`
}

// Valid reports whether w is well-formed.
func (w Weight) Valid() error {
	if !w.Criteria.Valid() {
		return errors.New("sdwan: criteria is unknown")
	}
	if w.Weight < 0 || w.Weight > 100 {
		return errors.New("sdwan: weight must be 0..100")
	}
	return nil
}

// ApplicationClass is the application
// profile that selects a path.
type ApplicationClass struct {
	Name string `json:"name"`
	// DSCP is the DSCP marking.
	DSCP int `json:"dscp,omitempty"`
	// DestinationCIDR is the optional
	// match filter.
	DestinationCIDR string `json:"destinationCidr,omitempty"`
	// SourceVRF is the optional source VRF
	// filter.
	SourceVRF string `json:"sourceVrf,omitempty"`
}

// Valid reports whether a is well-formed.
func (a ApplicationClass) Valid() error {
	if a.Name == "" {
		return errors.New("sdwan: application class name is required")
	}
	if a.DSCP < 0 || a.DSCP > 63 {
		return errors.New("sdwan: DSCP out of range")
	}
	return nil
}

// Path is a candidate egress path.
type Path struct {
	ID          string `json:"id"`
	Description string `json:"description,omitempty"`
	// WAN is the egress WAN id (matches
	// internal/multiwan).
	WAN string `json:"wan"`
	// Tunnel is the optional tunnel id.
	Tunnel     string `json:"tunnel,omitempty"`
	Cost       int    `json:"cost,omitempty"`       // 0..1000
	Preference int    `json:"preference,omitempty"` // higher = preferred
}

// Valid reports whether p is well-formed.
func (p Path) Valid() error {
	if p.ID == "" {
		return errors.New("sdwan: path id is required")
	}
	if p.WAN == "" {
		return errors.New("sdwan: wan is required")
	}
	if p.Cost < 0 {
		return errors.New("sdwan: cost must be non-negative")
	}
	return nil
}

// Policy is the SD-WAN policy.
type Policy struct {
	ID               string           `json:"id"`
	Name             string           `json:"name"`
	ApplicationClass ApplicationClass `json:"applicationClass"`
	Paths            []Path           `json:"paths"`
	Weights          []Weight         `json:"weights"`
	// HoldDown is the minimum time the
	// policy must remain on a path before
	// flipping.
	HoldDownSec int `json:"holdDownSec,omitempty"`
	// HysteresisMs is the score gap required
	// for a path flip.
	HysteresisMs float64 `json:"hysteresisMs,omitempty"`
}

var idPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:-]{0,63}$`)

// Valid reports whether p is well-formed.
func (p Policy) Valid() error {
	if !idPattern.MatchString(p.ID) {
		return errors.New("sdwan: policy id is not canonical")
	}
	if err := p.ApplicationClass.Valid(); err != nil {
		return err
	}
	if len(p.Paths) == 0 {
		return errors.New("sdwan: at least one path is required")
	}
	for _, pa := range p.Paths {
		if err := pa.Valid(); err != nil {
			return err
		}
	}
	for _, w := range p.Weights {
		if err := w.Valid(); err != nil {
			return err
		}
	}
	if p.HoldDownSec < 0 || p.HoldDownSec > 3600 {
		return errors.New("sdwan: holdDownSec must be 0..3600")
	}
	if p.HysteresisMs < 0 {
		return errors.New("sdwan: hysteresisMs must be non-negative")
	}
	return nil
}

// PathScore is the per-path score input.
type PathScore struct {
	PathID     string  `json:"pathId"`
	LatencyMs  float64 `json:"latencyMs"`
	JitterMs   float64 `json:"jitterMs"`
	LossPct    float64 `json:"lossPct"`
	Reachable  bool    `json:"reachable"`
	Cost       int     `json:"cost"`
	Preference int     `json:"preference"`
}

// Brownout is the brownout detection
// output: a path is "brownout" when it is
// reachable but degraded (loss / latency
// / jitter above the policy threshold).
type Brownout struct {
	Brownout bool   `json:"brownout"`
	Reason   string `json:"reason,omitempty"`
}

// DetectBrownout reports whether the path
// is in brownout. The thresholds are
// expressed in absolute units.
func DetectBrownout(s PathScore, latencyTh float64, jitterTh float64, lossTh float64) Brownout {
	if !s.Reachable {
		return Brownout{Brownout: true, Reason: "unreachable"}
	}
	if s.LatencyMs > latencyTh {
		return Brownout{Brownout: true, Reason: "latency > threshold"}
	}
	if s.JitterMs > jitterTh {
		return Brownout{Brownout: true, Reason: "jitter > threshold"}
	}
	if s.LossPct > lossTh {
		return Brownout{Brownout: true, Reason: "loss > threshold"}
	}
	return Brownout{}
}

// Score applies the policy weights to a
// PathScore. Lower score is better. The
// result is deterministic: the same
// inputs always produce the same number.
func Score(p Policy, scores map[string]PathScore) (string, float64) {
	weight := map[Criteria]int{}
	for _, w := range p.Weights {
		weight[w.Criteria] = w.Weight
	}
	bestID := ""
	best := 1e18
	// Sort paths by id to ensure deterministic
	// tie-breaking.
	ps := append([]Path(nil), p.Paths...)
	sort.Slice(ps, func(i, k int) bool { return ps[i].ID < ps[k].ID })
	for _, pa := range ps {
		s, ok := scores[pa.ID]
		if !ok {
			continue
		}
		if !s.Reachable {
			continue
		}
		score := float64(0)
		score += float64(weight[CriterionLatency]) * s.LatencyMs
		score += float64(weight[CriterionJitter]) * s.JitterMs
		score += float64(weight[CriterionLoss]) * s.LossPct
		score += float64(weight[CriterionCost]) * float64(s.Cost)
		score -= float64(weight[CriterionPreference]) * float64(s.Preference)
		score -= float64(weight[CriterionReachability]) * 1000
		if score < best {
			best = score
			bestID = pa.ID
		}
	}
	return bestID, best
}

// Health is the per-policy health view.
type Health struct {
	PolicyID    string `json:"policyId"`
	CurrentPath string `json:"currentPath"`
	BestPath    string `json:"bestPath"`
	Brownout    bool   `json:"brownout"`
	Reason      string `json:"reason,omitempty"`
}

// Manager is the V1 SD-WAN store. Manager
// is goroutine-safe.
type Manager struct {
	mu       sync.Mutex
	policies map[string]Policy
	current  map[string]string
	health   map[string]Health
}

// NewManager returns an empty manager.
func NewManager() *Manager {
	return &Manager{policies: map[string]Policy{}, current: map[string]string{}, health: map[string]Health{}}
}

// Set replaces the policy.
func (m *Manager) Set(p Policy) error {
	if err := p.Valid(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.policies[p.ID] = p
	return nil
}

// Evaluate applies the policy to the
// supplied scores, applies hysteresis
// (only flip when the new score is
// materially better), and updates the
// current-path and health records.
func (m *Manager) Evaluate(id string, scores map[string]PathScore, latencyTh, jitterTh, lossTh float64) (Health, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.policies[id]
	if !ok {
		return Health{}, errors.New("sdwan: unknown policy id")
	}
	bestID, bestScore := Score(p, scores)
	prev := m.current[id]
	cur := prev
	if prev != bestID {
		// Hysteresis: only flip when the new
		// score is materially better (or the
		// previous path is in brownout).
		prevScore := 0.0
		if prev != "" {
			prevScore = bestScore
			if ps, ok := scores[prev]; ok {
				prevScore = prevScoreFor(p, ps, prevScore, scores)
			}
		}
		if prev == "" || isBrownout(scores[prev], latencyTh, jitterTh, lossTh) || (prevScore-bestScore) >= p.HysteresisMs {
			cur = bestID
		}
	}
	m.current[id] = cur
	h := Health{PolicyID: id, CurrentPath: cur, BestPath: bestID}
	if cur != "" {
		if bo := DetectBrownout(scores[cur], latencyTh, jitterTh, lossTh); bo.Brownout {
			h.Brownout = true
			h.Reason = bo.Reason
		}
	}
	m.health[id] = h
	return h, nil
}

func prevScoreFor(p Policy, ps PathScore, fallback float64, scores map[string]PathScore) float64 {
	// Compute the score for the previous
	// path using the same weights.
	weight := map[Criteria]int{}
	for _, w := range p.Weights {
		weight[w.Criteria] = w.Weight
	}
	score := float64(0)
	score += float64(weight[CriterionLatency]) * ps.LatencyMs
	score += float64(weight[CriterionJitter]) * ps.JitterMs
	score += float64(weight[CriterionLoss]) * ps.LossPct
	score += float64(weight[CriterionCost]) * float64(ps.Cost)
	score -= float64(weight[CriterionPreference]) * float64(ps.Preference)
	score -= float64(weight[CriterionReachability]) * 1000
	return score
}

func isBrownout(s PathScore, latencyTh, jitterTh, lossTh float64) bool {
	return DetectBrownout(s, latencyTh, jitterTh, lossTh).Brownout
}

// Health returns a copy of the health
// record.
func (m *Manager) Health(id string) (Health, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	h, ok := m.health[id]
	return h, ok
}
