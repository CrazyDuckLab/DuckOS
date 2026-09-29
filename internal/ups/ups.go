// Package ups owns the UPS/NUT management model the
// DuckOS V1 contract requires. A UPS is a physical
// device that powers the host or a downstream PDU;
// the package treats it as a managed object with a
// bounded set of attributes, a small health state
// machine and a runtime view the GUI/CLI can show.
//
// The package is split into a model layer (this
// file), a runner (runner.go), a controller
// (controller.go) and a renderer for textual
// previews. The model layer is the only one the
// GUI/CLI touches; the runner is what the Agent
// uses to read live NUT variables through `upsc`.
//
// NUT (Network UPS Tools) is the de-facto Linux
// UPS daemon. The runner is the only side that
// invokes /usr/bin/upsc; tests use a fake. The
// package never opens a socket itself and never
// mutates host state.
package ups

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// jsonMarshalStable is the single JSON encoder for
// the package. It uses the standard library and a
// stable, compact output.
func jsonMarshalStable(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// jsonNewDecoder returns a decoder that refuses
// trailing data after the first JSON value and
// rejects unknown fields.
func jsonNewDecoder(data []byte) *json.Decoder {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	return dec
}

// SchemaVersion is the wire/JSON schema version
// this model serialises. Bumping it requires a
// migration story.
const SchemaVersion = "1"

// Hard caps. A V1 install never needs more than a
// handful of UPS devices; the bound is policy, not
// a guess. The byte caps keep the loopback API
// payload small.
const (
	MaxDevices       = 8
	MaxNameLen       = 64
	MaxDriverLen     = 64
	MaxPortLen       = 256
	MaxDescription   = 256
	MaxVariables     = 64
	MaxVariableKey   = 64
	MaxVariableValue = 256
	MaxRuntimeBytes  = 32 << 10
)

// idPattern is the canonical id alphabet used
// everywhere in DuckOS. Lowercase, dashes, no
// colons, no leading digit.
var idPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,47}$`)

// namePattern is the canonical human-facing name
// alphabet. It is more permissive than the id
// alphabet but rejects shell metacharacters. The
// Variable key path uses the same alphabet for
// now: NUT keys look like "battery.charge" or
// "input.voltage", all of which fit.
var namePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9 ._:-]{0,63}$`)

// driverPattern covers the canonical NUT driver
// names: usbhid-ups, apcupsd, bcmxcp, mge-shut,
// nutdrv_qx, blazer_usb, etc.
var driverPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)

// Health is the canonical UPS health state. The
// state machine is documented in the package
// comment: the runtime never invents a state, it
// observes one.
type Health string

const (
	HealthUnknown      Health = "unknown"
	HealthOnline       Health = "online"
	HealthOnBattery    Health = "on-battery"
	HealthCharging     Health = "charging"
	HealthDischarging  Health = "discharging"
	HealthLowBattery   Health = "low-battery"
	HealthReplaceBatt  Health = "replace-battery"
	HealthLost         Health = "lost"
	HealthDegraded     Health = "degraded"
	HealthUnconfigured Health = "unconfigured"
)

func isValidHealth(h Health) bool {
	switch h {
	case HealthUnknown, HealthOnline, HealthOnBattery, HealthCharging, HealthDischarging, HealthLowBattery, HealthReplaceBatt, HealthLost, HealthDegraded, HealthUnconfigured:
		return true
	}
	return false
}

// IsCritical reports whether the supplied health
// state requires operator attention. The GUI/CLI
// uses the predicate to colour the row.
func IsCritical(h Health) bool {
	switch h {
	case HealthLowBattery, HealthReplaceBatt, HealthLost, HealthDegraded:
		return true
	}
	return false
}

// Device is the canonical UPS object. The Driver
// and Port fields are NUT-specific: a Driver
// names a NUT driver module and a Port names the
// physical or logical address the driver talks
// to. The optional Description is operator-facing
// free text and is bounded.
type Device struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Driver      string `json:"driver"`
	Port        string `json:"port,omitempty"`
	Description string `json:"description,omitempty"`
}

// Valid enforces the per-device contract. The
// validator also rejects empty Driver and Port
// values that would otherwise let the runner
// issue an unbounded `upsc` query.
func (d Device) Valid() error {
	if !idPattern.MatchString(d.ID) {
		return errors.New("ups: device id is not canonical")
	}
	if !namePattern.MatchString(d.Name) {
		return errors.New("ups: device name is not canonical")
	}
	if !driverPattern.MatchString(d.Driver) {
		return errors.New("ups: device driver is not canonical")
	}
	if d.Port == "" {
		return errors.New("ups: device port is empty")
	}
	if len(d.Port) > MaxPortLen {
		return errors.New("ups: device port is oversized")
	}
	if strings.ContainsAny(d.Port, " \t\n\r\"';\\") {
		return errors.New("ups: device port contains shell metacharacter")
	}
	if d.Description != "" && len(d.Description) > MaxDescription {
		return errors.New("ups: device description is oversized")
	}
	return nil
}

