package network

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"sort"
	"strconv"
)

// KernelRoute contains only reproducible static unicast state in root-enrolled
// private tables. Unknown attributes in those tables are rejected, not omitted
// from recovery evidence. Main/local and foreign tables are never imported.
type KernelRoute struct {
	Family      string `json:"family"`
	Table       int    `json:"table"`
	Destination string `json:"destination"`
	Interface   string `json:"interface"`
	Gateway     string `json:"gateway,omitempty"`
	Metric      int    `json:"metric"`
	Scope       int    `json:"scope"`
	OnLink      bool   `json:"onLink"`
}

type RouteSnapshot struct {
	Version int           `json:"version"`
	Tables  []int         `json:"tables"`
	Routes  []KernelRoute `json:"routes"`
}

func routeNumber(raw json.RawMessage, fallback int) (int, error) {
	if len(raw) == 0 {
		return fallback, nil
	}
	var value int
	if json.Unmarshal(raw, &value) == nil && string(raw) != "null" {
		return value, nil
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		value, err := strconv.Atoi(text)
		if err == nil && strconv.Itoa(value) == text {
			return value, nil
		}
	}
	return 0, errors.New("route inventory requires numeric identifiers")
}

func decodePrivateRoutes(raw []byte, family string, tables map[int]bool) ([]KernelRoute, error) {
	if len(raw) > 2<<20 || (family != "ipv4" && family != "ipv6") {
		return nil, errors.New("route inventory exceeds bounds or family is invalid")
	}
	var objects []map[string]json.RawMessage
	if json.Unmarshal(raw, &objects) != nil || objects == nil || len(objects) > 8192 {
		return nil, errors.New("invalid or oversized route inventory")
	}
	routes := []KernelRoute{}
	for _, object := range objects {
		var destinationMarker string
		if json.Unmarshal(object["dst"], &destinationMarker) != nil || destinationMarker == "" {
			return nil, errors.New("incomplete route inventory entry")
		}
		table, err := routeNumber(object["table"], 254)
		if err != nil {
			return nil, err
		}
		if !tables[table] {
			continue
		}
		if !privateRouteTable(table) {
			return nil, errors.New("route inventory scope is not private")
		}
		for key := range object {
			switch key {
			case "dst", "table", "protocol", "scope", "gateway", "dev", "metric", "flags", "pref", "type":
			default:
				return nil, errors.New("unsupported attribute in owned routing table")
			}
		}
		get := func(key string) (string, error) {
			var s string
			if v, ok := object[key]; ok && (json.Unmarshal(v, &s) != nil || bytes.Equal(v, []byte("null"))) {
				return "", errors.New("invalid route text attribute")
			}
			return s, nil
		}
		protocol, err := routeNumber(object["protocol"], 3)
		if err != nil || protocol != 4 {
			return nil, errors.New("owned routing table contains non-static route")
		}
		scope, err := routeNumber(object["scope"], 0)
		if err != nil || (scope != 0 && scope != 253) {
			return nil, errors.New("unsupported route scope")
		}
		metric, err := routeNumber(object["metric"], 0)
		if err != nil || metric < 0 || metric > 2147483647 {
			return nil, errors.New("invalid route metric")
		}
		destination, err := get("dst")
		if err != nil || destination == "" {
			return nil, errors.New("missing route destination")
		}
		// M-13: reject non-canonical destination strings
		// before any parse. The upstream parser is
		// permissive about leading zeros and unicode; DuckOS
		// refuses them so the same route cannot be imported
		// in two different textual shapes (e.g. "0000:000::0"
		// vs "::" vs "::/128").
		if destination != "default" && !canonicalDestination(destination, family) {
			return nil, errors.New("non-canonical destination")
		}
		if destination == "default" {
			destination = "0.0.0.0/0"
			if family == "ipv6" {
				destination = "::/0"
			}
		} else if address, err := netip.ParseAddr(destination); err == nil {
			destination = netip.PrefixFrom(address, address.BitLen()).String()
		}
		// M-13: DuckOS rejects any destination string that
		// contains characters outside the canonical IPv4
		// or IPv6 textual grammar before parse. The upstream
		// parser accepts unicode and zero-width whitespace
		// in some inputs (e.g. "Ꭴ" or "0000:000::0") which
		// would otherwise reach downstream validation as a
		// different shape than the operator typed.
		if !canonicalDestination(destination, family) {
			return nil, errors.New("non-canonical destination")
		}
		prefix, err := netip.ParsePrefix(destination)
		if err != nil || prefix != prefix.Masked() || prefix.Addr().Is4In6() || prefix.Addr().Is4() != (family == "ipv4") || prefix.Addr().IsMulticast() {
			return nil, errors.New("unsupported route destination")
		}
		dev, err := get("dev")
		if err != nil || !linkName.MatchString(dev) || dev == "lo" {
			return nil, errors.New("unsupported route interface")
		}
		gateway, err := get("gateway")
		if err != nil {
			return nil, err
		}
		if gateway != "" {
			address, err := netip.ParseAddr(gateway)
			if err != nil || !usable(address) || address.Is4() != (family == "ipv4") || address.Zone() != "" {
				return nil, errors.New("unsupported route gateway")
			}
			gateway = address.String()
		}
		typ, err := get("type")
		if err != nil || (typ != "" && typ != "unicast") {
			return nil, errors.New("unsupported route type")
		}
		pref, err := get("pref")
		if err != nil || (pref != "" && (family != "ipv6" || pref != "medium")) {
			return nil, errors.New("unsupported IPv6 route preference")
		}
		var flags []string
		if v, ok := object["flags"]; ok && (json.Unmarshal(v, &flags) != nil || flags == nil) {
			return nil, errors.New("invalid route flags")
		}
		onLink := false
		for _, flag := range flags {
			if flag != "onlink" || onLink || gateway == "" {
				return nil, errors.New("route flags cannot be reproduced safely")
			}
			onLink = true
		}
		routes = append(routes, KernelRoute{Family: family, Table: table, Destination: prefix.String(), Interface: dev, Gateway: gateway, Metric: metric, Scope: scope, OnLink: onLink})
		if len(routes) > 512 {
			return nil, errors.New("owned route count exceeds recovery limit")
		}
	}
	return routes, nil
}

