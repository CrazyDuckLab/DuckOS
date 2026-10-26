// Package xray durable catalog (DV01 source-backed lister).
//
// FileCatalog persists the durable Server / User / Profile /
// Subscription records to a 0600-mode JSON file under a
// parent directory that is fsync-ed on every commit. The
// catalog is the only source of truth the Xray listers read
// from; without it /api/v1/objects/xray.* returns
// engine_unavailable per the audit's honest-gap policy.
//
// On read the catalog returns the durable records verbatim;
// secret bytes (UUID, password, short IDs) are stored as
// vault references in the canonical record, never in the
// catalog itself.
package xray

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"syscall"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/persist"
)

// FileCatalog is the on-disk Server/User/Profile/Subscription
// catalog. It is safe for concurrent use; reads and writes
// are serialised under a single mutex plus the per-write
// file+directory fsync.
type FileCatalog struct {
	dir           string
	path          string
	mu            sync.Mutex
	state         fileState
	degraded      error
	pendingDigest [32]byte
	// fault is an instance-local durability-stage oracle for fault tests.
	fault func(string) error
}

type fileState struct {
	Servers       []Server       `json:"servers"`
	Users         []User         `json:"users"`
	Profiles      []Profile      `json:"profiles"`
	Subscriptions []Subscription `json:"subscriptions"`
}

const maxCatalogBytes = 4 << 20

func cloneFileState(st fileState) (fileState, error) {
	raw, err := json.Marshal(st)
	if err != nil {
		return fileState{}, errors.New("xray catalog: invalid snapshot")
	}
	var out fileState
	if err := json.Unmarshal(raw, &out); err != nil {
		return fileState{}, errors.New("xray catalog: invalid snapshot")
	}
	return out, nil
}

func (c *FileCatalog) cloneLocked() (fileState, error) {
	if c.degraded != nil {
		return fileState{}, c.degraded
	}
	return cloneFileState(c.state)
}
func (c *FileCatalog) Snapshot(_ context.Context) (fileState, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.cloneLocked()
}
func (c *FileCatalog) Restore(_ context.Context, st fileState) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.commit(st)
}

// OpenFileCatalog prepares the durable catalog rooted at dir.
// The file is created with 0600 permissions on first write
// and the parent directory is fsync-ed to make the catalog
// recoverable after a crash. A corrupt file is treated as a
// contract-correct "honest unavailable" surface.
func OpenFileCatalog(dir string) (*FileCatalog, error) {
	if dir == "" {
		return nil, errors.New("xray catalog: directory required")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("xray catalog: mkdir: %w", err)
	}
	path := filepath.Join(dir, "catalog.json")
	c := &FileCatalog{dir: dir, path: path}
	if err := c.reload(); err != nil {
		return nil, err
	}
	return c, nil
}

func readCatalog(path string) (fileState, []byte, error) {
	var st fileState
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return st, nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return st, nil, errors.New("xray catalog: not a regular file")
	}
	raw, err := io.ReadAll(io.LimitReader(f, maxCatalogBytes+1))
	if err != nil || len(raw) > maxCatalogBytes {
		return st, nil, errors.New("xray catalog: read failed or size limit exceeded")
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return st, nil, errors.New("xray catalog: invalid document")
	}
	if err := json.Unmarshal(raw, &st); err != nil {
		return st, nil, errors.New("xray catalog: corrupt document")
	}
	return st, raw, nil
}
func (c *FileCatalog) reload() error {
	st, _, err := readCatalog(c.path)
	if errors.Is(err, os.ErrNotExist) {
		c.state = fileState{}
		return nil
	}
	if err != nil {
		return err
	}
	c.state = st
	return nil
}
func syncCatalogDir(dir string) error {
	f, err := os.Open(dir)
	if err != nil {
		return err
	}
	err = f.Sync()
	closeErr := f.Close()
	if err != nil {
		return err
	}
	return closeErr
}
func (c *FileCatalog) stage(name string) error {
	if c.fault != nil {
		return c.fault(name)
	}
	return nil
}
func (c *FileCatalog) commit(next fileState) error {
	if c.degraded != nil {
		return c.degraded
	}
	data, err := json.MarshalIndent(next, "", "  ")
	if err != nil || len(data) > maxCatalogBytes {
		return errors.New("xray catalog: encoding or size limit failure")
	}
	// Sever all caller-owned nested pointers/maps/slices before publication.
	var isolated fileState
	if err := json.Unmarshal(data, &isolated); err != nil {
		return errors.New("xray catalog: invalid state")
	}
	tmp := c.path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return fmt.Errorf("xray catalog: temporary open: %w", err)
	}
	renamed := false
	defer func() {
		_ = f.Close()
		if !renamed {
			_ = os.Remove(tmp)
		}
	}()
	if err := f.Chmod(0600); err != nil {
		return err
	}
	if err := c.stage("write"); err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		return err
	}
	if err := c.stage("file-sync"); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := c.stage("close"); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := c.stage("rename"); err != nil {
		return err
	}
	if err := os.Rename(tmp, c.path); err != nil {
		return err
	}
	renamed = true
	// From this point the new state is visible. Never overwrite it with a stale
	// snapshot on an uncertain outcome; refuse reads and writes until reconcile.
	c.state = isolated
	c.pendingDigest = sha256.Sum256(data)
	err = c.stage("directory-sync")
	if err == nil {
		err = syncCatalogDir(c.dir)
	}
	if err != nil {
		c.degraded = fmt.Errorf("%w: xray catalog directory sync", persist.ErrUncertain)
		return c.degraded
	}
	c.pendingDigest = [32]byte{}
	return nil
}