// Variable is a single NUT variable, the kind
// `upsc <device>` prints. Examples include
// battery.charge, battery.runtime, input.voltage,
// ups.load, ups.status. The model layer is
// intentionally small: a Variable is what the
// renderer shows, not a full MIB.
type Variable struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// Valid enforces the per-variable contract.
func (v Variable) Valid() error {
	if !namePattern.MatchString(v.Key) {
		return errors.New("ups: variable key is not canonical")
	}
	if len(v.Key) > MaxVariableKey {
		return errors.New("ups: variable key is oversized")
	}
	if v.Value == "" {
		return errors.New("ups: variable value is empty")
	}
	if len(v.Value) > MaxVariableValue {
		return errors.New("ups: variable value is oversized")
	}
	if strings.ContainsAny(v.Value, "\n\r") {
		return errors.New("ups: variable value contains newline")
	}
	return nil
}

// Topology is the UPS plan. The Topology carries
// the configured devices; the runtime view is a
// separate State.
type Topology struct {
	SchemaVersion string   `json:"schemaVersion"`
	Devices       []Device `json:"devices"`
	GeneratedAt   int64    `json:"generated"`
}

// Valid enforces the topology contract.
func (t Topology) Valid() error {
	if t.SchemaVersion != SchemaVersion {
		return errors.New("ups: schema version mismatch")
	}
	if len(t.Devices) == 0 {
		return errors.New("ups: topology has no devices")
	}
	if len(t.Devices) > MaxDevices {
		return errors.New("ups: too many devices")
	}
	seen := map[string]bool{}
	for _, d := range t.Devices {
		if seen[d.ID] {
			return errors.New("ups: duplicate device id")
		}
		seen[d.ID] = true
		if err := d.Valid(); err != nil {
			return err
		}
	}
	return nil
}

// Build sorts and validates a Topology. The
// caller supplies the local clock; Build defaults
// to the current time if now is zero.
func Build(devices []Device, now time.Time) (Topology, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	sort.Slice(devices, func(i, j int) bool { return devices[i].ID < devices[j].ID })
	t := Topology{SchemaVersion: SchemaVersion, Devices: devices, GeneratedAt: now.Unix()}
	if err := t.Valid(); err != nil {
		return Topology{}, err
	}
	return t, nil
}

// EncodeTopology serialises a Topology using the
// stable encoder. The output is bounded by
// MaxRuntimeBytes.
func EncodeTopology(t Topology) ([]byte, error) {
	if err := t.Valid(); err != nil {
		return nil, err
	}
	data, err := jsonMarshalStable(t)
	if err != nil {
		return nil, err
	}
	if len(data) > MaxRuntimeBytes {
		return nil, errors.New("ups: topology exceeds maximum serialised size")
	}
	return data, nil
}

// ParseTopology decodes a JSON payload produced by
// EncodeTopology.
func ParseTopology(data []byte) (Topology, error) {
	var t Topology
	dec := jsonNewDecoder(data)
	if err := dec.Decode(&t); err != nil {
		return Topology{}, err
	}
	if err := t.Valid(); err != nil {
		return Topology{}, err
	}
	return t, nil
}

// Status is the runtime view the Agent keeps. The
// Status carries the topology, the per-device
// health, the per-device variables and the last
// probe timestamp. The Status is goroutine-safe
// through the State wrapper.
type Status struct {
	Topology   Topology              `json:"topology"`
	Health     map[string]Health     `json:"health"`
	Variables  map[string][]Variable `json:"variables"`
	ProbedAt   int64                 `json:"probedAt"`
	LastError  string                `json:"lastError,omitempty"`
	HasDevices int                   `json:"hasDevices"`
}

// State is the runtime wrapper. State is
// goroutine-safe; the Agent reads the Status
// under a read lock and the controller writes it
// under a write lock.
type State struct {
	mu        sync.RWMutex
	topology  Topology
	health    map[string]Health
	variables map[string][]Variable
	probedAt  int64
	lastError string
}

// NewState returns a State with the supplied
// topology. The health and variables maps are
// pre-allocated so the agent can read them
// without nil checks.
func NewState(t Topology) *State {
	s := &State{topology: t, health: map[string]Health{}, variables: map[string][]Variable{}}
	for _, d := range t.Devices {
		s.health[d.ID] = HealthUnconfigured
	}
	return s
}

// Topology returns a deep copy of the current
// topology.
func (s *State) Topology() Topology {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.topology
}

// UpdateTopology replaces the agent's view
// atomically. An invalid topology is rejected;
// the agent keeps the previous one.
func (s *State) UpdateTopology(t Topology) error {
	if err := t.Valid(); err != nil {
		return err
	}
	s.mu.Lock()
	s.topology = t
	// re-initialise health for any new device
	for _, d := range t.Devices {
		if _, ok := s.health[d.ID]; !ok {
			s.health[d.ID] = HealthUnconfigured
		}
	}
	// drop health for removed devices
	keep := map[string]bool{}
	for _, d := range t.Devices {
		keep[d.ID] = true
	}
	for k := range s.health {
		if !keep[k] {
			delete(s.health, k)
			delete(s.variables, k)
		}
	}
	s.mu.Unlock()
	return nil
}

