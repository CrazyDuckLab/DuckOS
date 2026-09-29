// canary/canary.go: V1 canary apply +
// transaction dependency ordering +
// event correlation + before/after path
// + stale detection + safe destructive
// + retention + decommission + lab mode
// + acceptance runner surface.
package canary

import (
	"errors"
	"regexp"
	"sort"
	"sync"
	"time"
)

// ApplyScope is the canary scope.
type ApplyScope string

const (
	ScopeVRF       ApplyScope = "vrf"
	ScopeVLAN      ApplyScope = "vlan"
	ScopeTenant    ApplyScope = "tenant"
	ScopeInterface ApplyScope = "interface"
	ScopeNamespace ApplyScope = "namespace"
)

// Valid reports whether s is known.
func (s ApplyScope) Valid() bool {
	switch s {
	case ScopeVRF, ScopeVLAN, ScopeTenant, ScopeInterface, ScopeNamespace:
		return true
	}
	return false
}

// Canary is a canary apply descriptor.
type Canary struct {
	ID     string     `json:"id"`
	Scope  ApplyScope `json:"scope"`
	Target string     `json:"target"`
	// Promoted tracks the promotion
	// state.
	Promoted   bool      `json:"promoted"`
	PromotedAt time.Time `json:"promotedAt,omitempty"`
	CreatedAt  time.Time `json:"createdAt"`
	Result     string    `json:"result,omitempty"`
}

var idPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:-]{0,63}$`)

// Valid reports whether c is well-formed.
func (c Canary) Valid() error {
	if !idPattern.MatchString(c.ID) {
		return errors.New("canary: id is not canonical")
	}
	if !c.Scope.Valid() {
		return errors.New("canary: scope is invalid")
	}
	if c.Target == "" {
		return errors.New("canary: target is required")
	}
	return nil
}

// CanaryStore is the canary store.
type CanaryStore struct {
	mu       sync.Mutex
	canaries map[string]Canary
}

// NewCanaryStore returns an empty store.
func NewCanaryStore() *CanaryStore {
	return &CanaryStore{canaries: map[string]Canary{}}
}

// Add inserts a canary.
func (s *CanaryStore) Add(c Canary) error {
	if err := c.Valid(); err != nil {
		return err
	}
	if c.CreatedAt.IsZero() {
		c.CreatedAt = time.Now().UTC()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.canaries[c.ID] = c
	return nil
}

// Promote marks a canary promoted.
func (s *CanaryStore) Promote(id, result string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.canaries[id]
	if !ok {
		return errors.New("canary: unknown id")
	}
	c.Promoted = true
	c.PromotedAt = time.Now().UTC()
	c.Result = result
	s.canaries[id] = c
	return nil
}

// All returns every canary sorted.
func (s *CanaryStore) All() []Canary {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Canary, 0, len(s.canaries))
	for _, c := range s.canaries {
		out = append(out, c)
	}
	sort.Slice(out, func(i, k int) bool { return out[i].ID < out[k].ID })
	return out
}

// TxnStep is one step in a transaction.
type TxnStep struct {
	Name      string   `json:"name"`
	Kind      string   `json:"kind"` // "interface" | "address" | "route" | "firewall" | "service" | "dns"
	DependsOn []string `json:"dependsOn,omitempty"`
}

// TxnPlan is the dependency-ordered plan.
type TxnPlan struct {
	ID    string    `json:"id"`
	Steps []TxnStep `json:"steps"`
}

// PlanTopo orders the steps in dependency
// order. Cycles are reported.
func PlanTopo(plan TxnPlan) (TxnPlan, error) {
	// Simple Kahn's algorithm.
	in := map[string]int{}
	out := map[string][]string{}
	for _, s := range plan.Steps {
		if _, ok := in[s.Name]; !ok {
			in[s.Name] = 0
		}
		for _, d := range s.DependsOn {
			in[s.Name]++
			out[d] = append(out[d], s.Name)
		}
	}
	var ready []string
	for name, deg := range in {
		if deg == 0 {
			ready = append(ready, name)
		}
	}
	sort.Strings(ready)
	ordered := []TxnStep{}
	visited := map[string]bool{}
	for len(ready) > 0 {
		n := ready[0]
		ready = ready[1:]
		if visited[n] {
			continue
		}
		visited[n] = true
		for _, s := range plan.Steps {
			if s.Name == n {
				ordered = append(ordered, s)
				break
			}
		}
		for _, next := range out[n] {
			in[next]--
			if in[next] == 0 {
				ready = append(ready, next)
			}
		}
		sort.Strings(ready)
	}
	if len(ordered) != len(plan.Steps) {
		return TxnPlan{}, errors.New("canary: dependency cycle detected")
	}
	return TxnPlan{ID: plan.ID, Steps: ordered}, nil
}

// Incident is one correlated incident.
type Incident struct {
	ID         string    `json:"id"`
	StartedAt  time.Time `json:"startedAt"`
	EndedAt    time.Time `json:"endedAt,omitempty"`
	Events     []string  `json:"events"`
	Objects    []string  `json:"objects"`
	RootObject string    `json:"rootObject,omitempty"`
	Confidence int       `json:"confidence"`
	Summary    string    `json:"summary"`
}

// IncidentStore is the incident store.
type IncidentStore struct {
	mu        sync.Mutex
	incidents map[string]Incident
}

// NewIncidentStore returns an empty store.
func NewIncidentStore() *IncidentStore {
	return &IncidentStore{incidents: map[string]Incident{}}
}

// Open starts a new incident.
func (s *IncidentStore) Open(i Incident) error {
	if i.ID == "" {
		return errors.New("canary: incident id is required")
	}
	if i.StartedAt.IsZero() {
		i.StartedAt = time.Now().UTC()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.incidents[i.ID] = i
	return nil
}

// Correlate adds an event / object to an
// incident.
func (s *IncidentStore) Correlate(id, event, object string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	i, ok := s.incidents[id]
	if !ok {
		return errors.New("canary: unknown incident")
	}
	i.Events = append(i.Events, event)
	if object != "" {
		i.Objects = append(i.Objects, object)
	}
	s.incidents[id] = i
	return nil
}

// Close ends an incident.
func (s *IncidentStore) Close(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	i, ok := s.incidents[id]
	if !ok {
		return errors.New("canary: unknown incident")
	}
	if i.EndedAt.IsZero() {
		i.EndedAt = time.Now().UTC()
		s.incidents[id] = i
	}
	return nil
}

// All returns every incident sorted.
func (s *IncidentStore) All() []Incident {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Incident, 0, len(s.incidents))
	for _, i := range s.incidents {
		out = append(out, i)
	}
	sort.Slice(out, func(i, k int) bool { return out[i].StartedAt.Before(out[k].StartedAt) })
	return out
}

// PathSnapshot is one decision-point
// snapshot for the before/after path.
type PathSnapshot struct {
	At       time.Time `json:"at"`
	Kind     string    `json:"kind"`
	ObjectID string    `json:"objectId"`
	Decision string    `json:"decision"`
	Note     string    `json:"note,omitempty"`
}

// PathDiff is the before/after diff.
type PathDiff struct {
	Before []PathSnapshot `json:"before"`
	After  []PathSnapshot `json:"after"`
	// Changed returns the per-object
	// decision changes.
	Changed []string `json:"changed"`
}

// PathCapture is the path-snapshot store.
type PathCapture struct {
	mu     sync.Mutex
	before []PathSnapshot
	after  []PathSnapshot
}

// NewPathCapture returns an empty store.
func NewPathCapture() *PathCapture { return &PathCapture{} }

// CaptureBefore records a before snapshot.
func (c *PathCapture) CaptureBefore(s PathSnapshot) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.before = append(c.before, s)
}

// CaptureAfter records an after snapshot.
func (c *PathCapture) CaptureAfter(s PathSnapshot) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.after = append(c.after, s)
}

// Diff returns the before/after diff.
func (c *PathCapture) Diff() PathDiff {
	c.mu.Lock()
	defer c.mu.Unlock()
	beforeMap := map[string]PathSnapshot{}
	for _, s := range c.before {
		beforeMap[s.Kind+":"+s.ObjectID] = s
	}
	afterMap := map[string]PathSnapshot{}
	for _, s := range c.after {
		afterMap[s.Kind+":"+s.ObjectID] = s
	}
	diff := PathDiff{Before: append([]PathSnapshot(nil), c.before...), After: append([]PathSnapshot(nil), c.after...)}
	for k, a := range afterMap {
		if b, ok := beforeMap[k]; !ok || b.Decision != a.Decision {
			diff.Changed = append(diff.Changed, k)
		}
	}
	sort.Strings(diff.Changed)
	return diff
}

// StaleObject is a stale-object finding.
type StaleObject struct {
	Kind     string    `json:"kind"`
	ID       string    `json:"id"`
	Reason   string    `json:"reason"`
	LastUsed time.Time `json:"lastUsed,omitempty"`
}

// StaleDetector is the stale-object store.
type StaleDetector struct {
	mu      sync.Mutex
	objects map[string]StaleObject
}

// NewStaleDetector returns an empty store.
func NewStaleDetector() *StaleDetector {
	return &StaleDetector{objects: map[string]StaleObject{}}
}

// Add inserts a finding.
func (d *StaleDetector) Add(o StaleObject) {
	if o.ID == "" {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.objects[o.Kind+":"+o.ID] = o
}

// All returns every finding sorted.
func (d *StaleDetector) All() []StaleObject {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]StaleObject, 0, len(d.objects))
	for _, o := range d.objects {
		out = append(out, o)
	}
	sort.Slice(out, func(i, k int) bool {
		if out[i].Kind != out[k].Kind {
			return out[i].Kind < out[k].Kind
		}
		return out[i].ID < out[k].ID
	})
	return out
}

// DestructiveAction is the safe-
// destructive action preview.
type DestructiveAction struct {
	Name              string `json:"name"`
	Scope             string `json:"scope"`
	Impact            string `json:"impact"`
	ManagementTouched bool   `json:"managementTouched"`
	RequiresConfirm   bool   `json:"requiresConfirm"`
}

// ActionCatalog is the catalog of safe
// destructive actions.
type ActionCatalog struct {
	mu      sync.Mutex
	actions map[string]DestructiveAction
}

// NewActionCatalog returns an empty store.
func NewActionCatalog() *ActionCatalog {
	return &ActionCatalog{actions: map[string]DestructiveAction{}}
}

// Register inserts an action.
func (c *ActionCatalog) Register(a DestructiveAction) {
	if a.Name == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.actions[a.Name] = a
}

// Get returns an action.
func (c *ActionCatalog) Get(name string) (DestructiveAction, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	a, ok := c.actions[name]
	return a, ok
}

// All returns every action sorted.
func (c *ActionCatalog) All() []DestructiveAction {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]DestructiveAction, 0, len(c.actions))
	for _, a := range c.actions {
		out = append(out, a)
	}
	sort.Slice(out, func(i, k int) bool { return out[i].Name < out[k].Name })
	return out
}

// Retention tier for telemetry.
type RetentionTier struct {
	Name       string `json:"name"`
	Resolution string `json:"resolution"` // "1s" | "1m" | "1h"
	MaxAgeDays int    `json:"maxAgeDays"`
	MaxBytes   int64  `json:"maxBytes"`
}

// RetentionStore is the retention store.
type RetentionStore struct {
	mu    sync.Mutex
	tiers []RetentionTier
}

// NewRetentionStore returns an empty store.
func NewRetentionStore() *RetentionStore { return &RetentionStore{} }

// SetTiers replaces the tier list.
func (s *RetentionStore) SetTiers(t []RetentionTier) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tiers = append([]RetentionTier(nil), t...)
}

// Tiers returns a copy.
func (s *RetentionStore) Tiers() []RetentionTier {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]RetentionTier(nil), s.tiers...)
}

// Decommission is one decommission
// workflow record.
type Decommission struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	ObjectID string `json:"objectId"`
	// Stage tracks the workflow: "migrate" | "disable" | "remove" | "cleanup".
	Stage        string    `json:"stage"`
	CreatedAt    time.Time `json:"createdAt"`
	CompletedAt  time.Time `json:"completedAt,omitempty"`
	Dependencies []string  `json:"dependencies,omitempty"`
}

// DecommissionStore is the decommission
// store.
type DecommissionStore struct {
	mu  sync.Mutex
	rec map[string]Decommission
}

// NewDecommissionStore returns an empty
// store.
func NewDecommissionStore() *DecommissionStore {
	return &DecommissionStore{rec: map[string]Decommission{}}
}

// Add inserts a record.
func (s *DecommissionStore) Add(d Decommission) error {
	if d.ID == "" {
		return errors.New("canary: decommission id required")
	}
	if d.CreatedAt.IsZero() {
		d.CreatedAt = time.Now().UTC()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rec[d.ID] = d
	return nil
}

// Advance moves the workflow forward.
func (s *DecommissionStore) Advance(id, stage string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.rec[id]
	if !ok {
		return errors.New("canary: unknown decommission")
	}
	d.Stage = stage
	if stage == "cleanup" {
		d.CompletedAt = time.Now().UTC()
	}
	s.rec[id] = d
	return nil
}

// All returns every record.
func (s *DecommissionStore) All() []Decommission {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Decommission, 0, len(s.rec))
	for _, d := range s.rec {
		out = append(out, d)
	}
	sort.Slice(out, func(i, k int) bool { return out[i].ID < out[k].ID })
	return out
}

// Lab is a lab environment descriptor.
type Lab struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Networks []string `json:"networks,omitempty"`
	// ProductionSafe toggles whether the
	// lab exposes any production surface.
	ProductionSafe bool `json:"productionSafe,omitempty"`
}

// Valid reports whether l is well-formed.
func (l Lab) Valid() error {
	if !idPattern.MatchString(l.ID) {
		return errors.New("canary: lab id is not canonical")
	}
	if l.Name == "" {
		return errors.New("canary: lab name is required")
	}
	if !l.ProductionSafe {
		// Production interfaces are
		// forbidden by default; the
		// ProductionSafe flag must be
		// explicitly set when the lab must
		// touch a production surface.
		for _, n := range l.Networks {
			if n == "eth0" || n == "production" {
				return errors.New("canary: lab touches production surface")
			}
		}
	}
	return nil
}

// LabStore is the lab store.
type LabStore struct {
	mu   sync.Mutex
	labs map[string]Lab
}

// NewLabStore returns an empty store.
func NewLabStore() *LabStore { return &LabStore{labs: map[string]Lab{}} }

// Add inserts a lab.
func (s *LabStore) Add(l Lab) error {
	if err := l.Valid(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.labs[l.ID] = l
	return nil
}

// All returns every lab sorted.
func (s *LabStore) All() []Lab {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Lab, 0, len(s.labs))
	for _, l := range s.labs {
		out = append(out, l)
	}
	sort.Slice(out, func(i, k int) bool { return out[i].ID < out[k].ID })
	return out
}

// ScenarioResult is the outcome of an
// acceptance scenario run.
type ScenarioResult string

const (
	ResultPass          ScenarioResult = "pass"
	ResultFail          ScenarioResult = "fail"
	ResultBlocked       ScenarioResult = "blocked"
	ResultHardwareReq   ScenarioResult = "hardware-required"
	ResultCredentialReq ScenarioResult = "credential-required"
)

// Valid reports whether r is known.
func (r ScenarioResult) Valid() bool {
	switch r {
	case ResultPass, ResultFail, ResultBlocked, ResultHardwareReq, ResultCredentialReq:
		return true
	}
	return false
}

// ScenarioRun is one scenario run.
type ScenarioRun struct {
	ID         string         `json:"id"`
	ScenarioID string         `json:"scenarioId"`
	Result     ScenarioResult `json:"result"`
	StartedAt  time.Time      `json:"startedAt"`
	EndedAt    time.Time      `json:"endedAt,omitempty"`
	Detail     string         `json:"detail,omitempty"`
}

// AccRunner is the acceptance runner
// store.
type AccRunner struct {
	mu   sync.Mutex
	runs []ScenarioRun
}

// NewAccRunner returns an empty runner.
func NewAccRunner() *AccRunner { return &AccRunner{} }

// Record appends a run.
func (r *AccRunner) Record(s ScenarioRun) error {
	if s.ID == "" || s.ScenarioID == "" {
		return errors.New("canary: scenario id and run id required")
	}
	if !s.Result.Valid() {
		return errors.New("canary: scenario result is invalid")
	}
	if s.StartedAt.IsZero() {
		s.StartedAt = time.Now().UTC()
	}
	if !s.EndedAt.IsZero() && s.EndedAt.Before(s.StartedAt) {
		return errors.New("canary: endedAt before startedAt")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.runs = append(r.runs, s)
	return nil
}

// All returns every run sorted by start
// time.
func (r *AccRunner) All() []ScenarioRun {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]ScenarioRun, len(r.runs))
	copy(out, r.runs)
	sort.Slice(out, func(i, k int) bool { return out[i].StartedAt.Before(out[k].StartedAt) })
	return out
}

// Counts returns the result counts.
func (r *AccRunner) Counts() map[ScenarioResult]int {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := map[ScenarioResult]int{}
	for _, run := range r.runs {
		out[run.Result]++
	}
	return out
}
