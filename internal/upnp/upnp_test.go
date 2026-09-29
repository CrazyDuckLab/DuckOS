// upnp/upnp_test.go pins the V1 UPnP/PCP
// model.
package upnp

import "testing"
import "time"

func TestNetworkValid(t *testing.T) {
	good := Network{NetworkRef: "lan", Protocols: []Protocol{ProtoUPnP, ProtoPCP}, TTLSec: 3600}
	if err := good.Valid(); err != nil {
		t.Fatal(err)
	}
	noProto := Network{NetworkRef: "lan", TTLSec: 3600}
	if err := noProto.Valid(); err == nil {
		t.Fatal("at least one protocol must fail")
	}
}

func TestMappingValid(t *testing.T) {
	good := Mapping{ID: "m-1", NetworkRef: "lan", Protocol: ProtoUPnP, ClientIP: "192.168.1.10", ExternalPort: 5000, InternalPort: 5000, InternalHost: "192.168.1.10", CreatedAt: time.Now()}
	if err := good.Valid(); err != nil {
		t.Fatal(err)
	}
	bad := Mapping{ID: "m-1", NetworkRef: "lan", Protocol: "garbage"}
	if err := bad.Valid(); err == nil {
		t.Fatal("unknown protocol must fail")
	}
}

func TestManagerDefaultOff(t *testing.T) {
	m := NewManager()
	if m.IsEnabled("lan") {
		t.Fatal("default must be off")
	}
}

func TestManagerEnableAndMappings(t *testing.T) {
	m := NewManager()
	if err := m.EnableNetwork(Network{NetworkRef: "lan", Protocols: []Protocol{ProtoUPnP}, TTLSec: 3600}); err != nil {
		t.Fatal(err)
	}
	if !m.IsEnabled("lan") {
		t.Fatal("after Enable, IsEnabled must be true")
	}
	if err := m.AddMapping(Mapping{ID: "m-1", NetworkRef: "lan", Protocol: ProtoUPnP, ClientIP: "192.168.1.10", ExternalPort: 5000, InternalPort: 5000, InternalHost: "192.168.1.10", CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if got := len(m.Mappings()); got != 1 {
		t.Fatalf("Mappings count: %d", got)
	}
	m.RevokeMapping("m-1")
	if got := len(m.Mappings()); got != 0 {
		t.Fatalf("after Revoke, Mappings count: %d", got)
	}
	m.DisableNetwork("lan")
	if m.IsEnabled("lan") {
		t.Fatal("after Disable, IsEnabled must be false")
	}
}