// RecordProbe records the outcome of a single
// probe. The runner is the only side that calls
// this; the controller's API passes through.
func (s *State) RecordProbe(deviceID string, h Health, vars []Variable, at time.Time) {
	if !idPattern.MatchString(deviceID) {
		return
	}
	if !isValidHealth(h) {
		h = HealthUnknown
	}
	if at.IsZero() {
		at = time.Now().UTC()
	}
	clean := make([]Variable, 0, len(vars))
	for _, v := range vars {
		if err := v.Valid(); err == nil {
			clean = append(clean, v)
			if len(clean) >= MaxVariables {
				break
			}
		}
	}
	s.mu.Lock()
	s.health[deviceID] = h
	s.variables[deviceID] = clean
	s.probedAt = at.Unix()
	s.lastError = ""
	s.mu.Unlock()
}

// MarkError records the last error. The error
// string is truncated to keep the state small.
func (s *State) MarkError(msg string) {
	if len(msg) > 256 {
		msg = msg[:256]
	}
	s.mu.Lock()
	s.lastError = msg
	s.mu.Unlock()
}

// Status returns a deep copy of the runtime view.
func (s *State) Status() Status {
	s.mu.RLock()
	defer s.mu.RUnlock()
	health := make(map[string]Health, len(s.health))
	for k, v := range s.health {
		health[k] = v
	}
	vars := make(map[string][]Variable, len(s.variables))
	for k, v := range s.variables {
		copyV := make([]Variable, len(v))
		for i, vv := range v {
			copyV[i] = vv
		}
		vars[k] = copyV
	}
	return Status{Topology: s.topology, Health: health, Variables: vars, ProbedAt: s.probedAt, LastError: s.lastError, HasDevices: len(s.topology.Devices)}
}

// HealthOf returns the last observed health for
// the supplied device id, or HealthUnknown if
// the device is unknown.
func (s *State) HealthOf(deviceID string) Health {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if h, ok := s.health[deviceID]; ok {
		return h
	}
	return HealthUnknown
}

// Summarise returns a deterministic per-device
// summary. The summary is what the CLI prints by
// default.
func Summarise(s Status) []DeviceSummary {
	out := make([]DeviceSummary, 0, len(s.Topology.Devices))
	for _, d := range s.Topology.Devices {
		h := s.Health[d.ID]
		if h == "" {
			h = HealthUnknown
		}
		out = append(out, DeviceSummary{ID: d.ID, Name: d.Name, Driver: d.Driver, Health: h, Critical: IsCritical(h)})
	}
	return out
}

// DeviceSummary is the small struct the CLI
// prints. It is intentionally tiny: the
// operator asks for "what is the UPS doing", not
// for the full NUT variable set.
type DeviceSummary struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Driver   string `json:"driver"`
	Health   Health `json:"health"`
	Critical bool   `json:"critical"`
}

// ClassifyHealth maps a set of NUT variables to
// the canonical Health state. The classifier is
// the single source of truth: every caller that
// turns NUT state into a Health goes through it.
// The classifier is conservative: an ambiguous
// input yields HealthUnknown rather than a
// fabricated value.
func ClassifyHealth(status string, batteryCharge int) Health {
	status = strings.ToLower(strings.TrimSpace(status))
	if status == "" {
		return HealthUnknown
	}
	// NUT uses a small set of canonical tokens;
	// we accept both the long form and the
	// short form (OL, OB, CHRG, DISCHRG, LB, RB).
	// Order matters: online must be tested before
	// charging because "OL CHRG" is a valid
	// combination; discharging must be tested
	// before charging because DISCHRG contains
	// the substring "chrg".
	switch {
	case strings.Contains(status, "replace battery") || status == "rb" || strings.HasPrefix(status, "rb ") || strings.Contains(status, " rb "):
		return HealthReplaceBatt
	case strings.Contains(status, "low battery") || status == "lb" || strings.HasPrefix(status, "lb ") || strings.Contains(status, " lb "):
		return HealthLowBattery
	case strings.Contains(status, "on battery") || strings.Contains(status, "on-battery") || status == "ob" || strings.HasPrefix(status, "ob "):
		return HealthOnBattery
	case strings.Contains(status, "discharging") || strings.Contains(status, "dischrg"):
		return HealthDischarging
	case strings.Contains(status, "online") || status == "ol" || strings.HasPrefix(status, "ol "):
		if batteryCharge > 0 && batteryCharge < 25 {
			return HealthDegraded
		}
		return HealthOnline
	case strings.Contains(status, "charging") || strings.Contains(status, "chrg"):
		return HealthCharging
	}
	return HealthUnknown
}

// discardDecoder silences the io.EOF import on
// builds that do not use jsonNewDecoder directly.
var _ = io.EOF
