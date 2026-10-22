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
	persist  func(DurableState) (CommitResult, error)
	// revision is the monotonic generation of the committed state;
	// it survives restart through DurableState.Revision.
	revision uint64
	// degraded carries the active COMMIT_UNCERTAIN diagnosis, or ""
	// when storage is healthy. Degraded storage BLOCKS mutations.
	degraded string
	// staged holds in-memory staged rotation credentials (Milestone
	// 1): plaintext is never persisted; a controller restart drops
	// the staging and the operator re-stages (old stays valid).
	staged map[string]*stagedRotation
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

// DurableState is the complete persisted registry state: nodes and
// enrollment tokens. After a restart the manager is reconstructed
// exactly from this snapshot, so every security-relevant transition
// (revocation, rotation, suspension, token consumption) is durable
// only when the snapshot carrying it has been committed.
type DurableState struct {
	// Revision is the monotonic generation of this snapshot; it is
	// the reconciliation anchor between memory and disk.
	Revision    uint64            `json:"revision"`
	Nodes       []Node            `json:"nodes"`
	Enrollments []EnrollmentToken `json:"enrollments"`
}

// CommitResult carries the commit-phase information the manager
// needs to distinguish pre-rename failures (NOT_COMMITTED: the
// previous durable state remains authoritative → roll back) from
// post-rename uncertainty (COMMIT_UNCERTAIN: the new — restrictive
// — state is what the directory shows; NEVER roll back, enter the
// degraded fail-closed mode until reconciliation).
type CommitResult struct {
	Uncertain bool
}

// SetPersist installs the durability hook. The hook MUST commit the
// whole DurableState atomically and MUST report the commit phase;
// mutation callers act on the phase, never on the error alone.
func (m *Manager) SetPersist(fn func(DurableState) (CommitResult, error)) { m.persist = fn }

