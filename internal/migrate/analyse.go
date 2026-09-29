// analyse.go owns the per-format analyser. The
// analyser is a small, deterministic state
// machine per format. It walks the source line
// by line, classifies each significant line and
// emits an Item. Comments and blank lines are
// reported as "recognized" with intent "comment"
// so the operator can see what was ignored.

package migrate

import (
	"errors"
	"strings"
	"time"
)

// ErrSourceTooLarge is the error returned by
// Analyse when the supplied source exceeds the
// MaxSourceBytes cap. The error is reported
// before any allocation so a hostile source
// cannot OOM the Agent.
var ErrSourceTooLarge = errors.New("migrate: source exceeds the maximum size")

// ErrSourceEmpty is the error returned by
// Analyse when the supplied source has no
// significant lines. The error is non-fatal:
// the operator can still see the empty Plan in
// the GUI.
var ErrSourceEmpty = errors.New("migrate: source has no significant lines")

// Analyse is the entry point. The function
// takes a Source (filename + bytes) and a clock
// for the generated timestamp. The function
// auto-detects the format, runs the appropriate
// per-format analyser, and returns the bounded
// Plan. The Plan is validated before return;
// the caller can rely on Plan.Valid.
func Analyse(src Source, data []byte, now time.Time) (Plan, error) {
	if len(data) > MaxSourceBytes {
		return Plan{}, ErrSourceTooLarge
	}
	format, confidence := DetectFormat(data)
	// The Format hint supplied in Source wins
	// over the auto-detected format. The
	// confidence is the higher of the two.
	if src.Format != "" && src.Format != FormatUnknown && isValidFormat(src.Format) {
		format = src.Format
		confidence = ConfidenceHigh
	}
	src.Bytes = len(data)
	src.Lines = countLines(data)
	src.CapturedAt = nowUnix(now)
	items, notes, err := analyseBytes(format, data)
	if err != nil {
		return Plan{}, err
	}
	if len(items) == 0 {
		return Plan{}, ErrSourceEmpty
	}
	p := Plan{SchemaVersion: SchemaVersion, Source: src, Format: format, Confidence: confidence, Items: items, Notes: notes, GeneratedAt: nowUnix(now)}
	if err := p.Valid(); err != nil {
		return Plan{}, err
	}
	return p, nil
}

// analyseBytes dispatches to the per-format
// analyser. An unknown format returns the
// "everything is unsupported" plan so the
// operator can still inspect the source.
func analyseBytes(format FormatKind, data []byte) ([]Item, []string, error) {
	switch format {
	case FormatMikroTik:
		return analyseMikroTik(data)
	case FormatOpenWrtUCI:
		return analyseOpenWrt(data)
	case FormatLinuxInterfaces:
		return analyseLinuxInterfaces(data)
	case FormatKerio:
		return analyseKerio(data)
	case FormatVyOS:
		return analyseVyOS(data)
	}
	// Unknown format: every line is reported
	// as unsupported with intent "unknown" so
	// the operator can still see what was
	// in the source.
	items := []Item{}
	lines := strings.Split(string(data), "\n")
	for i, raw := range lines {
		raw = trimLine(raw)
		if raw == "" {
			continue
		}
		items = append(items, Item{Line: i + 1, Raw: raw, Intent: "unknown", Status: StatusUnsupported, Note: "format not detected"})
		if len(items) >= MaxItems {
			break
		}
	}
	return items, []string{"Format not detected; the source is reported as unsupported. Specify the format with --format."}, nil
}

