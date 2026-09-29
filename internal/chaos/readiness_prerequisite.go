package chaos

import "context"

// PrerequisiteProbe records installation evidence, not API readiness or live
// enforcement. A binary/module/directory alone cannot establish feature support.
type PrerequisiteProbe struct{ ReadinessProbe }

func (p PrerequisiteProbe) Probe(ctx context.Context) (Readiness, string, error) {
	state, note, err := p.ReadinessProbe.Probe(ctx)
	if err == nil && state == ReadySupported {
		return ReadyExperimental, "Prerequisite only; runtime/API integration unverified. " + note, nil
	}
	return state, note, err
}
