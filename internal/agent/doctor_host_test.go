package agent

import (
	"context"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/doctor"
)

type stubSource struct {
	name   string
	called atomic.Int32
}

func (s *stubSource) Name() string { return s.name }
func (s *stubSource) Contribute(b *doctor.Builder) {
	s.called.Add(1)
	b.AddSubsystem(doctor.Subsystem{Name: s.name, Status: "ok"})
}

func TestDoctorHostAggregatesSources(t *testing.T) {
	a := &stubSource{name: "a"}
	b := &stubSource{name: "b"}
	h := NewDoctorHost("test", "boot-1", a, b)
	snap, err := h.Doctor(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if a.called.Load() == 0 || b.called.Load() == 0 {
		t.Fatal("not all sources were called")
	}
	if len(snap.Subsystem) != 2 {
		t.Fatalf("expected 2 subsystems, got %d", len(snap.Subsystem))
	}
}

func TestDoctorHostNilSourceSkipped(t *testing.T) {
	stub := &stubSource{name: "live"}
	h := NewDoctorHost("test", "boot-1", stub, nil)
	snap, err := h.Doctor(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if stub.called.Load() == 0 {
		t.Fatal("stub was not called")
	}
	if len(snap.Subsystem) != 1 {
		t.Fatalf("expected 1 subsystem, got %d", len(snap.Subsystem))
	}
}

func TestDoctorHostContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	h := NewDoctorHost("test", "boot-1")
	if _, err := h.Doctor(ctx); err == nil {
		t.Fatal("cancelled context was accepted")
	}
}

func TestDoctorHostNilReceiver(t *testing.T) {
	var h *DoctorHost
	snap, err := h.Doctor(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snap.SchemaVersion != "" {
		t.Fatalf("nil host should produce an empty snapshot, got %+v", snap)
	}
	_ = doctor.SchemaVersion
}

func TestDoctorHostDefaultSources(t *testing.T) {
	h := NewDoctorHost("test", "boot-1")
	snap, err := h.Doctor(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	hasNetwork := false
	for _, s := range snap.Subsystem {
		if s.Name == "network" {
			hasNetwork = true
		}
	}
	if !hasNetwork {
		t.Fatalf("network source missing: %+v", snap.Subsystem)
	}
	// The Linux system source is silent: it never adds a
	// Subsystem entry because it never fails on a real host.
	// The non-Linux stub returns an "unavailable" Subsystem
	// entry. Either way, the system source must contribute
	// *some* observable state.
	if runtime.GOOS == "linux" {
		if snap.System.Kernel == "" {
			t.Fatalf("system source did not contribute: %+v", snap.System)
		}
	} else {
		found := false
		for _, s := range snap.Subsystem {
			if s.Name == "system" {
				found = true
			}
		}
		if !found {
			t.Fatalf("system source did not contribute: %+v", snap.Subsystem)
		}
	}
}

func TestDoctorHostTimestampStable(t *testing.T) {
	fixed := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	h := NewDoctorHost("test", "boot-1")
	h.now = func() time.Time { return fixed }
	snap, _ := h.Doctor(context.Background())
	if !snap.GeneratedAt.Equal(fixed) {
		t.Fatalf("expected generatedAt %v, got %v", fixed, snap.GeneratedAt)
	}
}

func TestDoctorHostConcurrent(t *testing.T) {
	h := NewDoctorHost("test", "boot-1", &stubSource{name: "x"})
	done := make(chan struct{}, 16)
	for i := 0; i < 16; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			if _, err := h.Doctor(context.Background()); err != nil {
				t.Errorf("doctor: %v", err)
			}
		}()
	}
	for i := 0; i < 16; i++ {
		<-done
	}
}

func TestDoctorHostEnvironmentAndBootIDRecorded(t *testing.T) {
	h := NewDoctorHost("production", "boot-42")
	snap, err := h.Doctor(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snap.Environment != "production" {
		t.Fatalf("expected production, got %q", snap.Environment)
	}
	if snap.BootID != "boot-42" {
		t.Fatalf("expected boot-42, got %q", snap.BootID)
	}
}
