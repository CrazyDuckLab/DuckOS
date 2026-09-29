package network

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"sort"
)

// KernelPolicyRule is the reproducible subset of the enrolled RPDB ranges.
// Unknown selectors/actions in those ranges are conflicts, never ignored state.
type KernelPolicyRule struct {
	Family               string `json:"family"`
	Priority             int    `json:"priority"`
	From                 string `json:"from,omitempty"`
	To                   string `json:"to,omitempty"`
	Table                int    `json:"table"`
	SuppressPrefixLength int    `json:"suppressPrefixLength"`
	Unreachable          bool   `json:"unreachable"`
	Protocol             int    `json:"protocol"`
}
type PolicySnapshot struct {
	Version int                `json:"version"`
	Rules   []KernelPolicyRule `json:"rules"`
}

func ownedPolicyPriority(priority int) bool {
	return priority >= 10000 && priority <= 12047 || priority >= 20000 && priority <= 21999
}

func normalizePolicySnapshot(s PolicySnapshot) (PolicySnapshot, error) {
	if s.Version != 1 || len(s.Rules) > 3072 {
		return PolicySnapshot{}, errors.New("policy snapshot exceeds scope or version")
	}
	normal := PolicySnapshot{Version: 1, Rules: append([]KernelPolicyRule{}, s.Rules...)}
	seen := map[string]bool{}
	for i := range normal.Rules {
		r := &normal.Rules[i]
		if (r.Family != "ipv4" && r.Family != "ipv6") || !ownedPolicyPriority(r.Priority) || r.Protocol < 0 || r.Protocol > 255 || r.Unreachable != (r.Priority%2 == 1) {
			return PolicySnapshot{}, errors.New("policy rule is outside supported owned shape")
		}
		for _, value := range []*string{&r.From, &r.To} {
			if *value == "" {
				continue
			}
			p, err := netip.ParsePrefix(*value)
			if err != nil || p != p.Masked() || p.Addr().Is4In6() || p.Addr().Is4() != (r.Family == "ipv4") || p.Addr().IsMulticast() {
				return PolicySnapshot{}, errors.New("invalid policy snapshot selector")
			}
			if p.Bits() == 0 {
				*value = ""
			} else {
				*value = p.String()
			}
		}
		connected := r.Priority < 20000
		if connected && (r.From != "" || r.To == "") {
			return PolicySnapshot{}, errors.New("invalid connected policy selector")
		}
		if r.Unreachable {
			if r.Table != 0 || r.SuppressPrefixLength != -1 {
				return PolicySnapshot{}, errors.New("unreachable rule has lookup attributes")
			}
		} else if connected {
			p := netip.MustParsePrefix(r.To)
			if r.Table != 254 || r.SuppressPrefixLength != p.Bits()-1 {
				return PolicySnapshot{}, errors.New("invalid connected lookup suppression")
			}
		} else if !privateRouteTable(r.Table) || r.SuppressPrefixLength != -1 {
			return PolicySnapshot{}, errors.New("invalid private policy lookup")
		}
		key := fmt.Sprintf("%s/%d", r.Family, r.Priority)
		if seen[key] {
			return PolicySnapshot{}, errors.New("duplicate priority cannot be safely restored")
		}
		seen[key] = true
	}
	sort.Slice(normal.Rules, func(i, j int) bool {
		a, b := normal.Rules[i], normal.Rules[j]
		if a.Family != b.Family {
			return a.Family < b.Family
		}
		return a.Priority < b.Priority
	})
	data, err := json.Marshal(normal)
	if err != nil || len(data) > 1<<20 {
		return PolicySnapshot{}, errors.New("policy snapshot exceeds recovery capacity")
	}
	return normal, nil
}

func DecodePolicySnapshot(raw []byte) (PolicySnapshot, error) {
	var s PolicySnapshot
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if len(raw) > 1<<20 || d.Decode(&s) != nil || d.Decode(new(any)) != io.EOF || s.Rules == nil {
		return s, errors.New("invalid policy recovery document")
	}
	return normalizePolicySnapshot(s)
}

