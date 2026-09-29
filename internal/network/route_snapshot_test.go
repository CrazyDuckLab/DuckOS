package network

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

const syntheticRouteV4 = `[{"dst":"default","table":"10000","gateway":"192.0.2.1","dev":"testwan0","protocol":"4","metric":100,"flags":["onlink"]}]`
const syntheticRouteV6 = `[{"dst":"2001:db8::1","table":10000,"dev":"testwan0","protocol":"4","metric":1024,"flags":[],"pref":"medium"}]`

func TestPrivateRouteCaptureUsesNumericBoundedInventory(t *testing.T) {
	var calls [][]string
	h := PrivateRouteHost{tables: []int{10001, 10000}, names: map[string]bool{"testwan0": true}, run: func(_ context.Context, args []string) ([]byte, error) {
		calls = append(calls, args)
		if args[2] == "-4" {
			return []byte(syntheticRouteV4), nil
		}
		return []byte(syntheticRouteV6), nil
	}}
	s, err := h.Capture(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(calls, [][]string{{"-j", "-N", "-4", "route", "show", "table", "all"}, {"-j", "-N", "-6", "route", "show", "table", "all"}}) {
		t.Fatal("unexpected inventory command", calls)
	}
	if !reflect.DeepEqual(s.Tables, []int{10000, 10001}) || len(s.Routes) != 2 || s.Version != 1 {
		t.Fatal("invalid snapshot scope")
	}
	byFamily := map[string]KernelRoute{}
	for _, r := range s.Routes {
		byFamily[r.Family] = r
	}
	if byFamily["ipv4"].Destination != "0.0.0.0/0" || !byFamily["ipv4"].OnLink || byFamily["ipv6"].Destination != "2001:db8::1/128" {
		t.Fatal("kernel route semantics lost")
	}
}

func TestPrivateRouteCaptureNeverConvertsFailureToAbsence(t *testing.T) {
	h := PrivateRouteHost{tables: []int{10000}, run: func(context.Context, []string) ([]byte, error) {
		return nil, errors.New("synthetic tool failure")
	}}
	if _, err := h.Capture(context.Background()); err == nil {
		t.Fatal("failed inventory became empty recovery state")
	}
	h.run = func(context.Context, []string) ([]byte, error) { return []byte(`[]`), nil }
	s, err := h.Capture(context.Background())
	if err != nil || len(s.Routes) != 0 || len(s.Tables) != 1 {
		t.Fatal("valid empty owned tables rejected", err)
	}
	h.tables = []int{254}
	if _, err := h.Capture(context.Background()); err == nil {
		t.Fatal("main table admitted to private recovery")
	}
}

func TestPrivateRouteInventoryRejectsLossyState(t *testing.T) {
	for name, mutate := range map[string]func(map[string]any){
		"dynamic":             func(r map[string]any) { r["protocol"] = "16" },
		"missing protocol":    func(r map[string]any) { delete(r, "protocol") },
		"multipath":           func(r map[string]any) { r["multipath"] = []any{} },
		"preferred source":    func(r map[string]any) { r["prefsrc"] = "192.0.2.2" },
		"nexthop object":      func(r map[string]any) { r["nhid"] = 3 },
		"metric overflow":     func(r map[string]any) { r["metric"] = uint64(4294967295) },
		"scope":               func(r map[string]any) { r["scope"] = "254" },
		"link down":           func(r map[string]any) { r["flags"] = []string{"onlink", "linkdown"} },
		"duplicate flag":      func(r map[string]any) { r["flags"] = []string{"onlink", "onlink"} },
		"null flags":          func(r map[string]any) { r["flags"] = nil },
		"type":                func(r map[string]any) { r["type"] = "blackhole" },
		"name injection":      func(r map[string]any) { r["dev"] = "testwan0;id" },
		"noncanonical prefix": func(r map[string]any) { r["dst"] = "192.0.2.1/24" },
		"wrong family":        func(r map[string]any) { r["gateway"] = "2001:db8::1" },
		"table alias":         func(r map[string]any) { r["table"] = "custom-name" },
		"null destination":    func(r map[string]any) { r["dst"] = nil },
	} {
		t.Run(name, func(t *testing.T) {
			var r []map[string]any
			_ = json.Unmarshal([]byte(syntheticRouteV4), &r)
			mutate(r[0])
			b, _ := json.Marshal(r)
			if _, err := decodePrivateRoutes(b, "ipv4", map[int]bool{10000: true}); err == nil {
				t.Fatal("unsupported state accepted for recovery")
			}
		})
	}
	for _, bad := range []string{`null`, `{}`, `[{}]`, `[] []`, strings.Repeat(" ", (2<<20)+1)} {
		if _, err := decodePrivateRoutes([]byte(bad), "ipv4", map[int]bool{10000: true}); err == nil {
			t.Fatal("invalid inventory accepted")
		}
	}
}

func TestForeignTablesAreExcludedWithoutImportingUnsupportedSemantics(t *testing.T) {
	r, err := decodePrivateRoutes([]byte(`[{"dst":"default","table":"254","multipath":[{}]},{"dst":"192.0.2.2","table":"255","type":"local"}]`), "ipv4", map[int]bool{10000: true})
	if err != nil || len(r) != 0 {
		t.Fatal("foreign tables imported", err)
	}
}

func TestRouteSnapshotRejectsDuplicateKeysAndMalformedRecovery(t *testing.T) {
	routes, err := decodePrivateRoutes([]byte(syntheticRouteV4), "ipv4", map[int]bool{10000: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := normalizeRouteSnapshot([]int{10000}, append(routes, routes[0])); err == nil {
		t.Fatal("ambiguous route key accepted")
	}
	routes[0].Interface = "../../foreign"
	if _, err := normalizeRouteSnapshot([]int{10000}, routes); err == nil {
		t.Fatal("malformed recovery route accepted")
	}
}

func FuzzPrivateRouteInventory(f *testing.F) {
	f.Add([]byte(syntheticRouteV4))
	f.Add([]byte(syntheticRouteV6))
	f.Add([]byte(`[]`))
	f.Fuzz(func(t *testing.T, data []byte) {
		for _, family := range []string{"ipv4", "ipv6"} {
			routes, err := decodePrivateRoutes(data, family, map[int]bool{10000: true})
			if err != nil {
				continue
			}
			for _, r := range routes {
				if err := r.validate(); err != nil {
					t.Fatal("decoder returned unsafe route", err)
				}
			}
		}
	})
}
