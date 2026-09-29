//go:build !linux

package agent

import (
	"errors"
	"net"
)

func Listen(string, uint32) (net.Listener, error) {
	return nil, errors.New("Duck Agent requires Linux SO_PEERCRED and Linux host state")
}
