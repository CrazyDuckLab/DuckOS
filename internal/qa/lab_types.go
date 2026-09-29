package qa

import "testing"

// ManagementBoundary is the protected set of resources on
// the qualification host that the lab must never touch.
// See lab_linux.go for the Linux implementation.
type ManagementBoundary struct {
	Interface string
	Address   string // CIDR
	Gateway   string
	SSHAlive  bool
}

// Lab is one disposable network namespace with optional
// veth peers, an optional bridge, and a teardown that
// always runs. See lab_linux.go for the Linux
// implementation; lab_other.go provides a non-Linux stub.
type Lab struct {
	Name       string
	NetNS      string
	OwnedLinks []string
	OwnedRules []string
	t          testing.TB
}

// DefaultManagementBoundary returns the duck-dev-01
// production boundary that every destructive test must
// re-check before and after.
func DefaultManagementBoundary() ManagementBoundary {
	return ManagementBoundary{
		Interface: "eth0",
		Address:   "172.18.4.149/16",
		Gateway:   "172.18.1.254",
		SSHAlive:  true,
	}
}
