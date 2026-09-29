// hostapd/hostapd_test.go pins the V1
// hostapd configuration model and renderer.
package hostapd

import "testing"

func TestBSSValid(t *testing.T) {
	good := BSS{Name: "office", Interface: "wlan0", Driver: "nl80211", SSID: "DuckOS", Band: Band5G, Channel: 36, Security: SecWPA3Personal, Passphrase: "correct horse battery"}
	if err := good.Valid(); err != nil {
		t.Fatal(err)
	}
	noSSID := BSS{Name: "x", Interface: "wlan0", Driver: "nl80211", Band: Band5G, Channel: 36, Security: SecOpen}
	if err := noSSID.Valid(); err == nil {
		t.Fatal("missing SSID must fail")
	}
	badChan := BSS{Name: "x", Interface: "wlan0", Driver: "nl80211", SSID: "DuckOS", Band: Band5G, Channel: 999, Security: SecOpen}
	if err := badChan.Valid(); err == nil {
		t.Fatal("channel out of range must fail")
	}
	shortPwd := BSS{Name: "x", Interface: "wlan0", Driver: "nl80211", SSID: "DuckOS", Band: Band5G, Channel: 36, Security: SecWPA2Personal, Passphrase: "short"}
	if err := shortPwd.Valid(); err == nil {
		t.Fatal("short passphrase must fail")
	}
	ent := BSS{Name: "x", Interface: "wlan0", Driver: "nl80211", SSID: "DuckOS", Band: Band5G, Channel: 36, Security: SecWPA2Enterprise}
	if err := ent.Valid(); err == nil {
		t.Fatal("enterprise without auth server must fail")
	}
}

func TestRenderDeterministic(t *testing.T) {
	c := Config{
		BSSes: []BSS{
			{Name: "guest", Interface: "wlan0", Driver: "nl80211", SSID: "Guest", Band: Band2G, Channel: 6, Security: SecOpen, Bridge: "br-guest"},
			{Name: "main", Interface: "wlan1", Driver: "nl80211", SSID: "Main", Band: Band5G, Channel: 36, Security: SecWPA3Personal, Passphrase: "correct horse battery", WMM: true, IEE80211w: 2},
		},
	}
	first, err := Render(c)
	if err != nil {
		t.Fatal(err)
	}
	hash := hashOf(first)
	for i := 0; i < 10; i++ {
		got, err := Render(c)
		if err != nil {
			t.Fatal(err)
		}
		if hashOf(got) != hash {
			t.Fatalf("render not deterministic")
		}
	}
	if !contains(first, "ssid=Main") || !contains(first, "ssid=Guest") {
		t.Fatalf("missing ssid: %s", first)
	}
	if !contains(first, "wpa=2") || !contains(first, "sae_password=") {
		t.Fatalf("missing wpa3 fields: %s", first)
	}
}

func TestManagerLifecycle(t *testing.T) {
	m := NewManager(nil)
	c1 := Config{BSSes: []BSS{{Name: "a", Interface: "wlan0", Driver: "nl80211", SSID: "A", Band: Band5G, Channel: 36, Security: SecOpen}}}
	c2 := Config{BSSes: []BSS{{Name: "a", Interface: "wlan0", Driver: "nl80211", SSID: "A2", Band: Band5G, Channel: 36, Security: SecOpen}}}
	m.Set(c1)
	m.Set(c2)
	if err := m.Rollback(); err != nil {
		t.Fatal(err)
	}
	if err := m.Rollback(); err == nil {
		t.Fatal("double rollback must fail")
	}
}

func TestManagerStations(t *testing.T) {
	m := NewManager(nil)
	m.SetStation(Station{MAC: "aa:bb:cc:dd:ee:01", Signal: -42, BSS: "main"})
	m.SetStation(Station{MAC: "aa:bb:cc:dd:ee:02", Signal: -68, BSS: "main"})
	if len(m.Stations()) != 2 {
		t.Fatalf("stations: %d", len(m.Stations()))
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func hashOf(s string) string {
	var h uint64 = 1469598103934665603
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= 1099511628211
	}
	if h == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for h > 0 {
		i--
		buf[i] = byte('0' + h%10)
		h /= 10
	}
	return string(buf[i:])
}
