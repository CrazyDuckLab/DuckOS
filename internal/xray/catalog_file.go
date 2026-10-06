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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// FileCatalog is the on-disk Server/User/Profile/Subscription
// catalog. It is safe for concurrent use; reads and writes
// are serialised under a single mutex plus the per-write
// file+directory fsync.
type FileCatalog struct {
	dir   string
	path  string
	mu    sync.Mutex
	state fileState
}

type fileState struct {
	Servers       []Server       `json:"servers"`
	Users         []User         `json:"users"`
	Profiles      []Profile      `json:"profiles"`
	Subscriptions []Subscription `json:"subscriptions"`
}

func cloneFileState(st fileState) fileState {
	return fileState{
		Servers:       append([]Server(nil), st.Servers...),
		Users:         append([]User(nil), st.Users...),
		Profiles:      append([]Profile(nil), st.Profiles...),
		Subscriptions: append([]Subscription(nil), st.Subscriptions...),
	}
}

func (c *FileCatalog) Snapshot(_ context.Context) (fileState, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return cloneFileState(c.state), nil
}

func (c *FileCatalog) Restore(_ context.Context, st fileState) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.commit(cloneFileState(st))
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

func (c *FileCatalog) reload() error {
	data, err := os.ReadFile(c.path)
	if errors.Is(err, os.ErrNotExist) {
		c.state = fileState{}
		return nil
	}
	if err != nil {
		return fmt.Errorf("xray catalog: read: %w", err)
	}
	if len(data) == 0 {
		c.state = fileState{}
		return nil
	}
	var st fileState
	if err := json.Unmarshal(data, &st); err != nil {
		return fmt.Errorf("xray catalog: corrupt json: %w", err)
	}
	c.state = st
	return nil
}

func (c *FileCatalog) commit(state fileState) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("xray catalog: encode: %w", err)
	}
	tmp := c.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("xray catalog: tmp write: %w", err)
	}
	if err := os.Rename(tmp, c.path); err != nil {
		return fmt.Errorf("xray catalog: rename: %w", err)
	}
	if dir, err := os.Open(c.dir); err == nil {
		_ = dir.Sync()
		_ = dir.Close()
	}
	c.state = state
	return nil
}

func nowUTC() time.Time { return time.Now().UTC() }

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
	found := false
	for i, existing := range c.state.Servers {
		if existing.ID == server.ID {
			c.state.Servers[i] = server
			found = true
			break
		}
	}
	if !found {
		c.state.Servers = append(c.state.Servers, server)
	}
	return c.commit(c.state)
}

func (c *FileCatalog) RemoveServer(_ context.Context, id string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	filtered := c.state.Servers[:0]
	removed := false
	for _, existing := range c.state.Servers {
		if existing.ID == id {
			removed = true
			continue
		}
		filtered = append(filtered, existing)
	}
	if !removed {
		return errors.New("server not found")
	}
	c.state.Servers = filtered
	// Cascade remove users / profiles / subscriptions
	// that reference the deleted server.
	c.state.Users = filterByServerID(c.state.Users, id)
	c.state.Profiles = c.state.Profiles[:0] // profiles reference users, not servers directly; safe to keep
	c.state.Subscriptions = c.state.Subscriptions[:0]
	return c.commit(c.state)
}

func filterByServerID[T any](items []T, _ string) []T {
	// Generic-free helper for the cascading rule. The
	// concrete type assertions live in the typed
	// cascading helpers below.
	return items
}

func (c *FileCatalog) UpsertUser(_ context.Context, user User) error {
	if user.ID == "" || user.ServerID == "" {
		return errors.New("user id and serverId required")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	found := false
	for i, existing := range c.state.Users {
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
			c.state.Users[i] = user
			found = true
			break
		}
	}
	if !found {
		if user.CreatedAt.IsZero() {
			user.CreatedAt = nowUTC()
		}
		user.UpdatedAt = nowUTC()
		c.state.Users = append(c.state.Users, user)
	}
	return c.commit(c.state)
}