// analyseMikroTik parses a MikroTik .rsc export.
// The canonical block syntax is
//
//	/ip/address
//	add address=192.0.2.1/24 interface=ether1
//	set [find interface=ether1] disabled=no
//
// Every /section line opens a new menu; every
// add/set/remove line is a statement. The
// classifier maps well-known menus to DuckOS
// intents and reports unknown statements as
// attention.
func analyseMikroTik(data []byte) ([]Item, []string, error) {
	items := []Item{}
	notes := []string{}
	currentSection := ""
	for i, raw := range strings.Split(string(data), "\n") {
		raw = trimLine(raw)
		if raw == "" {
			continue
		}
		if strings.HasPrefix(raw, "#") {
			items = append(items, Item{Line: i + 1, Section: currentSection, Raw: raw, Intent: "comment", Status: StatusRecognized})
			continue
		}
		// Section header: /ip/address
		if strings.HasPrefix(raw, "/") {
			currentSection = trimSection(raw)
			items = append(items, Item{Line: i + 1, Section: currentSection, Raw: raw, Intent: "section", Status: statusForMikroTikSection(currentSection), Note: "menu header"})
			continue
		}
		// Statement: add address=...
		verb, args, ok := splitFirstToken(raw)
		if !ok {
			items = append(items, Item{Line: i + 1, Section: currentSection, Raw: raw, Intent: "unknown", Status: StatusAttention, Note: "no verb recognised"})
			continue
		}
		intent, status, note := classifyMikroTikStatement(currentSection, verb, args)
		items = append(items, Item{Line: i + 1, Section: currentSection, Raw: raw, Intent: intent, Status: status, Note: note})
		if len(items) >= MaxItems {
			notes = append(notes, "Item cap reached; remaining lines were not classified.")
			break
		}
	}
	return items, notes, nil
}

// statusForMikroTikSection reports the
// classification of a MikroTik section header.
// Headers that map to a known DuckOS intent
// are recognized; unknown headers (e.g.
// /system script, /ip service) are reported
// as attention so the operator can decide
// whether to import them.
func statusForMikroTikSection(section string) Status {
	switch section {
	case "/ip address", "/ip route", "/ip firewall filter", "/ip firewall nat", "/ip firewall mangle", "/ip firewall raw", "/interface", "/interface ethernet", "/interface bridge", "/interface vlan", "/queue tree", "/queue simple":
		return StatusRecognized
	}
	return StatusAttention
}

// classifyMikroTikStatement maps a section +
// verb to an intent. The mapping is small on
// purpose: the package never tries to be a
// full MikroTik parser; it surfaces what
// DuckOS can map and reports the rest.
func classifyMikroTikStatement(section, verb, args string) (intent string, status Status, note string) {
	switch section {
	case "/ip address":
		switch verb {
		case "add":
			return "duckos.network.address", StatusConvertible, "review IPv4/IPv6 family"
		case "set", "remove":
			return "duckos.network.address", StatusRecognized, "address mutation"
		}
	case "/ip route":
		switch verb {
		case "add":
			return "duckos.routing.route", StatusConvertible, "review metric and gateway"
		case "set", "remove":
			return "duckos.routing.route", StatusRecognized, "route mutation"
		}
	case "/ip firewall filter", "/ip firewall nat", "/ip firewall mangle", "/ip firewall raw":
		switch verb {
		case "add":
			return "duckos.firewall.rule", StatusAttention, "firewall rules need operator confirmation"
		case "set", "remove", "move", "disable", "enable":
			return "duckos.firewall.rule", StatusRecognized, "firewall mutation"
		}
	case "/interface", "/interface ethernet", "/interface bridge", "/interface vlan":
		switch verb {
		case "add", "set":
			return "duckos.network.link", StatusConvertible, "interface definition"
		case "remove":
			return "duckos.network.link", StatusRecognized, "interface removal"
		}
	case "/queue tree", "/queue simple":
		switch verb {
		case "add", "set":
			return "duckos.qos.shaper", StatusConvertible, "queueing rule"
		case "remove":
			return "duckos.qos.shaper", StatusRecognized, "queue removal"
		}
	}
	return "unknown", StatusAttention, "DuckOS does not auto-translate this MikroTik statement; manual review required"
}

