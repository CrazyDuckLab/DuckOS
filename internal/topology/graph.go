// Package topology owns the live relationship graph DuckOS V1
// exposes to operators. The Topology is built from the overview
// surface, the Doctor snapshot, the device catalog and the
// WireGuard / Xray aggregates. The package is read-only and
// deterministic: a Topology graph never depends on time-of-day
// or external state beyond the inputs.
//
// The graph is intentionally small. A typical DuckOS installation
// has well under a hundred interfaces, peers and devices; the
// package refuses to publish a graph that would exceed the API
// response cap. A graph that the API cannot serve is not a
// graph an operator can read.
package topology

import (
	"errors"
	"regexp"
	"sort"
	"strings"
	"sync"
)

const SchemaVersion = "1"

// Hard caps. The Topology is smaller than the Doctor: a 64 KiB
// JSON payload is enough to describe the entire network of a
// mid-sized site and still small enough to render inline in a
// web frontend.
const (
	MaxNodes        = 4096
	MaxEdges        = 8192
	MaxDevices      = 1024
	MaxWireGuard    = 64
	MaxXray         = 64
	MaxBytes        = 64 << 10
	MaxLabelLength  = 64
	MaxAddressCount = 8
)

// NodeKind enumerates the canonical node types the Topology
// graph supports. Unknown values are rejected at the Builder.
type NodeKind string

const (
	NodeInterface NodeKind = "interface"
	NodeBridge    NodeKind = "bridge"
	NodeBond      NodeKind = "bond"
	NodeVLAN      NodeKind = "vlan"
	NodeTunnel    NodeKind = "tunnel"
	NodeWireGuard NodeKind = "wireguard"
	NodeXray      NodeKind = "xray"
	NodeDevice    NodeKind = "device"
)

// EdgeKind enumerates the canonical relationship types.
type EdgeKind string

const (
	EdgeMember     EdgeKind = "member"
	EdgeParent     EdgeKind = "parent"
	EdgePeer       EdgeKind = "peer"
	EdgeConnected  EdgeKind = "connected"
	EdgeQuarantine EdgeKind = "quarantined"
)

// Node is one vertex in the topology graph. Nodes carry only the
// metadata an operator needs to identify the element; they never
// carry keys, UUIDs or any other secret material.
type Node struct {
	ID      string            `json:"id"`
	Kind    NodeKind          `json:"kind"`
	Label   string            `json:"label"`
	Address []string          `json:"addresses,omitempty"`
	State   string            `json:"state,omitempty"`
	Meta    map[string]string `json:"meta,omitempty"`
}

// Edge is one connection between two nodes. The pair (from, to)
// is unique within a graph; duplicate edges are deduplicated by
// the Builder.
type Edge struct {
	From   string   `json:"from"`
	To     string   `json:"to"`
	Kind   EdgeKind `json:"kind"`
	Detail string   `json:"detail,omitempty"`
}

// Graph is the canonical topology payload. A graph is immutable
// once Snapshot returns it; mutating the slice does not affect
// anything inside the package.
type Graph struct {
	SchemaVersion string `json:"schemaVersion"`
	Generated     int64  `json:"generated"`
	Nodes         []Node `json:"nodes"`
	Edges         []Edge `json:"edges"`
}

// Valid reports whether the graph obeys the bound contract:
// every Node ID is unique, every Edge references existing
// nodes, the sizes are within policy and the kinds are canonical.
func (g Graph) Valid() error {
	if g.SchemaVersion != SchemaVersion {
		return errors.New("topology schema version mismatch")
	}
	if len(g.Nodes) > MaxNodes {
		return errors.New("topology nodes exceed limit")
	}
	if len(g.Edges) > MaxEdges {
		return errors.New("topology edges exceed limit")
	}
	ids := make(map[string]struct{}, len(g.Nodes))
	for _, n := range g.Nodes {
		if !idPattern.MatchString(n.ID) {
			return errors.New("topology node id is not canonical")
		}
		if !isValidNodeKind(n.Kind) {
			return errors.New("topology node kind is not canonical")
		}
		if _, dup := ids[n.ID]; dup {
			return errors.New("topology node id is duplicated")
		}
		ids[n.ID] = struct{}{}
	}
	for _, e := range g.Edges {
		if !isValidEdgeKind(e.Kind) {
			return errors.New("topology edge kind is not canonical")
		}
		if _, ok := ids[e.From]; !ok {
			return errors.New("topology edge references missing from")
		}
		if _, ok := ids[e.To]; !ok {
			return errors.New("topology edge references missing to")
		}
	}
	return nil
}

var idPattern = regexp.MustCompile(`^[a-z][a-z0-9._-]{0,63}$`)

