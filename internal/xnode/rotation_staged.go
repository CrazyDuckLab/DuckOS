// Package xnode / rotation_staged.go: the operator/UI-driven staged
// rotation flow that never strands a node.
//
// Problem fixed (Milestone 1): the center UI's rotate endpoint used
// RotateCredential, which commits a fresh random credential on the
// controller IMMEDIATELY — the node keeps only the old credential
// and loses all controller access (stranded).
//
// Staged protocol (controller never stores node credential
// plaintext at rest; the staged plaintext lives only in memory
// until pulled):
//
//  1. StageRotation(nodeID): generate + remember in memory, mark
//     the node PENDING_ROTATION. The OLD credential stays fully
//     valid. State: PENDING.
//  2. The node PULLS the staged credential by authenticating with
//     its CURRENT credential (PullStagedRotation). State: DELIVERED
//     (the node has proven possession of the new material is still
//     pending — delivery is proven at confirm time).
//  3. The node adopts the new credential locally (BeginTransition:
//     old+new both work) and CONFIRMS by authenticating with the
//     NEW credential (ConfirmStagedRotation). Possession proven →
//     the controller commits: old credential invalid. State:
//     CONFIRMED.
//
// Failure behavior: controller restart before step 3 loses the
// in-memory staged credential → the flow returns to empty (old still
// valid everywhere) → the operator re-stages. A node that pulled but
// never confirmed keeps old+new locally; the controller-side staged
// entry is dropped on restart and re-staging re-issues a fresh
// credential. A staged rotation can be CANCELLED explicitly. At no
// point is the old credential invalidated before the node has
// proven possession of the new one.
package xnode

import (
	"crypto/hmac"
	"errors"
)

// RotationState is the operator-visible staged rotation state.
type RotationState string

const (
	RotationNone      RotationState = ""
	RotationPending   RotationState = "PENDING"
	RotationDelivered RotationState = "DELIVERED"
	RotationConfirmed RotationState = "CONFIRMED"
)

type stagedRotation struct {
	newCredential string // plaintext, memory only
	newHash       string
	delivered     bool
}

// StageRotation generates a new credential and stages it for node
// pull. The old credential remains valid; nothing is committed.
func (m *Manager) StageRotation(nodeID string) (string, RotationState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.requireHealthyStorage(); err != nil {
		return "", RotationNone, err
	}
	node, ok := m.nodes[nodeID]
	if !ok || node.Status == StatusRevoked {
		return "", RotationNone, errors.New("xnode: node is unknown or revoked")
	}
	if node.Kind == KindLocal {
		return "", RotationNone, errors.New("xnode: local nodes carry no credential")
	}
	if m.staged == nil {
		m.staged = map[string]*stagedRotation{}
	}
	if _, inflight := m.staged[nodeID]; inflight {
		return "", RotationNone, errors.New("xnode: a staged rotation is already in flight; cancel or complete it first")
	}
	credential, err := newSecret()
	if err != nil {
		return "", RotationNone, err
	}
	m.staged[nodeID] = &stagedRotation{newCredential: credential, newHash: hashToken("xnode-cred", credential)}
	return credential, RotationPending, nil
}

// PullStagedRotation is called by the NODE (authenticated with its
// current credential) to retrieve the staged credential. Delivery
// alone does not invalidate anything.
func (m *Manager) PullStagedRotation(nodeID, currentCredential string) (string, RotationState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	// authenticateLocked: the registry lock is already held —
	// calling Authenticate here would self-deadlock on the
	// non-reentrant RWMutex (found by the lifecycle test).
	if !m.authenticateLocked(nodeID, currentCredential) {
		return "", RotationNone, errors.New("xnode: pull rejected: current credential does not authenticate")
	}
	staged, ok := m.staged[nodeID]
	if !ok {
		return "", RotationNone, errors.New("xnode: no staged rotation for this node")
	}
	staged.delivered = true
	return staged.newCredential, RotationDelivered, nil
}

// ConfirmStagedRotation is called by the NODE authenticating with
// the NEW credential. Possession proven → the controller commits:
// the staged credential becomes the node credential and the old one
// is invalidated (durably, through the commit-protocol hook).
func (m *Manager) ConfirmStagedRotation(nodeID, newCredential string) (RotationState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.requireHealthyStorage(); err != nil {
		return RotationNone, err
	}
	newHash := hashToken("xnode-cred", newCredential)
	// B4-02 idempotent replay: this exact credential was already
	// durably committed for this node.
	if m.completed[nodeID] == newHash {
		return RotationConfirmed, nil
	}
	staged, ok := m.staged[nodeID]
	if !ok {
		return RotationNone, errors.New("xnode: no staged rotation for this node (re-stage after a controller restart)")
	}
	if !hmac.Equal([]byte(staged.newHash), []byte(newHash)) {
		return RotationNone, errors.New("xnode: confirmation credential does not match the staged rotation")
	}
	node, ok := m.nodes[nodeID]
	if !ok || node.Status == StatusRevoked {
		return RotationNone, errors.New("xnode: node is unknown or revoked")
	}
	prevHash := node.CredentialHash
	node.CredentialHash = staged.newHash
	res, perr := m.persistLocked()
	if perr != nil {
		if !res.Uncertain {
			node.CredentialHash = prevHash
		}
		return RotationNone, m.commitOutcomeLocked(res, perr, "staged rotation commit")
	}
	// Durable tombstone (bounded): survives controller restarts via
	// the registry persistence hook.
	if len(m.completed) >= 256 {
		m.completed = map[string]string{} // bounded reset; history is advisory metadata
	}
	m.completed[nodeID] = staged.newHash
	delete(m.staged, nodeID)
	return RotationConfirmed, nil
}

// CancelStagedRotation aborts an in-flight staged rotation. Nothing
// was committed, so both sides keep working on the old credential.
func (m *Manager) CancelStagedRotation(nodeID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.staged[nodeID]; !ok {
		return errors.New("xnode: no staged rotation for this node")
	}
	delete(m.staged, nodeID)
	return nil
}

// RotationStatus reports the operator-visible staged rotation state.
func (m *Manager) RotationStatus(nodeID string) RotationState {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if _, done := m.completed[nodeID]; done {
		return RotationConfirmed
	}
	staged, ok := m.staged[nodeID]
	if !ok {
		return RotationNone
	}
	if staged.delivered {
		return RotationDelivered
	}
	return RotationPending
}

// authenticateLocked is the lock-free core of Authenticate for
// callers that already hold m.mu: constant-time credential match
// against the node's current hash, restricted to active/stale
// nodes. It deliberately skips markSeen (advisory metadata).
func (m *Manager) authenticateLocked(nodeID, credential string) bool {
	if nodeID == "" || credential == "" {
		return false
	}
	node, ok := m.nodes[nodeID]
	if !ok {
		return false
	}
	return node.CredentialHash != "" &&
		hmac.Equal([]byte(node.CredentialHash), []byte(hashToken("xnode-cred", credential))) &&
		(node.Status == StatusActive || node.Status == StatusStale)
}
