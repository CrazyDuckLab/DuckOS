// Package wireguard owns the WireGuard server, peer and key lifecycle for
// DuckOS. The model is explicit and stable: a Server is a single tunnel
// interface, a Peer is a public-key/allowed-IP tuple with optional
// preshared key, expiry and isolated routing intent. Servers and peers
// are addressable by stable 32-hex ids and persisted in the canonical
// state journal. The package never holds private key bytes; the
// Generation hook returns them only to the secret owner.
package wireguard

import (
	"bytes"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net/netip"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	MaxServers = 32
	MaxPeers   = 256
	MaxPSK     = 32
)

var idPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)
var endpointName = regexp.MustCompile(`^[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$`)
var serverName = regexp.MustCompile(`^[a-z][a-z0-9._-]{0,47}$`)

// Server is a single WireGuard tunnel. It owns a private key (not
// persisted in canonical state) and a public identity. The
// AllowedPeers list is the set of peers the server has been instructed
// to accept; a peer is rejected by the kernel adapter if it is not in
// the list.
type Server struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	Interface     string    `json:"interface"`
	ListenPort    int       `json:"listenPort"`
	Addresses     []string  `json:"addresses"`
	DNS           []string  `json:"dns,omitempty"`
	MTU           int       `json:"mtu,omitempty"`
	PrivateKeyRef KeyRef    `json:"privateKey"`
	PublicKey     string    `json:"publicKey"`
	Peers         []Peer    `json:"peers"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

// KeyRef is a pointer at the secret store; private key bytes are
// looked up via Reference lookup, never copied into canonical state.
type KeyRef struct {
	VaultID   string `json:"vaultId"`
	Revision  string `json:"revision"`
	KeyID     string `json:"keyId"`
	CreatedAt string `json:"createdAt"`
}

// Peer is a public-key/allowed-IP tuple the server is configured to
// accept. A peer is rendered into the kernel and the corresponding
// client document is generated from the same record.
type Peer struct {
	ID                  string   `json:"id"`
	ServerID            string   `json:"serverId"`
	Name                string   `json:"name"`
	PublicKey           string   `json:"publicKey"`
	PresharedKeyRef     *KeyRef  `json:"presharedKeyRef,omitempty"`
	AllowedIPs          []string `json:"allowedIPs"`
	Endpoint            string   `json:"endpoint,omitempty"`
	PersistentKeepalive int      `json:"persistentKeepalive,omitempty"`
	ExpiresAt           string   `json:"expiresAt,omitempty"`
	IsolatedTo          []string `json:"isolatedTo,omitempty"`
	// Intent is the explicit isolation policy. FullTunnel means the peer
	// is the user's whole network (default router). RouterOnly limits the
	// peer to the router's management plane. InternetOnly restricts to
	// the WAN. Selection uses the IsolatedTo allow list. Custom is a
	// free-form AllowedIPs set.
	Intent    string    `json:"intent"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// IsValidIntent returns whether the named intent is one of the
// canonical isolation modes.
func IsValidIntent(s string) bool {
	switch s {
	case IntentFullTunnel, IntentRouterOnly, IntentInternetOnly, IntentSelection, IntentCustom:
		return true
	}
	return false
}

const (
	IntentFullTunnel   = "full-tunnel"
	IntentRouterOnly   = "router-only"
	IntentInternetOnly = "internet-only"
	IntentSelection    = "selection"
	IntentCustom       = "custom"
)

// GenerateServerKey returns (private, public) WireGuard X25519 keys.
// Private bytes are the secret owner's responsibility; the public key
// is canonical and stored in Server.PublicKey. The caller is expected
// to wrap the private bytes in a vault record and reference it via
// KeyRef.
func GenerateServerKey() (priv []byte, pub string, err error) {
	priv = make([]byte, 32)
	if _, err = rand.Read(priv); err != nil {
		return nil, "", errors.New("wireguard key generation unavailable")
	}
	priv[0] &= 248
	priv[31] &= 127
	priv[31] |= 64
	k, err := ecdh.X25519().NewPrivateKey(priv)
	if err != nil {
		return nil, "", errors.New("wireguard key generation unavailable")
	}
	return priv, base64.StdEncoding.EncodeToString(k.PublicKey().Bytes()), nil
}

// ValidateServer enforces the bounded contract every Server must
// satisfy. It never touches the network; it only refuses malformed
// intent before the kernel adapter sees it.
func (s Server) Validate() error {
	if !idPattern.MatchString(s.ID) || !serverName.MatchString(s.Name) {
		return errors.New("invalid server id or name")
	}
	if !serverName.MatchString(s.Interface) {
		return errors.New("invalid wireguard interface name")
	}
	if s.ListenPort < 1 || s.ListenPort > 65535 {
		return errors.New("wireguard listen port out of bounds")
	}
	if len(s.Addresses) == 0 || len(s.Addresses) > 16 {
		return errors.New("wireguard server requires 1..16 addresses")
	}
	for _, raw := range s.Addresses {
		p, err := netip.ParsePrefix(raw)
		if err != nil {
			return fmt.Errorf("invalid wireguard address %q: %w", raw, err)
		}
		if !p.Addr().IsValid() || p.Addr().IsMulticast() || p.Addr().IsUnspecified() {
			return fmt.Errorf("invalid wireguard address %q", raw)
		}
	}
	if s.MTU != 0 && (s.MTU < 1280 || s.MTU > 9000) {
		return errors.New("wireguard mtu out of bounds")
	}
	if s.PrivateKeyRef.VaultID == "" || s.PrivateKeyRef.Revision == "" || s.PrivateKeyRef.KeyID == "" {
		return errors.New("wireguard server requires a vault key reference")
	}
	if s.PublicKey == "" {
		return errors.New("wireguard server public key is required")
	}
	if len(s.Peers) > MaxPeers {
		return errors.New("wireguard server peer limit exceeded")
	}
	seenPeers := map[string]bool{}
	for _, p := range s.Peers {
		if err := p.Validate(s.ID); err != nil {
			return err
		}
		if seenPeers[p.PublicKey] {
			return errors.New("duplicate peer public key")
		}
		seenPeers[p.PublicKey] = true
	}
	return nil
}

