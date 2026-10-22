// Package xray / sharelink.go: client-consumable share links and
// strict connection-URI import. The builder produces resolved
// links: the caller supplies the vault-resolved credential bytes
// and the REALITY public parameters, and the result is directly
// importable by standard Xray clients (v2rayN, v2rayNG, Streisand,
// NekoBox, sing-box). The canonical catalog keeps holding vault
// references only; resolution happens in the delivery path.
package xray

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// LinkParams carries the resolved, non-secret parameters a share
// link needs beyond the credential itself. Secret bytes are passed
// separately and never logged.
type LinkParams struct {
	// Credential is the resolved client secret: a UUID for
	// VLESS/VMess, a password for Trojan/Shadowsocks.
	Credential string
	// RealityPublicKey and ShortID are required for REALITY
	// inbounds (the client needs pbk + sid).
	RealityPublicKey string
	ShortID          string
	// SNI / Fingerprint default from the server when empty.
	SNI         string
	Fingerprint string
}

// BuildResolvedShareLink returns the client URI for (server, user).
// Supported: vless, trojan, vmess, shadowsocks. The link is bounded
// to MaxShareLinkBytes. SOCKS/HTTP clients have no standard share
// URI and are rejected.
func BuildResolvedShareLink(server Server, user User, p LinkParams) (string, error) {
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
	hostPort := joinHostPortLink(host, port)
	network := string(server.Transport)
	if network == "tcp" && server.TransportSettings != nil && server.TransportSettings.TCP != nil && server.TransportSettings.TCP.HeaderType == "http" {
		network = "tcp" // header obfuscation stays inside the tcp family
	}
	q := url.Values{}
	q.Set("type", network)
	switch server.Transport {
	case TransportWeb:
		path := "/" + server.Name
		hostHeader := ""
		if ts := server.TransportSettings; ts != nil && ts.WS != nil {
			if ts.WS.Path != "" {
				path = ts.WS.Path
			}
			hostHeader = ts.WS.Host
		}
		q.Set("path", path)
		if hostHeader != "" {
			q.Set("host", hostHeader)
		}
	case TransportGRPC:
		serviceName := "duckos-" + server.Name
		if ts := server.TransportSettings; ts != nil && ts.GRPC != nil && ts.GRPC.ServiceName != "" {
			serviceName = ts.GRPC.ServiceName
		}
		q.Set("serviceName", serviceName)
		if ts := server.TransportSettings; ts != nil && ts.GRPC != nil && ts.GRPC.MultiMode {
			q.Set("mode", "multi")
		}
	case TransportXHTTP:
		path, hostHeader, mode := "/"+server.Name, "", ""
		if ts := server.TransportSettings; ts != nil && ts.XHTTP != nil {
			if ts.XHTTP.Path != "" {
				path = ts.XHTTP.Path
			}
			hostHeader = ts.XHTTP.Host
			mode = ts.XHTTP.Mode
		}
		q.Set("path", path)
		if hostHeader != "" {
			q.Set("host", hostHeader)
		}
		if mode != "" {
			q.Set("mode", mode)
		}
	case TransportHTTPUpgrade:
		path, hostHeader := "/"+server.Name, ""
		if ts := server.TransportSettings; ts != nil && ts.HTTPUpgrade != nil {
			if ts.HTTPUpgrade.Path != "" {
				path = ts.HTTPUpgrade.Path
			}
			hostHeader = ts.HTTPUpgrade.Host
		}
		q.Set("path", path)
		if hostHeader != "" {
			q.Set("host", hostHeader)
		}
	case TransportKCP:
		headerType := ""
		if ts := server.TransportSettings; ts != nil && ts.KCP != nil {
			headerType = ts.KCP.HeaderType
		}
		if headerType != "" {
			q.Set("headerType", headerType)
		}
	}
	switch server.Security {
	case SecurityTLS:
		q.Set("security", "tls")
		sni := p.SNI
		if sni == "" && len(server.RealitySNI) > 0 {
			sni = server.RealitySNI[0]
		}
		if sni != "" {
			q.Set("sni", sni)
		}
		fp := p.Fingerprint
		if fp == "" {
			fp = server.Fingerprint
		}
		if fp != "" {
			q.Set("fp", fp)
		}
	case SecurityReality:
		if p.RealityPublicKey == "" || p.ShortID == "" {
			return "", errors.New("REALITY share link requires the resolved public key and short id")
		}
		q.Set("security", "reality")
		q.Set("pbk", p.RealityPublicKey)
		q.Set("sid", p.ShortID)
		sni := p.SNI
		if sni == "" && len(server.RealitySNI) > 0 {
			sni = server.RealitySNI[0]
		}
		if sni == "" {
			return "", errors.New("REALITY share link requires an SNI")
		}
		q.Set("sni", sni)
		fp := p.Fingerprint
		if fp == "" {
			fp = server.Fingerprint
		}
		if fp == "" {
			fp = "chrome"
		}
		q.Set("fp", fp)
	default:
		q.Set("security", "none")
	}
	label := user.Name
	if user.Email != "" {
		label = user.Email
	}
	var link string
	switch server.Protocol {
	case ProtocolVLESS:
		if user.Flow != "" {
			q.Set("flow", user.Flow)
		}
		q.Set("encryption", "none")
		link = fmt.Sprintf("vless://%s@%s?%s#%s", url.PathEscape(p.Credential), hostPort, q.Encode(), url.PathEscape(label))
	case ProtocolTrojan:
		link = fmt.Sprintf("trojan://%s@%s?%s#%s", url.PathEscape(p.Credential), hostPort, q.Encode(), url.PathEscape(label))
	case ProtocolVMess:
		link, err = buildVMessLink(server, user, hostPort, p, label)
		if err != nil {
			return "", err
		}
	case ProtocolShadowsocks:
		userinfo := base64.RawURLEncoding.EncodeToString([]byte(server.SSMethod + ":" + p.Credential))
		link = fmt.Sprintf("ss://%s@%s#%s", userinfo, hostPort, url.PathEscape(label))
		// Shadowsocks clients ignore vless-style query params.
		return boundLink(link)
	default:
		return "", fmt.Errorf("xray: no share URI for protocol %q", server.Protocol)
	}
	return boundLink(link)
}

