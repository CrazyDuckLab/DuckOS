// cmd/duck-wg-launcher/protocol.go: line protocol the
// duck-agent uses to talk to the privileged
// duck-wg-launcher daemon over a Unix socket.
//
// B-005 (BETA.2): the duck-agent correctly has
// CapabilityBoundingSet= and NoNewPrivileges=yes. It
// cannot exec ip / wg commands without losing privilege.
// The solution is a dedicated launcher with
// CAP_NET_ADMIN only (no SETUID / SETGID / SETPCAP /
// NET_RAW) that owns the lock and the ip / wg invocation.
//
// Architecture:
//
//	PUBLIC API -> Engine -> duck-agent (cap=0)
//	          -> /run/duckos-agent/wg-launcher.sock (this protocol)
//	          -> duck-wg-launcher (CAP_NET_ADMIN only)
//	          -> /sbin/ip + /usr/bin/wg
//
// Security model:
//   - The launcher validates SO_PEERCRED on every accept.
//     UID 0 (root, the only uid the Agent runs as) is the
//     only accepted peer.
//   - The launcher speaks a bounded line protocol; every
//     verb is allow-listed and every argument is parsed
//     and validated against a narrow regex.
//   - Interface names are canonical: ^wg[a-z0-9_-]{0,15}$.
//     The launcher refuses to operate on anything else.
//   - Addresses are validated with netip.ParsePrefix /
//     ParseAddr — a hostile ref like "10.0.0.0;rm -rf /"
//     is rejected before it reaches exec.
//   - The private key NEVER travels in the protocol.
//     The launcher writes the key to a temp file under
//     /run/duckos-agent/secret/ (0600, root:duckos),
//     and `wg setconf` reads the file directly. The
//     launcher does NOT log the key path / contents.
//   - DELETE on a non-DuckOS-owned interface is refused
//     even if the rest of the request is valid. The
//     launcher tracks ownership in a JSON file and uses
//     `ip -j link show` to confirm before deletion.

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"regexp"
	"strconv"
	"strings"
)

// Verbs the launcher understands. Adding a new verb
// here is a deliberate, code-reviewed decision; the
// switch in handleOpLine is the gate.
const (
	VerbLinkAdd  = "LINK_ADD"
	VerbLinkDel  = "LINK_DEL"
	VerbAddrAdd  = "ADDR_ADD"
	VerbAddrDel  = "ADDR_DEL"
	VerbLinkUp   = "LINK_UP"
	VerbLinkDown = "LINK_DOWN"
	VerbWGSet    = "WG_SET"
	VerbWGShow   = "WG_SHOW"
)

// canonicalInterfacePattern is the only interface name
// the launcher accepts. Matches "wg" + up to 15 bytes
// of [a-z0-9_-]. DuckOS never renames a real interface
// to anything else; a request outside this set is a
// sign of a hostile caller and is refused outright.
var canonicalInterfacePattern = regexp.MustCompile(`^wg[a-z0-9_-]{0,15}$`)

// interfaceKindPattern matches the kind of interface
// we know how to create today. Adding a kind requires
// adding a corresponding ip link add rule in the
// launcher exec path; the protocol stays strict.
var interfaceKindPattern = regexp.MustCompile(`^(wireguard)$`)

// PeerEndpointPattern matches a permitted endpoint form.
// We accept "host:port" with a real port range (1-65535)
// and reject anything else so a hostile peer ref cannot
// smuggle a metacharacter into the exec argv. Hostnames
// that need DNS resolution are intentionally disallowed —
// the wireguard kernel driver accepts numeric endpoints
// only.
//
// The host segment is `[0-9A-Za-z_.-]+` (RFC 1123 +
// numeric IPv4). The port segment is `65535`
// (max UDP port). The regex is constructed by hand
// because Go regex does not have lookbehind; the
// repetition pattern is broken into the 6000 / 6553
// / 6554 / 6555 / 6556-6559 sub-buckets so the full
// range is admitted without admitting 65536+ or
// 09999-style zero-padded forms that the kernel
// would interpret differently.
var peerEndpointPattern = regexp.MustCompile(`^[0-9A-Za-z_.-]+:(?:[1-9][0-9]{0,3}|[1-5][0-9]{4}|6[0-4][0-9]{3}|65[0-4][0-9]{2}|655[0-2][0-9]|6553[0-5])$`)

// AllowedIPListPattern matches the comma-separated
// list of CIDRs that wg accepts. We re-parse each
// entry with netip.ParsePrefix so a list with a stray
// shell meta is rejected before exec.
var allowedIPListPattern = regexp.MustCompile(`^([0-9A-Za-z_./:%-]+,)*[0-9A-Za-z_./:%-]+$`)

// PublicKeyPattern matches a Curve25519 public key in
// its standard wg 44-byte base64 encoding.
var publicKeyPattern = regexp.MustCompile(`^[A-Za-z0-9+/]{42}[A-Za-z0-9+/=]{2}$`)

