// Package multiwan owns the multi-WAN route group, health
// probe and failover model the V1 contract requires. A
// route group is a small set of WAN interfaces that share
// a single egress path; the Agent keeps one default route
// per group, and the group's health is the disjunction of
// its members' health probes.
//
// The package is split into a model layer, an evaluator
// and a controller. The model layer is the only one the
// GUI/CLI touches. The evaluator is what runs against the
// live state. The controller is the small piece that ties
// the model and the evaluator together and is the only
// place Safe Apply, route tables and policy rules are
// mentioned.
//
// The package never opens a socket itself; the live probe
// surface is supplied by a pluggable ProbeRunner. The
// reference implementation is in speedlab; the production
// Agent will wire it through a separate, lower-privilege
// boundary.
package multiwan

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

// discardDecoder is a helper for the io.EOF check below.
var _ = io.EOF

const SchemaVersion = "1"

// Hard caps. A DuckOS V1 install with Multi-WAN never
// exceeds a handful of route groups. The bound is policy,
// not a guess: a single installer is not expected to manage
// hundreds of WAN links.
const (
	MaxGroups        = 8
	MaxMembersPer    = 4
	MaxTargetsPer    = 8
	MaxProbeInterval = 60 * time.Second
	MinProbeInterval = 1 * time.Second
	MaxProbeTimeout  = 5 * time.Second
	MaxRTTMicros     = 60 * 1_000_000 // 60 s ceiling per sample
	MaxTotalBytes    = 64 << 10
)

// Strategy enumerates the canonical failover strategies
// the model supports. Unknown strategies are rejected.
type Strategy string

const (
	StrategyActiveActive Strategy = "active-active"
	StrategyActiveBackup Strategy = "active-backup"
)

func isValidStrategy(s Strategy) bool {
	switch s {
	case StrategyActiveActive, StrategyActiveBackup:
		return true
	}
	return false
}

// HealthMethod enumerates the canonical probe methods. The
// evaluator runs only methods that are present in the
// production ProbeRunner; unknown methods are rejected at
// Validate time.
type HealthMethod string

const (
	HealthMethodICMP  HealthMethod = "icmp"  // ICMP echo (RTT)
	HealthMethodHTTPS HealthMethod = "https" // TCP+TLS RTT
)

func isValidMethod(m HealthMethod) bool {
	switch m {
	case HealthMethodICMP, HealthMethodHTTPS:
		return true
	}
	return false
}

// Member is one WAN interface inside a group. The Member
// is identified by its canonical network.Interface id; the
// evaluator resolves the id to a Linux name when running
// against live state.
type Member struct {
	ID          string `json:"id"`
	InterfaceID string `json:"interfaceId"`
	Weight      int    `json:"weight"`
	Health      string `json:"health,omitempty"`
	Reason      string `json:"reason,omitempty"`
}

// Probe is one health probe. The target is a host or URL
// the production ProbeRunner can reach. Probes run on the
// interval declared in the group; members inherit the
// group's probe set.
type Probe struct {
	Target  string        `json:"target"`
	Method  HealthMethod  `json:"method"`
	Timeout time.Duration `json:"timeout"`
}

// Group is the canonical multi-WAN route group. The Group
// owns the WAN members, the health probes, the failover
// strategy and the policy routing table id used to
// implement the failover.
type Group struct {
	ID            string        `json:"id"`
	Name          string        `json:"name"`
	Strategy      Strategy      `json:"strategy"`
	Members       []Member      `json:"members"`
	Probes        []Probe       `json:"probes"`
	Interval      time.Duration `json:"interval"`
	Table         int           `json:"table"`
	Primary       string        `json:"primary,omitempty"`
	LastSelection string        `json:"lastSelection,omitempty"`
}

