// Package main / nodestate.go: AUD-04 — the durable migration
// boundary for the node-security catalog.
//
// The vulnerability: the legacy two-file catalog (nodes.json +
// enrollments.json) was re-migrated whenever the unified snapshot
// went missing, silently restoring pre-upgrade ACTIVE credentials —
// including nodes revoked after the migration.
//
// The boundary: a separate committed marker (xnode-migrated.json)
// records that migration COMPLETED. Startup rules:
//
//   - state snapshot present → use it (migration state irrelevant).
//   - state snapshot absent/empty AND marker present → the post-
//     migration state was LOST: fail closed, never fall back to
//     legacy records (the operator restores a backup explicitly).
//   - state snapshot absent/empty AND marker absent → genuine first
//     migration: validate legacy, commit the snapshot, then commit
//     the marker. An interrupted sequence (snapshot committed,
//     marker lost) is safe: the snapshot itself guards re-migration.
//
// Legacy files are never written and never auto-activated again.
package nodestate

import (
	"fmt"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/persist"
	"github.com/CrazyDuckLab/DuckOS/internal/xnode"
)

// NodeStateFile and NodeMigratedMarker are the durable file names
// of the atomic node-security snapshot and its migration boundary.
const (
	NodeStateFile      = "xnode-state.json"
	NodeMigratedMarker = "xnode-migrated.json"
)

type migrationMarker struct {
	MigratedAt time.Time `json:"migratedAt"`
	Revision   uint64    `json:"revision"`
	Nodes      int       `json:"nodes"`
}

// ensureNodeStateMigrated opens (or creates) the unified node state
// snapshot under dir, enforcing the migration boundary described
// above. The returned file is the single authoritative store; the
// legacy files, if any, are never written.
func EnsureMigrated(storeDir string) (*persist.File[xnode.DurableState], error) {
	stateFile, err := persist.Open[xnode.DurableState](storeDir, NodeStateFile)
	if err != nil {
		// A CORRUPT unified snapshot is never repaired from legacy
		// records: fail closed.
		return nil, fmt.Errorf("xnode: unified state snapshot unreadable; refusing legacy fallback: %w", err)
	}
	marker, merr := persist.Open[migrationMarker](storeDir, NodeMigratedMarker)
	if merr != nil {
		return nil, fmt.Errorf("xnode: migration marker unreadable; refusing legacy fallback: %w", merr)
	}
	st := stateFile.Get()
	migratedBefore := marker.Get().MigratedAt.After(time.Time{}) || marker.Get().Revision > 0
	if len(st.Nodes) > 0 || len(st.Enrollments) > 0 || st.Revision > 0 {
		// Snapshot is live. Ensure the marker exists (it may have
		// been lost while the snapshot survived — harmless, but the
		// boundary must be durable for future snapshot losses).
		if !migratedBefore {
			if out, cerr := marker.Update(func(cur *migrationMarker) {
				*cur = migrationMarker{MigratedAt: time.Now().UTC(), Revision: st.Revision, Nodes: len(st.Nodes)}
			}); cerr != nil || out == persist.CommitUncertain {
				return nil, fmt.Errorf("xnode: migration boundary commit uncertain (outcome=%s): %w", out, cerr)
			}
		}
		return stateFile, nil
	}
	if migratedBefore {
		// AUD-04: migration already happened; the unified snapshot
		// was lost afterwards. NEVER resurrect legacy credentials.
		return nil, fmt.Errorf("xnode: post-migration state snapshot is missing; legacy records are quarantined and will NOT be reactivated (restore a backup explicitly)")
	}
	// Genuine first migration.
	legacyNodes, lerr := persist.Open[[]xnode.Node](storeDir, "nodes.json")
	if lerr != nil {
		return nil, lerr
	}
	legacyEnroll, lerr := persist.Open[[]xnode.EnrollmentToken](storeDir, "enrollments.json")
	if lerr != nil {
		return nil, lerr
	}
	migrated, merr := xnode.MigrateLegacy(legacyNodes.Get(), legacyEnroll.Get())
	if merr != nil {
		return nil, fmt.Errorf("xnode: legacy catalog rejected: %w", merr)
	}
	if len(migrated.Nodes) == 0 && len(migrated.Enrollments) == 0 {
		// Fresh install: nothing to migrate; establish the boundary
		// so a future stray legacy file can never activate.
		if out, cerr := marker.Update(func(cur *migrationMarker) {
			*cur = migrationMarker{MigratedAt: time.Now().UTC(), Nodes: 0}
		}); cerr != nil || out == persist.CommitUncertain {
			return nil, fmt.Errorf("xnode: migration boundary commit uncertain (outcome=%s): %w", out, cerr)
		}
		return stateFile, nil
	}
	out, cerr := stateFile.Update(func(cur *xnode.DurableState) { *cur = migrated })
	if cerr != nil || out == persist.CommitUncertain {
		return nil, fmt.Errorf("xnode: migration not durably committed (outcome=%s): %w", out, cerr)
	}
	if out2, cerr2 := marker.Update(func(cur *migrationMarker) {
		*cur = migrationMarker{MigratedAt: time.Now().UTC(), Revision: migrated.Revision, Nodes: len(migrated.Nodes)}
	}); cerr2 != nil || out2 == persist.CommitUncertain {
		// The snapshot IS committed; the marker loss only means a
		// future loss of the snapshot could re-open the legacy path.
		// That must never happen silently: fail closed as well.
		return nil, fmt.Errorf("xnode: migration boundary not durably committed (outcome=%s): %w", out2, cerr2)
	}
	return stateFile, nil
}