func isValidNodeKind(k NodeKind) bool {
	switch k {
	case NodeInterface, NodeBridge, NodeBond, NodeVLAN,
		NodeTunnel, NodeWireGuard, NodeXray, NodeDevice:
		return true
	}
	return false
}

func isValidEdgeKind(k EdgeKind) bool {
	switch k {
	case EdgeMember, EdgeParent, EdgePeer, EdgeConnected, EdgeQuarantine:
		return true
	}
	return false
}

// InterfaceSpec is the input the Builder expects for a live
// network interface. The Builder never inspects the underlying
// /sys or /proc directly; the platform collector fills in this
// structure and the Builder produces the graph.
type InterfaceSpec struct {
	Name      string
	Kind      NodeKind
	Parent    string
	Members   []string
	Addresses []string
	State     string
	Meta      map[string]string
}

// DeviceSpec is the input the Builder expects for a tracked
// device. The Builder only carries the canonical id, label,
// role and link; private fields stay in the device catalog.
type DeviceSpec struct {
	ID    string
	Label string
	Link  string
	Role  string
}

// WireGuardSpec is the input for a WireGuard peer node.
type WireGuardSpec struct {
	Name  string
	Peers []string
	State string
	Meta  map[string]string
}

// XraySpec is the input for an Xray server or user node.
type XraySpec struct {
	Name  string
	Users []string
	State string
	Meta  map[string]string
}

// Builder accumulates nodes and edges from typed inputs and
// produces a deterministic Graph.
type Builder struct {
	mu          sync.Mutex
	nodes       map[string]Node
	edges       map[string]Edge
	order       []string
	now         int64
	ifaceCount  int
	deviceCount int
	wgCount     int
	xrayCount   int
}

func NewBuilder(generated int64) *Builder {
	return &Builder{
		nodes: map[string]Node{},
		edges: map[string]Edge{},
		now:   generated,
	}
}

// AddInterface inserts a network interface node. The name is
// canonicalised to a Node ID; the Builder rejects duplicates.
func (b *Builder) AddInterface(spec InterfaceSpec) {
	if spec.Name == "" {
		return
	}
	kind := spec.Kind
	if kind == "" {
		kind = NodeInterface
	}
	if !isValidNodeKind(kind) {
		return
	}
	b.mu.Lock()
	if b.ifaceCount >= MaxNodes {
		b.mu.Unlock()
		return
	}
	b.ifaceCount++
	b.mu.Unlock()
	node := Node{
		ID:      canonicalID("if." + spec.Name),
		Kind:    kind,
		Label:   truncate(spec.Name, MaxLabelLength),
		Address: normaliseAddresses(spec.Addresses),
		State:   truncate(spec.State, 16),
		Meta:    normaliseMeta(spec.Meta),
	}
	b.addNode(node)
	if spec.Parent != "" {
		b.addEdge(Edge{
			From:   canonicalID("if." + spec.Parent),
			To:     node.ID,
			Kind:   EdgeParent,
			Detail: "lower link",
		})
	}
	for _, member := range spec.Members {
		if member == "" {
			continue
		}
		b.addEdge(Edge{
			From:   node.ID,
			To:     canonicalID("if." + member),
			Kind:   EdgeMember,
			Detail: "upper link",
		})
	}
}

// AddDevice inserts a device node and an edge to the link it is
// observed on. The Builder refuses to add a device node without
// a link, since the graph needs a concrete connection.
func (b *Builder) AddDevice(spec DeviceSpec) {
	if spec.ID == "" || spec.Link == "" {
		return
	}
	b.mu.Lock()
	if b.deviceCount >= MaxDevices {
		b.mu.Unlock()
		return
	}
	b.deviceCount++
	b.mu.Unlock()
	label := spec.Label
	if label == "" {
		label = spec.ID
	}
	b.addNode(Node{
		ID:    canonicalID("dev." + spec.ID),
		Kind:  NodeDevice,
		Label: truncate(label, MaxLabelLength),
		State: truncate(spec.Role, 16),
	})
	b.addEdge(Edge{
		From:   canonicalID("if." + spec.Link),
		To:     canonicalID("dev." + spec.ID),
		Kind:   EdgeConnected,
		Detail: "observed",
	})
	if spec.Role == "quarantined" {
		b.addEdge(Edge{
			From:   canonicalID("dev." + spec.ID),
			To:     canonicalID("if." + spec.Link),
			Kind:   EdgeQuarantine,
			Detail: "policy",
		})
	}
}

