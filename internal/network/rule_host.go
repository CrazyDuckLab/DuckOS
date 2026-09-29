package network

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"runtime"
	"strconv"
	"sync"
	"time"
)

// PolicyRuleHost is a root-enrolled Agent primitive, not an enabled API/RPC.
// Its caller must journal before/candidate and install an independent traffic
// guard before mutation. Neither ip rule changes nor networkd reload are atomic.
type PolicyRuleHost struct {
	mu     sync.Mutex
	tables map[int]bool
	run    func(context.Context, []string) ([]byte, error)
}

func NewPolicyRuleHost(e Enrollment) (*PolicyRuleHost, error) {
	if runtime.GOOS != "linux" || os.Geteuid() != 0 {
		return nil, errors.New("policy operations require privileged Linux Agent")
	}
	if err := e.Validate(); err != nil {
		return nil, err
	}
	if !e.PolicyRouting {
		return nil, errors.New("policy priority ranges are not root-enrolled")
	}
	tables := map[int]bool{}
	for _, table := range e.RoutingTables {
		tables[table] = true
	}
	return &PolicyRuleHost{tables: tables, run: runRouteInventory}, nil
}

func (h *PolicyRuleHost) checkScope(s PolicySnapshot) error {
	normal, err := normalizePolicySnapshot(s)
	if err != nil {
		return err
	}
	for _, r := range normal.Rules {
		if r.Table != 0 && r.Table != 254 && !h.tables[r.Table] {
			return errors.New("policy lookup table is not root-enrolled")
		}
	}
	return nil
}
func equalPolicy(a, b PolicySnapshot) bool {
	x, e1 := normalizePolicySnapshot(a)
	y, e2 := normalizePolicySnapshot(b)
	left, _ := json.Marshal(x)
	right, _ := json.Marshal(y)
	return e1 == nil && e2 == nil && string(left) == string(right)
}
func policyKey(r KernelPolicyRule) string { raw, _ := json.Marshal(r); return string(raw) }

func (h *PolicyRuleHost) Capture(ctx context.Context) (PolicySnapshot, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.capture(ctx)
}
func (h *PolicyRuleHost) capture(ctx context.Context) (PolicySnapshot, error) {
	s := PolicySnapshot{Version: 1}
	for _, family := range []string{"ipv4", "ipv6"} {
		flag := "-4"
		if family == "ipv6" {
			flag = "-6"
		}
		raw, err := h.run(ctx, []string{"-j", "-N", "-details", flag, "rule", "show"})
		if err != nil {
			return PolicySnapshot{}, errors.New("policy inventory unavailable")
		}
		rules, err := decodePolicyInventory(raw, family)
		if err != nil {
			return PolicySnapshot{}, err
		}
		s.Rules = append(s.Rules, rules...)
	}
	normal, err := normalizePolicySnapshot(s)
	if err != nil {
		return PolicySnapshot{}, err
	}
	return normal, h.checkScope(normal)
}
func (h *PolicyRuleHost) Verify(ctx context.Context, want PolicySnapshot) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.verify(ctx, want)
}
func (h *PolicyRuleHost) verify(ctx context.Context, want PolicySnapshot) error {
	if err := h.checkScope(want); err != nil {
		return err
	}
	live, err := h.capture(ctx)
	if err != nil {
		return err
	}
	if !equalPolicy(live, want) {
		return errors.New("owned routing policy differs from expected state")
	}
	return nil
}

func (h *PolicyRuleHost) Apply(parent context.Context, before, candidate PolicySnapshot) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	if err := h.checkScope(before); err != nil {
		return err
	}
	if err := h.checkScope(candidate); err != nil {
		return err
	}
	if err := h.verify(ctx, before); err != nil {
		return err
	}
	return h.change(ctx, before, candidate)
}

// Restore accepts interrupted states only from the journaled union. Unknown
// drift and duplicate priorities remain untouched; no flush command is used.
func (h *PolicyRuleHost) Restore(parent context.Context, before, candidate PolicySnapshot) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	if err := h.checkScope(before); err != nil {
		return err
	}
	if err := h.checkScope(candidate); err != nil {
		return err
	}
	live, err := h.capture(ctx)
	if err != nil {
		return err
	}
	known := map[string]bool{}
	for _, s := range []PolicySnapshot{before, candidate} {
		normal, _ := normalizePolicySnapshot(s)
		for _, r := range normal.Rules {
			known[policyKey(r)] = true
		}
	}
	for _, r := range live.Rules {
		if !known[policyKey(r)] {
			return errors.New("unknown policy drift blocks restoration")
		}
	}
	return h.change(ctx, live, before)
}

func policyArguments(action string, r KernelPolicyRule) []string {
	family := "-4"
	if r.Family == "ipv6" {
		family = "-6"
	}
	args := []string{family, "rule", action, "priority", strconv.Itoa(r.Priority), "protocol", strconv.Itoa(r.Protocol)}
	from, to := r.From, r.To
	if from == "" {
		from = "all"
	}
	if to == "" {
		to = "all"
	}
	args = append(args, "from", from, "to", to)
	if r.Unreachable {
		args = append(args, "unreachable")
	} else {
		args = append(args, "table", strconv.Itoa(r.Table))
		if r.SuppressPrefixLength >= 0 {
			args = append(args, "suppress_prefixlength", strconv.Itoa(r.SuppressPrefixLength))
		}
	}
	return args
}

func (h *PolicyRuleHost) change(ctx context.Context, from, to PolicySnapshot) error {
	old, _ := normalizePolicySnapshot(from)
	next, _ := normalizePolicySnapshot(to)
	before, after := map[string]bool{}, map[string]bool{}
	for _, r := range old.Rules {
		before[policyKey(r)] = true
	}
	for _, r := range next.Rules {
		after[policyKey(r)] = true
	}
	var commands [][]string
	// Keep old guards until their lookups are removed; install new guards before
	// their lookups. Selector replacements still require the outer traffic guard.
	for _, guard := range []bool{false, true} {
		for _, r := range old.Rules {
			if r.Unreachable == guard && !after[policyKey(r)] {
				commands = append(commands, policyArguments("delete", r))
			}
		}
	}
	for _, guard := range []bool{true, false} {
		for _, r := range next.Rules {
			if r.Unreachable == guard && !before[policyKey(r)] {
				commands = append(commands, policyArguments("add", r))
			}
		}
	}
	if len(commands) > 256 {
		return errors.New("policy mutation exceeds bounded transaction work")
	}
	for _, args := range commands {
		if ctx.Err() != nil {
			return errors.New("policy operation interrupted; recovery required")
		}
		if _, err := h.run(ctx, args); err != nil {
			return errors.New("policy mutation outcome uncertain; recovery required")
		}
	}
	return h.verify(ctx, to)
}
