//go:build !linux

package doctor

type NetworkSource struct{}

func (s *NetworkSource) Name() string { return "network" }
func (s *NetworkSource) Contribute(b *Builder) {
	b.AddSubsystem(Subsystem{Name: "network", Status: "unavailable", Detail: "non-linux host"})
}
