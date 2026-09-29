package network

import "net/netip"

// Linux substitutes IP6_RT_PRIO_USER for a zero IPv6 route metric. Use the
// effective value in validation, rendering and simulation, not just readback.
func effectiveRouteMetric(r Route) int {
	p, err := netip.ParsePrefix(r.Destination)
	if err == nil && p.Addr().Is6() && r.Metric == 0 {
		return 1024
	}
	return r.Metric
}

// PrivateRoutesForConfig compiles only static routes in explicitly supplied
// private table scope. Main/local routes and dynamically learned routes are
// separate resources; this function never imports them or enrolls a table.
func PrivateRoutesForConfig(c Config, tables []int) (RouteSnapshot, error) {
	if err := c.Validate(); err != nil {
		return RouteSnapshot{}, err
	}
	names := map[string]string{}
	for _, link := range c.Interfaces {
		names[link.ID] = link.Name
	}
	routes := []KernelRoute{}
	for _, r := range c.Routes {
		if !privateRouteTable(r.Table) {
			continue
		}
		p, _ := netip.ParsePrefix(r.Destination)
		family, scope := "ipv6", 0
		if p.Addr().Is4() {
			family = "ipv4"
			if r.Gateway == "" {
				scope = 253
			}
		}
		routes = append(routes, KernelRoute{Family: family, Table: r.Table,
			Destination: r.Destination, Interface: names[r.Interface], Gateway: r.Gateway,
			Metric: effectiveRouteMetric(r), Scope: scope, OnLink: r.Gateway != ""})
	}
	return normalizeRouteSnapshot(tables, routes)
}
