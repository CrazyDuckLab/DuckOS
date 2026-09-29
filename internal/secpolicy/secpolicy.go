// secpolicy/secpolicy.go: V1 security
// policy / posture / blast-radius /
// crash-loop / watchdog / graceful
// degradation / resource budget /
// tenant quotas / webhooks surface.
package secpolicy

import (
	"errors"
	"regexp"
	"sort"
	"sync"
	"time"
)

// Policy is the typed policy guardrail.
type Policy struct {
	ID string `json:"id"`
	// Severity is the violation severity.
	Severity string `json:"severity"` // "error" | "warning" | "advisory"
	// Description explains the rule.
	Description string `json:"description"`
	// Enforce toggles the hard-mode
	// behaviour.
	Enforce bool `json:"enforce"`
}

var idPattern = regexp.MustCompile(`^[a-z][a-z0-9._-]{0,63}$`)

// Valid reports whether p is well-formed.
func (p Policy) Valid() error {
	if !idPattern.MatchString(p.ID) {
		return errors.New("secpolicy: id is not canonical")
	}
	switch p.Severity {
	case "error", "warning", "advisory":
	default:
		return errors.New("secpolicy: severity is invalid")
	}
	return nil
}

// Violation is one observed breach.
type Violation struct {
	PolicyID string    `json:"policyId"`
	Severity string    `json:"severity"`
	Object   string    `json:"object"`
	At       time.Time `json:"at"`
	// Override toggles the explicit
	// operator override flag.
	Override      bool   `json:"override,omitempty"`
	OverrideActor string `json:"overrideActor,omitempty"`
}

// PolicyStore is the policy store.
type PolicyStore struct {
	mu        sync.Mutex
	policies  map[string]Policy
	overrides map[string]bool
}

// NewPolicyStore returns an empty store.
func NewPolicyStore() *PolicyStore {
	return &PolicyStore{policies: map[string]Policy{}, overrides: map[string]bool{}}
}

// Set inserts a policy.
func (s *PolicyStore) Set(p Policy) error {
	if err := p.Valid(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.policies[p.ID] = p
	return nil
}

// Override marks a policy as overridden.
func (s *PolicyStore) Override(policyID, actor string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.overrides[policyID] = true
	_ = actor
}

// All returns every policy sorted.
func (s *PolicyStore) All() []Policy {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Policy, 0, len(s.policies))
	for _, p := range s.policies {
		out = append(out, p)
	}
	sort.Slice(out, func(i, k int) bool { return out[i].ID < out[k].ID })
	return out
}

// PostureFinding is a posture observation.
type PostureFinding struct {
	Severity    string `json:"severity"`
	Code        string `json:"code"`
	Summary     string `json:"summary"`
	Evidence    string `json:"evidence,omitempty"`
	Remediation string `json:"remediation,omitempty"`
}

// Posture is the typed posture report.
type Posture struct {
	GeneratedAt time.Time        `json:"generatedAt"`
	Findings    []PostureFinding `json:"findings"`
	Score       int              `json:"score"` // 0..100, qualitative
}

// PostureStore is the posture store.
type PostureStore struct {
	mu  sync.Mutex
	gen Posture
}

// NewPostureStore returns an empty store.
func NewPostureStore() *PostureStore { return &PostureStore{} }

// Set replaces the posture.
func (s *PostureStore) Set(p Posture) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gen = p
}

// Get returns the current posture.
func (s *PostureStore) Get() Posture {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.gen
}

// BlastRadius is the per-candidate
// impact estimate.
type BlastRadius struct {
	CandidateID    string   `json:"candidateId"`
	AffectedIFs    []string `json:"affectedIfs"`
	AffectedVLANs  []string `json:"affectedVlans"`
	AffectedRoutes []string `json:"affectedRoutes"`
	AffectedUsers  []string `json:"affectedUsers"`
	// CriticalServices is the list of
	// critical service ids touched.
	CriticalServices []string `json:"criticalServices"`
	// ManagementImpact toggles whether
	// the management plane is touched.
	ManagementImpact bool   `json:"managementImpact"`
	Risk             string `json:"risk"` // "low" | "medium" | "high" | "critical"
	Reason           string `json:"reason,omitempty"`
}

// BlastStore is the blast-radius store.
type BlastStore struct {
	mu sync.Mutex
	r  map[string]BlastRadius
}

// NewBlastStore returns an empty store.
func NewBlastStore() *BlastStore { return &BlastStore{r: map[string]BlastRadius{}} }

// Set records the blast radius.
func (s *BlastStore) Set(b BlastRadius) {
	if b.CandidateID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.r[b.CandidateID] = b
}

// Get returns the blast radius.
func (s *BlastStore) Get(id string) (BlastRadius, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, ok := s.r[id]
	return b, ok
}

// CrashState is the crash-loop detection
// state.
type CrashState string

const (
	CrashOK         CrashState = "ok"
	CrashLoop       CrashState = "loop"
	CrashQuarantine CrashState = "quarantined"
)

// CrashRecord is the per-target crash
// record.
type CrashRecord struct {
	Target  string     `json:"target"`
	State   CrashState `json:"state"`
	Count   int        `json:"count"`
	FirstAt time.Time  `json:"firstAt"`
	LastAt  time.Time  `json:"lastAt"`
	// MaxRetries is the policy cap.
	MaxRetries int `json:"maxRetries"`
}

// CrashManager is the crash-loop
// detector.
type CrashManager struct {
	mu      sync.Mutex
	records map[string]*CrashRecord
}

// NewCrashManager returns an empty
// manager.
func NewCrashManager() *CrashManager { return &CrashManager{records: map[string]*CrashRecord{}} }

