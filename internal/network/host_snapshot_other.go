//go:build !linux

package network

import (
	"context"
	"encoding/json"
	"errors"
)

func CaptureHost(context.Context, Config, Config) (json.RawMessage, error) {
	return nil, errors.New("host network snapshot requires Linux; state is not simulated")
}