// IssueEnrollmentToken mints a single-use enrollment token. The
// plaintext token is returned exactly once.
func (m *Manager) IssueEnrollmentToken(ttl time.Duration) (string, EnrollmentToken, error) {
	// R-05: the degraded gate and the mutation share ONE critical
	// section — a concurrent persistLocked cannot transition the
	// domain to degraded between the check and the write.
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.requireHealthyStorage(); err != nil {
		return "", EnrollmentToken{}, err
	}
	token, err := newSecret()
	if err != nil {
		return "", EnrollmentToken{}, err
	}
	now := m.clock().UTC()
	rec := EnrollmentToken{TokenHash: hashToken("xnode-enroll", token), CreatedAt: now}
	if ttl != 0 {
		rec.ExpiresAt = now.Add(ttl)
	}
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
	res, perr := m.persistLocked()
	if perr != nil {
		if !res.Uncertain {
			delete(m.enroll, rec.TokenHash)
		}
		return "", EnrollmentToken{}, m.commitOutcomeLocked(res, perr, "enrollment token issuance")
	}
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
	m.mu.Lock()
	if err := m.requireHealthyStorage(); err != nil {
		m.mu.Unlock()
		return Node{}, "", err
	}
	digest := hashToken("xnode-enroll", token)
	rec, ok := m.enroll[digest]
	if ok {
		now := m.clock().UTC()
		ok = !rec.Used && (rec.ExpiresAt.IsZero() || now.Before(rec.ExpiresAt))
	}
	if !ok {
		m.mu.Unlock()
		return Node{}, "", errors.New("xnode: enrollment token is unknown, used or expired")
	}
	// Transactional enrollment: burn the token, create the node,
	// commit — any failure rolls BOTH back, so a consumed token
	// never reappears usable after a restart and a failed durable
	// commit is never acknowledged as a successful enrollment.
	rec.Used = true
	rec.UsedBy = "pending"
	credential, err := newSecret()
	if err != nil {
		rec.Used, rec.UsedBy = false, ""
		m.mu.Unlock()
		return Node{}, "", err
	}
	id, err := newID()
	if err != nil {
		rec.Used, rec.UsedBy = false, ""
		m.mu.Unlock()
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
	if len(m.nodes) >= MaxNodes {
		rec.Used, rec.UsedBy = false, ""
		m.mu.Unlock()
		return Node{}, "", errors.New("xnode: node capacity exceeded")
	}
	m.nodes[id] = &node
	rec.UsedBy = id
	res, perr := m.persistLocked()
	if perr != nil {
		if !res.Uncertain {
			delete(m.nodes, id)
			rec.Used, rec.UsedBy = false, ""
		}
		m.mu.Unlock()
		return Node{}, "", m.commitOutcomeLocked(res, perr, "enrollment")
	}
	m.mu.Unlock()
	return node, credential, nil
}

// EnrollLocal registers the coordinator's own node. Local nodes do
// not carry credentials: trust is the in-process agent boundary.
func (m *Manager) EnrollLocal(name string) (Node, error) {
	if !namePattern.MatchString(name) {
		return Node{}, errors.New("xnode: node name is not canonical")
	}
	m.mu.Lock()
	if err := m.requireHealthyStorage(); err != nil {
		m.mu.Unlock()
		return Node{}, err
	}
	m.mu.Unlock()
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
	res, perr := m.persistLocked()
	if perr != nil {
		if !res.Uncertain {
			delete(m.nodes, id)
		}
		return Node{}, m.commitOutcomeLocked(res, perr, "local enrollment")
	}
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
	// R-05: gate and rotation share one critical section.
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.requireHealthyStorage(); err != nil {
		return "", err
	}
	credential, err := newSecret()
	if err != nil {
		return "", err
	}
	node, ok := m.nodes[nodeID]
	if !ok || node.Status == StatusRevoked {
		return "", errors.New("xnode: node is unknown or revoked")
	}
	if node.Kind == KindLocal {
		return "", errors.New("xnode: local nodes carry no credential")
	}
	prevHash := node.CredentialHash
	node.CredentialHash = hashToken("xnode-cred", credential)
	res, perr := m.persistLocked()
	if perr != nil {
		if !res.Uncertain {
			node.CredentialHash = prevHash
		}
		return "", m.commitOutcomeLocked(res, perr, "credential rotation")
	}
	return credential, nil
}

// Revoke revokes a node permanently. Revoked nodes keep their
// record for forensics but fail every authentication and
// reconciliation.
func (m *Manager) Revoke(nodeID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.requireHealthyStorage(); err != nil {
		return err
	}
	node, ok := m.nodes[nodeID]
	if !ok {
		return errors.New("xnode: node is unknown")
	}
	prevStatus, prevErr := node.Status, node.LastError
	node.Status = StatusRevoked
	node.LastError = "revoked by operator"
	// SECURITY (UQ-01): the operation succeeds ONLY when the
	// durable snapshot carrying the revocation is committed. On a
	// failed commit the in-memory revocation is rolled back, the
	// error propagates, and no success is acknowledged — the old
	// credential must not silently resurrect after a restart.
	res, perr := m.persistLocked()
	if perr != nil {
		if !res.Uncertain {
			node.Status, node.LastError = prevStatus, prevErr
		}
		return m.commitOutcomeLocked(res, perr, "revocation")
	}
	return nil
}

// Suspend / Resume implement manual maintenance mode.
func (m *Manager) Suspend(nodeID string) error { return m.setStatus(nodeID, StatusSuspended) }
func (m *Manager) Resume(nodeID string) error  { return m.setStatus(nodeID, StatusActive) }

func (m *Manager) setStatus(nodeID string, status Status) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.requireHealthyStorage(); err != nil {
		return err
	}
	node, ok := m.nodes[nodeID]
	if !ok || node.Status == StatusRevoked {
		return errors.New("xnode: node is unknown or revoked")
	}
	prev := node.Status
	node.Status = status
	res, perr := m.persistLocked()
	if perr != nil {
		if !res.Uncertain {
			node.Status = prev
		}
		return m.commitOutcomeLocked(res, perr, "status change")
	}
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

// persistLocked commits the complete registry state under an
// already-held lock and returns the durability error verbatim. A
// nil hook (memory-only mode, tests) reports success; production
// wiring MUST install a hook. SECURITY: a failed commit means the
// transition is NOT acknowledged — callers roll back and fail.
func (m *Manager) persistLocked() (CommitResult, error) {
	if m.persist == nil {
		return CommitResult{}, nil
	}
	m.revision++
	st := DurableState{Revision: m.revision}
	for _, n := range m.nodes {
		c := *n
		st.Nodes = append(st.Nodes, c)
	}
	sort.Slice(st.Nodes, func(i, j int) bool { return st.Nodes[i].ID < st.Nodes[j].ID })
	for _, e := range m.enroll {
		c := *e
		st.Enrollments = append(st.Enrollments, c)
	}
	sort.Slice(st.Enrollments, func(i, j int) bool { return st.Enrollments[i].TokenHash < st.Enrollments[j].TokenHash })
	res, err := m.persist(st)
	if err != nil && res.Uncertain {
		// COMMIT_UNCERTAIN: the restrictive NEW state stays in
		// memory (rolling it back would let the next mutation
		// overwrite the revoked snapshot — the exact UQ-02
		// resurrection). Storage is degraded; mutations block.
		m.degraded = fmt.Sprintf("commit_uncertain at revision %d: %v", st.Revision, err)
	}
	return res, err
}

// Degraded reports the active storage-uncertainty diagnosis.
func (m *Manager) Degraded() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.degraded
}

