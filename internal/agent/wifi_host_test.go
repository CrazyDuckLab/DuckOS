package agent

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/wifi"
)

func sampleAP() wifi.AP {
	return wifi.AP{ID: "office-ap", Name: "Office AP", SSID: "DuckOS-WiFi", Interface: "wlan0", Band: wifi.Band5G, Channel: 36, Security: wifi.SecurityWPA2WPA3, PassphraseRef: "vault://office-passphrase", Country: "US", Enabled: true}
}

func TestNewWifiHostDefaultUnsupported(t *testing.T) {
	h := NewWifiHost()
	st, err := h.Wifi(context.Background())
	if err != nil {
		t.Fatalf("wifi: %v", err)
	}
	if st.Capability.Supported {
		t.Fatalf("expected default capability to be unsupported")
	}
}

func TestNewWifiHostWithStateBad(t *testing.T) {
	bad := wifi.Topology{SchemaVersion: "0"}
	if _, err := NewWifiHostWithState(bad, wifi.Capability{}); err == nil {
		t.Fatalf("expected bad topology to be rejected")
	}
	badCap := wifi.Capability{Supported: true}
	if _, err := NewWifiHostWithState(wifi.Topology{SchemaVersion: wifi.SchemaVersion}, badCap); err == nil {
		t.Fatalf("expected bad capability to be rejected")
	}
}

func TestWifiHostRoundTripWithCapability(t *testing.T) {
	tp, err := wifi.Build([]wifi.AP{sampleAP()}, time.Unix(1, 0))
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	cap := wifi.Capability{Supported: true, Driver: "iwlwifi", Interface: "wlan0", Bands: []wifi.Band{wifi.Band5G}, RegDomain: "US"}
	h, err := NewWifiHostWithState(tp, cap)
	if err != nil {
		t.Fatalf("new host: %v", err)
	}
	st, err := h.Wifi(context.Background())
	if err != nil {
		t.Fatalf("wifi: %v", err)
	}
	if !st.Capability.Supported {
		t.Fatalf("expected supported capability")
	}
	if st.Health["office-ap"] != wifi.HealthEnabled {
		t.Fatalf("expected enabled, got %s", st.Health["office-ap"])
	}
}

func TestWifiHostApUnsupportedWithoutCapability(t *testing.T) {
	tp, _ := wifi.Build([]wifi.AP{sampleAP()}, time.Unix(1, 0))
	h, _ := NewWifiHostWithState(tp, wifi.Capability{Supported: false})
	st, _ := h.Wifi(context.Background())
	if st.Health["office-ap"] != wifi.HealthUnsupported {
		t.Fatalf("expected unsupported, got %s", st.Health["office-ap"])
	}
}

func TestWifiHostUpdateTopology(t *testing.T) {
	h := NewWifiHost()
	tp, _ := wifi.Build([]wifi.AP{sampleAP()}, time.Unix(1, 0))
	if err := h.UpdateTopology(tp); err != nil {
		t.Fatalf("update: %v", err)
	}
	st, _ := h.Wifi(context.Background())
	if st.HasAPs != 1 {
		t.Fatalf("expected 1 ap, got %d", st.HasAPs)
	}
	if st.Health["office-ap"] != wifi.HealthUnsupported {
		t.Fatalf("expected unsupported (no capability), got %s", st.Health["office-ap"])
	}
	// enable capability
	cap := wifi.Capability{Supported: true, Driver: "iwlwifi", Interface: "wlan0", Bands: []wifi.Band{wifi.Band5G}}
	if err := h.UpdateCapability(cap); err != nil {
		t.Fatalf("update capability: %v", err)
	}
	st, _ = h.Wifi(context.Background())
	if st.Health["office-ap"] != wifi.HealthEnabled {
		t.Fatalf("expected enabled after capability update, got %s", st.Health["office-ap"])
	}
	// bad capability is rejected
	if err := h.UpdateCapability(wifi.Capability{Supported: true}); err == nil {
		t.Fatalf("expected bad capability to be rejected")
	}
}

func TestWifiHostNilReceiver(t *testing.T) {
	var h *WifiHost
	if _, err := h.Wifi(context.Background()); err == nil {
		t.Fatalf("expected nil receiver to fail")
	}
}

func TestServeWifiEndpoint(t *testing.T) {
	tp, _ := wifi.Build([]wifi.AP{sampleAP()}, time.Unix(1, 0))
	h, _ := NewWifiHostWithState(tp, wifi.Capability{Supported: true, Driver: "iwlwifi", Interface: "wlan0", Bands: []wifi.Band{wifi.Band5G}})
	srv := NewHandler(nil)
	srv.EnableWifi(h)
	r := httptest.NewRequest("GET", WifiPath, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d (%s)", w.Code, w.Body.String())
	}
	if w.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("expected json content type")
	}
}

func TestServeWifiWithoutHost(t *testing.T) {
	srv := NewHandler(nil)
	r := httptest.NewRequest("GET", WifiPath, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	if w.Code != 503 {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

func TestServeWifiMethodNotAllowed(t *testing.T) {
	srv := NewHandler(nil)
	r := httptest.NewRequest("POST", WifiPath, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	if w.Code != 405 {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}
