// wifi_test.go covers the model layer of the
// Wi-Fi subsystem.
package wifi

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func sampleAP() AP {
	return AP{ID: "office-ap", Name: "Office AP", SSID: "DuckOS-WiFi", Interface: "wlan0", Band: Band5G, Channel: 36, Security: SecurityWPA2WPA3, PassphraseRef: "vault://office-passphrase", Country: "US", Enabled: true}
}

func TestAPValid(t *testing.T) {
	if err := sampleAP().Valid(); err != nil {
		t.Fatalf("expected base ap to validate: %v", err)
	}
	cases := []struct {
		name    string
		mut     func(*AP)
		wantErr string
	}{
		{"bad id", func(a *AP) { a.ID = "9" }, "id is not canonical"},
		{"empty name", func(a *AP) { a.Name = "" }, "name is empty or oversized"},
		{"oversized name", func(a *AP) { a.Name = strings.Repeat("a", MaxNameLen+1) }, "name is empty or oversized"},
		{"bad ssid control char", func(a *AP) { a.SSID = "with\x01ctrl" }, "ssid is not canonical"},
		{"bad interface", func(a *AP) { a.Interface = "9bad" }, "interface is not canonical"},
		{"bad band", func(a *AP) { a.Band = "60ghz" }, "band is not canonical"},
		{"bad security", func(a *AP) { a.Security = "wep" }, "security is not canonical"},
		{"passphrase required", func(a *AP) { a.PassphraseRef = "" }, "passphraseRef is required"},
		{"bad passphrase", func(a *AP) { a.PassphraseRef = "x;rm -rf /" }, "shell metacharacter"},
		{"bad country", func(a *AP) { a.Country = "USA" }, "country is not a canonical country code"},
		{"bad channel 2g", func(a *AP) { a.Band = Band2G; a.Channel = 200 }, "channel is not allowed for the band"},
		{"bad channel 5g", func(a *AP) { a.Band = Band5G; a.Channel = 200 }, "channel is not allowed for the band"},
		{"bad channel 6g", func(a *AP) { a.Band = Band6G; a.Channel = 0 }, "channel is not allowed for the band"},
		{"ok 2g channel 6", func(a *AP) { a.Band = Band2G; a.Channel = 6 }, ""},
		{"ok 5g channel 149", func(a *AP) { a.Band = Band5G; a.Channel = 149 }, ""},
		{"ok 6g channel 33", func(a *AP) { a.Band = Band6G; a.Channel = 33 }, ""},
		{"description oversized", func(a *AP) { a.Description = strings.Repeat("a", 257) }, "description is oversized"},
		{"open security no passphrase", func(a *AP) { a.Security = SecurityOpen; a.PassphraseRef = "" }, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := sampleAP()
			tc.mut(&a)
			err := a.Valid()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected %q, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestCapabilityValid(t *testing.T) {
	unsupported := Capability{Supported: false}
	if err := unsupported.Valid(); err != nil {
		t.Fatalf("expected unsupported to validate: %v", err)
	}
	supported := Capability{Supported: true, Driver: "iwlwifi", Interface: "wlan0", Bands: []Band{Band5G}, RegDomain: "US"}
	if err := supported.Valid(); err != nil {
		t.Fatalf("expected supported to validate: %v", err)
	}
	noDriver := Capability{Supported: true}
	if err := noDriver.Valid(); err == nil {
		t.Fatalf("expected missing driver to fail")
	}
	badInterface := Capability{Supported: true, Driver: "iwlwifi", Interface: "9bad"}
	if err := badInterface.Valid(); err == nil {
		t.Fatalf("expected bad interface to fail")
	}
	badReg := Capability{Supported: true, Driver: "iwlwifi", RegDomain: "USA"}
	if err := badReg.Valid(); err == nil {
		t.Fatalf("expected bad regDomain to fail")
	}
}

func TestStationValid(t *testing.T) {
	good := Station{ID: "st-1", APID: "office-ap", MAC: "aa:bb:cc:dd:ee:ff", SignalDbm: -65, ConnectedAt: 1}
	if err := good.Valid(); err != nil {
		t.Fatalf("expected base station to validate: %v", err)
	}
	cases := []struct {
		name    string
		mut     func(*Station)
		wantErr string
	}{
		{"bad id", func(s *Station) { s.ID = "9" }, "id is not canonical"},
		{"bad ap", func(s *Station) { s.APID = "9" }, "apId is not canonical"},
		{"bad mac", func(s *Station) { s.MAC = "aa:bb" }, "mac is not canonical"},
		{"signal too low", func(s *Station) { s.SignalDbm = -200 }, "signal out of range"},
		{"signal too high", func(s *Station) { s.SignalDbm = 50 }, "signal out of range"},
		{"negative connectedAt", func(s *Station) { s.ConnectedAt = -1 }, "connectedAt is negative"},
		{"hostname oversized", func(s *Station) { s.Hostname = strings.Repeat("a", 65) }, "hostname is oversized"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := good
			tc.mut(&s)
			err := s.Valid()
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected %q, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestTopologyValid(t *testing.T) {
	if _, err := Build([]AP{sampleAP()}, time.Unix(1, 0)); err != nil {
		t.Fatalf("expected build to succeed, got %v", err)
	}
	wrong := Topology{SchemaVersion: "0", APs: []AP{sampleAP()}}
	if err := wrong.Valid(); err == nil {
		t.Fatalf("expected schema version error")
	}
	dup := Topology{SchemaVersion: SchemaVersion, APs: []AP{sampleAP(), sampleAP()}}
	if err := dup.Valid(); err == nil {
		t.Fatalf("expected duplicate id error")
	}
}

func TestBuildSorts(t *testing.T) {
	a1 := sampleAP()
	a2 := sampleAP()
	a2.ID = "z-second"
	tp, err := Build([]AP{a2, a1}, time.Unix(1, 0))
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if tp.APs[0].ID != "office-ap" || tp.APs[1].ID != "z-second" {
		t.Fatalf("expected sorted aps, got %+v", tp.APs)
	}
}

func TestEncodeDecodeRoundtrip(t *testing.T) {
	tp, err := Build([]AP{sampleAP()}, time.Unix(1, 0))
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	data, err := EncodeTopology(tp)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	got, err := ParseTopology(data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	da, _ := json.Marshal(tp)
	db, _ := json.Marshal(got)
	if !bytes.Equal(da, db) {
		t.Fatalf("roundtrip mismatch")
	}
	// unknown field
	bad := []byte(`{"schemaVersion":"1","aps":[{"id":"x","name":"y","ssid":"z","band":"5g","channel":36,"security":"open","bogus":1}]}`)
	if _, err := ParseTopology(bad); err == nil {
		t.Fatalf("expected parse to fail on unknown field")
	}
	// truncated
	if _, err := ParseTopology(data[:5]); err == nil {
		t.Fatalf("expected parse to fail on truncated")
	}
}

func TestSummarise(t *testing.T) {
	tp, _ := Build([]AP{sampleAP()}, time.Unix(1, 0))
	sum := Summarise(tp, map[string]Health{"office-ap": HealthEnabled}, map[string]int{"office-ap": 4})
	if len(sum) != 1 {
		t.Fatalf("expected 1 summary, got %d", len(sum))
	}
	if sum[0].Health != HealthEnabled || sum[0].Stations != 4 {
		t.Fatalf("unexpected summary: %+v", sum[0])
	}
	// disabled
	disabled := sampleAP()
	disabled.ID = "off"
	disabled.Enabled = false
	tp2, _ := Build([]AP{disabled}, time.Unix(1, 0))
	sum2 := Summarise(tp2, nil, nil)
	if sum2[0].Health != HealthDisabled {
		t.Fatalf("expected disabled, got %s", sum2[0].Health)
	}
}

func TestIsLegacy(t *testing.T) {
	if !IsLegacy(SecurityOpen) {
		t.Fatalf("expected open to be legacy")
	}
	for _, s := range []Security{SecurityWPA2PSK, SecurityWPA3SAE, SecurityWPA2WPA3, SecurityWPA3Enterprise} {
		if IsLegacy(s) {
			t.Fatalf("expected %s to not be legacy", s)
		}
	}
}

func TestStatusEncode(t *testing.T) {
	tp, _ := Build([]AP{sampleAP()}, time.Unix(1, 0))
	st := Status{Topology: tp, Health: map[string]Health{"office-ap": HealthEnabled}, Stations: map[string]int{"office-ap": 2}, Capability: Capability{Supported: true, Driver: "iwlwifi", Interface: "wlan0", Bands: []Band{Band5G}}, HasAPs: 1, ProbedAt: 7}
	data, err := st.Encode()
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if len(data) > MaxStatusBytes {
		t.Fatalf("status exceeds cap")
	}
}
