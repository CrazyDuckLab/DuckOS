// sites_test.go covers the model layer of the
// sites subsystem. The renderer and controller
// live in their own files.
package sites

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func sampleSite() Site {
	return Site{ID: "blog", Name: "Personal blog", Domain: "blog.example.com", Listener: Listener{InterfaceID: "wan1", Port: 443, Protocol: ProtocolTLS}, Target: Target{Host: "192.168.1.10", Port: 8080, Protocol: ProtocolHTTP}, TLS: TLS{Enabled: true, CertRef: "vault://blog-cert", KeyRef: "vault://blog-key", SNI: "blog.example.com"}, DDNS: DDNS{Provider: DDNSProviderDuckDNS, Hostname: "blog.duckdns.org", RefreshMins: 5, SecretRef: "vault://ddns-token"}, Enabled: true}
}

func TestSiteValid(t *testing.T) {
	if err := sampleSite().Valid(); err != nil {
		t.Fatalf("expected base site to validate: %v", err)
	}
	cases := []struct {
		name    string
		mut     func(*Site)
		wantErr string
	}{
		{"bad id", func(s *Site) { s.ID = "9" }, "id is not canonical"},
		{"empty name", func(s *Site) { s.Name = "" }, "name is empty or oversized"},
		{"oversized name", func(s *Site) { s.Name = strings.Repeat("a", MaxNameLen+1) }, "name is empty or oversized"},
		{"bad domain", func(s *Site) { s.Domain = "-bad" }, "domain is not a canonical hostname"},
		{"bad listener interface", func(s *Site) { s.Listener.InterfaceID = "9bad" }, "interfaceId is not canonical"},
		{"bad listener port", func(s *Site) { s.Listener.Port = 0 }, "port is out of range"},
		{"bad listener port high", func(s *Site) { s.Listener.Port = 70000 }, "port is out of range"},
		{"bad listener protocol", func(s *Site) { s.Listener.Protocol = "sctp" }, "protocol is not canonical"},
		{"bad target host", func(s *Site) { s.Target.Host = "" }, "target host is empty"},
		{"bad target host shell", func(s *Site) { s.Target.Host = "127.0.0.1; rm -rf /" }, "shell metacharacter"},
		{"bad target port", func(s *Site) { s.Target.Port = 0 }, "target port is out of range"},
		{"bad target protocol", func(s *Site) { s.Target.Protocol = "sctp" }, "target protocol is not canonical"},
		{"tls without cert", func(s *Site) { s.TLS.Enabled = true; s.TLS.CertRef = "" }, "certRef is required"},
		{"tls without key", func(s *Site) { s.TLS.Enabled = true; s.TLS.KeyRef = "" }, "keyRef is required"},
		{"bad tls sni", func(s *Site) { s.TLS.SNI = "-bad" }, "tls sni is not a canonical hostname"},
		{"ddns without hostname", func(s *Site) { s.DDNS.Provider = DDNSProviderDuckDNS; s.DDNS.Hostname = "" }, "ddns hostname is required"},
		{"bad ddns refresh", func(s *Site) { s.DDNS.RefreshMins = 0 }, "ddns refreshMins is out of range"},
		{"description oversized", func(s *Site) { s.Description = strings.Repeat("a", 257) }, "site description is oversized"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := sampleSite()
			tc.mut(&s)
			err := s.Valid()
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected %q, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestTopologyValid(t *testing.T) {
	if _, err := Build([]Site{sampleSite()}, time.Unix(1, 0)); err != nil {
		t.Fatalf("expected build to succeed, got %v", err)
	}
	if _, err := Build(nil, time.Now()); err == nil {
		t.Fatalf("expected build to fail on empty")
	}
	s2 := sampleSite()
	s2.ID = "blog"
	if _, err := Build([]Site{sampleSite(), s2}, time.Now()); err == nil {
		t.Fatalf("expected build to fail on duplicate id")
	}
	wrong := Topology{SchemaVersion: "0"}
	if err := wrong.Valid(); err == nil || !strings.Contains(err.Error(), "schema version") {
		t.Fatalf("expected schema version error, got %v", err)
	}
}

func TestEncodeDecodeRoundtrip(t *testing.T) {
	tp, err := Build([]Site{sampleSite()}, time.Unix(1, 0))
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
	bad := []byte(`{"schemaVersion":"1","sites":[{"id":"blog","name":"x","domain":"x.example.com","listener":{"port":443,"protocol":"tcp"},"target":{"host":"127.0.0.1","port":80,"protocol":"http"},"bogus":1}]}`)
	if _, err := ParseTopology(bad); err == nil {
		t.Fatalf("expected parse to fail on unknown field")
	}
	// truncated
	if _, err := ParseTopology(data[:5]); err == nil {
		t.Fatalf("expected parse to fail on truncated")
	}
}

func TestSummarise(t *testing.T) {
	tp, _ := Build([]Site{sampleSite()}, time.Unix(1, 0))
	sum := Summarise(tp, map[string]Health{"blog": HealthEnabled})
	if len(sum) != 1 {
		t.Fatalf("expected 1 summary, got %d", len(sum))
	}
	if !sum[0].TLS || !sum[0].DDNS {
		t.Fatalf("expected tls+ddns flags, got %+v", sum[0])
	}
	if sum[0].Health != HealthEnabled {
		t.Fatalf("expected health=enabled, got %s", sum[0].Health)
	}
	// disabled site is reported as disabled
	disabled := sampleSite()
	disabled.Enabled = false
	disabled.ID = "disabled-site"
	tp2, _ := Build([]Site{disabled}, time.Unix(1, 0))
	sum2 := Summarise(tp2, nil)
	if sum2[0].Health != HealthDisabled {
		t.Fatalf("expected disabled, got %s", sum2[0].Health)
	}
}

func TestItoa(t *testing.T) {
	if itoa(0) != "0" {
		t.Fatalf("expected 0")
	}
	if itoa(443) != "443" {
		t.Fatalf("expected 443, got %q", itoa(443))
	}
	if itoa(8080) != "8080" {
		t.Fatalf("expected 8080")
	}
}

func TestFormatListener(t *testing.T) {
	l := Listener{InterfaceID: "wan1", Port: 443, Protocol: ProtocolTLS}
	if got := formatListener(l); got != "wan1:tls/443" {
		t.Fatalf("expected wan1:tls/443, got %q", got)
	}
	empty := Listener{Port: 80, Protocol: ProtocolHTTP}
	if got := formatListener(empty); got != "http/80" {
		t.Fatalf("expected http/80, got %q", got)
	}
}

func TestFormatTarget(t *testing.T) {
	t2 := Target{Host: "192.168.1.10", Port: 8080, Protocol: ProtocolHTTP}
	if got := formatTarget(t2); got != "192.168.1.10:http/8080" {
		t.Fatalf("expected 192.168.1.10:http/8080, got %q", got)
	}
}
