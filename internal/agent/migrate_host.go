package agent

import (
	"context"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/migrate"
)

// MigrateHost is the Agent-side runtime that
// owns the migration analysis surface. The
// host is goroutine-safe; the Agent calls
// Migrate from the loopback handler under a
// concurrency cap. The host is read-only: the
// analyser never executes the source commands
// and never mutates host state.
type MigrateHost struct {
	clock func() time.Time
}

// NewMigrateHost returns a host with a default
// clock. The constructor is the single entry
// point used by the duck-agent startup.
func NewMigrateHost() *MigrateHost {
	return &MigrateHost{clock: func() time.Time { return time.Now().UTC() }}
}

// Migrate runs the analyser against the
// supplied source. The host is a thin
// pass-through: the package owns the bounded
// contract; the host owns nothing but the
// clock.
func (h *MigrateHost) Migrate(_ context.Context, src migrate.Source, body []byte) (migrate.Plan, error) {
	if h == nil {
		return migrate.Plan{}, nil
	}
	return migrate.Analyse(src, body, h.clock())
}
