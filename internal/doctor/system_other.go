//go:build !linux

package doctor

import "time"

// SystemSource on non-Linux hosts is intentionally minimal: the
// Doctor explicitly reports "unavailable" for /proc-dependent
// values rather than fabricating fake data. The Source still
// exposes the architecture, since runtime.GOARCH is portable.
type SystemSource struct {
	GeneratedAt time.Time
	BootID      string
}

func (s *SystemSource) Name() string { return "system" }
func (s *SystemSource) Contribute(b *Builder) {
	b.AddSubsystem(Subsystem{Name: "system", Status: "unavailable", Detail: "non-linux host"})
}
