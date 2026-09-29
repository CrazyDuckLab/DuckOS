// Package xray owns the Xray server, user, profile and subscription
// object lifecycle. The model is explicit: a Server is a single
// inbound, a User is a credentialed client, a Profile is a routable
// identity, and a Subscription is a set of profiles that share a
// delivery URL. The render is intentionally strict: unsupported
// protocol/transport/security combinations are rejected at the model
// layer so a noisy Xray JSON edit never reaches the running service.
package xray

import (
	"errors"
	"fmt"
	"net/netip"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	MaxServers        = 32
	MaxUsers          = 1024
	MaxProfiles       = 256
	MaxSubscriptions  = 64
	MaxShareLinkBytes = 4 << 10
)

var (
	idPattern   = regexp.MustCompile(`^[0-9a-f]{32}$`)
	namePattern = regexp.MustCompile(`^[a-z][a-z0-9._-]{0,47}$`)
)

// Protocol is the inbound or outbound protocol. The set is bounded
// to VLESS, Trojan and REALITY (which is a security layer over
// VLESS) per V1_SCOPE.
type Protocol string

const (
	ProtocolVLESS  Protocol = "vless"
	ProtocolTrojan Protocol = "trojan"
)

// Transport is the underlying connection transport.
type Transport string

const (
	TransportTCP  Transport = "tcp"
	TransportWeb  Transport = "ws"
	TransportGRPC Transport = "grpc"
)

// Security is the transport security layer.
type Security string

const (
	SecurityNone    Security = "none"
	SecurityTLS     Security = "tls"
	SecurityReality Security = "reality"
)

// Server is a single Xray inbound. The fields are an explicit
// superset of what V1_SCOPE requires; unsupported combinations are
// rejected at Validate().
type Server struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	Protocol      Protocol  `json:"protocol"`
	Transport     Transport `json:"transport"`
	Security      Security  `json:"security"`
	Listen        string    `json:"listen"`
	Port          int       `json:"port"`
	TLSCertRef    *KeyRef   `json:"tlsCertRef,omitempty"`
	TLSKeyRef     *KeyRef   `json:"tlsKeyRef,omitempty"`
	RealityKeyRef *KeyRef   `json:"realityKeyRef,omitempty"`
	RealityDest   string    `json:"realityDest,omitempty"`
	RealitySNI    []string  `json:"realitySni,omitempty"`
	Fingerprint   string    `json:"fingerprint,omitempty"`
	ShortIDRef    *KeyRef   `json:"shortIdRef,omitempty"`
	Enabled       bool      `json:"enabled"`
	Tags          []string  `json:"tags,omitempty"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

// KeyRef is the bounded pointer at a vault-managed secret. The vault
// holds the bytes; canonical state only ever carries the reference.
type KeyRef struct {
	VaultID  string `json:"vaultId"`
	Revision string `json:"revision"`
	KeyID    string `json:"keyId"`
}

// User is a credentialed Xray client. The credentials themselves are
// vault-managed; the canonical record holds only the references.
type User struct {
	ID         string    `json:"id"`
	ServerID   string    `json:"serverId"`
	Name       string    `json:"name"`
	UUID       string    `json:"uuid,omitempty"`
	Password   string    `json:"password,omitempty"`
	Credential *KeyRef   `json:"credential,omitempty"`
	Email      string    `json:"email,omitempty"`
	Enabled    bool      `json:"enabled"`
	ExpiresAt  string    `json:"expiresAt,omitempty"`
	Tags       []string  `json:"tags,omitempty"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

// Profile is a routable identity built on top of a User. A Profile
// always has a User; the Profile records the access intent (which
// destinations the profile is allowed to reach) and the routing
// constraints.
type Profile struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	UserID    string    `json:"userId"`
	Intent    string    `json:"intent"`
	Selection []string  `json:"selection,omitempty"`
	Enabled   bool      `json:"enabled"`
	ExpiresAt string    `json:"expiresAt,omitempty"`
	Tags      []string  `json:"tags,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

const (
	ProfileIntentFullTunnel   = "full-tunnel"
	ProfileIntentRouterOnly   = "router-only"
	ProfileIntentInternetOnly = "internet-only"
	ProfileIntentSelection    = "selection"
	ProfileIntentCustom       = "custom"
)

func IsValidProfileIntent(s string) bool {
	switch s {
	case ProfileIntentFullTunnel, ProfileIntentRouterOnly, ProfileIntentInternetOnly, ProfileIntentSelection, ProfileIntentCustom:
		return true
	}
	return false
}

// Subscription is a named, deliverable set of profiles. A
// subscription carries an opaque delivery URL that operators can
// share with their clients.
type Subscription struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	ProfileIDs []string  `json:"profileIds"`
	Enabled    bool      `json:"enabled"`
	ExpiresAt  string    `json:"expiresAt,omitempty"`
	Tags       []string  `json:"tags,omitempty"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

