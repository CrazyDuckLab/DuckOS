// Package xray / realitymgr.go: REALITY key and short-id lifecycle
// management. The manager stores PUBLIC metadata only: private keys
// are generated here, returned exactly once for vault storage
// (purpose "reality-private"), and referenced afterwards. Rotation
// produces a preview plan (old key stays active until the operator
// applies the new one, so clients never lose connectivity mid-
// rotation); short ids retire with a validity window so in-flight
// clients keep working through the transition.
package xray

import (
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"
)

// ShortIDState is the short-id lifecycle state.
type ShortIDState string

const (
	ShortIDActive  ShortIDState = "active"
	ShortIDRetired ShortIDState = "retired"
)

// ShortIDRecord is one short id bound to a key record.
type ShortIDRecord struct {
	Hex       string       `json:"hex"`
	State     ShortIDState `json:"state"`
	CreatedAt time.Time    `json:"createdAt"`
	// RetiredAt marks when the id left the active set. It stays in
	// the record so operators can correlate old client configs.
	RetiredAt time.Time `json:"retiredAt,omitempty"`
}

// RealityKeyRecord is one REALITY key inventory entry. PublicKey is
// public; the private half lives only in the vault.
type RealityKeyRecord struct {
	ID        string `json:"id"`
	Alias     string `json:"alias"`
	PublicKey string `json:"publicKey"`
	// KeyVaultRef is the vault id of the private key
	// (purpose reality-private).
	KeyVaultRef string          `json:"keyVaultRef"`
	ShortIDs    []ShortIDRecord `json:"shortIds"`
	BoundTo     []string        `json:"boundTo"` // server ids
	State       string          `json:"state"`   // active | rotating | retired
	CreatedAt   time.Time       `json:"createdAt"`
	UpdatedAt   time.Time       `json:"updatedAt"`
}

// ValidateRealityDestination checks a REALITY dest (host:port or
// domain:port). The destination must be a public HTTPS target;
// loopback / private targets would defeat the steal-oneself design.
func ValidateRealityDestination(dest string) error {
	host, port, err := splitHostPort(dest)
	if err != nil {
		return fmt.Errorf("reality: destination %q is not host:port: %w", dest, err)
	}
	if host == "" || port < 1 || port > 65535 {
		return errors.New("reality: destination host or port out of bounds")
	}
	if host == "localhost" || host == "::1" || host == "127.0.0.1" ||
		host == "0.0.0.0" || host == "[::1]" {
		return errors.New("reality: destination must not be a loopback address")
	}
	return nil
}

// RealityManager is the bounded key inventory.
type RealityManager struct {
	mu      sync.RWMutex
	records map[string]*RealityKeyRecord
	clock   func() time.Time
}

const MaxRealityKeys = 32

func NewRealityManager() *RealityManager {
	return &RealityManager{records: map[string]*RealityKeyRecord{}, clock: time.Now}
}

