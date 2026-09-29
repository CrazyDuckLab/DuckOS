package agent

import (
	"context"
	"sync"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/doctor"
)

// DoctorHost is the privileged Agent's Doctor entry point. It
// composes the platform-specific Doctor sources plus the
// privileged observation surface and the per-subsystem state
// from the Agent itself. The Host is the single owner of the
// Source set; tests can swap it for a fake.
type DoctorHost struct {
	environment string
	bootID      string
	sources     []doctor.Source
	now         func() time.Time
}

// NewDoctorHost wires the default Source set. The default set is
// platform-appropriate: on Linux it includes the SystemSource and
// NetworkSource that read /proc and /sys directly. The Host is
// goroutine-safe: Contribute is called once per snapshot.
func NewDoctorHost(env, bootID string, sources ...doctor.Source) *DoctorHost {
	if len(sources) == 0 {
		sources = defaultDoctorSources()
	}
	return &DoctorHost{
		environment: env,
		bootID:      bootID,
		sources:     sources,
		now:         time.Now,
	}
}

func defaultDoctorSources() []doctor.Source {
	return []doctor.Source{
		&doctor.SystemSource{},
		&doctor.NetworkSource{},
	}
}

// Doctor returns a Snapshot built from every registered Source.
// Sources run sequentially; the Builder is concurrency-safe so
// callers may run multiple Doctors in parallel.
func (h *DoctorHost) Doctor(ctx context.Context) (doctor.Snapshot, error) {
	if h == nil {
		return doctor.Snapshot{}, nil
	}
	if err := ctx.Err(); err != nil {
		return doctor.Snapshot{}, err
	}
	now := h.now().UTC()
	b := doctor.NewBuilder(h.environment, "duckos", h.bootID, now, 0)
	var mu sync.Mutex
	for _, src := range h.sources {
		if src == nil {
			continue
		}
		if err := ctx.Err(); err != nil {
			return doctor.Snapshot{}, err
		}
		mu.Lock()
		src.Contribute(b)
		mu.Unlock()
	}
	snap := b.Snapshot()
	return snap, snap.Valid()
}