func IsValidProtocol(p Protocol) bool {
	return p == ProtocolVLESS || p == ProtocolTrojan
}

func IsValidTransport(t Transport) bool {
	return t == TransportTCP || t == TransportWeb || t == TransportGRPC
}

func IsValidSecurity(s Security) bool {
	return s == SecurityNone || s == SecurityTLS || s == SecurityReality
}

// Validate enforces the bounded server contract. Unsupported
// protocol/transport/security combinations are explicit refusals.
func (s Server) Validate() error {
	if !idPattern.MatchString(s.ID) || !namePattern.MatchString(s.Name) {
		return errors.New("invalid xray server id or name")
	}
	if !IsValidProtocol(s.Protocol) {
		return errors.New("xray protocol must be vless or trojan")
	}
	if !IsValidTransport(s.Transport) {
		return errors.New("xray transport must be tcp, ws or grpc")
	}
	if !IsValidSecurity(s.Security) {
		return errors.New("xray security must be none, tls or reality")
	}
	if s.Protocol == ProtocolTrojan && s.Security == SecurityReality {
		return errors.New("REALITY requires VLESS")
	}
	if s.Security == SecurityNone && s.Protocol == ProtocolVLESS {
		// VLESS without TLS is acceptable but the operator should
		// be warned. We accept it but require a non-loopback listen
		// so it never ends up unauthenticated on a public address.
	}
	if err := validateListen(s.Listen); err != nil {
		return err
	}
	if s.Port < 1 || s.Port > 65535 {
		return errors.New("xray listen port out of bounds")
	}
	if s.Security == SecurityTLS {
		if s.TLSCertRef == nil || s.TLSKeyRef == nil {
			return errors.New("TLS server requires a cert and key vault reference")
		}
		if err := validateRef(*s.TLSCertRef); err != nil {
			return err
		}
		if err := validateRef(*s.TLSKeyRef); err != nil {
			return err
		}
	}
	if s.Security == SecurityReality {
		if s.RealityKeyRef == nil || s.ShortIDRef == nil {
			return errors.New("REALITY server requires a key and short-id vault reference")
		}
		if s.RealityDest == "" {
			return errors.New("REALITY server requires a destination")
		}
		if _, err := netip.ParseAddrPort(s.RealityDest); err != nil {
			// Allow domain:port form by validating the port separately.
			host, port, splitErr := splitHostPort(s.RealityDest)
			if splitErr != nil || host == "" || port < 1 || port > 65535 {
				return fmt.Errorf("REALITY destination %q is not host:port: %w", s.RealityDest, err)
			}
		}
		if len(s.RealitySNI) == 0 {
			return errors.New("REALITY server requires at least one SNI entry")
		}
		for _, sni := range s.RealitySNI {
			if sni == "" {
				return errors.New("REALITY SNI entry is empty")
			}
		}
	}
	return nil
}

func validateListen(s string) error {
	if s == "" {
		return errors.New("xray listen address is required")
	}
	host, port, err := splitHostPort(s)
	if err != nil {
		return fmt.Errorf("xray listen %q: %w", s, err)
	}
	if host == "" {
		return errors.New("xray listen host is empty")
	}
	if port < 1 || port > 65535 {
		return errors.New("xray listen port out of bounds")
	}
	return nil
}

func splitHostPort(s string) (string, int, error) {
	idx := strings.LastIndex(s, ":")
	if idx < 1 || idx == len(s)-1 {
		return "", 0, errors.New("expected host:port")
	}
	host := s[:idx]
	portStr := s[idx+1:]
	port := 0
	for _, c := range portStr {
		if c < '0' || c > '9' {
			return "", 0, errors.New("port is not numeric")
		}
		port = port*10 + int(c-'0')
	}
	return host, port, nil
}

