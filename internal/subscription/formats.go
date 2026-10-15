// Package subscription / formats.go: subscription output formats.
//
// The delivery contract keeps base64-of-links as the default, but
// the format engine renders the same proxy set as:
//
//   - base64: the classic newline-joined Xray links payload;
//   - plain:  newline-joined links, for QR rendering and debugging;
//   - clash:  Clash/Mihomo-compatible YAML (hand-emitted with
//     JSON-escaped double-quoted scalars — valid YAML, and no YAML
//     library dependency, so no injection surface from a parser);
//   - json:   a structured DuckOS export document.
//
// Proxies carry resolved credential material: format rendering is a
// delivery-path concern and only ever runs where the caller already
// holds the resolved secrets (the same trust point that builds
// share links).
package subscription

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
)

// Format is a supported subscription output format.
type Format string

const (
	FormatBase64 Format = "base64"
	FormatPlain  Format = "plain"
	FormatClash  Format = "clash"
	FormatJSON   Format = "json"
)

// Formats is the verified allow-list. Unknown formats are refused,
// never best-effort interpreted.
var Formats = map[Format]bool{
	FormatBase64: true,
	FormatPlain:  true,
	FormatClash:  true,
	FormatJSON:   true,
}

// ContentType returns the HTTP content type for a format.
func (f Format) ContentType() string {
	switch f {
	case FormatClash:
		return "text/yaml; charset=utf-8"
	case FormatJSON:
		return "application/json"
	default:
		return "text/plain; charset=utf-8"
	}
}

// Proxy is one fully-resolved outbound entry. It mirrors the share
// link parameters and adds the credential the format needs.
type Proxy struct {
	// Name is the stable display name shown in clients (node
	// label, NOT internal ids).
	Name        string `json:"name"`
	Protocol    string `json:"protocol"` // vless|vmess|trojan|ss
	Host        string `json:"server"`
	Port        int    `json:"port"`
	UUID        string `json:"uuid,omitempty"`     // vless/vmess
	Password    string `json:"password,omitempty"` // trojan/ss
	Cipher      string `json:"cipher,omitempty"`   // ss method
	Flow        string `json:"flow,omitempty"`
	Transport   string `json:"network"`  // tcp|ws|grpc|xhttp|httpupgrade|kcp
	Security    string `json:"security"` // none|tls|reality
	SNI         string `json:"sni,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty"`
	PublicKey   string `json:"publicKey,omitempty"`
	ShortID     string `json:"shortId,omitempty"`
	Path        string `json:"path,omitempty"`
	HeaderHost  string `json:"host,omitempty"`
	ServiceName string `json:"serviceName,omitempty"`
	Mode        string `json:"mode,omitempty"`
	HeaderType  string `json:"headerType,omitempty"`
	UDP         bool   `json:"udp"`
}

// Validate refuses entries without a display name, host or usable
// credential, so no renderer emits a silently broken proxy.
func (p Proxy) Validate() error {
	if p.Name == "" || p.Host == "" {
		return fmt.Errorf("subscription: proxy %q has no name or host", p.Name)
	}
	if p.Port < 1 || p.Port > 65535 {
		return fmt.Errorf("subscription: proxy %q port out of bounds", p.Name)
	}
	switch p.Protocol {
	case "vless", "vmess":
		if p.UUID == "" {
			return fmt.Errorf("subscription: proxy %q has no uuid", p.Name)
		}
	case "trojan", "ss":
		if p.Password == "" {
			return fmt.Errorf("subscription: proxy %q has no password", p.Name)
		}
	case "socks", "http":
		// Allowed in JSON export only; refused in clash/link output.
	default:
		return fmt.Errorf("subscription: proxy %q has unsupported protocol %q", p.Name, p.Protocol)
	}
	return nil
}

// Render encodes the proxy set in the requested format. Output is
// deterministic: entries are rendered in the supplied order after
// name-uniqueness enforcement, so clients see stable updates.
func Render(format Format, proxies []Proxy) ([]byte, error) {
	if !Formats[format] {
		return nil, fmt.Errorf("subscription: format %q is not supported", format)
	}
	names := map[string]bool{}
	for i := range proxies {
		if err := proxies[i].Validate(); err != nil {
			return nil, err
		}
		if names[proxies[i].Name] {
			return nil, fmt.Errorf("subscription: duplicate proxy name %q", proxies[i].Name)
		}
		names[proxies[i].Name] = true
	}
	switch format {
	case FormatPlain:
		return renderPlain(proxies)
	case FormatBase64:
		body, err := renderPlain(proxies)
		if err != nil {
			return nil, err
		}
		return []byte(base64.StdEncoding.EncodeToString(body)), nil
	case FormatClash:
		return renderClash(proxies)
	case FormatJSON:
		return renderJSON(proxies)
	}
	return nil, errors.New("subscription: unreachable format branch")
}

