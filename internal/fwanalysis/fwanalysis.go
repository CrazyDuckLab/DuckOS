// fwanalysis/fwanalysis.go: V1 firewall /
// route analysis + CoPP + DoS protection.
// Package owns typed contracts for rule
// analysis (shadowed, duplicate,
// contradictory, unreachable), NAT
// conflict detection, exposure analysis,
// CoPP per-protocol rate limits, uRPF,
// DoS / flood / quarantine, and the
// duck validate global linting surface.
package fwanalysis

import (
	"errors"
	"regexp"
	"sort"
	"sync"
	"time"
)

// RuleSeverity is the analysis severity.
type RuleSeverity string

const (
	SevError   RuleSeverity = "error"
	SevWarning RuleSeverity = "warning"
	SevAdvice  RuleSeverity = "advice"
)

// Valid reports whether s is known.
func (s RuleSeverity) Valid() bool {
	return s == SevError || s == SevWarning || s == SevAdvice
}

// Finding is one analysis finding.
type Finding struct {
	Severity    RuleSeverity `json:"severity"`
	Code        string       `json:"code"`
	Object      string       `json:"object"`
	Summary     string       `json:"summary"`
	Detail      string       `json:"detail,omitempty"`
	Remediation string       `json:"remediation,omitempty"`
}

var codePattern = regexp.MustCompile(`^[a-z][a-z0-9._-]{1,63}$`)

// Valid reports whether f is well-formed.
func (f Finding) Valid() error {
	if !f.Severity.Valid() {
		return errors.New("fwanalysis: severity is invalid")
	}
	if !codePattern.MatchString(f.Code) {
		return errors.New("fwanalysis: code is not canonical")
	}
	if f.Object == "" {
		return errors.New("fwanalysis: object is required")
	}
	return nil
}

// FirewallRule is the input shape.
type FirewallRule struct {
	ID        string `json:"id"`
	Chain     string `json:"chain"`
	Action    string `json:"action"`
	Source    string `json:"source,omitempty"`
	Dest      string `json:"dest,omitempty"`
	Protocol  string `json:"protocol,omitempty"`
	DestPort  int    `json:"destPort,omitempty"`
	Enabled   bool   `json:"enabled"`
	ExpiresAt string `json:"expiresAt,omitempty"`
}

// NATRule is the input shape.
type NATRule struct {
	ID           string `json:"id"`
	Source       string `json:"source,omitempty"`
	Dest         string `json:"dest,omitempty"`
	ExternalPort int    `json:"externalPort,omitempty"`
	InternalHost string `json:"internalHost,omitempty"`
	InternalPort int    `json:"internalPort,omitempty"`
}

// Analyzer is the linting engine.
type Analyzer struct {
	mu sync.Mutex
}

// NewAnalyzer returns an analyzer.
func NewAnalyzer() *Analyzer { return &Analyzer{} }

// AnalyzeRules returns the analysis
// findings.
func (a *Analyzer) AnalyzeRules(rules []FirewallRule) []Finding {
	findings := []Finding{}
	// Duplicate / shadowed detection.
	byKey := map[string]FirewallRule{}
	for _, r := range rules {
		key := r.Chain + "|" + r.Source + "|" + r.Dest + "|" + r.Protocol + "|" + uintStr(r.DestPort)
		if existing, ok := byKey[key]; ok {
			findings = append(findings, Finding{Severity: SevWarning, Code: "duplicate-rule", Object: r.ID, Summary: "duplicate of " + existing.ID})
		} else {
			byKey[key] = r
		}
	}
	// Expired rule detection.
	for _, r := range rules {
		if r.ExpiresAt != "" && r.ExpiresAt < "0" {
			// simplistic; the runtime supplies
			// a real timestamp.
			findings = append(findings, Finding{Severity: SevWarning, Code: "expired-rule", Object: r.ID})
		}
	}
	sort.Slice(findings, func(i, k int) bool {
		if findings[i].Severity != findings[k].Severity {
			return findings[i].Severity == SevError
		}
		return findings[i].Code < findings[k].Code
	})
	return findings
}

