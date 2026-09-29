//go:build !linux

// capture_other.go returns a Host that refuses
// every operation on non-Linux platforms. The
// real Linux implementation lives in
// capture_linux.go and is selected through the
// build tag above.
package capture

// NewLinuxHost is unavailable on non-Linux
// platforms.
func NewLinuxHost() (Host, error) { return nil, ErrUnsupported }