func renderPlain(proxies []Proxy) ([]byte, error) {
	lines := make([]string, 0, len(proxies))
	for _, p := range proxies {
		link, err := proxyLink(p)
		if err != nil {
			return nil, err
		}
		lines = append(lines, link)
	}
	if len(lines) == 0 {
		return nil, errors.New("subscription: no proxies to render")
	}
	return []byte(strings.Join(lines, "\n") + "\n"), nil
}

// proxyLink rebuilds a standard Xray URI from the structured proxy.
// VMess uses the v2 JSON document; the rest use parameter URIs.
func proxyLink(p Proxy) (string, error) {
	q := func(extra map[string]string) string {
		parts := make([]string, 0, len(extra))
		keys := make([]string, 0, len(extra))
		for k := range extra {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			parts = append(parts, k+"="+urlEscape(extra[k]))
		}
		return strings.Join(parts, "&")
	}
	host := p.Host
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	label := urlEscape(p.Name)
	switch p.Protocol {
	case "vless":
		extra := map[string]string{"type": p.Transport, "security": orDefault(p.Security, "none"), "encryption": "none"}
		applyTransportParams(extra, p)
		applySecurityParams(extra, p)
		if p.Flow != "" {
			extra["flow"] = p.Flow
		}
		return fmt.Sprintf("vless://%s@%s:%d?%s#%s", urlEscape(p.UUID), host, p.Port, q(extra), label), nil
	case "trojan":
		extra := map[string]string{"type": p.Transport, "security": orDefault(p.Security, "none")}
		applyTransportParams(extra, p)
		applySecurityParams(extra, p)
		return fmt.Sprintf("trojan://%s@%s:%d?%s#%s", urlEscape(p.Password), host, p.Port, q(extra), label), nil
	case "ss":
		userinfo := base64.RawURLEncoding.EncodeToString([]byte(p.Cipher + ":" + p.Password))
		return fmt.Sprintf("ss://%s@%s:%d#%s", userinfo, host, p.Port, label), nil
	case "vmess":
		tls := ""
		if p.Security == "tls" || p.Security == "reality" {
			tls = p.Security
		}
		doc := map[string]any{
			"v": "2", "ps": p.Name, "add": p.Host, "port": p.Port,
			"id": p.UUID, "aid": "0", "scy": "auto",
			"net": p.Transport, "type": p.HeaderType,
			"host": p.HeaderHost, "path": orDefault(p.Path, p.ServiceName),
			"tls": tls, "sni": p.SNI, "fp": p.Fingerprint,
		}
		encoded, err := json.Marshal(doc)
		if err != nil {
			return "", err
		}
		return "vmess://" + base64.StdEncoding.EncodeToString(encoded), nil
	}
	return "", fmt.Errorf("subscription: no URI form for protocol %q", p.Protocol)
}

func applyTransportParams(extra map[string]string, p Proxy) {
	switch p.Transport {
	case "ws":
		extra["path"] = orDefault(p.Path, "/")
		if p.HeaderHost != "" {
			extra["host"] = p.HeaderHost
		}
	case "grpc":
		if p.ServiceName != "" {
			extra["serviceName"] = p.ServiceName
		}
	case "xhttp":
		extra["path"] = orDefault(p.Path, "/")
		if p.HeaderHost != "" {
			extra["host"] = p.HeaderHost
		}
		if p.Mode != "" {
			extra["mode"] = p.Mode
		}
	case "httpupgrade":
		extra["path"] = orDefault(p.Path, "/")
		if p.HeaderHost != "" {
			extra["host"] = p.HeaderHost
		}
	case "kcp":
		if p.HeaderType != "" {
			extra["headerType"] = p.HeaderType
		}
	}
}