// analyseOpenWrt parses a UCI config. The
// canonical block is
//
//	config interface 'lan'
//	  option ifname 'eth0'
//	  option proto 'static'
//	  option ipaddr '192.0.2.1'
//
// The classifier maps interface, firewall,
// network and wireless sections to DuckOS
// intents and reports anything else as
// attention.
func analyseOpenWrt(data []byte) ([]Item, []string, error) {
	items := []Item{}
	notes := []string{}
	currentType := ""
	currentName := ""
	for i, raw := range strings.Split(string(data), "\n") {
		raw = trimLine(raw)
		if raw == "" {
			continue
		}
		if strings.HasPrefix(raw, "#") {
			items = append(items, Item{Line: i + 1, Section: currentName, Raw: raw, Intent: "comment", Status: StatusRecognized})
			continue
		}
		if strings.HasPrefix(raw, "config ") {
			parts := strings.Fields(raw)
			if len(parts) >= 3 {
				currentType = parts[1]
				currentName = strings.Trim(parts[2], "'\"")
			}
			intent, status, note := classifyOpenWrtSection(currentType)
			items = append(items, Item{Line: i + 1, Section: currentName, Raw: raw, Intent: intent, Status: status, Note: note})
			continue
		}
		if strings.HasPrefix(raw, "option ") || strings.HasPrefix(raw, "list ") {
			key, value, ok := splitKeyValue(raw)
			if !ok {
				items = append(items, Item{Line: i + 1, Section: currentName, Raw: raw, Intent: "unknown", Status: StatusAttention, Note: "no key/value recognised"})
				continue
			}
			intent, status, note := classifyOpenWrtOption(currentType, key, value)
			items = append(items, Item{Line: i + 1, Section: currentName, Raw: raw, Intent: intent, Status: status, Note: note})
			continue
		}
		items = append(items, Item{Line: i + 1, Section: currentName, Raw: raw, Intent: "unknown", Status: StatusAttention})
		if len(items) >= MaxItems {
			notes = append(notes, "Item cap reached; remaining lines were not classified.")
			break
		}
	}
	return items, notes, nil
}

// classifyOpenWrtSection maps a UCI config type
// to a DuckOS intent. Unrecognised types are
// reported as attention so the operator can
// decide whether to import them manually.
func classifyOpenWrtSection(t string) (intent string, status Status, note string) {
	switch t {
	case "interface":
		return "duckos.network.link", StatusConvertible, "UCI interface"
	case "switch", "switch_vlan":
		return "duckos.network.vlan", StatusConvertible, "UCI switch section"
	case "route":
		return "duckos.routing.route", StatusConvertible, "UCI static route"
	case "rule":
		return "duckos.firewall.rule", StatusAttention, "UCI ipset rule"
	case "defaults", "rule6":
		return "duckos.firewall.rule", StatusAttention, "UCI firewall rule"
	case "zone", "forwarding":
		return "duckos.firewall.zone", StatusConvertible, "UCI firewall zone"
	case "redirect":
		return "duckos.firewall.nat", StatusConvertible, "UCI NAT redirect"
	case "wifi-iface":
		return "duckos.wifi.ap", StatusAttention, "UCI wifi-iface requires driver/regulatory support"
	case "wireless", "wifi-device":
		return "duckos.wifi.device", StatusAttention, "UCI wireless device requires driver/regulatory support"
	case "dhcp", "dhcp-host":
		return "duckos.dhcp.lease", StatusConvertible, "UCI DHCP section"
	}
	return "unknown", StatusAttention, "DuckOS does not auto-translate this UCI section"
}

// classifyOpenWrtOption maps a UCI option to
// an intent. The mapping is per-section-type.
func classifyOpenWrtOption(t, key, value string) (intent string, status Status, note string) {
	switch t {
	case "interface":
		switch key {
		case "proto", "ifname", "ipaddr", "netmask", "gateway", "dns":
			return "duckos.network.link", StatusConvertible, "interface option"
		}
		return "duckos.network.link", StatusAttention, "review interface option"
	case "route":
		return "duckos.routing.route", StatusConvertible, "static route option"
	case "rule", "rule6", "defaults":
		return "duckos.firewall.rule", StatusAttention, "firewall rule option"
	case "zone":
		return "duckos.firewall.zone", StatusConvertible, "zone option"
	case "redirect":
		return "duckos.firewall.nat", StatusConvertible, "NAT redirect"
	}
	return "unknown", StatusAttention, "DuckOS does not auto-translate this UCI option"
}

