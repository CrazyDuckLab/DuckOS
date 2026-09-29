//go:build !linux

package wireguard

import "context"

// IPRouteRunner is a stub on non-Linux hosts. Production code never
// reaches this path; tests use the synthetic fakeRunner instead.
type IPRouteRunner struct{}

func (IPRouteRunner) Run(_ context.Context, _ []string) (string, error) {
	return "", errUnsupportedAdapter
}

var errUnsupportedAdapter = &unsupportedAdapterError{}

type unsupportedAdapterError struct{}

func (*unsupportedAdapterError) Error() string {
	return "wireguard kernel adapter requires a Linux host"
}

// LocalFilesystem is a stub on non-Linux hosts. The kernel adapter
// refuses to operate; tests use the synthetic fakeFilesystem.
type LocalFilesystem struct{}

func (LocalFilesystem) WriteAtomic(_ context.Context, _ string, _ uint32, _ []byte) error {
	return errUnsupportedAdapter
}
func (LocalFilesystem) Remove(_ context.Context, _ string) error { return errUnsupportedAdapter }

// VaultKeyResolver is a stub on non-Linux hosts.
type VaultKeyResolver struct{}

func (VaultKeyResolver) Resolve(_ context.Context, _ KeyRef) ([]byte, error) {
	return nil, errUnsupportedAdapter
}