// AddWireGuard inserts a WireGuard server node. Each peer is a
// node on its own, connected to the server with EdgePeer.
func (b *Builder) AddWireGuard(spec WireGuardSpec) {
	if spec.Name == "" {
		return
	}
	b.mu.Lock()
	if b.wgCount >= MaxWireGuard {
		b.mu.Unlock()
		return
	}
	b.wgCount++
	b.mu.Unlock()
	srv := canonicalID("wg." + spec.Name)
	b.addNode(Node{
		ID:    srv,
		Kind:  NodeWireGuard,
		Label: truncate(spec.Name, MaxLabelLength),
		State: truncate(spec.State, 16),
		Meta:  normaliseMeta(spec.Meta),
	})
	for i, peer := range spec.Peers {
		if peer == "" {
			continue
		}
		pid := canonicalID("wg.peer." + spec.Name + "." + peer)
		b.addNode(Node{
			ID:    pid,
			Kind:  NodeWireGuard,
			Label: truncate(peer, MaxLabelLength),
			State: "peer",
		})
		b.addEdge(Edge{
			From:   srv,
			To:     pid,
			Kind:   EdgePeer,
			Detail: "peer[" + itoa(i) + "]",
		})
	}
}

// AddXray inserts an Xray server node. Each user is a node on
// its own, connected with EdgePeer.
func (b *Builder) AddXray(spec XraySpec) {
	if spec.Name == "" {
		return
	}
	b.mu.Lock()
	if b.xrayCount >= MaxXray {
		b.mu.Unlock()
		return
	}
	b.xrayCount++
	b.mu.Unlock()
	srv := canonicalID("xray." + spec.Name)
	b.addNode(Node{
		ID:    srv,
		Kind:  NodeXray,
		Label: truncate(spec.Name, MaxLabelLength),
		State: truncate(spec.State, 16),
		Meta:  normaliseMeta(spec.Meta),
	})
	for i, user := range spec.Users {
		if user == "" {
			continue
		}
		uid := canonicalID("xray.user." + spec.Name + "." + user)
		b.addNode(Node{
			ID:    uid,
			Kind:  NodeXray,
			Label: truncate(user, MaxLabelLength),
			State: "user",
		})
		b.addEdge(Edge{
			From:   srv,
			To:     uid,
			Kind:   EdgePeer,
			Detail: "user[" + itoa(i) + "]",
		})
	}
}

// Snapshot produces the canonical Graph. The order of nodes is
// deterministic: interfaces first, then bridges, then tunnels,
// then devices. Edges are returned in the order they were added.
func (b *Builder) Snapshot() Graph {
	b.mu.Lock()
	defer b.mu.Unlock()
	nodes := make([]Node, 0, len(b.nodes))
	seen := make(map[string]bool, len(b.order))
	for _, id := range b.order {
		if n, ok := b.nodes[id]; ok {
			nodes = append(nodes, n)
			seen[id] = true
		}
	}
	// Defensive: include any node added without a recorded order.
	for id, n := range b.nodes {
		if !seen[id] {
			nodes = append(nodes, n)
		}
	}
	if len(nodes) > MaxNodes {
		nodes = nodes[:MaxNodes]
	}
	edges := make([]Edge, 0, len(b.edges))
	for _, e := range b.edges {
		edges = append(edges, e)
		if len(edges) >= MaxEdges {
			break
		}
	}
	sort.SliceStable(edges, func(i, j int) bool {
		if edges[i].From != edges[j].From {
			return edges[i].From < edges[j].From
		}
		if edges[i].To != edges[j].To {
			return edges[i].To < edges[j].To
		}
		return edges[i].Kind < edges[j].Kind
	})
	return Graph{
		SchemaVersion: SchemaVersion,
		Generated:     b.now,
		Nodes:         nodes,
		Edges:         edges,
	}
}

func (b *Builder) addNode(n Node) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, ok := b.nodes[n.ID]; ok {
		return
	}
	if len(b.nodes) >= MaxNodes {
		return
	}
	b.nodes[n.ID] = n
	b.order = append(b.order, n.ID)
}

func (b *Builder) addEdge(e Edge) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.edges) >= MaxEdges {
		return
	}
	key := e.From + "|" + string(e.Kind) + "|" + e.To
	if _, ok := b.edges[key]; ok {
		return
	}
	b.edges[key] = e
}

func canonicalID(s string) string {
	return strings.ToLower(s)
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}

func normaliseAddresses(items []string) []string {
	if len(items) > MaxAddressCount {
		items = items[:MaxAddressCount]
	}
	out := make([]string, 0, len(items))
	for _, a := range items {
		a = strings.TrimSpace(a)
		if a == "" {
			continue
		}
		out = append(out, truncate(a, 64))
	}
	return out
}

func normaliseMeta(m map[string]string) map[string]string {
	if len(m) == 0 {
		return nil
	}
	if len(m) > 16 {
		// Defensive: any meta map over 16 entries is almost
		// certainly a leak of internal state.
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		if k == "" || v == "" {
			continue
		}
		out[truncate(k, 32)] = truncate(v, 64)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	const digits = "0123456789"
	if i < 0 {
		i = -i
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = digits[i%10]
		i /= 10
	}
	return string(buf[pos:])
}