// analyseLinuxInterfaces parses a
// /etc/network/interfaces file. The format is
// line-based; the section is implicit in the
// most recent `iface` or `auto` line.
func analyseLinuxInterfaces(data []byte) ([]Item, []string, error) {
	items := []Item{}
	notes := []string{}
	currentSection := ""
	for i, raw := range strings.Split(string(data), "\n") {
		raw = trimLine(raw)
		if raw == "" {
			continue
		}
		if strings.HasPrefix(raw, "#") {
			items = append(items, Item{Line: i + 1, Section: currentSection, Raw: raw, Intent: "comment", Status: StatusRecognized})
			continue
		}
		verb, args, ok := splitFirstToken(raw)
		if !ok {
			items = append(items, Item{Line: i + 1, Section: currentSection, Raw: raw, Intent: "unknown", Status: StatusAttention})
			continue
		}
		switch verb {
		case "auto", "allow-hotplug":
			currentSection = args
			items = append(items, Item{Line: i + 1, Section: currentSection, Raw: raw, Intent: "duckos.network.link", Status: StatusConvertible, Note: "auto-boot stanza"})
		case "iface":
			currentSection = args
			items = append(items, Item{Line: i + 1, Section: currentSection, Raw: raw, Intent: "duckos.network.link", Status: StatusConvertible, Note: "interface definition"})
		case "address", "netmask", "gateway", "dns-nameservers", "mtu":
			items = append(items, Item{Line: i + 1, Section: currentSection, Raw: raw, Intent: "duckos.network.link", Status: StatusConvertible, Note: "interface option"})
		case "up", "down", "pre-up", "post-down":
			items = append(items, Item{Line: i + 1, Section: currentSection, Raw: raw, Intent: "duckos.script", Status: StatusAttention, Note: "shell hook requires manual review"})
		case "vlan-raw-device", "vlan-id":
			items = append(items, Item{Line: i + 1, Section: currentSection, Raw: raw, Intent: "duckos.network.vlan", Status: StatusConvertible, Note: "VLAN option"})
		case "bridge_ports", "bridge_stp":
			items = append(items, Item{Line: i + 1, Section: currentSection, Raw: raw, Intent: "duckos.network.bridge", Status: StatusConvertible, Note: "bridge option"})
		default:
			items = append(items, Item{Line: i + 1, Section: currentSection, Raw: raw, Intent: "unknown", Status: StatusAttention, Note: "unrecognised option"})
		}
		if len(items) >= MaxItems {
			notes = append(notes, "Item cap reached; remaining lines were not classified.")
			break
		}
	}
	return items, notes, nil
}

// analyseKerio parses a Kerio-style config.
// The canonical block is
//
//	[SectionName]
//	Variable = Value
//	Variable2 = Value2
//
// The classifier maps well-known sections to
// DuckOS intents and reports unknown sections
// as attention.
func analyseKerio(data []byte) ([]Item, []string, error) {
	items := []Item{}
	notes := []string{}
	currentSection := ""
	for i, raw := range strings.Split(string(data), "\n") {
		raw = trimLine(raw)
		if raw == "" {
			continue
		}
		if strings.HasPrefix(raw, "#") || strings.HasPrefix(raw, ";") {
			items = append(items, Item{Line: i + 1, Section: currentSection, Raw: raw, Intent: "comment", Status: StatusRecognized})
			continue
		}
		if strings.HasPrefix(raw, "[") && strings.HasSuffix(raw, "]") {
			currentSection = strings.TrimSpace(raw[1 : len(raw)-1])
			intent, status, note := classifyKerioSection(currentSection)
			items = append(items, Item{Line: i + 1, Section: currentSection, Raw: raw, Intent: intent, Status: status, Note: note})
			continue
		}
		key, _, ok := splitKeyValue(raw)
		if !ok {
			items = append(items, Item{Line: i + 1, Section: currentSection, Raw: raw, Intent: "unknown", Status: StatusAttention})
			continue
		}
		intent, status, note := classifyKerioVariable(currentSection, key)
		items = append(items, Item{Line: i + 1, Section: currentSection, Raw: raw, Intent: intent, Status: status, Note: note})
		if len(items) >= MaxItems {
			notes = append(notes, "Item cap reached; remaining lines were not classified.")
			break
		}
	}
	return items, notes, nil
}

