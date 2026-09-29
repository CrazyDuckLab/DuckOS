// topology/evidence.go: G09 topology evidence overlay.
//
// The base Graph in graph.go reports the current state of
// every node and edge but carries no provenance — the
// operator cannot tell whether a state was observed live
// or copied from the journal. This file adds an Evidence
// overlay that records:
//
//   - Source: which subsystem produced the entry
//     (overview, doctor, device-catalog, netlink, etc.)
//   - ObservedAt: when the underlying probe ran
//   - Confidence: a normalised 0..1 score; the operator
//     can show "fresh" vs "stale" badges from this field
//     without re-deriving anything client-side
//
// Evidence is an additive field. Existing Graph consumers
// continue to work; new consumers can opt into the
// freshness rules. The package never drops the base
// fields, so the wire format stays backward compatible.
package topology

import (
	"sort"
	"sync"
	"time"
)

// EvidenceSource enumerates the canonical provenance
// sources for topology evidence. New sources must be
// appended to preserve wire-format stability.
type EvidenceSource string

const (
	SourceOverview      EvidenceSource = "overview"
	SourceDoctor        EvidenceSource = "doctor"
	SourceDeviceCatalog EvidenceSource = "device-catalog"
	SourceNetlink       EvidenceSource = "netlink"
	SourceWireGuard     EvidenceSource = "wireguard"
	SourceXray          EvidenceSource = "xray"
	SourceMultiWAN      EvidenceSource = "multiwan"
	SourceManual        EvidenceSource = "manual"
)

// Valid reports whether s is a known source.
func (s EvidenceSource) Valid() bool {
	switch s {
	case SourceOverview, SourceDoctor, SourceDeviceCatalog,
		SourceNetlink, SourceWireGuard, SourceXray,
		SourceMultiWAN, SourceManual:
		return true
	}
	return false
}

// Evidence is the provenance record attached to a Node or
// Edge. Confidence is clamped to [0, 1] on Write; the
// overlay never carries out-of-range values.
type Evidence struct {
	Source     EvidenceSource `json:"source"`
	ObservedAt time.Time      `json:"observedAt"`
	Confidence float64        `json:"confidence"`
	// Note is a short, operator-facing reason (e.g.
	// "kernel rtnetlink link query", "FRR zebra state").
	Note string `json:"note,omitempty"`
}

// Valid reports whether e is well-formed.
func (e Evidence) Valid() bool {
	if !e.Source.Valid() {
		return false
	}
	if e.ObservedAt.IsZero() {
		return false
	}
	if e.Confidence < 0 || e.Confidence > 1 {
		return false
	}
	return true
}

// StaleThreshold is the duration after which evidence is
// considered stale. Callers may override via SetStale.
const StaleThreshold = 90 * time.Second

// EvidenceStore overlays a Graph with per-node and
// per-edge provenance. The store is goroutine-safe and
// bounded by the underlying graph's node/edge counts.
type EvidenceStore struct {
	mu         sync.RWMutex
	nodes      map[string]Evidence
	edges      map[edgeKey]Evidence
	staleAfter time.Duration
}

// NewEvidenceStore returns an empty store.
func NewEvidenceStore() *EvidenceStore {
	return &EvidenceStore{
		nodes:      map[string]Evidence{},
		edges:      map[edgeKey]Evidence{},
		staleAfter: StaleThreshold,
	}
}

// SetStale overrides the staleness threshold.
func (s *EvidenceStore) SetStale(d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if d <= 0 {
		d = StaleThreshold
	}
	s.staleAfter = d
}

// StaleAfter returns the current staleness threshold.
func (s *EvidenceStore) StaleAfter() time.Duration {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.staleAfter
}

// edgeKey is the canonical map key for an edge.
type edgeKey struct {
	from string
	to   string
	kind string
}