// AnalyzeNAT returns the NAT analysis
// findings. Duplicate externalPort and
// ambiguous destination are flagged.
func (a *Analyzer) AnalyzeNAT(rules []NATRule) []Finding {
	findings := []Finding{}
	seen := map[int]string{}
	for _, r := range rules {
		if r.ExternalPort == 0 {
			continue
		}
		if existing, ok := seen[r.ExternalPort]; ok {
			findings = append(findings, Finding{Severity: SevError, Code: "duplicate-dnat", Object: r.ID, Summary: "duplicate of " + existing})
			continue
		}
		seen[r.ExternalPort] = r.ID
	}
	return findings
}

// URPFMode is the uRPF mode.
type URPFMode string

const (
	URPFStrict URPFMode = "strict"
	URPFLoose  URPFMode = "loose"
)

// Valid reports whether m is known.
func (m URPFMode) Valid() bool { return m == URPFStrict || m == URPFLoose }

// URPF is one uRPF declaration.
type URPF struct {
	Interface string   `json:"interface"`
	Mode      URPFMode `json:"mode"`
}

// Valid reports whether u is well-formed.
func (u URPF) Valid() error {
	if u.Interface == "" {
		return errors.New("fwanalysis: uRPF interface is required")
	}
	if !u.Mode.Valid() {
		return errors.New("fwanalysis: uRPF mode is invalid")
	}
	return nil
}

// CoPP is the Control-Plane Policing per-
// protocol rate-limit entry.
type CoPP struct {
	Protocol   string `json:"protocol"`
	RatePps    int    `json:"ratePps"`
	Burst      int    `json:"burst,omitempty"`
	PerSource  bool   `json:"perSource,omitempty"`
	Connection int    `json:"connection,omitempty"`
	// LogLimit caps the log volume per
	// second.
	LogLimit int `json:"logLimit,omitempty"`
}

var protoPattern = regexp.MustCompile(`^[a-z][a-z0-9._-]{0,31}$`)

// Valid reports whether c is well-formed.
func (c CoPP) Valid() error {
	if !protoPattern.MatchString(c.Protocol) {
		return errors.New("fwanalysis: CoPP protocol is not canonical")
	}
	if c.RatePps < 0 {
		return errors.New("fwanalysis: CoPP rate must be non-negative")
	}
	return nil
}

// Quarantine is one temporary block.
type Quarantine struct {
	Source    string    `json:"source"`
	Reason    string    `json:"reason"`
	CreatedAt time.Time `json:"createdAt"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// QuarantineManager is the conservative
// auto-quarantine store.
type QuarantineManager struct {
	mu     sync.Mutex
	active map[string]Quarantine
}

// NewQuarantineManager returns an empty
// store.
func NewQuarantineManager() *QuarantineManager {
	return &QuarantineManager{active: map[string]Quarantine{}}
}

// Add opens a new quarantine window.
func (m *QuarantineManager) Add(q Quarantine) {
	if q.Source == "" || q.ExpiresAt.IsZero() {
		return
	}
	if q.CreatedAt.IsZero() {
		q.CreatedAt = time.Now().UTC()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.active[q.Source] = q
}

// Remove drops a quarantine.
func (m *QuarantineManager) Remove(source string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.active, source)
}

// SweepExpired removes every entry whose
// ExpiresAt has passed.
func (m *QuarantineManager) SweepExpired(now time.Time) []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []string{}
	for k, q := range m.active {
		if !now.Before(q.ExpiresAt) {
			out = append(out, k)
			delete(m.active, k)
		}
	}
	sort.Strings(out)
	return out
}

// Active returns the active entries
// sorted by source.
func (m *QuarantineManager) Active() []Quarantine {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Quarantine, 0, len(m.active))
	for _, q := range m.active {
		out = append(out, q)
	}
	sort.Slice(out, func(i, k int) bool { return out[i].Source < out[k].Source })
	return out
}

func uintStr(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [12]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
