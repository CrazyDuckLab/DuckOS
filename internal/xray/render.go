package xray

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Config is the bounded Xray JSON configuration that the Agent
// renders. It is the minimal superset the V1 contract requires:
// inbounds, outbounds, routing, and dns. The config is purely
// declarative; secret bytes are referenced via vault pointers, not
// embedded.
//
// The Version field is a DuckOS-internal marker (the on-disk journal
// version, NOT the xray-core schema version) and is intentionally
// hidden from the wire. xray-core rejects numeric values for the
// top-level `version` key (its own VersionConfig is a structured
// object) and the empty Xray config that DuckOS uses as its
// reconciliation baseline parses fine without one.
type Config struct {
	Version   int        `json:"-"`
	Inbounds  []Inbound  `json:"inbounds"`
	Outbounds []Outbound `json:"outbounds"`
	Routing   *Routing   `json:"routing,omitempty"`
	DNS       *DNS       `json:"dns,omitempty"`
}

type Inbound struct {
	Tag            string          `json:"tag"`
	Listen         string          `json:"listen"`
	Port           int             `json:"port"`
	Protocol       string          `json:"protocol"`
	Settings       json.RawMessage `json:"settings"`
	StreamSettings json.RawMessage `json:"streamSettings,omitempty"`
	Sniffing       *Sniffing       `json:"sniffing,omitempty"`
}

type Outbound struct {
	Tag      string `json:"tag"`
	Protocol string `json:"protocol"`
}

type Routing struct {
	DomainStrategy string        `json:"domainStrategy,omitempty"`
	Rules          []RoutingRule `json:"rules"`
}

type RoutingRule struct {
	Type        string   `json:"type"`
	OutboundTag string   `json:"outboundTag"`
	IPs         []string `json:"ip,omitempty"`
	Domains     []string `json:"domain,omitempty"`
	Port        string   `json:"port,omitempty"`
}

type DNS struct {
	Servers []string `json:"servers"`
}

type Sniffing struct {
	Enabled      bool     `json:"enabled"`
	DestOverride []string `json:"destOverride"`
}

// RenderConfig builds the Xray JSON config for the given objects.
// The render is bounded: only objects with valid intent and matched
// security are emitted. The function never embeds secret bytes; the
// rendered config carries vault pointers that the kernel adapter
// resolves before handing JSON to the Xray process.
func RenderConfig(servers []Server, users []User, profiles []Profile, subscriptions []Subscription) (Config, error) {
	if len(servers) > MaxServers {
		return Config{}, fmt.Errorf("xray: server limit exceeded (%d > %d)", len(servers), MaxServers)
	}
	cfg := Config{Version: 1}
	cfg.Outbounds = []Outbound{{Tag: "direct", Protocol: "freedom"}}
	for _, server := range servers {
		if !server.Enabled {
			continue
		}
		if err := server.Validate(); err != nil {
			return Config{}, fmt.Errorf("xray: server %s: %w", server.Name, err)
		}
		inbound, err := renderInbound(server, users)
		if err != nil {
			return Config{}, err
		}
		cfg.Inbounds = append(cfg.Inbounds, inbound)
	}
	// Build routing rules from profiles and subscriptions.
	rule := buildRoutingRule(profiles, subscriptions)
	if rule != nil {
		cfg.Routing = &Routing{DomainStrategy: "AsIs", Rules: []RoutingRule{*rule}}
	}
	return cfg, nil
}

// RenderRuntimeConfig omits listeners with no active clients. A draft has no
// listener; revoking its final client removes that listener on the next apply.
func RenderRuntimeConfig(servers []Server, users []User) (Config, error) {
	active := make([]Server, 0, len(servers))
	for _, server := range servers {
		if err := server.Validate(); err != nil {
			return Config{}, err
		}
		if server.Enabled && len(matchUsers(server, users)) > 0 {
			active = append(active, server)
		}
	}
	return RenderConfig(active, users, nil, nil)
}

func renderInbound(server Server, users []User) (Inbound, error) {
	matched := matchUsers(server, users)
	if len(matched) == 0 {
		return Inbound{}, fmt.Errorf("xray: server %s has no enabled users", server.Name)
	}
	settings, err := buildProtocolSettings(server, matched)
	if err != nil {
		return Inbound{}, err
	}
	stream, err := buildStreamSettings(server)
	if err != nil {
		return Inbound{}, err
	}
	tag := "in-" + server.Name
	// xray(8) inbound.listen is host-only; port is a
	// separate field. The production server.Listen
	// is host:port; split the host out so the rendered
	// config matches the Xray schema. xray(8) refuses
	// to load the config when listen is "host:port".
	host, _, err := splitHostPort(server.Listen)
	if err != nil {
		return Inbound{}, err
	}
	return Inbound{
		Tag:            tag,
		Listen:         host,
		Port:           server.Port,
		Protocol:       string(server.Protocol),
		Settings:       settings,
		StreamSettings: stream,
		Sniffing:       &Sniffing{Enabled: true, DestOverride: []string{"http", "tls"}},
	}, nil
}

