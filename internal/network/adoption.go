package network

import (
	"errors"
	"fmt"
)

// LiveLinkAdopter reports the bounded, host-attested identity of one
// physical link currently in the kernel. The adoption boundary consults
// it ONLY to decide whether a desired-interface ID that does not appear
// in the durable "before" config can be classified as an adoption of an
// already-existing enrolled link (allowed) versus an attempt to create a
// kernel link on demand (refused).
//
// The kernel is never mutated by the adoption check itself: it is a
// read-only inspection of the link's name and hardware address. Links
// are never created, deleted, renamed or cycled by this path. The
// renderer, scoped reconfigure and managed-routing host own any actual
// write — and only after the adoption check has cleared.
type LiveLinkAdopter interface {
	// LookupLink returns the live kernel identity for the interface
	// whose name matches the supplied argument, or (zero, false) when
	// the kernel does not expose a link at that name. The interface
	// reports only what an adoption needs: a stable hardware address
	// (or the absence of one) and a presence flag.
	LookupLink(name string) (LiveLinkIdentity, bool)
}

// LiveLinkIdentity is the slice of kernel-observed state the adoption
// boundary inspects. It exposes just enough to anchor an adoption audit:
// the link's hardware address (the enrollment binds by MAC) and a flag
// saying whether the link carries a stable, reproducible MAC at all.
// Everything else — link flags, MTU, addresses, VLANs — is the managed
// routing host's concern, not the adoption's.
type LiveLinkIdentity struct {
	MAC    string
	HasMAC bool
}

// enforceAdoptionBoundary classifies each desired interface whose ID
// does not appear in the durable "before" config as either an adoption
// of an already-existing enrolled link (allowed) or an attempted kernel
// creation (refused).
//
// The boundary is the smallest correct extension of enforceLinkShape-
// Equality and is intentionally separate from it:
//
//   - enforceLinkShapeEquality keeps the strict invariant that an
//     existing link cannot be removed, renamed, re-kinded, re-roled or
//     have its management flag flipped. Those refusals protect the
//     kernel's stable contract with operators and are unchanged.
//   - enforceAdoptionBoundary allows "I want to manage eth1 for the
//     first time" only when the durable config is empty for that ID AND
//     the operator-visible enrollment and a host-attested live link
//     agree on every observable attribute of the binding.
//
// Acceptance rule (all must hold):
//
//  1. The desired interface has both ID and Name.
//  2. The enrollment exposes a PhysicalBinding with the same ID and
//     same Name (binding.Name == next.Name) AND ExternallyManaged=false.
//  3. The enrollment binding carries a non-empty hardware address.
//  4. The live kernel exposes a link at exactly that Name AND that
//     link carries a stable hardware address.
//  5. The live hardware address matches the enrolled hardware address
//     (exact compare, hex case-insensitive). The enrolled binding is
//     the source of truth for what the operator audited.
//
// Failures:
//   - missing enrollment: refuse as "managed routing cannot add a link".
//   - externally-managed enrollment: refuse as "managed routing cannot
//     add a link ... binding is externally managed". Operators must
//     migrate that binding out of the external manager first.
//   - name/id mismatch: refuse as identity mismatch.
//   - missing/non-stable live link: refuse as the adoption requires an
//     already-present link.
//   - MAC mismatch: refuse as identity mismatch.
//
// The function never mutates state: no netlink messages, no file
// writes, no journal updates.
func enforceAdoptionBoundary(enrollment Enrollment, live LiveLinkAdopter, b, n Config) error {
	beforeByID := map[string]Interface{}
	for _, link := range b.Interfaces {
		beforeByID[link.ID] = link
	}
	// Collect the IDs that appear in next but not in before. The
	// boundary is a structural no-op when the link set is unchanged,
	// so existing captures whose candidate only mutates an already-
	// tracked link pay no enrollment/snapshotter cost. Adjudication
	// only fires when there is at least one interface whose adoption
	// status is unsettled.
	var added []Interface
	for _, next := range n.Interfaces {
		if _, ok := beforeByID[next.ID]; ok {
			continue
		}
		added = append(added, next)
	}
	if len(added) == 0 {
		return nil
	}
	if err := enrollment.Validate(); err != nil {
		return fmt.Errorf("managed routing cannot adopt a link: enrollment invalid: %w", err)
	}
	if live == nil {
		return errors.New("managed routing cannot adopt a link: live link snapshotter is unavailable")
	}
	for _, next := range added {
		if err := admitAdoptionOne(enrollment, live, next); err != nil {
			return err
		}
	}
	return nil
}

// admitAdoptionOne evaluates one desired Interface against the
// adoption boundary. It returns nil only when the interface is fully
// adoption-eligible; otherwise it returns an error whose message names
// the offending link and the precise refusal reason. The function is
// pure: it does not touch the live snapshotter or the durable store.
func admitAdoptionOne(enrollment Enrollment, live LiveLinkAdopter, next Interface) error {
	if next.ID == "" || next.Name == "" {
		return fmt.Errorf("managed routing cannot add a link with no id or name")
	}
	var binding *PhysicalBinding
	for i := range enrollment.Physical {
		if enrollment.Physical[i].ID == next.ID {
			e := enrollment.Physical[i]
			binding = &e
			break
		}
	}
	if binding == nil {
		return fmt.Errorf("managed routing cannot add a link %q: no enrolled physical binding matches that id", next.ID)
	}
	if binding.ExternallyManaged {
		return fmt.Errorf("managed routing cannot add a link %q: enrolled binding is externally managed", next.ID)
	}
	if binding.Name != next.Name {
		return fmt.Errorf("managed routing cannot add a link %q: desired name %q does not match enrolled binding name %q", next.ID, next.Name, binding.Name)
	}
	if binding.MAC == "" {
		return fmt.Errorf("managed routing cannot add a link %q: enrolled binding carries no hardware address", next.ID)
	}
	obs, ok := live.LookupLink(next.Name)
	if !ok {
		return fmt.Errorf("managed routing cannot add a link %q: live kernel has no link with that name", next.ID)
	}
	if !obs.HasMAC || obs.MAC == "" {
		return fmt.Errorf("managed routing cannot add a link %q: live kernel link has no stable hardware address", next.ID)
	}
	if !sameMAC(obs.MAC, binding.MAC) {
		return fmt.Errorf("managed routing cannot add a link %q: live kernel MAC %s does not match enrolled MAC %s", next.ID, obs.MAC, binding.MAC)
	}
	return nil
}

// sameMAC compares two MAC strings byte-by-byte, case-insensitive on
// hex digits. Empty strings are never equal.
func sameMAC(a, b string) bool {
	if len(a) == 0 || len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ai, bi := a[i], b[i]
		if ai >= 'A' && ai <= 'F' {
			ai += 'a' - 'A'
		}
		if bi >= 'A' && bi <= 'F' {
			bi += 'a' - 'A'
		}
		if ai != bi {
			return false
		}
	}
	return true
}