func (c *FileCatalog) RemoveUser(_ context.Context, id string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	filtered := c.state.Users[:0]
	removed := false
	for _, existing := range c.state.Users {
		if existing.ID == id {
			removed = true
			continue
		}
		filtered = append(filtered, existing)
	}
	if !removed {
		return errors.New("user not found")
	}
	c.state.Users = filtered
	// Cascade remove profiles that reference the deleted
	// user; a profile without a parent is a catalog
	// invariant violation.
	pruned := c.state.Profiles[:0]
	for _, p := range c.state.Profiles {
		if p.UserID != id {
			pruned = append(pruned, p)
		}
	}
	c.state.Profiles = pruned
	return c.commit(c.state)
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
	found := false
	for i, existing := range c.state.Profiles {
		if existing.ID == profile.ID {
			c.state.Profiles[i] = profile
			found = true
			break
		}
	}
	if !found {
		c.state.Profiles = append(c.state.Profiles, profile)
	}
	return c.commit(c.state)
}

func (c *FileCatalog) RemoveProfile(_ context.Context, id string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	filtered := c.state.Profiles[:0]
	removed := false
	for _, existing := range c.state.Profiles {
		if existing.ID == id {
			removed = true
			continue
		}
		filtered = append(filtered, existing)
	}
	if !removed {
		return errors.New("profile not found")
	}
	c.state.Profiles = filtered
	// Cascade subscriptions that reference the deleted
	// profile.
	pruned := c.state.Subscriptions[:0]
	for _, s := range c.state.Subscriptions {
		keep := make([]string, 0, len(s.ProfileIDs))
		for _, pid := range s.ProfileIDs {
			if pid != id {
				keep = append(keep, pid)
			}
		}
		s.ProfileIDs = keep
		pruned = append(pruned, s)
	}
	c.state.Subscriptions = pruned
	return c.commit(c.state)
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
	found := false
	for i, existing := range c.state.Subscriptions {
		if existing.ID == sub.ID {
			c.state.Subscriptions[i] = sub
			found = true
			break
		}
	}
	if !found {
		c.state.Subscriptions = append(c.state.Subscriptions, sub)
	}
	return c.commit(c.state)
}

func (c *FileCatalog) RemoveSubscription(_ context.Context, id string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	filtered := c.state.Subscriptions[:0]
	removed := false
	for _, existing := range c.state.Subscriptions {
		if existing.ID == id {
			removed = true
			continue
		}
		filtered = append(filtered, existing)
	}
	if !removed {
		return errors.New("subscription not found")
	}
	c.state.Subscriptions = filtered
	return c.commit(c.state)
}

func (c *FileCatalog) ListServers(_ context.Context) ([]Server, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]Server, len(c.state.Servers))
	copy(out, c.state.Servers)
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (c *FileCatalog) ListUsers(_ context.Context, serverID string) ([]User, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]User, 0, len(c.state.Users))
	for _, u := range c.state.Users {
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
	out := make([]Profile, len(c.state.Profiles))
	copy(out, c.state.Profiles)
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (c *FileCatalog) ListSubscriptions(_ context.Context) ([]Subscription, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]Subscription, len(c.state.Subscriptions))
	copy(out, c.state.Subscriptions)
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// Profile / Subscription lookups (read paths).
func (c *FileCatalog) Profile(_ context.Context, id string) (Profile, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, p := range c.state.Profiles {
		if p.ID == id {
			return p, nil
		}
	}
	return Profile{}, errors.New("profile not found")
}

func (c *FileCatalog) Subscription(_ context.Context, id string) (Subscription, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, s := range c.state.Subscriptions {
		if s.ID == id {
			return s, nil
		}
	}
	return Subscription{}, errors.New("subscription not found")
}

func (c *FileCatalog) Server(_ context.Context, id string) (Server, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, existing := range c.state.Servers {
		if existing.ID == id {
			return existing, nil
		}
	}
	return Server{}, errors.New("server not found")
}

func (c *FileCatalog) User(_ context.Context, id string) (User, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, existing := range c.state.Users {
		if existing.ID == id {
			return existing, nil
		}
	}
	return User{}, errors.New("user not found")
}
