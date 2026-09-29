// Package qos owns the bounded Quality-of-Service
// errors shared by every platform-specific
// adapter. The errors are defined in a separate
// file so the contract is the same on Linux
// and on non-Linux builds.
package qos

import "errors"

// ErrUnavailable is returned by the non-Linux
// runner on every method. The Linux runner
// returns it only when the qos subsystem has
// not been configured; the Agent matches on
// this sentinel to surface a clean diagnostic.
var ErrUnavailable = errors.New("qos: linux runner is unavailable on this platform")
