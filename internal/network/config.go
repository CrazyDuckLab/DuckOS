// Package network defines canonical managed networking intent. Linux names are
// bindings, not object identities. This schema intentionally rejects unsupported
// modes rather than accepting settings an adapter would silently ignore.
package network

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"regexp"
)

type Config struct {
	Version     int               `json:"version"`
	Interfaces  []Interface       `json:"interfaces"`
	Routes      []Route           `json:"routes"`
	PolicyRules []PolicyRule      `json:"policyRules,omitempty"`
	Management  *ManagementPolicy `json:"managementPolicy,omitempty"`
	Firewall    Firewall          `json:"firewall"`
	DNS         DNS               `json:"dns"`
}
type Interface struct {
	ID         string      `json:"id"`
	Name       string      `json:"name"`
	Role       string      `json:"role"`
	Kind       string      `json:"kind"`
	Parent     string      `json:"parent,omitempty"`
	VLAN       int         `json:"vlan,omitempty"`
	MTU        int         `json:"mtu"`
	Addresses  []string    `json:"addresses"`
	DHCPClient string      `json:"dhcpClient"`
	AcceptRA   bool        `json:"acceptRA"`
	Management bool        `json:"management"`
	DHCPServer *DHCPServer `json:"dhcpServer,omitempty"`
}
type DHCPServer struct {
	Start        string        `json:"start"`
	End          string        `json:"end"`
	LeaseSeconds int           `json:"leaseSeconds"`
	Reservations []Reservation `json:"reservations"`
}
type Reservation struct {
	MAC      string `json:"mac"`
	Address  string `json:"address"`
	Hostname string `json:"hostname"`
}
type Route struct {
	ID          string `json:"id"`
	Interface   string `json:"interface"`
	Destination string `json:"destination"`
	Gateway     string `json:"gateway,omitempty"`
	Metric      int    `json:"metric"`
	Table       int    `json:"table"`
}
type Firewall struct {
	Masquerade []string  `json:"masquerade"`
	Forwards   []Forward `json:"forwards"`
	Rules      []Rule    `json:"rules"`
}
type Forward struct {
	ID         string `json:"id"`
	WAN        string `json:"wan"`
	Protocol   string `json:"protocol"`
	Port       int    `json:"port"`
	Target     string `json:"target"`
	TargetPort int    `json:"targetPort"`
}
type Rule struct {
	ID              string `json:"id"`
	From            string `json:"from"`
	To              string `json:"to"`
	Action          string `json:"action"`
	Protocol        string `json:"protocol"`
	DestinationPort int    `json:"destinationPort,omitempty"`
}
type DNS struct {
	Upstreams []string `json:"upstreams"`
	Records   []Record `json:"records"`
}
type Record struct {
	Name    string `json:"name"`
	Address string `json:"address"`
}

var identifier = regexp.MustCompile(`^[a-z][a-z0-9-]{0,47}$`)
var linkName = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_-]{0,14}$`)
var hostName = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?(\.[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?)*$`)
var macAddress = regexp.MustCompile(`^[0-9a-f]{2}(:[0-9a-f]{2}){5}$`)