func buildVMessLink(server Server, user User, hostPort string, p LinkParams, label string) (string, error) {
	// vmess:// carries a base64 JSON document (v2rayN convention).
	// The credential is the UUID; security is fixed to auto.
	network := string(server.Transport)
	path, hostHeader := "", ""
	switch server.Transport {
	case TransportWeb:
		path, hostHeader = "/"+server.Name, ""
		if ts := server.TransportSettings; ts != nil && ts.WS != nil {
			if ts.WS.Path != "" {
				path = ts.WS.Path
			}
			hostHeader = ts.WS.Host
		}
	case TransportGRPC:
		path = "duckos-" + server.Name
		if ts := server.TransportSettings; ts != nil && ts.GRPC != nil && ts.GRPC.ServiceName != "" {
			path = ts.GRPC.ServiceName
		}
	case TransportXHTTP:
		path, hostHeader = "/"+server.Name, ""
		if ts := server.TransportSettings; ts != nil && ts.XHTTP != nil {
			if ts.XHTTP.Path != "" {
				path = ts.XHTTP.Path
			}
			hostHeader = ts.XHTTP.Host
		}
	case TransportHTTPUpgrade:
		path, hostHeader = "/"+server.Name, ""
		if ts := server.TransportSettings; ts != nil && ts.HTTPUpgrade != nil {
			if ts.HTTPUpgrade.Path != "" {
				path = ts.HTTPUpgrade.Path
			}
			hostHeader = ts.HTTPUpgrade.Host
		}
	}
	tls := ""
	sni, fp := p.SNI, p.Fingerprint
	switch server.Security {
	case SecurityTLS:
		tls = "tls"
	case SecurityReality:
		tls = "reality"
	default:
		sni, fp = "", ""
	}
	if sni == "" && len(server.RealitySNI) > 0 {
		sni = server.RealitySNI[0]
	}
	if fp == "" {
		fp = server.Fingerprint
	}
	headerType := ""
	if ts := server.TransportSettings; ts != nil && ts.TCP != nil {
		headerType = ts.TCP.HeaderType
	}
	doc := map[string]any{
		"v":    "2",
		"ps":   label,
		"add":  hostOnly(hostPort),
		"port": portOf(hostPort),
		"id":   p.Credential,
		"aid":  "0",
		"scy":  "auto",
		"net":  network,
		"type": headerType,
		"host": hostHeader,
		"path": path,
		"tls":  tls,
		"sni":  sni,
		"fp":   fp,
	}
	encoded, err := json.Marshal(doc)
	if err != nil {
		return "", err
	}
	return "vmess://" + base64.StdEncoding.EncodeToString(encoded), nil
}