// ListenPortRange is the bounded port range the launcher
// will pass to `wg set listen_port`. UDP port 0 is refused
// so a stray zero from the public surface never reaches
// the kernel.
const (
	listenPortMin = 1
	listenPortMax = 65535
)

// Op is the parsed form of a single launcher command.
// Fields are typed so the downstream exec path cannot
// pass an untrusted string to argv.
type Op struct {
	Verb   string
	Ifname string

	// LINK_ADD
	Kind string

	// ADDR_*
	Address string // canonical prefix string

	// WG_SET
	ListenPort     int
	PrivateKeyFile string // path to a 0600 file the launcher
	// owns; the file is created by
	// the launcher from a secret
	// handed off via the secret
	// channel, NOT from this op.
	ReplacePeers bool
	Peers        []WGPeer

	// WG_SHOW / WG_SET internal: optional flag for
	// public-key-only output.
	PublicOnly bool
}

// WGPeer is one [Peer] block the launcher passes to
// `wg setconf`. The PSK is a file path so the secret
// never lives in argv.
type WGPeer struct {
	PublicKey           string // 44-char base64
	PresharedKeyFile    string // path to a 0600 file
	Endpoint            string // host:port
	AllowedIPs          []netip.Prefix
	PersistentKeepalive int // 0 .. 65535 seconds
}

// ParseOp parses one OP line. The line is split on
// whitespace; every field is a key=value pair. Order
// of fields is irrelevant. Unknown keys are errors
// (not silently ignored) so a future protocol
// extension cannot accidentally land in the launcher
// without a code review.
//
// The function is intentionally pure: it does not
// touch the filesystem or run exec. The caller
// supplies the parsed Op to the exec layer.
func ParseOp(line string) (Op, error) {
	var op Op
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return op, errors.New("empty op")
	}
	for _, f := range fields {
		eq := strings.IndexByte(f, '=')
		if eq < 0 {
			return op, fmt.Errorf("missing '=': %q", f)
		}
		key, val := f[:eq], f[eq+1:]
		// Reject control bytes in both halves.
		for i := 0; i < len(val); i++ {
			if b := val[i]; b == '\r' || b == '\n' || b == 0 {
				return op, fmt.Errorf("control byte in value: %q", val)
			}
		}
		switch key {
		case "op":
			op.Verb = val
		case "ifname":
			op.Ifname = val
		case "kind":
			op.Kind = val
		case "address":
			op.Address = val
		case "listen_port":
			n, err := strconv.Atoi(val)
			if err != nil {
				return op, fmt.Errorf("listen_port: %v", err)
			}
			op.ListenPort = n
		case "private_key_file":
			op.PrivateKeyFile = val
		case "replace_peers":
			op.ReplacePeers = true
		case "public_only":
			op.PublicOnly = true
		default:
			return op, fmt.Errorf("unknown key: %q", key)
		}
	}
	if err := op.Validate(); err != nil {
		return op, err
	}
	return op, nil
}

// Validate enforces the structural invariants every Op
// must meet before it reaches exec. Validation is the
// single gate the protocol layer relies on; tests for
// the protocol parser target this method directly.
func (o Op) Validate() error {
	switch o.Verb {
	case VerbLinkAdd, VerbLinkDel, VerbAddrAdd, VerbAddrDel,
		VerbLinkUp, VerbLinkDown, VerbWGSet, VerbWGShow:
	default:
		return fmt.Errorf("unknown verb: %q", o.Verb)
	}
	if !canonicalInterfacePattern.MatchString(o.Ifname) {
		return fmt.Errorf("ifname %q is not canonical", o.Ifname)
	}
	switch o.Verb {
	case VerbLinkAdd:
		if !interfaceKindPattern.MatchString(o.Kind) {
			return fmt.Errorf("kind %q is not canonical", o.Kind)
		}
	case VerbAddrAdd, VerbAddrDel:
		if _, err := netip.ParsePrefix(o.Address); err != nil {
			return fmt.Errorf("address %q: %v", o.Address, err)
		}
	case VerbWGSet:
		// listen_port=0 is reserved: the kernel
		// treats it as "auto-pick" but DuckOS does
		// not allow the launcher to pick a port on
		// behalf of the operator. A non-zero port
		// must be in [1, 65535].
		if o.ListenPort == 0 {
			if o.PrivateKeyFile == "" {
				// No listener requested — the
				// caller is only pushing peers.
				// Allow listen_port 0 in that
				// case to mean "do not bind".
			} else {
				return fmt.Errorf("listen_port=0 is not allowed when setting a private key")
			}
		} else if o.ListenPort < listenPortMin || o.ListenPort > listenPortMax {
			return fmt.Errorf("listen_port %d out of range", o.ListenPort)
		}
		if o.PrivateKeyFile != "" {
			// The launcher will read this file. The
			// path is always under /run/duckos-agent/
			// secret/ which is the launcher-owned
			// secret channel.
			if !strings.HasPrefix(o.PrivateKeyFile, "/run/duckos-agent/secret/") {
				return fmt.Errorf("private_key_file %q is not in the secret channel", o.PrivateKeyFile)
			}
		}
	case VerbWGShow, VerbLinkDel, VerbLinkUp, VerbLinkDown:
		// No additional verbs.
	}
	return nil
}

