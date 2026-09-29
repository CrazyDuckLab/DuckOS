// backup/restore.go: V1 logical backup
// surface. The package complements the
// existing file-envelope layer in
// backup.go with the high-level concerns
// required by R12.AH:
//
//   - two backup modes (configuration-only
//     and full-protected)
//   - hardware remap on different-host
//     restore
//   - persistent logical interface identity
//   - schema versioning for the logical
//     backup model
//   - restore preview / diff / selective
//     restore
//
// The runtime is a thin mapping on top of
// the existing envelope.
package backup

import (
	"errors"
	"regexp"
	"sort"
	"strconv"
	"sync"
)

// BackupMode is the logical backup kind.
type BackupMode string

const (
	// BackupModeConfigOnly excludes every
	// vault secret. Safe to share; cannot be
	// used to recover protected services.
	BackupModeConfigOnly BackupMode = "config-only"
	// BackupModeFullProtected embeds the
	// encrypted secret material. Restoring
	// requires the matching passphrase.
	BackupModeFullProtected BackupMode = "full-protected"
)

// Valid reports whether m is known.
func (m BackupMode) Valid() bool {
	return m == BackupModeConfigOnly || m == BackupModeFullProtected
}

// CurrentSchemaVersion is the current
// logical-backup schema version. The
// Migrators registry uses this value as the
// target.
const CurrentSchemaVersion = 2

// InterfaceBinding is a logical interface
// identity. The persistent logical name is
// what the configuration refers to; the
// physical binding maps the logical name to
// the current host's interface.
type InterfaceBinding struct {
	Logical  string `json:"logical"`
	Physical string `json:"physical"`
	MAC      string `json:"mac,omitempty"`
	Kind     string `json:"kind"` // "ethernet" | "wifi" | "vlan" | "bridge" | "bond" | "wwan" | "loopback" | "tunnel"
}

// Valid reports whether i is well-formed.
func (i InterfaceBinding) Valid() error {
	if i.Logical == "" {
		return errors.New("backup: logical name is required")
	}
	if i.Physical == "" {
		return errors.New("backup: physical binding is required")
	}
	if i.Kind == "" {
		return errors.New("backup: interface kind is required")
	}
	return nil
}

// LogicalManifest is the logical backup
// metadata. It rides alongside the existing
// file envelope and is the input to
// restore-preview.
type LogicalManifest struct {
	BackupID        string             `json:"backupId"`
	CreatedAt       string             `json:"createdAt"`
	DuckOSVersion   string             `json:"duckosVersion"`
	SchemaVersion   int                `json:"schemaVersion"`
	Mode            BackupMode         `json:"mode"`
	Interfaces      []InterfaceBinding `json:"interfaces"`
	IncludesSecrets bool               `json:"includesSecrets"`
	Notes           string             `json:"notes,omitempty"`
	Diagnostics     []string           `json:"diagnostics,omitempty"`
}

var logicalIDPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:-]{0,63}$`)

// Valid reports whether m is well-formed.
func (m LogicalManifest) Valid() error {
	if !logicalIDPattern.MatchString(m.BackupID) {
		return errors.New("backup: backupId is not canonical")
	}
	if !m.Mode.Valid() {
		return errors.New("backup: mode is invalid")
	}
	if m.SchemaVersion <= 0 {
		return errors.New("backup: schemaVersion must be positive")
	}
	if m.IncludesSecrets && m.Mode != BackupModeFullProtected {
		return errors.New("backup: includesSecrets requires full-protected mode")
	}
	for _, b := range m.Interfaces {
		if err := b.Valid(); err != nil {
			return err
		}
	}
	return nil
}

// RemapRequest is the operator's hardware
// remap input on a different-host restore.
// Each entry maps a logical interface from
// the source manifest to a physical
// interface on the target host.
type RemapRequest struct {
	Logical  string `json:"logical"`
	Physical string `json:"physical"`
	MAC      string `json:"mac,omitempty"`
}

// Valid reports whether r is well-formed.
func (r RemapRequest) Valid() error {
	if r.Logical == "" {
		return errors.New("backup: logical name is required")
	}
	if r.Physical == "" {
		return errors.New("backup: physical binding is required")
	}
	return nil
}

// Remap applies the operator's remap to the
// manifest. It returns the remapped
// manifest and the list of unmapped logical
// interfaces (which the operator must
// resolve before the restore is allowed).
//
// Invalid requests (failing Valid) are
// silently dropped and surfaced as
// "malformed" entries in the diagnostics
// list so the operator can fix them. The
// caller is expected to surface the
// diagnostics before accepting the plan.
func Remap(in LogicalManifest, reqs []RemapRequest) (LogicalManifest, []string) {
	out := in
	// A preview must not change the source backup or another pending preview.
	out.Interfaces = append([]InterfaceBinding(nil), in.Interfaces...)
	out.Diagnostics = append([]string(nil), in.Diagnostics...)
	idx := map[string]RemapRequest{}
	for _, r := range reqs {
		if err := r.Valid(); err != nil {
			out.Diagnostics = append(out.Diagnostics, "malformed-remap:"+err.Error())
			continue
		}
		idx[r.Logical] = r
	}
	seen := map[string]bool{}
	for i := range out.Interfaces {
		if r, ok := idx[out.Interfaces[i].Logical]; ok {
			out.Interfaces[i].Physical = r.Physical
			if r.MAC != "" {
				out.Interfaces[i].MAC = r.MAC
			}
			seen[out.Interfaces[i].Logical] = true
		}
	}
	missing := []string{}
	for _, b := range out.Interfaces {
		if !seen[b.Logical] {
			missing = append(missing, b.Logical)
		}
	}
	sort.Strings(missing)
	return out, missing
}

// RestorePlan is the preview/diff of a
// restore. The plan lists every object
// that will be added, modified, or removed,
// and the dependencies the restore will
// touch.
type RestorePlan struct {
	BackupID       string   `json:"backupId"`
	Adds           []string `json:"adds"`
	Modifies       []string `json:"modifies"`
	Removes        []string `json:"removes"`
	Impact         []string `json:"impact"`
	RequiresSecret bool     `json:"requiresSecret"`
	SafeToApply    bool     `json:"safeToApply"`
}

// PlanRestore compares the backup's
// configuration object index against the
// current configuration and returns a
// RestorePlan. backupKeys is the set of
// object keys the backup carries; liveKeys
// is the set the live system has. The diff
// is computed by key.
//
// SafeToApply is true only when the plan
// requires no destructive operation
// (Removes), no live overwrite
// (Modifies) and no protected-secret
// (RequiresSecret). Any conflict, removal
// or required secret downgrades the plan
// to a dry-run preview.
func PlanRestore(backupID string, requiresSecret bool, backupKeys map[string]bool, liveKeys map[string]bool) RestorePlan {
	plan := RestorePlan{BackupID: backupID, SafeToApply: true, RequiresSecret: requiresSecret}
	for k := range backupKeys {
		if _, ok := liveKeys[k]; !ok {
			plan.Adds = append(plan.Adds, k)
		} else {
			plan.Modifies = append(plan.Modifies, k)
		}
	}
	for k := range liveKeys {
		if _, ok := backupKeys[k]; !ok {
			plan.Removes = append(plan.Removes, k)
		}
	}
	sort.Strings(plan.Adds)
	sort.Strings(plan.Modifies)
	sort.Strings(plan.Removes)
	if len(plan.Removes) > 0 {
		plan.Impact = append(plan.Impact, "removes-referenced-objects")
	}
	if len(plan.Modifies) > 0 {
		plan.Impact = append(plan.Impact, "overwrites-live-objects")
	}
	if requiresSecret {
		plan.Impact = append(plan.Impact, "requires-secret")
	}
	// Downgrade: any conflict, removal or secret
	// requirement turns the plan into a preview.
	if len(plan.Modifies) > 0 || len(plan.Removes) > 0 || requiresSecret {
		plan.SafeToApply = false
	}
	return plan
}

// MigratorFunc is the cross-version
// restore contract. A registered migrator
// upgrades an older object-key set to a
// newer schema.
type MigratorFunc func(in map[string]bool) (map[string]bool, error)

// Migrators is the typed registry of schema
// upgrades. It is goroutine-safe.
type Migrators struct {
	mu    sync.Mutex
	table map[int]MigratorFunc
}

// NewMigrators returns an empty registry.
func NewMigrators() *Migrators { return &Migrators{table: map[int]MigratorFunc{}} }

// Register adds a migrator for the supplied
// source schema. The migrator must produce
// an object-key set at source+1.
func (m *Migrators) Register(fromSchema int, fn MigratorFunc) error {
	if fn == nil {
		return errors.New("backup: migrator is nil")
	}
	if fromSchema <= 0 {
		return errors.New("backup: source schema must be positive")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.table[fromSchema]; ok {
		return errors.New("backup: migrator already registered")
	}
	m.table[fromSchema] = fn
	return nil
}

// Apply chains the registered migrators to
// upgrade fromSchema -> CurrentSchemaVersion.
// A future-schema archive is rejected.
func (m *Migrators) Apply(in map[string]bool, fromSchema int) (map[string]bool, error) {
	if fromSchema > CurrentSchemaVersion {
		return in, errors.New("backup: archive schema is newer than the running agent")
	}
	if fromSchema == CurrentSchemaVersion {
		return in, nil
	}
	if fromSchema < 1 {
		return in, errors.New("backup: archive schema is invalid")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	cur := in
	cursor := fromSchema
	for cursor < CurrentSchemaVersion {
		fn, ok := m.table[cursor]
		if !ok {
			return cur, errors.New("backup: no migrator from schema " + strconv.Itoa(cursor))
		}
		next, err := fn(cur)
		if err != nil {
			return cur, err
		}
		cur = next
		cursor++
	}
	return cur, nil
}
