// wan/config.go: beta.4 WAN configuration validation and
// observed-state collection.
//
// The Provider model (provider.go) is the declared intent.
// This file adds:
//
//   - strict validation for the DHCP / static IPv4 modes
//     (the two modes the beta.4 milestone commits to)
//   - the Observation type: what the kernel actually reports
//     (link state, addresses, default route) — deliberately
//     distinct from configuration
//
// LIVE APPLY IS NOT IMPLEMENTED on this milestone: mutating
// a live WAN uplink is a 🔴 РИСК operation that must go
// through the managed-routing enrollment flow (networkd) and
// Safe Apply. The IPC surface stages configuration and
// observes state; it refuses to imply that a staged provider
// is active.
package wan

import (
	"errors"
	"fmt"
	"net/netip"
)

// MaxDNSServers bounds the DNS list of a static config.
const MaxDNSServers = 8

// ValidStatic checks the static sub-configuration: address
// CIDR (masked), gateway IP in the same family, DNS servers
// bounded and parseable.
func (s StaticConfig) Valid() error {
	if s.Address == "" {
		return errors.New("wan: static address is required")
	}
	p, err := netip.ParsePrefix(s.Address)
	if err != nil {
		return fmt.Errorf("wan: static address is not a CIDR prefix: %w", err)
	}
	if p != p.Masked() {
		return errors.New("wan: static address prefix is not masked")
	}
	if p.Addr().IsLoopback() || p.Addr().IsMulticast() || p.Addr().IsUnspecified() {
		return errors.New("wan: static address is not a usable unicast address")
	}
	if s.Gateway != "" {
		gw, err := netip.ParseAddr(s.Gateway)
		if err != nil {
			return fmt.Errorf("wan: gateway is not an IP address: %w", err)
		}
		if gw.Is4() != p.Addr().Is4() {
			return errors.New("wan: gateway address family differs from the address")
		}
		if gw.IsLoopback() || gw.IsMulticast() || gw.IsUnspecified() {
			return errors.New("wan: gateway is not a usable address")
		}
	}
	if len(s.DNSServers) > MaxDNSServers {
		return errors.New("wan: too many DNS servers")
	}
	for _, dns := range s.DNSServers {
		addr, err := netip.ParseAddr(dns)
		if err != nil {
			return fmt.Errorf("wan: DNS server %q is not an IP address", dns)
		}
		if addr.IsMulticast() || addr.IsUnspecified() {
			return errors.New("wan: DNS server is not a usable address")
		}
	}
	return nil
}

// ValidProviderComplete is the mode-aware full validation
// used when staging a provider: it adds the DHCP/static
// exclusivity checks the base Valid() leaves open.
func ValidProviderComplete(p Provider) error {
	if err := p.Valid(); err != nil {
		return err
	}
	switch p.Mode {
	case ModeDHCP:
		if p.Static != nil {
			return errors.New("wan: mode=dhcp must not carry a static config")
		}
		if p.PPPoE != nil || p.L2TP != nil {
			return errors.New("wan: mode=dhcp must not carry tunnel configs")
		}
	case ModeStatic:
		if p.PPPoE != nil || p.L2TP != nil {
			return errors.New("wan: mode=static must not carry tunnel configs")
		}
		if err := p.Static.Valid(); err != nil {
			return err
		}
	}
	return nil
}

// Observation is the kernel-reported state of one link.
// Every field is observed truth, never configuration.
type Observation struct {
	Link     string   `json:"link"`
	Up       bool     `json:"up"`
	MTU      int      `json:"mtu"`
	Hardware string   `json:"hardware,omitempty"`
	IPv4     []string `json:"ipv4,omitempty"`
	IPv6     []string `json:"ipv6,omitempty"`
	// DefaultRouteV4 is the observed default gateway and its
	// egress device ("gateway via device"), empty when the
	// host has no IPv4 default route through this link.
	DefaultRouteV4 string `json:"defaultRouteV4,omitempty"`
	// Unsupported marks build targets where the collector
	// cannot observe honestly (e.g. non-linux).
	Unsupported string `json:"unsupported,omitempty"`
	// ObservedAt is RFC3339.
	ObservedAt string `json:"observedAt"`
}
