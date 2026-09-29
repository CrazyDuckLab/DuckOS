package network

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestPrivateRoutePlanPreservesScopeAndKernelSemantics(t *testing.T) {
	c := fixture()
	c.Interfaces[0].Addresses = append(c.Interfaces[0].Addresses, "2001:db8:2::2/64")
	c.Routes = append(c.Routes,
		Route{ID: "private-v4", Interface: "uplink", Destination: "0.0.0.0/0", Gateway: "192.0.2.1", Table: 10000},
		Route{ID: "private-v6", Interface: "uplink", Destination: "::/0", Gateway: "2001:db8:2::1", Table: 10000},
		Route{ID: "direct-v4", Interface: "inside", Destination: "203.0.113.0/24", Table: 10001},
		Route{ID: "direct-v6", Interface: "inside", Destination: "2001:db8:3::/64", Table: 10001})
	before, _ := json.Marshal(c)
	s, err := PrivateRoutesForConfig(c, []int{10001, 10000})
	if err != nil {
		t.Fatal(err)
	}
	want, _ := normalizeRouteSnapshot([]int{10000, 10001}, []KernelRoute{
		{Family: "ipv4", Table: 10000, Destination: "0.0.0.0/0", Gateway: "192.0.2.1", Interface: "testwan0", OnLink: true},
		{Family: "ipv6", Table: 10000, Destination: "::/0", Gateway: "2001:db8:2::1", Interface: "testwan0", Metric: 1024, OnLink: true},
		{Family: "ipv4", Table: 10001, Destination: "203.0.113.0/24", Interface: "testlan0", Scope: 253},
		{Family: "ipv6", Table: 10001, Destination: "2001:db8:3::/64", Interface: "testlan0", Metric: 1024},
	})
	if !reflect.DeepEqual(s, want) {
		t.Fatalf("got %+v want %+v", s, want)
	}
	after, _ := json.Marshal(c)
	if string(before) != string(after) {
		t.Fatal("changed canonical input")
	}
	if _, err := PrivateRoutesForConfig(c, []int{10000}); err == nil {
		t.Fatal("silently omitted unenrolled route")
	}
	if _, err := PrivateRoutesForConfig(c, []int{254, 10000, 10001}); err == nil {
		t.Fatal("accepted main table ownership")
	}
}

func TestIPv6ZeroMetricMatchesRenderValidationAndSimulation(t *testing.T) {
	c, q := staticTrafficFixture()
	c.Interfaces[0].Addresses = append(c.Interfaces[0].Addresses, "2001:db8:2::2/64")
	c.Routes = append(c.Routes,
		Route{ID: "v6-default-metric", Interface: "uplink", Destination: "::/0", Gateway: "2001:db8:2::1", Table: 254},
		Route{ID: "v6-preferred", Interface: "uplink", Destination: "::/0", Gateway: "2001:db8:2::3", Table: 254, Metric: 100})
	plan, err := Render(c)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plan.Networkd["10-duckos-uplink.network"], "Destination=::/0\nMetric=1024\n") {
		t.Fatal("render lost effective metric")
	}
	q.Source, q.Destination = "2001:db8:1::20", "2001:db8:4::20"
	e, err := Simulate(c, q)
	if err != nil || e.Gateway != "2001:db8:2::3" {
		t.Fatalf("incorrect IPv6 route choice: %+v %v", e, err)
	}
	c.Routes[len(c.Routes)-1].Metric = 1024
	if err := c.Validate(); err == nil {
		t.Fatal("accepted two equal effective IPv6 route metrics")
	}
}

func TestRecoveryRouteRequiresEffectiveIPv6Metric(t *testing.T) {
	r := KernelRoute{Family: "ipv6", Table: 10000, Destination: "::/0", Interface: "testwan0"}
	if _, err := normalizeRouteSnapshot([]int{10000}, []KernelRoute{r}); err == nil {
		t.Fatal("accepted non-reproducible zero metric")
	}
	r.Metric = 1024
	if _, err := normalizeRouteSnapshot([]int{10000}, []KernelRoute{r}); err != nil {
		t.Fatal(err)
	}
}
