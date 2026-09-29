package network

import (
	"context"
	"encoding/json"
	"net/netip"
	"testing"
)

func managementFixture(t *testing.T) (ManagementAuthority, Config, ManagementEvidence) {
	t.Helper()
	e := syntheticEnrollment()
	authority := NewManagementAuthority(e)
	cfg, err := Decode(mustMarshal(t, managementAuthorityConfig()))
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	evidence := ManagementEvidence{
		Links: []ManagementLinkEvidence{
			{Name: "testwan0", Role: "wan", Up: true, Agent: true, HasMAC: true, MTU: 1500},
			{Name: "testlan0", Role: "lan", Up: true, Agent: true, HasMAC: true, MTU: 1500},
		},
		MainAddresses: []ManagementAddressEvidence{
			{Link: "testwan0", Addr: netip.MustParsePrefix("192.0.2.10/24"), Agent: true},
			{Link: "testlan0", Addr: netip.MustParsePrefix("198.51.100.1/24"), Agent: true},
		},
		Listeners: []ManagementListenerEvidence{
			{Link: "testlan0", Address: netip.MustParseAddr("198.51.100.1"), Port: 443, Proto: "tcp", Service: "api", Agent: true},
		},
		OwnedNeighbors: map[string]bool{},
	}
	return authority, cfg, evidence
}

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func managementAuthorityConfig() any {
	return map[string]any{
		"version": 1,
		"interfaces": []map[string]any{
			{"id": "uplink", "name": "testwan0", "kind": "physical", "role": "wan", "mtu": 1500, "dhcpClient": "none", "management": false, "addresses": []string{"192.0.2.10/24"}},
			{"id": "inside", "name": "testlan0", "kind": "physical", "role": "lan", "mtu": 1500, "dhcpClient": "none", "management": true, "addresses": []string{"198.51.100.1/24"}},
		},
	}
}

func TestVerifyManagementOwnershipAcceptsMatchingEvidence(t *testing.T) {
	authority, cfg, evidence := managementFixture(t)
	if err := VerifyManagementOwnership(context.Background(), authority, cfg, evidence); err != nil {
		t.Fatal(err)
	}
}

func TestVerifyManagementOwnershipRejectsMissingLink(t *testing.T) {
	authority, cfg, evidence := managementFixture(t)
	evidence.Links = evidence.Links[:1]
	if err := VerifyManagementOwnership(context.Background(), authority, cfg, evidence); err == nil {
		t.Fatal("missing link was not rejected")
	}
}

func TestVerifyManagementOwnershipRejectsDownLink(t *testing.T) {
	authority, cfg, evidence := managementFixture(t)
	evidence.Links[1].Up = false
	if err := VerifyManagementOwnership(context.Background(), authority, cfg, evidence); err == nil {
		t.Fatal("down link was not rejected")
	}
}

func TestVerifyManagementOwnershipRejectsUnenrolledLink(t *testing.T) {
	authority, cfg, evidence := managementFixture(t)
	evidence.Links[1].Agent = false
	if err := VerifyManagementOwnership(context.Background(), authority, cfg, evidence); err == nil {
		t.Fatal("non-agent link was not rejected")
	}
}

func TestVerifyManagementOwnershipRejectsMTUDrift(t *testing.T) {
	authority, cfg, evidence := managementFixture(t)
	evidence.Links[1].MTU = 1400
	if err := VerifyManagementOwnership(context.Background(), authority, cfg, evidence); err == nil {
		t.Fatal("MTU drift was not rejected")
	}
}

func TestVerifyManagementOwnershipRejectsForeignMainRoute(t *testing.T) {
	authority, cfg, evidence := managementFixture(t)
	evidence.MainRoutes = []KernelRoute{{Family: "ipv4", Table: 0, Destination: "0.0.0.0/0", Interface: "testlan0", Gateway: "198.51.100.254", Metric: 50}}
	if err := VerifyManagementOwnership(context.Background(), authority, cfg, evidence); err == nil {
		t.Fatal("foreign main route was not rejected")
	}
}

func TestVerifyManagementOwnershipRejectsForeignAddress(t *testing.T) {
	authority, cfg, evidence := managementFixture(t)
	evidence.MainAddresses = append(evidence.MainAddresses, ManagementAddressEvidence{Link: "testlan0", Addr: netip.MustParsePrefix("198.51.100.99/24"), Agent: false})
	if err := VerifyManagementOwnership(context.Background(), authority, cfg, evidence); err == nil {
		t.Fatal("foreign address was not rejected")
	}
}

func TestVerifyManagementOwnershipRejectsForeignListener(t *testing.T) {
	authority, cfg, evidence := managementFixture(t)
	evidence.Listeners = append(evidence.Listeners, ManagementListenerEvidence{Link: "testlan0", Address: netip.MustParseAddr("198.51.100.1"), Port: 22, Proto: "tcp", Service: "ssh", Agent: false})
	if err := VerifyManagementOwnership(context.Background(), authority, cfg, evidence); err == nil {
		t.Fatal("foreign listener was not rejected")
	}
}

func TestVerifyManagementOwnershipRejectsUnauthorizedService(t *testing.T) {
	authority, cfg, evidence := managementFixture(t)
	evidence.Listeners = append(evidence.Listeners, ManagementListenerEvidence{Link: "testlan0", Address: netip.MustParseAddr("198.51.100.1"), Port: 80, Proto: "tcp", Service: "telnet", Agent: false})
	if err := VerifyManagementOwnership(context.Background(), authority, cfg, evidence); err == nil {
		t.Fatal("unauthorized service listener was not rejected")
	}
}

