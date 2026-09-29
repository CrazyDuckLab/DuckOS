package network

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"sync"
	"time"
)

type guardedFirewall interface {
	firewallKernel
	InstallForwardGuard(context.Context, NFTSnapshot) (NFTSnapshot, error)
	ApplyGuarded(context.Context, Config, NFTSnapshot) (NFTSnapshot, error)
	ReleaseForwardGuard(context.Context) error
}
type privateRouteKernel interface {
	checkScope(RouteSnapshot) error
	Capture(context.Context) (RouteSnapshot, error)
	Verify(context.Context, RouteSnapshot) error
	Apply(context.Context, RouteSnapshot, RouteSnapshot) error
	Restore(context.Context, RouteSnapshot, RouteSnapshot) error
}
type policyRuleKernel interface {
	checkScope(PolicySnapshot) error
	Capture(context.Context) (PolicySnapshot, error)
	Verify(context.Context, PolicySnapshot) error
	Apply(context.Context, PolicySnapshot, PolicySnapshot) error
	Restore(context.Context, PolicySnapshot, PolicySnapshot) error
}

// RoutedKernel coordinates the kernel portion of a network transaction. The
// outer Agent coordinator owns durable journaling, files, daemon lifecycle,
// enrollment binding checks, management verification and exclusive execution.
// Apply deliberately leaves forwarding blocked until those gates complete.
type RoutedKernel struct {
	mu       sync.Mutex
	firewall guardedFirewall
	routes   privateRouteKernel
	rules    policyRuleKernel
	tables   []int
	admitted string
}

type routedRecovery struct {
	Version   int             `json:"version"`
	Candidate string          `json:"candidate"`
	Firewall  *NFTSnapshot    `json:"firewall"`
	Routes    *RouteSnapshot  `json:"routes"`
	Rules     *PolicySnapshot `json:"rules"`
}

func NewRoutedKernel(e Enrollment) (*RoutedKernel, error) {
	f, err := NewFirewallHost()
	if err != nil {
		return nil, err
	}
	r, err := NewPrivateRouteHost(e)
	if err != nil {
		return nil, err
	}
	p, err := NewPolicyRuleHost(e)
	if err != nil {
		return nil, err
	}
	return &RoutedKernel{firewall: f, routes: r, rules: p, tables: append([]int(nil), e.RoutingTables...)}, nil
}

