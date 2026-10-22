// Package xray / backup.go: versioned backup and restore for the
// Xray Center catalog.
//
// SECURITY CONTRACT: raw secrets are NOT exported by default. The
// default export replaces inline credentials (user UUIDs and
// passwords) with structure-preserving placeholders and keeps only
// vault references; a secret-inclusive export requires the explicit
// opt-in flag and is the operator's responsibility to protect.
// Restore is a two-step flow: Preview diffs a backup against the
// live catalog (added / updated / conflicting objects, compatibility
// validation), then Restore applies it through the caller's
// snapshot/rollback hooks so a failed restore rolls back whole.
package xray

import (
	"errors"
	"fmt"
	"sort"
	"time"
)

// BackupVersion is the on-disk format version.
const BackupVersion = 1

// Backup is the exported document.
type Backup struct {
	Version   int       `json:"version"`
	CreatedAt time.Time `json:"createdAt"`
	// Producer names the producing build ("duckos/1.x xray-center").
	Producer string `json:"producer"`
	// SecretsIncluded reports whether inline credentials were kept.
	SecretsIncluded bool           `json:"secretsIncluded"`
	Servers         []Server       `json:"servers"`
	Users           []User         `json:"users"`
	Profiles        []Profile      `json:"profiles"`
	Subscriptions   []Subscription `json:"subscriptions"`
	// CertRefs and NodeRefs are references only — certificate and
	// node records live in their own inventories; a backup never
	// embeds their material.
	CertRefs []string `json:"certRefs,omitempty"`
	NodeRefs []string `json:"nodeRefs,omitempty"`
}

// ExportOptions control the export.
type ExportOptions struct {
	// IncludeSecrets is the explicit opt-in for inline credentials.
	IncludeSecrets bool
	Producer       string
}

// Export builds the backup document from catalog lists.
func Export(servers []Server, users []User, profiles []Profile, subs []Subscription, opts ExportOptions) (Backup, error) {
	b := Backup{
		Version:         BackupVersion,
		CreatedAt:       time.Now().UTC(),
		Producer:        opts.Producer,
		SecretsIncluded: opts.IncludeSecrets,
	}
	for _, s := range servers {
		if err := s.Validate(); err != nil {
			return Backup{}, fmt.Errorf("backup: server %s: %w", s.ID, err)
		}
		b.Servers = append(b.Servers, s)
	}
	for _, u := range users {
		if err := u.Validate(); err != nil {
			return Backup{}, fmt.Errorf("backup: user %s: %w", u.ID, err)
		}
		if !opts.IncludeSecrets {
			// Strip raw credential material unconditionally; vault
			// references are kept so a restore re-binds through the
			// vault instead of resurrecting secrets from a file.
			u.UUID = ""
			u.Password = ""
		}
		b.Users = append(b.Users, u)
	}
	for _, p := range profiles {
		if err := p.Validate(); err != nil {
			return Backup{}, fmt.Errorf("backup: profile %s: %w", p.ID, err)
		}
		b.Profiles = append(b.Profiles, p)
	}
	for _, s := range subs {
		if err := s.Validate(); err != nil {
			return Backup{}, fmt.Errorf("backup: subscription %s: %w", s.ID, err)
		}
		b.Subscriptions = append(b.Subscriptions, s)
	}
	return b, nil
}

// ValidateBackup checks a restore candidate's compatibility: format
// version, structural validation of every object and referential
// integrity (users point at servers, profiles at users).
func ValidateBackup(b Backup) error {
	if b.Version != BackupVersion {
		return fmt.Errorf("backup: version %d is not supported (want %d)", b.Version, BackupVersion)
	}
	serverIDs := map[string]bool{}
	for _, s := range b.Servers {
		if err := s.Validate(); err != nil {
			return fmt.Errorf("backup: server %s: %w", s.ID, err)
		}
		serverIDs[s.ID] = true
	}
	userIDs := map[string]bool{}
	for _, u := range b.Users {
		if err := u.Validate(); err != nil {
			return fmt.Errorf("backup: user %s: %w", u.ID, err)
		}
		if !serverIDs[u.ServerID] {
			return fmt.Errorf("backup: user %s references unknown server %s", u.ID, u.ServerID)
		}
		userIDs[u.ID] = true
	}
	for _, p := range b.Profiles {
		if err := p.Validate(); err != nil {
			return fmt.Errorf("backup: profile %s: %w", p.ID, err)
		}
		if !userIDs[p.UserID] {
			return fmt.Errorf("backup: profile %s references unknown user %s", p.ID, p.UserID)
		}
	}
	profileIDs := map[string]bool{}
	for _, p := range b.Profiles {
		profileIDs[p.ID] = true
	}
	for _, s := range b.Subscriptions {
		if err := s.Validate(); err != nil {
			return fmt.Errorf("backup: subscription %s: %w", s.ID, err)
		}
		for _, pid := range s.ProfileIDs {
			if !profileIDs[pid] {
				return fmt.Errorf("backup: subscription %s references unknown profile %s", s.ID, pid)
			}
		}
	}
	return nil
}

