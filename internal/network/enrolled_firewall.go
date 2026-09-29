package network

import (
	"context"
	"encoding/json"
)

// EnrolledFirewallAdapter adds actual root enrollment and live networkd/link
// preconditions to the scoped firewall transaction adapter. It still does not
// replace network-wide traffic/reachability verification or the composite
// adapter needed for changes to interface, route, DHCP and DNS intent.
type EnrolledFirewallAdapter struct {
	inner             *FirewallAdapter
	enrollment        Enrollment
	captureHost       func(context.Context, Config, Config) (json.RawMessage, error)
	networkdInventory func(context.Context) ([]NetworkdLink, error)
}

func NewEnrolledFirewallAdapter(privateAgentDirectory string) (*EnrolledFirewallAdapter, error) {
	inner, err := NewFirewallAdapter()
	if err != nil {
		return nil, err
	}
	enrollment, err := ReadEnrollment(privateAgentDirectory)
	if err != nil {
		return nil, err
	}
	return &EnrolledFirewallAdapter{inner: inner, enrollment: enrollment, captureHost: CaptureHost, networkdInventory: ReadNetworkdInventory}, nil
}

func (a *EnrolledFirewallAdapter) check(ctx context.Context, before, next json.RawMessage) error {
	b, n, err := firewallPair(before, next)
	if err != nil {
		return err
	}
	raw, err := a.captureHost(ctx, b, n)
	if err != nil {
		return err
	}
	live, err := DecodeHostSnapshot(raw, b, n)
	if err != nil {
		return err
	}
	if err := a.enrollment.CheckBindings(b, n, live); err != nil {
		return err
	}
	inventory, err := a.networkdInventory(ctx)
	if err != nil {
		return err
	}
	return CheckNetworkdOwnership(b, n, inventory)
}

func (a *EnrolledFirewallAdapter) Preflight(ctx context.Context, before, next json.RawMessage) error {
	if err := a.check(ctx, before, next); err != nil {
		return err
	}
	return a.inner.Preflight(ctx, before, next)
}

func (a *EnrolledFirewallAdapter) Capture(ctx context.Context, before, next json.RawMessage) (json.RawMessage, error) {
	if err := a.check(ctx, before, next); err != nil {
		return nil, err
	}
	return a.inner.Capture(ctx, before, next)
}

func (a *EnrolledFirewallAdapter) Apply(ctx context.Context, before, next json.RawMessage) error {
	if err := a.check(ctx, before, next); err != nil {
		return err
	}
	return a.inner.Apply(ctx, before, next)
}

func (a *EnrolledFirewallAdapter) Verify(ctx context.Context, config json.RawMessage) error {
	if err := a.check(ctx, config, config); err != nil {
		return err
	}
	return a.inner.Verify(ctx, config)
}

func (a *EnrolledFirewallAdapter) Restore(ctx context.Context, before, next json.RawMessage) error {
	return a.inner.Restore(ctx, before, next)
}

func (a *EnrolledFirewallAdapter) RestoreCaptured(ctx context.Context, before, next, evidence json.RawMessage) error {
	// Restoring the exclusively owned table must still be attempted if a NIC
	// disappeared or networkd failed. Subsequent Verify reports those failures
	// and keeps the controller in recovery-required rather than claiming health.
	return a.inner.RestoreCaptured(ctx, before, next, evidence)
}
