//go:build !linux

package qa

import "testing"

// CaptureManagementBoundary is a no-op on non-Linux.
func CaptureManagementBoundary(t testing.TB) ManagementBoundary {
	return ManagementBoundary{}
}

// AssertBoundaryEqual is a no-op on non-Linux.
func AssertBoundaryEqual(t testing.TB, before, after ManagementBoundary) {}

// NewLab is a stub on non-Linux: it skips.
func NewLab(t testing.TB, name string) *Lab {
	t.Skip("linux lab requires Linux")
	return nil
}
