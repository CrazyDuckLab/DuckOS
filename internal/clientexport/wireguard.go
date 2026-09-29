package clientexport

import (
	"bytes"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"regexp"
	"strconv"
	"strings"
)

type WireGuardClient struct {
	Addresses           []string
	DNS                 []string
	ServerPublicKey     string
	Endpoint            string
	AllowedIPs          []string
	MTU                 int
	PersistentKeepalive int
}

// GenerateWireGuardKey uses OS randomness and the standard X25519 clamp.
// The private bytes are returned only to the secret owner, never printed.
func GenerateWireGuardKey() ([]byte, string, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, "", errors.New("WireGuard key generation unavailable")
	}
	key[0] &= 248
	key[31] &= 127
	key[31] |= 64
	private, err := ecdh.X25519().NewPrivateKey(key)
	if err != nil {
		return nil, "", errors.New("WireGuard key generation unavailable")
	}
	return key, base64.StdEncoding.EncodeToString(private.PublicKey().Bytes()), nil
}

var endpointName = regexp.MustCompile(`^[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$`)

func clientAddress(a netip.Addr) bool {
	return a.IsValid() && !a.IsUnspecified() && !a.IsMulticast() && !a.Is4In6() && a.Zone() == ""
}

// WireGuard renders standard mobile/wg-quick client configuration, never hooks
// or host scripts. Secrets are separate parameters, not canonical public fields.
func WireGuard(id, revision string, c WireGuardClient, privateKey, presharedKey []byte) (*Document, error) {
	invalid := errors.New("invalid WireGuard client configuration")
	if !objectID.MatchString(id) || len(privateKey) != 32 || bytes.Equal(privateKey, make([]byte, 32)) ||
		(len(presharedKey) != 0 && (len(presharedKey) != 32 || bytes.Equal(presharedKey, make([]byte, 32)))) ||
		len(c.Addresses) == 0 || len(c.Addresses) > 16 || len(c.DNS) > 8 || len(c.AllowedIPs) == 0 || len(c.AllowedIPs) > 64 ||
		(c.MTU != 0 && (c.MTU < 1280 || c.MTU > 9000)) || c.PersistentKeepalive < 0 || c.PersistentKeepalive > 65535 {
		return nil, invalid
	}
	serverKey, err := base64.StdEncoding.Strict().DecodeString(c.ServerPublicKey)
	if err != nil || len(serverKey) != 32 || base64.StdEncoding.EncodeToString(serverKey) != c.ServerPublicKey || bytes.Equal(serverKey, make([]byte, 32)) {
		return nil, invalid
	}
	private, err := ecdh.X25519().NewPrivateKey(privateKey)
	if err != nil || bytes.Equal(private.PublicKey().Bytes(), serverKey) {
		return nil, invalid
	}
	peer, err := ecdh.X25519().NewPublicKey(serverKey)
	if err != nil {
		return nil, invalid
	}
	if _, err := private.ECDH(peer); err != nil {
		return nil, invalid
	}
	host, port, err := net.SplitHostPort(c.Endpoint)
	if err != nil || len(host) > 253 {
		return nil, invalid
	}
	number, err := strconv.Atoi(port)
	if err != nil || number < 1 || number > 65535 || strconv.Itoa(number) != port {
		return nil, invalid
	}
	if ip, err := netip.ParseAddr(host); err == nil {
		if !clientAddress(ip) {
			return nil, invalid
		}
	} else if !endpointName.MatchString(host) {
		return nil, invalid
	}
	seen := map[string]bool{}
	for _, raw := range c.Addresses {
		p, err := netip.ParsePrefix(raw)
		if err != nil || !clientAddress(p.Addr()) || p.String() != raw || seen[raw] {
			return nil, invalid
		}
		seen[raw] = true
	}
	clear(seen)
	for _, raw := range c.DNS {
		a, err := netip.ParseAddr(raw)
		if err != nil || !clientAddress(a) || a.String() != raw || seen[raw] {
			return nil, invalid
		}
		seen[raw] = true
	}
	clear(seen)
	for _, raw := range c.AllowedIPs {
		p, err := netip.ParsePrefix(raw)
		if err != nil || p != p.Masked() || p.String() != raw || p.Addr().Is4In6() || p.Addr().IsMulticast() || seen[raw] {
			return nil, invalid
		}
		seen[raw] = true
	}
	var b strings.Builder
	fmt.Fprintf(&b, "[Interface]\nPrivateKey = %s\nAddress = %s\n", base64.StdEncoding.EncodeToString(privateKey), strings.Join(c.Addresses, ", "))
	if len(c.DNS) > 0 {
		fmt.Fprintf(&b, "DNS = %s\n", strings.Join(c.DNS, ", "))
	}
	if c.MTU != 0 {
		fmt.Fprintf(&b, "MTU = %d\n", c.MTU)
	}
	fmt.Fprintf(&b, "\n[Peer]\nPublicKey = %s\n", c.ServerPublicKey)
	if len(presharedKey) > 0 {
		fmt.Fprintf(&b, "PresharedKey = %s\n", base64.StdEncoding.EncodeToString(presharedKey))
	}
	fmt.Fprintf(&b, "AllowedIPs = %s\nEndpoint = %s\n", strings.Join(c.AllowedIPs, ", "), c.Endpoint)
	if c.PersistentKeepalive != 0 {
		fmt.Fprintf(&b, "PersistentKeepalive = %d\n", c.PersistentKeepalive)
	}
	return NewDocument(id, revision, "wireguard", "duck-"+id[:8]+".conf", b.String())
}
