// Regression coverage for BD-001 — verified coordinator never
// attaches / Safe Apply backend returns 503. The Diff endpoint
// must compute a structured diff when the coordinator is wired
// (fail-open) but refuse with ErrConfigRuntimeUnavailable when
// only the journal is available (fail-closed).
//
// DuckOS Community source is distributed under GPL-3.0-only;
// see the root LICENSE file.
package engine

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func TestDiffWithCoordinatorComputesStructuredDiff(t *testing.T) {
	// initial carries only the fields the Diff/Validation
	// pipeline actually inspects in this test; the rest of
	// the interface schema is irrelevant to the BD-001
	// regression that this test guards. The management
	// interface satisfies the "static management LAN is
	// required" precondition the validated network planner
	// enforces on any non-empty config.
	initial := json.RawMessage(`{"version":1,"interfaces":[{"id":"eth0","name":"eth0","role":"wan","kind":"physical","mtu":1500,"addresses":[],"dhcpClient":"ipv4","acceptRA":false,"management":false},{"id":"eth1-mgmt","name":"eth1","role":"lan","kind":"physical","mtu":1500,"addresses":["10.99.99.1/24"],"dhcpClient":"none","acceptRA":false,"management":true}]}`)
	socket, _ := startFakeAgent(t, revFromRaw(t, initial))
	cfg, _, _ := newWiredConfig(t, socket, initial)

	// Candidate removes the WAN interface, keeps the
	// management LAN so the "static management LAN is
	// required" precondition is still satisfied, and
	// adds a DNS upstream to drive a non-empty finding
	// slice.
	candidate := json.RawMessage(`{"version":1,"interfaces":[{"id":"eth1-mgmt","name":"eth1","role":"lan","kind":"physical","mtu":1500,"addresses":["10.99.99.1/24"],"dhcpClient":"none","acceptRA":false,"management":true}],"firewall":{"rules":[],"forwards":[],"masquerade":[]},"dns":{"upstreams":["1.1.1.1"],"records":[]}}`)
	diff, err := cfg.Diff(context.Background(), ConfigDiffRequest{Candidate: candidate})
	if err != nil {
		t.Fatalf("Diff with coordinator: %v", err)
	}
	if diff.DiffDigest == "" {
		t.Fatal("Diff did not populate DiffDigest")
	}
	if len(diff.Findings) == 0 {
		t.Fatal("Diff did not produce findings; BD-001 must remain closed")
	}
	codes := map[string]bool{}
	for _, f := range diff.Findings {
		if f.Code == "" || f.Severity == "" {
			t.Fatalf("finding missing code or severity: %+v", f)
		}
		codes[f.Code] = true
	}
	if !codes["firewall_changed"] && !codes["interfaces_changed"] {
		t.Fatalf("expected firewall or interfaces finding, got %v", codes)
	}
}

func TestDiffIdenticalCandidateYieldsNoChanges(t *testing.T) {
	initial := json.RawMessage(`{"version":1,"interfaces":[{"id":"eth0","name":"eth0","role":"wan","kind":"physical","mtu":1500,"addresses":[],"dhcpClient":"ipv4","acceptRA":false,"management":false},{"id":"eth1-mgmt","name":"eth1","role":"lan","kind":"physical","mtu":1500,"addresses":["10.99.99.1/24"],"dhcpClient":"none","acceptRA":false,"management":true}]}`)
	socket, _ := startFakeAgent(t, revFromRaw(t, initial))
	cfg, _, _ := newWiredConfig(t, socket, initial)
	candidate := json.RawMessage(`{"version":1,"interfaces":[{"id":"eth0","name":"eth0","role":"wan","kind":"physical","mtu":1500,"addresses":[],"dhcpClient":"ipv4","acceptRA":false,"management":false},{"id":"eth1-mgmt","name":"eth1","role":"lan","kind":"physical","mtu":1500,"addresses":["10.99.99.1/24"],"dhcpClient":"none","acceptRA":false,"management":true}]}`)
	diff, err := cfg.Diff(context.Background(), ConfigDiffRequest{Candidate: candidate})
	if err != nil {
		t.Fatalf("Diff identical: %v", err)
	}
	if len(diff.Findings) != 1 || diff.Findings[0].Code != "no_changes" {
		t.Fatalf("expected single no_changes finding, got %+v", diff.Findings)
	}
}

func TestDiffInvalidCandidateReturns400(t *testing.T) {
	initial := json.RawMessage(`{"version":1}`)
	socket, _ := startFakeAgent(t, revFromRaw(t, initial))
	cfg, _, _ := newWiredConfig(t, socket, initial)
	// Garbage JSON should be refused as a validation failure,
	// not as 503 config_runtime_unavailable.
	_, err := cfg.Diff(context.Background(), ConfigDiffRequest{Candidate: json.RawMessage(`{not json`)})
	if err == nil {
		t.Fatal("expected validation error for garbage candidate")
	}
	if errors.Is(err, ErrConfigRuntimeUnavailable) {
		t.Fatalf("garbage candidate must not be reported as coordinator unavailable: %v", err)
	}
}

// Journal-only Diff (no coordinator) must still refuse — the
// existing torture test asserts this contract; we re-assert
// here for clarity because this is the gate that BD-001
// flipped.
func TestDiffWithoutCoordinatorStillRefuses(t *testing.T) {
	c := freshConfig(t)
	_, err := c.Diff(context.Background(), ConfigDiffRequest{Candidate: json.RawMessage(`{"version":1,"interfaces":[{"id":"x"}]}`)})
	if !errors.Is(err, ErrConfigRuntimeUnavailable) {
		t.Fatalf("journal-only Diff must return ErrConfigRuntimeUnavailable, got %v", err)
	}
}
