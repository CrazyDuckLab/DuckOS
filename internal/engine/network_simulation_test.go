package engine

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestNetworkSimulationNeedsNoAgentOrNetwork(t *testing.T) {
	e := New("/nonexistent/synthetic-agent.sock", "TEST")
	const candidate = `{"version":1,"interfaces":[{"id":"inside","name":"testlan0","kind":"physical","role":"lan","mtu":1500,"dhcpClient":"none","management":true,"addresses":["198.51.100.1/24"]}]}`
	const question = `{"ingress":"inside","source":"198.51.100.20","destination":"203.0.113.20","protocol":"tcp","connectionState":"new"}`
	const request = `{"candidate":` + candidate + `,"question":` + question + `}`
	data, err := e.SimulateNetwork(context.Background(), json.RawMessage(request))
	if err != nil || !strings.Contains(string(data), `"hostValidated":false`) || !strings.Contains(string(data), `"applyAvailable":false`) || !strings.Contains(string(data), `"managedDecision":"drop"`) {
		t.Fatal(string(data), err)
	}
	for _, bad := range []string{`{}`, request + ` {}`, strings.Replace(request, `"protocol":"tcp"`, `"protocol":"tcp","probe":true`, 1), strings.Replace(request, `"203.0.113.20"`, `"example.test"`, 1), strings.Repeat("x", 256<<10+1)} {
		if _, err := e.SimulateNetwork(context.Background(), json.RawMessage(bad)); err == nil {
			t.Fatal("invalid request accepted")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := e.SimulateNetwork(ctx, json.RawMessage(request)); err != context.Canceled {
		t.Fatal("cancellation ignored", err)
	}
}