// Record observes a crash / restart.
func (m *CrashManager) Record(target string, maxRetries int) CrashState {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.records[target]
	if !ok {
		r = &CrashRecord{Target: target, State: CrashOK, MaxRetries: maxRetries, FirstAt: time.Now().UTC()}
		m.records[target] = r
	}
	r.Count++
	r.LastAt = time.Now().UTC()
	if r.Count >= r.MaxRetries {
		r.State = CrashQuarantine
	} else if r.Count > 1 {
		r.State = CrashLoop
	}
	return r.State
}

// Reset clears the record.
func (m *CrashManager) Reset(target string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.records, target)
}

// All returns every record.
func (m *CrashManager) All() []CrashRecord {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]CrashRecord, 0, len(m.records))
	for _, r := range m.records {
		out = append(out, *r)
	}
	sort.Slice(out, func(i, k int) bool { return out[i].Target < out[k].Target })
	return out
}

// WatchdogState is the per-service
// watchdog state.
type WatchdogState struct {
	Service     string    `json:"service"`
	ProcessUp   bool      `json:"processUp"`
	Functioning bool      `json:"functioning"`
	Restarts    int       `json:"restarts"`
	UpdatedAt   time.Time `json:"updatedAt"`
	Detail      string    `json:"detail,omitempty"`
}

// WatchdogManager is the watchdog store.
type WatchdogManager struct {
	mu      sync.Mutex
	records map[string]WatchdogState
}

// NewWatchdogManager returns an empty
// manager.
func NewWatchdogManager() *WatchdogManager {
	return &WatchdogManager{records: map[string]WatchdogState{}}
}

// Set replaces the watchdog record.
func (m *WatchdogManager) Set(w WatchdogState) {
	if w.Service == "" {
		return
	}
	w.UpdatedAt = time.Now().UTC()
	m.mu.Lock()
	defer m.mu.Unlock()
	m.records[w.Service] = w
}

// All returns every record sorted.
func (m *WatchdogManager) All() []WatchdogState {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]WatchdogState, 0, len(m.records))
	for _, r := range m.records {
		out = append(out, r)
	}
	sort.Slice(out, func(i, k int) bool { return out[i].Service < out[k].Service })
	return out
}

// ResourceBudget is the per-component
// resource projection.
type ResourceBudget struct {
	Component      string `json:"component"`
	CPUPercent     int    `json:"cpuPercent"`
	MemoryPercent  int    `json:"memoryPercent"`
	Pressure       string `json:"pressure"` // "ok" | "elevated" | "high"
	Warn           bool   `json:"warn"`
	Recommendation string `json:"recommendation,omitempty"`
}

// BudgetStore is the budget store.
type BudgetStore struct {
	mu sync.Mutex
	r  map[string]ResourceBudget
}

// NewBudgetStore returns an empty store.
func NewBudgetStore() *BudgetStore { return &BudgetStore{r: map[string]ResourceBudget{}} }

// Set records a budget.
func (s *BudgetStore) Set(b ResourceBudget) {
	if b.Component == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.r[b.Component] = b
}

// All returns every budget sorted.
func (s *BudgetStore) All() []ResourceBudget {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]ResourceBudget, 0, len(s.r))
	for _, b := range s.r {
		out = append(out, b)
	}
	sort.Slice(out, func(i, k int) bool { return out[i].Component < out[k].Component })
	return out
}

// Quota is a per-tenant / per-VRF quota.
type Quota struct {
	Tenant        string `json:"tenant"`
	ConntrackMax  int    `json:"conntrackMax,omitempty"`
	LeasesMax     int    `json:"leasesMax,omitempty"`
	MappingsMax   int    `json:"mappingsMax,omitempty"`
	SessionsMax   int    `json:"sessionsMax,omitempty"`
	BandwidthKbps int    `json:"bandwidthKbps,omitempty"`
}

// QuotaStore is the per-tenant quota
// store.
type QuotaStore struct {
	mu sync.Mutex
	r  map[string]Quota
}

// NewQuotaStore returns an empty store.
func NewQuotaStore() *QuotaStore { return &QuotaStore{r: map[string]Quota{}} }

// Set records a quota.
func (s *QuotaStore) Set(q Quota) {
	if q.Tenant == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.r[q.Tenant] = q
}

// All returns every quota sorted.
func (s *QuotaStore) All() []Quota {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Quota, 0, len(s.r))
	for _, q := range s.r {
		out = append(out, q)
	}
	sort.Slice(out, func(i, k int) bool { return out[i].Tenant < out[k].Tenant })
	return out
}

// Webhook is the signed-webhook endpoint.
type Webhook struct {
	ID         string   `json:"id"`
	URL        string   `json:"url"`
	SecretRef  string   `json:"secretRef"` // vault ref
	Events     []string `json:"events,omitempty"`
	MaxRetries int      `json:"maxRetries,omitempty"`
	BackoffSec int      `json:"backoffSec,omitempty"`
}

// WebhookStore is the webhook store.
type WebhookStore struct {
	mu sync.Mutex
	r  map[string]Webhook
}

// NewWebhookStore returns an empty store.
func NewWebhookStore() *WebhookStore { return &WebhookStore{r: map[string]Webhook{}} }

// Add inserts a webhook.
func (s *WebhookStore) Add(w Webhook) error {
	if w.ID == "" || w.URL == "" || w.SecretRef == "" {
		return errors.New("secpolicy: webhook id+url+secret required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.r[w.ID] = w
	return nil
}

// All returns every webhook sorted.
func (s *WebhookStore) All() []Webhook {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Webhook, 0, len(s.r))
	for _, w := range s.r {
		out = append(out, w)
	}
	sort.Slice(out, func(i, k int) bool { return out[i].ID < out[k].ID })
	return out
}
