// upgrade/migrate_test.go pins the V1
// migration runner contract.
package upgrade

import (
	"errors"
	"testing"
)

func TestMigrationStepValid(t *testing.T) {
	good := MigrationStep{Name: "v1-to-v2", FromSchema: 1, ToSchema: 2, Apply: func(in []byte) ([]byte, error) { return in, nil }}
	if err := good.Valid(); err != nil {
		t.Fatal(err)
	}
	bad := MigrationStep{Name: "!bad", FromSchema: 1, ToSchema: 2, Apply: func(in []byte) ([]byte, error) { return in, nil }}
	if err := bad.Valid(); err == nil {
		t.Fatal("non-canonical name must fail")
	}
	bad2 := MigrationStep{Name: "x", FromSchema: 2, ToSchema: 1, Apply: func(in []byte) ([]byte, error) { return in, nil }}
	if err := bad2.Valid(); err == nil {
		t.Fatal("to<=from must fail")
	}
	bad3 := MigrationStep{Name: "x", FromSchema: 1, ToSchema: 2}
	if err := bad3.Valid(); err == nil {
		t.Fatal("missing Apply must fail")
	}
}

func TestMigrationChainAddAndPlan(t *testing.T) {
	c := NewMigrationChain()
	if err := c.Add(MigrationStep{Name: "v1-v2", FromSchema: 1, ToSchema: 2, Apply: func(in []byte) ([]byte, error) { return in, nil }}); err != nil {
		t.Fatal(err)
	}
	if err := c.Add(MigrationStep{Name: "v2-v3", FromSchema: 2, ToSchema: 3, Apply: func(in []byte) ([]byte, error) { return in, nil }}); err != nil {
		t.Fatal(err)
	}
	if err := c.Add(MigrationStep{Name: "v1-v2", FromSchema: 1, ToSchema: 2, Apply: func(in []byte) ([]byte, error) { return in, nil }}); err == nil {
		t.Fatal("duplicate must fail")
	}
	plan, err := c.Plan(1, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 2 {
		t.Fatalf("plan steps: %d", len(plan.Steps))
	}
}

func TestMigrationChainOverlap(t *testing.T) {
	c := NewMigrationChain()
	if err := c.Add(MigrationStep{Name: "v1-v3", FromSchema: 1, ToSchema: 3, Apply: func(in []byte) ([]byte, error) { return in, nil }}); err != nil {
		t.Fatal(err)
	}
	if err := c.Add(MigrationStep{Name: "v2-v4", FromSchema: 2, ToSchema: 4, Apply: func(in []byte) ([]byte, error) { return in, nil }}); err == nil {
		t.Fatal("overlap must fail")
	}
}

func TestMigrationPlanNoPath(t *testing.T) {
	c := NewMigrationChain()
	if err := c.Add(MigrationStep{Name: "v1-v2", FromSchema: 1, ToSchema: 2, Apply: func(in []byte) ([]byte, error) { return in, nil }}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Plan(2, 3); err == nil {
		t.Fatal("no path must fail")
	}
}

func TestMigrationPlanSameSchema(t *testing.T) {
	c := NewMigrationChain()
	plan, err := c.Plan(2, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 0 {
		t.Fatalf("same-schema plan must have no steps, got %d", len(plan.Steps))
	}
}

func TestMigrationPlanDowngrade(t *testing.T) {
	c := NewMigrationChain()
	if _, err := c.Plan(3, 1); err == nil {
		t.Fatal("down-migration must fail")
	}
}

func TestMigrationRun(t *testing.T) {
	c := NewMigrationChain()
	if err := c.Add(MigrationStep{Name: "v1-v2", FromSchema: 1, ToSchema: 2, Apply: func(in []byte) ([]byte, error) { return append(in, '-', '2'), nil }}); err != nil {
		t.Fatal(err)
	}
	if err := c.Add(MigrationStep{Name: "v2-v3", FromSchema: 2, ToSchema: 3, Apply: func(in []byte) ([]byte, error) { return append(in, '-', '3'), nil }}); err != nil {
		t.Fatal(err)
	}
	plan, err := c.Plan(1, 3)
	if err != nil {
		t.Fatal(err)
	}
	validate := func(in []byte) error {
		if len(in) == 0 {
			return errors.New("empty")
		}
		return nil
	}
	r, err := Run(plan, []byte("base"), validate)
	if err != nil {
		t.Fatal(err)
	}
	if string(r.Config) != "base-2-3" {
		t.Fatalf("config: %q", string(r.Config))
	}
	if len(r.Steps) != 2 || r.Steps[0] != "v1-v2" || r.Steps[1] != "v2-v3" {
		t.Fatalf("steps: %v", r.Steps)
	}
}

func TestMigrationRunValidateFails(t *testing.T) {
	c := NewMigrationChain()
	if err := c.Add(MigrationStep{Name: "v1-v2", FromSchema: 1, ToSchema: 2, Apply: func(in []byte) ([]byte, error) { return nil, nil }}); err != nil {
		t.Fatal(err)
	}
	plan, _ := c.Plan(1, 2)
	validate := func(in []byte) error { return errors.New("invalid") }
	if _, err := Run(plan, []byte("base"), validate); err == nil {
		t.Fatal("validate failure must abort")
	}
}
