// Package acceptance is the V1 acceptance
// scenarios contract. Each Scenario is a
// reusable, parameterized test shape that
// expresses a real operator situation: WAN
// replacement, CGNAT change, ACME failure,
// invalid manual cert, domain change,
// dependency-aware deletion, restore on
// different hardware, Safe Apply management
// loss, update migration failure, storage
// full, disabled≠failed, WireGuard access
// changes, temporary access, publish service
// intent, overlapping networks, IPv4/IPv6
// parity, backup WAN through modem, factory
// reset.
//
// The package is intentionally a thin model
// over the existing subsystems. The tests in
// scenarios_test.go exercise the contract on
// Darwin; the live Linux qualification is the
// full Linux run in tests/integration.
package acceptance

import "time"

// ScenarioID is a stable identifier for an
// acceptance scenario.
type ScenarioID string

const (
	ScenarioWANReplacement    ScenarioID = "A.wan-replacement"
	ScenarioCGNATChange       ScenarioID = "B.cgnat-change"
	ScenarioACMESuccess       ScenarioID = "C1.acme-success"
	ScenarioACMEFailure       ScenarioID = "C2.acme-failure"
	ScenarioInvalidCert       ScenarioID = "D.invalid-cert"
	ScenarioDomainChange      ScenarioID = "E.domain-change"
	ScenarioDependencyDelete  ScenarioID = "F.dependency-delete"
	ScenarioRestoreOnNewHW    ScenarioID = "G.restore-on-new-hw"
	ScenarioSafeApplyLoss     ScenarioID = "H.safe-apply-management-loss"
	ScenarioUpdateMigFailure  ScenarioID = "I.update-migration-failure"
	ScenarioStorageFull       ScenarioID = "J.storage-full"
	ScenarioDisabledNotFailed ScenarioID = "K.disabled-not-failed"
	ScenarioWGAccessChange    ScenarioID = "L.wg-access-change"
	ScenarioTemporaryAccess   ScenarioID = "M.temporary-access"
	ScenarioPublishIntent     ScenarioID = "N.publish-service-intent"
	ScenarioOverlappingNets   ScenarioID = "O.overlapping-networks"
	ScenarioIPv6Parity        ScenarioID = "P.ipv6-parity"
	ScenarioBackupWANModem    ScenarioID = "Q.backup-wan-modem"
	ScenarioFactoryReset      ScenarioID = "R.factory-reset"
)

// Outcome is the result of a scenario run.
type Outcome string

const (
	OutcomePass    Outcome = "pass"
	OutcomeFail    Outcome = "fail"
	OutcomeSkip    Outcome = "skip"
	OutcomePending Outcome = "pending"
)

// Result is the per-scenario outcome. The
// agent retains the latest run so the future
// UI can show "what was last qualified?".
type Result struct {
	ID         ScenarioID `json:"id"`
	Outcome    Outcome    `json:"outcome"`
	Reason     string     `json:"reason,omitempty"`
	StartedAt  time.Time  `json:"startedAt"`
	FinishedAt time.Time  `json:"finishedAt"`
	// Notes is a free-form operator-facing
	// summary; it is never used to gate a
	// decision.
	Notes string `json:"notes,omitempty"`
}
