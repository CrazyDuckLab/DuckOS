// bind.go: the cross-platform listener-bind descriptor. The
// Linux socket layer consumes it; the cross-platform tests
// exercise the security predicates without sockets.
package frontend

import (
	"net/netip"

	"github.com/CrazyDuckLab/DuckOS/internal/upnp"
)

// Bind describes one network the listeners serve.
type Bind struct {
	NetworkRef string
	Iface      string
	// Addrs are the interface's local IPv4 addresses (one
	// listener socket set per address).
	Addrs []netip.Addr
	// Prefixes are the interface's on-link IPv4 subnets.
	// When non-empty, a mapping client whose source address
	// falls outside every prefix is refused as cross-subnet
	// (beta.3 security requirement). Empty means "unknown
	// topology": the check degrades to accept and the
	// allow-list remains the control.
	Prefixes []netip.Prefix
	// Protocols selects which listeners start for this bind.
	Protocols []upnp.Protocol
}

// InPrefixes reports whether addr falls inside at least one
// of the bind's on-link prefixes.
func (b Bind) InPrefixes(addr netip.Addr) bool {
	if len(b.Prefixes) == 0 {
		return true
	}
	for _, p := range b.Prefixes {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}