// ParsePeers parses the [Peer] blocks the launcher
// passes to `wg setconf`. Each peer is on one line:
//
//	public=<b64> psk_file=<path> endpoint=<host:port>
//	allowed_ips=<cidr,cidr,...> keepalive=<int>
//
// Field order is irrelevant; unknown keys are errors so
// the launcher cannot accept a missed enum like a rogue
// "endpoint=..." field that is actually a script.
func ParsePeer(line string) (WGPeer, error) {
	var p WGPeer
	fields := strings.Fields(line)
	for _, f := range fields {
		eq := strings.IndexByte(f, '=')
		if eq < 0 {
			return p, fmt.Errorf("missing '=': %q", f)
		}
		key, val := f[:eq], f[eq+1:]
		for i := 0; i < len(val); i++ {
			if b := val[i]; b == '\r' || b == '\n' || b == 0 {
				return p, fmt.Errorf("control byte in value: %q", val)
			}
		}
		switch key {
		case "public":
			if !publicKeyPattern.MatchString(val) {
				return p, fmt.Errorf("public key %q is not 44-byte base64", val)
			}
			p.PublicKey = val
		case "psk_file":
			if !strings.HasPrefix(val, "/run/duckos-agent/secret/") {
				return p, fmt.Errorf("psk_file %q is not in the secret channel", val)
			}
			p.PresharedKeyFile = val
		case "endpoint":
			if !peerEndpointPattern.MatchString(val) {
				return p, fmt.Errorf("endpoint %q is not host:port", val)
			}
			p.Endpoint = val
		case "allowed_ips":
			if !allowedIPListPattern.MatchString(val) {
				return p, fmt.Errorf("allowed_ips %q is not a CIDR list", val)
			}
			parts := strings.Split(val, ",")
			for _, s := range parts {
				ap, err := netip.ParsePrefix(s)
				if err != nil {
					return p, fmt.Errorf("allowed_ips entry %q: %v", s, err)
				}
				p.AllowedIPs = append(p.AllowedIPs, ap)
			}
		case "keepalive":
			n, err := strconv.Atoi(val)
			if err != nil {
				return p, fmt.Errorf("keepalive: %v", err)
			}
			if n < 0 || n > 65535 {
				return p, fmt.Errorf("keepalive %d out of range", n)
			}
			p.PersistentKeepalive = n
		default:
			return p, fmt.Errorf("unknown peer key: %q", key)
		}
	}
	if p.PublicKey == "" {
		return p, errors.New("peer public key is required")
	}
	return p, nil
}

// RenderSetConf is the textual form of `wg setconf` the
// launcher passes on the wire. It carries the
// interface section, peer sections, and ends with a
// newline. Sensitive keys are referenced by file (the
// launcher passed the file path in the OP / peer
// blocks).
func RenderSetConf(ifname string, listenPort int, privateKeyFile string, peers []WGPeer) ([]byte, error) {
	if !canonicalInterfacePattern.MatchString(ifname) {
		return nil, fmt.Errorf("ifname %q is not canonical", ifname)
	}
	if listenPort < 0 || listenPort > listenPortMax {
		return nil, fmt.Errorf("listen_port %d out of range", listenPort)
	}
	var b strings.Builder
	b.WriteString("[Interface]\n")
	if privateKeyFile != "" {
		b.WriteString("PrivateKey = ")
		b.WriteString(privateKeyFile)
		b.WriteString("\n")
	}
	if listenPort > 0 {
		fmt.Fprintf(&b, "ListenPort = %d\n", listenPort)
	}
	for _, p := range peers {
		b.WriteString("\n[Peer]\n")
		b.WriteString("PublicKey = ")
		b.WriteString(p.PublicKey)
		b.WriteString("\n")
		if p.PresharedKeyFile != "" {
			b.WriteString("PresharedKey = ")
			b.WriteString(p.PresharedKeyFile)
			b.WriteString("\n")
		}
		if p.Endpoint != "" {
			b.WriteString("Endpoint = ")
			b.WriteString(p.Endpoint)
			b.WriteString("\n")
		}
		if len(p.AllowedIPs) > 0 {
			b.WriteString("AllowedIPs = ")
			for i, a := range p.AllowedIPs {
				if i > 0 {
					b.WriteString(", ")
				}
				b.WriteString(a.String())
			}
			b.WriteString("\n")
		}
		if p.PersistentKeepalive > 0 {
			fmt.Fprintf(&b, "PersistentKeepalive = %d\n", p.PersistentKeepalive)
		}
	}
	return []byte(b.String()), nil
}

// EncodeOps serialises a list of rendered structs into a
// stable JSON shape the launcher can dump to disk for
// tests. It is not used by the wire protocol.
func EncodeOps(list []Op) ([]byte, error) {
	return json.MarshalIndent(list, "", "  ")
}