func boundLink(link string) (string, error) {
	if len(link) > MaxShareLinkBytes {
		return "", errors.New("xray share link exceeds size limit")
	}
	return link, nil
}

// joinHostPortLink brackets IPv6 literals for URI use.
func joinHostPortLink(host string, port int) string {
	if strings.Contains(host, ":") {
		return "[" + host + "]:" + strconv.Itoa(port)
	}
	return host + ":" + strconv.Itoa(port)
}

func hostOnly(hostPort string) string {
	if i := strings.LastIndex(hostPort, ":"); i >= 0 && strings.Contains(hostPort, "]") {
		return strings.Trim(hostPort[:i], "[]")
	}
	if i := strings.LastIndex(hostPort, ":"); i >= 0 {
		return hostPort[:i]
	}
	return hostPort
}

func portOf(hostPort string) int {
	if i := strings.LastIndex(hostPort, ":"); i >= 0 {
		p, _ := strconv.Atoi(hostPort[i+1:])
		return p
	}
	return 0
}

// ParsedConnection is a strictly-validated inbound connection URI
// ready to be mapped onto (Server, User) pairs by the importer.
type ParsedConnection struct {
	Protocol    Protocol
	Name        string // label fragment
	Host        string
	Port        int
	Credential  string // UUID or password
	Method      string // shadowsocks cipher
	Transport   Transport
	Security    Security
	Flow        string
	SNI         string
	Fingerprint string
	PublicKey   string // REALITY pbk
	ShortID     string // REALITY sid
	Path        string
	HostHeader  string
	ServiceName string
	Mode        string // xhttp mode
	HeaderType  string
}

// ParseConnectionURI strictly parses a vless://, trojan://, vmess://
// or ss:// URI. Unknown query parameters are preserved in UnknownQuery
// so an import never silently drops settings.
type ParsedConnectionWithUnknown struct {
	ParsedConnection
	UnknownQuery map[string]string
}

// ParseConnectionURI parses and validates the URI. Unsupported
// protocol/transport/security combinations are explicit errors, not
// best-effort interpretations.
func ParseConnectionURI(raw string) (ParsedConnectionWithUnknown, error) {
	raw = strings.TrimSpace(raw)
	var out ParsedConnectionWithUnknown
	switch {
	case strings.HasPrefix(raw, "vmess://"):
		return parseVMessURI(raw)
	case strings.HasPrefix(raw, "ss://"):
		return parseSSURI(raw)
	case strings.HasPrefix(raw, "vless://"), strings.HasPrefix(raw, "trojan://"):
		return parseStandardURI(raw)
	default:
		return out, errors.New("xray: unsupported connection scheme (want vless, vmess, trojan or ss)")
	}
}

