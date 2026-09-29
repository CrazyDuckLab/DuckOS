// svcstate/svcstate_test.go pins the V1
// service-lifecycle state machine.
package svcstate

import (
	"errors"
	"testing"
	"time"
)

func TestStateValid(t *testing.T) {
	for _, s := range []State{StateStopped, StateStarting, StateRunning, StateReloading, StateDegraded, StateFailing, StateRestarting} {
		if !s.Valid() {
			t.Fatalf("%s must be valid", s)
		}
	}
	if State("garbage").Valid() {
		t.Fatal("garbage must be invalid")
	}
	if !StateRunning.Healthy() {
		t.Fatal("running must be healthy")
	}
	if StateStopped.Healthy() {
		t.Fatal("stopped must not be healthy")
	}
}

func TestServiceValid(t *testing.T) {
	good := Service{Name: "xray", State: StateRunning, DisruptionBudget: 30 * time.Second}
	if err := good.Valid(); err != nil {
		t.Fatal(err)
	}
	bad := Service{Name: "Xray", State: StateRunning}
	if err := bad.Valid(); err == nil {
		t.Fatal("non-canonical name must fail")
	}
	bad2 := Service{Name: "xray", State: "garbage"}
	if err := bad2.Valid(); err == nil {
		t.Fatal("unknown state must fail")
	}
}

func TestManagerCRUD(t *testing.T) {
	m := NewManager(time.Now)
	if err := m.Set(Service{Name: "xray", State: StateRunning}); err != nil {
		t.Fatal(err)
	}
	if _, ok := m.Get("xray"); !ok {
		t.Fatal("get failed")
	}
	if len(m.List()) != 1 {
		t.Fatal("list")
	}
}

func TestManagerReload(t *testing.T) {
	m := NewManager(time.Now)
	if err := m.Set(Service{Name: "xray", State: StateRunning, LastConfig: "v1"}); err != nil {
		t.Fatal(err)
	}
	validate := func(c []byte) error { return nil }
	apply := func(c []byte) (State, error) { return StateReloading, nil }
	if err := m.ReloadConfig("xray", []byte("new"), "v2", validate, apply); err != nil {
		t.Fatal(err)
	}
	s, _ := m.Get("xray")
	if s.LastConfig != "v2" || s.LastGoodConfig != "v1" {
		t.Fatalf("last/lastgood: %+v", s)
	}
}

func TestManagerReloadValidationFails(t *testing.T) {
	m := NewManager(time.Now)
	if err := m.Set(Service{Name: "xray", State: StateRunning, LastConfig: "v1"}); err != nil {
		t.Fatal(err)
	}
	validate := func(c []byte) error { return errors.New("bad") }
	apply := func(c []byte) (State, error) { return StateReloading, nil }
	if err := m.ReloadConfig("xray", []byte("new"), "v2", validate, apply); err == nil {
		t.Fatal("validation must fail")
	}
	s, _ := m.Get("xray")
	if s.LastConfig != "v1" {
		t.Fatal("active config must not change on validation failure")
	}
}

func TestManagerReloadApplyFails(t *testing.T) {
	m := NewManager(time.Now)
	if err := m.Set(Service{Name: "xray", State: StateRunning, LastConfig: "v1"}); err != nil {
		t.Fatal(err)
	}
	validate := func(c []byte) error { return nil }
	apply := func(c []byte) (State, error) { return StateStopped, errors.New("apply failed") }
	if err := m.ReloadConfig("xray", []byte("new"), "v2", validate, apply); err == nil {
		t.Fatal("apply failure must error")
	}
	s, _ := m.Get("xray")
	if s.LastError == "" {
		t.Fatal("last error must be set")
	}
	if s.State != StateFailing {
		t.Fatalf("state: %s", s.State)
	}
}

func TestManagerRollback(t *testing.T) {
	m := NewManager(time.Now)
	if err := m.Set(Service{Name: "xray", State: StateRunning, LastConfig: "v2", LastGoodConfig: "v1"}); err != nil {
		t.Fatal(err)
	}
	if err := m.Rollback("xray", "test"); err != nil {
		t.Fatal(err)
	}
	s, _ := m.Get("xray")
	if s.LastConfig != "v1" {
		t.Fatal("rollback must restore last-good")
	}
}

func TestManagerRollbackNoGood(t *testing.T) {
	m := NewManager(time.Now)
	if err := m.Set(Service{Name: "xray", State: StateRunning, LastConfig: "v1"}); err != nil {
		t.Fatal(err)
	}
	if err := m.Rollback("xray", "test"); err == nil {
		t.Fatal("rollback without last-good must fail")
	}
}

func TestManagerSnapshot(t *testing.T) {
	m := NewManager(time.Now)
	m.Set(Service{Name: "xray", State: StateRunning})
	m.Set(Service{Name: "dns", State: StateStopped})
	m.Set(Service{Name: "fw", State: StateFailing, LastError: "rule fail"})
	snap := m.Snapshot()
	if len(snap) != 3 {
		t.Fatal("size")
	}
	for _, h := range snap {
		switch h.Name {
		case "xray":
			if !h.ProcessUp || !h.Functioning {
				t.Fatalf("xray health: %+v", h)
			}
		case "dns":
			if h.ProcessUp || h.Functioning {
				t.Fatalf("dns health: %+v", h)
			}
		case "fw":
			if !h.ProcessUp || h.Functioning {
				t.Fatalf("fw health: %+v", h)
			}
		}
	}
}
