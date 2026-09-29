package network

import (
	"context"
	"errors"
	"io"
	"strings"

	"github.com/CrazyDuckLab/DuckOS/internal/state"
)

// RecoverRoutingArtifacts is the pre-networkd boot phase. It never executes
// network commands, confirms a candidate or clears its durable pending record.
// The normal Agent must later restore and verify kernel/daemon state before
// recording rollback. The caller must hold an existing, exclusively locked
// private journal; missing state must not be replaced by an empty baseline.
//
// The kernel parameter supplies a boot-only ownership gate for the no-pending
// branch. It is also reused to validate the journaled recovery evidence in
// the pending branch. A nil kernel is treated as an explicit refusal: the
// caller cannot start networkd without an enrolled host ownership check.
func RecoverRoutingArtifacts(ctx context.Context, store *state.Store, files *ArtifactDirectory, enrollment Enrollment, kernel BootKernel) error {
	if store == nil || files == nil {
		return errors.New("boot recovery requires existing journal and artifact directory")
	}
	if kernel == nil {
		return errors.New("boot recovery requires an enrolled host kernel")
	}
	if err := enrollment.Validate(); err != nil {
		return err
	}
	if !enrollment.NetworkdReload || !enrollment.PolicyRouting {
		return errors.New("boot routing recovery authority is absent")
	}
	snapshot, err := store.Snapshot()
	if err != nil {
		return err
	}
	before, err := Decode(snapshot.Current.Config)
	if err != nil {
		return err
	}
	pb, err := Render(before)
	if err != nil {
		return err
	}
	scope := &NetworkdReloadHost{enrollment: enrollment}
	if err := scope.checkNames(before); err != nil {
		return err
	}
	if snapshot.Pending == nil {
		// The no-pending branch must verify that the current host state still
		// matches the durable intent. Without this gate a foreign route in an
		// enrolled table, a revoked table, or a missing route would only be
		// detected when the next apply attempted to mutate it.
		if err := validateBootOwnership(ctx, kernel, before, enrollment); err != nil {
			return err
		}
		if err := checkBootArtifactInventory(files, pb.Networkd, nil, false); err != nil {
			return err
		}
		return files.Check(pb.Networkd, pb.Networkd)
	}
	// The pending branch must still reject a kernel whose scope no longer
	// matches the root enrollment. Otherwise a revoked table could carry a
	// partial transaction that the recovery evidence alone cannot detect.
	if err := validateBootScope(kernel, enrollment); err != nil {
		return err
	}
	// Scope-only hosts validate journal contents without executing Linux tools.
	names, tables := map[string]bool{}, map[int]bool{}
	for _, b := range enrollment.Physical {
		names[b.Name] = true
	}
	for _, name := range enrollment.VLANNames {
		names[name] = true
	}
	for _, table := range enrollment.RoutingTables {
		tables[table] = true
	}
	managed := &RoutedKernel{tables: enrollment.RoutingTables,
		routes: &PrivateRouteHost{tables: enrollment.RoutingTables, names: names},
		rules:  &PolicyRuleHost{tables: tables}}
	adapter := &ManagedRoutingAdapter{kernel: managed}
	e, _, candidate, err := adapter.decode(snapshot.Current.Config, snapshot.Pending.Candidate.Config, snapshot.Pending.Recovery)
	if err != nil {
		return err
	}
	if _, err := managed.decode(candidate, e.Kernel); err != nil {
		return err
	}
	// Validate logical bindings too, not only interface names in route snapshots.
	if err := scope.checkNames(candidate); err != nil {
		return err
	}
	pn, err := Render(candidate)
	if err != nil {
		return err
	}
	if err := rejectExternallyManagedMutation(enrollment, before, candidate); err != nil {
		return err
	}
	if err := checkBootArtifactInventory(files, pb.Networkd, pn.Networkd, true); err != nil {
		return err
	}
	// Startup undo must use the same write scope as the live transaction.
	// Legacy journals may still describe an unchanged external management NIC.
	scoped := unionInterfaceIDs(changedInterfaceIDs(before, candidate), renderedBodyChanged(pb.Networkd, pn.Networkd))
	if err := files.Restore(filterNetworkdByIDs(pb.Networkd, scoped), filterNetworkdByIDs(pn.Networkd, scoped)); err != nil {
		return err
	}
	if err := checkBootArtifactInventory(files, pb.Networkd, nil, false); err != nil {
		return err
	}
	return files.Check(pb.Networkd, pb.Networkd)
}

func checkBootArtifactInventory(files *ArtifactDirectory, before, candidate map[string]string, staging bool) error {
	dir, err := files.root.Open(".")
	if err != nil {
		return err
	}
	defer dir.Close()
	entries, err := dir.ReadDir(4097)
	if err != nil && !errors.Is(err, io.EOF) || len(entries) > 4096 {
		return errors.New("boot artifact inventory unavailable or exceeds limit")
	}
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, "10-duckos-") {
			continue
		}
		if staging {
			name = strings.TrimSuffix(name, ".duckos-tmp")
		}
		_, old := before[name]
		_, next := candidate[name]
		if !old && !next {
			return errors.New("unrecognized managed artifact blocks network startup")
		}
	}
	return nil
}
