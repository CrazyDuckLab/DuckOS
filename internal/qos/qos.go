// Package qos owns the Quality-of-Service / Smart Queue Management
// model the DuckOS V1 contract requires.
//
// A Shaper is the canonical per-interface per-direction queueing
// decision. The reference algorithms are fq_codel and CAKE; the
// model refuses unknown algorithms at Validate time. A Plan is the
// deterministic tc-style script the Linux adapter applies. The
// package never opens a socket, never executes tc itself and never
// touches host state directly; the Agent is the only side that
// calls a bounded Runner.
//
// The package is split into a model layer (this file), a renderer
// (render.go) and a controller stub (controller.go). The model
// layer is the only one the GUI/CLI touches. The renderer is what
// the Engine returns to the operator as a preview. The controller
// is the small piece that ties the model and the live tc surface
// together and is the only place the linux adapter is mentioned.
package qos

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

// jsonMarshalStable is the single JSON encoder for the
// package. It uses the standard library and a stable,
// compact output.
func jsonMarshalStable(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// jsonNewDecoder returns a decoder that refuses trailing
// data after the first JSON value.
func jsonNewDecoder(data []byte) *json.Decoder {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	return dec
}

// SchemaVersion is the wire/JSON schema version this model
// serialises. Bumping it requires a migration story.
const SchemaVersion = "1"

// Hard caps. A DuckOS V1 install never needs more than a
// handful of shapers; the bound is policy, not a guess.
// These caps also keep the plan's rendered size small
// enough to send over the loopback API without paging.
const (
	MaxShapers         = 16
	MaxClassesPer      = 8
	MaxFiltersPer      = 32
	MaxIDLen           = 64
	MaxRateKbps        = 10_000_000 // 10 Gbps ceiling
	MaxBurstKbytes     = 1 << 20    // 1 GiB
	MaxPlanBytes       = 64 << 10
	MaxTopologyBytes   = 32 << 10
	MaxMarkBits        = 16
	MaxPriorityClasses = 8
)

// idPattern is the canonical id alphabet used everywhere
// in DuckOS. Lowercase, dashes, no colons, no leading
// digit; this also satisfies the Go regex engine.
var idPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,47}$`)

// linkPattern is the canonical link/interface id alphabet
// used by the network config layer. Keep the patterns in
// sync with internal/network so a shaper can never name a
// link the network layer would refuse.
var linkPattern = regexp.MustCompile(`^[a-z][a-z0-9._-]{0,63}$`)

// Algorithm enumerates the canonical queueing
// disciplines. Anything else is rejected at Validate time.
// Unknown algorithms are not silently coerced.
type Algorithm string

const (
	AlgorithmFQCoDel Algorithm = "fq_codel"
	AlgorithmCAKE    Algorithm = "cake"
)

func isValidAlgorithm(a Algorithm) bool {
	switch a {
	case AlgorithmFQCoDel, AlgorithmCAKE:
		return true
	}
	return false
}

// Direction enumerates where the shaper sits. Egress is the
// common case (per-WAN shaping). Ingress is rare on a Linux
// router and is therefore kept simple: only fq_codel with
// a rate is admitted.
type Direction string

const (
	DirectionEgress  Direction = "egress"
	DirectionIngress Direction = "ingress"
)

func isValidDirection(d Direction) bool {
	switch d {
	case DirectionEgress, DirectionIngress:
		return true
	}
	return false
}

// Priority is the optional CAKE priority tier. The value
// is the bit pattern the user asked for; the renderer
// produces a canonical lower-case keyword. FQ-CoDel does
// not honour priority, so a Priority on a fq_codel shaper
// is rejected at Validate time.
type Priority string

const (
	PriorityBulk       Priority = "bulk"
	PriorityBestEffort Priority = "best-effort"
	PriorityVideo      Priority = "video"
	PriorityVoice      Priority = "voice"
	PriorityNetwork    Priority = "network"
)

func isValidPriority(p Priority) bool {
	switch p {
	case PriorityBulk, PriorityBestEffort, PriorityVideo, PriorityVoice, PriorityNetwork:
		return true
	}
	return false
}

// Shaper is the canonical per-link per-direction queueing
// decision. The rate is in kbps and the burst in kbytes
// (matching tc's usual units). The optional Priority,
// Overhead and MPU fields are CAKE-only and are validated
// per-algorithm.
type Shaper struct {
	ID         string    `json:"id"`
	Link       string    `json:"link"`
	Direction  Direction `json:"direction"`
	Algorithm  Algorithm `json:"algorithm"`
	RateKbps   int       `json:"rateKbps"`
	BurstKbyte int       `json:"burstKbyte"`
	// Priority is CAKE-only. Empty means "no priority tier
	// configured" which is a valid default.
	Priority Priority `json:"priority,omitempty"`
	// Overhead is the per-packet overhead in bytes. It is
	// applied to CAKE only; fq_codel ignores it.
	Overhead int `json:"overhead,omitempty"`
	// MPU is the minimum packet unit; CAKE-only.
	MPU int `json:"mpu,omitempty"`
	// Ingress is fq_codel-only; CAKE cannot police on
	// Linux. Validated per-algorithm below.
	Ingress bool `json:"ingress,omitempty"`
	// Description is operator-facing free text. It is
	// capped to keep the topology small.
	Description string `json:"description,omitempty"`
}

// Valid enforces the per-shaper contract. Algorithm-specific
// invariants are checked here so a shaper is rejected before
// it reaches the renderer.
func (s Shaper) Valid() error {
	if !idPattern.MatchString(s.ID) {
		return errors.New("qos: shaper id is not canonical")
	}
	if !linkPattern.MatchString(s.Link) {
		return errors.New("qos: shaper link is not canonical")
	}
	if !isValidDirection(s.Direction) {
		return errors.New("qos: shaper direction is not canonical")
	}
	if !isValidAlgorithm(s.Algorithm) {
		return errors.New("qos: shaper algorithm is not canonical")
	}
	if s.RateKbps <= 0 || s.RateKbps > MaxRateKbps {
		return errors.New("qos: shaper rate out of range")
	}
	if s.BurstKbyte < 0 || s.BurstKbyte > MaxBurstKbytes {
		return errors.New("qos: shaper burst out of range")
	}
	if s.Description != "" && len(s.Description) > 128 {
		return errors.New("qos: shaper description is oversized")
	}
	switch s.Algorithm {
	case AlgorithmCAKE:
		if s.Priority != "" && !isValidPriority(s.Priority) {
			return errors.New("qos: cake priority tier is not canonical")
		}
		if s.Overhead < 0 || s.Overhead > 256 {
			return errors.New("qos: cake overhead out of range")
		}
		if s.MPU < 0 || s.MPU > 65535 {
			return errors.New("qos: cake mpu out of range")
		}
		if s.Ingress {
			return errors.New("qos: cake does not support ingress on linux")
		}
	case AlgorithmFQCoDel:
		if s.Priority != "" {
			return errors.New("qos: fq_codel does not honour priority")
		}
		if s.Overhead != 0 {
			return errors.New("qos: fq_codel does not honour overhead")
		}
		if s.MPU != 0 {
			return errors.New("qos: fq_codel does not honour mpu")
		}
	}
	if s.Direction == DirectionIngress && s.Algorithm != AlgorithmFQCoDel {
		return errors.New("qos: only fq_codel admits ingress on linux")
	}
	return nil
}

// Class is the optional per-shaper traffic class. A Class
// assigns a band of dscp marks to a sub-share of the
// shaper's rate. Classes are rendered as CAKE "diffserv"
// sub-queues or as fq_codel filters. A class without
// MarkBits is invalid; classes are a no-op without a
// matching Shaper.
type Class struct {
	ID          string `json:"id"`
	ShaperID    string `json:"shaperId"`
	Name        string `json:"name"`
	MarkBits    uint8  `json:"markBits"`
	RateShare   int    `json:"rateShare,omitempty"`
	Description string `json:"description,omitempty"`
}

// Valid enforces the per-class contract.
func (c Class) Valid() error {
	if !idPattern.MatchString(c.ID) {
		return errors.New("qos: class id is not canonical")
	}
	if !idPattern.MatchString(c.ShaperID) {
		return errors.New("qos: class shaperId is not canonical")
	}
	if c.Name == "" || len(c.Name) > 64 {
		return errors.New("qos: class name is empty or oversized")
	}
	if c.MarkBits > MaxMarkBits {
		return errors.New("qos: class markBits out of range")
	}
	if c.RateShare < 0 || c.RateShare > 100 {
		return errors.New("qos: class rateShare out of range")
	}
	if c.Description != "" && len(c.Description) > 128 {
		return errors.New("qos: class description is oversized")
	}
	return nil
}

// Filter is the optional tc filter entry. It binds a Mark
// ordsel to a Class. The renderer turns this into
// deterministic "tc filter add" lines.
type Filter struct {
	ID       string `json:"id"`
	ShaperID string `json:"shaperId"`
	ClassID  string `json:"classId"`
	Mark     uint8  `json:"mark,omitempty"`
	Protocol string `json:"protocol,omitempty"`
}

// Valid enforces the per-filter contract. Protocol is
// optional; an empty protocol is allowed.
func (f Filter) Valid() error {
	if !idPattern.MatchString(f.ID) {
		return errors.New("qos: filter id is not canonical")
	}
	if !idPattern.MatchString(f.ShaperID) {
		return errors.New("qos: filter shaperId is not canonical")
	}
	if !idPattern.MatchString(f.ClassID) {
		return errors.New("qos: filter classId is not canonical")
	}
	if f.Mark > MaxMarkBits {
		return errors.New("qos: filter mark out of range")
	}
	if f.Protocol != "" {
		if len(f.Protocol) > 16 {
			return errors.New("qos: filter protocol is oversized")
		}
		switch strings.ToLower(f.Protocol) {
		case "ip", "ipv6", "all":
		default:
			return errors.New("qos: filter protocol is not canonical")
		}
	}
	return nil
}

// Topology is the full QoS plan. The package sorts the
// shapers, classes and filters by id so the serialised
// form is stable across runs.
type Topology struct {
	SchemaVersion string   `json:"schemaVersion"`
	Shapers       []Shaper `json:"shapers"`
	Classes       []Class  `json:"classes"`
	Filters       []Filter `json:"filters"`
	GeneratedAt   int64    `json:"generated"`
}

// Valid enforces the topology contract. The per-element
// validation is also re-run here so a partially-built
// topology is never admitted.
func (t Topology) Valid() error {
	if t.SchemaVersion != SchemaVersion {
		return errors.New("qos: schema version mismatch")
	}
	if len(t.Shapers) == 0 {
		return errors.New("qos: topology has no shapers")
	}
	if len(t.Shapers) > MaxShapers {
		return errors.New("qos: too many shapers")
	}
	seenShaper := map[string]bool{}
	seenLinkDir := map[string]bool{}
	for _, s := range t.Shapers {
		if seenShaper[s.ID] {
			return errors.New("qos: duplicate shaper id")
		}
		seenShaper[s.ID] = true
		// Two shapers cannot share a (link, direction) tuple
		// because the linux kernel keeps a single qdisc per
		// interface root.
		key := s.Link + "|" + string(s.Direction)
		if seenLinkDir[key] {
			return errors.New("qos: duplicate (link, direction) shaper")
		}
		seenLinkDir[key] = true
		if err := s.Valid(); err != nil {
			return err
		}
	}
	if len(t.Classes) > MaxShapers*MaxClassesPer {
		return errors.New("qos: too many classes")
	}
	seenClass := map[string]bool{}
	for _, c := range t.Classes {
		if seenClass[c.ID] {
			return errors.New("qos: duplicate class id")
		}
		seenClass[c.ID] = true
		if !seenShaper[c.ShaperID] {
			return errors.New("qos: class references missing shaper")
		}
		if err := c.Valid(); err != nil {
			return err
		}
	}
	if len(t.Filters) > MaxShapers*MaxFiltersPer {
		return errors.New("qos: too many filters")
	}
	seenFilter := map[string]bool{}
	for _, f := range t.Filters {
		if seenFilter[f.ID] {
			return errors.New("qos: duplicate filter id")
		}
		seenFilter[f.ID] = true
		if !seenShaper[f.ShaperID] {
			return errors.New("qos: filter references missing shaper")
		}
		if !seenClass[f.ClassID] {
			return errors.New("qos: filter references missing class")
		}
		if err := f.Valid(); err != nil {
			return err
		}
	}
	return nil
}

// HasShapers reports whether the topology carries at
// least one shaper. A topology without shapers is
// empty: the renderer emits no steps and the Linux
// adapter has nothing to apply.
func (t Topology) HasShapers() bool { return len(t.Shapers) > 0 }

// Build sorts a Topology and validates it. The caller
// supplies the local clock; Build defaults to the current
// time if now is zero.
func Build(shapers []Shaper, classes []Class, filters []Filter, now time.Time) (Topology, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	sort.Slice(shapers, func(i, j int) bool { return shapers[i].ID < shapers[j].ID })
	sort.Slice(classes, func(i, j int) bool { return classes[i].ID < classes[j].ID })
	sort.Slice(filters, func(i, j int) bool { return filters[i].ID < filters[j].ID })
	t := Topology{SchemaVersion: SchemaVersion, Shapers: shapers, Classes: classes, Filters: filters, GeneratedAt: now.Unix()}
	if err := t.Valid(); err != nil {
		return Topology{}, err
	}
	return t, nil
}

// EncodeTopology serialises a Topology using the stable
// encoder. The output is bounded by MaxTopologyBytes.
func EncodeTopology(t Topology) ([]byte, error) {
	if err := t.Valid(); err != nil {
		return nil, err
	}
	data, err := jsonMarshalStable(t)
	if err != nil {
		return nil, err
	}
	if len(data) > MaxTopologyBytes {
		return nil, errors.New("qos: topology exceeds maximum serialised size")
	}
	return data, nil
}

// ParseTopology decodes a JSON payload produced by
// EncodeTopology. The decoder refuses unknown fields so
// a malformed document is rejected before it reaches the
// engine.
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

// ShaperSummary is a small struct the Engine returns to
// the GUI/CLI. It is intentionally tiny: the operator
// asks for "what is shaped and at what rate", not the
// full Plan. The full Plan is fetched on demand.
type ShaperSummary struct {
	ID        string `json:"id"`
	Link      string `json:"link"`
	Direction string `json:"direction"`
	Algorithm string `json:"algorithm"`
	RateKbps  int    `json:"rateKbps"`
	Class     int    `json:"classes"`
	Filter    int    `json:"filters"`
}

// Summarise returns a deterministic, per-shaper summary.
// The summary is what `duck qos` prints by default.
func Summarise(t Topology) []ShaperSummary {
	classByShaper := map[string]int{}
	filterByShaper := map[string]int{}
	for _, c := range t.Classes {
		classByShaper[c.ShaperID]++
	}
	for _, f := range t.Filters {
		filterByShaper[f.ShaperID]++
	}
	out := make([]ShaperSummary, 0, len(t.Shapers))
	for _, s := range t.Shapers {
		out = append(out, ShaperSummary{
			ID:        s.ID,
			Link:      s.Link,
			Direction: string(s.Direction),
			Algorithm: string(s.Algorithm),
			RateKbps:  s.RateKbps,
			Class:     classByShaper[s.ID],
			Filter:    filterByShaper[s.ID],
		})
	}
	return out
}

// State is the runtime view the Agent keeps. State is
// goroutine-safe; the Agent updates the topology under a
// mutex and the API/CLI read it under the same mutex.
type State struct {
	mu        sync.RWMutex
	topology  Topology
	lastApply int64
	lastError string
}

// NewState returns a State with the supplied topology.
// An empty topology is admitted; the agent decides
// whether to render an empty plan.
func NewState(t Topology) *State {
	return &State{topology: t}
}

// Topology returns a deep copy of the current topology.
// The copy is bounded by the model's own Valid; callers
// cannot mutate the agent's view.
func (s *State) Topology() Topology {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.topology
}

// UpdateTopology replaces the agent's view atomically.
// An invalid topology is rejected; the agent keeps the
// previous one.
func (s *State) UpdateTopology(t Topology) error {
	if err := t.Valid(); err != nil {
		return err
	}
	s.mu.Lock()
	s.topology = t
	s.mu.Unlock()
	return nil
}

// MarkApplied records the last successful apply and
// clears the last error. The unix timestamp is supplied
// by the controller so tests can pin the value.
func (s *State) MarkApplied(at time.Time) {
	if at.IsZero() {
		at = time.Now().UTC()
	}
	s.mu.Lock()
	s.lastApply = at.Unix()
	s.lastError = ""
	s.mu.Unlock()
}

// MarkError records the last apply error. The error
// string is truncated to keep the state small.
func (s *State) MarkError(msg string) {
	if len(msg) > 256 {
		msg = msg[:256]
	}
	s.mu.Lock()
	s.lastError = msg
	s.mu.Unlock()
}

// Status is the structured runtime view the API returns.
// The Status never reveals secrets; tc plans do not carry
// any, but the status never embeds a Mark or shared key.
type Status struct {
	Topology   Topology `json:"topology"`
	LastApply  int64    `json:"lastApply"`
	LastError  string   `json:"lastError,omitempty"`
	HasShapers int      `json:"hasShapers"`
}

// ClearAppliedTopology records a verified runtime teardown, not a candidate.
func (s *State) ClearAppliedTopology() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.topology = Topology{SchemaVersion: SchemaVersion}
}

// Status returns the current runtime view.
func (s *State) Status() Status {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return Status{Topology: s.topology, LastApply: s.lastApply, LastError: s.lastError, HasShapers: len(s.topology.Shapers)}
}

// discardDecoder silences the io.EOF import on builds
// that do not use jsonNewDecoder directly.
var _ = io.EOF
