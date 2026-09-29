package network

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

// Synthetic kernel for fault injection only. Native tests separately exercise ip.
type syntheticRoutes struct {
	live      []KernelRoute
	known     []KernelRoute
	mutations int
	failAfter bool
}

func (k *syntheticRoutes) run(_ context.Context, args []string) ([]byte, error) {
	if args[0] == "-j" {
		family := "ipv4"
		if args[2] == "-6" {
			family = "ipv6"
		}
		rows := []map[string]any{}
		for _, r := range k.live {
			if r.Family != family {
				continue
			}
			flags := []string{}
			if r.OnLink {
				flags = append(flags, "onlink")
			}
			row := map[string]any{"dst": r.Destination, "table": r.Table, "protocol": 4, "scope": r.Scope, "metric": r.Metric, "dev": r.Interface, "flags": flags}
			if r.Gateway != "" {
				row["gateway"] = r.Gateway
			}
			rows = append(rows, row)
		}
		return json.Marshal(rows)
	}
	for _, r := range k.known {
		for _, action := range []string{"add", "delete"} {
			if !reflect.DeepEqual(args, routeArguments(action, r)) {
				continue
			}
			k.mutations++
			if action == "add" {
				k.live = append(k.live, r)
			} else {
				for index, live := range k.live {
					if live == r {
						k.live = append(k.live[:index], k.live[index+1:]...)
						break
					}
				}
			}
			if k.failAfter {
				k.failAfter = false
				return nil, errors.New("synthetic lost acknowledgement after kernel mutation")
			}
			return nil, nil
		}
	}
	return nil, errors.New("unexpected synthetic command")
}

func routeMutationFixture() (*PrivateRouteHost, *syntheticRoutes, RouteSnapshot, RouteSnapshot) {
	a := KernelRoute{Family: "ipv4", Table: 10000, Destination: "0.0.0.0/0", Interface: "testwan0", Gateway: "192.0.2.1", Metric: 100, OnLink: true}
	b := a
	b.Gateway = "192.0.2.3"
	k := &syntheticRoutes{live: []KernelRoute{a}, known: []KernelRoute{a, b}}
	h := &PrivateRouteHost{tables: []int{10000}, names: map[string]bool{"testwan0": true}, run: k.run}
	before, _ := normalizeRouteSnapshot(h.tables, []KernelRoute{a})
	next, _ := normalizeRouteSnapshot(h.tables, []KernelRoute{b})
	return h, k, before, next
}

func TestPrivateRouteApplyReadbackAndRestore(t *testing.T) {
	h, k, before, next := routeMutationFixture()
	ctx := context.Background()
	if err := h.Apply(ctx, before, next); err != nil {
		t.Fatal(err)
	}
	if k.mutations != 2 || h.Verify(ctx, next) != nil {
		t.Fatal("candidate did not replace exact owned route")
	}
	if err := h.Restore(ctx, before, next); err != nil || h.Verify(ctx, before) != nil {
		t.Fatal("restoration failed", err)
	}
	count := k.mutations
	if err := h.Restore(ctx, before, next); err != nil || k.mutations != count {
		t.Fatal("repeated restoration rewrote already restored state", err)
	}
}

func TestPrivateRouteLostAcknowledgementRequiresRecoveryNotReplay(t *testing.T) {
	h, k, before, next := routeMutationFixture()
	k.failAfter = true
	ctx := context.Background()
	if err := h.Apply(ctx, before, next); err == nil {
		t.Fatal("lost acknowledgement reported success")
	}
	if len(k.live) != 0 || k.mutations != 1 {
		t.Fatal("uncertain delete was retried or followed by candidate add")
	}
	if err := h.Apply(ctx, before, next); err == nil || k.mutations != 1 {
		t.Fatal("candidate replay mutated partial state")
	}
	if err := h.Restore(ctx, before, next); err != nil || h.Verify(ctx, before) != nil {
		t.Fatal("partial state could not recover", err)
	}
}

func TestPrivateRouteRefusesDriftAndScopeEscapeBeforeMutation(t *testing.T) {
	h, k, before, next := routeMutationFixture()
	k.live[0].Gateway = "192.0.2.9"
	if err := h.Apply(context.Background(), before, next); err == nil || k.mutations != 0 {
		t.Fatal("apply overwrote drift")
	}
	if err := h.Restore(context.Background(), before, next); err == nil || k.mutations != 0 {
		t.Fatal("restore destroyed unknown route")
	}
	for _, mutate := range []func(*RouteSnapshot){
		func(s *RouteSnapshot) { s.Tables = []int{254}; s.Routes[0].Table = 254 },
		func(s *RouteSnapshot) { s.Routes[0].Interface = "foreign0" },
		func(s *RouteSnapshot) { s.Routes[0].Destination = "default;id" },
		func(s *RouteSnapshot) { s.Version = 2 },
	} {
		h, k, before, next := routeMutationFixture()
		mutate(&next)
		if err := h.Apply(context.Background(), before, next); err == nil || k.mutations != 0 {
			t.Fatal("scope escape reached kernel")
		}
	}
}

func TestPrivateRouteArgumentsAreExactAndNeverFlushOrReplace(t *testing.T) {
	_, _, before, _ := routeMutationFixture()
	args := routeArguments("delete", before.Routes[0])
	want := []string{"-4", "route", "delete", "0.0.0.0/0", "table", "10000", "proto", "4", "scope", "0", "metric", "100", "dev", "testwan0", "via", "192.0.2.1", "onlink"}
	if !reflect.DeepEqual(args, want) || strings.Contains(strings.Join(args, " "), "flush") {
		t.Fatal("unexpected deletion scope", args)
	}
}