func TestVerifyManagementOwnershipAcceptsAgentListeners(t *testing.T) {
	authority, cfg, evidence := managementFixture(t)
	evidence.Listeners = append(evidence.Listeners,
		ManagementListenerEvidence{Link: "testlan0", Address: netip.MustParseAddr("198.51.100.1"), Port: 22, Proto: "tcp", Service: "ssh", Agent: true},
		ManagementListenerEvidence{Link: "testlan0", Address: netip.MustParseAddr("198.51.100.1"), Port: 80, Proto: "tcp", Service: "web", Agent: true},
	)
	if err := VerifyManagementOwnership(context.Background(), authority, cfg, evidence); err != nil {
		t.Fatal("agent listeners on a managed link were refused", err)
	}
}

func TestVerifyManagementOwnershipRejectsForeignLLDPNeighbor(t *testing.T) {
	authority, cfg, evidence := managementFixture(t)
	evidence.LLDPNeighbors = []ManagementNeighborEvidence{{Link: "testlan0", Chassis: "chassis-foreign", System: "switch-foreign", Port: "Gi0/1", Foreign: true}}
	if err := VerifyManagementOwnership(context.Background(), authority, cfg, evidence); err == nil {
		t.Fatal("foreign LLDP neighbor was not rejected")
	}
}

func TestVerifyManagementOwnershipAllowsRegisteredLLDPNeighbor(t *testing.T) {
	authority, cfg, evidence := managementFixture(t)
	evidence.LLDPNeighbors = []ManagementNeighborEvidence{{Link: "testlan0", Chassis: "chassis-known", System: "switch-known", Port: "Gi0/2", Foreign: true}}
	evidence.OwnedNeighbors = map[string]bool{"chassis-known": true}
	if err := VerifyManagementOwnership(context.Background(), authority, cfg, evidence); err != nil {
		t.Fatal("registered LLDP neighbor was refused", err)
	}
}

func TestVerifyManagementOwnershipRejectsReservedNameCollision(t *testing.T) {
	authority, cfg, evidence := managementFixture(t)
	evidence.Links = append(evidence.Links, ManagementLinkEvidence{Name: "testguest0", Role: "guest", Up: true, Agent: false, HasMAC: true, MTU: 1500})
	if err := VerifyManagementOwnership(context.Background(), authority, cfg, evidence); err == nil {
		t.Fatal("reserved name collision was not rejected")
	}
}

func TestVerifyManagementOwnershipRejectsInvalidConfig(t *testing.T) {
	authority, _, evidence := managementFixture(t)
	bad := Config{}
	if err := VerifyManagementOwnership(context.Background(), authority, bad, evidence); err == nil {
		t.Fatal("empty config was not rejected")
	}
}

// TestVerifyManagementOwnershipAcceptsEmptyFirstBootBaseline pins the
// fresh-install first-boot semantics: a candidate whose durable Config
// declares no managed interfaces (Interfaces=[]) is vacuously safe.
// The host-plane check still fires — reserved names declared by the
// root enrollment must be absent from the live host — but there is
// nothing per-link to attest when the candidate carries no links.
// This is the gate that lets the very first Safe Apply introduce a
// DuckOS-managed interface through the adoption boundary.
func TestVerifyManagementOwnershipAcceptsEmptyFirstBootBaseline(t *testing.T) {
	authority, _, _ := managementFixture(t)
	empty := Config{Version: 1}
	evidence := ManagementEvidence{
		Links: []ManagementLinkEvidence{
			{Name: "testwan0", Role: "wan", Up: true, Agent: false, HasMAC: true, MTU: 1500},
			{Name: "testlan0", Role: "lan", Up: true, Agent: false, HasMAC: true, MTU: 1500},
		},
	}
	if err := VerifyManagementOwnership(context.Background(), authority, empty, evidence); err != nil {
		t.Fatalf("empty first-boot config was refused: %v", err)
	}
}

// TestVerifyManagementOwnershipRejectsReservedNameOnEmptyConfig
// proves that the host-plane check fires even when the candidate
// declares no managed interfaces. A reserved name squatting on the
// host is a parallel management plane and must be refused regardless
// of the durable Config shape.
func TestVerifyManagementOwnershipRejectsReservedNameOnEmptyConfig(t *testing.T) {
	e := syntheticEnrollment()
	authority := NewManagementAuthority(e)
	authority.Reserved["testlan0"] = true
	empty := Config{Version: 1}
	evidence := ManagementEvidence{
		Links: []ManagementLinkEvidence{
			{Name: "testlan0", Role: "lan", Up: true, Agent: false, HasMAC: true, MTU: 1500},
		},
	}
	if err := VerifyManagementOwnership(context.Background(), authority, empty, evidence); err == nil {
		t.Fatal("reserved name squatting on host was not rejected against empty cfg")
	}
}

func TestFormatRouteListIsBounded(t *testing.T) {
	routes := []KernelRoute{
		{Family: "ipv4", Destination: "10.0.0.0/8", Interface: "testwan0", Gateway: "192.0.2.1", Metric: 50},
		{Family: "ipv4", Destination: "0.0.0.0/0", Interface: "testwan0", Gateway: "192.0.2.1", Metric: 100},
	}
	got := formatRouteList(routes)
	if got == "" || got[0] != '[' {
		t.Fatal("route list formatting wrong", got)
	}
}
