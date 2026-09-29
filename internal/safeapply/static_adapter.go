package safeapply

import (
	"context"
	"encoding/json"
	"errors"
)

// StaticAdapter is a bounded, do-nothing adapter that only validates candidate
// JSON. It is used during the second-stage cold-boot gate so the network
// service can report its startup state before the full managed-routing
// adapter is wired in (R1.4). The adapter never mutates host state and
// refuses every apply. Production callers must replace it with the real
// adapter before admitting work; the recovery_required status remains the
// fall-back safety net.
type StaticAdapter struct{}

func NewStaticAdapter() *StaticAdapter { return &StaticAdapter{} }

func (StaticAdapter) Preflight(_ context.Context, _, _ json.RawMessage) error {
	return errors.New("static adapter: managed routing adapter is not yet wired")
}

func (StaticAdapter) Apply(_ context.Context, _, _ json.RawMessage) error {
	return errors.New("static adapter: managed routing adapter is not yet wired")
}

func (StaticAdapter) Verify(_ context.Context, _ json.RawMessage) error {
	return errors.New("static adapter: managed routing adapter is not yet wired")
}

func (StaticAdapter) Restore(_ context.Context, _, _ json.RawMessage) error {
	return errors.New("static adapter: managed routing adapter is not yet wired")
}