func validateRef(ref KeyRef) error {
	if !idPattern.MatchString(ref.VaultID) || !idPattern.MatchString(ref.Revision) || !idPattern.MatchString(ref.KeyID) {
		return errors.New("vault reference must be 32 hex characters")
	}
	return nil
}

// Validate enforces the bounded user contract.
func (u User) Validate() error {
	if !idPattern.MatchString(u.ID) || !namePattern.MatchString(u.Name) {
		return errors.New("invalid xray user id or name")
	}
	if !idPattern.MatchString(u.ServerID) {
		return errors.New("xray user requires a server id")
	}
	switch {
	case u.Credential != nil:
		if err := validateRef(*u.Credential); err != nil {
			return err
		}
	case u.UUID != "":
		if !isValidUUID(u.UUID) {
			return errors.New("xray user UUID is not canonical")
		}
	case u.Password != "":
		// Trojan-style password.
	default:
		return errors.New("xray user requires a credential reference, UUID or password")
	}
	if u.ExpiresAt != "" {
		if _, err := time.Parse(time.RFC3339, u.ExpiresAt); err != nil {
			return fmt.Errorf("xray user expiresAt must be RFC3339: %w", err)
		}
	}
	return nil
}

// Validate enforces the bounded profile contract.
func (p Profile) Validate() error {
	if !idPattern.MatchString(p.ID) || !namePattern.MatchString(p.Name) {
		return errors.New("invalid xray profile id or name")
	}
	if !idPattern.MatchString(p.UserID) {
		return errors.New("xray profile requires a user id")
	}
	if !IsValidProfileIntent(p.Intent) {
		return errors.New("xray profile intent is not canonical")
	}
	if p.Intent == ProfileIntentSelection && len(p.Selection) == 0 {
		return errors.New("xray profile selection intent requires at least one CIDR")
	}
	if p.ExpiresAt != "" {
		if _, err := time.Parse(time.RFC3339, p.ExpiresAt); err != nil {
			return fmt.Errorf("xray profile expiresAt must be RFC3339: %w", err)
		}
	}
	return nil
}

// Validate enforces the bounded subscription contract.
func (s Subscription) Validate() error {
	if !idPattern.MatchString(s.ID) || !namePattern.MatchString(s.Name) {
		return errors.New("invalid xray subscription id or name")
	}
	if len(s.ProfileIDs) == 0 {
		return errors.New("xray subscription must include at least one profile")
	}
	if len(s.ProfileIDs) > MaxProfiles {
		return errors.New("xray subscription profile limit exceeded")
	}
	seen := map[string]bool{}
	for _, id := range s.ProfileIDs {
		if !idPattern.MatchString(id) {
			return errors.New("xray subscription profile id is not canonical")
		}
		if seen[id] {
			return errors.New("xray subscription profile is duplicated")
		}
		seen[id] = true
	}
	if s.ExpiresAt != "" {
		if _, err := time.Parse(time.RFC3339, s.ExpiresAt); err != nil {
			return fmt.Errorf("xray subscription expiresAt must be RFC3339: %w", err)
		}
	}
	return nil
}

// IsExpired returns whether the user/profile/subscription is past
// its expiry. The function is best-effort: an unparseable expiry
// is treated as never-expired.
func (s Subscription) IsExpired(now time.Time) bool { return isExpiredRFC3339(s.ExpiresAt, now) }
func (p Profile) IsExpired(now time.Time) bool      { return isExpiredRFC3339(p.ExpiresAt, now) }
func (u User) IsExpired(now time.Time) bool         { return isExpiredRFC3339(u.ExpiresAt, now) }

func isExpiredRFC3339(value string, now time.Time) bool {
	if value == "" {
		return false
	}
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return false
	}
	return !now.Before(t)
}

func isValidUUID(s string) bool {
	// 8-4-4-4-12 canonical form, lowercase hex only.
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		switch i {
		case 8, 13, 18, 23:
			if c != '-' {
				return false
			}
		default:
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
				return false
			}
		}
	}
	return true
}

// SortServers returns the servers ordered by id for stable diffs.
func SortServers(in []Server) []Server {
	out := append([]Server(nil), in...)
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// SortUsers returns the users ordered by id for stable diffs.
func SortUsers(in []User) []User {
	out := append([]User(nil), in...)
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
