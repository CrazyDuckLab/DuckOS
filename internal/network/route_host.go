package network

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"runtime"
	"time"
)

// PrivateRouteHost captures only root-enrolled private route tables. It does not
// enroll resources or expose command arguments through IPC. Mutation and full
// link/RPDB recovery remain gated by the composite network transaction.
type PrivateRouteHost struct {
	tables []int
	names  map[string]bool
	run    func(context.Context, []string) ([]byte, error)
}

func NewPrivateRouteHost(enrollment Enrollment) (*PrivateRouteHost, error) {
	if runtime.GOOS != "linux" || os.Geteuid() != 0 {
		return nil, errors.New("route operations require privileged Linux Agent")
	}
	if err := enrollment.Validate(); err != nil {
		return nil, err
	}
	names := map[string]bool{}
	for _, binding := range enrollment.Physical {
		names[binding.Name] = true
	}
	for _, name := range enrollment.VLANNames {
		names[name] = true
	}
	return &PrivateRouteHost{tables: append([]int(nil), enrollment.RoutingTables...), names: names, run: runRouteInventory}, nil
}

func (h *PrivateRouteHost) Capture(ctx context.Context) (RouteSnapshot, error) {
	if _, err := normalizeRouteSnapshot(h.tables, nil); err != nil {
		return RouteSnapshot{}, err
	}
	tables := map[int]bool{}
	for _, table := range h.tables {
		tables[table] = true
	}
	routes := []KernelRoute{}
	for _, family := range []string{"ipv4", "ipv6"} {
		flag := "-4"
		if family == "ipv6" {
			flag = "-6"
		}
		// Listing all tables distinguishes successful absence from a failed
		// table-specific query. Numeric IDs avoid host alias interpretation.
		raw, err := h.run(ctx, []string{"-j", "-N", flag, "route", "show", "table", "all"})
		if err != nil {
			return RouteSnapshot{}, err
		}
		part, err := decodePrivateRoutes(raw, family, tables)
		if err != nil {
			return RouteSnapshot{}, err
		}
		routes = append(routes, part...)
	}
	s, err := normalizeRouteSnapshot(h.tables, routes)
	if err != nil {
		return s, err
	}
	return s, h.checkScope(s)
}

func runRouteInventory(parent context.Context, args []string) ([]byte, error) {
	if runtime.GOOS != "linux" || os.Geteuid() != 0 {
		return nil, errors.New("route inventory unavailable")
	}
	ctx, cancel := context.WithTimeout(parent, 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/usr/sbin/ip", args...)
	cmd.Env = []string{"PATH=/usr/sbin:/usr/bin", "LANG=C", "LC_ALL=C"}
	cmd.Dir = "/"
	cmd.Stderr = io.Discard
	cmd.WaitDelay = time.Second
	var output boundedRouteOutput
	cmd.Stdout = &output
	if err := cmd.Run(); err != nil {
		return nil, errors.New("route inventory failed or exceeded limits")
	}
	return append([]byte(nil), output.data...), nil
}

type boundedRouteOutput struct{ data []byte }

func (b *boundedRouteOutput) Write(p []byte) (int, error) {
	if len(p) > (2<<20)-len(b.data) {
		return 0, errors.New("route inventory exceeds capacity")
	}
	b.data = append(b.data, p...)
	return len(p), nil
}
