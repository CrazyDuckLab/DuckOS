// Package xnode / migrate.go: safe migration from the legacy
// two-file node catalog (nodes.json + enrollments.json) to the
// single atomic DurableState snapshot.
//
// Migration rules (UQ-02 §7):
//   - Every record is validated; inconsistent input aborts the
//     migration with a typed error (no partial activation).
//   - Node identities, credential hashes, revocation/suspension
//     states and enrollment-token consumption marks are preserved
//     verbatim — migration never activates a revoked credential.
//   - The caller writes the produced snapshot through the commit
//     protocol and only then switches to the new layout; the legacy
//     files stay on disk as rollback evidence.
package xnode

import (
	"errors"
	"fmt"
)

// MigrateLegacy validates and converts the legacy records into a
// DurableState. It is a pure function: no I/O, no side effects, so
// the caller controls the transactional commit.
func MigrateLegacy(nodes []Node, enrollments []EnrollmentToken) (DurableState, error) {
	st := DurableState{}
	seenNode := map[string]bool{}
	localNodes := 0
	for i, n := range nodes {
		switch {
		case n.ID == "":
			return DurableState{}, fmt.Errorf("migrate: node %d has no id", i)
		case seenNode[n.ID]:
			return DurableState{}, fmt.Errorf("migrate: duplicate node id %s", n.ID)
		case n.Kind != KindLocal && n.Kind != KindRemote:
			return DurableState{}, fmt.Errorf("migrate: node %s has unknown kind %q", n.ID, n.Kind)
		case n.Status != StatusPending && n.Status != StatusActive && n.Status != StatusSuspended &&
			n.Status != StatusStale && n.Status != StatusRevoked:
			return DurableState{}, fmt.Errorf("migrate: node %s has unknown status %q", n.ID, n.Status)
		case n.Kind == KindRemote && n.CredentialHash == "":
			return DurableState{}, fmt.Errorf("migrate: remote node %s has no credential hash", n.ID)
		case n.Status == StatusRevoked && n.CredentialHash == "" && n.Kind == KindRemote:
			return DurableState{}, fmt.Errorf("migrate: revoked node %s lost its credential hash", n.ID)
		}
		if n.Kind == KindLocal {
			localNodes++
			if localNodes > 1 {
				return DurableState{}, errors.New("migrate: more than one local node")
			}
		}
		seenNode[n.ID] = true
		st.Nodes = append(st.Nodes, n)
	}
	seenToken := map[string]bool{}
	for i, e := range enrollments {
		switch {
		case e.TokenHash == "":
			return DurableState{}, fmt.Errorf("migrate: enrollment token %d has no digest", i)
		case seenToken[e.TokenHash]:
			return DurableState{}, fmt.Errorf("migrate: duplicate enrollment token digest")
		case e.Used && e.UsedBy == "":
			// A consumed token must point at its node; consumption
			// without a target is inconsistent.
			return DurableState{}, fmt.Errorf("migrate: consumed enrollment token %d has no consumer", i)
		case e.Used && e.UsedBy != "pending" && !seenNode[e.UsedBy]:
			return DurableState{}, fmt.Errorf("migrate: enrollment token %d references unknown node %s", i, e.UsedBy)
		}
		seenToken[e.TokenHash] = true
		st.Enrollments = append(st.Enrollments, e)
	}
	return st, nil
}
