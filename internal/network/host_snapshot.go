package network

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/netip"
	"sort"
	"time"
)

// HostSnapshot is private recovery evidence, never a source of desired intent.
// It deliberately contains no arbitrary paths, daemon configs or credentials.
type HostSnapshot struct {
	Version        int        `json:"version"`
	CapturedAt     time.Time  `json:"capturedAt"`
	IPv6Forwarding bool       `json:"ipv6Forwarding"`
	Links          []HostLink `json:"links"`
}
type HostLink struct {
	Name           string   `json:"name"`
	Index          int      `json:"index"`
	MAC            string   `json:"mac"`
	MTU            int      `json:"mtu"`
	Up             bool     `json:"up"`
	IPv4Forwarding bool     `json:"ipv4Forwarding"`
	IPv6Forwarding bool     `json:"ipv6Forwarding"`
	Addresses      []string `json:"addresses"`
}

func PhysicalBindings(before, next Config) ([]string, error) {
	if err := before.Validate(); err != nil {
		return nil, err
	}
	if err := next.Validate(); err != nil {
		return nil, err
	}
	set := map[string]bool{}
	for _, c := range []Config{before, next} {
		for _, i := range c.Interfaces {
			if i.Kind == "physical" {
				set[i.Name] = true
			}
		}
	}
	names := make([]string, 0, len(set))
	for name := range set {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}
func DecodeHostSnapshot(raw json.RawMessage, before, next Config) (HostSnapshot, error) {
	var s HostSnapshot
	if len(raw) > 1<<20 {
		return s, errors.New("host snapshot exceeds limit")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(&s); err != nil {
		return s, errors.New("invalid host snapshot JSON")
	}
	if d.Decode(new(any)) != io.EOF {
		return s, errors.New("trailing host snapshot data")
	}
	names, err := PhysicalBindings(before, next)
	if err != nil {
		return s, err
	}
	if s.Version != 1 || s.CapturedAt.IsZero() || len(s.Links) != len(names) {
		return s, errors.New("host snapshot version or binding mismatch")
	}
	seen := map[string]bool{}
	indexes := map[int]bool{}
	for _, l := range s.Links {
		found := sort.SearchStrings(names, l.Name)
		if found == len(names) || names[found] != l.Name || seen[l.Name] || l.Index < 1 || indexes[l.Index] || l.MTU < 68 || l.MTU > 65535 || !macAddress.MatchString(l.MAC) || len(l.Addresses) > 64 {
			return s, errors.New("invalid host link evidence")
		}
		seen[l.Name] = true
		indexes[l.Index] = true
		addresses := map[string]bool{}
		for _, raw := range l.Addresses {
			p, e := netip.ParsePrefix(raw)
			if e != nil || p.Addr().Is4In6() || p.Addr().IsMulticast() || p.Addr().IsUnspecified() || addresses[raw] {
				return s, errors.New("invalid captured host address")
			}
			addresses[raw] = true
		}
	}
	return s, nil
}
