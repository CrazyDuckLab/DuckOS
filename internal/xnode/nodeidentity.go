// Package xnode / nodeidentity.go: the persistent identity of a
// NODE-ROLE DuckOS instance (the machine that RUNS Xray and accepts
// coordinator control traffic).
//
// Transition protocol (rotation): the coordinator delivers the new
// credential over the channel authenticated with the CURRENT
// credential; the node persists it as PENDING (both credentials
// authenticate during the transition), acknowledges, the
// coordinator commits its side, then sends commit-credential which
// promotes pending→current atomically. An interrupted transition
// (lost ack, lost commit message, restart) leaves both credentials
// working, so the node is never orphaned; the coordinator retries
// the same delivery idempotently. Pending credentials are cleared
// on commit — old credentials are not retained indefinitely (a
// commit that never arrives leaves the pair until the next
// successful rotation, which replaces the pending slot).
package xnode

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/persist"
)

// NodeIdentityDocument is the durable node-role identity.
type NodeIdentityDocument struct {
	// CurrentHash authenticates the coordinator today.
	CurrentHash string `json:"currentHash"`
	// PendingHash, when set, authenticates during an interrupted or
	// in-flight rotation transition.
	PendingHash string `json:"pendingHash,omitempty"`
	// PendingCredential is the plaintext of the pending (transition)
	// credential, kept ONLY in this 0600 store so an interrupted
	// rotation stays recoverable without ever exposing the secret
	// through argv, stdout or shell history. It is cleared when the
	// transition commits. The plaintext never leaves the node's own
	// durable secret domain (the same trust boundary that guards the
	// Xray vault references).
	PendingCredential string    `json:"pendingCredential,omitempty"`
	AdoptedAt         time.Time `json:"adoptedAt"`
}

func nodeCredentialHash(credential string) string {
	sum := sha256.Sum256([]byte("xnode-cred\x00" + credential))
	return hex.EncodeToString(sum[:])
}

// NodeIdentity is the persistent credential holder of the node role.
type NodeIdentity struct {
	mu   sync.RWMutex
	file *persist.File[NodeIdentityDocument]
}

// OpenNodeIdentity loads (or creates) the node-role identity store.
func OpenNodeIdentity(dir string) (*NodeIdentity, error) {
	f, err := persist.Open[NodeIdentityDocument](dir, "node-identity.json")
	if err != nil {
		return nil, err
	}
	return &NodeIdentity{file: f}, nil
}

// Adopted reports whether an identity has been established.
func (n *NodeIdentity) Adopted() bool {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.file.Get().CurrentHash != ""
}