// ReconcileStorage resolves a COMMIT_UNCERTAIN state against the
// authoritative disk content:
//
//   - disk revision == in-memory revision: the uncertain commit
//     landed; storage recovers, state unchanged.
//   - disk revision differs: the disk is authoritative for
//     restarts; adopt it wholesale and tell the operator to verify
//     recent revocations — never a silent resolution.
func (m *Manager) ReconcileStorage(st DurableState) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.degraded == "" {
		return "healthy", nil
	}
	switch {
	case st.Revision == m.revision:
		m.degraded = ""
		return "recovered: uncertain commit is on disk", nil
	case st.Revision > m.revision:
		return "", errors.New("xnode: disk revision ahead of memory; refusing to reconcile")
	default:
		// AUD-03: an older snapshot is adopted ONLY through a
		// restrictive merge. Security state is monotonic across
		// reconciliation: a node revoked in EITHER view stays
		// revoked, a consumed enrollment token stays consumed, and
		// the most recently acknowledged credential hash wins (an
		// older snapshot can never silently reactivate a rotated-out
		// credential). Every protective decision is reported —
		// nothing is silently activated, and the operator directive
		// to re-verify remains.
		adopted := 0
		protectedRevocations := 0
		protectedSuspensions := 0
		protectedConsumptions := 0
		protectedRotations := 0
		for i := range st.Nodes {
			n := st.Nodes[i]
			if prev, ok := m.nodes[n.ID]; ok {
				// R-01: node status is monotonic across
				// reconciliation under the precedence
				// REVOKED > SUSPENDED > everything else. A node
				// suspended or revoked in EITHER view keeps the
				// restrictive status; a legitimate resume is an
				// ordinary durably-committed mutation and is not
				// affected (by the time an older snapshot is
				// reconciled, a committed resume is already IN that
				// newer memory state).
				if statusRank(prev.Status) > statusRank(n.Status) {
					if prev.Status == StatusRevoked {
						protectedRevocations++
					} else {
						protectedSuspensions++
					}
					n.Status = prev.Status
					n.LastError = prev.LastError
				}
				if prev.CredentialHash != n.CredentialHash && prev.CredentialHash != "" {
					// Memory holds the newest acknowledged rotation.
					n.CredentialHash = prev.CredentialHash
					protectedRotations++
				}
			}
			m.nodes[n.ID] = &n
			adopted++
		}
		for i := range st.Enrollments {
			e := st.Enrollments[i]
			if prev, ok := m.enroll[e.TokenHash]; ok {
				if prev.Used && !e.Used {
					e.Used = true
					e.UsedBy = prev.UsedBy
					protectedConsumptions++
				}
			}
			m.enroll[e.TokenHash] = &e
		}
		m.revision = st.Revision
		m.degraded = ""
		return fmt.Sprintf("recovered: disk state adopted with restrictive merge (%d nodes, %d revocation(s) protected, %d suspension(s) protected, %d rotation(s) protected, %d consumption(s) protected; verify recent operations)", adopted, protectedRevocations, protectedSuspensions, protectedRotations, protectedConsumptions), nil
	}
}

// statusRank is the security precedence of a node status:
// REVOKED (irreversible operator decision) > SUSPENDED (maintenance
// hold) > everything else. Reconciliation never adopts a snapshot
// with a lower-rank status than the in-memory one.
func statusRank(s Status) int {
	switch s {
	case StatusRevoked:
		return 3
	case StatusSuspended:
		return 2
	default:
		return 1
	}
}

// commitOutcomeLocked resolves the persist result for a mutation.
// UNCERTAIN keeps the new (restrictive) state and reports a
// non-success with the degraded-storage diagnosis; NOT_COMMITTED
// expects the caller to have rolled back and reports the plain
// storage failure.
func (m *Manager) commitOutcomeLocked(res CommitResult, err error, what string) error {
	if err == nil {
		return nil
	}
	if res.Uncertain {
		return fmt.Errorf("xnode: %s commit outcome UNCERTAIN; restrictive state retained, storage degraded: %w", what, err)
	}
	return fmt.Errorf("xnode: %s not committed (durable state unchanged): %w", what, err)
}

// requireHealthyStorageLocked is the mutation gate for callers that
// do NOT yet hold the lock: it acquires the registry lock for the
// check, closing the TOCTOU window against a concurrent
// persistLocked degraded transition (AUD-05).
func (m *Manager) requireHealthyStorageLocked() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.requireHealthyStorage()
}

// requireHealthyStorage is the gate for callers that ALREADY hold
// m.mu — it must never lock itself (deadlock).
func (m *Manager) requireHealthyStorage() error {
	if m.degraded != "" {
		return fmt.Errorf("xnode: storage degraded (%s); reconciliation required before mutations", m.degraded)
	}
	return nil
}

// LoadState seeds the registry from persisted state (startup):
// nodes INCLUDING revoked records (revocation survives restart) and
// enrollment tokens with their consumption marks.
func (m *Manager) LoadState(st DurableState) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nodes = map[string]*Node{}
	m.enroll = map[string]*EnrollmentToken{}
	for i := range st.Nodes {
		n := st.Nodes[i]
		m.nodes[n.ID] = &n
	}
	for i := range st.Enrollments {
		e := st.Enrollments[i]
		m.enroll[e.TokenHash] = &e
	}
	m.revision = st.Revision
}
