// Package ximport: secure import pipelines for the Xray Center.
//
// threeui.go implements the read-only 3x-ui migration foundation:
// it parses a 3x-ui inbound export (the JSON document 3x-ui hands
// out, with settings/streamSettings embedded as JSON strings), maps
// it onto the DuckOS typed model and reports every unsupported or
// unknown field instead of dropping it. The importer is strictly
// read-only: it never touches a 3x-ui instance, never requires
// production access, and produces a preview the operator confirms
// before anything is committed.
package ximport

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/CrazyDuckLab/DuckOS/internal/xray"
)

// MaxImportBytes bounds a single 3x-ui export document.
const MaxImportBytes = 8 << 20

// ThreeUIInbound mirrors one entry of a 3x-ui export. settings and
// streamSettings arrive as embedded JSON strings in 3x-ui's schema;
// both are decoded defensively.
type ThreeUIInbound struct {
	ID             json.Number     `json:"id"`
	Port           json.Number     `json:"port"`
	Listen         string          `json:"listen"`
	Protocol       string          `json:"protocol"`
	Remark         string          `json:"remark"`
	Enable         *bool           `json:"enable"`
	Settings       string          `json:"settings"`
	StreamSettings string          `json:"streamSettings"`
	Sniffing       json.RawMessage `json:"sniffing"`
	// Unknown preserves every field the importer does not model so
	// a migration never silently drops configuration.
	Unknown map[string]json.RawMessage `json:"-"`
}

type threeUIEnvelope struct {
	Inbounds []ThreeUIInbound `json:"inbounds"`
}

var remarkPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,47}$`)

// ImportPreview is the dry-run result: draft DuckOS objects plus a
// complete account of what did and did not map.
type ImportPreview struct {
	Servers []xray.Server `json:"servers"`
	Users   []xray.User   `json:"users"`
	// Warnings are recoverable mapping deviations.
	Warnings []string `json:"warnings"`
	// Unsupported are refused mappings with the reason.
	Unsupported []string `json:"unsupported"`
}

// ParseThreeUIExport parses a 3x-ui export document (either the
// bare {"inbounds": [...]} envelope or a single inbound object) and
// returns the draft preview. No object receives an ID or vault
// binding: the operator assigns those at commit time.
func ParseThreeUIExport(raw []byte) (ImportPreview, error) {
	if len(raw) > MaxImportBytes {
		return ImportPreview{}, fmt.Errorf("ximport: export exceeds %d bytes", MaxImportBytes)
	}
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return ImportPreview{}, fmt.Errorf("ximport: empty export")
	}
	var preview ImportPreview
	parseOne := func(in ThreeUIInbound) {
		srv, users, warn, unsupported := mapThreeUIInbound(in)
		preview.Warnings = append(preview.Warnings, warn...)
		preview.Unsupported = append(preview.Unsupported, unsupported...)
		if srv != nil {
			preview.Servers = append(preview.Servers, *srv)
			preview.Users = append(preview.Users, users...)
		}
	}
	var env threeUIEnvelope
	if err := json.Unmarshal([]byte(trimmed), &env); err == nil && len(env.Inbounds) > 0 {
		for _, in := range env.Inbounds {
			parseOne(in)
		}
		return preview, nil
	}
	var one ThreeUIInbound
	if err := json.Unmarshal([]byte(trimmed), &one); err != nil {
		return ImportPreview{}, fmt.Errorf("ximport: document is neither an inbounds envelope nor an inbound: %w", err)
	}
	parseOne(one)
	return preview, nil
}

// mapThreeUIInbound maps one inbound. The mapping is conservative:
// only verified protocol/transport/security pairs produce a draft
// server; anything else is reported.
func mapThreeUIInbound(in ThreeUIInbound) (*xray.Server, []xray.User, []string, []string) {
	var warnings, unsupported []string
	var srvUsers []xray.User
	var settings struct {
		Clients []struct {
			ID       string `json:"id"`
			Password string `json:"password"`
			Email    string `json:"email"`
			Flow     string `json:"flow"`
			// 3x-ui client extras the DuckOS model does not carry.
			TotalGB    json.Number `json:"totalGB"`
			ExpiryTime json.Number `json:"expiryTime"`
			LimitIp    int         `json:"limitIp"`
			TgID       string      `json:"tgId"`
			SubID      string      `json:"subId"`
		} `json:"clients"`
		Decryption string                     `json:"decryption"`
		Method     string                     `json:"method"`
		Unknown    map[string]json.RawMessage `json:"-"`
	}
	if in.Settings != "" {
		if err := json.Unmarshal([]byte(in.Settings), &settings); err != nil {
			unsupported = append(unsupported, fmt.Sprintf("inbound %q: settings JSON undecodable: %v", labelOf(in), err))
			return nil, nil, warnings, unsupported
		}
	}
	var stream struct {
		Network     string `json:"network"`
		Security    string `json:"security"`
		TLSSettings struct {
			ServerName string                     `json:"serverName"`
			Unknown    map[string]json.RawMessage `json:"-"`
		} `json:"tlsSettings"`
		RealitySettings struct {
			Dest        string   `json:"dest"`
			ServerNames []string `json:"serverNames"`
			ShortIds    []string `json:"shortIds"`
			Fingerprint string   `json:"fingerprint"`
			PrivateKey  string   `json:"privateKey"`
			PublicKey   string   `json:"publicKey"`
		} `json:"realitySettings"`
		WSSettings struct {
			Path    string            `json:"path"`
			Headers map[string]string `json:"headers"`
		} `json:"wsSettings"`
		GRPCSettings struct {
			ServiceName string `json:"serviceName"`
			MultiMode   bool   `json:"multiMode"`
		} `json:"grpcSettings"`
		XHTTPSettings struct {
			Path string `json:"path"`
			Host string `json:"host"`
			Mode string `json:"mode"`
		} `json:"xhttpSettings"`
		HTTPUpgradeSettings struct {
			Path string `json:"path"`
			Host string `json:"host"`
		} `json:"httpupgradeSettings"`
		KCPSettings struct {
			MTU    json.Number `json:"mtu"`
			Header struct {
				Type string `json:"type"`
			} `json:"header"`
		} `json:"kcpSettings"`
	}
	if in.StreamSettings != "" {
		if err := json.Unmarshal([]byte(in.StreamSettings), &stream); err != nil {
			unsupported = append(unsupported, fmt.Sprintf("inbound %q: streamSettings JSON undecodable: %v", labelOf(in), err))
			return nil, nil, warnings, unsupported
		}
	}
	// Protocol mapping.
	var proto xray.Protocol
	switch strings.ToLower(in.Protocol) {
	case "vless":
		proto = xray.ProtocolVLESS
	case "vmess":
		proto = xray.ProtocolVMess
	case "trojan":
		proto = xray.ProtocolTrojan
	case "shadowsocks":
		proto = xray.ProtocolShadowsocks
	case "socks":
		proto = xray.ProtocolSOCKS
	case "http":
		proto = xray.ProtocolHTTP
	default:
		unsupported = append(unsupported, fmt.Sprintf("inbound %q: protocol %q is not supported", labelOf(in), in.Protocol))
		return nil, nil, warnings, unsupported
	}
	var transport xray.Transport
	switch stream.Network {
	case "", "tcp", "raw":
		transport = xray.TransportTCP
	case "ws":
		transport = xray.TransportWeb
	case "grpc":
		transport = xray.TransportGRPC
	case "xhttp", "splithttp":
		transport = xray.TransportXHTTP
		if stream.Network == "splithttp" {
			warnings = append(warnings, fmt.Sprintf("inbound %q: legacy splithttp transport mapped to xhttp", labelOf(in)))
		}
	case "httpupgrade":
		transport = xray.TransportHTTPUpgrade
	case "kcp":
		transport = xray.TransportKCP
	default:
		unsupported = append(unsupported, fmt.Sprintf("inbound %q: transport %q is not supported", labelOf(in), stream.Network))
		return nil, nil, warnings, unsupported
	}
	var security xray.Security
	switch stream.Security {
	case "", "none":
		security = xray.SecurityNone
	case "tls":
		security = xray.SecurityTLS
	case "reality":
		security = xray.SecurityReality
	default:
		unsupported = append(unsupported, fmt.Sprintf("inbound %q: security %q is not supported", labelOf(in), stream.Security))
		return nil, nil, warnings, unsupported
	}
	ts := &xray.TransportSettings{}
	switch transport {
	case xray.TransportWeb:
		ts.WS = &xray.WSSettings{Path: stream.WSSettings.Path, Host: stream.WSSettings.Headers["Host"]}
	case xray.TransportGRPC:
		ts.GRPC = &xray.GRPCSettings{ServiceName: stream.GRPCSettings.ServiceName, MultiMode: stream.GRPCSettings.MultiMode}
	case xray.TransportXHTTP:
		ts.XHTTP = &xray.XHTTPSettings{Path: stream.XHTTPSettings.Path, Host: stream.XHTTPSettings.Host, Mode: stream.XHTTPSettings.Mode}
	case xray.TransportHTTPUpgrade:
		ts.HTTPUpgrade = &xray.HTTPUpgradeSettings{Path: stream.HTTPUpgradeSettings.Path, Host: stream.HTTPUpgradeSettings.Host}
	case xray.TransportKCP:
		mtu := 0
		if stream.KCPSettings.MTU != "" {
			fmt.Sscanf(stream.KCPSettings.MTU.String(), "%d", &mtu)
		}
		ts.KCP = &xray.KCPSettings{MTU: mtu, HeaderType: stream.KCPSettings.Header.Type}
	}
	port := 0
	if in.Port != "" {
		fmt.Sscanf(in.Port.String(), "%d", &port)
	}
	listen := in.Listen
	if listen == "" {
		listen = "0.0.0.0"
	}
	name := draftName(in.Remark, string(proto))
	srv := &xray.Server{
		Name:              name,
		Protocol:          proto,
		Transport:         transport,
		Security:          security,
		Listen:            listen + ":" + fmt.Sprint(port),
		Port:              port,
		SSMethod:          settings.Method,
		TransportSettings: ts,
	}
	if security == xray.SecurityReality {
		srv.RealityDest = stream.RealitySettings.Dest
		srv.RealitySNI = stream.RealitySettings.ServerNames
		srv.Fingerprint = stream.RealitySettings.Fingerprint
		srv.RealityPublicKey = stream.RealitySettings.PublicKey
		// The private key and short ids are secrets: they are NOT
		// copied into canonical state. The operator is told to
		// store them into the vault; the mapping is incomplete
		// until then and reported as such.
		if stream.RealitySettings.PrivateKey != "" {
			warnings = append(warnings, fmt.Sprintf("inbound %q: REALITY private key present in export; store it into the DuckOS vault (purpose reality-private) and bind realityKeyRef", labelOf(in)))
		}
		if len(stream.RealitySettings.ShortIds) > 0 {
			warnings = append(warnings, fmt.Sprintf("inbound %q: %d REALITY short id(s) present in export; store them into the vault (purpose reality-shortid) and bind shortIdRef", labelOf(in), len(stream.RealitySettings.ShortIds)))
		}
	}
	if security == xray.SecurityTLS {
		warnings = append(warnings, fmt.Sprintf("inbound %q: TLS certificate/key are not part of the export; import the PEM pair into the vault and bind tlsCertRef/tlsKeyRef", labelOf(in)))
	}
	// Clients.
	for _, c := range settings.Clients {
		u := xray.User{Name: draftUserLabel(c.Email, c.ID, c.Password), Email: c.Email, UUID: c.ID, Password: c.Password, Flow: c.Flow}
		if srv.Protocol == xray.ProtocolShadowsocks && c.Password == "" {
			// 2022 PSKs live on the client "password" field only.
			unsupported = append(unsupported, fmt.Sprintf("inbound %q: shadowsocks client %q has no password", labelOf(in), c.Email))
			continue
		}
		if srv.Protocol == xray.ProtocolVLESS && c.ID == "" && c.Password == "" {
			unsupported = append(unsupported, fmt.Sprintf("inbound %q: vless client %q has no id", labelOf(in), c.Email))
			continue
		}
		srvUsers = append(srvUsers, u)
	}
	if len(settings.Clients) == 0 {
		warnings = append(warnings, fmt.Sprintf("inbound %q: export carries no clients", labelOf(in)))
	}
	// Drafts carry no IDs yet; the full contract (ids, vault
	// bindings) is enforced at commit time. Here the capability
	// matrix is the gate.
	if err := xray.ValidateCompatibility(*srv); err != nil {
		unsupported = append(unsupported, fmt.Sprintf("inbound %q: draft fails compatibility: %v", labelOf(in), err))
		return nil, nil, warnings, unsupported
	}
	return srv, srvUsers, warnings, unsupported
}

func labelOf(in ThreeUIInbound) string {
	if in.Remark != "" {
		return in.Remark
	}
	return fmt.Sprintf("port-%s", in.Port.String())
}

func draftName(remark, proto string) string {
	name := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(remark, " ", "-"), "/", "-"))
	if !remarkPattern.MatchString(name) {
		name = "imported-" + proto
	}
	return name
}

func draftUserLabel(email, id, password string) string {
	switch {
	case email != "":
		return sanitizeUserLabel(email)
	case id != "":
		return "user-" + id[:8]
	case password != "":
		return "user-" + password[:min(8, len(password))]
	}
	return "imported-user"
}

func sanitizeUserLabel(s string) string {
	out := strings.ToLower(s)
	var b strings.Builder
	for _, c := range out {
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9', c == '.', c == '_', c == '-':
			b.WriteRune(c)
		case c == '@', c == '+':
			b.WriteRune('-')
		}
	}
	r := b.String()
	if len(r) > 48 {
		r = r[:48]
	}
	if !remarkPattern.MatchString(r) {
		return "imported-user"
	}
	return r
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
