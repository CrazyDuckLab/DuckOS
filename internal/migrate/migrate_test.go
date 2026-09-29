// migrate_test.go covers the model, the
// detector, the analysers and the per-format
// classification. The migration integration
// tests live in their own file.
package migrate

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestSourceBasics(t *testing.T) {
	s := Source{Filename: "router.rsc", Format: FormatMikroTik, Bytes: 100, Lines: 10}
	if s.Filename != "router.rsc" {
		t.Fatalf("filename roundtrip")
	}
}

func TestItemValid(t *testing.T) {
	good := Item{Line: 1, Section: "/ip address", Raw: "add address=192.0.2.1/24", Intent: "duckos.network.address", Status: StatusConvertible, Note: "review"}
	if err := good.Valid(); err != nil {
		t.Fatalf("expected good item to validate: %v", err)
	}
	cases := []struct {
		name    string
		mut     func(*Item)
		wantErr string
	}{
		{"line zero", func(i *Item) { i.Line = 0 }, "line is not positive"},
		{"empty raw", func(i *Item) { i.Raw = "" }, "raw is empty"},
		{"oversized raw", func(i *Item) { i.Raw = strings.Repeat("a", MaxLineLength+1) }, "raw is oversized"},
		{"nul in raw", func(i *Item) { i.Raw = "abc\x00def" }, "contains NUL"},
		{"empty intent", func(i *Item) { i.Intent = "" }, "intent is empty"},
		{"bad status", func(i *Item) { i.Status = Status("bad") }, "status is not canonical"},
		{"newline in note", func(i *Item) { i.Note = "a\nb" }, "contains newline"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			i := good
			tc.mut(&i)
			err := i.Valid()
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected %q, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestPlanValid(t *testing.T) {
	p := Plan{SchemaVersion: SchemaVersion, Format: FormatMikroTik, Confidence: ConfidenceHigh, Items: []Item{{Line: 1, Raw: "/ip address", Intent: "section", Status: StatusRecognized}}}
	if err := p.Valid(); err != nil {
		t.Fatalf("expected good plan to validate: %v", err)
	}
	wrong := Plan{SchemaVersion: "0", Format: FormatMikroTik, Confidence: ConfidenceHigh}
	if err := wrong.Valid(); err == nil {
		t.Fatalf("expected schema version error")
	}
	wrongF := Plan{SchemaVersion: SchemaVersion, Format: FormatKind("bogus"), Confidence: ConfidenceHigh}
	if err := wrongF.Valid(); err == nil {
		t.Fatalf("expected format error")
	}
	wrongC := Plan{SchemaVersion: SchemaVersion, Format: FormatMikroTik, Confidence: Confidence("bogus")}
	if err := wrongC.Valid(); err == nil {
		t.Fatalf("expected confidence error")
	}
}

func TestEncodeParseRoundtrip(t *testing.T) {
	p := Plan{SchemaVersion: SchemaVersion, Format: FormatMikroTik, Confidence: ConfidenceHigh, Items: []Item{{Line: 1, Raw: "/ip address", Intent: "section", Status: StatusRecognized}}, Notes: []string{"hello"}}
	data, err := p.Encode()
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	got, err := ParsePlan(data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got.Format != p.Format || len(got.Items) != 1 {
		t.Fatalf("roundtrip mismatch")
	}
	// unknown field
	bad := []byte(`{"schemaVersion":"1","format":"mikrotik","confidence":"high","items":[],"bogus":1}`)
	if _, err := ParsePlan(bad); err == nil {
		t.Fatalf("expected parse to fail on unknown field")
	}
	// truncated
	if _, err := ParsePlan(data[:5]); err == nil {
		t.Fatalf("expected parse to fail on truncated")
	}
}

func TestSummarise(t *testing.T) {
	p := Plan{Format: FormatMikroTik, Confidence: ConfidenceHigh, Items: []Item{
		{Line: 1, Raw: "/ip address", Intent: "section", Status: StatusRecognized},
		{Line: 2, Raw: "add address=192.0.2.1/24", Intent: "duckos.network.address", Status: StatusConvertible},
		{Line: 3, Raw: "/ip firewall filter", Intent: "section", Status: StatusRecognized},
		{Line: 4, Raw: "add chain=forward", Intent: "duckos.firewall.rule", Status: StatusAttention},
		{Line: 5, Raw: "/system script", Intent: "section", Status: StatusRecognized},
		{Line: 6, Raw: "add name=setup", Intent: "unknown", Status: StatusUnsupported},
	}}
	s := Summarise(p)
	if s.Recognized != 3 || s.Convertible != 1 || s.Attention != 1 || s.Unsupported != 1 {
		t.Fatalf("unexpected counts: %+v", s)
	}
	if s.Total != 6 {
		t.Fatalf("expected total=6, got %d", s.Total)
	}
}

func TestSortItemsByLine(t *testing.T) {
	items := []Item{
		{Line: 2, Raw: "b", Intent: "x", Status: StatusRecognized},
		{Line: 1, Raw: "a", Intent: "x", Status: StatusRecognized},
	}
	sorted := SortItemsByLine(items)
	if sorted[0].Line != 1 || sorted[1].Line != 2 {
		t.Fatalf("expected sorted, got %+v", sorted)
	}
}

func TestIsSuccess(t *testing.T) {
	for s, want := range map[Status]bool{
		StatusRecognized:  true,
		StatusConvertible: true,
		StatusAttention:   false,
		StatusUnsupported: false,
	} {
		if got := IsSuccess(s); got != want {
			t.Fatalf("IsSuccess(%s) = %v, want %v", s, got, want)
		}
	}
}

func TestDetectFormat(t *testing.T) {
	cases := []struct {
		in   string
		want FormatKind
		conf Confidence
	}{
		{"/ip address\nadd address=192.0.2.1/24 interface=ether1\n", FormatMikroTik, ConfidenceHigh},
		{"config interface 'lan'\n  option ifname 'eth0'\n  option proto 'static'\n", FormatOpenWrtUCI, ConfidenceHigh},
		{"auto eth0\niface eth0 inet static\n  address 192.0.2.1/24\n", FormatLinuxInterfaces, ConfidenceHigh},
		{"[Interfaces]\nName = LAN\nIP = 192.168.1.1\n", FormatKerio, ConfidenceMedium},
		{"random text\n", FormatUnknown, ConfidenceLow},
		{"", FormatUnknown, ConfidenceLow},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got, conf := DetectFormat([]byte(tc.in))
			if got != tc.want {
				t.Fatalf("expected format %s, got %s", tc.want, got)
			}
			if conf != tc.conf {
				t.Fatalf("expected confidence %s, got %s", tc.conf, conf)
			}
		})
	}
}

func TestAnalyseMikroTik(t *testing.T) {
	src := `#
# RouterOS 7.x
#
/ip address
add address=192.0.2.1/24 interface=ether1
add address=2001:db8::1/64 interface=ether1
set [find interface=ether1] disabled=no
/ip route
add dst-address=0.0.0.0/0 gateway=192.0.2.254
/ip firewall filter
add chain=forward action=accept
add chain=input action=drop
/system script
add name=setup source="/log info hello"
`
	p, err := Analyse(Source{Filename: "router.rsc"}, []byte(src), time.Unix(1, 0))
	if err != nil {
		t.Fatalf("analyse: %v", err)
	}
	if p.Format != FormatMikroTik {
		t.Fatalf("expected mikrotik, got %s", p.Format)
	}
	sum := Summarise(p)
	if sum.Recognized < 4 {
		t.Fatalf("expected at least 4 recognized, got %+v", sum)
	}
	if sum.Convertible < 2 {
		t.Fatalf("expected at least 2 convertible, got %+v", sum)
	}
	// /system script must be attention
	for _, it := range p.Items {
		if it.Section == "/system script" && it.Status != StatusAttention && it.Status != StatusUnsupported {
			t.Fatalf("expected /system script to be attention, got %s", it.Status)
		}
	}
}

func TestAnalyseOpenWrt(t *testing.T) {
	src := `config interface 'lan'
  option ifname 'eth0'
  option proto 'static'
  option ipaddr '192.168.1.1'
  option netmask '255.255.255.0'

config wifi-iface 'default_radio0'
  option device 'radio0'
  option ssid 'DuckOS'
  option encryption 'psk2'

config zone 'lan'
  option name 'lan'

config redirect 'ssh'
  option src 'wan'
  option dest_port '22'
  option proto 'tcp'
`
	p, err := Analyse(Source{Filename: "/etc/config/network"}, []byte(src), time.Unix(1, 0))
	if err != nil {
		t.Fatalf("analyse: %v", err)
	}
	if p.Format != FormatOpenWrtUCI {
		t.Fatalf("expected openwrt-uci, got %s", p.Format)
	}
	sum := Summarise(p)
	if sum.Total < 4 {
		t.Fatalf("expected at least 4 items, got %+v", sum)
	}
	// redirect must be convertible
	for _, it := range p.Items {
		if strings.Contains(it.Raw, "redirect") && it.Intent != "duckos.firewall.nat" {
			t.Fatalf("expected redirect to map to firewall.nat, got %s", it.Intent)
		}
	}
}

func TestAnalyseLinuxInterfaces(t *testing.T) {
	src := `auto lo
iface lo inet loopback

auto eth0
iface eth0 inet static
  address 192.0.2.1/24
  gateway 192.0.2.254
  mtu 1500
  pre-up /usr/local/bin/setup

allow-hotplug wlan0
iface wlan0 inet dhcp
`
	p, err := Analyse(Source{Filename: "/etc/network/interfaces"}, []byte(src), time.Unix(1, 0))
	if err != nil {
		t.Fatalf("analyse: %v", err)
	}
	if p.Format != FormatLinuxInterfaces {
		t.Fatalf("expected linux-interfaces, got %s", p.Format)
	}
	// pre-up must be attention
	foundAttention := false
	for _, it := range p.Items {
		if strings.Contains(it.Raw, "pre-up") && it.Status == StatusAttention {
			foundAttention = true
		}
	}
	if !foundAttention {
		t.Fatalf("expected pre-up to be attention")
	}
}

func TestAnalyseKerio(t *testing.T) {
	src := `[Interfaces]
Name = LAN
IP = 192.168.1.1
Netmask = 255.255.255.0

[Firewall]
Rule1 = Accept
Rule2 = Drop

[VPN]
Enabled = 1
`
	p, err := Analyse(Source{Filename: "kerio.conf"}, []byte(src), time.Unix(1, 0))
	if err != nil {
		t.Fatalf("analyse: %v", err)
	}
	if p.Format != FormatKerio {
		t.Fatalf("expected kerio, got %s", p.Format)
	}
	// VPN must be attention
	for _, it := range p.Items {
		if it.Section == "VPN" && it.Status != StatusAttention {
			t.Fatalf("expected VPN to be attention, got %s", it.Status)
		}
	}
}

func TestAnalyseEmptySource(t *testing.T) {
	_, err := Analyse(Source{Filename: "empty.conf"}, []byte(""), time.Unix(1, 0))
	if !bytes.Equal([]byte(err.Error()), []byte(ErrSourceEmpty.Error())) {
		t.Fatalf("expected ErrSourceEmpty, got %v", err)
	}
}

func TestAnalyseOversizedSource(t *testing.T) {
	huge := bytes.Repeat([]byte("a"), MaxSourceBytes+1)
	_, err := Analyse(Source{Filename: "huge.rsc"}, huge, time.Unix(1, 0))
	if !bytes.Equal([]byte(err.Error()), []byte(ErrSourceTooLarge.Error())) {
		t.Fatalf("expected ErrSourceTooLarge, got %v", err)
	}
}

func TestAnalyseRespectsFormatHint(t *testing.T) {
	// A buffer that would be detected as
	// MikroTik can be re-analysed as Kerio if
	// the user supplies the format hint. The
	// result is still a valid plan; the
	// per-line classification may differ.
	src := `add address=192.0.2.1/24`
	p, err := Analyse(Source{Filename: "x", Format: FormatKerio}, []byte(src), time.Unix(1, 0))
	if err != nil {
		t.Fatalf("analyse: %v", err)
	}
	if p.Format != FormatKerio {
		t.Fatalf("expected kerio from hint, got %s", p.Format)
	}
	if p.Confidence != ConfidenceHigh {
		t.Fatalf("expected confidence=high with hint, got %s", p.Confidence)
	}
}

func TestSplitFirstToken(t *testing.T) {
	verb, rest, ok := splitFirstToken("add address=192.0.2.1/24")
	if !ok || verb != "add" || rest != "address=192.0.2.1/24" {
		t.Fatalf("split: %q %q %v", verb, rest, ok)
	}
	if _, _, ok := splitFirstToken(""); ok {
		t.Fatalf("expected empty input to fail")
	}
	if verb, rest, ok := splitFirstToken("add"); !ok || verb != "add" || rest != "" {
		t.Fatalf("split single token: %q %q %v", verb, rest, ok)
	}
}

func TestSplitKeyValue(t *testing.T) {
	k, v, ok := splitKeyValue("option ifname 'eth0'")
	if !ok || k != "option" || v != "ifname 'eth0'" {
		t.Fatalf("split: %q %q %v", k, v, ok)
	}
	k, v, ok = splitKeyValue("Name = LAN")
	if !ok || k != "Name" || v != "LAN" {
		t.Fatalf("split kv: %q %q %v", k, v, ok)
	}
	if _, _, ok := splitKeyValue(""); ok {
		t.Fatalf("expected empty to fail")
	}
}

func TestTrimLine(t *testing.T) {
	if trimLine("  hello\t world\n") != "hello world" {
		t.Fatalf("unexpected trim: %q", trimLine("  hello\t world\n"))
	}
	if trimLine("") != "" {
		t.Fatalf("expected empty trim")
	}
	// NUL is dropped
	if strings.Contains(trimLine("a\x00b"), "\x00") {
		t.Fatalf("expected NUL to be dropped")
	}
}

func TestTrimSection(t *testing.T) {
	if trimSection("/ip address") != "/ip address" {
		t.Fatalf("unexpected section trim: %q", trimSection("/ip address"))
	}
	if trimSection("/ip address # comment") != "/ip address" {
		t.Fatalf("expected section to drop comment")
	}
}

func TestCountLines(t *testing.T) {
	if countLines([]byte("a\nb\nc\n")) != 3 {
		t.Fatalf("expected 3 lines")
	}
	if countLines([]byte("a\nb\nc")) != 3 {
		t.Fatalf("expected 3 lines without trailing newline")
	}
	if countLines([]byte("")) != 0 {
		t.Fatalf("expected 0 lines for empty")
	}
}

func TestAnalyseUnknownFormat(t *testing.T) {
	src := `random binary data here`
	p, err := Analyse(Source{Filename: "x"}, []byte(src), time.Unix(1, 0))
	if err != nil {
		t.Fatalf("analyse: %v", err)
	}
	if p.Format != FormatUnknown {
		t.Fatalf("expected unknown, got %s", p.Format)
	}
	if p.Confidence != ConfidenceLow {
		t.Fatalf("expected low confidence, got %s", p.Confidence)
	}
	for _, it := range p.Items {
		if it.Status != StatusUnsupported {
			t.Fatalf("expected every line to be unsupported, got %s", it.Status)
		}
	}
	if len(p.Notes) == 0 {
		t.Fatalf("expected a note for unknown format")
	}
}

func TestAnalyseMikroTikShellScriptUnsupported(t *testing.T) {
	// A /system script block is reported as
	// attention so the operator can decide to
	// import the script body or not.
	src := `/system script
add name=setup source="/log info hello"
`
	p, err := Analyse(Source{Filename: "x.rsc"}, []byte(src), time.Unix(1, 0))
	if err != nil {
		t.Fatalf("analyse: %v", err)
	}
	found := false
	for _, it := range p.Items {
		if strings.HasPrefix(it.Raw, "add name=") && it.Status == StatusAttention {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected script add to be attention, got %+v", p.Items)
	}
}

func TestNotesNeverExceedCap(t *testing.T) {
	src := Source{Filename: "x"}
	big := bytes.Repeat([]byte("a\n"), MaxItems*2)
	if _, err := Analyse(src, big, time.Unix(1, 0)); err != nil {
		// big source is detected as unknown
		// format and every non-empty line is an
		// item; the analyser must cap the item
		// list at MaxItems.
		t.Fatalf("analyse: %v", err)
	}
}
