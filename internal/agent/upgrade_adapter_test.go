package agent

import (
	"testing"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/upgrade"
)

func TestActivationHealthRunnerRequiresActiveRelease(t *testing.T) {
	o := upgrade.NewOrchestrator(&upgradeStoreStub{}, nil, "amd64", func() time.Time { return time.Unix(1, 0) })
	runner := activationHealthRunner{orchestrator: o}
	if err := runner.Run(upgrade.HealthCheck{Name: "active-release", Command: "status"}); err == nil {
		t.Fatal("activation health runner accepted empty active release")
	}
	o.SetState(upgrade.State{Channel: upgrade.ChannelStable, Policy: upgrade.PolicyNotify, Active: "rel-1", UpdatedAt: time.Unix(1, 0)})
	if err := runner.Run(upgrade.HealthCheck{Name: "active-release", Command: "status"}); err == nil {
		t.Fatal("activation health runner accepted metadata without runtime evidence")
	}
	if err := runner.Run(upgrade.HealthCheck{Name: "active-release", Command: "unsupported"}); err == nil {
		t.Fatal("activation health runner accepted unsupported command")
	}
}

type upgradeStoreStub struct{}

func (*upgradeStoreStub) Materialize(upgrade.ReleaseMeta, []byte) error { return nil }
func (*upgradeStoreStub) Activate(string) error                         { return nil }
func (*upgradeStoreStub) List() ([]string, error)                       { return []string{}, nil }
func (*upgradeStoreStub) ReadMeta(string) (upgrade.ReleaseMeta, error) {
	return upgrade.ReleaseMeta{}, nil
}
func (*upgradeStoreStub) ReadPayload(string) ([]byte, error) { return nil, nil }
func (*upgradeStoreStub) Remove(string) error                { return nil }
func (*upgradeStoreStub) FreeBytes() (int64, error)          { return 1 << 20, nil }
func (*upgradeStoreStub) VerifyFingerprint(string) error     { return nil }
