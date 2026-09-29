package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/network"
)

func TestEmptyFirstBootBaselineDoesNotReloadNetworkManager(t *testing.T) {
	if runtime.GOOS != "linux" || os.Geteuid() != 0 {
		t.Skip("requires Linux root entry path; command is intercepted")
	}
	dir := t.TempDir()
	marker := filepath.Join(dir, "called")
	if err := os.WriteFile(filepath.Join(dir, "networkctl"), []byte("#!/bin/sh\n: > \"$DUCKOS_TEST_MARKER\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("DUCKOS_TEST_MARKER", marker)
	if err := persistBaselineArtifacts(json.RawMessage(`{"version":1}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("empty baseline invoked manager-wide reload", err)
	}
}

// TestBuildFirstBootBaselineConfigSkipsExternallyManagedBindings pins the
// first-boot baseline invariant: the durable journal seeded from the
// installer-prepared sentinel MUST NOT describe any physical binding as
// a DuckOS-managed interface, because the downstream networkd renderer
// would emit a 10-duckos-<iface>.network file for each binding in the
// durable intent. That file would claim a link currently owned by
// another network manager (typically dhcpcd for the management NIC) and
// the foreign manager's running state would be lost.
//
// The first-boot baseline is therefore intentionally empty: an empty
// Interfaces slice is a documented coherent state that the renderer
// treats as "destroy the duckos nftables table and emit no networkd
// profiles". The network service is admitted, the durable journal
// opens, and the operator populates the durable intent via Safe Apply.
//
// The same scope-isolation invariant is independently pinned by
// internal/network/scope_isolation_test.go for the Apply/Restore path.
func TestBuildFirstBootBaselineConfigSkipsExternallyManagedBindings(t *testing.T) {
	enr := network.Enrollment{
		Version:       1,
		CreatedAt:     time.Now().UTC(),
		FirewallTable: "inet duckos",
		RoutingTables: []int{10000},
		Physical: []network.PhysicalBinding{
			{ID: "eth0", Name: "eth0", MAC: "00:15:5d:04:07:07", ExternallyManaged: true},
			{ID: "eth1", Name: "eth1", MAC: "00:15:5d:04:07:0e"},
			{ID: "eth2", Name: "eth2", MAC: "00:15:5d:04:07:0f"},
		},
	}
	raw, err := buildFirstBootBaselineConfig(enr)
	if err != nil {
		t.Fatalf("buildFirstBootBaselineConfig: %v", err)
	}
	var cfg struct {
		Version    int              `json:"version"`
		Interfaces []map[string]any `json:"interfaces"`
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("decode baseline: %v", err)
	}
	if len(cfg.Interfaces) != 0 {
		t.Fatalf("first-boot baseline admitted %d managed interfaces; want 0 so the renderer emits no networkd profiles", len(cfg.Interfaces))
	}
}

// TestBuildFirstBootBaselineConfigAllExternallyManagedStillValidates pins the
// degenerate case where every physical binding is externally managed (a host
// enrolled only to claim cross-manager identity, not to manage any link).
// The baseline must collapse to zero managed interfaces so the renderer
// degrades to its documented empty-config path; it must NOT panic.
func TestBuildFirstBootBaselineConfigAllExternallyManagedStillValidates(t *testing.T) {
	enr := network.Enrollment{
		Version:       1,
		CreatedAt:     time.Now().UTC(),
		FirewallTable: "inet duckos",
		RoutingTables: []int{10000},
		Physical: []network.PhysicalBinding{
			{ID: "eth0", Name: "eth0", MAC: "00:15:5d:04:07:07", ExternallyManaged: true},
		},
	}
	raw, err := buildFirstBootBaselineConfig(enr)
	if err != nil {
		t.Fatalf("buildFirstBootBaselineConfig: %v", err)
	}
	var cfg struct {
		Interfaces []map[string]any `json:"interfaces"`
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("decode baseline: %v", err)
	}
	if len(cfg.Interfaces) != 0 {
		t.Fatalf("expected 0 managed interfaces when all bindings are externally managed, got %d", len(cfg.Interfaces))
	}
}