// Adopt records the FIRST credential (enrollment). Re-adoption over
// a live identity is refused — rotation is the re-binding path.
func (n *NodeIdentity) Adopt(credential string) error {
	if credential == "" {
		return errors.New("xnode: credential is required")
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	doc := n.file.Get()
	if doc.CurrentHash != "" {
		return errors.New("xnode: identity already adopted; use rotation")
	}
	doc.CurrentHash = nodeCredentialHash(credential)
	doc.AdoptedAt = time.Now().UTC()
	out, err := n.file.Update(func(cur *NodeIdentityDocument) { *cur = doc })
	if err != nil || out == persist.CommitUncertain {
		return fmt.Errorf("xnode: identity adoption not durably committed (outcome=%s): %w", out, err)
	}
	return nil
}

// Accepts reports whether the credential authenticates the
// coordinator right now (current or pending slot), constant-time.
func (n *NodeIdentity) Accepts(credential string) bool {
	if credential == "" {
		return false
	}
	n.mu.RLock()
	defer n.mu.RUnlock()
	doc := n.file.Get()
	digest := nodeCredentialHash(credential)
	if doc.CurrentHash != "" && hmac.Equal([]byte(doc.CurrentHash), []byte(digest)) {
		return true
	}
	return doc.PendingHash != "" && hmac.Equal([]byte(doc.PendingHash), []byte(digest))
}

// BeginTransition persists the NEW credential as pending. The
// current credential keeps working until commit — a lost delivery
// never orphans the node.
func (n *NodeIdentity) BeginTransition(newCredential string) error {
	if newCredential == "" {
		return errors.New("xnode: new credential is required")
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	doc := n.file.Get()
	if doc.CurrentHash == "" {
		return errors.New("xnode: identity not adopted")
	}
	doc.PendingHash = nodeCredentialHash(newCredential)
	doc.PendingCredential = newCredential
	out, err := n.file.Update(func(cur *NodeIdentityDocument) { *cur = doc })
	if err != nil || out == persist.CommitUncertain {
		return fmt.Errorf("xnode: rotation transition not durably committed (outcome=%s): %w", out, err)
	}
	return nil
}

// Commit promotes the pending credential to current. Committing a
// credential that was never delivered is refused (desync guard).
func (n *NodeIdentity) Commit(newCredential string) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	doc := n.file.Get()
	pending := nodeCredentialHash(newCredential)
	// Idempotent: a confirm whose ack was lost already committed —
	// re-committing the CURRENT credential is a no-op success.
	if doc.PendingHash == "" && doc.CurrentHash == pending {
		return nil
	}
	if doc.PendingHash == "" || doc.PendingHash != pending {
		return errors.New("xnode: commit-credential does not match the pending transition")
	}
	doc.CurrentHash = pending
	doc.PendingHash = ""
	doc.PendingCredential = ""
	out, err := n.file.Update(func(cur *NodeIdentityDocument) { *cur = doc })
	if err != nil || out == persist.CommitUncertain {
		return fmt.Errorf("xnode: rotation commit not durably committed (outcome=%s): %w", out, err)
	}
	return nil
}

// PendingCredential reports a durably stored pending (transition)
// credential, if any. Used by the CLI recovery path after an
// interrupted rotation: the plaintext lives only in the node's own
// 0600 identity store, so recovery never needs the secret in argv,
// on stdout or in shell history.
func (n *NodeIdentity) PendingCredential() (string, bool) {
	n.mu.RLock()
	defer n.mu.RUnlock()
	doc := n.file.Get()
	if doc.PendingHash == "" || doc.PendingCredential == "" {
		return "", false
	}
	// Integrity guard: only return the plaintext when it matches the
	// durable pending hash (a partially written or tampered document
	// must not surface a credential).
	if !hmac.Equal([]byte(doc.PendingHash), []byte(nodeCredentialHash(doc.PendingCredential))) {
		return "", false
	}
	return doc.PendingCredential, true
}

// PendingHashMatches reports whether the presented credential equals
// the durably pending one, without promoting anything.
func (n *NodeIdentity) PendingHashMatches(credential string) bool {
	if credential == "" {
		return false
	}
	n.mu.RLock()
	defer n.mu.RUnlock()
	doc := n.file.Get()
	return doc.PendingHash != "" &&
		hmac.Equal([]byte(doc.PendingHash), []byte(nodeCredentialHash(credential)))
}

// Reload re-reads the durable identity document from disk into the
// in-memory mirror. The node control-plane calls this before every
// authentication decision so a rotation completed by ANOTHER process
// (the duck CLI) is adopted by the RUNNING NodeServer immediately:
// updating the file alone must not leave the live runtime on a stale
// in-memory credential. An unreadable document keeps the in-memory
// state (fail-closed: the server keeps authenticating with what it
// last knew rather than locking itself out on a transient I/O error).
func (n *NodeIdentity) Reload() error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if err := n.file.Reload(); err != nil {
		// A missing document means the identity was never written
		// yet; the in-memory (zero or adopted) state stays
		// authoritative in that case.
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	return nil
}

// ConfirmPending verifies the presented new credential against the
// durable pending hash and finalizes the transition.
func (n *NodeIdentity) ConfirmPending(newCredential string) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	doc := n.file.Get()
	pending := nodeCredentialHash(newCredential)
	if doc.PendingHash == "" || doc.PendingHash != pending {
		return errors.New("xnode: no matching pending credential in the local identity store")
	}
	doc.CurrentHash = pending
	doc.PendingHash = ""
	doc.PendingCredential = ""
	out, err := n.file.Update(func(cur *NodeIdentityDocument) { *cur = doc })
	if err != nil || out == persist.CommitUncertain {
		return fmt.Errorf("xnode: local finalization not durably committed (outcome=%s): %w", out, err)
	}
	return nil
}

// HasPending reports whether a pending transition exists.
func (n *NodeIdentity) HasPending() bool {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.file.Get().PendingHash != ""
}
