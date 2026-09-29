// Cross-subsystem contract tests. Every V1
// subsystem exposes a JSON serialisation;
// the GUI, the CLI and the loopback API
// consume the same wire format. The tests
// pin the wire format: a regression in any
// field name or enum value would silently
// break the GUI/CLI.
package contracts

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/capture"
	"github.com/CrazyDuckLab/DuckOS/internal/devices"
	"github.com/CrazyDuckLab/DuckOS/internal/doctor"
	"github.com/CrazyDuckLab/DuckOS/internal/migrate"
	"github.com/CrazyDuckLab/DuckOS/internal/multiwan"
	"github.com/CrazyDuckLab/DuckOS/internal/qos"
	"github.com/CrazyDuckLab/DuckOS/internal/sites"
	"github.com/CrazyDuckLab/DuckOS/internal/ups"
	"github.com/CrazyDuckLab/DuckOS/internal/wifi"
)

// TestMultiWANStatusWireFormat pins the multi-WAN
// JSON envelope. A regression here would break
// the GUI failover view.
func TestMultiWANStatusWireFormat(t *testing.T) {
	g := multiwan.Group{ID: "wan-group", Name: "primary", Strategy: multiwan.StrategyActiveBackup, Members: []multiwan.Member{{ID: "wan-1", InterfaceID: "if-wan-1", Weight: 10}}, Probes: []multiwan.Probe{{Target: "1.1.1.1", Method: multiwan.HealthMethodICMP, Timeout: time.Second}}, Interval: 5 * time.Second, Table: 200, Primary: "wan-1"}
	tp, err := multiwan.Build([]multiwan.Group{g}, time.Unix(1, 0))
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	raw, err := json.Marshal(tp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, want := range []string{`"schemaVersion":"1"`, `"groups"`, `"id":"wan-group"`, `"members"`, `"probes"`, `"interval":5000`, `"table":200`} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("missing %q in %s", want, raw)
		}
	}
}

// TestQoSPlanWireFormat pins the QoS plan
// envelope. The Plan is what the LinuxRunner
// executes; a regression in the field names
// would break the Apply/Preview paths.
func TestQoSPlanWireFormat(t *testing.T) {
	sh := qos.Shaper{ID: "wan1", Link: "wan1", Direction: qos.DirectionEgress, Algorithm: qos.AlgorithmCAKE, RateKbps: 1000, BurstKbyte: 32, Priority: qos.PriorityVoice}
	tp, _ := qos.Build([]qos.Shaper{sh}, nil, nil, time.Unix(1, 0))
	p, _ := qos.Render(tp)
	raw, _ := p.Encode()
	// DuckOS Priority is translated to a CAKE
	// diffserv keyword by the renderer. Voice →
	// diffserv8, video → diffserv4, network →
	// diffserv3, best-effort/bulk → besteffort.
	for _, want := range []string{`"schemaVersion":"1"`, `"steps"`, `"kind":"qdisc"`, `"link":"wan1"`, `"cake"`, `"diffserv8"`, `"bandwidth"`, `"1000kbit"`} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("missing %q in %s", want, raw)
		}
	}
}

// TestUPSStatusWireFormat pins the UPS status
// envelope. The status is what the GUI uses to
// colour the device list.
func TestUPSStatusWireFormat(t *testing.T) {
	tp, _ := ups.Build([]ups.Device{{ID: "ups-1", Name: "Office UPS", Driver: "usbhid-ups", Port: "auto"}}, time.Unix(1, 0))
	st := ups.NewState(tp)
	st.RecordProbe("ups-1", ups.HealthOnline, []ups.Variable{{Key: "battery.charge", Value: "95"}}, time.Unix(7, 0))
	data, _ := json.Marshal(st.Status())
	for _, want := range []string{`"schemaVersion":"1"`, `"topology"`, `"health"`, `"ups-1":"online"`, `"variables"`, `"battery.charge"`, `"probedAt":7`} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("missing %q in %s", want, data)
		}
	}
}

// TestMigratePlanWireFormat pins the migration
// plan envelope. The plan is what the GUI/CLI
// shows the operator before any DuckOS object
// is created.
func TestMigratePlanWireFormat(t *testing.T) {
	src := `/ip address
add address=192.0.2.1/24 interface=ether1
add address=2001:db8::1/64 interface=ether1
set [find interface=ether1] disabled=no
`
	p, err := migrate.Analyse(migrate.Source{Filename: "router.rsc"}, []byte(src), time.Unix(1, 0))
	if err != nil {
		t.Fatalf("analyse: %v", err)
	}
	raw, _ := p.Encode()
	for _, want := range []string{`"schemaVersion":"1"`, `"format":"mikrotik"`, `"items"`, `"duckos.network.address"`, `"convertible"`} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("missing %q in %s", want, raw)
		}
	}
}