func matchUsers(server Server, users []User) []User {
	matched := []User{}
	now := time.Now()
	for _, u := range users {
		if u.ServerID != server.ID || !u.Enabled || u.IsExpired(now) {
			continue
		}
		matched = append(matched, u)
	}
	sort.Slice(matched, func(i, j int) bool { return matched[i].ID < matched[j].ID })
	return matched
}

func buildProtocolSettings(server Server, users []User) (json.RawMessage, error) {
	switch server.Protocol {
	case ProtocolVLESS:
		// VLESS inbound settings: xray(8) requires
		// settings.clients (NOT settings.vnext —
		// vnext is the outbound form). xray(8) also
		// requires settings.decryption: "none";
		// without it xray fails to start with
		// "please add/set 'decryption':'none'".
		clients := []map[string]any{}
		for _, u := range users {
			entry := map[string]any{"id": u.UUID, "level": 0}
			if u.Email != "" {
				entry["email"] = u.Email
			}
			clients = append(clients, entry)
		}
		payload := map[string]any{
			"clients":    clients,
			"decryption": "none",
		}
		return json.Marshal(payload)
	case ProtocolTrojan:
		passwords := []map[string]any{}
		for _, u := range users {
			entry := map[string]any{"password": u.Password, "level": 0}
			if u.Email != "" {
				entry["email"] = u.Email
			}
			passwords = append(passwords, entry)
		}
		payload := map[string]any{
			"clients": passwords,
		}
		return json.Marshal(payload)
	}
	return nil, fmt.Errorf("xray: unsupported protocol %q", server.Protocol)
}

func buildStreamSettings(server Server) (json.RawMessage, error) {
	switch server.Security {
	case SecurityNone:
		// No transport security, no stream settings.
		return nil, nil
	case SecurityTLS:
		settings := map[string]any{
			"security": "tls",
			"tlsSettings": map[string]any{
				"certificates": []map[string]any{{
					"certificateFile": "/var/lib/duckos-agent/xray/" + server.Name + ".crt",
					"keyFile":         "/var/lib/duckos-agent/xray/" + server.Name + ".key",
				}},
			},
		}
		applyTransport(server, settings)
		return json.Marshal(settings)
	case SecurityReality:
		settings := map[string]any{
			"security": "reality",
			"realitySettings": map[string]any{
				"dest":        server.RealityDest,
				"serverNames": sortedStrings(server.RealitySNI),
				"fingerprint": server.Fingerprint,
				"shortIds": []string{
					"<resolved at apply time; see vault record " + keyIDOf(server.ShortIDRef) + ">",
				},
				"privateKey": "<resolved at apply time; see vault record " + keyIDOf(server.RealityKeyRef) + ">",
			},
		}
		applyTransport(server, settings)
		return json.Marshal(settings)
	}
	return nil, fmt.Errorf("xray: unsupported security %q", server.Security)
}

func applyTransport(server Server, settings map[string]any) {
	switch server.Transport {
	case TransportTCP:
		settings["network"] = "tcp"
	case TransportWeb:
		settings["network"] = "ws"
		settings["wsSettings"] = map[string]any{
			"path": "/" + server.Name,
		}
	case TransportGRPC:
		settings["network"] = "grpc"
		settings["grpcSettings"] = map[string]any{
			"serviceName": "duckos-" + server.Name,
		}
	}
}

func buildRoutingRule(profiles []Profile, _ []Subscription) *RoutingRule {
	if len(profiles) == 0 {
		return nil
	}
	ips := []string{}
	intent := ""
	now := time.Now()
	for _, p := range profiles {
		// M-24: every profile must pass Validate before
		// being rendered. A profile with an empty UserID or
		// a non-canonical Intent would otherwise silently
		// drop into the routing rule.
		if err := p.Validate(); err != nil {
			continue
		}
		if !p.Enabled || p.IsExpired(now) {
			continue
		}
		if intent == "" {
			intent = p.Intent
		}
		ips = append(ips, p.Selection...)
	}
	if intent == "" {
		return nil
	}
	ips = sortedStrings(ips)
	switch intent {
	case ProfileIntentFullTunnel, ProfileIntentInternetOnly:
		return &RoutingRule{Type: "field", OutboundTag: "direct"}
	case ProfileIntentSelection:
		return &RoutingRule{Type: "field", OutboundTag: "direct", IPs: ips}
	case ProfileIntentRouterOnly, ProfileIntentCustom:
		// Renderer must not silently choose a default. The default
		// rule for these intents is "no route", which is enforced
		// by leaving the routing block nil.
	}
	return nil
}

