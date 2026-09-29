// storagewatch/storagewatch_test.go pins
// the V1 storage-failure surface.
package storagewatch

import "testing"

func TestPathStatCompute(t *testing.T) {
	s := PathStat{Path: "/", TotalBytes: 1000, FreeBytes: 100, UsedBytes: 900}
	s = s.Compute(90)
	if s.Mode != ModeNearFull || s.UsedPct != 90 {
		t.Fatalf("near-full: %+v", s)
	}
	s = PathStat{Path: "/", TotalBytes: 1000, FreeBytes: 0, UsedBytes: 1000}
	s = s.Compute(90)
	if s.Mode != ModeFull {
		t.Fatalf("full: %+v", s)
	}
	s = PathStat{Path: "/", TotalBytes: 1000, FreeBytes: 500, UsedBytes: 500}
	s = s.Compute(90)
	if s.Mode != ModeNormal {
		t.Fatalf("normal: %+v", s)
	}
	s = PathStat{Path: "/", TotalBytes: 1000, FreeBytes: 500, UsedBytes: 500, ReadOnly: true}
	s = s.Compute(90)
	if s.Mode != ModeReadOnly {
		t.Fatalf("read-only: %+v", s)
	}
}

func TestManagerWorstPath(t *testing.T) {
	m := NewManager(nil)
	m.SetStat(PathStat{Path: "/var", TotalBytes: 1000, FreeBytes: 100, UsedBytes: 900, Mode: ModeNearFull})
	m.SetStat(PathStat{Path: "/etc", TotalBytes: 1000, FreeBytes: 0, UsedBytes: 1000, Mode: ModeFull})
	worst, ok := m.WorstPath()
	if !ok || worst.Path != "/etc" {
		t.Fatalf("worst: %+v ok=%v", worst, ok)
	}
}

func TestManagerUpdatePreflight(t *testing.T) {
	m := NewManager(nil)
	m.SetStat(PathStat{Path: "/var", TotalBytes: 1000, FreeBytes: 500, UsedBytes: 500, Mode: ModeNormal})
	if res := m.UpdatePreflight("/var", 100); !res.Allowed {
		t.Fatalf("must allow: %+v", res)
	}
	if res := m.UpdatePreflight("/var", 1000); res.Allowed {
		t.Fatal("must refuse")
	}
	m.SetStat(PathStat{Path: "/var", TotalBytes: 1000, FreeBytes: 500, UsedBytes: 500, Mode: ModeReadOnly})
	if res := m.UpdatePreflight("/var", 100); res.Allowed {
		t.Fatal("read-only must refuse")
	}
	if res := m.UpdatePreflight("/missing", 100); res.Allowed {
		t.Fatal("missing path must refuse")
	}
}

func TestManagerDegradation(t *testing.T) {
	m := NewManager(nil)
	m.SetSubsystem(Subsystem{Name: "audit", Priority: PriorityTelemetry, BoundedBytes: 1024})
	m.SetSubsystem(Subsystem{Name: "routing", Priority: PriorityCore, BoundedBytes: 1024})
	m.SetSubsystem(Subsystem{Name: "ui", Priority: PriorityManagement, BoundedBytes: 1024})
	m.SetStat(PathStat{Path: "/var", TotalBytes: 1000, FreeBytes: 100, UsedBytes: 900, Mode: ModeNearFull})
	plan := m.PlanDegradation()
	if len(plan.DropOrder) != 3 {
		t.Fatalf("size: %d", len(plan.DropOrder))
	}
	if plan.DropOrder[0] != "audit" || plan.DropOrder[2] != "routing" {
		t.Fatalf("drop order: %v", plan.DropOrder)
	}
	if plan.KeepOrder[0] != "routing" || plan.KeepOrder[2] != "audit" {
		t.Fatalf("keep order: %v", plan.KeepOrder)
	}
	if plan.WorstPath != "/var" {
		t.Fatalf("worst: %s", plan.WorstPath)
	}
}