// TestSitesTopologyWireFormat pins the sites
// topology envelope.
func TestSitesTopologyWireFormat(t *testing.T) {
	s := sites.Site{ID: "blog", Name: "Personal blog", Domain: "blog.example.com", Listener: sites.Listener{InterfaceID: "wan1", Port: 443, Protocol: sites.ProtocolTLS}, Target: sites.Target{Host: "192.168.1.10", Port: 8080, Protocol: sites.ProtocolHTTP}, Enabled: true}
	tp, _ := sites.Build([]sites.Site{s}, time.Unix(1, 0))
	raw, _ := sites.EncodeTopology(tp)
	for _, want := range []string{`"schemaVersion":"1"`, `"sites"`, `"id":"blog"`, `"listener"`, `"wan1"`, `"target"`, `"192.168.1.10"`} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("missing %q in %s", want, raw)
		}
	}
}

// TestWifiTopologyWireFormat pins the Wi-Fi
// topology envelope.
func TestWifiTopologyWireFormat(t *testing.T) {
	a := wifi.AP{ID: "office-ap", Name: "Office AP", SSID: "DuckOS-WiFi", Interface: "wlan0", Band: wifi.Band5G, Channel: 36, Security: wifi.SecurityWPA2WPA3, PassphraseRef: "vault://office-passphrase", Country: "US", Enabled: true}
	tp, _ := wifi.Build([]wifi.AP{a}, time.Unix(1, 0))
	raw, _ := wifi.EncodeTopology(tp)
	for _, want := range []string{`"schemaVersion":"1"`, `"aps"`, `"id":"office-ap"`, `"DuckOS-WiFi"`, `"wlan0"`, `"5g"`, `"wpa2-wpa3"`} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("missing %q in %s", want, raw)
		}
	}
}

// TestDoctorSnapshotWireFormat pins the doctor
// snapshot envelope.
func TestDoctorSnapshotWireFormat(t *testing.T) {
	b := doctor.NewBuilder("test", "duckos-dev", "boot-1", time.Unix(1, 0).UTC(), 0)
	snap := b.Snapshot()
	raw, _ := json.Marshal(snap)
	for _, want := range []string{`"schemaVersion"`, `"generatedAt"`, `"environment"`, `"test"`, `"subsystems"`} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("missing %q in %s", want, raw)
		}
	}
}

// TestDevicesCatalogWireFormat pins the
// devices catalog envelope. The catalog is what
// the GUI shows in the topology view.
func TestDevicesCatalogWireFormat(t *testing.T) {
	c := devices.NewCatalog()
	if _, err := c.Record(devices.Observation{MAC: "aa:bb:cc:dd:ee:ff", Address: "192.168.1.10", Link: "br0", At: time.Unix(1, 0)}); err != nil {
		t.Fatalf("record: %v", err)
	}
	raw, _ := json.Marshal(c.ListDevices())
	for _, want := range []string{`"aa:bb:cc:dd:ee:ff"`, `"192.168.1.10"`, `"br0"`} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("missing %q in %s", want, raw)
		}
	}
}

// TestCaptureRequestWireFormat pins the capture
// request envelope.
func TestCaptureRequestWireFormat(t *testing.T) {
	r := capture.Request{ID: "cap-1", Link: "wan1", Operator: "operator-1", MaxBytes: 1 << 20, Filter: "tcp port 443"}
	raw, _ := json.Marshal(r)
	for _, want := range []string{`"id":"cap-1"`, `"link":"wan1"`, `"operator":"operator-1"`, `"maxBytes":1048576`, `"filter":"tcp port 443"`} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("missing %q in %s", want, raw)
		}
	}
}

// TestAllSchemasUseSchemaVersion pins the
// "schemaVersion" field convention. Every
// V1 subsystem must carry a schemaVersion
// field so the wire format can evolve
// safely.
func TestAllSchemasUseSchemaVersion(t *testing.T) {
	cases := []struct {
		name string
		raw  []byte
	}{
		{"multiwan", mustMarshal(multiwan.Topology{SchemaVersion: multiwan.SchemaVersion})},
		{"qos", mustMarshal(qos.Topology{SchemaVersion: qos.SchemaVersion})},
		{"ups", mustMarshal(ups.Topology{SchemaVersion: ups.SchemaVersion})},
		{"migrate", mustMarshal(migrate.Plan{SchemaVersion: migrate.SchemaVersion})},
		{"sites", mustMarshal(sites.Topology{SchemaVersion: sites.SchemaVersion})},
		{"wifi", mustMarshal(wifi.Topology{SchemaVersion: wifi.SchemaVersion})},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if !strings.Contains(string(tc.raw), `"schemaVersion"`) {
				t.Fatalf("expected schemaVersion in %s", tc.raw)
			}
		})
	}
}

func mustMarshal(v any) []byte {
	raw, _ := json.Marshal(v)
	return raw
}