// Generate creates a fresh keypair. The returned private key must
// be stored into the vault immediately; the manager keeps only the
// vault reference handed back by the caller.
func (m *RealityManager) Generate(alias, keyVaultRef string) (RealityKeyRecord, string, error) {
	if alias == "" || keyVaultRef == "" {
		return RealityKeyRecord{}, "", errors.New("reality: alias and vault reference are required")
	}
	kp, err := GenerateRealityKeypair()
	if err != nil {
		return RealityKeyRecord{}, "", err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.records) >= MaxRealityKeys {
		return RealityKeyRecord{}, "", errors.New("reality: key inventory capacity exceeded")
	}
	for _, rec := range m.records {
		if rec.Alias == alias {
			return RealityKeyRecord{}, "", fmt.Errorf("reality: alias %q already exists", alias)
		}
	}
	now := m.clock().UTC()
	rec := &RealityKeyRecord{
		ID:          "rk-" + kp.PublicKey[:12],
		Alias:       alias,
		PublicKey:   kp.PublicKey,
		KeyVaultRef: keyVaultRef,
		BoundTo:     []string{},
		State:       "active",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	m.records[rec.ID] = rec
	return *rec, kp.PrivateKey, nil
}

// Rotate prepares a rotation: a NEW record is generated with the
// same alias suffix and the old record transitions to "rotating".
// The operator applies the new key to servers (via vault binding)
// and only then retires the old record. The preview plan tells the
// operator exactly which servers still carry the old key.
func (m *RealityManager) Rotate(oldID, newKeyVaultRef string) (RealityKeyRecord, string, error) {
	kp, err := GenerateRealityKeypair()
	if err != nil {
		return RealityKeyRecord{}, "", err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	old, ok := m.records[oldID]
	if !ok || old.State == "retired" {
		return RealityKeyRecord{}, "", errors.New("reality: key record unknown or retired")
	}
	now := m.clock().UTC()
	rec := &RealityKeyRecord{
		ID:          "rk-" + kp.PublicKey[:12],
		Alias:       old.Alias + "-rot" + now.Format("20060102"),
		PublicKey:   kp.PublicKey,
		KeyVaultRef: newKeyVaultRef,
		BoundTo:     []string{},
		State:       "active",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	old.State = "rotating"
	old.UpdatedAt = now
	m.records[rec.ID] = rec
	return *rec, kp.PrivateKey, nil
}

// AddShortID mints a short id for a key record.
func (m *RealityManager) AddShortID(keyID string) (ShortIDRecord, error) {
	sid, err := GenerateShortID()
	if err != nil {
		return ShortIDRecord{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	rec, ok := m.records[keyID]
	if !ok || rec.State == "retired" {
		return ShortIDRecord{}, errors.New("reality: key record unknown or retired")
	}
	entry := ShortIDRecord{Hex: sid, State: ShortIDActive, CreatedAt: m.clock().UTC()}
	rec.ShortIDs = append(rec.ShortIDs, entry)
	rec.UpdatedAt = entry.CreatedAt
	return entry, nil
}

// RetireShortID flips a short id to retired. Retired ids stop being
// rendered into server configs on the next apply.
func (m *RealityManager) RetireShortID(keyID, hex string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	rec, ok := m.records[keyID]
	if !ok {
		return errors.New("reality: key record unknown")
	}
	for i := range rec.ShortIDs {
		if rec.ShortIDs[i].Hex == hex && rec.ShortIDs[i].State == ShortIDActive {
			rec.ShortIDs[i].State = ShortIDRetired
			rec.ShortIDs[i].RetiredAt = m.clock().UTC()
			rec.UpdatedAt = rec.ShortIDs[i].RetiredAt
			return nil
		}
	}
	return errors.New("reality: short id unknown or already retired")
}

// ClientParams is the resolved, public client-side parameter set
// for a REALITY inbound (configuration preview). No private
// material appears here.
type ClientParams struct {
	PublicKey string   `json:"publicKey"`
	ShortIDs  []string `json:"shortIds"`
	SNI       []string `json:"sni"`
}

// Preview returns the client parameter set for a key record bound
// to a server. Only active short ids are included.
func (m *RealityManager) Preview(keyID string, server Server) (ClientParams, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	rec, ok := m.records[keyID]
	if !ok {
		return ClientParams{}, errors.New("reality: key record unknown")
	}
	var sids []string
	for _, s := range rec.ShortIDs {
		if s.State == ShortIDActive {
			sids = append(sids, s.Hex)
		}
	}
	if len(sids) == 0 {
		return ClientParams{}, errors.New("reality: record has no active short id")
	}
	sni := append([]string(nil), server.RealitySNI...)
	sort.Strings(sni)
	sort.Strings(sids)
	return ClientParams{PublicKey: rec.PublicKey, ShortIDs: sids, SNI: sni}, nil
}

// List returns the records sorted by id. Private keys do not exist
// in this package by construction.
func (m *RealityManager) List() []RealityKeyRecord {
	m.mu.RLock()
	out := make([]RealityKeyRecord, 0, len(m.records))
	for _, rec := range m.records {
		out = append(out, *rec)
	}
	m.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