// Degraded exposes only a non-secret diagnosis for the existing storage UI.
func (c *FileCatalog) Degraded() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.degraded != nil {
		return "catalog commit uncertain; runtime must remain stopped until reconciliation"
	}
	return ""
}

// ReconcileStorage never imports a different/older document. It confirms the
// exact renamed snapshot and its durability before admitting new operations.
func (c *FileCatalog) ReconcileStorage() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.degraded == nil {
		return nil
	}
	st, raw, err := readCatalog(c.path)
	if err != nil {
		return err
	}
	if sha256.Sum256(raw) != c.pendingDigest {
		return errors.New("xray catalog: reconcile snapshot mismatch; remain fail-closed")
	}
	f, err := os.OpenFile(c.path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return err
	}
	syncErr := f.Sync()
	closeErr := f.Close()
	if syncErr != nil {
		return syncErr
	}
	if closeErr != nil {
		return closeErr
	}
	if err := c.stage("directory-sync"); err != nil {
		return err
	}
	if err := syncCatalogDir(c.dir); err != nil {
		return err
	}
	c.state = st
	c.degraded = nil
	c.pendingDigest = [32]byte{}
	return nil
}

func nowUTC() time.Time { return time.Now().UTC() }

// Dir returns the catalog's state directory (for tooling that must
// re-open the same durable state).
func (c *FileCatalog) Dir() string { return c.dir }

// GetSubscription returns one durable subscription record.
func (c *FileCatalog) GetSubscription(_ context.Context, id string) (Subscription, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	snapshot, err := c.cloneLocked()
	if err != nil {
		return Subscription{}, err
	}
	for _, s := range snapshot.Subscriptions {
		if s.ID == id {
			return s, nil
		}
	}
	return Subscription{}, fmt.Errorf("xray: subscription %s not found", id)
}

// GetProfile returns one durable profile record.
func (c *FileCatalog) GetProfile(_ context.Context, id string) (Profile, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	snapshot, err := c.cloneLocked()
	if err != nil {
		return Profile{}, err
	}
	for _, p := range snapshot.Profiles {
		if p.ID == id {
			return p, nil
		}
	}
	return Profile{}, fmt.Errorf("xray: profile %s not found", id)
}

func (c *FileCatalog) UpsertServer(_ context.Context, server Server) error {
	if server.ID == "" {
		return errors.New("server id required")
	}
	if server.CreatedAt.IsZero() {
		server.CreatedAt = nowUTC()
	}
	server.UpdatedAt = nowUTC()
	c.mu.Lock()
	defer c.mu.Unlock()
	next, err := c.cloneLocked()
	if err != nil {
		return err
	}
	found := false
	for i, existing := range next.Servers {
		if existing.ID == server.ID {
			next.Servers[i] = server
			found = true
			break
		}
	}
	if !found {
		next.Servers = append(next.Servers, server)
	}
	return c.commit(next)
}

