// recovery/recovery_test.go pins the V1
// recovery surface.
package recovery

import (
	"testing"
	"time"
)

func TestResetKindValid(t *testing.T) {
	if !ResetFull.Valid() || !ResetNetworkOnly.Valid() || !ResetAdminOnly.Valid() || !ResetFromBackup.Valid() {
		t.Fatal("known kinds must be valid")
	}
	if ResetKind("garbage").Valid() {
		t.Fatal("garbage must be invalid")
	}
}

func TestEmergencyAccessValid(t *testing.T) {
	now := time.Now()
	good := EmergencyAccess{ID: "em-1", Reason: "lockout", Operator: "admin", CreatedAt: now, ExpiresAt: now.Add(time.Hour)}
	if err := good.Valid(); err != nil {
		t.Fatal(err)
	}
	tooLong := EmergencyAccess{ID: "em-1", Reason: "x", Operator: "admin", CreatedAt: now, ExpiresAt: now.Add(48 * time.Hour)}
	if err := tooLong.Valid(); err == nil {
		t.Fatal(">24h window must fail")
	}
	expired := EmergencyAccess{ID: "em-1", Reason: "x", Operator: "admin", CreatedAt: now, ExpiresAt: now}
	if err := expired.Valid(); err == nil {
		t.Fatal("expired window must fail")
	}
	noReason := EmergencyAccess{ID: "em-1", Operator: "admin", CreatedAt: now, ExpiresAt: now.Add(time.Hour)}
	if err := noReason.Valid(); err == nil {
		t.Fatal("missing reason must fail")
	}
}

func TestEmergencyAccessActive(t *testing.T) {
	now := time.Now()
	a := EmergencyAccess{ID: "em-1", Reason: "x", Operator: "admin", CreatedAt: now, ExpiresAt: now.Add(time.Hour)}
	if !a.Active(now.Add(time.Minute)) {
		t.Fatal("mid-window must be active")
	}
	if a.Active(now.Add(2 * time.Hour)) {
		t.Fatal("past expiry must be inactive")
	}
}

func TestEnvironmentValid(t *testing.T) {
	if !EnvActive.Valid() || !EnvPrevious.Valid() || !EnvRecovery.Valid() {
		t.Fatal("known envs must be valid")
	}
	if Environment("garbage").Valid() {
		t.Fatal("garbage must be invalid")
	}
}

func TestRebootReasonValid(t *testing.T) {
	if !RebootOperator.Valid() || !RebootUpdate.Valid() || !RebootHealthFailure.Valid() {
		t.Fatal("known reasons must be valid")
	}
	if RebootReason("garbage").Valid() {
		t.Fatal("garbage must be invalid")
	}
}

func TestResetRequestValid(t *testing.T) {
	good := ResetRequest{Kind: ResetNetworkOnly, Operator: "admin", Reason: "rolling back"}
	if err := good.Valid(); err != nil {
		t.Fatal(err)
	}
	fromBackup := ResetRequest{Kind: ResetFromBackup, Operator: "admin", Reason: "restore"}
	if err := fromBackup.Valid(); err == nil {
		t.Fatal("from-backup without backupId must fail")
	}
	fromBackupOK := ResetRequest{Kind: ResetFromBackup, BackupID: "bk-1", Operator: "admin", Reason: "restore"}
	if err := fromBackupOK.Valid(); err != nil {
		t.Fatal(err)
	}
}

func TestResetRequestImpact(t *testing.T) {
	imp := ResetRequest{Kind: ResetFull, Operator: "admin", Reason: "test"}.Impact()
	if !imp.WipesConfig || !imp.WipesVault || !imp.WipesIdentity || !imp.WipesAudit || !imp.WipesBackups {
		t.Fatalf("full must wipe everything: %+v", imp)
	}
	imp = ResetRequest{Kind: ResetNetworkOnly, Operator: "admin", Reason: "test"}.Impact()
	if !imp.WipesConfig || imp.WipesVault {
		t.Fatalf("network-only must wipe config but not vault: %+v", imp)
	}
	imp = ResetRequest{Kind: ResetAdminOnly, Operator: "admin", Reason: "test"}.Impact()
	if !imp.WipesIdentity || imp.WipesConfig {
		t.Fatalf("admin-only must wipe identity but not config: %+v", imp)
	}
}

func TestManagerEmergency(t *testing.T) {
	now := time.Now()
	m := NewManager(EnvActive, func() time.Time { return now })
	if _, err := m.OpenEmergency(EmergencyAccess{ID: "em-1", Reason: "x", Operator: "admin", CreatedAt: now, ExpiresAt: now.Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.OpenEmergency(EmergencyAccess{ID: "em-2", Reason: "y", Operator: "admin", CreatedAt: now, ExpiresAt: now.Add(-time.Hour)}); err == nil {
		t.Fatal("expired-at-create must fail")
	}
	active := m.ActiveEmergency(now.Add(5 * time.Minute))
	if len(active) != 1 || active[0].ID != "em-1" {
		t.Fatalf("active: %+v", active)
	}
	if err := m.CloseEmergency("em-1"); err != nil {
		t.Fatal(err)
	}
	if len(m.ActiveEmergency(now.Add(5*time.Minute))) != 0 {
		t.Fatal("closed window must be inactive")
	}
}

func TestManagerSetEnvironment(t *testing.T) {
	m := NewManager(EnvActive, nil)
	if err := m.SetEnvironment(EnvRecovery); err != nil {
		t.Fatal(err)
	}
	if m.Environment() != EnvRecovery {
		t.Fatal("env not updated")
	}
	if err := m.SetEnvironment("garbage"); err == nil {
		t.Fatal("garbage env must fail")
	}
}

func TestManagerRebootHistory(t *testing.T) {
	now := time.Now()
	m := NewManager(EnvActive, func() time.Time { return now })
	for i := 0; i < 5; i++ {
		if err := m.RecordReboot(RebootOperator, "manual"); err != nil {
			t.Fatal(err)
		}
	}
	h := m.RebootHistory()
	if len(h) != 5 {
		t.Fatalf("history: %d", len(h))
	}
}

func TestDefaultShutdownPlan(t *testing.T) {
	p := DefaultShutdownPlan(RebootOperator, "manual")
	if len(p.Steps) != 5 {
		t.Fatalf("steps: %d", len(p.Steps))
	}
	for _, s := range p.Steps {
		if !s.Required {
			t.Fatalf("all steps must be required: %+v", s)
		}
	}
}