// Capture returns bounded private journal evidence, not an API response. The
// caller must persist it before Apply; a capture admits exactly one attempt.
func (k *RoutedKernel) Capture(ctx context.Context, candidate Config) (json.RawMessage, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	k.admitted = ""
	routes, err := PrivateRoutesForConfig(candidate, k.tables)
	if err != nil {
		return nil, err
	}
	if err := k.routes.checkScope(routes); err != nil {
		return nil, err
	}
	rules, err := PolicySnapshotForConfig(candidate)
	if err != nil {
		return nil, err
	}
	if err := k.rules.checkScope(rules); err != nil {
		return nil, err
	}
	if err := k.firewall.Check(ctx, candidate); err != nil {
		return nil, err
	}
	f, err := k.firewall.Capture(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := guardSnapshot(f); err != nil {
		return nil, err
	}
	r, err := k.routes.Capture(ctx)
	if err != nil {
		return nil, err
	}
	p, err := k.rules.Capture(ctx)
	if err != nil {
		return nil, err
	}
	evidence := routedRecovery{Version: 1, Candidate: configFingerprint(candidate), Firewall: &f, Routes: &r, Rules: &p}
	raw, err := json.Marshal(evidence)
	if err != nil || len(raw) > 1<<20 {
		return nil, errors.New("routed recovery evidence exceeds capacity")
	}
	if _, err := k.decode(candidate, raw); err != nil {
		return nil, err
	}
	k.admitted = string(raw)
	return raw, nil
}

func (k *RoutedKernel) decode(candidate Config, raw json.RawMessage) (routedRecovery, error) {
	var e routedRecovery
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if len(raw) > 1<<20 || d.Decode(&e) != nil || d.Decode(new(any)) != io.EOF || e.Version != 1 || e.Candidate != configFingerprint(candidate) || e.Firewall == nil || e.Routes == nil || e.Rules == nil {
		return e, errors.New("invalid routed recovery evidence")
	}
	candidateRoutes, err := PrivateRoutesForConfig(candidate, k.tables)
	if err != nil {
		return e, err
	}
	if err := k.routes.checkScope(candidateRoutes); err != nil {
		return e, err
	}
	candidateRules, err := PolicySnapshotForConfig(candidate)
	if err != nil {
		return e, err
	}
	if err := k.rules.checkScope(candidateRules); err != nil {
		return e, err
	}
	if _, err := guardSnapshot(*e.Firewall); err != nil {
		return e, err
	}
	if e.Firewall.Objects == nil || e.Routes.Version != 1 || e.Routes.Tables == nil || e.Routes.Routes == nil || e.Rules.Rules == nil {
		return e, errors.New("incomplete routed recovery evidence")
	}
	r, err := normalizeRouteSnapshot(e.Routes.Tables, e.Routes.Routes)
	if err != nil {
		return e, err
	}
	scope, err := normalizeRouteSnapshot(k.tables, nil)
	if err != nil {
		return e, err
	}
	a, _ := json.Marshal(r.Tables)
	b, _ := json.Marshal(scope.Tables)
	if !bytes.Equal(a, b) {
		return e, errors.New("routed recovery table scope differs")
	}
	if _, err := normalizePolicySnapshot(*e.Rules); err != nil {
		return e, err
	}
	if err := k.routes.checkScope(*e.Routes); err != nil {
		return e, err
	}
	if err := k.rules.checkScope(*e.Rules); err != nil {
		return e, err
	}
	return e, nil
}

// Apply retains an unconditional forwarding barrier on success and failure.
// The durable outer transaction must recover every failed or uncertain attempt.
func (k *RoutedKernel) Apply(ctx context.Context, candidate Config, raw json.RawMessage) (NFTSnapshot, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	if k.admitted == "" || k.admitted != string(raw) {
		return NFTSnapshot{}, errors.New("routed apply lacks a fresh capture")
	}
	k.admitted = ""
	e, err := k.decode(candidate, raw)
	if err != nil {
		return NFTSnapshot{}, err
	}
	r, _ := PrivateRoutesForConfig(candidate, k.tables)
	p, err := PolicySnapshotForConfig(candidate)
	if err != nil {
		return NFTSnapshot{}, err
	}
	if err := k.routes.Verify(ctx, *e.Routes); err != nil {
		return NFTSnapshot{}, err
	}
	if err := k.rules.Verify(ctx, *e.Rules); err != nil {
		return NFTSnapshot{}, err
	}
	guard, err := k.firewall.InstallForwardGuard(ctx, *e.Firewall)
	if err != nil {
		return NFTSnapshot{}, err
	}
	if err := k.routes.Apply(ctx, *e.Routes, r); err != nil {
		return NFTSnapshot{}, err
	}
	if err := k.rules.Apply(ctx, *e.Rules, p); err != nil {
		return NFTSnapshot{}, err
	}
	guard, err = k.firewall.ApplyGuarded(ctx, candidate, guard)
	if err != nil {
		return NFTSnapshot{}, err
	}
	if err := k.routes.Verify(ctx, r); err != nil {
		return NFTSnapshot{}, err
	}
	if err := k.rules.Verify(ctx, p); err != nil {
		return NFTSnapshot{}, err
	}
	if err := verifyForwardGuard(guard); err != nil {
		return NFTSnapshot{}, err
	}
	return guard, k.firewall.Verify(ctx, guard)
}

// Restore reinstates the before firewall's management rules with a forwarding
// barrier first, then restores routes/rules from their journaled unions. It
// keeps the barrier in place: the outer coordinator releases it only after
// restoring and verifying all remaining resources and management access.
func (k *RoutedKernel) Restore(ctx context.Context, candidate Config, raw json.RawMessage) (NFTSnapshot, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	k.admitted = ""
	e, err := k.decode(candidate, raw)
	if err != nil {
		return NFTSnapshot{}, err
	}
	r, _ := PrivateRoutesForConfig(candidate, k.tables)
	p, err := PolicySnapshotForConfig(candidate)
	if err != nil {
		return NFTSnapshot{}, err
	}
	guard, _ := guardSnapshot(*e.Firewall)
	if err := k.firewall.Restore(ctx, guard); err != nil {
		return NFTSnapshot{}, err
	}
	if err := k.routes.Restore(ctx, *e.Routes, r); err != nil {
		return NFTSnapshot{}, err
	}
	if err := k.rules.Restore(ctx, *e.Rules, p); err != nil {
		return NFTSnapshot{}, err
	}
	if err := k.routes.Verify(ctx, *e.Routes); err != nil {
		return NFTSnapshot{}, err
	}
	if err := k.rules.Verify(ctx, *e.Rules); err != nil {
		return NFTSnapshot{}, err
	}
	return guard, k.firewall.Verify(ctx, guard)
}
