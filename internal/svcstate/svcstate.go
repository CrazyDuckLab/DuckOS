// svcstate/svcstate.go: V1 service-lifecycle
// state machine. The package owns the typed
// contract for validate-then-reload,
// minimal-disruption reload, last-known-good
// retention, per-service rollback, status,
// and watchdog-differentiation between
// "process running" and "function working".
//
// The runtime is a thin mapping to systemd /
// s6 / runit; the package itself does not
// spawn processes.
package svcstate

import (
	"errors"
	"regexp"
	"sort"
	"sync"
	"time"
)

// State is the per-service state.
type State string

const (
	StateStopped    State = "stopped"
	StateStarting   State = "starting"
	StateRunning    State = "running"
	StateReloading  State = "reloading"
	StateDegraded   State = "degraded"
	StateFailing    State = "failing"
	StateRestarting State = "restarting"
)

// Valid reports whether s is known.
func (s State) Valid() bool {
	switch s {
	case StateStopped, StateStarting, StateRunning, StateReloading,
		StateDegraded, StateFailing, StateRestarting:
		return true
	}
	return false
}

// Healthy reports whether the state
// represents a working service.
func (s State) Healthy() bool { return s == StateRunning || s == StateReloading }

// Service is a typed service record.
type Service struct {
	Name             string        `json:"name"`
	State            State         `json:"state"`
	LastConfig       string        `json:"lastConfig,omitempty"` // hash of the active config
	LastGoodConfig   string        `json:"lastGoodConfig,omitempty"`
	LastError        string        `json:"lastError,omitempty"`
	LastReload       time.Time     `json:"lastReload,omitempty"`
	RestartCount     int           `json:"restartCount"`
	DisruptionBudget time.Duration `json:"disruptionBudget,omitempty"`
}

var namePattern = regexp.MustCompile(`^[a-z][a-z0-9._-]{0,63}$`)

// Valid reports whether s is well-formed.
func (s Service) Valid() error {
	if !namePattern.MatchString(s.Name) {
		return errors.New("svcstate: name is not canonical")
	}
	if !s.State.Valid() {
		return errors.New("svcstate: state is invalid")
	}
	if s.DisruptionBudget < 0 {
		return errors.New("svcstate: disruption budget must be non-negative")
	}
	return nil
}

// ValidateFunc is the runtime hook that
// validates a candidate config without
// applying it.
type ValidateFunc func(candidate []byte) error

// ApplyFunc is the runtime hook that
// applies a validated config and reports
// the new state. The function must NOT
// permanently mutate state on error; the
// caller is expected to roll back.
type ApplyFunc func(candidate []byte) (newState State, err error)

// Manager is the V1 service store.
// Manager is goroutine-safe.
type Manager struct {
	mu       sync.Mutex
	services map[string]Service
	clock    func() time.Time
}

// NewManager returns an empty manager.
func NewManager(clock func() time.Time) *Manager {
	if clock == nil {
		clock = time.Now
	}
	return &Manager{services: map[string]Service{}, clock: clock}
}

// Set replaces a service record.
func (m *Manager) Set(s Service) error {
	if err := s.Valid(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.services[s.Name] = s
	return nil
}

// Get returns a copy of the service.
func (m *Manager) Get(name string) (Service, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.services[name]
	return s, ok
}

// List returns every service sorted by
// name.
func (m *Manager) List() []Service {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Service, 0, len(m.services))
	for _, s := range m.services {
		out = append(out, s)
	}
	sort.Slice(out, func(i, k int) bool { return out[i].Name < out[k].Name })
	return out
}

// ValidateConfig is the typed "validate
// before apply" surface. The runtime is
// expected to dispatch to the supplied
// ValidateFunc. The function is pure on
// the candidate; the manager is not
// modified.
func (m *Manager) ValidateConfig(name string, candidate []byte, validate ValidateFunc) error {
	if validate == nil {
		return errors.New("svcstate: validate func is nil")
	}
	if _, ok := m.Get(name); !ok {
		return errors.New("svcstate: unknown service")
	}
	return validate(candidate)
}

// ReloadConfig runs the validate-then-apply
// sequence. The candidate is validated
// first; a failing validation aborts the
// reload without disturbing the active
// state. A successful apply updates the
// service record and promotes the active
// config to LastGoodConfig.
func (m *Manager) ReloadConfig(name string, candidate []byte, hash string, validate ValidateFunc, apply ApplyFunc) error {
	if err := m.ValidateConfig(name, candidate, validate); err != nil {
		return errors.New("svcstate: validation failed: " + err.Error())
	}
	if apply == nil {
		return errors.New("svcstate: apply func is nil")
	}
	newState, err := apply(candidate)
	if err != nil {
		m.recordFailure(name, err.Error())
		return errors.New("svcstate: apply failed: " + err.Error())
	}
	if !newState.Valid() {
		return errors.New("svcstate: apply returned unknown state")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.services[name]
	if s.LastConfig != "" {
		s.LastGoodConfig = s.LastConfig
	}
	s.LastConfig = hash
	s.State = newState
	s.LastError = ""
	s.LastReload = m.clock().UTC()
	m.services[name] = s
	return nil
}

// Rollback restores the LastGoodConfig
// hash on the service record. The runtime
// is expected to call the apply hook with
// the last-known-good payload; the manager
// only updates the record.
func (m *Manager) Rollback(name string, reason string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.services[name]
	if !ok {
		return errors.New("svcstate: unknown service")
	}
	if s.LastGoodConfig == "" {
		return errors.New("svcstate: no last-known-good config to roll back to")
	}
	prev := s.LastConfig
	s.LastConfig = s.LastGoodConfig
	s.LastGoodConfig = prev
	s.State = StateReloading
	s.LastError = "rolled-back: " + reason
	s.LastReload = m.clock().UTC()
	m.services[name] = s
	return nil
}

func (m *Manager) recordFailure(name, errMsg string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.services[name]
	if !ok {
		return
	}
	s.LastError = errMsg
	s.RestartCount++
	if s.State == StateRunning {
		s.State = StateFailing
	}
	m.services[name] = s
}

// HealthSnapshot is the per-service health
// view that distinguishes "process up"
// from "function working".
type HealthSnapshot struct {
	Name        string `json:"name"`
	ProcessUp   bool   `json:"processUp"`
	Functioning bool   `json:"functioning"`
	State       State  `json:"state"`
	Detail      string `json:"detail,omitempty"`
}

// Snapshot returns the health view for
// every service. A service is ProcessUp
// when its state is not StateStopped; it is
// Functioning when its state is Healthy().
func (m *Manager) Snapshot() []HealthSnapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]HealthSnapshot, 0, len(m.services))
	for _, s := range m.services {
		out = append(out, HealthSnapshot{
			Name:        s.Name,
			ProcessUp:   s.State != StateStopped,
			Functioning: s.State.Healthy(),
			State:       s.State,
			Detail:      s.LastError,
		})
	}
	sort.Slice(out, func(i, k int) bool { return out[i].Name < out[k].Name })
	return out
}
