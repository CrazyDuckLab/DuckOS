package network

import (
	"context"
	"encoding/json"
	"errors"
	"net/netip"
	"reflect"
	"testing"
)

// This executor is only a fault-injection fixture; production always invokes
// the bounded Linux iproute2 executor and validates its numeric JSON inventory.
type syntheticPolicy struct {
	live, known []KernelPolicyRule
	commands    [][]string
	failAfter   int
}

func (k *syntheticPolicy) run(ctx context.Context, args []string) ([]byte, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if args[0] == "-j" {
		family := "ipv4"
		if args[3] == "-6" {
			family = "ipv6"
		}
		rows := []map[string]any{}
		for _, r := range k.live {
			if r.Family != family {
				continue
			}
			row := map[string]any{"priority": r.Priority, "src": "all", "protocol": r.Protocol}
			for _, item := range []struct{ key, length, value string }{{"src", "srclen", r.From}, {"dst", "dstlen", r.To}} {
				if item.value != "" {
					p := netip.MustParsePrefix(item.value)
					row[item.key] = p.Addr().String()
					if p.Bits() != p.Addr().BitLen() {
						row[item.length] = p.Bits()
					}
				}
			}
			if r.Unreachable {
				row["action"] = "unreachable"
			} else {
				row["table"] = r.Table
				if r.SuppressPrefixLength >= 0 {
					row["suppress_prefixlen"] = r.SuppressPrefixLength
				}
			}
			rows = append(rows, row)
		}
		return json.Marshal(rows)
	}
	for _, r := range k.known {
		for _, action := range []string{"add", "delete"} {
			if !reflect.DeepEqual(args, policyArguments(action, r)) {
				continue
			}
			k.commands = append(k.commands, append([]string(nil), args...))
			if action == "add" {
				k.live = append(k.live, r)
			} else {
				for i, live := range k.live {
					if live == r {
						k.live = append(k.live[:i], k.live[i+1:]...)
						break
					}
				}
			}
			if len(k.commands) == k.failAfter {
				k.failAfter = 0
				return nil, errors.New("synthetic lost acknowledgement")
			}
			return nil, nil
		}
	}
	return nil, errors.New("unexpected synthetic rule command")
}

func policyHostFixture() (*PolicyRuleHost, *syntheticPolicy, PolicySnapshot, PolicySnapshot) {
	a := KernelPolicyRule{Family: "ipv4", Priority: 20000, From: "198.51.100.0/24", Table: 10000, SuppressPrefixLength: -1, Protocol: 4}
	guard := a
	guard.Priority++
	guard.Table = 0
	guard.Unreachable = true
	b := a
	b.Table = 10001
	k := &syntheticPolicy{live: []KernelPolicyRule{a, guard}, known: []KernelPolicyRule{a, guard, b}}
	h := &PolicyRuleHost{tables: map[int]bool{10000: true, 10001: true}, run: k.run}
	return h, k, PolicySnapshot{Version: 1, Rules: []KernelPolicyRule{a, guard}}, PolicySnapshot{Version: 1, Rules: []KernelPolicyRule{b, guard}}
}

func TestPolicyHostApplyRestoreAndNoBlindReplay(t *testing.T) {
	h, k, before, next := policyHostFixture()
	ctx := context.Background()
	if err := h.Apply(ctx, before, next); err != nil {
		t.Fatal(err)
	}
	if len(k.commands) != 2 || h.Verify(ctx, next) != nil {
		t.Fatal("incorrect policy replacement", k.commands)
	}
	if err := h.Apply(ctx, before, next); err == nil || len(k.commands) != 2 {
		t.Fatal("blind replay admitted")
	}
	if err := h.Restore(ctx, before, next); err != nil || h.Verify(ctx, before) != nil {
		t.Fatal("restore failed", err)
	}
	count := len(k.commands)
	if err := h.Restore(ctx, before, next); err != nil || len(k.commands) != count {
		t.Fatal("idempotent recovery rewrote rules", err)
	}
}

func TestPolicyHostLostAcknowledgementAndDrift(t *testing.T) {
	for _, failedCommand := range []int{1, 2} {
		h, k, before, next := policyHostFixture()
		k.failAfter = failedCommand
		if err := h.Apply(context.Background(), before, next); err == nil {
			t.Fatal("uncertain mutation reported success")
		}
		if err := h.Restore(context.Background(), before, next); err != nil || h.Verify(context.Background(), before) != nil {
			t.Fatal("uncertain mutation not recovered", err)
		}
	}
	h, k, before, next := policyHostFixture()
	k.live[0].Protocol = 99
	if err := h.Restore(context.Background(), before, next); err == nil || len(k.commands) != 0 {
		t.Fatal("unknown drift changed")
	}
	k.live = append(k.live, k.live[0])
	if _, err := h.Capture(context.Background()); err == nil {
		t.Fatal("duplicate priorities captured ambiguously")
	}
}

func TestPolicyHostGuardOrderAndScope(t *testing.T) {
	h, k, before, _ := policyHostFixture()
	next := PolicySnapshot{Version: 1, Rules: append([]KernelPolicyRule(nil), before.Rules...)}
	for i := range next.Rules {
		next.Rules[i].From = "203.0.113.0/24"
		k.known = append(k.known, next.Rules[i])
	}
	if err := h.Apply(context.Background(), before, next); err != nil {
		t.Fatal(err)
	}
	want := [][]string{policyArguments("delete", before.Rules[0]), policyArguments("delete", before.Rules[1]), policyArguments("add", next.Rules[1]), policyArguments("add", next.Rules[0])}
	if !reflect.DeepEqual(k.commands, want) {
		t.Fatal("guard ordering differs", k.commands)
	}
	h, k, before, next = policyHostFixture()
	delete(h.tables, 10001)
	if err := h.Apply(context.Background(), before, next); err == nil || len(k.commands) != 0 {
		t.Fatal("unenrolled table mutated")
	}
	next.Rules[0].Priority = 32766
	if err := h.Apply(context.Background(), before, next); err == nil || len(k.commands) != 0 {
		t.Fatal("system priority mutated")
	}
}

func TestPolicyHostCapacityAndCancellationBeforeMutation(t *testing.T) {
	h, k, before, _ := policyHostFixture()
	next := PolicySnapshot{Version: 1}
	for i := 0; i < 129; i++ {
		r := before.Rules[0]
		r.Priority = 20000 + i*2
		guard := r
		guard.Priority++
		guard.Table = 0
		guard.Unreachable = true
		next.Rules = append(next.Rules, r, guard)
	}
	empty := PolicySnapshot{Version: 1}
	k.live = nil
	if err := h.Apply(context.Background(), empty, next); err == nil || len(k.commands) != 0 {
		t.Fatal("oversized mutation started")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := h.Apply(ctx, empty, before); err == nil || len(k.commands) != 0 {
		t.Fatal("cancelled mutation started")
	}
}
