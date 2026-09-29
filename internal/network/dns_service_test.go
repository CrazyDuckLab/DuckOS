package network

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	serviceunit "github.com/CrazyDuckLab/DuckOS/packaging/systemd"
)

func dnsObservation(active bool) string {
	state := "ActiveState=inactive\nSubState=dead\nMainPID=0\n"
	if active {
		state = "ActiveState=active\nSubState=running\nMainPID=123\n"
	}
	return state + "LoadState=loaded\nFragmentPath=" + dnsUnitPath + "\nDropInPaths=\nNeedDaemonReload=no\nUnitFileState=disabled\nUser=duck-dnsmasq\nGroup=duck-dnsmasq\n"
}

func TestDNSServiceObservationRejectsAmbiguousOwnership(t *testing.T) {
	for _, active := range []bool{false, true} {
		got, err := decodeDNSService([]byte(dnsObservation(active)))
		if err != nil || got != (DNSServiceSnapshot{Version: 1, Active: active}) {
			t.Fatalf("valid observation: %+v %v", got, err)
		}
	}
	base := dnsObservation(false)
	cases := []string{
		base + "DropInPaths=\n", base + "Unexpected=value\n",
		strings.ReplaceAll(base, "DropInPaths=", "DropInPaths=/etc/override.conf"),
		strings.ReplaceAll(base, "NeedDaemonReload=no", "NeedDaemonReload=yes"),
		strings.ReplaceAll(base, "User=duck-dnsmasq", "User=root"),
		strings.ReplaceAll(base, dnsUnitPath, "/usr/lib/systemd/system/dnsmasq.service"),
		strings.ReplaceAll(base, "UnitFileState=disabled", "UnitFileState=masked"),
		strings.ReplaceAll(base, "ActiveState=inactive", "ActiveState=failed"),
		strings.ReplaceAll(base, "SubState=dead", "SubState=start"),
		strings.ReplaceAll(base, "MainPID=0", "MainPID=00"),
		strings.ReplaceAll(base, "MainPID=0", "MainPID=123"),
		strings.ReplaceAll(base, "DropInPaths=\n", ""), strings.Repeat("x", 16385),
	}
	for i, raw := range cases {
		if _, err := decodeDNSService([]byte(raw)); err == nil {
			t.Errorf("accepted invalid observation %d", i)
		}
	}
}

