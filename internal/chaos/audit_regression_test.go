package chaos

import (
	"context"
	"testing"
)

func TestAuditPrerequisiteIsNotFeatureSupport(t *testing.T) {
	for _, s := range []Readiness{ReadySupported, ReadyHWPending, ReadyDegraded} {
		p := PrerequisiteProbe{ProbeFunc{Name: "vpn.xray", Run: func(context.Context) (Readiness, string, error) { return s, "binary found", nil }}}
		got, _, err := p.Probe(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if s == ReadySupported && got != ReadyExperimental {
			t.Fatal("prerequisite presented as ready")
		}
		if s != ReadySupported && got != s {
			t.Fatal("lost failure evidence")
		}
	}
}
