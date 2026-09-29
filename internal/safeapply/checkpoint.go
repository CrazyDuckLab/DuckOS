// safeapply/checkpoint.go: V1 named
// checkpoints. A checkpoint is a labelled
// durable snapshot of the canonical
// configuration. The operator labels
// checkpoints ("before ISP migration") and
// the system retains a bounded number of
// them. Restore is a one-step operation that
// stages the checkpoint's candidate through
// the existing Safe Apply machinery.
package safeapply

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"sort"
	"sync"
	"time"
)

// MaxCheckpoints is the bounded number of
// checkpoints DuckOS retains per Agent.
const MaxCheckpoints = 64

// CheckpointID is the durable identifier.
type CheckpointID string

// Checkpoint is one named snapshot.
type Checkpoint struct {
	ID          CheckpointID    `json:"id"`
	Label       string          `json:"label"`
	CreatedAt   time.Time       `json:"createdAt"`
	CreatedBy   string          `json:"createdBy"`
	RevisionID  string          `json:"revisionId"`
	Config      json.RawMessage `json:"config"`
	Description string          `json:"description,omitempty"`
}

var labelPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 _.-]{0,63}$`)

// CheckpointStore is the in-memory +
// durable store. The Store is goroutine-safe.
type CheckpointStore struct {
	mu      sync.Mutex
	byID    map[CheckpointID]Checkpoint
	byLabel map[string]CheckpointID
	clock   func() time.Time
}

// NewCheckpointStore returns an empty store.
func NewCheckpointStore(clock func() time.Time) *CheckpointStore {
	if clock == nil {
		clock = time.Now
	}
	return &CheckpointStore{byID: map[CheckpointID]Checkpoint{}, byLabel: map[string]CheckpointID{}, clock: clock}
}

// Put stores a checkpoint under a label.
// The label must be canonical. The store
// refuses a label that already exists; the
// operator must explicitly remove the old
// checkpoint first.
func (s *CheckpointStore) Put(label, createdBy, revisionID string, config json.RawMessage, description string) (Checkpoint, error) {
	if !labelPattern.MatchString(label) {
		return Checkpoint{}, errors.New("checkpoint: label is not canonical")
	}
	if !json.Valid(config) {
		return Checkpoint{}, errors.New("checkpoint: config is not valid JSON")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.byLabel[label]; ok {
		return Checkpoint{}, errors.New("checkpoint: label already exists; remove it first")
	}
	if len(s.byID) >= MaxCheckpoints {
		return Checkpoint{}, errors.New("checkpoint: capacity exceeded")
	}
	sum := sha256.Sum256(append([]byte("DuckOS/checkpoint/v1\x00"+label+"\x00"), config...))
	id := CheckpointID(hex.EncodeToString(sum[:16]))
	cp := Checkpoint{ID: id, Label: label, CreatedAt: s.clock().UTC(), CreatedBy: createdBy, RevisionID: revisionID, Config: config, Description: description}
	s.byID[id] = cp
	s.byLabel[label] = id
	return cp, nil
}

// Get returns a checkpoint by ID.
func (s *CheckpointStore) Get(id CheckpointID) (Checkpoint, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp, ok := s.byID[id]
	return cp, ok
}

// GetByLabel returns a checkpoint by label.
func (s *CheckpointStore) GetByLabel(label string) (Checkpoint, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, ok := s.byLabel[label]
	if !ok {
		return Checkpoint{}, false
	}
	return s.byID[id], true
}

// List returns checkpoints sorted by creation
// time (newest first).
func (s *CheckpointStore) List() []Checkpoint {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Checkpoint, 0, len(s.byID))
	for _, cp := range s.byID {
		out = append(out, cp)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

// Remove deletes a checkpoint.
func (s *CheckpointStore) Remove(id CheckpointID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp, ok := s.byID[id]
	if !ok {
		return errors.New("checkpoint: not found")
	}
	delete(s.byID, id)
	delete(s.byLabel, cp.Label)
	return nil
}
