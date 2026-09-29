// backup/restore_test.go pins the V1 logical
// backup surface.
package backup

import "testing"

func TestBackupModeValid(t *testing.T) {
	if !BackupModeConfigOnly.Valid() || !BackupModeFullProtected.Valid() {
		t.Fatal("known modes must be valid")
	}
	if BackupMode("garbage").Valid() {
		t.Fatal("garbage must be invalid")
	}
}

func TestInterfaceBindingValid(t *testing.T) {
	good := InterfaceBinding{Logical: "wan", Physical: "eth0", Kind: "ethernet"}
	if err := good.Valid(); err != nil {
		t.Fatal(err)
	}
	bad := InterfaceBinding{Logical: "", Physical: "eth0", Kind: "ethernet"}
	if err := bad.Valid(); err == nil {
		t.Fatal("missing logical must fail")
	}
	bad2 := InterfaceBinding{Logical: "wan", Physical: "", Kind: "ethernet"}
	if err := bad2.Valid(); err == nil {
		t.Fatal("missing physical must fail")
	}
	bad3 := InterfaceBinding{Logical: "wan", Physical: "eth0", Kind: ""}
	if err := bad3.Valid(); err == nil {
		t.Fatal("missing kind must fail")
	}
}

func TestLogicalManifestValid(t *testing.T) {
	good := LogicalManifest{BackupID: "bk-1", SchemaVersion: 1, Mode: BackupModeConfigOnly, Interfaces: []InterfaceBinding{{Logical: "wan", Physical: "eth0", Kind: "ethernet"}}}
	if err := good.Valid(); err != nil {
		t.Fatal(err)
	}
	includesSecretsBad := LogicalManifest{BackupID: "bk-1", SchemaVersion: 1, Mode: BackupModeConfigOnly, IncludesSecrets: true}
	if err := includesSecretsBad.Valid(); err == nil {
		t.Fatal("config-only with secrets must fail")
	}
	fullGood := LogicalManifest{BackupID: "bk-1", SchemaVersion: 1, Mode: BackupModeFullProtected, IncludesSecrets: true}
	if err := fullGood.Valid(); err != nil {
		t.Fatal(err)
	}
	badID := LogicalManifest{BackupID: "!bad", SchemaVersion: 1, Mode: BackupModeConfigOnly}
	if err := badID.Valid(); err == nil {
		t.Fatal("non-canonical id must fail")
	}
}

func TestRemap(t *testing.T) {
	in := LogicalManifest{BackupID: "bk-1", SchemaVersion: 1, Mode: BackupModeConfigOnly, Interfaces: []InterfaceBinding{
		{Logical: "wan", Physical: "eth0", Kind: "ethernet"},
		{Logical: "lan", Physical: "eth1", Kind: "ethernet"},
	}}
	out, missing := Remap(in, []RemapRequest{{Logical: "wan", Physical: "enp0s3"}})
	if len(missing) != 1 || missing[0] != "lan" {
		t.Fatalf("missing: %v", missing)
	}
	if out.Interfaces[0].Physical != "enp0s3" {
		t.Fatalf("physical not remapped: %+v", out.Interfaces[0])
	}
}

func TestRemapRequestValid(t *testing.T) {
	good := RemapRequest{Logical: "wan", Physical: "eth0"}
	if err := good.Valid(); err != nil {
		t.Fatal(err)
	}
	if err := (RemapRequest{}).Valid(); err == nil {
		t.Fatal("empty must fail")
	}
}

func TestPlanRestore(t *testing.T) {
	plan := PlanRestore("bk-1", true, map[string]bool{"a": true, "b": true}, map[string]bool{"b": true, "c": true})
	if !plan.RequiresSecret {
		t.Fatal("requires-secret must be set")
	}
	// M-20: the plan is unsafe because it has Modifies,
	// Removes and RequiresSecret.
	if plan.SafeToApply {
		t.Fatal("plan must NOT be safe when Modifies/Removes/Secret are present (M-20)")
	}
	if len(plan.Adds) != 1 || plan.Adds[0] != "a" {
		t.Fatalf("adds: %v", plan.Adds)
	}
	if len(plan.Modifies) != 1 || plan.Modifies[0] != "b" {
		t.Fatalf("modifies: %v", plan.Modifies)
	}
	if len(plan.Removes) != 1 || plan.Removes[0] != "c" {
		t.Fatalf("removes: %v", plan.Removes)
	}
	if len(plan.Impact) == 0 {
		t.Fatal("impact must be set when removes exist")
	}
	// Every downgrade reason must appear in Impact.
	want := map[string]bool{"removes-referenced-objects": true, "overwrites-live-objects": true, "requires-secret": true}
	for _, s := range plan.Impact {
		delete(want, s)
	}
	if len(want) > 0 {
		t.Fatalf("missing impact markers: %v (got %v)", want, plan.Impact)
	}
}

func TestMigratorsApply(t *testing.T) {
	m := NewMigrators()
	if err := m.Register(1, func(in map[string]bool) (map[string]bool, error) {
		out := map[string]bool{}
		for k := range in {
			out[k+":v2"] = true
		}
		return out, nil
	}); err != nil {
		t.Fatal(err)
	}
	cur, err := m.Apply(map[string]bool{"a": true, "b": true}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !cur["a:v2"] || !cur["b:v2"] {
		t.Fatalf("not migrated: %+v", cur)
	}
}

func TestMigratorsFuture(t *testing.T) {
	m := NewMigrators()
	if _, err := m.Apply(map[string]bool{}, 3); err == nil {
		t.Fatal("future schema must be rejected")
	}
}

func TestMigratorsNoPath(t *testing.T) {
	m := NewMigrators()
	if _, err := m.Apply(map[string]bool{}, 1); err == nil {
		t.Fatal("missing migrator must fail")
	}
}

func TestMigratorsDuplicate(t *testing.T) {
	m := NewMigrators()
	if err := m.Register(1, func(in map[string]bool) (map[string]bool, error) { return in, nil }); err != nil {
		t.Fatal(err)
	}
	if err := m.Register(1, func(in map[string]bool) (map[string]bool, error) { return in, nil }); err == nil {
		t.Fatal("duplicate must fail")
	}
}