// SetNode records evidence for a node. Existing entries
// are replaced. Confidence is clamped to [0, 1].
func (s *EvidenceStore) SetNode(id string, ev Evidence) {
	if id == "" {
		return
	}
	if ev.Confidence < 0 {
		ev.Confidence = 0
	} else if ev.Confidence > 1 {
		ev.Confidence = 1
	}
	if ev.ObservedAt.IsZero() {
		ev.ObservedAt = time.Now().UTC()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nodes[id] = ev
}

// SetEdge records evidence for an edge.
func (s *EvidenceStore) SetEdge(from, to, kind string, ev Evidence) {
	if from == "" || to == "" || kind == "" {
		return
	}
	if ev.Confidence < 0 {
		ev.Confidence = 0
	} else if ev.Confidence > 1 {
		ev.Confidence = 1
	}
	if ev.ObservedAt.IsZero() {
		ev.ObservedAt = time.Now().UTC()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.edges[edgeKey{from: from, to: to, kind: kind}] = ev
}

// NodeEvidence returns the evidence for a node.
func (s *EvidenceStore) NodeEvidence(id string) (Evidence, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ev, ok := s.nodes[id]
	return ev, ok
}

// EdgeEvidence returns the evidence for an edge.
func (s *EvidenceStore) EdgeEvidence(from, to, kind string) (Evidence, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ev, ok := s.edges[edgeKey{from: from, to: to, kind: kind}]
	return ev, ok
}

// AllNodeEvidence returns every node evidence sorted by
// id so the audit surface is deterministic.
func (s *EvidenceStore) AllNodeEvidence() []NodeEvidenceEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]NodeEvidenceEntry, 0, len(s.nodes))
	for id, ev := range s.nodes {
		out = append(out, NodeEvidenceEntry{ID: id, Evidence: ev})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// AllEdgeEvidence returns every edge evidence sorted by
// (from, to, kind).
func (s *EvidenceStore) AllEdgeEvidence() []EdgeEvidenceEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]EdgeEvidenceEntry, 0, len(s.edges))
	for k, ev := range s.edges {
		out = append(out, EdgeEvidenceEntry{From: k.from, To: k.to, Kind: k.kind, Evidence: ev})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].From != out[j].From {
			return out[i].From < out[j].From
		}
		if out[i].To != out[j].To {
			return out[i].To < out[j].To
		}
		return out[i].Kind < out[j].Kind
	})
	return out
}

// StaleNodes returns the IDs of nodes whose evidence is
// older than the configured staleness threshold. The
// list is sorted for deterministic output.
func (s *EvidenceStore) StaleNodes() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	now := time.Now().UTC()
	var stale []string
	for id, ev := range s.nodes {
		if now.Sub(ev.ObservedAt) > s.staleAfter {
			stale = append(stale, id)
		}
	}
	sort.Strings(stale)
	return stale
}

// Counters summarises the evidence store so the API can
// render a freshness badge without iterating the map.
type EvidenceCounters struct {
	Nodes      int `json:"nodes"`
	Edges      int `json:"edges"`
	StaleNodes int `json:"staleNodes"`
	Sources    int `json:"sources"`
}

// Counters returns the current counts. The function is
// cheap and may be polled.
func (s *EvidenceStore) Counters() EvidenceCounters {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sources := map[EvidenceSource]struct{}{}
	for _, ev := range s.nodes {
		sources[ev.Source] = struct{}{}
	}
	for _, ev := range s.edges {
		sources[ev.Source] = struct{}{}
	}
	now := time.Now().UTC()
	stale := 0
	for _, ev := range s.nodes {
		if now.Sub(ev.ObservedAt) > s.staleAfter {
			stale++
		}
	}
	return EvidenceCounters{
		Nodes:      len(s.nodes),
		Edges:      len(s.edges),
		StaleNodes: stale,
		Sources:    len(sources),
	}
}

// NodeEvidenceEntry pairs a node id with its evidence.
type NodeEvidenceEntry struct {
	ID       string   `json:"id"`
	Evidence Evidence `json:"evidence"`
}

// EdgeEvidenceEntry pairs an edge triple with its evidence.
type EdgeEvidenceEntry struct {
	From     string   `json:"from"`
	To       string   `json:"to"`
	Kind     string   `json:"kind"`
	Evidence Evidence `json:"evidence"`
}
