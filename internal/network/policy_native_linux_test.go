//go:build linux

package network

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// This is RPDB semantics evidence, not networkd lifecycle qualification.
// It runs only inside the explicitly opted-in disposable network namespace.
func TestPolicyNativeMissingTableCannotFallThrough(t *testing.T) {
	requireIsolatedNetworkNamespace(t)
	call := func(args ...string) ([]byte, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "/usr/sbin/ip", args...)
		cmd.Env = []string{"PATH=/usr/sbin:/usr/bin", "LANG=C", "LC_ALL=C"}
		cmd.WaitDelay = time.Second
		return cmd.CombinedOutput()
	}
	run := func(args ...string) {
		t.Helper()
		if out, err := call(args...); err != nil {
			t.Fatalf("native ip %v: %v: %s", args, err, out)
		}
	}
	// Adding an existing name fails without modifying it. Cleanup is armed only
	// after successful creation of this test-owned virtual interface.
	run("link", "add", "duckpolicy0", "type", "dummy")
	defer func() {
		if out, err := call("link", "delete", "duckpolicy0"); err != nil {
			t.Errorf("native cleanup: %v: %s", err, out)
		}
	}()
	run("link", "set", "duckpolicy0", "up")
	run("address", "add", "198.51.100.10/32", "dev", "duckpolicy0")
	run("-4", "route", "add", "default", "dev", "duckpolicy0", "table", "main")
	run("-4", "route", "add", "default", "dev", "duckpolicy0", "table", "10000")
	plan, err := Render(policyFixture())
	if err != nil {
		t.Fatal(err)
	}
	// Translate the generated, allowlisted rule fields to ip for the kernel
	// semantic test. No arbitrary command or configuration input is accepted.
	for _, section := range strings.Split(plan.Networkd["10-duckos-inside.network"], "\n[RoutingPolicyRule]\n")[1:] {
		fields := map[string]string{}
		for _, line := range strings.Split(strings.TrimSpace(section), "\n") {
			key, value, ok := strings.Cut(line, "=")
			if !ok {
				t.Fatal("unexpected generated rule line", line)
			}
			fields[key] = value
		}
		family := "-4"
		if fields["Family"] == "ipv6" {
			family = "-6"
		}
		args := []string{family, "rule", "add", "priority", fields["Priority"]}
		for _, pair := range [][2]string{{"From", "from"}, {"To", "to"}, {"Table", "lookup"}, {"SuppressPrefixLength", "suppress_prefixlength"}} {
			if value := fields[pair[0]]; value != "" {
				args = append(args, pair[1], value)
			}
		}
		if fields["Type"] == "unreachable" {
			args = append(args, "unreachable")
		}
		run(args...)
	}
	get := []string{"-4", "route", "get", "203.0.113.10", "from", "198.51.100.10"}
	if out, err := call(get...); err != nil {
		t.Fatalf("selected table not usable before removal: %v: %s", err, out)
	}
	candidate := policyFixture()
	candidate.Interfaces[0].DHCPClient = "none"
	question := TrafficQuestion{Ingress: "inside", Source: "198.51.100.10", Destination: "203.0.113.10", Protocol: "tcp", Port: 443, ConnectionState: "new"}
	explanation, err := Simulate(candidate, question)
	if err != nil || explanation.ManagedDecision != "allow" || !hasTrafficStep(explanation, "policy-routing", "table-lookup", "staff-egress") {
		t.Fatal("candidate evaluator disagreed with selected-table kernel lookup", explanation, err)
	}
	run("-4", "route", "delete", "default", "dev", "duckpolicy0", "table", "10000")
	// Keep a valid but nonmatching private route in both candidate and kernel.
	// The question must hit the generated guard, not the still-working main default.
	run("-4", "route", "add", "203.0.113.99/32", "dev", "duckpolicy0", "table", "10000")
	candidate.Routes[1].Destination = "203.0.113.99/32"
	explanation, err = Simulate(candidate, question)
	if err != nil || explanation.ManagedDecision != "drop" || !hasTrafficStep(explanation, "policy-routing", "unreachable-guard", "staff-egress") {
		t.Fatal("candidate evaluator failed to model the private-table miss", explanation, err)
	}
	if out, err := call(get...); err == nil {
		t.Fatalf("empty selected table leaked to main: %s", out)
	}
	// Main remains usable: only removing the generated guard permits fallback.
	run("-4", "rule", "delete", "priority", "20001")
	if out, err := call(get...); err != nil {
		t.Fatalf("test did not establish a working fallback route: %v: %s", err, out)
	}
}
