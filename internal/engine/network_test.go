package engine

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestNetworkPreviewRejectsInvalidIntentWithoutAgent(t *testing.T) {
	e := New("/nonexistent/synthetic-agent.sock", "TEST")
	for _, raw := range []string{`{}`, `{"version":2}`, `{"version":1,"shell":"anything"}`, `{"version":1} {}`, strings.Repeat("x", 256<<10+1)} {
		if _, err := e.PreviewNetwork(context.Background(), json.RawMessage(raw)); err == nil {
			t.Fatal("invalid preview accepted")
		}
	}
	data, err := e.PreviewNetwork(context.Background(), json.RawMessage(`{"version":1}`))
	if err != nil || !strings.Contains(string(data), `"hostValidated":false`) || !strings.Contains(string(data), `"applyAvailable":false`) || strings.Contains(string(data), "hook input") {
		t.Fatal("preview implied host validation or active baseline", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = e.PreviewNetwork(ctx, json.RawMessage(`{"version":1}`)); err != context.Canceled {
		t.Fatal("cancellation ignored", err)
	}
}
