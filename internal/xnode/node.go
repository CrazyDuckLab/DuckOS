// Package xnode implements the Xray Center multi-node
// orchestration model: secure enrollment, mutual authentication,
// identity, capability discovery, health, desired/observed
// reconciliation and revocation for Xray nodes managed by one
// DuckOS instance.
//
// Trust model:
//
//   - Enrollment uses single-use CSPRNG enrollment tokens; only
//     SHA-256 digests are stored, mirroring the subscription
//     delivery token posture.
//   - A node authenticates with a long-lived node credential
//     (bearer over HTTPS); the credential is shown exactly once at
//     enrollment and stored only as a digest. Rotation replaces the
//     digest; the old credential stops validating immediately.
//   - Revocation is immediate and terminal: revoked nodes cannot
//     re-enroll with the same token or credential.
//   - The coordinator never serves VPN traffic itself; it is
//     purely a control-plane node (kind "local" may run its own
//     Xray runtime through the existing agent).
//
// The Manager is storage-agnostic: persistence is a hook. Remote
// communication is behind the Adapter interface so the concrete
// transport (agent loopback for the local node, authenticated HTTPS
// for remote DuckOS nodes) is injectable and testable.
package xnode

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"sync"
	"time"
)

// Limits bound the cluster model.
const (
	MaxNodes            = 64
	MaxEnrollmentTokens = 32
	maxCredentialBytes  = 32
	namePatternStr      = `^[a-z][a-z0-9._-]{0,47}$`
)

var namePattern = regexp.MustCompile(namePatternStr)

// Kind distinguishes control-plane topology.
type Kind string

const (
	// KindLocal is the DuckOS instance the coordinator runs on.
	KindLocal Kind = "local"
	// KindRemote is a remote DuckOS node enrolled over HTTPS.
	KindRemote Kind = "remote"
)

// Status is the node lifecycle state. Status is always derived from
// observed evidence, never assumed from configuration.
type Status string

const (
	StatusPending   Status = "pending"
	StatusActive    Status = "active"
	StatusSuspended Status = "suspended"
	StatusStale     Status = "stale"
	StatusRevoked   Status = "revoked"
)

// Node is one managed Xray node.
type Node struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Kind         Kind     `json:"kind"`
	Endpoint     string   `json:"endpoint,omitempty"` // https URL, remote only
	Version      string   `json:"version,omitempty"`  // discovered, not configured
	Capabilities []string `json:"capabilities,omitempty"`
	// CredentialHash is sha256(domain||credential). The plaintext
	// node credential is returned exactly once at enrollment.
	CredentialHash string    `json:"credentialHash"`
	Status         Status    `json:"status"`
	Tags           []string  `json:"tags,omitempty"`
	EnrolledAt     time.Time `json:"enrolledAt"`
	LastSeenAt     time.Time `json:"lastSeenAt,omitempty"`
	LastError      string    `json:"lastError,omitempty"`
}

// NodeInfo is the self-description a node reports at enrollment and
// on every discovery round.
type NodeInfo struct {
	Name         string   `json:"name"`
	Version      string   `json:"version"`
	Capabilities []string `json:"capabilities"`
}

func (i NodeInfo) validate() error {
	if !namePattern.MatchString(i.Name) {
		return errors.New("xnode: node name is not canonical")
	}
	if len(i.Capabilities) > 64 {
		return errors.New("xnode: too many capabilities")
	}
	for _, c := range i.Capabilities {
		if c == "" || len(c) > 64 {
			return errors.New("xnode: capability label is empty or oversized")
		}
	}
	return nil
}

// EnrollmentToken is the at-rest record of an enrollment token.
type EnrollmentToken struct {
	TokenHash string    `json:"tokenHash"`
	CreatedAt time.Time `json:"createdAt"`
	ExpiresAt time.Time `json:"expiresAt"`
	UsedBy    string    `json:"usedBy,omitempty"` // node id after use
	Used      bool      `json:"used"`
}

func hashToken(domain, token string) string {
	sum := sha256.Sum256([]byte(domain + "\x00" + token))
	return hex.EncodeToString(sum[:])
}

