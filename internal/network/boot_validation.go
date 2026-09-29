package network

import (
	"context"
	"errors"
)

// BootKernel is the narrow contract used by pre-networkd recovery. It captures
// only what the boot gate needs: live private route tables and live policy
// rules in the root-enrolled scope, plus the table scope itself. The real
// implementation drives /usr/sbin/ip through the existing bounded PrivateRoute
// and PolicyRule hosts; tests inject synthetic hosts that return prepared
// snapshots without touching a kernel. Boot validation never mutates state.
type BootKernel interface {
	EnrolledTables() []int
	CaptureRoutes(ctx context.Context) (RouteSnapshot, error)
	CaptureRules(ctx context.Context) (PolicySnapshot, error)
}

// RoutedBootKernel adapts a fully constructed RoutedKernel (with real
// Linux-backed hosts) to the boot kernel contract. The caller still owns the
// underlying kernel; pre-networkd recovery must not assume it has captured
// host state for a transaction.
type RoutedBootKernel struct {
	Kernel *RoutedKernel
}

func (r RoutedBootKernel) EnrolledTables() []int {
	return r.Kernel.tables
}

func (r RoutedBootKernel) CaptureRoutes(ctx context.Context) (RouteSnapshot, error) {
	return r.Kernel.routes.Capture(ctx)
}

func (r RoutedBootKernel) CaptureRules(ctx context.Context) (PolicySnapshot, error) {
	return r.Kernel.rules.Capture(ctx)
}

// validateBootOwnership is the no-pending root-enrollment gate. It must run
// before networkd is permitted to start. The gate catches four failure modes
// that the pre-networkd artifact stage cannot see on its own:
//
//  1. A previously captured transaction in an enrolled private route table that
//     no longer matches the durable "before" intent (drift).
//  2. An unknown route or policy rule in an enrolled table that was never
//     journaled (silent injection).
//  3. A revoked enrollment scope: any route or rule whose table is no longer
//     in the active root enrollment.
//  4. Missing routes or rules the durable "before" configuration requires.
//
// The gate never mutates state. It is paired with a kernel that has already
// refused unmanaged tables during Capture, so the only reachable error here
// is intent drift, missing intent or a host capture failure. Every refusal
// keeps the Agent out of the recovered networking state.
func validateBootOwnership(ctx context.Context, kernel BootKernel, before Config, enrollment Enrollment) error {
	if kernel == nil {
		return errors.New("boot ownership requires an enrolled kernel")
	}
	if err := enrollment.Validate(); err != nil {
		return err
	}
	if err := before.Validate(); err != nil {
		return err
	}
	enrolled := map[int]bool{}
	for _, table := range kernel.EnrolledTables() {
		if !privateRouteTable(table) || !sameIntSet(table, enrollment.RoutingTables) {
			return errors.New("kernel table scope diverges from root enrollment")
		}
		enrolled[table] = true
	}
	if !enrollment.PolicyRouting && len(before.PolicyRules) > 0 {
		return errors.New("policy routing is not root-enrolled")
	}
	liveRoutes, err := kernel.CaptureRoutes(ctx)
	if err != nil {
		return err
	}
	for _, r := range liveRoutes.Routes {
		if !enrolled[r.Table] {
			return errors.New("live route in revoked private table")
		}
	}
	liveRules, err := kernel.CaptureRules(ctx)
	if err != nil {
		return err
	}
	for _, r := range liveRules.Rules {
		if r.Table != 0 && r.Table != 254 && !enrolled[r.Table] {
			return errors.New("live policy rule in revoked private table")
		}
	}
	expectedRoutes, err := PrivateRoutesForConfig(before, kernel.EnrolledTables())
	if err != nil {
		return err
	}
	if !equalRoutes(liveRoutes, expectedRoutes) {
		return errors.New("private route state diverges from durable intent before networkd")
	}
	expectedRules, err := PolicySnapshotForConfig(before)
	if err != nil {
		return err
	}
	if !equalPolicy(liveRules, expectedRules) {
		return errors.New("policy rule state diverges from durable intent before networkd")
	}
	return nil
}

func sameIntSet(table int, tables []int) bool {
	for _, t := range tables {
		if t == table {
			return true
		}
	}
	return false
}

// validateBootScope is a cheap pre-flight check that the supplied kernel
// advertises exactly the same table scope as the root enrollment. It must
// run before any recovery evidence is decoded. A mismatch means the boot
// gate is being asked to honour a contract that the host no longer agrees
// with, so the boot must remain blocked. The function does not capture
// live state and never mutates anything.
func validateBootScope(kernel BootKernel, enrollment Enrollment) error {
	if kernel == nil {
		return errors.New("boot recovery requires an enrolled host kernel")
	}
	if err := enrollment.Validate(); err != nil {
		return err
	}
	want := map[int]bool{}
	for _, table := range enrollment.RoutingTables {
		want[table] = true
	}
	got := map[int]bool{}
	for _, table := range kernel.EnrolledTables() {
		got[table] = true
	}
	if len(want) != len(got) {
		return errors.New("kernel table scope diverges from root enrollment")
	}
	for table := range want {
		if !got[table] {
			return errors.New("kernel table scope is missing root-enrolled table")
		}
	}
	for table := range got {
		if !want[table] {
			return errors.New("kernel table scope contains a non-enrolled table")
		}
	}
	return nil
}

// ValidateRuntimeOwnership is the second stage of the cold-boot model. It
// runs after the pre-networkd oneshot has restored files and after networkd
// has started, but before the Agent admits any apply. The check is the
// networkd-aware counterpart to validateBootOwnership: the live state must
// match the journaled "before" intent, no foreign route or policy rule may
// have appeared in an enrolled private table, and the firewall's table
// membership must agree with the root enrollment. The function never
// mutates state. A non-nil error means the service must remain in
// recovery_required and refuse to admit any change request.
func ValidateRuntimeOwnership(ctx context.Context, kernel BootKernel, before Config, enrollment Enrollment) error {
	if err := validateBootScope(kernel, enrollment); err != nil {
		return err
	}
	if err := validateBootOwnership(ctx, kernel, before, enrollment); err != nil {
		return err
	}
	return nil
}
