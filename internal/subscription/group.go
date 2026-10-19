// Package subscription / group.go: subscription groups — an ordered,
// multi-node selection of servers with stable client-facing display
// names. A group is the operator-facing answer to "one subscription,
// many nodes, friendly names, defined order". Group members
// reference canonical objects by id; the display name is the only
// client-visible part.
package subscription

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"sync"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/persist"
)

var groupIDPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:-]{0,63}$`)
var displayNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 ._/()-]{0,63}$`)

// GroupMember is one server entry inside a group.
type GroupMember struct {
	NodeID   string `json:"nodeId,omitempty"`
	ServerID string `json:"serverId"`
	UserID   string `json:"userId"`
	// DisplayName is the client-facing label (geographic grouping
	// lives here: "Netherlands", "Germany", ...). Internal ids are
	// never exposed through delivery.
	DisplayName string `json:"displayName"`
	Enabled     bool   `json:"enabled"`
}

// Group is an ordered subscription group.
type Group struct {
	ID        string        `json:"id"`
	Name      string        `json:"name"`
	Format    Format        `json:"format"`
	Enabled   bool          `json:"enabled"`
	Members   []GroupMember `json:"members"`
	ExpiresAt string        `json:"expiresAt,omitempty"`
	Tags      []string      `json:"tags,omitempty"`
	CreatedAt time.Time     `json:"createdAt"`
	UpdatedAt time.Time     `json:"updatedAt"`
}

// Validate enforces the group contract: canonical ids, unique and
// canonical display names, at least one enabled member, supported
// format.
func (g Group) Validate() error {
	if !groupIDPattern.MatchString(g.ID) {
		return errors.New("subscription: group id is not canonical")
	}
	if g.Name == "" {
		return errors.New("subscription: group name is required")
	}
	if g.Format != "" && !Formats[g.Format] {
		return fmt.Errorf("subscription: group format %q is not supported", g.Format)
	}
	if len(g.Members) == 0 {
		return errors.New("subscription: group requires at least one member")
	}
	if len(g.Members) > 256 {
		return errors.New("subscription: group member limit exceeded")
	}
	enabled := 0
	names := map[string]bool{}
	for i, m := range g.Members {
		if m.ServerID == "" || m.UserID == "" {
			return fmt.Errorf("subscription: group member %d has no server or user", i)
		}
		if !displayNamePattern.MatchString(m.DisplayName) {
			return fmt.Errorf("subscription: group member %d display name is not canonical", i)
		}
		if names[m.DisplayName] {
			return fmt.Errorf("subscription: duplicate display name %q", m.DisplayName)
		}
		names[m.DisplayName] = true
		if m.Enabled {
			enabled++
		}
	}
	if enabled == 0 {
		return errors.New("subscription: group requires at least one enabled member")
	}
	if g.ExpiresAt != "" {
		if _, err := time.Parse(time.RFC3339, g.ExpiresAt); err != nil {
			return fmt.Errorf("subscription: group expiresAt must be RFC3339: %w", err)
		}
	}
	return nil
}

// IsExpired reports whether the group is past its expiry.
func (g Group) IsExpired(now time.Time) bool {
	if g.ExpiresAt == "" {
		return false
	}
	t, err := time.Parse(time.RFC3339, g.ExpiresAt)
	if err != nil {
		return false
	}
	return !now.Before(t)
}

// ActiveMembers returns the enabled members in declared order.
func (g Group) ActiveMembers() []GroupMember {
	out := make([]GroupMember, 0, len(g.Members))
	for _, m := range g.Members {
		if m.Enabled {
			out = append(out, m)
		}
	}
	return out
}

// GroupStore is a bounded group table with an optional
// persistence hook (state survives restart).
type GroupStore struct {
	mu      sync.RWMutex
	groups  map[string]Group
	clock   func() time.Time
	persist func([]Group) (persist.CommitOutcome, error)
	// degraded blocks mutations after an uncertain commit.
	degraded string
}

const MaxGroups = 256

func NewGroupStore() *GroupStore {
	return &GroupStore{groups: map[string]Group{}, clock: time.Now}
}

