package vault

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/state"
)

const MaxRecords = 128

type storedRecord struct {
	Reference Reference `json:"reference"`
	Envelope  []byte    `json:"envelope"`
}
type database struct {
	Version    int            `json:"version"`
	KeyID      string         `json:"keyId"`
	Records    []storedRecord `json:"records"`
	Tombstones []tombstone    `json:"tombstones,omitempty"`
}

type tombstone struct {
	Reference Reference `json:"reference"`
	Actor     string    `json:"actor"`
	Reason    string    `json:"reason"`
	At        time.Time `json:"at"`
}

// Store retains immutable secret revisions. Only references may be copied into
// ordinary desired state. Its master key must be supplied by the key custodian.
type Store struct {
	mu     sync.Mutex
	state  *state.Store
	key    []byte
	db     database
	closed bool
}

func (s *Store) String() string               { return "[private vault store]" }
func (s *Store) GoString() string             { return s.String() }
func (s *Store) LogValue() slog.Value         { return slog.StringValue(s.String()) }
func (s *Store) MarshalJSON() ([]byte, error) { return nil, ErrRecord }

// InitializeStore is an explicit provisioning operation on an empty directory.
// A missing or damaged existing database is never replaced during normal Open.
func InitializeStore(path string, master []byte) (*Store, error) {
	entries, err := os.ReadDir(path)
	if err != nil || len(entries) != 0 {
		return nil, ErrRecord
	}
	return openStore(path, master)
}

func OpenStore(path string, master []byte) (*Store, error) {
	info, err := os.Lstat(filepath.Join(path, "state.json"))
	if err != nil || !info.Mode().IsRegular() {
		return nil, ErrRecord
	}
	return openStore(path, master)
}

func openStore(path string, master []byte) (*Store, error) {
	if len(master) != 32 || bytes.Equal(master, make([]byte, 32)) {
		return nil, ErrRecord
	}
	key := bytes.Clone(master)
	fingerprint := sha256.Sum256(append([]byte("DuckOS/vault/key-id/v1\x00"), key...))
	keyID := hex.EncodeToString(fingerprint[:16])
	initial, _ := json.Marshal(database{Version: 1, KeyID: keyID, Records: []storedRecord{}})
	validate := func(raw json.RawMessage) error { _, err := decodeDatabase(key, keyID, raw); return err }
	persistent, err := state.Open(path, initial, validate)
	if err != nil {
		clear(key)
		return nil, ErrRecord
	}
	ok := false
	defer func() {
		if !ok {
			persistent.Close()
			clear(key)
		}
	}()
	snapshot, err := persistent.Snapshot()
	if err != nil {
		return nil, ErrRecord
	}
	// Secret creation has no external side effect before the canonical commit.
	// An interrupted creation is never implicitly promoted on restart.
	if snapshot.Pending != nil {
		if snapshot.Pending.Phase != state.RollingBack {
			if err := persistent.Transition(snapshot.Pending.ID, state.RollingBack, time.Time{}); err != nil {
				return nil, ErrRecord
			}
		}
		if err := persistent.RolledBack(snapshot.Pending.ID); err != nil {
			return nil, ErrRecord
		}
	}
	db, err := decodeDatabase(key, keyID, snapshot.Current.Config)
	if err != nil {
		return nil, ErrRecord
	}
	ok = true
	return &Store{state: persistent, key: key, db: db}, nil
}

func decodeDatabase(key []byte, keyID string, raw json.RawMessage) (database, error) {
	var db database
	if len(raw) > state.MaxConfigBytes {
		return db, ErrRecord
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&db) != nil || d.Decode(new(any)) != io.EOF || db.Version != 1 || db.KeyID != keyID || db.Records == nil || len(db.Records) > MaxRecords {
		return database{}, ErrRecord
	}
	seen := map[string]bool{}
	for _, r := range db.Records {
		if r.Reference.KeyID != keyID || seen[r.Reference.Revision] {
			return database{}, ErrRecord
		}
		seen[r.Reference.Revision] = true
		secret, err := Open(key, r.Reference, r.Envelope)
		if err != nil {
			return database{}, ErrRecord
		}
		secret.Clear()
	}
	return db, nil
}

func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	clear(s.key)
	return s.state.Close()
}

func (s *Store) Get(ref Reference) (*Secret, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, ErrRecord
	}
	for _, r := range s.db.Records {
		if r.Reference == ref {
			return Open(s.key, ref, r.Envelope)
		}
	}
	return nil, ErrRecord
}

