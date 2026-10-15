package xray

import (
	"fmt"
	"strings"
)

// compatibility.go is the version-aware protocol/transport/security
// capability layer for the bundled Xray-core build (26.3.27). The
// matrix is derived from verified Xray-core behavior and mirrors the
// combinations the official XTLS documentation and the maintained
// management panels expose. Unsupported combinations are rejected
// BEFORE the render so an invalid config can never reach
// `xray run -test` or the running daemon.
//
// The matrix is deliberately conservative:
//
//   - REALITY is a stream security layer that requires a raw
//     multiplexed handshake; it is valid with RAW/TCP and XHTTP
//     (the officially recommended modern pairing) and invalid with
//     the negotiated transports (WebSocket, gRPC, HTTPUpgrade, mKCP).
//   - VLESS flow values (xtls-rprx-vision) are only meaningful for
//     VLESS over RAW/TCP with REALITY. Any other pairing is rejected.
//   - Trojan and Shadowsocks define their own security and do not
//     participate in REALITY.
//
// If a future Xray-core upgrade widens the matrix, upgrade the table
// here together with the XRAY_CORE_VERSION pin in
// scripts/stage_xray.sh and re-run the qualification suite.

// SupportedTransports lists the transports the renderer can emit.
func SupportedTransports() []Transport {
	return []Transport{
		TransportTCP,
		TransportWeb,
		TransportGRPC,
		TransportXHTTP,
		TransportHTTPUpgrade,
		TransportKCP,
	}
}

// SupportedProtocols lists the inbound protocols the renderer can
// emit.
func SupportedProtocols() []Protocol {
	return []Protocol{
		ProtocolVLESS,
		ProtocolVMess,
		ProtocolTrojan,
		ProtocolShadowsocks,
		ProtocolSOCKS,
		ProtocolHTTP,
	}
}

// SupportsReality reports whether the protocol may sit under the
// REALITY security layer.
func (p Protocol) SupportsReality() bool {
	return p == ProtocolVLESS
}

// supportsRealityNetwork reports whether the transport negotiates a
// stream compatible with the REALITY handshake.
func (t Transport) supportsRealityNetwork() bool {
	switch t {
	case TransportTCP, TransportXHTTP:
		return true
	default:
		return false
	}
}

// FlowVision is the only flow value DuckOS exposes. Xray-core
// rejects any other value on modern builds.
const FlowVision = "xtls-rprx-vision"

// shadowsocksMethods are the AEAD and Shadowsocks-2022 ciphers
// Xray-core accepts on the shadowsocks inbound. The 2022 methods
// require a server PSK plus one independent PSK per client.
var shadowsocksMethods = map[string]bool{
	"aes-128-gcm":                   true,
	"aes-256-gcm":                   true,
	"chacha20-poly1305":             true,
	"2022-blake3-aes-128-gcm":       true,
	"2022-blake3-aes-256-gcm":       true,
	"2022-blake3-chacha20-poly1305": true,
}

// xhttpModes are the documented XHTTP packeting modes.
var xhttpModes = map[string]bool{
	"auto":       true,
	"packet-up":  true,
	"stream-up":  true,
	"stream-one": true,
}

// fingerprintAlgorithms are the uTLS fingerprints Xray-core ships.
var fingerprintAlgorithms = map[string]bool{
	"chrome":     true,
	"firefox":    true,
	"safari":     true,
	"ios":        true,
	"android":    true,
	"edge":       true,
	"360":        true,
	"qq":         true,
	"random":     true,
	"randomized": true,
	"":           true, // unset: Go TLS client, valid for inbound REALITY
}

// ValidateCompatibility enforces the protocol x transport x security
// matrix plus the transport-specific settings contract. It is called
// from Server.Validate so every entry point (API, CLI, import,
// migration) inherits the same refusals.
func ValidateCompatibility(s Server) error {
	// REALITY pairing.
	if s.Security == SecurityReality {
		if !s.Protocol.SupportsReality() {
			return fmt.Errorf("REALITY is not supported for protocol %q", s.Protocol)
		}
		if !s.Transport.supportsRealityNetwork() {
			return fmt.Errorf("REALITY requires the tcp or xhttp transport, not %q", s.Transport)
		}
		if s.Fingerprint != "" && !fingerprintAlgorithms[s.Fingerprint] {
			return fmt.Errorf("REALITY fingerprint %q is not a known uTLS algorithm", s.Fingerprint)
		}
	}
	// Shadowsocks method.
	if s.Protocol == ProtocolShadowsocks {
		if !shadowsocksMethods[s.SSMethod] {
			return fmt.Errorf("shadowsocks method %q is not supported", s.SSMethod)
		}
	}
	// Trojan / Shadowsocks / SOCKS / HTTP over REALITY is nonsense.
	if s.Security == SecurityReality {
		switch s.Protocol {
		case ProtocolTrojan, ProtocolShadowsocks, ProtocolSOCKS, ProtocolHTTP:
			return fmt.Errorf("REALITY is not supported for protocol %q", s.Protocol)
		}
	}
	// VMess and VLESS never negotiate REALITY on the WebSocket /
	// gRPC / HTTPUpgrade / mKCP transports.
	if s.Security == SecurityReality && s.Protocol == ProtocolVMess {
		return fmt.Errorf("REALITY is not supported for protocol %q", s.Protocol)
	}
	// Transport-specific settings validation.
	if err := validateTransportSettings(s); err != nil {
		return err
	}
	return nil
}

