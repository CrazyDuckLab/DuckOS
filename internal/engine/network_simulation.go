package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"

	"github.com/CrazyDuckLab/DuckOS/internal/network"
	"github.com/CrazyDuckLab/DuckOS/internal/state"
)

// SimulateNetwork is an offline use case. No observed data is synthesized when
// the candidate lacks dynamic addresses, routes, conntrack or tunnel health.
func (e *Engine) SimulateNetwork(ctx context.Context, raw json.RawMessage) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(raw) > state.MaxConfigBytes {
		return nil, errors.New("simulation request exceeds limit")
	}
	var input struct {
		Candidate json.RawMessage         `json:"candidate"`
		Question  network.TrafficQuestion `json:"question"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&input) != nil || decoder.Decode(new(any)) != io.EOF {
		return nil, errors.New("invalid simulation request")
	}
	candidate, err := network.Decode(input.Candidate)
	if err != nil {
		return nil, errors.New("invalid simulation candidate")
	}
	// A prediction cannot approve intent which exceeds the executable renderer's
	// artifact bounds. Rendering here has no Agent or filesystem side effects.
	if _, err := network.Render(candidate); err != nil {
		return nil, errors.New("simulation candidate cannot be rendered")
	}
	explanation, err := network.Simulate(candidate, input.Question)
	if err != nil {
		return nil, errors.New("invalid simulation question")
	}
	result := struct {
		SchemaVersion  string                     `json:"schemaVersion"`
		HostValidated  bool                       `json:"hostValidated"`
		ApplyAvailable bool                       `json:"applyAvailable"`
		Explanation    network.TrafficExplanation `json:"explanation"`
	}{SchemaVersion: "1", Explanation: explanation}
	data, err := json.Marshal(result)
	if err != nil || len(data) > 256<<10 {
		return nil, errors.New("simulation response exceeds limit")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return data, nil
}