func Decode(data json.RawMessage) (Config, error) {
	var c Config
	if len(data) > 256<<10 {
		return c, errors.New("network configuration exceeds size limit")
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(&c); err != nil {
		return c, errors.New("invalid network configuration JSON")
	}
	if d.Decode(new(any)) != io.EOF {
		return c, errors.New("trailing network configuration")
	}
	return c, c.Validate()
}

func (c Config) Validate() error {
	if len(c.Interfaces) == 0 && (len(c.DNS.Upstreams) != 0 || len(c.DNS.Records) != 0) {
		return errors.New("DNS configuration requires a managed LAN")
	}
	if c.Version != 1 || len(c.Interfaces) > 64 || len(c.Routes) > 512 || len(c.Firewall.Rules) > 512 || len(c.Firewall.Forwards) > 256 || len(c.Firewall.Masquerade) > 16 || len(c.DNS.Upstreams) > 16 || len(c.DNS.Records) > 256 {
		return errors.New("unsupported version or object limit exceeded")
	}
	ids := map[string]bool{}
	names := map[string]bool{}
	links := map[string]Interface{}
	addID := func(id string) error {
		if !identifier.MatchString(id) || ids[id] {
			return errors.New("invalid or duplicate object ID")
		}
		ids[id] = true
		return nil
	}
	management := 0
	lanCount := 0
	for _, i := range c.Interfaces {
		if i.Role == "lan" {
			lanCount++
		}
		if err := addID(i.ID); err != nil {
			return err
		}
		if !linkName.MatchString(i.Name) || names[i.Name] || i.Name == "lo" {
			return errors.New("invalid, duplicate, or reserved interface name")
		}
		names[i.Name] = true
		links[i.ID] = i
		if i.Role != "wan" && i.Role != "lan" {
			return errors.New("unsupported interface role")
		}
		if i.Kind != "physical" && i.Kind != "vlan" && i.Kind != "bridge" {
			return errors.New("unsupported interface kind")
		}
		if i.Kind == "physical" && (i.Parent != "" || i.VLAN != 0) {
			// A physical interface may carry a parent reference only when
			// the parent is a bridge. The parent has not been added to the
			// links map yet at this point; defer the bridge-parent check
			// to a second pass below.
			if i.VLAN != 0 {
				return errors.New("physical interface has VLAN settings")
			}
		}
		if i.Kind == "vlan" && (i.Parent == i.ID || i.VLAN < 1 || i.VLAN > 4094) {
			return errors.New("invalid VLAN")
		}
		if i.Kind == "bridge" {
			if i.VLAN != 0 {
				return errors.New("bridge interface has VLAN settings")
			}
			if i.DHCPClient != "none" || i.AcceptRA {
				return errors.New("bridge cannot request DHCP or RA")
			}
		}
		if i.MTU < 1280 || i.MTU > 9000 || len(i.Addresses) > 16 {
			return errors.New("invalid MTU or address limit")
		}
		if i.DHCPClient != "none" && i.DHCPClient != "ipv4" && i.DHCPClient != "ipv6" && i.DHCPClient != "dual" {
			return errors.New("unsupported DHCP client mode")
		}
		if i.Role == "lan" && (i.DHCPClient != "none" || i.AcceptRA) {
			return errors.New("managed LAN cannot accept upstream DHCP or RA")
		}
		if i.Management {
			if c.Management != nil {
				return errors.New("explicit management policy cannot mix with legacy interface management flags")
			}
			if i.Role != "lan" || len(i.Addresses) == 0 {
				return errors.New("management requires statically addressed LAN")
			}
			management++
		}
		seen := map[netip.Addr]bool{}
		for _, raw := range i.Addresses {
			p, e := netip.ParsePrefix(raw)
			if e != nil || !usable(p.Addr()) || seen[p.Addr()] {
				return errors.New("invalid or duplicate interface address")
			}
			seen[p.Addr()] = true
		}
		if i.DHCPServer != nil {
			if err := validateDHCP(i); err != nil {
				return fmt.Errorf("DHCP %s: %w", i.ID, err)
			}
		}
	}
	if lanCount == 0 && (len(c.DNS.Upstreams) > 0 || len(c.DNS.Records) > 0) {
		return errors.New("DNS service requires a managed LAN listener")
	}
	if c.Management != nil {
		if err := c.validateManagement(links, addID); err != nil {
			return err
		}
	} else if len(c.Interfaces) > 0 && management == 0 {
		return errors.New("at least one static management LAN is required")
	}
	vlanKeys := map[string]bool{}
	bridgeChildren := map[string]int{}
	for _, i := range c.Interfaces {
		if i.Kind == "physical" && i.Parent != "" {
			parent, ok := links[i.Parent]
			if !ok || parent.Kind != "bridge" {
				return errors.New("physical interface parent must be a bridge")
			}
		}
		if i.Kind == "vlan" {
			p, ok := links[i.Parent]
			if !ok || p.Kind != "physical" || i.MTU > p.MTU {
				return errors.New("VLAN requires physical parent with sufficient MTU")
			}
			key := fmt.Sprintf("%s/%d", i.Parent, i.VLAN)
			if vlanKeys[key] {
				return errors.New("duplicate parent VLAN")
			}
			vlanKeys[key] = true
		}
		if i.Kind == "bridge" {
			children := 0
			for _, child := range c.Interfaces {
				if child.Parent == i.ID {
					if child.Kind != "physical" {
						return errors.New("bridge children must be physical")
					}
					children++
				}
			}
			if children == 0 {
				return errors.New("bridge requires at least one physical child")
			}
			bridgeChildren[i.ID] = children
		}
	}
	// Overlapping directly connected networks across interfaces are ambiguous.
	for a, x := range c.Interfaces {
		for _, y := range c.Interfaces[a+1:] {
			for _, xs := range x.Addresses {
				xp, _ := netip.ParsePrefix(xs)
				for _, ys := range y.Addresses {
					yp, _ := netip.ParsePrefix(ys)
					if xp.Overlaps(yp) {
						return errors.New("overlapping interface networks")
					}
				}
			}
		}
	}
	routeKeys := map[string]bool{}
	for _, r := range c.Routes {
		if err := addID(r.ID); err != nil {
			return err
		}
		i, ok := links[r.Interface]
		if !ok {
			return errors.New("route references missing interface")
		}
		p, e := netip.ParsePrefix(r.Destination)
		if e != nil || p != p.Masked() || p.Addr().Is4In6() || r.Metric < 0 || r.Metric > 65535 || (r.Table != 254 && !privateRouteTable(r.Table)) {
			return errors.New("invalid route or table outside managed range")
		}
		key := fmt.Sprintf("%d/%s/%d", r.Table, r.Destination, effectiveRouteMetric(r))
		if routeKeys[key] {
			return errors.New("ambiguous duplicate route; multipath requires an explicit route group")
		}
		routeKeys[key] = true
		if r.Gateway != "" {
			g, e := netip.ParseAddr(r.Gateway)
			if e != nil || !usable(g) || g.Is4() != p.Addr().Is4() {
				return errors.New("invalid route gateway")
			}
			onlink := g.Is6() && g.IsLinkLocalUnicast()
			for _, a := range i.Addresses {
				prefix, _ := netip.ParsePrefix(a)
				onlink = onlink || prefix.Contains(g)
			}
			if !onlink {
				return errors.New("gateway is not on a configured link")
			}
		}
	}
	if err := c.validatePolicy(addID); err != nil {
		return err
	}
	masq := map[string]bool{}
	for _, id := range c.Firewall.Masquerade {
		i, ok := links[id]
		if !ok || i.Role != "wan" || masq[id] {
			return errors.New("masquerade requires unique WAN references")
		}
		masq[id] = true
	}
	ports := map[string]bool{}
	for _, f := range c.Firewall.Forwards {
		if err := addID(f.ID); err != nil {
			return err
		}
		i, ok := links[f.WAN]
		if !ok || i.Role != "wan" || !protocol(f.Protocol) || !port(f.Port) || !port(f.TargetPort) {
			return errors.New("invalid port forward")
		}
		a, e := netip.ParseAddr(f.Target)
		if e != nil || !a.Is4() || !usable(a) {
			return errors.New("forward target must be unicast IPv4")
		}
		found := false
		for _, lan := range c.Interfaces {
			if lan.Role != "lan" {
				continue
			}
			for _, raw := range lan.Addresses {
				p, _ := netip.ParsePrefix(raw)
				if p.Addr() == a {
					return errors.New("forward target cannot be a router address")
				}
				found = found || (p.Contains(a) && usableSubnetHost(p, a))
			}
		}
		if !found {
			return errors.New("forward target must belong to managed LAN and not router")
		}
		key := fmt.Sprintf("%s/%s/%d", f.WAN, f.Protocol, f.Port)
		if ports[key] {
			return errors.New("duplicate external port forward")
		}
		ports[key] = true
	}
	for _, r := range c.Firewall.Rules {
		if err := addID(r.ID); err != nil {
			return err
		}
		if _, ok := links[r.From]; !ok {
			return errors.New("rule source missing")
		}
		if _, ok := links[r.To]; !ok {
			return errors.New("rule destination missing")
		}
		if r.Action != "accept" && r.Action != "drop" {
			return errors.New("unsupported firewall action")
		}
		if r.Protocol != "any" && !protocol(r.Protocol) {
			return errors.New("unsupported firewall protocol")
		}
		if r.DestinationPort != 0 && (!port(r.DestinationPort) || r.Protocol == "any") {
			return errors.New("invalid rule destination port")
		}
	}
	for _, u := range c.DNS.Upstreams {
		a, e := netip.ParseAddr(u)
		if e != nil || !usable(a) {
			return errors.New("invalid DNS upstream")
		}
	}
	for _, r := range c.DNS.Records {
		a, e := netip.ParseAddr(r.Address)
		if e != nil || !usable(a) || len(r.Name) > 253 || !hostName.MatchString(r.Name) {
			return errors.New("invalid local DNS record")
		}
	}
	return nil
}

func validateDHCP(i Interface) error {
	d := i.DHCPServer
	if i.Role != "lan" || d.LeaseSeconds < 120 || d.LeaseSeconds > 604800 || len(d.Reservations) > 256 {
		return errors.New("invalid server role, lease, or reservation count")
	}
	start, e1 := netip.ParseAddr(d.Start)
	end, e2 := netip.ParseAddr(d.End)
	if e1 != nil || e2 != nil || !start.Is4() || !end.Is4() || start.Compare(end) > 0 {
		return errors.New("invalid IPv4 pool")
	}
	var subnet netip.Prefix
	for _, raw := range i.Addresses {
		p, _ := netip.ParsePrefix(raw)
		if p.Contains(start) && p.Contains(end) {
			subnet = p
			break
		}
	}
	if !subnet.IsValid() || subnet.Bits() > 30 {
		return errors.New("pool outside usable LAN subnet")
	}
	last := subnet.Masked().Addr().As4()
	hostmask := uint32(1)<<(32-subnet.Bits()) - 1
	n := uint32(last[0])<<24 | uint32(last[1])<<16 | uint32(last[2])<<8 | uint32(last[3])
	n |= hostmask
	broadcast := netip.AddrFrom4([4]byte{byte(n >> 24), byte(n >> 16), byte(n >> 8), byte(n)})
	if start == subnet.Masked().Addr() || end == broadcast {
		return errors.New("pool includes network or broadcast")
	}
	for _, raw := range i.Addresses {
		p, _ := netip.ParsePrefix(raw)
		if p.Addr().Compare(start) >= 0 && p.Addr().Compare(end) <= 0 {
			return errors.New("pool contains router address")
		}
	}
	macs := map[string]bool{}
	addrs := map[netip.Addr]bool{}
	routerAddresses := map[netip.Addr]bool{}
	for _, raw := range i.Addresses {
		p, _ := netip.ParsePrefix(raw)
		routerAddresses[p.Addr()] = true
	}
	for _, r := range d.Reservations {
		a, e := netip.ParseAddr(r.Address)
		if e != nil || !subnet.Contains(a) || routerAddresses[a] || a == subnet.Masked().Addr() || a == broadcast || addrs[a] || !unicastMAC(r.MAC) || macs[r.MAC] || len(r.Hostname) > 63 || !hostName.MatchString(r.Hostname) {
			return errors.New("invalid or duplicate DHCP reservation")
		}
		macs[r.MAC] = true
		addrs[a] = true
	}
	return nil
}

func unicastMAC(raw string) bool {
	if !macAddress.MatchString(raw) {
		return false
	}
	m, err := net.ParseMAC(raw)
	if err != nil || len(m) != 6 || m[0]&1 != 0 {
		return false
	}
	for _, b := range m {
		if b != 0 {
			return true
		}
	}
	return false
}

func usableSubnetHost(prefix netip.Prefix, addr netip.Addr) bool {
	if !prefix.Contains(addr) || !usable(addr) {
		return false
	}
	if !addr.Is4() || prefix.Bits() >= 31 {
		return true
	}
	if addr == prefix.Masked().Addr() {
		return false
	}
	b := addr.As4()
	n := uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
	hostmask := uint32(1)<<(32-prefix.Bits()) - 1
	return n&hostmask != hostmask
}
func usable(a netip.Addr) bool {
	return a.IsValid() && !a.IsUnspecified() && !a.IsMulticast() && !a.IsLoopback() && !a.Is4In6() && a.Zone() == ""
}
func protocol(p string) bool { return p == "tcp" || p == "udp" }
func port(p int) bool        { return p >= 1 && p <= 65535 }
