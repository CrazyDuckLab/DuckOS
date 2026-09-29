// lifecycle/dependencies.go owns the V1
// dependency-aware deletion preflight. Before
// the Agent deletes a VLAN, WAN, tunnel,
// certificate, service, or any other managed
// object, it must report every object that
// depends on the target. A silent destructive
// cascade is forbidden by the V1 contract.
//
// The Dependency model is intentionally small:
// every object the Agent owns is identified by
// (Kind, ID). Edges are (Target -> Dependent).
// A deletion preflight returns the set of
// dependents so the operator can decide.
package lifecycle

import (
	"errors"
	"regexp"
	"sort"
	"sync"
)

// ObjectKind enumerates the V1 object kinds
// the dependency graph recognises.
type ObjectKind string

const (
	KindInterface     ObjectKind = "interface"
	KindWAN           ObjectKind = "wan"
	KindVLAN          ObjectKind = "vlan"
	KindTunnel        ObjectKind = "tunnel"
	KindCertificate   ObjectKind = "certificate"
	KindService       ObjectKind = "service"
	KindXrayServer    ObjectKind = "xray-server"
	KindXrayUser      ObjectKind = "xray-user"
	KindSubscription  ObjectKind = "subscription"
	KindSite          ObjectKind = "site"
	KindWiFiAP        ObjectKind = "wifi-ap"
	KindWiFiClient    ObjectKind = "wifi-client"
	KindWireGuardPeer ObjectKind = "wireguard-peer"
)

var objectIDPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:-]{0,63}$`)

// Object is a node in the dependency graph.
type Object struct {
	Kind ObjectKind `json:"kind"`
	ID   string     `json:"id"`
}

// Valid reports whether o has a recognised
// kind and a canonical id.
func (o Object) Valid() bool {
	if o.Kind == "" || o.ID == "" {
		return false
	}
	if !objectIDPattern.MatchString(o.ID) {
		return false
	}
	switch o.Kind {
	case KindInterface, KindWAN, KindVLAN, KindTunnel, KindCertificate, KindService,
		KindXrayServer, KindXrayUser, KindSubscription, KindSite, KindWiFiAP, KindWiFiClient, KindWireGuardPeer:
		return true
	}
	return false
}

// Reference is a directed edge: the
// dependent depends on the target.
type Reference struct {
	Dependent Object `json:"dependent"`
	Target    Object `json:"target"`
	// Reason is a short, operator-facing
	// explanation (e.g. "binds 443/tls",
	// "issued for domain", "uplink"). It is
	// shown in the deletion preflight
	// response.
	Reason string `json:"reason,omitempty"`
}

// Graph is the in-memory dependency graph.
// The Graph is goroutine-safe.
type Graph struct {
	mu sync.Mutex
	// Forward: target -> dependents.
	forward map[Object]map[Object]string
	// Reverse: dependent -> targets.
	reverse map[Object]map[Object]string
	// Ownership/Provenance overlay (G08). Embedded so
	// the canonical Graph surface stays a single object.
	OwnershipStore
}

// NewGraph returns an empty Graph with the G08
// ownership/provenance overlay initialised.
func NewGraph() *Graph {
	g := &Graph{
		forward:        map[Object]map[Object]string{},
		reverse:        map[Object]map[Object]string{},
		OwnershipStore: OwnershipStore{ownership: map[Object]OwnershipRecord{}, provenance: map[Object][]ProvenanceEvent{}, maxEvents: MaxProvenanceEvents},
	}
	return g
}

// ErrSelfReference is returned by Add when
// the dependent and the target are the same
// object.
var ErrSelfReference = errors.New("lifecycle: an object cannot depend on itself")

// ErrInvalidObject is returned by Add when
// either the dependent or the target fails
// Object.Valid.
var ErrInvalidObject = errors.New("lifecycle: dependent or target is invalid")

// Add inserts a Reference into the graph.
// The function refuses self-references and
// invalid objects.
func (g *Graph) Add(ref Reference) error {
	if !ref.Dependent.Valid() || !ref.Target.Valid() {
		return ErrInvalidObject
	}
	if ref.Dependent == ref.Target {
		return ErrSelfReference
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.forward[ref.Target] == nil {
		g.forward[ref.Target] = map[Object]string{}
	}
	g.forward[ref.Target][ref.Dependent] = ref.Reason
	if g.reverse[ref.Dependent] == nil {
		g.reverse[ref.Dependent] = map[Object]string{}
	}
	g.reverse[ref.Dependent][ref.Target] = ref.Reason
	return nil
}

// Remove deletes a single Reference.
func (g *Graph) Remove(ref Reference) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if m, ok := g.forward[ref.Target]; ok {
		delete(m, ref.Dependent)
	}
	if m, ok := g.reverse[ref.Dependent]; ok {
		delete(m, ref.Target)
	}
}

// Dependents returns every object that
// depends on target, sorted for stable
// output.
func (g *Graph) Dependents(target Object) []Reference {
	g.mu.Lock()
	defer g.mu.Unlock()
	m := g.forward[target]
	out := make([]Reference, 0, len(m))
	for d, reason := range m {
		out = append(out, Reference{Dependent: d, Target: target, Reason: reason})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Dependent.Kind != out[j].Dependent.Kind {
			return out[i].Dependent.Kind < out[j].Dependent.Kind
		}
		return out[i].Dependent.ID < out[j].Dependent.ID
	})
	return out
}

// Targets returns every object that dependent
// depends on, sorted.
func (g *Graph) Targets(dependent Object) []Reference {
	g.mu.Lock()
	defer g.mu.Unlock()
	m := g.reverse[dependent]
	out := make([]Reference, 0, len(m))
	for t, reason := range m {
		out = append(out, Reference{Dependent: dependent, Target: t, Reason: reason})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Target.Kind != out[j].Target.Kind {
			return out[i].Target.Kind < out[j].Target.Kind
		}
		return out[i].Target.ID < out[j].Target.ID
	})
	return out
}

// Preflight is the deletion preflight. It
// returns a DeletionPlan that lists every
// dependent, ordered by (Kind, ID). The
// caller decides whether to cascade the
// deletion; the Graph never does it
// automatically.
type DeletionPlan struct {
	Target     Object      `json:"target"`
	Dependents []Reference `json:"dependents"`
	// HasDependents is true when at least
	// one Reference was found.
	HasDependents bool `json:"hasDependents"`
}

// Preflight returns a DeletionPlan for
// target. An invalid target returns an error.
func (g *Graph) Preflight(target Object) (DeletionPlan, error) {
	if !target.Valid() {
		return DeletionPlan{}, ErrInvalidObject
	}
	deps := g.Dependents(target)
	return DeletionPlan{Target: target, Dependents: deps, HasDependents: len(deps) > 0}, nil
}
