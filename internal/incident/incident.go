// incident/incident.go: V1 operational
// incident experience. The package
// composes the existing health, topology,
// event timeline, change history,
// dependency, Explain Why, checkpoint,
// and rollback surfaces into a single
// 30-second view the operator consults
// after an incident.
//
// The runtime is a thin read-only
// aggregator; the package does not start
// services and never mutates state. The
// view is built on demand from the
// injected Sources.
package incident

import (
	"errors"
	"sort"
	"sync"
	"time"
)

// Severity is the per-object severity in
// the incident view.
type Severity string

const (
	SevOK       Severity = "ok"
	SevInfo     Severity = "info"
	SevWarning  Severity = "warning"
	SevError    Severity = "error"
	SevCritical Severity = "critical"
)

// Valid reports whether s is known.
func (s Severity) Valid() bool {
	switch s {
	case SevOK, SevInfo, SevWarning, SevError, SevCritical:
		return true
	}
	return false
}

// Order returns the severity rank.
func (s Severity) Order() int {
	switch s {
	case SevOK:
		return 0
	case SevInfo:
		return 1
	case SevWarning:
		return 2
	case SevError:
		return 3
	case SevCritical:
		return 4
	}
	return -1
}

// ChangeEvent is one entry in the change
// history. The Agent retains at most 1024
// entries.
type ChangeEvent struct {
	At         time.Time `json:"at"`
	Actor      string    `json:"actor"`
	Kind       string    `json:"kind"`
	Object     string    `json:"object"`
	Summary    string    `json:"summary"`
	Reversible bool      `json:"reversible"`
}

// TimelineEvent is one entry in the
// incident event timeline. The Agent
// retains at most 1024 entries.
type TimelineEvent struct {
	At       time.Time `json:"at"`
	Source   string    `json:"source"`
	Severity Severity  `json:"severity"`
	Summary  string    `json:"summary"`
	Object   string    `json:"object,omitempty"`
}

// HealthObject is the per-object health
// view.
type HealthObject struct {
	Kind      string    `json:"kind"`
	ID        string    `json:"id"`
	Severity  Severity  `json:"severity"`
	Summary   string    `json:"summary"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Checkpoint is a saved configuration
// checkpoint the operator can roll back
// to.
type Checkpoint struct {
	ID        string    `json:"id"`
	Label     string    `json:"label"`
	CreatedAt time.Time `json:"createdAt"`
	Operator  string    `json:"operator"`
}

// View is the operator's 30-second view.
type View struct {
	GeneratedAt   time.Time       `json:"generatedAt"`
	Unhealthy     []HealthObject  `json:"unhealthy"`
	RecentChanges []ChangeEvent   `json:"recentChanges"`
	Timeline      []TimelineEvent `json:"timeline"`
	Checkpoints   []Checkpoint    `json:"checkpoints"`
	WorstSeverity Severity        `json:"worstSeverity"`
	RollbackHint  string          `json:"rollbackHint,omitempty"`
}

// Sources is the injected data the view
// builder consumes. The runtime supplies
// the implementations; the package does
// not assume how the data is fetched.
type Sources interface {
	Health() []HealthObject
	Changes(since time.Time) []ChangeEvent
	Timeline(since time.Time) []TimelineEvent
	Checkpoints() []Checkpoint
}

// Builder assembles a View from a Sources
// implementation. Builder is goroutine-safe.
type Builder struct {
	mu      sync.Mutex
	srcs    Sources
	maxHist int
	clock   func() time.Time
}

// NewBuilder returns a Builder. maxHist
// caps the per-section history.
func NewBuilder(srcs Sources, maxHist int, clock func() time.Time) *Builder {
	if maxHist <= 0 {
		maxHist = 64
	}
	if clock == nil {
		clock = time.Now
	}
	return &Builder{srcs: srcs, maxHist: maxHist, clock: clock}
}

// Build returns the View for the supplied
// since horizon. The function never
// returns an error: a partial view with
// an empty section is acceptable; the
// caller can still observe the worst
// severity.
func (b *Builder) Build(since time.Time) (View, error) {
	if b.srcs == nil {
		return View{}, errors.New("incident: sources are nil")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	v := View{GeneratedAt: b.clock().UTC(), WorstSeverity: SevOK}
	for _, h := range b.srcs.Health() {
		if h.Severity.Order() >= SevWarning.Order() {
			v.Unhealthy = append(v.Unhealthy, h)
		}
		if h.Severity.Order() > v.WorstSeverity.Order() {
			v.WorstSeverity = h.Severity
		}
	}
	sort.SliceStable(v.Unhealthy, func(i, k int) bool {
		if v.Unhealthy[i].Severity.Order() != v.Unhealthy[k].Severity.Order() {
			return v.Unhealthy[i].Severity.Order() > v.Unhealthy[k].Severity.Order()
		}
		return v.Unhealthy[i].ID < v.Unhealthy[k].ID
	})
	changes := b.srcs.Changes(since)
	sort.SliceStable(changes, func(i, k int) bool { return changes[i].At.After(changes[k].At) })
	if len(changes) > b.maxHist {
		changes = changes[:b.maxHist]
	}
	v.RecentChanges = changes
	tl := b.srcs.Timeline(since)
	sort.SliceStable(tl, func(i, k int) bool { return tl[i].At.After(tl[k].At) })
	if len(tl) > b.maxHist {
		tl = tl[:b.maxHist]
	}
	v.Timeline = tl
	cps := b.srcs.Checkpoints()
	sort.SliceStable(cps, func(i, k int) bool { return cps[i].CreatedAt.After(cps[k].CreatedAt) })
	if len(cps) > b.maxHist {
		cps = cps[:b.maxHist]
	}
	v.Checkpoints = cps
	if v.WorstSeverity.Order() >= SevError.Order() && len(cps) > 0 {
		v.RollbackHint = "rollback to checkpoint " + cps[0].ID + " (" + cps[0].Label + ")"
	}
	return v, nil
}

// FuncSources adapts plain functions to
// the Sources interface. Useful for tests
// and for the early integration phase.
type FuncSources struct {
	HealthFn      func() []HealthObject
	ChangesFn     func(time.Time) []ChangeEvent
	TimelineFn    func(time.Time) []TimelineEvent
	CheckpointsFn func() []Checkpoint
}

// Health returns the health view.
func (f FuncSources) Health() []HealthObject { return f.HealthFn() }

// Changes returns the change history since t.
func (f FuncSources) Changes(since time.Time) []ChangeEvent {
	if f.ChangesFn == nil {
		return nil
	}
	return f.ChangesFn(since)
}

// Timeline returns the event timeline since t.
func (f FuncSources) Timeline(since time.Time) []TimelineEvent {
	if f.TimelineFn == nil {
		return nil
	}
	return f.TimelineFn(since)
}

// Checkpoints returns the saved checkpoints.
func (f FuncSources) Checkpoints() []Checkpoint {
	if f.CheckpointsFn == nil {
		return nil
	}
	return f.CheckpointsFn()
}