// classifyKerioSection maps a Kerio-style
// section to a DuckOS intent. The mapping is
// intentionally small; unrecognised sections
// are reported as attention.
func classifyKerioSection(s string) (intent string, status Status, note string) {
	switch strings.ToLower(s) {
	case "interfaces", "interface":
		return "duckos.network.link", StatusConvertible, "Kerio interface section"
	case "routes", "routing":
		return "duckos.routing.route", StatusConvertible, "Kerio routing section"
	case "firewall":
		return "duckos.firewall.rule", StatusAttention, "Kerio firewall section"
	case "nat", "portforward":
		return "duckos.firewall.nat", StatusConvertible, "Kerio NAT section"
	case "dns", "dhcp":
		return "duckos.dns.zone", StatusConvertible, "Kerio DNS/DHCP section"
	case "vpn", "ipsec":
		return "duckos.vpn", StatusAttention, "Kerio VPN/IPsec section"
	}
	return "unknown", StatusAttention, "DuckOS does not auto-translate this Kerio section"
}

// classifyKerioVariable maps a Kerio-style
// variable to a DuckOS intent. The mapping
// honours the section context.
func classifyKerioVariable(section, key string) (intent string, status Status, note string) {
	switch strings.ToLower(section) {
	case "interfaces", "interface":
		return "duckos.network.link", StatusConvertible, "interface option"
	case "routes", "routing":
		return "duckos.routing.route", StatusConvertible, "route option"
	case "firewall":
		return "duckos.firewall.rule", StatusAttention, "firewall option"
	case "nat", "portforward":
		return "duckos.firewall.nat", StatusConvertible, "NAT option"
	}
	return "unknown", StatusAttention, "DuckOS does not auto-translate this Kerio variable"
}

// splitFirstToken splits a line into its first
// token and the remainder. Empty input yields
// ok=false.
func splitFirstToken(s string) (string, string, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", "", false
	}
	idx := strings.IndexAny(s, " \t")
	if idx < 0 {
		return s, "", true
	}
	return s[:idx], strings.TrimSpace(s[idx+1:]), true
}

// splitKeyValue splits a "key = value" or
// "key value" line. Quoted values are kept
// verbatim; the surrounding quotes are
// preserved.
func splitKeyValue(s string) (string, string, bool) {
	idx := -1
	for i, r := range s {
		if r == '=' {
			idx = i
			break
		}
		if r == ' ' || r == '\t' {
			// no-op; the value may be the rest
			// of the line
		}
	}
	if idx < 0 {
		// fall back to first space
		sp := strings.IndexAny(s, " \t")
		if sp < 0 {
			return "", "", false
		}
		return strings.TrimSpace(s[:sp]), strings.TrimSpace(s[sp+1:]), true
	}
	return strings.TrimSpace(s[:idx]), strings.TrimSpace(s[idx+1:]), true
}

// trimSection trims a MikroTik section header
// to the canonical form, dropping trailing
// comments only. MikroTik section names may
// contain spaces (e.g. "/ip address", "/ip
// firewall filter") so the function must not
// break on whitespace.
func trimSection(s string) string {
	if i := strings.Index(s, "#"); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return strings.TrimSpace(s)
}

// countLines returns the number of newlines in
// the supplied bytes, plus one for the
// terminal line if the input is not empty.
func countLines(data []byte) int {
	if len(data) == 0 {
		return 0
	}
	n := bytesCount(data, '\n')
	if data[len(data)-1] != '\n' {
		n++
	}
	return n
}

// bytesCount is a tiny helper that counts the
// occurrences of a byte in a slice. The
// standard library has no equivalent.
func bytesCount(data []byte, c byte) int {
	n := 0
	for _, b := range data {
		if b == c {
			n++
		}
	}
	return n
}
