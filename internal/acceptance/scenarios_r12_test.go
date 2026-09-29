// scenarios_r12_test.go pins the V1
// 65-scenario registry.
package acceptance

import "testing"

func TestAllScenariosCount(t *testing.T) {
	all := AllScenarios()
	if len(all) < 65 {
		t.Fatalf("expected at least 65 scenarios, got %d", len(all))
	}
	// Ensure every ID is unique.
	seen := map[ScenarioID]bool{}
	for _, id := range all {
		if seen[id] {
			t.Fatalf("duplicate id: %s", id)
		}
		seen[id] = true
	}
}

func TestAllScenariosR11Retained(t *testing.T) {
	all := AllScenarios()
	want := map[ScenarioID]bool{
		ScenarioWANReplacement:    true,
		ScenarioCGNATChange:       true,
		ScenarioACMEFailure:       true,
		ScenarioInvalidCert:       true,
		ScenarioDomainChange:      true,
		ScenarioDependencyDelete:  true,
		ScenarioRestoreOnNewHW:    true,
		ScenarioSafeApplyLoss:     true,
		ScenarioUpdateMigFailure:  true,
		ScenarioStorageFull:       true,
		ScenarioDisabledNotFailed: true,
		ScenarioWGAccessChange:    true,
		ScenarioTemporaryAccess:   true,
		ScenarioPublishIntent:     true,
		ScenarioOverlappingNets:   true,
		ScenarioIPv6Parity:        true,
		ScenarioBackupWANModem:    true,
		ScenarioFactoryReset:      true,
	}
	for _, id := range all {
		delete(want, id)
	}
	for id := range want {
		t.Fatalf("missing R11 scenario: %s", id)
	}
}
