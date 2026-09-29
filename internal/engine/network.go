package engine

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/CrazyDuckLab/DuckOS/internal/network"
)

// PreviewNetwork validates desired state without persisting it or contacting
// the privileged Agent. A renderable plan is not evidence of host compatibility.
func (e *Engine) PreviewNetwork(ctx context.Context, raw json.RawMessage) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	candidate, err := network.Decode(raw)
	if err != nil {
		return nil, errors.New("network configuration is invalid or unsupported")
	}
	plan, err := network.Render(candidate)
	if err != nil {
		return nil, errors.New("network plan could not be rendered")
	}
	result := struct {
		SchemaVersion  string       `json:"schemaVersion"`
		HostValidated  bool         `json:"hostValidated"`
		ApplyAvailable bool         `json:"applyAvailable"`
		Plan           network.Plan `json:"plan"`
	}{SchemaVersion: "1", Plan: plan}
	data, err := json.Marshal(result)
	if err != nil || len(data) > 2<<20 {
		return nil, errors.New("network preview exceeds response limit")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return data, nil
}