func parseStandardURI(raw string) (ParsedConnectionWithUnknown, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return ParsedConnectionWithUnknown{}, fmt.Errorf("xray: connection uri: %w", err)
	}
	out := ParsedConnectionWithUnknown{}
	out.Protocol = Protocol(strings.TrimSuffix(u.Scheme, ":"))
	out.Credential = u.User.Username()
	if out.Credential == "" {
		return out, errors.New("xray: connection uri carries no credential")
	}
	host := u.Hostname()
	port, err := strconv.Atoi(u.Port())
	if err != nil || host == "" || port < 1 || port > 65535 {
		return out, errors.New("xray: connection uri host:port is not parseable")
	}
	out.Host, out.Port = host, port
	if frag := u.Fragment; frag != "" {
		out.Name = frag
	}
	q := u.Query()
	out.UnknownQuery = map[string]string{}
	for k, v := range q {
		if len(v) == 0 {
			continue
		}
		switch k {
		case "type":
			t := Transport(v[0])
			switch t {
			case TransportTCP, TransportWeb, TransportGRPC, TransportXHTTP, TransportHTTPUpgrade, TransportKCP:
				out.Transport = t
			case "raw":
				out.Transport = TransportTCP
			default:
				return out, fmt.Errorf("xray: unsupported transport %q", v[0])
			}
		case "security":
			switch s := Security(v[0]); s {
			case SecurityNone, SecurityTLS, SecurityReality:
				out.Security = s
			default:
				return out, fmt.Errorf("xray: unsupported security %q", v[0])
			}
		case "flow":
			if v[0] != FlowVision {
				return out, fmt.Errorf("xray: unsupported flow %q", v[0])
			}
			out.Flow = v[0]
		case "sni", "peer":
			out.SNI = v[0]
		case "fp":
			out.Fingerprint = v[0]
		case "pbk":
			out.PublicKey = v[0]
		case "sid":
			out.ShortID = v[0]
		case "path":
			out.Path = v[0]
		case "host":
			out.HostHeader = v[0]
		case "serviceName":
			out.ServiceName = v[0]
		case "mode":
			out.Mode = v[0]
		case "headerType":
			out.HeaderType = v[0]
		default:
			out.UnknownQuery[k] = v[0]
		}
	}
	if out.Transport == "" {
		out.Transport = TransportTCP
	}
	if out.Security == "" {
		out.Security = SecurityNone
	}
	return out, nil
}

func parseVMessURI(raw string) (ParsedConnectionWithUnknown, error) {
	payload, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(raw, "vmess://"))
	if err != nil {
		payload, err = base64.RawStdEncoding.DecodeString(strings.TrimPrefix(raw, "vmess://"))
	}
	if err != nil {
		payload, err = base64.URLEncoding.DecodeString(strings.TrimPrefix(raw, "vmess://"))
	}
	if err != nil {
		return ParsedConnectionWithUnknown{}, fmt.Errorf("xray: vmess uri payload is not base64: %w", err)
	}
	var doc struct {
		Add  string      `json:"add"`
		Port json.Number `json:"port"`
		ID   string      `json:"id"`
		Ps   string      `json:"ps"`
		Net  string      `json:"net"`
		TLS  string      `json:"tls"`
		SNI  string      `json:"sni"`
		Fp   string      `json:"fp"`
		Host string      `json:"host"`
		Path string      `json:"path"`
		Type string      `json:"type"`
		Scy  string      `json:"scy"`
	}
	if err := json.Unmarshal(payload, &doc); err != nil {
		return ParsedConnectionWithUnknown{}, fmt.Errorf("xray: vmess uri payload is not a v2 JSON document: %w", err)
	}
	if doc.Scy != "" && doc.Scy != "auto" {
		return ParsedConnectionWithUnknown{}, fmt.Errorf("xray: vmess encryption %q is not supported (auto only)", doc.Scy)
	}
	port, err := strconv.Atoi(doc.Port.String())
	if err != nil || doc.Add == "" || port < 1 || port > 65535 || doc.ID == "" {
		return ParsedConnectionWithUnknown{}, errors.New("xray: vmess uri add/port/id are missing or invalid")
	}
	out := ParsedConnectionWithUnknown{}
	out.Protocol = ProtocolVMess
	out.Host, out.Port, out.Credential, out.Name = doc.Add, port, doc.ID, doc.Ps
	switch Transport(doc.Net) {
	case TransportTCP, TransportWeb, TransportGRPC, TransportXHTTP, TransportHTTPUpgrade, TransportKCP:
		out.Transport = Transport(doc.Net)
	default:
		return out, fmt.Errorf("xray: unsupported vmess transport %q", doc.Net)
	}
	switch Security(doc.TLS) {
	case "", SecurityNone:
		out.Security = SecurityNone
	case SecurityTLS, SecurityReality:
		out.Security = Security(doc.TLS)
	default:
		return out, fmt.Errorf("xray: unsupported vmess security %q", doc.TLS)
	}
	out.SNI, out.Fingerprint = doc.SNI, doc.Fp
	out.HostHeader, out.Path, out.HeaderType = doc.Host, doc.Path, doc.Type
	if out.Security == SecurityReality && out.Transport != TransportTCP && out.Transport != TransportXHTTP {
		return out, errors.New("xray: REALITY requires the tcp or xhttp transport")
	}
	return out, nil
}

