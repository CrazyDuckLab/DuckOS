package agent

import (
	"testing"

	"github.com/CrazyDuckLab/DuckOS/internal/upgrade"
)

func TestH08SelectionIsNotRuntimeHealth(t *testing.T) {
	for _, result := range []string{"activated", "health-ok", "staged"} {
		if computeHealthy(upgrade.State{Active: "candidate"}, []upgrade.HistoryEntry{{Result: result}}) {
			t.Fatalf("metadata-only %q claimed health", result)
		}
	}
	o := upgrade.NewOrchestrator(nil, nil, "amd64", nil)
	o.SetState(upgrade.State{Active: "candidate"})
	if err := (activationHealthRunner{orchestrator: o}).Run(upgrade.HealthCheck{Name: "runtime", Command: "status"}); err == nil {
		t.Fatal("unwired runtime provider claimed success")
	}
}
