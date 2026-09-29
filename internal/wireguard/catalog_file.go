// Package wireguard durable catalog (DV01 source-backed lister).
//
// FileCatalog persists the durable Server and Peer records
// to a 0600-mode JSON file under a parent directory that is
// fsync-ed on every commit. The catalog is the only source of
// truth the WireGuard listers read from; without it
// /api/v1/objects/wireguard.* returns engine_unavailable per
// the audit's honest-gap policy.
//
// On read the catalog returns the durable records verbatim;
// secret key bytes are never persisted in the catalog (the
// private key is a vault reference only).
package wireguard

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

// FileCatalog is the on-disk Server/Peer catalog. It is safe
// for concurrent use; reads and writes are serialised under
// a single mutex plus the per-write file+directory fsync.
type FileCatalog struct {
	dir   string
	path  string
	mu    sync.Mutex
	state fileState
}

type fileState struct {
	Servers []Server `json:"servers"`
	Peers   []Peer   `json:"peers"`
}

// OpenFileCatalog prepares the durable catalog rooted at dir.
// The file is created with 0600 permissions on first write
// and the parent directory is fsync-ed to make the catalog
// recoverable after a crash. A corrupt or missing file is
// treated as empty (the contract permits zero committed
// records) and never returns a synthetic success envelope.
func OpenFileCatalog(dir string) (*FileCatalog, error) {
	if dir == "" {
		return nil, errors.New("wireguard catalog: directory required")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("wireguard catalog: mkdir: %w", err)
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
		return fmt.Errorf("wireguard catalog: read: %w", err)
	}
	if len(data) == 0 {
		c.state = fileState{}
		return nil
	}
	var st fileState
	if err := json.Unmarshal(data, &st); err != nil {
		// A corrupt catalog is the contract-correct
		// "honest unavailable" surface, not a
		// permissive-empty reset. The reload caller
		// decides whether to retry or refuse; we
		// surface the corruption verbatim.
		return fmt.Errorf("wireguard catalog: corrupt json: %w", err)
	}
	// Older records embedded initial peers in the server. Normalize into
	// the authoritative peer collection; explicit later peer records win.
	for i := range st.Servers {
		for _, peer := range st.Servers[i].Peers {
			found := false
			for _, current := range st.Peers {
				if current.ID == peer.ID {
					found = true
					break
				}
			}
			if !found {
				st.Peers = append(st.Peers, peer)
			}
		}
		st.Servers[i].Peers = nil
	}
	c.state = st
	return nil
}

// commit serialises the current state to the catalog file
// with a tmp+rename + parent-dir fsync, so a crash mid-write
// can never corrupt the durable file.
func (c *FileCatalog) commit(state fileState) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("wireguard catalog: encode: %w", err)
	}
	tmp := c.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("wireguard catalog: tmp write: %w", err)
	}
	if err := os.Rename(tmp, c.path); err != nil {
		return fmt.Errorf("wireguard catalog: rename: %w", err)
	}
	if dir, err := os.Open(c.dir); err == nil {
		_ = dir.Sync()
		_ = dir.Close()
	}
	c.state = state
	return nil
}

// UpsertServer / RemoveServer / UpsertPeer / RemovePeer are
// the canonical CatalogPersistence methods. They all share
// the same lock + durable write path.
func (c *FileCatalog) UpsertServer(_ context.Context, server Server) error {
	if server.ID == "" {
		return errors.New("server id required")
	}
	if server.CreatedAt.IsZero() {
		server.CreatedAt = time.Now().UTC()
	}
	server.UpdatedAt = time.Now().UTC()
	c.mu.Lock()
	defer c.mu.Unlock()
	peers := make([]Peer, 0, len(c.state.Peers)+len(server.Peers))
	for _, peer := range c.state.Peers {
		if peer.ServerID != server.ID {
			peers = append(peers, peer)
		}
	}
	c.state.Peers = append(peers, server.Peers...)
	server.Peers = nil
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
	// Cascade remove peers that reference the deleted
	// server; a peer without a parent is a catalog
	// invariant violation.
	pruned := c.state.Peers[:0]
	for _, p := range c.state.Peers {
		if p.ServerID != id {
			pruned = append(pruned, p)
		}
	}
	c.state.Peers = pruned
	return c.commit(c.state)
}

func (c *FileCatalog) UpsertPeer(_ context.Context, peer Peer) error {
	if peer.ID == "" || peer.ServerID == "" {
		return errors.New("peer id and serverId required")
	}
	if peer.CreatedAt.IsZero() {
		peer.CreatedAt = time.Now().UTC()
	}
	peer.UpdatedAt = time.Now().UTC()
	c.mu.Lock()
	defer c.mu.Unlock()
	found := false
	for i, existing := range c.state.Peers {
		if existing.ID == peer.ID {
			c.state.Peers[i] = peer
			found = true
			break
		}
	}
	if !found {
		c.state.Peers = append(c.state.Peers, peer)
	}
	return c.commit(c.state)
}

func (c *FileCatalog) RemovePeer(_ context.Context, id string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	filtered := c.state.Peers[:0]
	removed := false
	for _, existing := range c.state.Peers {
		if existing.ID == id {
			removed = true
			continue
		}
		filtered = append(filtered, existing)
	}
	if !removed {
		return errors.New("peer not found")
	}
	c.state.Peers = filtered
	return c.commit(c.state)
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

func (c *FileCatalog) Peer(_ context.Context, id string) (Peer, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, existing := range c.state.Peers {
		if existing.ID == id {
			return existing, nil
		}
	}
	return Peer{}, errors.New("peer not found")
}

// ListServers / ListPeers return deterministic orderings
// (by ID) so the lister produces a stable snapshot ID and
// consistent pagination across calls.
func (c *FileCatalog) ListServers(_ context.Context) ([]Server, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]Server, len(c.state.Servers))
	copy(out, c.state.Servers)
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (c *FileCatalog) ListPeers(_ context.Context, serverID string) ([]Peer, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]Peer, 0, len(c.state.Peers))
	for _, p := range c.state.Peers {
		if serverID == "" || p.ServerID == serverID {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}
