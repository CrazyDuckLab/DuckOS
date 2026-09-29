package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/CrazyDuckLab/DuckOS/internal/vault"
)

// VaultLifecycle owns the Agent's private vault store. The lifecycle
// is explicit: the Agent reads the master key from a file at startup,
// opens the vault, and hands out a resolver. Every wireguard/xray
// kernel adapter uses the resolver to look up private bytes on
// demand; no caller caches the bytes between operations.
//
// The lifecycle is intentionally small: Open, Close, Resolver,
// List, Revoke, Put. Each method either advances the durable state or
// returns an explicit refusal. A closed lifecycle is non-recoverable
// because the master key is bound to a single store instance.
type VaultLifecycle struct {
	mu      sync.Mutex
	store   *vault.Store
	path    string
	keyPath string
	keyID   string
	master  []byte
	closed  bool
}

// VaultDeps are the bounded inputs the lifecycle needs. The Master
// path is a 32-byte file with mode 0600 or 0640. The path is read at
// Open and never cached.
type VaultDeps struct {
	Path       string
	MasterFile string
}

// OpenVault reads the master key, opens or initializes the vault
// store, and returns the lifecycle. A missing Path or unreadable
// master key is an explicit refusal.
func OpenVault(_ context.Context, deps VaultDeps) (*VaultLifecycle, error) {
	if deps.Path == "" || deps.MasterFile == "" {
		return nil, errors.New("vault lifecycle requires explicit path and master key file")
	}
	master, err := readMasterKey(deps.MasterFile)
	if err != nil {
		return nil, fmt.Errorf("read vault master key: %w", err)
	}
	if err := os.MkdirAll(deps.Path, 0o700); err != nil {
		return nil, fmt.Errorf("vault path: %w", err)
	}
	store, err := vault.OpenStore(deps.Path, master)
	if err != nil {
		// Initialise a brand-new store when the directory is empty.
		store, err = vault.InitializeStore(deps.Path, master)
		if err != nil {
			return nil, fmt.Errorf("open vault: %w", err)
		}
	}
	return &VaultLifecycle{store: store, path: deps.Path, keyPath: deps.MasterFile, keyID: vaultFingerprint(master), master: master}, nil
}

// Close releases the in-memory master key and the durable store. The
// lifecycle cannot be reopened; callers must construct a new one.
func (l *VaultLifecycle) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return nil
	}
	l.closed = true
	clear(l.master)
	if l.store != nil {
		return l.store.Close()
	}
	return nil
}

// Resolver returns a key resolver that the kernel adapter can use
// without holding a reference to the lifecycle. The resolver consults
// the live store on every call so a rotation or revoke propagates
// without restarting the Agent.
func (l *VaultLifecycle) Resolver() *VaultResolver {
	return &VaultResolver{Lifecycle: l}
}

// KeyID returns the master key fingerprint. Callers verify that a
// reference's KeyID still matches the live key after rotation.
func (l *VaultLifecycle) KeyID() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.keyID
}

// List returns a metadata-only view of every stored reference.
func (l *VaultLifecycle) List() ([]vault.Reference, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return nil, errors.New("vault lifecycle is closed")
	}
	return l.store.ListReferences()
}

// Revoke marks the reference as unusable. Existing kernel adapters
// see ErrRevoked on the next resolve call.
func (l *VaultLifecycle) Revoke(id, revision, actor, reason string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return errors.New("vault lifecycle is closed")
	}
	return l.store.Revoke(id, revision, actor, reason)
}

// Put durably records a new secret. Private bytes are cleared from
// the caller's memory after return; the caller must own the bytes
// for as long as the put is in flight.
func (l *VaultLifecycle) Put(id, purpose string, plaintext []byte) (vault.Reference, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return vault.Reference{}, errors.New("vault lifecycle is closed")
	}
	return l.store.Put(id, purpose, plaintext)
}

// VaultResolver exposes a key resolver with the same shape
// the wireguard kernel adapter uses. The resolver is the only path
// that returns secret bytes to kernel adapters.
type VaultResolver struct {
	Lifecycle *VaultLifecycle
}

func (r VaultResolver) Resolve(_ context.Context, ref vault.Reference) ([]byte, error) {
	r.Lifecycle.mu.Lock()
	if r.Lifecycle.closed || r.Lifecycle.store == nil {
		r.Lifecycle.mu.Unlock()
		return nil, errors.New("vault lifecycle is closed")
	}
	if r.Lifecycle.store.IsRevoked(ref) {
		r.Lifecycle.mu.Unlock()
		return nil, errors.New("vault reference is revoked")
	}
	secret, err := r.Lifecycle.store.Get(ref)
	r.Lifecycle.mu.Unlock()
	if err != nil {
		return nil, err
	}
	if secret == nil {
		return nil, errors.New("vault reference resolves to no secret")
	}
	return secret.Reveal(), nil
}

// Standard private Agent state layout. The constants are exported so
// tests and the install scripts agree on the on-disk paths.
const (
	StandardVaultPath       = "/var/lib/duckos-agent/vault"
	StandardVaultMasterPath = "/var/lib/duckos-agent/vault/master.key"
)

func readMasterKey(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("master key file is not a regular file")
	}
	if info.Mode().Perm()&0o037 != 0 {
		return nil, errors.New("master key file is world-accessible")
	}
	if info.Size() > 256 {
		return nil, errors.New("master key file exceeds size limit")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(data) < 32 {
		return nil, errors.New("master key file is shorter than 32 bytes")
	}
	out := make([]byte, 32)
	copy(out, data[:32])
	clear(data)
	return out, nil
}

func vaultFingerprint(master []byte) string {
	// Replicates the algorithm in vault/openStore so the lifecycle
	// can surface the same KeyID. The vault store does
	// sha256("DuckOS/vault/key-id/v1\x00" || key).
	const prefix = "DuckOS/vault/key-id/v1\x00"
	sum := sha256.Sum256(append([]byte(prefix), master...))
	return hex.EncodeToString(sum[:16])
}

var _ = filepath.Join
