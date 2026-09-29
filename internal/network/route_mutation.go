package network

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
)

func (h *PrivateRouteHost) checkScope(s RouteSnapshot) error {
	if s.Version != 1 {
		return errors.New("unsupported route recovery version")
	}
	normal, err := normalizeRouteSnapshot(s.Tables, s.Routes)
	if err != nil {
		return err
	}
	owned, err := normalizeRouteSnapshot(h.tables, nil)
	if err != nil {
		return err
	}
	left, _ := json.Marshal(normal.Tables)
	right, _ := json.Marshal(owned.Tables)
	if string(left) != string(right) {
		return errors.New("route snapshot does not match root-enrolled table scope")
	}
	for _, route := range normal.Routes {
		if !h.names[route.Interface] {
			return errors.New("route interface is not root-enrolled")
		}
	}
	return nil
}

func equalRoutes(a, b RouteSnapshot) bool {
	x, e1 := normalizeRouteSnapshot(a.Tables, a.Routes)
	y, e2 := normalizeRouteSnapshot(b.Tables, b.Routes)
	left, _ := json.Marshal(x)
	right, _ := json.Marshal(y)
	return e1 == nil && e2 == nil && string(left) == string(right)
}

func (h *PrivateRouteHost) Verify(ctx context.Context, expected RouteSnapshot) error {
	if err := h.checkScope(expected); err != nil {
		return err
	}
	live, err := h.Capture(ctx)
	if err != nil {
		return err
	}
	if !equalRoutes(live, expected) {
		return errors.New("private route tables differ from expected state")
	}
	return nil
}

// Apply is an Agent-side primitive, not an enabled RPC. The composite controller
// must durably journal before/candidate and guard affected traffic first. iproute2
// operations are not atomic. An uncertain result requires Restore, never replay.
func (h *PrivateRouteHost) Apply(ctx context.Context, before, candidate RouteSnapshot) error {
	if err := h.checkScope(before); err != nil {
		return err
	}
	if err := h.checkScope(candidate); err != nil {
		return err
	}
	if err := h.Verify(ctx, before); err != nil {
		return err
	}
	return h.change(ctx, before, candidate)
}

// Restore admits only surviving members of the journaled before/candidate union.
// Unknown drift is preserved and reported instead of flushing an entire table.
func (h *PrivateRouteHost) Restore(ctx context.Context, before, candidate RouteSnapshot) error {
	if err := h.checkScope(before); err != nil {
		return err
	}
	if err := h.checkScope(candidate); err != nil {
		return err
	}
	live, err := h.Capture(ctx)
	if err != nil {
		return err
	}
	known := map[string]bool{}
	for _, snapshot := range []RouteSnapshot{before, candidate} {
		for _, route := range snapshot.Routes {
			known[routeKey(route)] = true
		}
	}
	for _, route := range live.Routes {
		if !known[routeKey(route)] {
			return errors.New("unknown route drift blocks safe restoration")
		}
	}
	return h.change(ctx, live, before)
}

func routeArguments(action string, r KernelRoute) []string {
	family := "-4"
	if r.Family == "ipv6" {
		family = "-6"
	}
	args := []string{family, "route", action, r.Destination, "table", strconv.Itoa(r.Table), "proto", "4", "scope", strconv.Itoa(r.Scope), "metric", strconv.Itoa(r.Metric), "dev", r.Interface}
	if r.Gateway != "" {
		args = append(args, "via", r.Gateway)
		if r.OnLink {
			args = append(args, "onlink")
		}
	}
	return args
}

func (h *PrivateRouteHost) change(ctx context.Context, from, to RouteSnapshot) error {
	old, _ := normalizeRouteSnapshot(from.Tables, from.Routes)
	next, _ := normalizeRouteSnapshot(to.Tables, to.Routes)
	before, after := map[string]bool{}, map[string]bool{}
	for _, r := range old.Routes {
		before[routeKey(r)] = true
	}
	for _, r := range next.Routes {
		after[routeKey(r)] = true
	}
	var commands [][]string
	for _, r := range old.Routes {
		if !after[routeKey(r)] {
			commands = append(commands, routeArguments("delete", r))
		}
	}
	for _, r := range next.Routes {
		if !before[routeKey(r)] {
			commands = append(commands, routeArguments("add", r))
		}
	}
	// Admission is checked before the first delete, including during recovery.
	if len(commands) > 256 {
		return errors.New("route mutation exceeds bounded transaction work")
	}
	for _, args := range commands {
		if _, err := h.run(ctx, args); err != nil {
			return errors.New("route mutation outcome uncertain; recovery required")
		}
	}
	return h.Verify(ctx, to)
}