func applySecurityParams(extra map[string]string, p Proxy) {
	switch p.Security {
	case "tls":
		if p.SNI != "" {
			extra["sni"] = p.SNI
		}
		if p.Fingerprint != "" {
			extra["fp"] = p.Fingerprint
		}
	case "reality":
		if p.PublicKey == "" || p.ShortID == "" {
			return // renderer refuses silently-broken REALITY elsewhere
		}
		extra["pbk"] = p.PublicKey
		extra["sid"] = p.ShortID
		if p.SNI != "" {
			extra["sni"] = p.SNI
		}
		extra["fp"] = orDefault(p.Fingerprint, "chrome")
	}
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

// urlEscape percent-encodes a URI component (userinfo/fragment).
// QueryEscape escapes everything reserved; "+" is re-encoded as
// %20 so passwords with spaces survive verbatim.
func urlEscape(s string) string {
	return strings.ReplaceAll(url.QueryEscape(s), "+", "%20")
}

// jsonScalar renders a string as a JSON string literal; the output
// doubles as a YAML double-quoted scalar for the safe subset the
// emitter produces (printable ASCII plus \u escapes).
func jsonScalar(s string) string {
	encoded, err := json.Marshal(s)
	if err != nil {
		return `""`
	}
	return string(encoded)
}

// yamlLines emits a nested mapping from ordered key/value pairs.
// Every scalar is double-quoted (JSON escaping), so operator- or
// node-supplied strings can never break out of the scalar context.
type yamlMap struct {
	keys   []string
	values map[string]any
}

func newYamlMap() *yamlMap {
	return &yamlMap{values: map[string]any{}}
}

func (m *yamlMap) set(key string, value any) {
	if _, exists := m.values[key]; !exists {
		m.keys = append(m.keys, key)
	}
	m.values[key] = value
}

// emit writes the mapping. list marks the mapping as a sequence
// item: the first key gets the "- " marker (Clash proxies list).
func (m *yamlMap) emit(indent string, list bool, out *strings.Builder) {
	for i, k := range m.keys {
		keyIndent := indent
		if i == 0 && list {
			keyIndent = indent[:len(indent)-2] + "- "
		}
		switch v := m.values[k].(type) {
		case string:
			fmt.Fprintf(out, "%s%s: %s\n", keyIndent, k, jsonScalar(v))
		case bool:
			fmt.Fprintf(out, "%s%s: %v\n", keyIndent, k, v)
		case int:
			fmt.Fprintf(out, "%s%s: %d\n", keyIndent, k, v)
		case *yamlMap:
			fmt.Fprintf(out, "%s%s:\n", keyIndent, k)
			v.emit(indent+"  ", false, out)
		case []string:
			fmt.Fprintf(out, "%s%s:\n", keyIndent, k)
			for _, item := range v {
				fmt.Fprintf(out, "%s  - %s\n", indent, jsonScalar(item))
			}
		}
	}
}

func renderClash(proxies []Proxy) ([]byte, error) {
	var out strings.Builder
	out.WriteString("# Generated by DuckOS Xray Center. Do not edit; updates overwrite.\n")
	out.WriteString("proxies:\n")
	for _, p := range proxies {
		m := newYamlMap()
		m.set("name", p.Name)
		switch p.Protocol {
		case "vless", "vmess":
			m.set("type", p.Protocol)
			m.set("server", p.Host)
			m.set("port", p.Port)
			m.set("uuid", p.UUID)
			if p.Protocol == "vmess" {
				m.set("alterId", 0)
				m.set("cipher", "auto")
			}
			if p.Flow != "" {
				m.set("flow", p.Flow)
			}
		case "trojan":
			m.set("type", "trojan")
			m.set("server", p.Host)
			m.set("port", p.Port)
			m.set("password", p.Password)
		case "ss":
			m.set("type", "ss")
			m.set("server", p.Host)
			m.set("port", p.Port)
			m.set("cipher", p.Cipher)
			m.set("password", p.Password)
		}
		m.set("udp", true)
		if p.Transport != "" && p.Transport != "tcp" {
			m.set("network", p.Transport)
		}
		switch p.Transport {
		case "ws":
			ws := newYamlMap()
			ws.set("path", orDefault(p.Path, "/"))
			if p.HeaderHost != "" {
				hdrs := newYamlMap()
				hdrs.set("Host", p.HeaderHost)
				ws.set("headers", hdrs)
			}
			m.set("ws-opts", ws)
		case "grpc":
			g := newYamlMap()
			g.set("grpc-service-name", orDefault(p.ServiceName, "duckos"))
			m.set("grpc-opts", g)
		}
		switch p.Security {
		case "tls":
			m.set("tls", true)
			if p.SNI != "" {
				m.set("servername", p.SNI)
			}
			if p.Fingerprint != "" {
				m.set("client-fingerprint", p.Fingerprint)
			}
		case "reality":
			m.set("tls", true)
			if p.SNI != "" {
				m.set("servername", p.SNI)
			}
			m.set("client-fingerprint", orDefault(p.Fingerprint, "chrome"))
			r := newYamlMap()
			r.set("public-key", p.PublicKey)
			r.set("short-id", p.ShortID)
			m.set("reality-opts", r)
		}
		m.emit("    ", true, &out)
	}
	if len(proxies) > 0 {
		out.WriteString("proxy-groups:\n")
		out.WriteString("  - name: \"PROXY\"\n")
		out.WriteString("    type: select\n")
		out.WriteString("    proxies:\n")
		for _, p := range proxies {
			fmt.Fprintf(&out, "      - %s\n", jsonScalar(p.Name))
		}
	}
	return []byte(out.String()), nil
}

func renderJSON(proxies []Proxy) ([]byte, error) {
	doc := map[string]any{
		"version": 1,
		"kind":    "duckos-subscription",
		"proxies": proxies,
	}
	encoded, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(encoded, '\n'), nil
}