func newSecret() (string, error) {
	buf := make([]byte, maxCredentialBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("xnode: generate secret: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func newID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("xnode: generate id: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

// Adapter is the per-node control-plane transport. Production
// wiring: local adapter → the in-process xray runtime; remote
// adapter → authenticated HTTPS against the node's agent.
type Adapter interface {
	// Discover returns the node's live self-description.
	Discover(ctx Ctx, node Node) (NodeInfo, error)
	// PushConfig stages and applies the desired Xray JSON on the
	// node through its own safe-apply path. The coordinator never
	// writes raw process state.
	PushConfig(ctx Ctx, node Node, config []byte) error
	// Health probes liveness without mutating anything.
	Health(ctx Ctx, node Node) (time.Duration, error)
}

// Ctx aliases context.Context to keep the interface definition
// readable without importing context into every signature line.
type Ctx = interface {
	Deadline() (deadline time.Time, ok bool)
	Done() <-chan struct{}
	Err() error
	Value(key any) any
}

// Manager is the node registry and reconciliation coordinator.
type Manager struct {
	mu       sync.RWMutex
	nodes    map[string]*Node
	enroll   map[string]*EnrollmentToken
	clock    func() time.Time
	adapter  Adapter
	staleTTL time.Duration
	persist  func([]*Node) error
}

func NewManager(adapter Adapter) *Manager {
	return &Manager{
		nodes:    map[string]*Node{},
		enroll:   map[string]*EnrollmentToken{},
		clock:    time.Now,
		adapter:  adapter,
		staleTTL: 5 * time.Minute,
	}
}

// SetClock overrides the wall clock (testing).
func (m *Manager) SetClock(fn func() time.Time) { m.clock = fn }

// SetPersist installs a persistence hook invoked after every
// mutation with the sorted node list.
func (m *Manager) SetPersist(fn func([]*Node) error) { m.persist = fn }

// IssueEnrollmentToken mints a single-use enrollment token. The
// plaintext token is returned exactly once.
func (m *Manager) IssueEnrollmentToken(ttl time.Duration) (string, EnrollmentToken, error) {
	token, err := newSecret()
	if err != nil {
		return "", EnrollmentToken{}, err
	}
	now := m.clock().UTC()
	rec := EnrollmentToken{TokenHash: hashToken("xnode-enroll", token), CreatedAt: now}
	if ttl != 0 {
		rec.ExpiresAt = now.Add(ttl)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.enroll) >= MaxEnrollmentTokens {
		for h, r := range m.enroll {
			if r.Used || (!r.ExpiresAt.IsZero() && now.After(r.ExpiresAt)) {
				delete(m.enroll, h)
			}
		}
		if len(m.enroll) >= MaxEnrollmentTokens {
			return "", EnrollmentToken{}, errors.New("xnode: enrollment token capacity exceeded")
		}
	}
	m.enroll[rec.TokenHash] = &rec
	return token, rec, nil
}

// Enroll exchanges a valid enrollment token for a node identity and
// a node credential. The credential is returned exactly once and
// stored only as a digest. Single-use: the token is burned even if
// the caller drops the returned credential.
func (m *Manager) Enroll(token string, info NodeInfo) (Node, string, error) {
	if err := info.validate(); err != nil {
		return Node{}, "", err
	}
	digest := hashToken("xnode-enroll", token)
	m.mu.Lock()
	rec, ok := m.enroll[digest]
	if ok {
		now := m.clock().UTC()
		ok = !rec.Used && (rec.ExpiresAt.IsZero() || now.Before(rec.ExpiresAt))
		rec.Used = true // single-use regardless of outcome beyond this point
	}
	m.mu.Unlock()
	if !ok {
		return Node{}, "", errors.New("xnode: enrollment token is unknown, used or expired")
	}
	credential, err := newSecret()
	if err != nil {
		return Node{}, "", err
	}
	id, err := newID()
	if err != nil {
		return Node{}, "", err
	}
	now := m.clock().UTC()
	node := Node{
		ID:             id,
		Kind:           KindRemote,
		Name:           info.Name,
		Version:        info.Version,
		Capabilities:   append([]string(nil), info.Capabilities...),
		CredentialHash: hashToken("xnode-cred", credential),
		Status:         StatusActive,
		EnrolledAt:     now,
		LastSeenAt:     now,
	}
	m.mu.Lock()
	if len(m.nodes) >= MaxNodes {
		m.mu.Unlock()
		return Node{}, "", errors.New("xnode: node capacity exceeded")
	}
	m.nodes[id] = &node
	m.mu.Unlock()
	m.persistNodes()
	return node, credential, nil
}

// EnrollLocal registers the coordinator's own node. Local nodes do
// not carry credentials: trust is the in-process agent boundary.
func (m *Manager) EnrollLocal(name string) (Node, error) {
	if !namePattern.MatchString(name) {
		return Node{}, errors.New("xnode: node name is not canonical")
	}
	id, err := newID()
	if err != nil {
		return Node{}, err
	}
	now := m.clock().UTC()
	node := Node{
		ID: id, Name: name, Kind: KindLocal,
		Status: StatusActive, EnrolledAt: now, LastSeenAt: now,
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, n := range m.nodes {
		if n.Kind == KindLocal {
			return Node{}, errors.New("xnode: local node already enrolled")
		}
	}
	if len(m.nodes) >= MaxNodes {
		return Node{}, errors.New("xnode: node capacity exceeded")
	}
	m.nodes[id] = &node
	m.persistNodes()
	return node, nil
}

// Authenticate validates a node credential in constant time.
func (m *Manager) Authenticate(nodeID, credential string) bool {
	if nodeID == "" || credential == "" {
		return false
	}
	m.mu.RLock()
	node, ok := m.nodes[nodeID]
	if ok {
		match := hmac.Equal([]byte(node.CredentialHash), []byte(hashToken("xnode-cred", credential))) &&
			node.CredentialHash != "" &&
			(node.Status == StatusActive || node.Status == StatusStale)
		rc := *node
		m.mu.RUnlock()
		if !match {
			return false
		}
		m.markSeen(nodeID, rc.Version, rc.Capabilities)
		return true
	}
	m.mu.RUnlock()
	return false
}

// RotateCredential replaces a node credential. The old credential
// stops validating immediately; the new one is returned once.
func (m *Manager) RotateCredential(nodeID string) (string, error) {
	credential, err := newSecret()
	if err != nil {
		return "", err
	}
	m.mu.Lock()
	node, ok := m.nodes[nodeID]
	if !ok || node.Status == StatusRevoked {
		m.mu.Unlock()
		return "", errors.New("xnode: node is unknown or revoked")
	}
	if node.Kind == KindLocal {
		m.mu.Unlock()
		return "", errors.New("xnode: local nodes carry no credential")
	}
	node.CredentialHash = hashToken("xnode-cred", credential)
	m.mu.Unlock()
	m.persistNodes()
	return credential, nil
}

// Revoke revokes a node permanently. Revoked nodes keep their
// record for forensics but fail every authentication and
// reconciliation.
func (m *Manager) Revoke(nodeID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	node, ok := m.nodes[nodeID]
	if !ok {
		return errors.New("xnode: node is unknown")
	}
	node.Status = StatusRevoked
	node.LastError = "revoked by operator"
	m.persistNodes()
	return nil
}

// Suspend / Resume implement manual maintenance mode.
func (m *Manager) Suspend(nodeID string) error { return m.setStatus(nodeID, StatusSuspended) }
func (m *Manager) Resume(nodeID string) error  { return m.setStatus(nodeID, StatusActive) }

func (m *Manager) setStatus(nodeID string, status Status) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	node, ok := m.nodes[nodeID]
	if !ok || node.Status == StatusRevoked {
		return errors.New("xnode: node is unknown or revoked")
	}
	node.Status = status
	m.persistNodes()
	return nil
}

// MarkSeen records a successful discovery round and refreshes the
// node's version/capabilities.
func (m *Manager) markSeen(nodeID, version string, capabilities []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	node, ok := m.nodes[nodeID]
	if !ok || node.Status == StatusRevoked {
		return
	}
	node.LastSeenAt = m.clock().UTC()
	node.Version = version
	node.Capabilities = append([]string(nil), capabilities...)
	if node.Status == StatusStale || node.Status == StatusPending {
		node.Status = StatusActive
	}
}

// Get returns a node by id.
func (m *Manager) Get(nodeID string) (Node, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	node, ok := m.nodes[nodeID]
	if !ok {
		return Node{}, false
	}
	return *node, true
}

// List returns the nodes sorted by id, with staleness derived from
// the clock. Observed staleness is computed, never stored.
func (m *Manager) List() []Node {
	m.mu.RLock()
	out := make([]Node, 0, len(m.nodes))
	now := m.clock()
	for _, n := range m.nodes {
		c := *n
		if c.Status == StatusActive && c.LastSeenAt.Before(now.Add(-m.staleTTL)) {
			c.Status = StatusStale
		}
		out = append(out, c)
	}
	m.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Eligible returns active (non-stale, non-revoked, non-suspended)
// nodes eligible to receive configuration or serve subscriptions.
func (m *Manager) Eligible() []Node {
	var out []Node
	for _, n := range m.List() {
		if n.Status == StatusActive {
			out = append(out, n)
		}
	}
	return out
}

// HealthReport is the observed health of one node.
type HealthReport struct {
	NodeID    string        `json:"nodeId"`
	Reachable bool          `json:"reachable"`
	Latency   time.Duration `json:"latencyMs"`
	Error     string        `json:"error,omitempty"`
	CheckedAt time.Time     `json:"checkedAt"`
}

// Health probes one node and records the outcome.
func (m *Manager) Health(ctx Ctx, nodeID string) (HealthReport, error) {
	m.mu.RLock()
	node, ok := m.nodes[nodeID]
	m.mu.RUnlock()
	report := HealthReport{NodeID: nodeID, CheckedAt: m.clock().UTC()}
	if !ok || node.Status == StatusRevoked {
		return report, errors.New("xnode: node is unknown or revoked")
	}
	if m.adapter == nil {
		report.Error = "no adapter wired"
		return report, errors.New("xnode: adapter is not wired")
	}
	latency, err := m.adapter.Health(ctx, *node)
	if err != nil {
		report.Error = err.Error()
		m.recordError(nodeID, report.Error)
		return report, err
	}
	report.Reachable, report.Latency = true, latency
	m.markSeen(nodeID, node.Version, node.Capabilities)
	return report, nil
}

func (m *Manager) recordError(nodeID, msg string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if node, ok := m.nodes[nodeID]; ok {
		// Bound the error field: never echo unbounded remote text.
		if len(msg) > 256 {
			msg = msg[:256]
		}
		node.LastError = msg
	}
}

// Plan is a desired/observed reconciliation plan for one node.
type Plan struct {
	NodeID     string `json:"nodeId"`
	Action     string `json:"action"` // "none" | "push" | "skip"
	Reason     string `json:"reason,omitempty"`
	ConfigHash string `json:"configHash,omitempty"`
}

// Reconcile pushes the desired Xray JSON to every eligible node.
// Per-node failure is contained: one node failing does not abort
// the fleet; the plan reports it and the node's LastError carries
// the detail. Local nodes are reached through the same Adapter
// (production wiring: the in-process runtime).
func (m *Manager) Reconcile(ctx Ctx, config []byte) []Plan {
	var plans []Plan
	for _, node := range m.Eligible() {
		plan := Plan{NodeID: node.ID}
		if m.adapter == nil {
			plan.Action, plan.Reason = "skip", "adapter is not wired"
			plans = append(plans, plan)
			continue
		}
		if err := m.adapter.PushConfig(ctx, node, config); err != nil {
			plan.Action, plan.Reason = "skip", "push failed"
			m.recordError(node.ID, err.Error())
		} else {
			plan.Action = "push"
			plan.ConfigHash = hashToken("xnode-config", string(config))[:16]
			m.markSeen(node.ID, node.Version, node.Capabilities)
		}
		plans = append(plans, plan)
	}
	return plans
}

func (m *Manager) persistNodes() {
	if m.persist == nil {
		return
	}
	m.mu.RLock()
	out := make([]*Node, 0, len(m.nodes))
	for _, n := range m.nodes {
		c := *n
		out = append(out, &c)
	}
	m.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	_ = m.persist(out)
}