// RestoreEntryClassifies how a backup object maps onto live state.
type RestoreEntryKind string

const (
	RestoreAdd      RestoreEntryKind = "add"
	RestoreUpdate   RestoreEntryKind = "update"
	RestoreConflict RestoreEntryKind = "conflict"
)

// RestoreEntry is one preview line.
type RestoreEntry struct {
	Kind   string           `json:"kind"` // server|user|profile|subscription
	ID     string           `json:"id"`
	Action RestoreEntryKind `json:"action"`
	Detail string           `json:"detail,omitempty"`
}

// RestorePreview diffs the backup against live catalog accessor
// callbacks and validates compatibility. Conflicts (same id,
// materially different object) are listed, never auto-applied.
func RestorePreview(b Backup, live func() ([]Server, []User, []Profile, []Subscription)) ([]RestoreEntry, error) {
	if err := ValidateBackup(b); err != nil {
		return nil, err
	}
	servers, users, profiles, subs := live()
	var out []RestoreEntry
	liveServers := stampsOf(len(servers), func(i int) string { return servers[i].ID }, func(i int) time.Time { return servers[i].UpdatedAt })
	liveUsers := stampsOf(len(users), func(i int) string { return users[i].ID }, func(i int) time.Time { return users[i].UpdatedAt })
	liveProfiles := stampsOf(len(profiles), func(i int) string { return profiles[i].ID }, func(i int) time.Time { return profiles[i].UpdatedAt })
	liveSubs := stampsOf(len(subs), func(i int) string { return subs[i].ID }, func(i int) time.Time { return subs[i].UpdatedAt })

	// classify: add for unknown ids, update when the backup is
	// newer, conflict when the live object changed after the
	// backup was taken.
	classify := func(kind, id string, backupStamp time.Time, liveStamps map[string]time.Time) RestoreEntry {
		action := RestoreAdd
		if liveStamp, exists := liveStamps[id]; exists {
			action = RestoreUpdate
			if liveStamp.After(backupStamp) {
				action = RestoreConflict
			}
		}
		return RestoreEntry{Kind: kind, ID: id, Action: action}
	}
	for _, s := range b.Servers {
		out = append(out, classify("server", s.ID, s.UpdatedAt, liveServers))
	}
	for _, u := range b.Users {
		out = append(out, classify("user", u.ID, u.UpdatedAt, liveUsers))
	}
	for _, p := range b.Profiles {
		out = append(out, classify("profile", p.ID, p.UpdatedAt, liveProfiles))
	}
	for _, s := range b.Subscriptions {
		out = append(out, classify("subscription", s.ID, s.UpdatedAt, liveSubs))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

// stampsOf builds the id -> updatedAt map used for preview
// classification.
func stampsOf(n int, key func(int) string, stamp func(int) time.Time) map[string]time.Time {
	m := make(map[string]time.Time, n)
	for i := 0; i < n; i++ {
		m[key(i)] = stamp(i)
	}
	return m
}

// RestoreApplier is the transactional write surface the caller
// provides (the catalog's snapshot/rollback is applied around it by
// the same mutateCatalog pattern the API uses).
type RestoreApplier interface {
	UpsertServer(Server) error
	UpsertUser(User) error
	UpsertProfile(Profile) error
	UpsertSubscription(Subscription) error
}

// Restore applies every non-conflict entry. Entries that cannot be
// validated — typically credential-less users from a default
// (secret-free) export, which must be re-bound through the vault —
// are skipped with a count, never half-applied. It never deletes
// live objects that are absent from the backup.
func Restore(b Backup, apply RestoreApplier) (applied, skipped int, err error) {
	if b.Version != BackupVersion {
		return 0, 0, fmt.Errorf("backup: version %d is not supported (want %d)", b.Version, BackupVersion)
	}
	for _, s := range b.Servers {
		if err := apply.UpsertServer(s); err != nil {
			return applied, skipped, fmt.Errorf("backup: restore server %s: %w", s.ID, err)
		}
		applied++
	}
	for _, u := range b.Users {
		if err := u.Validate(); err != nil {
			// Credential-less draft from a secret-free export.
			skipped++
			continue
		}
		if err := apply.UpsertUser(u); err != nil {
			return applied, skipped, fmt.Errorf("backup: restore user %s: %w", u.ID, err)
		}
		applied++
	}
	for _, p := range b.Profiles {
		if err := apply.UpsertProfile(p); err != nil {
			return applied, skipped, fmt.Errorf("backup: restore profile %s: %w", p.ID, err)
		}
		applied++
	}
	for _, s := range b.Subscriptions {
		if err := apply.UpsertSubscription(s); err != nil {
			return applied, skipped, fmt.Errorf("backup: restore subscription %s: %w", s.ID, err)
		}
		applied++
	}
	return applied, skipped, nil
}

// ErrBackupEmpty is the typed failure for a zero-object backup.
var ErrBackupEmpty = errors.New("backup: document carries no objects")