// Put durably creates a new immutable revision before a network transaction can
// reference it. It does not rotate a running service or authorize an export.
func (s *Store) Put(id, purpose string, plaintext []byte) (Reference, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || len(s.db.Records) >= MaxRecords {
		return Reference{}, ErrRecord
	}
	random := make([]byte, 16)
	if _, err := rand.Read(random); err != nil {
		return Reference{}, ErrRecord
	}
	ref := Reference{ID: id, Revision: hex.EncodeToString(random), Purpose: purpose, KeyID: s.db.KeyID}
	envelope, err := Seal(s.key, ref, plaintext)
	if err != nil {
		return Reference{}, ErrRecord
	}
	next := database{Version: 1, KeyID: s.db.KeyID, Records: append(append([]storedRecord{}, s.db.Records...), storedRecord{Reference: ref, Envelope: envelope})}
	raw, err := json.Marshal(next)
	if err != nil || len(raw) > state.MaxConfigBytes {
		return Reference{}, ErrRecord
	}
	if _, err := decodeDatabase(s.key, s.db.KeyID, raw); err != nil {
		return Reference{}, ErrRecord
	}
	if err := s.persist(raw); err != nil {
		// A failed durability acknowledgement may conceal a committed write.
		// Deny subsequent reads until the durable state has been reopened.
		s.closed = true
		clear(s.key)
		return Reference{}, ErrRecord
	}
	s.db = next
	return ref, nil
}

func (s *Store) persist(raw json.RawMessage) error {
	snapshot, err := s.state.Snapshot()
	if err != nil {
		return err
	}
	tx, err := s.state.Prepare(snapshot.Current.ID, raw, "vault", "secret.create")
	if err != nil {
		return err
	}
	if err := s.state.Transition(tx.ID, state.Applying, time.Time{}); err != nil {
		return err
	}
	if err := s.state.Transition(tx.ID, state.AwaitingConfirmation, time.Now().Add(2*time.Minute)); err != nil {
		return err
	}
	return s.state.Commit(tx.ID)
}

// ListReferences returns a metadata-only view of every stored reference.
// The function never returns secret bytes; callers use the references
// to resolve secrets through Get. The output is sorted by (id, revision)
// for stable diffs.
func (s *Store) ListReferences() ([]Reference, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, ErrRecord
	}
	out := make([]Reference, 0, len(s.db.Records))
	for _, r := range s.db.Records {
		out = append(out, r.Reference)
	}
	return out, nil
}

// Revoke marks a reference as unusable by appending a tombstone. The
// underlying envelope is still retained on disk for forensics; the
// Resolver must consult Tombstones to refuse the reference. A revoked
// reference cannot be used to resolve a secret; an attempt returns
// ErrRevoked so the caller can surface a bounded error to the user.
func (s *Store) Revoke(id, revision, actor, reason string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ErrRecord
	}
	now := time.Now().UTC()
	record, ok := s.findLocked(id, revision)
	if !ok {
		return ErrRecord
	}
	tombstones := append([]tombstone(nil), s.db.Tombstones...)
	tombstones = append(tombstones, tombstone{Reference: record.Reference, Actor: actor, Reason: reason, At: now})
	next := database{Version: 1, KeyID: s.db.KeyID, Records: s.db.Records, Tombstones: tombstones}
	raw, err := json.Marshal(next)
	if err != nil || len(raw) > state.MaxConfigBytes {
		return ErrRecord
	}
	if _, err := decodeDatabase(s.key, s.db.KeyID, raw); err != nil {
		return ErrRecord
	}
	if err := s.persist(raw); err != nil {
		s.closed = true
		clear(s.key)
		return ErrRecord
	}
	s.db = next
	return nil
}

// IsRevoked reports whether the given reference has been tombstoned.
func (s *Store) IsRevoked(ref Reference) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, t := range s.db.Tombstones {
		if t.Reference == ref {
			return true
		}
	}
	return false
}

// KeyID returns the vault's master key fingerprint. Callers can use
// it to verify that a reference's KeyID still matches the live key
// after a rotation.
func (s *Store) KeyID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.db.KeyID
}

func (s *Store) findLocked(id, revision string) (storedRecord, bool) {
	for _, r := range s.db.Records {
		if r.Reference.ID == id && r.Reference.Revision == revision {
			return r, true
		}
	}
	return storedRecord{}, false
}
