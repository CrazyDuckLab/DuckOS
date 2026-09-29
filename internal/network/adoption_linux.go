//go:build linux

package network

import "net"

// netlinkLiveLinks is the production LiveLinkAdopter. It answers
// LookupLink through the Go standard library's net.InterfaceByName,
// which the runtime fulfils with a single RTM_GETLINK netlink request;
// the call is read-only and never mutates the link or its state.
//
// The struct is a zero-cost handle: each LookupLink consults the kernel
// afresh. There is no in-adopter cache because the kernel's own link
// cache is the only source of truth and the production call sites invoke
// LookupLink from a single preflight goroutine. Tests construct the
// struct directly with an interfaceByName seam so they can swap a
// deterministic stub for the real netlink-backed lookup without making
// the production constructor conditional.
type netlinkLiveLinks struct {
	interfaceByName func(string) (*net.Interface, error)
}

// NewNetlinkLiveLinkAdopter returns the production LiveLinkAdopter
// wired to the Linux kernel via netlink. The constructor takes no
// arguments and the returned value is safe to share across goroutines
// because every LookupLink is a single read-only syscall.
func NewNetlinkLiveLinkAdopter() LiveLinkAdopter {
	return netlinkLiveLinks{interfaceByName: net.InterfaceByName}
}

// LookupLink returns the kernel's attested identity for an interface
// resolved by exact name. The mapping from caller-observed state to
// LiveLinkIdentity is:
//
//   - the kernel reports no link at that name → (zero, false); the
//     boundary treats this as a missing-link refusal;
//   - the kernel reports the link but the HardwareAddr is empty →
//     ({HasMAC: false}, true); the boundary treats this as a
//     no-stable-MAC refusal;
//   - the kernel reports the link with a non-empty HardwareAddr →
//     ({MAC: ..., HasMAC: true}, true); the boundary uses the MAC for
//     the hardware-address acceptance rule.
//
// The function never writes, never opens a control socket of its own
// and never blocks on a debounce timer. A single netlink dump fits in
// one packet; netlink failures degrade to (zero, false).
func (n netlinkLiveLinks) LookupLink(name string) (LiveLinkIdentity, bool) {
	lookup := n.interfaceByName
	if lookup == nil {
		lookup = net.InterfaceByName
	}
	link, err := lookup(name)
	if err != nil || link == nil {
		return LiveLinkIdentity{}, false
	}
	mac := link.HardwareAddr.String()
	if mac == "" {
		return LiveLinkIdentity{HasMAC: false}, true
	}
	return LiveLinkIdentity{MAC: mac, HasMAC: true}, true
}
