package network

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestEnrolledFirewallChecksDriftButStillAttemptsRecovery(t *testing.T) {
	e := syntheticEnrollment()
	h := &syntheticFirewallKernel{}
	var missing bool
	a := &EnrolledFirewallAdapter{inner: &FirewallAdapter{host: h}, enrollment: e,
		captureHost: func(context.Context, Config, Config) (json.RawMessage, error) {
			if missing {
				return nil, errors.New("synthetic missing NIC")
			}
			return json.Marshal(HostSnapshot{Version: 1, CapturedAt: time.Now().UTC(), Links: []HostLink{
				{Name: "testwan0", Index: 2, MAC: e.Physical[0].MAC, MTU: 1500},
				{Name: "testlan0", Index: 3, MAC: e.Physical[1].MAC, MTU: 1500},
			}})
		},
		networkdInventory: func(context.Context) ([]NetworkdLink, error) { return syntheticNetworkdInventory(), nil },
	}
	before, _ := json.Marshal(fixture())
	nextConfig := fixture()
	nextConfig.Firewall.Forwards[0].Port = 9443
	next, _ := json.Marshal(nextConfig)
	ctx := context.Background()
	if err := a.Preflight(ctx, before, next); err != nil {
		t.Fatal(err)
	}
	evidence, err := a.Capture(ctx, before, next)
	if err != nil {
		t.Fatal(err)
	}
	missing = true
	if err := a.Apply(ctx, before, next); err == nil || h.applies != 0 {
		t.Fatal("missing enrolled NIC did not block mutation")
	}
	missing = false
	if err := a.Apply(ctx, before, next); err != nil {
		t.Fatal(err)
	}
	missing = true
	if err := a.RestoreCaptured(ctx, before, next, evidence); err != nil {
		t.Fatal("missing NIC blocked table restoration", err)
	}
	if len(h.live.Objects) != 0 {
		t.Fatal("owned firewall was not restored")
	}
	if err := a.Verify(ctx, before); err == nil {
		t.Fatal("missing NIC reported healthy recovery")
	}
}