func TestDNSServiceLifecycleUsesOnlyFixedUnit(t *testing.T) {
	active := false
	var mutations []string
	h := &DNSServiceHost{readUnit: func() ([]byte, error) { return []byte(serviceunit.DNSMasq()), nil }}
	h.run = func(ctx context.Context, args []string) ([]byte, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if len(args) == 6 && args[2] == "--all" && args[3] == "show" && args[4] == dnsUnitName {
			return []byte(dnsObservation(active)), nil
		}
		if len(args) != 4 || args[0] != "--no-pager" || args[1] != "--no-ask-password" || args[3] != dnsUnitName {
			t.Fatalf("unexpected command: %v", args)
		}
		switch args[2] {
		case "start":
			active = true
		case "stop":
			active = false
		default:
			t.Fatalf("unexpected mutation: %v", args)
		}
		mutations = append(mutations, args[2])
		return nil, nil
	}
	before, err := h.Capture(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	after, err := h.SetActive(context.Background(), before, true)
	if err != nil || !after.Active {
		t.Fatalf("start: %+v %v", after, err)
	}
	if _, err := h.SetActive(context.Background(), before, false); err == nil {
		t.Fatal("accepted stale state")
	}
	if _, err := h.SetActive(context.Background(), after, true); err != nil {
		t.Fatal(err)
	}
	if _, err := h.SetActive(context.Background(), after, false); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(mutations, []string{"start", "stop"}) {
		t.Fatal(mutations)
	}
}

func TestDNSServiceDoesNotClaimSkippedOrUncertainStart(t *testing.T) {
	for _, lostAck := range []bool{false, true} {
		h := &DNSServiceHost{readUnit: func() ([]byte, error) { return []byte(serviceunit.DNSMasq()), nil }}
		h.run = func(_ context.Context, args []string) ([]byte, error) {
			if len(args) == 6 {
				return []byte(dnsObservation(false)), nil
			}
			if lostAck {
				return nil, errors.New("lost acknowledgement")
			}
			return nil, nil
		}
		if _, err := h.SetActive(context.Background(), DNSServiceSnapshot{Version: 1}, true); err == nil {
			t.Fatal("claimed successful start")
		}
	}
}

func TestDNSServiceRejectsModifiedUnitBeforeCommand(t *testing.T) {
	h := &DNSServiceHost{
		readUnit: func() ([]byte, error) { return []byte(serviceunit.DNSMasq() + "\n"), nil },
		run: func(context.Context, []string) ([]byte, error) {
			t.Fatal("command ran for modified unit")
			return nil, nil
		},
	}
	if _, err := h.Capture(context.Background()); err == nil {
		t.Fatal("accepted modified unit")
	}
}

func TestDNSServiceRecoveryStopsFailedUnitWithoutResettingFailure(t *testing.T) {
	failed := strings.ReplaceAll(strings.ReplaceAll(dnsObservation(false), "ActiveState=inactive", "ActiveState=failed"), "SubState=dead", "SubState=failed")
	for _, state := range []string{dnsObservation(false), dnsObservation(true), failed} {
		current := state
		stops := 0
		h := &DNSServiceHost{readUnit: func() ([]byte, error) { return []byte(serviceunit.DNSMasq()), nil }}
		h.run = func(_ context.Context, args []string) ([]byte, error) {
			if len(args) == 6 {
				return []byte(current), nil
			}
			if !reflect.DeepEqual(args, []string{"--no-pager", "--no-ask-password", "stop", dnsUnitName}) {
				t.Fatalf("unexpected recovery command: %v", args)
			}
			stops++
			if current != failed {
				current = dnsObservation(false)
			}
			return nil, nil
		}
		if err := h.QuiesceForRecovery(context.Background(), DNSServiceSnapshot{Version: 1}); err != nil {
			t.Fatal(err)
		}
		if stops != 1 {
			t.Fatalf("stop count %d", stops)
		}
		if state == failed {
			if _, err := h.Capture(context.Background()); err == nil {
				t.Fatal("failure became a stable before-state")
			}
		}
	}
}

func TestDNSServiceRecoveryRejectsDriftAndIncompleteStop(t *testing.T) {
	for _, mode := range []string{"enablement", "transition", "failed-process", "lost-ack", "still-active"} {
		t.Run(mode, func(t *testing.T) {
			state := dnsObservation(true)
			switch mode {
			case "enablement":
				state = strings.ReplaceAll(state, "UnitFileState=disabled", "UnitFileState=enabled")
			case "transition":
				state = strings.ReplaceAll(state, "ActiveState=active", "ActiveState=deactivating")
			case "failed-process":
				state = strings.ReplaceAll(strings.ReplaceAll(state, "ActiveState=active", "ActiveState=failed"), "SubState=running", "SubState=failed")
			}
			mutated := false
			h := &DNSServiceHost{readUnit: func() ([]byte, error) { return []byte(serviceunit.DNSMasq()), nil }}
			h.run = func(_ context.Context, args []string) ([]byte, error) {
				if len(args) == 6 {
					return []byte(state), nil
				}
				mutated = true
				if mode == "lost-ack" {
					return nil, errors.New("unknown outcome")
				}
				return nil, nil
			}
			if err := h.QuiesceForRecovery(context.Background(), DNSServiceSnapshot{Version: 1}); err == nil {
				t.Fatal("accepted unsafe recovery")
			}
			if mode != "lost-ack" && mode != "still-active" && mutated {
				t.Fatal("mutated before rejecting drift")
			}
		})
	}
}
