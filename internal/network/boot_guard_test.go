package network

import (
	"context"
	"errors"
	"strings"
	"testing"

	serviceunit "github.com/CrazyDuckLab/DuckOS/packaging/systemd"
)

func bootGuardObservation() string {
	return "LoadState=loaded\nActiveState=active\nSubState=exited\nFragmentPath=" + routingBootUnitPath + "\nDropInPaths=\nNeedDaemonReload=no\nUnitFileState=enabled\nExecMainStatus=0\nUser=root\nGroup=root\n"
}
func bootGuardSettledObservation() string {
	// Real-world state for a oneshot that has finished: systemd reports
	// ActiveState=inactive + SubState=dead once ExecMainStatus is the
	// terminal code and the unit is no longer reaped.
	return "LoadState=loaded\nActiveState=inactive\nSubState=dead\nFragmentPath=" + routingBootUnitPath + "\nDropInPaths=\nNeedDaemonReload=no\nUnitFileState=enabled\nExecMainStatus=0\nUser=root\nGroup=root\n"
}
func networkdBootObservation() string {
	return "LoadState=loaded\nActiveState=active\nNeedDaemonReload=no\nRequires=" + routingBootUnit + "\nAfter=local-fs.target " + routingBootUnit + "\n"
}
func networkdBootInactiveObservation() string {
	// Pre-first-Apply state: eth0 is externally managed, eth1/eth2 are
	// unmanaged, so systemd-networkd has no link to manage and goes
	// inactive. The boot ordering invariant under test is the Requires/
	// After wiring, not networkd's activeness.
	return "LoadState=loaded\nActiveState=inactive\nNeedDaemonReload=no\nRequires=" + routingBootUnit + "\nAfter=local-fs.target " + routingBootUnit + "\n"
}
func TestBootGuardRequiresLoadedSuccessfulPersistentOrdering(t *testing.T) {
	for _, mode := range []string{"valid", "modified-unit", "missing-dependency", "failed", "settled-oneshot", "runtime-enabled", "override", "stale", "missing-order", "missing-requirement", "duplicate", "missing-field", "networkd-pre-apply"} {
		t.Run(mode, func(t *testing.T) {
			g := routingBootGuard{
				readUnit: func() ([]byte, error) {
					raw := serviceunit.NetworkRecovery()
					if mode == "modified-unit" {
						raw += "\n"
					}
					return []byte(raw), nil
				},
				dependency: func() error {
					if mode == "missing-dependency" {
						return errors.New("absent")
					}
					return nil
				},
			}
			calls := 0
			g.run = func(_ context.Context, args []string) ([]byte, error) {
				calls++
				if len(args) != 6 || args[0] != "--no-pager" || args[1] != "--no-ask-password" || args[2] != "--all" || args[3] != "show" {
					t.Fatalf("unexpected command %v", args)
				}
				raw := bootGuardObservation()
				if args[4] == "systemd-networkd.service" {
					raw = networkdBootObservation()
					if mode == "networkd-pre-apply" {
						raw = networkdBootInactiveObservation()
					}
					if mode == "missing-order" {
						raw = strings.ReplaceAll(raw, "After=local-fs.target "+routingBootUnit, "After=local-fs.target")
					}
					if mode == "missing-requirement" {
						raw = strings.ReplaceAll(raw, "Requires="+routingBootUnit, "Requires=")
					}
				} else if args[4] != routingBootUnit {
					t.Fatal("unexpected unit")
				}
				switch mode {
				case "failed":
					raw = strings.ReplaceAll(raw, "ExecMainStatus=0", "ExecMainStatus=1")
				case "settled-oneshot":
					if args[4] == routingBootUnit {
						raw = bootGuardSettledObservation()
					}
				case "runtime-enabled":
					raw = strings.ReplaceAll(raw, "UnitFileState=enabled", "UnitFileState=enabled-runtime")
				case "override":
					raw = strings.ReplaceAll(raw, "DropInPaths=", "DropInPaths=/etc/override.conf")
				case "stale":
					raw = strings.ReplaceAll(raw, "NeedDaemonReload=no", "NeedDaemonReload=yes")
				case "duplicate":
					raw += "LoadState=loaded\n"
				case "missing-field":
					raw = strings.ReplaceAll(raw, "DropInPaths=\n", "")
				}
				return []byte(raw), nil
			}
			err := g.verify(context.Background())
			accept := mode == "valid" || mode == "settled-oneshot" || mode == "networkd-pre-apply"
			if (err == nil) != accept {
				t.Fatalf("mode %s: %v", mode, err)
			}
			if (mode == "modified-unit" || mode == "missing-dependency") && calls != 0 {
				t.Fatal("ran command before validating unit/dependency")
			}
		})
	}
}
