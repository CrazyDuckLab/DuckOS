package network

import (
	"encoding/json"
	"testing"
)

func syntheticNetworkdInventory() []NetworkdLink {
	return []NetworkdLink{
		{Index: 2, Name: "testwan0", AdministrativeState: "configured", NetworkFile: "/etc/systemd/network/10-duckos-uplink.network"},
		{Index: 3, Name: "testlan0", AdministrativeState: "configured", NetworkFile: "/etc/systemd/network/10-duckos-inside.network"},
	}
}

func TestNetworkdProfileOwnershipRefusesForeignAndPartialState(t *testing.T) {
	c := fixture()
	if err := CheckNetworkdOwnership(c, c, syntheticNetworkdInventory()); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func([]NetworkdLink) []NetworkdLink{
		"foreign winning file": func(i []NetworkdLink) []NetworkdLink {
			i[0].NetworkFile = "/run/systemd/network/05-foreign.network"
			return i
		},
		"dropin": func(i []NetworkdLink) []NetworkdLink {
			i[0].NetworkFileDropins = []string{"/etc/systemd/network/10-duckos-uplink.network.d/override.conf"}
			return i
		},
		"configuring":          func(i []NetworkdLink) []NetworkdLink { i[0].AdministrativeState = "configuring"; return i },
		"failed":               func(i []NetworkdLink) []NetworkdLink { i[0].AdministrativeState = "failed"; return i },
		"foreign virtual type": func(i []NetworkdLink) []NetworkdLink { i[0].Kind = "bridge"; return i },
		"missing":              func(i []NetworkdLink) []NetworkdLink { return i[:1] },
	} {
		t.Run(name, func(t *testing.T) {
			if CheckNetworkdOwnership(c, c, mutate(syntheticNetworkdInventory())) == nil {
				t.Fatal("unsafe ownership accepted")
			}
		})
	}
	newConfig := fixture()
	newConfig.Interfaces = append(newConfig.Interfaces, Interface{ID: "guest", Name: "testguest0", Kind: "vlan", Parent: "inside", VLAN: 20, MTU: 1500, Role: "lan", DHCPClient: "none", Addresses: []string{"203.0.113.1/24"}})
	if err := CheckNetworkdOwnership(c, newConfig, syntheticNetworkdInventory()); err != nil {
		t.Fatal(err)
	}
	collision := append(syntheticNetworkdInventory(), NetworkdLink{Index: 4, Name: "testguest0", Kind: "vlan"})
	if CheckNetworkdOwnership(c, newConfig, collision) == nil {
		t.Fatal("existing VLAN silently adopted")
	}
	newConfig.Interfaces[0].Kind, newConfig.Interfaces[0].Parent, newConfig.Interfaces[0].VLAN = "vlan", "inside", 10
	if CheckNetworkdOwnership(c, newConfig, syntheticNetworkdInventory()) == nil {
		t.Fatal("existing physical link reused as VLAN")
	}
}

func TestNetworkdInventoryBoundedShape(t *testing.T) {
	raw, _ := json.Marshal(map[string]any{"Interfaces": syntheticNetworkdInventory(), "FutureSystemdField": true})
	if _, err := DecodeNetworkdInventory(raw); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{`{}`, `{"Interfaces":null}`, `{"Interfaces":[{"Name":"x","Index":0}]}`, `{"Interfaces":[{"Name":"x","Index":2},{"Name":"x","Index":3}]}`, `{"Interfaces":[]} {}`} {
		if _, err := DecodeNetworkdInventory([]byte(raw)); err == nil {
			t.Fatal("invalid inventory accepted", raw)
		}
	}
	if _, err := DecodeNetworkdInventory(make([]byte, (2<<20)+1)); err == nil {
		t.Fatal("oversized inventory accepted")
	}
	var out boundedNetworkdOutput
	if _, err := out.Write(make([]byte, (2<<20)+1)); err == nil || out.Len() != 0 {
		t.Fatal("command output was not bounded")
	}
}

func TestNewPhysicalNetworkdEnrollmentRequiresUnmanagedState(t *testing.T) {
	live := syntheticNetworkdInventory()
	if CheckNetworkdOwnership(Config{Version: 1}, fixture(), live) == nil {
		t.Fatal("existing profiles adopted by empty intent")
	}
	for i := range live {
		live[i].NetworkFile = ""
		live[i].AdministrativeState = "unmanaged"
	}
	if err := CheckNetworkdOwnership(Config{Version: 1}, fixture(), live); err != nil {
		t.Fatal(err)
	}
}