func (c *FileCatalog) RemoveServer(_ context.Context, id string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	// Work on a copy: a rejected disk commit must not publish deletion in memory.
	next, err := c.cloneLocked()
	if err != nil {
		return err
	}
	keptServers := next.Servers[:0]
	found := false
	for _, server := range next.Servers {
		if server.ID == id {
			found = true
			continue
		}
		keptServers = append(keptServers, server)
	}
	if !found {
		return errors.New("server not found")
	}
	next.Servers = keptServers
	removedUsers := map[string]bool{}
	keptUsers := next.Users[:0]
	for _, user := range next.Users {
		if user.ServerID == id {
			removedUsers[user.ID] = true
			continue
		}
		keptUsers = append(keptUsers, user)
	}
	next.Users = keptUsers
	removedProfiles := map[string]bool{}
	keptProfiles := next.Profiles[:0]
	for _, profile := range next.Profiles {
		if removedUsers[profile.UserID] {
			removedProfiles[profile.ID] = true
			continue
		}
		keptProfiles = append(keptProfiles, profile)
	}
	next.Profiles = keptProfiles
	// Keep subscription identity and all profiles belonging to other servers.
	for index, sub := range next.Subscriptions {
		kept := make([]string, 0, len(sub.ProfileIDs))
		for _, profileID := range sub.ProfileIDs {
			if !removedProfiles[profileID] {
				kept = append(kept, profileID)
			}
		}
		next.Subscriptions[index].ProfileIDs = kept
	}
	return c.commit(next)
}

