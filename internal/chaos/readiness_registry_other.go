//go:build !linux

// chaos/readiness_registry_other.go: stub for non-Linux
// hosts. The duck-dev-01 / duckos-installer flow always
// runs on Linux, but go test ./... on a darwin host needs
// this stub so the chaos package builds. The stub wires
// only a "platform" probe so the API still has a row to
// render.
package chaos

// RegisterDefaultProbes on non-Linux hosts wires no
// daemon probes. The recorder remains functional for
// tests that exercise the contract directly.
func RegisterDefaultProbes(r *ReadinessRecorder) {}