func (s *GroupStore) Upsert(g Group) (Group, error) {
	if err := g.Validate(); err != nil {
		return Group{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	// AUD-06: mutations are refused while storage is degraded and
	// the bounded capacity is enforced inside the same critical
	// section as the mutation.
	if s.degraded != "" {
		return Group{}, fmt.Errorf("subscription: storage degraded (%s); reconciliation required before mutations", s.degraded)
	}
	if _, exists := s.groups[g.ID]; !exists && len(s.groups) >= MaxGroups {
		return Group{}, fmt.Errorf("subscription: group capacity exceeded (%d)", MaxGroups)
	}
	now := s.clock().UTC()
	if prev, ok := s.groups[g.ID]; ok {
		g.CreatedAt = prev.CreatedAt
	} else {
		g.CreatedAt = now
	}
	g.UpdatedAt = now
	prev, existed := s.groups[g.ID]
	s.groups[g.ID] = g
	outcome, perr := s.persistLocked()
	if perr != nil {
		if outcome != persist.CommitUncertain {
			if existed {
				s.groups[g.ID] = prev
			} else {
				delete(s.groups, g.ID)
			}
		} else {
			s.degraded = fmt.Sprintf("commit_uncertain on group upsert: %v", perr)
		}
		return Group{}, fmt.Errorf("subscription: group not committed durably: %w", perr)
	}
	return g, nil
}

// SetPersist installs the durability hook with commit-phase
// reporting.
func (s *GroupStore) SetPersist(fn func([]Group) (persist.CommitOutcome, error)) {
	s.persist = fn
}

// Degraded reports the active commit-uncertainty diagnosis.
func (s *GroupStore) Degraded() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.degraded
}

// Reconcile adopts the authoritative disk snapshot and clears the
// degraded flag.
func (s *GroupStore) Reconcile(groups []Group) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.degraded == "" {
		return "healthy"
	}
	s.groups = map[string]Group{}
	for _, g := range groups {
		s.groups[g.ID] = g
	}
	s.degraded = ""
	return "recovered: disk state adopted"
}

// LoadGroups seeds the store from persisted state (startup).
func (s *GroupStore) LoadGroups(groups []Group) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, g := range groups {
		s.groups[g.ID] = g
	}
}

func (s *GroupStore) persistLocked() (persist.CommitOutcome, error) {
	if s.persist == nil {
		return persist.CommitOK, nil
	}
	out := make([]Group, 0, len(s.groups))
	for _, g := range s.groups {
		out = append(out, g)
	}
	return s.persist(out)
}

func (s *GroupStore) Get(id string) (Group, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	g, ok := s.groups[id]
	return g, ok
}

func (s *GroupStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.degraded != "" {
		return fmt.Errorf("subscription: storage degraded (%s); reconciliation required before mutations", s.degraded)
	}
	if _, ok := s.groups[id]; !ok {
		return errors.New("subscription: group not found")
	}
	prev, existed := s.groups[id]
	delete(s.groups, id)
	outcome, perr := s.persistLocked()
	if perr != nil {
		if outcome != persist.CommitUncertain {
			if existed {
				s.groups[id] = prev
			}
		} else {
			s.degraded = fmt.Sprintf("commit_uncertain on group delete: %v", perr)
		}
		return fmt.Errorf("subscription: group deletion not committed durably: %w", perr)
	}
	return nil
}

func (s *GroupStore) List() []Group {
	s.mu.RLock()
	out := make([]Group, 0, len(s.groups))
	for _, g := range s.groups {
		out = append(out, g)
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// GroupProxies resolves a group into delivery-ready proxies using
// the caller-supplied resolver (the same trust point as share-link
// building). Disabled and expired members are excluded.
func (s *GroupStore) GroupProxies(id string, now time.Time, resolve func(GroupMember) (Proxy, error)) ([]Proxy, error) {
	g, ok := s.Get(id)
	if !ok {
		return nil, errors.New("subscription: group not found")
	}
	if !g.Enabled {
		return nil, errors.New("subscription: group is disabled")
	}
	if g.IsExpired(now) {
		return nil, errors.New("subscription: group is expired")
	}
	var out []Proxy
	for _, m := range g.ActiveMembers() {
		p, err := resolve(m)
		if err != nil {
			return nil, fmt.Errorf("subscription: member %q: %w", m.DisplayName, err)
		}
		out = append(out, p)
	}
	if len(out) == 0 {
		return nil, errors.New("subscription: group has no usable members")
	}
	return out, nil
}