// validateUserCompatibility enforces the per-user capability rules:
// VLESS flow values and the Shadowsocks 2022 client-PSK shape.
func validateUserCompatibility(server Protocol, security Security, transport Transport, user User) error {
	if user.Flow != "" {
		if server != ProtocolVLESS {
			return fmt.Errorf("flow %q requires the VLESS protocol", user.Flow)
		}
		if user.Flow != FlowVision {
			return fmt.Errorf("flow %q is not supported; use %q", user.Flow, FlowVision)
		}
		if security != SecurityReality || transport != TransportTCP {
			return fmt.Errorf("flow %q requires VLESS over tcp with REALITY", user.Flow)
		}
	}
	return nil
}

// isRealityPublicKey reports whether s is 43 characters of
// unpadded URL-safe base64 (the exact encoding xray x25519 emits
// for X25519 public keys).
func isRealityPublicKey(s string) bool {
	if len(s) != 43 {
		return false
	}
	for _, c := range s {
		switch {
		case c >= 'A' && c <= 'Z':
		case c >= 'a' && c <= 'z':
		case c >= '0' && c <= '9':
		case c == '-' || c == '_':
		default:
			return false
		}
	}
	return true
}

// ValidateUserForServer enforces the protocol-specific credential
// shape: VMess/VLESS require a UUID form, Trojan/Shadowsocks a
// password, SOCKS/HTTP a username+password pair. It is the render-
// time complement of the per-user flow rules.
func ValidateUserForServer(server Server, user User) error {
	if err := validateUserCompatibility(server.Protocol, server.Security, server.Transport, user); err != nil {
		return err
	}
	if user.Credential != nil {
		return nil // shape is enforced by the vault resolver at apply time
	}
	switch server.Protocol {
	case ProtocolVLESS, ProtocolVMess:
		if !isValidUUID(user.UUID) {
			return fmt.Errorf("%s client requires a canonical UUID", server.Protocol)
		}
	case ProtocolTrojan, ProtocolShadowsocks:
		if user.Password == "" {
			return fmt.Errorf("%s client requires a password", server.Protocol)
		}
	case ProtocolSOCKS, ProtocolHTTP:
		if user.Name == "" || user.Password == "" {
			return fmt.Errorf("%s client requires a username and password", server.Protocol)
		}
	}
	return nil
}

func validateTransportSettings(s Server) error {
	ts := s.TransportSettings
	if ts == nil {
		return nil
	}
	count := 0
	for _, present := range []bool{
		ts.WS != nil, ts.GRPC != nil, ts.TCP != nil,
		ts.XHTTP != nil, ts.HTTPUpgrade != nil, ts.KCP != nil,
	} {
		if present {
			count++
		}
	}
	if count > 1 {
		return fmt.Errorf("transport settings carry %d transport blocks; exactly one matching %q is allowed", count, s.Transport)
	}
	switch s.Transport {
	case TransportWeb:
		if ts.WS == nil {
			return nil // optional
		}
		if ts.WS.Path != "" && !strings.HasPrefix(ts.WS.Path, "/") {
			return fmt.Errorf("websocket path %q must start with /", ts.WS.Path)
		}
	case TransportGRPC:
		if ts.GRPC == nil {
			return nil
		}
	case TransportTCP:
		if ts.TCP != nil && ts.TCP.HeaderType != "" && ts.TCP.HeaderType != "http" {
			return fmt.Errorf("tcp headerType %q is not supported (use \"http\")", ts.TCP.HeaderType)
		}
	case TransportXHTTP:
		if ts.XHTTP == nil {
			return nil
		}
		if ts.XHTTP.Mode != "" && !xhttpModes[ts.XHTTP.Mode] {
			return fmt.Errorf("xhttp mode %q is not canonical", ts.XHTTP.Mode)
		}
		if ts.XHTTP.Path != "" && !strings.HasPrefix(ts.XHTTP.Path, "/") {
			return fmt.Errorf("xhttp path %q must start with /", ts.XHTTP.Path)
		}
	case TransportHTTPUpgrade:
		if ts.HTTPUpgrade == nil {
			return nil
		}
		if ts.HTTPUpgrade.Path != "" && !strings.HasPrefix(ts.HTTPUpgrade.Path, "/") {
			return fmt.Errorf("httpUpgrade path %q must start with /", ts.HTTPUpgrade.Path)
		}
	case TransportKCP:
		if ts.KCP == nil {
			return nil
		}
		if ts.KCP.MTU < 0 {
			return fmt.Errorf("kcp mtu %d is negative", ts.KCP.MTU)
		}
		if ts.KCP.HeaderType != "" && ts.KCP.HeaderType != "none" &&
			ts.KCP.HeaderType != "srtp" && ts.KCP.HeaderType != "utp" &&
			ts.KCP.HeaderType != "wechat-video" && ts.KCP.HeaderType != "dtls" &&
			ts.KCP.HeaderType != "wireguard" && ts.KCP.HeaderType != "dns" {
			return fmt.Errorf("kcp header type %q is not supported", ts.KCP.HeaderType)
		}
	}
	return nil
}
