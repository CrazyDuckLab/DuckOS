// dnsmasq.go: cross-platform reader for dnsmasq lease files
// (the format used by duck-dnsmasq.service). This is the
// read-only observation source behind the public
// /api/v1/dhcp/leases surface: the API never invents lease
// state, it reports what the DHCP server actually persists.
//
// dnsmasq lease-file line format (one lease per line):
//
//	<unix-expiry> <mac> <ipv4-or-ipv6> <hostname|*> <clientid|*>
//
// Lines that do not parse are skipped, never fatal: a
// truncated or mid-write file must not take the API down.
package dhcpsnoop

import (
	"bufio"
	"net/netip"
	"strconv"
	"strings"
	"time"
)

// ParseDnsmasqLeases decodes a dnsmasq lease file. Unknown
// or malformed lines are skipped; the hostname and client
// id are carried through when present.
func ParseDnsmasqLeases(data []byte) []Lease {
	var out []Lease
	sc := bufio.NewScanner(strings.NewReader(string(data)))
	sc.Buffer(make([]byte, 0, 4096), 16<<10)
	now := time.Now().Unix()
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		expiry, err := strconv.ParseInt(fields[0], 10, 64)
		if err != nil || expiry < 0 {
			continue
		}
		mac := normalizeMAC(fields[1])
		if mac == "" {
			continue
		}
		addr, err := netip.ParseAddr(fields[2])
		if err != nil {
			continue
		}
		lease := Lease{
			MAC:      mac,
			IP:       addr.String(),
			Expiry:   expiry,
			Trusted:  true,
			Observed: now,
		}
		if fields[3] != "*" {
			lease.Hostname = fields[3]
		}
		out = append(out, lease)
	}
	return out
}

// normalizeMAC accepts the common aa:bb:cc:dd:ee:ff and
// aa-bb-cc-dd-ee-ff spellings and returns the colon form.
// Anything else is refused (empty string).
func normalizeMAC(in string) string {
	in = strings.ToLower(strings.ReplaceAll(in, "-", ":"))
	parts := strings.Split(in, ":")
	if len(parts) != 6 {
		return ""
	}
	for _, p := range parts {
		if len(p) != 2 {
			return ""
		}
		for _, ch := range p {
			if (ch < '0' || ch > '9') && (ch < 'a' || ch > 'f') {
				return ""
			}
		}
	}
	return in
}
