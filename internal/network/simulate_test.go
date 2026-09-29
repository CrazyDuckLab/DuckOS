package network

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func staticTrafficFixture() (Config, TrafficQuestion) {
	c := fixture()
	c.Interfaces[0].DHCPClient = "none"
	return c, TrafficQuestion{Ingress: "inside", Source: "198.51.100.20", Destination: "203.0.113.80", Protocol: "tcp", Port: 443, ConnectionState: "new"}
}
func hasTrafficStep(e TrafficExplanation, stage, code, id string) bool {
	for _, step := range e.Steps {
		if step.Stage == stage && step.Code == code && step.ObjectID == id {
			return true
		}
	}
	return false
}
func TestSimulateNewFlowAndNAT(t *testing.T) {
	c, q := staticTrafficFixture()
	before, _ := json.Marshal(c)
	e, err := Simulate(c, q)
	if err != nil || e.Status != "conditional" || e.ManagedDecision != "allow" || e.Reachability != "not-tested" || e.Egress != "uplink" || e.Gateway != "192.0.2.1" || len(e.ConfigDigest) != 64 || len(e.Assumptions) < 4 {
		t.Fatal(e, err)
	}
	if !hasTrafficStep(e, "firewall", "accept", "internet") || !hasTrafficStep(e, "snat", "masquerade", "uplink") {
		t.Fatal(e)
	}
	again, err := Simulate(c, q)
	if err != nil || !reflect.DeepEqual(e, again) {
		t.Fatal("nondeterministic simulation")
	}
	after, _ := json.Marshal(c)
	if string(before) != string(after) {
		t.Fatal("simulation changed candidate")
	}
	c.Firewall.Rules[0].Action = "drop"
	dropped, err := Simulate(c, q)
	if err != nil || dropped.ManagedDecision != "drop" || dropped.ConfigDigest == e.ConfigDigest || hasTrafficStep(dropped, "snat", "masquerade", "uplink") {
		t.Fatal(dropped, err)
	}
}

func TestSimulationUsesDNATBeforeRouteAndFilter(t *testing.T) {
	c, q := staticTrafficFixture()
	q.Ingress, q.Source, q.Destination, q.Port = "uplink", "203.0.113.44", "192.0.2.2", 8443
	e, err := Simulate(c, q)
	if err != nil || e.ManagedDecision != "allow" || e.Destination != "198.51.100.10" || e.Port != 443 || e.Egress != "inside" || !hasTrafficStep(e, "firewall", "accept", "website") {
		t.Fatal(e, err)
	}
	dnat, route, firewall := -1, -1, -1
	for i, s := range e.Steps {
		switch s.Stage {
		case "dnat":
			dnat = i
		case "route":
			route = i
		case "firewall":
			firewall = i
		}
	}
	if !(dnat < route && route < firewall) {
		t.Fatal("incorrect hook order", e)
	}
	c.Firewall.Rules = append([]Rule{{ID: "block-web", From: "uplink", To: "inside", Protocol: "tcp", DestinationPort: 443, Action: "drop"}}, c.Firewall.Rules...)
	e, err = Simulate(c, q)
	if err != nil || e.ManagedDecision != "drop" || !hasTrafficStep(e, "firewall", "drop", "block-web") {
		t.Fatal("DNAT bypassed earlier firewall rule", e, err)
	}
	plan, err := Render(c)
	if err != nil || strings.Index(plan.NFTables, "tcp dport 443 counter drop") > strings.Index(plan.NFTables, "ct status dnat") {
		t.Fatal("renderer/evaluator rule order differs", err)
	}
}

func TestSimulationPrivatePolicyNeverFallsBack(t *testing.T) {
	c, q := staticTrafficFixture()
	c.Routes = append(c.Routes, Route{ID: "private-scope", Interface: "uplink", Destination: "203.0.113.0/25", Gateway: "192.0.2.1", Table: 10000})
	c.PolicyRules = []PolicyRule{{ID: "client-path", Family: "ipv4", From: "198.51.100.20/32", Priority: 20000, Table: 10000}}
	e, err := Simulate(c, q)
	if err != nil || e.ManagedDecision != "allow" || !hasTrafficStep(e, "policy-routing", "table-lookup", "client-path") {
		t.Fatal(e, err)
	}
	q.Destination = "203.0.113.200"
	e, err = Simulate(c, q)
	if err != nil || e.ManagedDecision != "drop" || e.Egress != "" || !hasTrafficStep(e, "policy-routing", "unreachable-guard", "client-path") {
		t.Fatal("private miss leaked to main", e, err)
	}
	q.Destination = "192.0.2.88"
	e, err = Simulate(c, q)
	if err != nil || e.ManagedDecision != "allow" || !hasTrafficStep(e, "policy-routing", "table-lookup", "connected:192.0.2.0/24") || hasTrafficStep(e, "policy-routing", "table-lookup", "client-path") {
		t.Fatal("connected protection order lost", e, err)
	}
}