// Validate enforces the bounded peer contract.
func (p Peer) Validate(serverID string) error {
	if !idPattern.MatchString(p.ID) || p.ServerID != serverID {
		return errors.New("invalid peer id or server reference")
	}
	if !serverName.MatchString(p.Name) {
		return errors.New("invalid peer name")
	}
	if len(p.PublicKey) != 44 {
		return errors.New("peer public key is not a 32-byte base64 value")
	}
	if p.PresharedKeyRef != nil {
		if p.PresharedKeyRef.VaultID == "" || p.PresharedKeyRef.Revision == "" || p.PresharedKeyRef.KeyID == "" {
			return errors.New("peer preshared key reference is incomplete")
		}
	}
	if len(p.AllowedIPs) == 0 || len(p.AllowedIPs) > 64 {
		return errors.New("peer requires 1..64 allowedIPs")
	}
	seen := map[string]bool{}
	for _, raw := range p.AllowedIPs {
		prefix, err := netip.ParsePrefix(raw)
		if err != nil || prefix != prefix.Masked() || prefix.Addr().IsMulticast() || seen[raw] {
			return fmt.Errorf("peer allowedIPs %q is invalid", raw)
		}
		seen[raw] = true
	}
	if p.Endpoint != "" {
		host, port, err := splitEndpoint(p.Endpoint)
		if err != nil {
			return fmt.Errorf("peer endpoint: %w", err)
		}
		if _, err := strconv.Atoi(port); err != nil {
			return errors.New("peer endpoint port is not numeric")
		}
		_ = host
	}
	if p.PersistentKeepalive < 0 || p.PersistentKeepalive > 65535 {
		return errors.New("peer persistent keepalive out of bounds")
	}
	if !IsValidIntent(p.Intent) {
		return errors.New("peer intent is not a canonical value")
	}
	if p.ExpiresAt != "" {
		if _, err := time.Parse(time.RFC3339, p.ExpiresAt); err != nil {
			return fmt.Errorf("peer expiresAt must be RFC3339: %w", err)
		}
	}
	return nil
}

// IsExpired returns whether the peer is past its expiry. The function
// is best-effort; an unparseable expiry is treated as never-expired.
func (p Peer) IsExpired(now time.Time) bool {
	if p.ExpiresAt == "" {
		return false
	}
	t, err := time.Parse(time.RFC3339, p.ExpiresAt)
	if err != nil {
		return false
	}
	return !now.Before(t)
}

func splitEndpoint(s string) (string, string, error) {
	idx := strings.LastIndex(s, ":")
	if idx < 1 || idx == len(s)-1 {
		return "", "", errors.New("endpoint must be host:port")
	}
	host := s[:idx]
	port := s[idx+1:]
	if host == "" {
		return "", "", errors.New("endpoint host is empty")
	}
	if !endpointName.MatchString(host) {
		// IPv4 literal or [IPv6] literal are accepted; the kernel
		// adapter validates routability.
		if _, err := netip.ParseAddr(host); err != nil {
			return "", "", fmt.Errorf("endpoint host %q is not valid", host)
		}
	}
	return host, port, nil
}

// AllowedIPsForIntent returns the canonical AllowedIPs set for the
// peer's intent. The function is the single source of truth for the
// intent → AllowedIPs mapping; the kernel adapter, the renderer and
// the client-export path all read through it.
func AllowedIPsForIntent(intent string, selection []string) ([]string, error) {
	switch intent {
	case IntentFullTunnel:
		return []string{"0.0.0.0/0", "::/0"}, nil
	case IntentRouterOnly:
		return []string{}, errors.New("router-only intent is the kernel adapter's job; the AllowedIPs set is empty")
	case IntentInternetOnly:
		// The kernel adapter may re-intersect with the WAN's interface
		// routes; the renderer treats this as "no private subnet".
		return []string{"0.0.0.0/0", "::/0"}, nil
	case IntentSelection:
		if len(selection) == 0 {
			return nil, errors.New("selection intent requires at least one explicit CIDR")
		}
		out := append([]string(nil), selection...)
		sort.Strings(out)
		return out, nil
	case IntentCustom:
		return nil, nil
	}
	return nil, errors.New("unknown peer intent")
}

// EncodeBase64Key returns the canonical base64 form of a 32-byte key.
func EncodeBase64Key(b []byte) string {
	return base64.StdEncoding.EncodeToString(b)
}

// DecodeBase64Key returns the strict 32-byte form of a base64 WireGuard
// key. It refuses weakly-encoded or all-zero keys.
func DecodeBase64Key(s string) ([]byte, error) {
	b, err := base64.StdEncoding.Strict().DecodeString(s)
	if err != nil {
		return nil, errors.New("wireguard key is not strict base64")
	}
	if len(b) != 32 || bytes.Equal(b, make([]byte, 32)) {
		return nil, errors.New("wireguard key length is invalid")
	}
	return b, nil
}