func (c *FileCatalog) UpsertUser(_ context.Context, user User) error {
	if user.ID == "" || user.ServerID == "" {
		return errors.New("user id and serverId required")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	next, err := c.cloneLocked()
	if err != nil {
		return err
	}
	found := false
	for i, existing := range next.Users {
		if existing.ID == user.ID {
			// B-006 (BETA.2): preserve CreatedAt from
			// the existing record. A common failure
			// mode was callers passing a fresh User
			// struct (CreatedAt is zero) on update,
			// which silently destroyed the audit
			// timestamp. We only stamp CreatedAt when
			// the record is brand-new; on update we
			// reuse the existing CreatedAt.
			if user.CreatedAt.IsZero() {
				user.CreatedAt = existing.CreatedAt
			}
			user.UpdatedAt = nowUTC()
			next.Users[i] = user
			found = true
			break
		}
	}
	if !found {
		if user.CreatedAt.IsZero() {
			user.CreatedAt = nowUTC()
		}
		user.UpdatedAt = nowUTC()
		next.Users = append(next.Users, user)
	}
	return c.commit(next)
}

func (c *FileCatalog) RemoveUser(_ context.Context, id string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	next, err := c.cloneLocked()
	if err != nil {
		return err
	}
	filtered := next.Users[:0]
	removed := false
	for _, existing := range next.Users {
		if existing.ID == id {
			removed = true
			continue
		}
		filtered = append(filtered, existing)
	}
	if !removed {
		return errors.New("user not found")
	}
	next.Users = filtered
	// Cascade remove profiles that reference the deleted
	// user; a profile without a parent is a catalog
	// invariant violation.
	removedProfiles := map[string]bool{}
	pruned := next.Profiles[:0]
	for _, p := range next.Profiles {
		if p.UserID != id {
			pruned = append(pruned, p)
		} else {
			removedProfiles[p.ID] = true
		}
	}
	next.Profiles = pruned
	for i, sub := range next.Subscriptions {
		kept := make([]string, 0, len(sub.ProfileIDs))
		for _, pid := range sub.ProfileIDs {
			if !removedProfiles[pid] {
				kept = append(kept, pid)
			}
		}
		next.Subscriptions[i].ProfileIDs = kept
	}
	return c.commit(next)
}

func (c *FileCatalog) UpsertProfile(_ context.Context, profile Profile) error {
	if profile.ID == "" || profile.UserID == "" {
		return errors.New("profile id and userId required")
	}
	if profile.CreatedAt.IsZero() {
		profile.CreatedAt = nowUTC()
	}
	profile.UpdatedAt = nowUTC()
	c.mu.Lock()
	defer c.mu.Unlock()
	next, err := c.cloneLocked()
	if err != nil {
		return err
	}
	found := false
	for i, existing := range next.Profiles {
		if existing.ID == profile.ID {
			next.Profiles[i] = profile
			found = true
			break
		}
	}
	if !found {
		next.Profiles = append(next.Profiles, profile)
	}
	return c.commit(next)
}

func (c *FileCatalog) RemoveProfile(_ context.Context, id string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	next, err := c.cloneLocked()
	if err != nil {
		return err
	}
	filtered := next.Profiles[:0]
	removed := false
	for _, existing := range next.Profiles {
		if existing.ID == id {
			removed = true
			continue
		}
		filtered = append(filtered, existing)
	}
	if !removed {
		return errors.New("profile not found")
	}
	next.Profiles = filtered
	// Cascade subscriptions that reference the deleted
	// profile.
	pruned := next.Subscriptions[:0]
	for _, s := range next.Subscriptions {
		keep := make([]string, 0, len(s.ProfileIDs))
		for _, pid := range s.ProfileIDs {
			if pid != id {
				keep = append(keep, pid)
			}
		}
		s.ProfileIDs = keep
		pruned = append(pruned, s)
	}
	next.Subscriptions = pruned
	return c.commit(next)
}

func (c *FileCatalog) UpsertSubscription(_ context.Context, sub Subscription) error {
	if sub.ID == "" {
		return errors.New("subscription id required")
	}
	if sub.CreatedAt.IsZero() {
		sub.CreatedAt = nowUTC()
	}
	sub.UpdatedAt = nowUTC()
	c.mu.Lock()
	defer c.mu.Unlock()
	next, err := c.cloneLocked()
	if err != nil {
		return err
	}
	found := false
	for i, existing := range next.Subscriptions {
		if existing.ID == sub.ID {
			next.Subscriptions[i] = sub
			found = true
			break
		}
	}
	if !found {
		next.Subscriptions = append(next.Subscriptions, sub)
	}
	return c.commit(next)
}

func (c *FileCatalog) RemoveSubscription(_ context.Context, id string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	next, err := c.cloneLocked()
	if err != nil {
		return err
	}
	filtered := next.Subscriptions[:0]
	removed := false
	for _, existing := range next.Subscriptions {
		if existing.ID == id {
			removed = true
			continue
		}
		filtered = append(filtered, existing)
	}
	if !removed {
		return errors.New("subscription not found")
	}
	next.Subscriptions = filtered
	return c.commit(next)
}

func (c *FileCatalog) ListServers(_ context.Context) ([]Server, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	snapshot, err := c.cloneLocked()
	if err != nil {
		return nil, err
	}
	out := make([]Server, len(snapshot.Servers))
	copy(out, snapshot.Servers)
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (c *FileCatalog) ListUsers(_ context.Context, serverID string) ([]User, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	snapshot, err := c.cloneLocked()
	if err != nil {
		return nil, err
	}
	out := make([]User, 0, len(snapshot.Users))
	for _, u := range snapshot.Users {
		if serverID == "" || u.ServerID == serverID {
			out = append(out, u)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// ListProfiles / ListSubscriptions extend the canonical
// CatalogPersistence with the read paths DV01 needs.
func (c *FileCatalog) ListProfiles(_ context.Context) ([]Profile, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	snapshot, err := c.cloneLocked()
	if err != nil {
		return nil, err
	}
	out := make([]Profile, len(snapshot.Profiles))
	copy(out, snapshot.Profiles)
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (c *FileCatalog) ListSubscriptions(_ context.Context) ([]Subscription, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	snapshot, err := c.cloneLocked()
	if err != nil {
		return nil, err
	}
	out := make([]Subscription, len(snapshot.Subscriptions))
	copy(out, snapshot.Subscriptions)
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// Profile / Subscription lookups (read paths).
func (c *FileCatalog) Profile(_ context.Context, id string) (Profile, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	snapshot, err := c.cloneLocked()
	if err != nil {
		return Profile{}, err
	}
	for _, p := range snapshot.Profiles {
		if p.ID == id {
			return p, nil
		}
	}
	return Profile{}, errors.New("profile not found")
}

func (c *FileCatalog) Subscription(_ context.Context, id string) (Subscription, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	snapshot, err := c.cloneLocked()
	if err != nil {
		return Subscription{}, err
	}
	for _, s := range snapshot.Subscriptions {
		if s.ID == id {
			return s, nil
		}
	}
	return Subscription{}, errors.New("subscription not found")
}

func (c *FileCatalog) Server(_ context.Context, id string) (Server, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	snapshot, err := c.cloneLocked()
	if err != nil {
		return Server{}, err
	}
	for _, existing := range snapshot.Servers {
		if existing.ID == id {
			return existing, nil
		}
	}
	return Server{}, errors.New("server not found")
}

func (c *FileCatalog) User(_ context.Context, id string) (User, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	snapshot, err := c.cloneLocked()
	if err != nil {
		return User{}, err
	}
	for _, existing := range snapshot.Users {
		if existing.ID == id {
			return existing, nil
		}
	}
	return User{}, errors.New("user not found")
}