func TestSimulationUncertaintyIsNotPermission(t *testing.T) {
	tests := map[string]func(*Config, *TrafficQuestion){
		"dynamic IPv4 routes": func(c *Config, q *TrafficQuestion) { c.Interfaces[0].DHCPClient = "ipv4" },
		"dynamic local DNAT": func(c *Config, q *TrafficQuestion) {
			c.Interfaces[0].DHCPClient = "ipv4"
			q.Ingress = "uplink"
			q.Source = "203.0.113.5"
			q.Port = 8443
		},
		"unspecified port before any": func(c *Config, q *TrafficQuestion) {
			q.Port = 0
			c.Firewall.Rules = append([]Rule{{ID: "port-block", From: "inside", To: "uplink", Protocol: "tcp", DestinationPort: 22, Action: "drop"}}, c.Firewall.Rules...)
		},
		"scoped IPv6":     func(c *Config, q *TrafficQuestion) { q.Source = "fe80::1"; q.Destination = "fe80::2" },
		"broadcast":       func(c *Config, q *TrafficQuestion) { q.Destination = "198.51.100.255" },
		"network address": func(c *Config, q *TrafficQuestion) { q.Destination = "198.51.100.0" },
		"local source":    func(c *Config, q *TrafficQuestion) { q.Source = "198.51.100.1" },
		"connected metric ambiguity": func(c *Config, q *TrafficQuestion) {
			q.Destination = "192.0.2.9"
			c.Routes = append(c.Routes, Route{ID: "overlap-route", Interface: "uplink", Destination: "192.0.2.0/24", Table: 254, Metric: 5})
		},
	}
	for name, change := range tests {
		t.Run(name, func(t *testing.T) {
			c, q := staticTrafficFixture()
			change(&c, &q)
			e, err := Simulate(c, q)
			if err != nil || e.Status != "unknown" || e.ManagedDecision != "unknown" {
				t.Fatal(e, err)
			}
		})
	}
}

func TestSimulationLocalAndExistingFlowsAreNotForwardPredictions(t *testing.T) {
	for _, state := range []string{"established", "related", "invalid", "new"} {
		c, q := staticTrafficFixture()
		q.ConnectionState = state
		if state == "new" {
			q.Destination = "198.51.100.1"
		}
		e, err := Simulate(c, q)
		if err != nil || e.Status != "unsupported" || e.ManagedDecision != "unknown" {
			t.Fatal(e, err)
		}
	}
}

func TestSimulationLongestPrefixThenMetricAndDefaultDrop(t *testing.T) {
	c, q := staticTrafficFixture()
	c.Routes = append(c.Routes, Route{ID: "specific-expensive", Interface: "uplink", Destination: "203.0.113.0/24", Gateway: "192.0.2.1", Table: 254, Metric: 50}, Route{ID: "specific-cheap", Interface: "uplink", Destination: "203.0.113.0/24", Gateway: "192.0.2.3", Table: 254, Metric: 20})
	e, err := Simulate(c, q)
	if err != nil || e.Gateway != "192.0.2.3" || !hasTrafficStep(e, "route", "selected", "specific-cheap") {
		t.Fatal(e, err)
	}
	c.Firewall.Rules = nil
	e, err = Simulate(c, q)
	if err != nil || e.ManagedDecision != "drop" || !hasTrafficStep(e, "firewall", "default-drop", "") {
		t.Fatal(e, err)
	}
	c.Routes = nil
	e, err = Simulate(c, q)
	if err != nil || e.ManagedDecision != "drop" || e.Egress != "" {
		t.Fatal(e, err)
	}
}

func TestSimulationIPv6AndOptionalPort(t *testing.T) {
	c, q := staticTrafficFixture()
	c.Interfaces[0].Addresses = append(c.Interfaces[0].Addresses, "2001:db8:2::2/64")
	c.Routes = append(c.Routes, Route{ID: "v6-default", Interface: "uplink", Destination: "::/0", Gateway: "2001:db8:2::1", Table: 254})
	q.Source, q.Destination, q.Protocol, q.Port = "2001:db8:1::20", "2001:db8:3::20", "ipv6-icmp", 0
	e, err := Simulate(c, q)
	if err != nil || e.ManagedDecision != "allow" || hasTrafficStep(e, "snat", "masquerade", "uplink") {
		t.Fatal(e, err)
	}
	c.Interfaces[0].AcceptRA = true
	e, err = Simulate(c, q)
	if err != nil || e.Status != "unknown" {
		t.Fatal(e, err)
	}
	c, q = staticTrafficFixture()
	q.Port = 0
	e, err = Simulate(c, q)
	if err != nil || e.ManagedDecision != "allow" {
		t.Fatal("port-independent policy needs no port", e, err)
	}
}

func TestSimulationRejectsMalformedQuestions(t *testing.T) {
	for _, change := range []func(*TrafficQuestion){
		func(q *TrafficQuestion) { q.Destination = "example.test" }, func(q *TrafficQuestion) { q.Source = "::ffff:198.51.100.20" },
		func(q *TrafficQuestion) { q.Ingress = "missing" }, func(q *TrafficQuestion) { q.Port = 65536 }, func(q *TrafficQuestion) { q.Protocol = "tcp; accept" },
		func(q *TrafficQuestion) { q.ConnectionState = "" }, func(q *TrafficQuestion) { q.Protocol = "icmp" },
	} {
		c, q := staticTrafficFixture()
		change(&q)
		if _, err := Simulate(c, q); err == nil {
			t.Fatal("invalid question accepted", q)
		}
	}
}
