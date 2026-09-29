// vlan/vlan_test.go pins the V1 VLAN/QinQ
// model.
package vlan

import (
	"strings"
	"testing"
)

func TestPortValid(t *testing.T) {
	good := Port{Link: "eth0", Bridge: "br0", Mode: PortAccess, PVID: 10}
	if err := good.Valid(); err != nil {
		t.Fatal(err)
	}
	trunk := Port{Link: "eth0", Bridge: "br0", Mode: PortTrunk, PVID: 1, Native: 1, Tagged: []int{10, 20}}
	if err := trunk.Valid(); err != nil {
		t.Fatal(err)
	}
	badPVID := Port{Link: "eth0", Bridge: "br0", Mode: PortAccess, PVID: 5000}
	if err := badPVID.Valid(); err == nil {
		t.Fatal("pvid out of range must fail")
	}
	qinq := Port{Link: "eth0", Bridge: "br0", Mode: PortQinQ, PVID: 10, QinQServiceTag: 100}
	if err := qinq.Valid(); err != nil {
		t.Fatal(err)
	}
	// qinq tag without qinq mode must fail.
	wrong := Port{Link: "eth0", Bridge: "br0", Mode: PortAccess, PVID: 10, QinQServiceTag: 100}
	if err := wrong.Valid(); err == nil {
		t.Fatal("qinq tag without qinq mode must fail")
	}
}

func TestBridgeValid(t *testing.T) {
	good := Bridge{ID: "br-1", Name: "br0", Ports: []Port{{Link: "eth0", Bridge: "br0", Mode: PortAccess, PVID: 10}}}
	if err := good.Valid(); err != nil {
		t.Fatal(err)
	}
}

func TestValidateTopologyDetectsIssues(t *testing.T) {
	m := NewManager()
	trunkNoNative := Bridge{ID: "br-1", Name: "br0", Ports: []Port{{Link: "eth0", Bridge: "br0", Mode: PortTrunk, PVID: 1, Tagged: []int{10}}}}
	if err := m.Set(trunkNoNative); err != nil {
		t.Fatal(err)
	}
	warnings := m.ValidateTopology()
	has := false
	for _, w := range warnings {
		if strings.Contains(w, "without a native VLAN") {
			has = true
		}
	}
	if !has {
		t.Fatalf("trunk without native must warn: %+v", warnings)
	}
}

func TestValidateTopologyIsolatedPorts(t *testing.T) {
	m := NewManager()
	multi := Bridge{ID: "br-1", Name: "br0", Ports: []Port{
		{Link: "eth0", Bridge: "br0", Mode: PortAccess, PVID: 10, Isolated: true},
		{Link: "eth1", Bridge: "br0", Mode: PortAccess, PVID: 10, Isolated: true},
	}}
	if err := m.Set(multi); err != nil {
		t.Fatal(err)
	}
	warnings := m.ValidateTopology()
	has := false
	for _, w := range warnings {
		if strings.Contains(w, "isolated ports") {
			has = true
		}
	}
	if !has {
		t.Fatalf("multiple isolated ports must warn: %+v", warnings)
	}
}

func TestManagerSetGetListDelete(t *testing.T) {
	m := NewManager()
	b := Bridge{ID: "br-1", Name: "br0", Ports: []Port{{Link: "eth0", Bridge: "br0", Mode: PortAccess, PVID: 10}}}
	if err := m.Set(b); err != nil {
		t.Fatal(err)
	}
	if _, ok := m.Get("br-1"); !ok {
		t.Fatal("Get must find the bridge")
	}
	m.Delete("br-1")
	if _, ok := m.Get("br-1"); ok {
		t.Fatal("Get after Delete must miss")
	}
}
