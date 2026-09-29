//go:build linux

package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/network"
	"github.com/CrazyDuckLab/DuckOS/internal/state"
)

func TestFirstBootRecoveryBeforeAgent(t *testing.T) {
	if os.Geteuid() != 0 || os.Getenv("DUCKOS_FIRST_BOOT_WITNESS") != "1" {
		t.Skip("requires opted-in isolated Linux root witness")
	}
	if os.Getenv("DUCKOS_FIRST_BOOT_CHILD") != "1" {
		cmd := exec.Command("unshare", "--mount", "--net", "--fork", os.Args[0], "-test.run=^TestFirstBootRecoveryBeforeAgent$", "-test.v")
		cmd.Env = append(os.Environ(), "DUCKOS_FIRST_BOOT_CHILD=1")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("isolated recovery: %v\n%s", err, out)
		}
		return
	}
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(syscall.Mount("", "/", "", syscall.MS_REC|syscall.MS_PRIVATE, ""))
	must(syscall.Mount("tmpfs", "/var/lib/duckos-agent", "tmpfs", 0, "mode=0700"))
	must(syscall.Mount("tmpfs", "/etc/systemd/network", "tmpfs", 0, "mode=0755"))
	must(os.Mkdir("/var/lib/duckos-agent/network", 0700))
	e := network.Enrollment{Version: 1, CreatedAt: time.Now(), FirewallTable: "inet duckos", NetworkdReload: true, PolicyRouting: true, RoutingTables: []int{10000}, Physical: []network.PhysicalBinding{{ID: "eth0", Name: "eth0", MAC: "02:00:00:00:00:01", ExternallyManaged: true}}}
	raw, err := json.Marshal(e)
	must(err)
	must(os.WriteFile("/var/lib/duckos-agent/enrollment.json", raw, 0600))
	if err := recoverNetworkArtifacts(); err == nil {
		t.Fatal("missing journal without installer sentinel admitted")
	}
	if _, err := os.Stat("/var/lib/duckos-agent/network/state.json"); !os.IsNotExist(err) {
		t.Fatal("unauthorized initialization", err)
	}
	must(os.WriteFile("/var/lib/duckos-agent/network/.installer-prepared", nil, 0600))
	must(recoverNetworkArtifacts())
	s, err := state.OpenExisting("/var/lib/duckos-agent/network", func(raw json.RawMessage) error { _, err := network.Decode(raw); return err })
	must(err)
	snapshot, err := s.Snapshot()
	must(err)
	must(s.Close())
	cfg, err := network.Decode(snapshot.Current.Config)
	must(err)
	if len(cfg.Interfaces) != 0 || snapshot.Pending != nil {
		t.Fatal("first boot must be empty without pending mutation")
	}
	entries, err := os.ReadDir("/etc/systemd/network")
	must(err)
	if len(entries) != 0 {
		t.Fatal("first-boot recovery wrote network profiles")
	}
	must(recoverNetworkArtifacts())
}
