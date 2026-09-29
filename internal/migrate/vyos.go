// vyos.go: V1 VyOS / Vyatta-style analyser.
//
// The VyOS configuration syntax is a brace-
// structured tree:
//
//	interfaces {
//	    ethernet eth0 {
//	        address 192.0.2.1/24
//	    }
//	}
//	protocols {
//	    bgp 64512 {
//	        neighbor 10.0.0.1 {
//	            remote-as 64513
//	        }
//	    }
//	}
//
// The analyser walks the source line by
// line, maintains a section stack, and
// classifies each significant `key value`
// line under the current section. Lines
// that match a known section are reported
// as recognized; lines that need operator
// confirmation are reported as
// convertible; unknown lines are reported
// as unsupported so the operator can
// inspect them.
package migrate

import "strings"

// vyOSSectionIntent maps a *type* path
// (joined by '/', excluding the trailing
// named section) to a section classification.
// The lookup walks the path: the longest
// matching type path wins.
var vyOSSectionIntent = map[string]sectionClassification{
	"interfaces/ethernet":    {Intent: "interface-ethernet", Status: StatusRecognized},
	"interfaces/loopback":    {Intent: "interface-loopback", Status: StatusRecognized},
	"interfaces/dummy":       {Intent: "interface-dummy", Status: StatusRecognized},
	"interfaces/vif":         {Intent: "vlan", Status: StatusRecognized},
	"protocols/static":       {Intent: "static-route", Status: StatusRecognized},
	"protocols/bgp":          {Intent: "bgp", Status: StatusRecognized},
	"protocols/bgp/neighbor": {Intent: "bgp-neighbor", Status: StatusRecognized},
	"protocols/ospf":         {Intent: "ospf", Status: StatusRecognized},
	"protocols/ospf/area":    {Intent: "ospf-area", Status: StatusRecognized},
	"vrf":                    {Intent: "vrf", Status: StatusRecognized},
	"firewall":               {Intent: "firewall", Status: StatusRecognized},
	"firewall/name":          {Intent: "firewall-chain", Status: StatusConvertible, Note: "operator must confirm chain semantics"},
	"nat":                    {Intent: "nat", Status: StatusRecognized},
	"nat/source":             {Intent: "nat-source", Status: StatusConvertible, Note: "operator must confirm masquerade semantics"},
	"nat/destination":        {Intent: "nat-destination", Status: StatusConvertible, Note: "operator must confirm DNAT semantics"},
	"system/name-server":     {Intent: "dns-nameserver", Status: StatusRecognized},
	"system/host-name":       {Intent: "system-hostname", Status: StatusRecognized},
	"high-availability/vrrp": {Intent: "vrrp", Status: StatusRecognized},
	"vpn/ipsec":              {Intent: "ipsec", Status: StatusConvertible, Note: "VyOS IPsec syntax differs from DuckOS"},
	"vpn/ipsec/site-to-site": {Intent: "ipsec-site", Status: StatusConvertible, Note: "review selectors and proposals"},
}

// vyOSKeyIntent maps a known "key value"
// line to its classification. The key is
// matched against the first whitespace-
// separated token on the line.
var vyOSKeyIntent = map[string]sectionClassification{
	"address":     {Intent: "address", Status: StatusRecognized},
	"remote-as":   {Intent: "bgp-remote-as", Status: StatusRecognized},
	"router-id":   {Intent: "ospf-router-id", Status: StatusRecognized},
	"next-hop":    {Intent: "static-route-next-hop", Status: StatusRecognized},
	"description": {Intent: "description", Status: StatusRecognized},
	"mtu":         {Intent: "interface-mtu", Status: StatusRecognized},
	"vrf":         {Intent: "vrf-binding", Status: StatusRecognized},
}

// sectionClassification is the per-section
// classification. Note is a free-form
// warning surfaced in the plan.
type sectionClassification struct {
	Intent string
	Status Status
	Note   string
}

// lookupVyOSSection walks the section path
// from longest to shortest and returns the
// first matching classification. A path of
// "interfaces/ethernet/eth0" matches
// "interfaces/ethernet".
func lookupVyOSSection(path string) (sectionClassification, bool) {
	parts := strings.Split(path, "/")
	for i := len(parts); i > 0; i-- {
		candidate := strings.Join(parts[:i], "/")
		if cls, ok := vyOSSectionIntent[candidate]; ok {
			return cls, true
		}
	}
	return sectionClassification{}, false
}

// analyseVyOS classifies every significant
// line in a VyOS / Vyatta-style source.
// The function returns one Item per
// significant line; blank lines and
// pure-`{` / `}` lines produce no Item.
func analyseVyOS(data []byte) ([]Item, []string, error) {
	items := []Item{}
	lines := strings.Split(string(data), "\n")
	// Section stack: each element is the
	// current section name at that depth.
	stack := []string{}
	for i, raw := range lines {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		line := i + 1
		if strings.HasSuffix(trimmed, "{") {
			// New section header.
			header := strings.TrimSpace(strings.TrimSuffix(trimmed, "{"))
			// A VyOS section header is
			// "<type> [name]"; we push the
			// type and the (optional) name
			// separately so the lookup can
			// strip the name.
			fields := strings.Fields(header)
			if len(fields) == 0 {
				continue
			}
			stack = append(stack, fields[0])
			if len(fields) > 1 {
				stack = append(stack, fields[1])
			}
			continue
		}
		if trimmed == "}" {
			if len(stack) == 0 {
				items = append(items, Item{Line: line, Raw: raw, Intent: "unbalanced", Status: StatusUnsupported, Note: "extra closing brace"})
				continue
			}
			stack = stack[:len(stack)-1]
			continue
		}
		path := strings.Join(stack, "/")
		cls, ok := lookupVyOSSection(path)
		if !ok {
			// Unknown section: classify the
			// line as unsupported so the
			// operator can decide.
			items = append(items, Item{Line: line, Raw: raw, Intent: path, Status: StatusUnsupported, Note: "section is not in the V1 importer"})
			continue
		}
		// Check whether the line is a known
		// "key value" pair inside the
		// recognised section; the key takes
		// precedence over the section intent
		// when present.
		key := strings.Fields(trimmed)
		var keyCls sectionClassification
		keyMatched := false
		if len(key) > 0 {
			keyCls, keyMatched = vyOSKeyIntent[key[0]]
		}
		intent := cls.Intent
		status := cls.Status
		note := cls.Note
		if keyMatched {
			intent = keyCls.Intent
			status = keyCls.Status
			note = keyCls.Note
		}
		item := Item{
			Line:   line,
			Raw:    raw,
			Intent: intent,
			Status: status,
		}
		if note != "" {
			item.Note = note
		}
		items = append(items, item)
		if len(items) >= MaxItems {
			break
		}
	}
	return items, nil, nil
}