func parseSSURI(raw string) (ParsedConnectionWithUnknown, error) {
	rest := strings.TrimPrefix(raw, "ss://")
	// SIP002: ss://base64(method:password)@host:port#label or
	// ss://method:password@host:port#label (plaintext userinfo).
	frag := ""
	if i := strings.Index(rest, "#"); i >= 0 {
		frag, rest = rest[i+1:], rest[:i]
	}
	var userinfo, hostport string
	if i := strings.Index(rest, "@"); i >= 0 {
		userinfo, hostport = rest[:i], rest[i+1:]
	} else {
		// Legacy full-base64 form: ss://base64(method:password@host:port)
		decoded, err := base64.StdEncoding.DecodeString(rest)
		if err != nil {
			decoded, err = base64.URLEncoding.DecodeString(rest)
		}
		if err != nil {
			return ParsedConnectionWithUnknown{}, errors.New("xray: ss uri is not SIP002 and does not decode")
		}
		rest = string(decoded)
		if i := strings.LastIndex(rest, "@"); i < 0 {
			return ParsedConnectionWithUnknown{}, errors.New("xray: ss uri has no host")
		} else {
			userinfo, hostport = rest[:i], rest[i+1:]
		}
	}
	method, password, found := strings.Cut(userinfo, ":")
	if !found {
		// base64url(method:password)
		decoded, err := base64.RawURLEncoding.DecodeString(userinfo)
		if err != nil {
			decoded, err = base64.StdEncoding.DecodeString(userinfo)
		}
		if err != nil {
			return ParsedConnectionWithUnknown{}, errors.New("xray: ss userinfo is not method:password")
		}
		method, password, found = strings.Cut(string(decoded), ":")
		if !found {
			return ParsedConnectionWithUnknown{}, errors.New("xray: ss decoded userinfo is not method:password")
		}
	}
	if !shadowsocksMethods[method] {
		return ParsedConnectionWithUnknown{}, fmt.Errorf("xray: ss method %q is not supported", method)
	}
	host, port, err := splitHostPort(hostport)
	if err != nil {
		hostport = strings.Trim(hostport, "[]")
		host, port, err = splitHostPort(hostport)
		if err != nil {
			return ParsedConnectionWithUnknown{}, fmt.Errorf("xray: ss uri host:port: %w", err)
		}
	}
	if host == "" || port < 1 || port > 65535 {
		return ParsedConnectionWithUnknown{}, errors.New("xray: ss uri host:port is not parseable")
	}
	out := ParsedConnectionWithUnknown{}
	out.Protocol = ProtocolShadowsocks
	out.Method, out.Credential = method, password
	out.Host, out.Port = host, port
	out.Name = frag
	out.Transport, out.Security = TransportTCP, SecurityNone
	return out, nil
}