// Valid enforces the per-group contract.
func (g Group) Valid() error {
	if !idPattern.MatchString(g.ID) {
		return errors.New("multiwan: group id is not canonical")
	}
	if g.Name == "" || len(g.Name) > 64 {
		return errors.New("multiwan: group name is empty or oversized")
	}
	if !isValidStrategy(g.Strategy) {
		return errors.New("multiwan: strategy is not canonical")
	}
	if len(g.Members) == 0 {
		return errors.New("multiwan: group has no members")
	}
	if len(g.Members) > MaxMembersPer {
		return errors.New("multiwan: too many members")
	}
	ids := map[string]bool{}
	ifaceIDs := map[string]bool{}
	weights := []int{}
	for _, m := range g.Members {
		if !idPattern.MatchString(m.ID) {
			return errors.New("multiwan: member id is not canonical")
		}
		if ids[m.ID] {
			return errors.New("multiwan: duplicate member id")
		}
		ids[m.ID] = true
		if !idPattern.MatchString(m.InterfaceID) {
			return errors.New("multiwan: member interfaceId is not canonical")
		}
		if ifaceIDs[m.InterfaceID] {
			return errors.New("multiwan: duplicate member interfaceId")
		}
		ifaceIDs[m.InterfaceID] = true
		if m.Weight < 1 || m.Weight > 100 {
			return errors.New("multiwan: member weight out of range")
		}
		weights = append(weights, m.Weight)
	}
	if len(g.Probes) > MaxTargetsPer {
		return errors.New("multiwan: too many probes")
	}
	for _, p := range g.Probes {
		if p.Target == "" || len(p.Target) > 128 {
			return errors.New("multiwan: probe target is empty or oversized")
		}
		if !isValidMethod(p.Method) {
			return errors.New("multiwan: probe method is not canonical")
		}
		if p.Timeout <= 0 || p.Timeout > MaxProbeTimeout {
			return errors.New("multiwan: probe timeout out of bounds")
		}
	}
	if g.Interval < MinProbeInterval || g.Interval > MaxProbeInterval {
		return errors.New("multiwan: interval out of bounds")
	}
	if g.Table < 0 || g.Table > 252 {
		return errors.New("multiwan: routing table id out of bounds")
	}
	if g.Primary != "" {
		if !idPattern.MatchString(g.Primary) {
			return errors.New("multiwan: primary is not canonical")
		}
		if !ids[g.Primary] {
			return errors.New("multiwan: primary references missing member")
		}
	}
	return nil
}

var idPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,47}$`)

// Topology is the multi-WAN plan the Agent applies. The
// topology is sorted by group id so a serialised form is
// stable across runs.
type Topology struct {
	SchemaVersion string  `json:"schemaVersion"`
	Groups        []Group `json:"groups"`
	GeneratedAt   int64   `json:"generated"`
}

// Valid enforces the topology contract.
func (t Topology) Valid() error {
	if t.SchemaVersion != SchemaVersion {
		return errors.New("multiwan: schema version mismatch")
	}
	if len(t.Groups) == 0 {
		return errors.New("multiwan: topology has no groups")
	}
	if len(t.Groups) > MaxGroups {
		return errors.New("multiwan: too many groups")
	}
	seen := map[string]bool{}
	tables := map[int]bool{}
	for _, g := range t.Groups {
		if seen[g.ID] {
			return errors.New("multiwan: duplicate group id")
		}
		seen[g.ID] = true
		if tables[g.Table] {
			return errors.New("multiwan: duplicate routing table id")
		}
		tables[g.Table] = true
		if err := g.Valid(); err != nil {
			return err
		}
	}
	return nil
}

// Build sorts a Topology and validates it.
func Build(groups []Group, now time.Time) (Topology, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].ID < groups[j].ID })
	t := Topology{SchemaVersion: SchemaVersion, Groups: groups, GeneratedAt: now.Unix()}
	if err := t.Valid(); err != nil {
		return Topology{}, err
	}
	return t, nil
}

// ProbeResult is the redacted summary a single probe
// returns. A failed probe is reported with a non-zero RTT
// and an Error string; the evaluator never sees a payload.
type ProbeResult struct {
	Target string
	Method HealthMethod
	RTTUS  int64
	OK     bool
	Error  string
	At     time.Time
}

// ProbeRunner is the contract a probe implementation must
// satisfy. The production wiring lives in speedlab; tests
// use a fake. A nil ProbeRunner is treated as a runner
// that always reports "not run".
type ProbeRunner interface {
	Probe(ctx interface{ Done() <-chan struct{} }, p Probe) ProbeResult
}

// Evaluator is the Agent-side runtime that turns a Topology
// and a probe surface into a Selection. The Evaluator is
// goroutine-safe: the snapshot methods are protected by a
// mutex and the evaluation itself is read-only against the
// topology.
type Evaluator struct {
	mu       sync.Mutex
	topology Topology
	results  map[string]ProbeResult // last per-probe result
	selects  map[string]Selection   // last per-group selection
}

// Selection is the canonical per-group failover output. A
// Selection is the contract the Engine + Agent eventually
// translate into routing/firewall state.
type Selection struct {
	GroupID    string    `json:"groupId"`
	MemberID   string    `json:"memberId"`
	Interface  string    `json:"interface"`
	Reason     string    `json:"reason"`
	SelectedAt time.Time `json:"selectedAt"`
	Health     string    `json:"health"`
}

// NewEvaluator builds an Evaluator from a topology. The
// caller must have already called Build so the topology is
// validated.
func NewEvaluator(t Topology) (*Evaluator, error) {
	if err := t.Valid(); err != nil {
		return nil, err
	}
	return &Evaluator{
		topology: t,
		results:  map[string]ProbeResult{},
		selects:  map[string]Selection{},
	}, nil
}

// Topology returns a copy of the current topology.
func (e *Evaluator) Topology() Topology {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := Topology{SchemaVersion: e.topology.SchemaVersion, Groups: append([]Group(nil), e.topology.Groups...), GeneratedAt: e.topology.GeneratedAt}
	return out
}

// UpdateTopology atomically replaces the topology. The
// caller is responsible for validating the new topology
// with Build.
func (e *Evaluator) UpdateTopology(t Topology) error {
	if err := t.Valid(); err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.topology = t
	return nil
}

// Select returns the per-group Selection the Evaluator
// would emit given the supplied probe results. The
// algorithm is the documented one:
//
//   - For each group, compute the set of members whose
//     interface is currently considered up (i.e., the
//     most recent probe for at least one target succeeded
//     inside the probe window).
//   - For active-active: pick the member with the lowest
//     average RTT across the latest results; if no probe
//     has run yet, the evaluator falls back to the group
//     Primary.
//   - For active-backup: pick Primary if it is up; if it
//     is down, pick the up member with the highest Weight
//     in stable order; if all are down, return a "degraded"
//     selection over the highest-Weight member.
func (e *Evaluator) Select(results map[string]ProbeResult) map[string]Selection {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := map[string]Selection{}
	now := time.Now().UTC()
	for _, g := range e.topology.Groups {
		// Filter to members with at least one healthy probe
		// inside the group's interval window.
		healthy := map[string]bool{}
		for _, p := range g.Probes {
			r, ok := results[p.Target]
			if !ok || !r.OK {
				continue
			}
			if now.Sub(r.At) > g.Interval+time.Second {
				continue
			}
			for _, m := range g.Members {
				healthy[m.ID] = true
			}
		}
		out[g.ID] = choose(g, healthy, now)
	}
	return out
}

// choose is the pure decision function the Select method
// uses. The function never panics on empty input.
func choose(g Group, healthy map[string]bool, now time.Time) Selection {
	members := append([]Member(nil), g.Members...)
	switch g.Strategy {
	case StrategyActiveActive:
		// active-active: prefer a healthy member; tie-break
		// by weight, then by id for determinism. The pure
		// form does not consult probe RTTs because the
		// Evaluator already filtered by healthy.
		ordered := make([]Member, 0, len(members))
		for _, m := range members {
			if healthy[m.ID] {
				ordered = append(ordered, m)
			}
		}
		if len(ordered) == 0 {
			return degraded(g, now)
		}
		sort.SliceStable(ordered, func(i, j int) bool {
			if ordered[i].Weight != ordered[j].Weight {
				return ordered[i].Weight > ordered[j].Weight
			}
			return ordered[i].ID < ordered[j].ID
		})
		winner := ordered[0]
		return Selection{
			GroupID:    g.ID,
			MemberID:   winner.ID,
			Interface:  winner.InterfaceID,
			Reason:     "active-active: best healthy member",
			SelectedAt: now,
			Health:     "ok",
		}
	case StrategyActiveBackup:
		// active-backup: try Primary first, then the first
		// healthy member by weight.
		if g.Primary != "" {
			for _, m := range members {
				if m.ID != g.Primary {
					continue
				}
				if healthy[m.ID] {
					return Selection{
						GroupID: g.ID, MemberID: m.ID,
						Interface:  m.InterfaceID,
						Reason:     "active-backup: primary is up",
						SelectedAt: now,
						Health:     "ok",
					}
				}
			}
		}
		ordered := make([]Member, 0, len(members))
		for _, m := range members {
			if healthy[m.ID] {
				ordered = append(ordered, m)
			}
		}
		if len(ordered) == 0 {
			return degraded(g, now)
		}
		sort.SliceStable(ordered, func(i, j int) bool {
			if ordered[i].Weight != ordered[j].Weight {
				return ordered[i].Weight > ordered[j].Weight
			}
			return ordered[i].ID < ordered[j].ID
		})
		winner := ordered[0]
		return Selection{
			GroupID: g.ID, MemberID: winner.ID,
			Interface:  winner.InterfaceID,
			Reason:     "active-backup: primary is down, secondary is up",
			SelectedAt: now,
			Health:     "ok",
		}
	}
	return degraded(g, now)
}

// degraded returns the highest-weight member in a degraded
// state. The function is the only place a Selection is
// produced when every probe failed.
func degraded(g Group, now time.Time) Selection {
	members := append([]Member(nil), g.Members...)
	sort.SliceStable(members, func(i, j int) bool {
		if members[i].Weight != members[j].Weight {
			return members[i].Weight > members[j].Weight
		}
		return members[i].ID < members[j].ID
	})
	return Selection{
		GroupID:    g.ID,
		MemberID:   members[0].ID,
		Interface:  members[0].InterfaceID,
		Reason:     "all probes failed; keeping highest-weight member for visibility",
		SelectedAt: now,
		Health:     "degraded",
	}
}

// RecordProbe stores a probe result and returns the
// resulting per-group Selection. The method is the canonical
// write path the Agent uses after a probe round.
func (e *Evaluator) RecordProbe(r ProbeResult) map[string]Selection {
	e.mu.Lock()
	if e.results == nil {
		e.results = map[string]ProbeResult{}
	}
	e.results[r.Target] = r
	results := map[string]ProbeResult{}
	for k, v := range e.results {
		results[k] = v
	}
	e.mu.Unlock()
	selections := e.Select(results)
	e.mu.Lock()
	if e.selects == nil {
		e.selects = map[string]Selection{}
	}
	for gid, sel := range selections {
		e.selects[gid] = sel
	}
	e.mu.Unlock()
	return selections
}

// LastSelections returns a copy of the most recent per-group
// selection. The function is the read path the API exposes.
func (e *Evaluator) LastSelections() map[string]Selection {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := map[string]Selection{}
	for k, v := range e.selects {
		out[k] = v
	}
	return out
}

// MemberHealth returns the most recent per-member health
// summary the Evaluator has produced. A member is "up" if
// at least one probe against it succeeded inside the
// group's interval; "down" if any probe failed; "unknown"
// if no probe has run yet.
func (e *Evaluator) MemberHealth() map[string]string {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := map[string]string{}
	for _, g := range e.topology.Groups {
		now := time.Now().UTC()
		up := map[string]int{}
		down := map[string]int{}
		for _, p := range g.Probes {
			r, ok := e.results[p.Target]
			if !ok {
				continue
			}
			if now.Sub(r.At) > g.Interval+time.Second {
				continue
			}
			for _, m := range g.Members {
				if r.OK {
					up[m.ID]++
				} else {
					down[m.ID]++
				}
			}
		}
		for _, m := range g.Members {
			switch {
			case up[m.ID] > 0 && down[m.ID] == 0:
				out[m.ID] = "up"
			case down[m.ID] > 0:
				out[m.ID] = "down"
			default:
				out[m.ID] = "unknown"
			}
		}
	}
	return out
}

// EncodeTopology is the canonical JSON encoding for a
// Topology. The encoding is stable: the same input produces
// the same bytes, regardless of map iteration order.
func EncodeTopology(t Topology) ([]byte, error) {
	if err := t.Valid(); err != nil {
		return nil, err
	}
	if len(t.Groups) == 0 {
		return nil, errors.New("multiwan: nothing to encode")
	}
	return jsonMarshalTopology(t)
}

// ParseTopology is the inverse of EncodeTopology. The
// decoder refuses unknown fields, oversized payloads and
// trailing data.
func ParseTopology(data []byte) (Topology, error) {
	if len(data) == 0 {
		return Topology{}, errors.New("multiwan: empty payload")
	}
	if len(data) > MaxTotalBytes {
		return Topology{}, errors.New("multiwan: payload exceeds size cap")
	}
	t, err := jsonUnmarshalTopology(data)
	if err != nil {
		return Topology{}, err
	}
	if err := t.Valid(); err != nil {
		return Topology{}, err
	}
	return t, nil
}

// ShortSummary returns a one-line human description of a
// Selection. The summary is for logs and the API; the full
// Selection struct is for the GUI.
func ShortSummary(s Selection) string {
	return s.GroupID + " -> " + s.MemberID + " (" + s.Interface + ", " + s.Health + ")"
}

// IsHealthy reports whether a Selection is safe to apply.
// A "degraded" selection is a "keep the path visible" move;
// the caller decides whether to actually re-route.
func IsHealthy(s Selection) bool {
	return s.Health == "ok"
}

// strings import kept for downstream consumers.
var _ = strings.HasPrefix

// jsonMarshalTopology is a thin wrapper around encoding/json
// that re-validates the topology so the caller cannot skip
// the bound contract. The wrapper is the single place the
// package touches encoding/json.
func jsonMarshalTopology(t Topology) ([]byte, error) {
	type alias Topology
	return jsonMarshalStable(alias(t))
}

// jsonUnmarshalTopology is a thin wrapper that decodes the
// canonical JSON, rejects trailing data, and returns the
// canonical Topology.
func jsonUnmarshalTopology(data []byte) (Topology, error) {
	type alias Topology
	var out alias
	dec := jsonNewDecoder(data)
	if err := dec.Decode(&out); err != nil {
		return Topology{}, err
	}
	return Topology(out), nil
}