func sortedStrings(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}

func keyIDOf(ref *KeyRef) string {
	if ref == nil {
		return ""
	}
	return ref.KeyID
}

// BuildShareLink returns a vless:// or trojan:// URI for a server
// and user. The link is bounded to 4 KiB. The credential is
// referenced by the supplied KeyRef; secret bytes are never
// embedded.
func BuildShareLink(server Server, user User) (string, error) {
	if err := server.Validate(); err != nil {
		return "", err
	}
	if err := user.Validate(); err != nil {
		return "", err
	}
	host, port, err := splitHostPort(server.Listen)
	if err != nil {
		return "", err
	}
	var sb strings.Builder
	switch server.Protocol {
	case ProtocolVLESS:
		if user.Credential == nil {
			return "", errors.New("VLESS share link requires a credential reference")
		}
		fmt.Fprintf(&sb, "vless://%s@%s:%d", user.Credential.VaultID, host, port)
		if user.Name != "" {
			fmt.Fprintf(&sb, "#%s", user.Name)
		}
	case ProtocolTrojan:
		if user.Credential == nil {
			return "", errors.New("Trojan share link requires a credential reference")
		}
		fmt.Fprintf(&sb, "trojan://%s@%s:%d", user.Credential.VaultID, host, port)
		if user.Name != "" {
			fmt.Fprintf(&sb, "#%s", user.Name)
		}
	default:
		return "", fmt.Errorf("xray: unsupported protocol %q", server.Protocol)
	}
	if len(sb.String()) > MaxShareLinkBytes {
		return "", errors.New("xray share link exceeds size limit")
	}
	return sb.String(), nil
}

// SubscriptionURL is the public, base64-encoded
// subscription URL payload. The format is the standard
// Xray base64-of-UTF8-newline-separated-share-links
// payload (one vless:// or trojan:// per line, base64
// encoded). The decoded payload is exactly the set of
// share links the renderer produces for the
// subscription's enabled profiles; the URL itself is
// the encoded representation an Xray-compatible client
// fetches.
//
// DV04-XRAY contract: the decoded payload of the
// subscription URL must equal the renderer-built
// payload for the same set of (server, user) pairs;
// the contract pins "decoded == rendered" so a QR
// round-trip is exact.
type SubscriptionURL struct {
	// URL is the public, base64-encoded subscription
	// URL the client fetches.
	URL string
	// Lines is the canonical share-link list the
	// renderer produced. Decoding URL (base64 std) and
	// splitting on newline MUST yield Lines
	// character-for-character.
	Lines []string
}

// BuildSubscriptionURL encodes the share-link list the
// renderer produces for the supplied servers + users
// as a base64-UTF8-newline-separated subscription URL.
// The encoded payload round-trips through standard
// base64 decode + newline split to exactly the same
// Lines slice. The function never embeds secret bytes;
// each share link carries a vault reference, not the
// raw credential.
func BuildSubscriptionURL(servers []Server, users []User) (SubscriptionURL, error) {
	var lines []string
	for _, s := range servers {
		if !s.Enabled {
			continue
		}
		if err := s.Validate(); err != nil {
			return SubscriptionURL{}, fmt.Errorf("xray: subscription server %s: %w", s.Name, err)
		}
		for _, u := range users {
			if u.ServerID != s.ID || !u.Enabled {
				continue
			}
			link, err := BuildShareLink(s, u)
			if err != nil {
				return SubscriptionURL{}, fmt.Errorf("xray: subscription link %s/%s: %w", s.Name, u.Name, err)
			}
			lines = append(lines, link)
		}
	}
	sort.Strings(lines)
	payload := strings.Join(lines, "\n")
	if payload == "" {
		return SubscriptionURL{}, errors.New("xray: subscription has no enabled links")
	}
	encoded := base64.StdEncoding.EncodeToString([]byte(payload))
	return SubscriptionURL{URL: encoded, Lines: lines}, nil
}

// DecodeSubscriptionURL returns the canonical Lines
// slice the encoded subscription URL carries. The
// decode is strict base64 (no whitespace tolerance
// beyond the standard newline) and the result MUST
// equal the renderer's Lines for the same (servers,
// users) tuple. The function is the second half of the
// QR round-trip the audit requires.
func DecodeSubscriptionURL(url string) ([]string, error) {
	raw, err := base64.StdEncoding.DecodeString(url)
	if err != nil {
		return nil, fmt.Errorf("xray: subscription url is not strict base64: %w", err)
	}
	text := string(raw)
	if text == "" {
		return nil, errors.New("xray: subscription url decoded to empty")
	}
	lines := strings.Split(text, "\n")
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		if l == "" {
			continue
		}
		out = append(out, l)
	}
	if len(out) == 0 {
		return nil, errors.New("xray: subscription url decoded to empty")
	}
	return out, nil
}