// PolicySnapshotForConfig resolves the same instructions used by networkd and
// simulation. Protocol 4 is static; runtime readback must prove exact equality.
func PolicySnapshotForConfig(c Config) (PolicySnapshot, error) {
	if err := c.Validate(); err != nil {
		return PolicySnapshot{}, err
	}
	s := PolicySnapshot{Version: 1}
	for _, r := range c.compilePolicy() {
		table := r.table
		if r.unreachable {
			table = 0
		}
		s.Rules = append(s.Rules, KernelPolicyRule{Family: r.family, Priority: r.priority, From: r.from, To: r.to, Table: table, SuppressPrefixLength: r.suppress, Unreachable: r.unreachable, Protocol: 4})
	}
	return normalizePolicySnapshot(s)
}

func decodePolicyInventory(raw []byte, family string) ([]KernelPolicyRule, error) {
	if len(raw) > 2<<20 || (family != "ipv4" && family != "ipv6") {
		return nil, errors.New("invalid policy inventory bounds")
	}
	var objects []map[string]json.RawMessage
	if json.Unmarshal(raw, &objects) != nil || objects == nil || len(objects) > 8192 {
		return nil, errors.New("invalid policy inventory")
	}
	s := PolicySnapshot{Version: 1}
	for _, object := range objects {
		priority, err := routeNumber(object["priority"], -1)
		if err != nil || priority < 0 || uint64(priority) > 0xffffffff {
			return nil, errors.New("policy priority cannot be identified")
		}
		if !ownedPolicyPriority(priority) {
			continue
		}
		for key := range object {
			switch key {
			case "priority", "src", "srclen", "dst", "dstlen", "table", "suppress_prefixlen", "action", "protocol":
			default:
				return nil, errors.New("unsupported selector in owned policy range")
			}
		}
		from, err := policyInventoryPrefix(object, "src", "srclen", family, true)
		if err != nil {
			return nil, err
		}
		to, err := policyInventoryPrefix(object, "dst", "dstlen", family, false)
		if err != nil {
			return nil, err
		}
		table, err := routeNumber(object["table"], 0)
		if err != nil {
			return nil, err
		}
		suppress, err := routeNumber(object["suppress_prefixlen"], -1)
		if err != nil {
			return nil, err
		}
		protocol, err := routeNumber(object["protocol"], 0)
		if err != nil {
			return nil, err
		}
		unreachable := false
		if action, ok := object["action"]; ok {
			if string(action) == `"unreachable"` || string(action) == `"7"` || string(action) == `7` {
				unreachable = true
			} else {
				return nil, errors.New("unsupported owned policy action")
			}
		}
		s.Rules = append(s.Rules, KernelPolicyRule{Family: family, Priority: priority, From: from, To: to, Table: table, SuppressPrefixLength: suppress, Unreachable: unreachable, Protocol: protocol})
	}
	normal, err := normalizePolicySnapshot(s)
	return normal.Rules, err
}

func policyInventoryPrefix(object map[string]json.RawMessage, key, lengthKey, family string, required bool) (string, error) {
	raw, present := object[key]
	if !present {
		if required || len(object[lengthKey]) != 0 {
			return "", errors.New("incomplete policy selector")
		}
		return "", nil
	}
	var value string
	if json.Unmarshal(raw, &value) != nil || value == "" {
		return "", errors.New("invalid policy selector text")
	}
	if value == "all" {
		bits, err := routeNumber(object[lengthKey], 0)
		if err != nil || bits != 0 {
			return "", errors.New("invalid wildcard policy selector")
		}
		return "", nil
	}
	if value == "0" {
		value = "0.0.0.0"
		if family == "ipv6" {
			value = "::"
		}
	}
	a, err := netip.ParseAddr(value)
	if err != nil || a.Is4In6() || a.Zone() != "" || a.Is4() != (family == "ipv4") {
		return "", errors.New("invalid policy address family")
	}
	bits, err := routeNumber(object[lengthKey], a.BitLen())
	if err != nil || bits < 0 || bits > a.BitLen() {
		return "", errors.New("invalid policy prefix length")
	}
	p := netip.PrefixFrom(a, bits)
	if p != p.Masked() {
		return "", errors.New("noncanonical policy selector")
	}
	return p.String(), nil
}