func normalizeRouteSnapshot(tables []int, routes []KernelRoute) (RouteSnapshot, error) {
	owned := map[int]bool{}
	for _, table := range tables {
		if !privateRouteTable(table) || owned[table] {
			return RouteSnapshot{}, errors.New("invalid private table scope")
		}
		owned[table] = true
	}
	if len(tables) > 128 || len(routes) > 512 {
		return RouteSnapshot{}, errors.New("route snapshot exceeds recovery limits")
	}
	s := RouteSnapshot{Version: 1, Tables: append([]int{}, tables...), Routes: append([]KernelRoute{}, routes...)}
	sort.Ints(s.Tables)
	sort.Slice(s.Routes, func(i, j int) bool { return routeKey(s.Routes[i]) < routeKey(s.Routes[j]) })
	keys := map[string]bool{}
	for _, route := range s.Routes {
		if err := route.validate(); err != nil {
			return RouteSnapshot{}, err
		}
		key := fmt.Sprintf("%s/%d/%s/%d", route.Family, route.Table, route.Destination, route.Metric)
		if !owned[route.Table] || keys[key] {
			return RouteSnapshot{}, errors.New("ambiguous or out-of-scope recovery route")
		}
		keys[key] = true
	}
	return s, nil
}

func (r KernelRoute) validate() error {
	p, err := netip.ParsePrefix(r.Destination)
	if err != nil || p != p.Masked() || p.String() != r.Destination ||
		(r.Family != "ipv4" && r.Family != "ipv6") || p.Addr().Is4() != (r.Family == "ipv4") ||
		p.Addr().Is4In6() || p.Addr().IsMulticast() || !privateRouteTable(r.Table) ||
		!linkName.MatchString(r.Interface) || r.Interface == "lo" || r.Metric < 0 ||
		(r.Family == "ipv6" && r.Metric == 0) ||
		r.Metric > 2147483647 || (r.Scope != 0 && r.Scope != 253) {
		return errors.New("invalid recovery route")
	}
	if r.Gateway == "" {
		if r.OnLink {
			return errors.New("on-link gateway flag without gateway")
		}
	} else {
		a, err := netip.ParseAddr(r.Gateway)
		if err != nil || !usable(a) || a.Is4() != (r.Family == "ipv4") || a.Zone() != "" || a.String() != r.Gateway {
			return errors.New("invalid recovery gateway")
		}
	}
	return nil
}

func routeKey(r KernelRoute) string {
	b, _ := json.Marshal(r)
	return string(b)
}

// canonicalDestination verifies the destination text is
// well-formed for the given family before any parse. It
// rejects non-canonical IPv6 shapes (e.g. "0000:000::0" with
// leading zeros) and any string that contains characters
// outside the IPv4 / IPv6 textual grammar. This is the
// M-13 boundary: the upstream parser is permissive about
// leading zeros and unicode; DuckOS refuses them so the
// same route cannot be imported in two different textual
// shapes.
func canonicalDestination(s, family string) bool {
	if s == "" {
		return false
	}
	switch family {
	case "ipv4":
		for _, c := range s {
			if (c < '0' || c > '9') && c != '.' && c != '/' {
				return false
			}
		}
		return true
	case "ipv6":
		hasZeroCompression := false
		for i, c := range s {
			switch {
			case (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F'):
				// Reject leading zeros inside a hex group
				// ("0000:000::0" style).
				if c == '0' && i > 0 && s[i-1] == ':' {
					if i+1 < len(s) && s[i+1] == '0' {
						return false
					}
				}
			case c == ':':
				if i > 0 && s[i-1] == ':' {
					if hasZeroCompression {
						// Multiple :: is non-canonical.
						return false
					}
					hasZeroCompression = true
				}
			case c == '/':
				// Prefix length suffix is allowed.
			default:
				return false
			}
		}
		return true
	}
	return false
}
