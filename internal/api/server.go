// Package api implements the authenticated, loopback management boundary.
package api

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/doctor"
	"github.com/CrazyDuckLab/DuckOS/internal/engine"
	"github.com/CrazyDuckLab/DuckOS/internal/healthauth"
	"github.com/CrazyDuckLab/DuckOS/internal/identity"
	"github.com/CrazyDuckLab/DuckOS/internal/migrate"
	"github.com/CrazyDuckLab/DuckOS/internal/model"
	"github.com/CrazyDuckLab/DuckOS/internal/multiwan"
	"github.com/CrazyDuckLab/DuckOS/internal/object"
	"github.com/CrazyDuckLab/DuckOS/internal/qos"
	"github.com/CrazyDuckLab/DuckOS/internal/sites"
	"github.com/CrazyDuckLab/DuckOS/internal/state"
	"github.com/CrazyDuckLab/DuckOS/internal/topology"
	"github.com/CrazyDuckLab/DuckOS/internal/ups"
	"github.com/CrazyDuckLab/DuckOS/internal/wifi"
	"github.com/CrazyDuckLab/DuckOS/pkg/buildinfo"
)

type Engine interface {
	Overview(context.Context) ([]byte, error)
	Ready(context.Context) error
	Readiness(context.Context) (engine.ReadinessSnapshot, error)
	Doctor(context.Context) (doctor.Snapshot, error)
	Topology(context.Context) (topology.Graph, error)
	MultiWAN(context.Context) (multiwan.Topology, map[string]multiwan.Selection, map[string]string, error)
	QoS(context.Context) (qos.Status, error)
	QoSApply(context.Context, qos.Plan) (qos.ApplyOutcome, error)
	QoSRollback(context.Context) (qos.ApplyOutcome, error)
	UPS(context.Context) (ups.Status, error)
	UPSProbe(context.Context) error
	Migrate(context.Context, migrate.Source, []byte) (migrate.Plan, error)
	Sites(context.Context) (sites.Status, error)
	Wifi(context.Context) (wifi.Status, error)
	Config() *engine.Config
	ObjectList(context.Context, string, object.CollectionParams) (object.Collection, error)
	Upgrade(context.Context) ([]byte, error)
	// UpgradeStage proxies a public
	// /api/v1/upgrade/stage POST to the Agent
	// /agent/v1/upgrade/stage IPC. DV07
	// production-path contract.
	UpgradeStage(context.Context, []byte) ([]byte, error)
	// UpgradeActivate proxies a public
	// /api/v1/upgrade/activate POST to the Agent.
	UpgradeActivate(context.Context, []byte) ([]byte, error)
	// UpgradeRollback proxies a public
	// /api/v1/upgrade/rollback POST to the Agent.
	UpgradeRollback(context.Context, []byte) ([]byte, error)
	// FRR proxies the public /api/v1/frr GET to the
	// Agent /agent/v1/frr IPC. DV09 production-path.
	FRR(context.Context) ([]byte, error)
	// FRRApply proxies a public /api/v1/frr/apply
	// POST to the Agent.
	FRRApply(context.Context, []byte) ([]byte, error)
	// FRRRollback proxies a public
	// /api/v1/frr/rollback POST to the Agent.
	FRRRollback(context.Context, []byte) ([]byte, error)
	ZTPIssue(context.Context, []byte) ([]byte, error)
	ZTPRedeem(context.Context, []byte) ([]byte, error)
	Backup(context.Context, string) ([]byte, error)
	// BackupRestore proxies a public
	// /api/v1/backup/restore POST to the Agent
	// /agent/v1/backup/restore IPC. DV08
	// production-path contract.
	BackupRestore(context.Context, []byte) ([]byte, error)
	// WireGuardProxy proxies the public /api/v1/wireguard/*
	// mutation to the privileged Agent
	// /agent/v1/wireguard/wireguard/v1/* IPC. The Engine
	// is the canonical bridge: it owns the Unix socket
	// client, the size cap and the typed-error envelope.
	// DV04-WG production-path contract.
	WireGuardProxy(ctx context.Context, method, path, token string, body io.Reader) (status int, data []byte, contentType string, err error)
	// XrayProxy proxies the public /api/v1/xray/*
	// mutation to the privileged Agent
	// /agent/v1/xray/xray/v1/* IPC. DV04-XRAY
	// production-path contract.
	XrayProxy(ctx context.Context, method, path, token string, body io.Reader) (status int, data []byte, contentType string, err error)
	// VaultProxy proxies the public /api/v1/vault/*
	// request to the privileged Agent
	// /agent/v1/vault/vault/v1/* IPC. DV04-VAULT
	// production-path contract. The vault never returns
	// secret bytes.
	VaultProxy(ctx context.Context, method, path, token string, body io.Reader) (status int, data []byte, contentType string, err error)
	// SpeedLabProxy proxies the public /api/v1/speedlab/*
	// request to the privileged Agent
	// /agent/v1/speedlab/* IPC. The Speed Lab never opens a
	// network connection from the public API path; only the
	// Agent collector is allowed to dial. DV04-SPEEDLAB
	// production-path contract.
	SpeedLabProxy(ctx context.Context, method, path, token string, body io.Reader) (status int, data []byte, contentType string, err error)
	// UPnPProxy proxies the public /api/v1/upnp/* request
	// to the privileged Agent /agent/v1/upnp/* IPC. The
	// public API never opens a network listener; only the
	// Agent is allowed to mutate nftables state. P065
	// production-path contract.
	UPnPProxy(ctx context.Context, method, path, token string, body io.Reader) (status int, data []byte, contentType string, err error)
	// WakeProxy proxies the public /api/v1/wake/* request
	// to the privileged Agent /agent/v1/wake/* IPC. Wake
	// never opens a raw socket from the public API path;
	// only the Agent is allowed to dial UDP/9. P065
	// production-path contract.
	WakeProxy(ctx context.Context, method, path, token string, body io.Reader) (status int, data []byte, contentType string, err error)
}
type Handler struct {
	updaterHealthKey []byte
	engine           Engine
	tokenHash        [32]byte
	assertionKey     string
	port             string
	web              http.Handler
	webRoot          *os.Root
	slots            chan struct{}
	identity         *identity.Service
	policy           *identity.Policy
	identitySlots    chan struct{}
	tlsOnly          bool
}

// ValidateListen requires a literal loopback address; hostname resolution cannot
// accidentally broaden the bind boundary during this read-only foundation.
func ValidateListen(address string) (string, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return "", err
	}
	ip := net.ParseIP(host)
	p, e := strconv.Atoi(port)
	if ip == nil || !ip.IsLoopback() || e != nil || p < 1 || p > 65535 {
		return "", errors.New("API requires a literal loopback address and port 1..65535")
	}
	return port, nil
}

func LoadToken(filename string) (string, error) {
	info, err := os.Lstat(filename)
	if err != nil {
		return "", errors.New("API token file cannot be read")
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0037 != 0 || info.Size() > 256 {
		return "", errors.New("API token must be a regular private file, mode 0600 or 0640")
	}
	f, err := os.Open(filename)
	if err != nil {
		return "", errors.New("API token file cannot be read")
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return "", errors.New("API token file changed during open")
	}
	data, err := io.ReadAll(io.LimitReader(f, 257))
	if err != nil || len(data) > 256 {
		return "", errors.New("API token file exceeds its limit")
	}
	token := strings.TrimSpace(string(data))
	if len(token) < 32 || len(token) > 128 {
		return "", errors.New("API token must contain 32..128 characters")
	}
	for _, c := range token {
		if c < '!' || c > '~' {
			return "", errors.New("API token must contain printable non-space ASCII")
		}
	}
	return token, nil
}

func New(engine Engine, token, listen, webDir string) (*Handler, error) {
	port, err := ValidateListen(listen)
	if err != nil {
		return nil, err
	}
	if len(token) < 32 || len(token) > 128 {
		return nil, errors.New("invalid API token length")
	}
	root, err := os.OpenRoot(webDir)
	if err != nil {
		return nil, errors.New("compiled web directory is unavailable")
	}
	index, err := root.Open("index.html")
	if err != nil {
		_ = root.Close()
		return nil, errors.New("compiled web index is unavailable")
	}
	_ = index.Close()
	return &Handler{engine: engine, tokenHash: sha256.Sum256([]byte(token)), port: port, web: http.FileServerFS(root.FS()), webRoot: root, slots: make(chan struct{}, 8)}, nil
}

func (h *Handler) EnableAgentAssertionKey(secret string) {
	h.assertionKey = secret
}
func (h *Handler) Close() error { return h.webRoot.Close() }

func (h *Handler) trustedHost(host string) bool {
	name, port, err := net.SplitHostPort(host)
	if err != nil {
		return false
	}
	if port != h.port {
		return false
	}
	return name == "localhost" || name == "127.0.0.1" || name == "::1"
}
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; font-src 'self'; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'; object-src 'none'")
	w.Header().Set("Cache-Control", "no-store")
	if h.tlsOnly && r.TLS == nil {
		problem(w, r, 400, errorCategoryFromCode("tls_required"), "tls_required", "TLS is required for this listener.")
		return
	}
	if !h.trustedHost(r.Host) {
		problem(w, r, 403, errorCategoryFromCode("untrusted_host"), "untrusted_host", "Management host is not allowed.")
		return
	}
	if origin := r.Header.Get("Origin"); origin != "" {
		scheme := "http"
		if h.tlsOnly {
			scheme = "https"
		}
		u, err := url.Parse(origin)
		if err != nil || u.Scheme != scheme || u.User != nil || u.Host != r.Host || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
			problem(w, r, 403, errorCategoryFromCode("untrusted_origin"), "untrusted_origin", "Cross-origin management requests are not allowed.")
			return
		}
	}
	if site := r.Header.Get("Sec-Fetch-Site"); site == "cross-site" {
		problem(w, r, 403, errorCategoryFromCode("untrusted_origin"), "untrusted_origin", "Cross-site requests are not allowed.")
		return
	}
	if h.identity != nil && strings.HasPrefix(r.URL.Path, "/api/v1/auth/") {
		h.serveIdentity(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/api/v1/network/") {
		h.serveNetwork(w, r)
		return
	}
	if r.URL.Path == "/api/v1/clients/export" {
		h.serveClientExport(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/api/v1/objects/") || r.URL.Path == "/api/v1/objects" {
		h.handleObjects(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/api/v1/config/") || r.URL.Path == "/api/v1/config" {
		h.handleConfig(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/api" {
		// G03: per-path method gating is now done inside
		// serveAPI. The blanket GET/HEAD gate that used to
		// live here blocked POST handlers (qos/apply,
		// qos/rollback, ups/probe, migrate) from ever
		// reaching their inner method checks. Body / query
		// validation is also enforced per-path so write
		// endpoints can accept a body while read endpoints
		// still reject it.
		h.serveAPI(w, r)
		return
	}
	h.serveWeb(w, r)
}
func (h *Handler) serveAPI(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/api/v1/health" {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			problem(w, r, 405, errorCategoryFromCode("method_not_allowed"), "method_not_allowed", "Health probe is read-only.")
			return
		}
		writeJSON(w, 200, map[string]string{"status": "ok", "schemaVersion": model.SchemaVersion})
		return
	}
	if r.URL.Path != "/api/v1/overview" && r.URL.Path != "/api/v1/ready" && r.URL.Path != "/api/v1/readiness" && r.URL.Path != "/api/v1/doctor" && r.URL.Path != "/api/v1/topology" && r.URL.Path != "/api/v1/multiwan" && r.URL.Path != "/api/v1/qos" && r.URL.Path != "/api/v1/qos/apply" && r.URL.Path != "/api/v1/qos/rollback" && r.URL.Path != "/api/v1/ups" && r.URL.Path != "/api/v1/ups/probe" && r.URL.Path != "/api/v1/migrate" && r.URL.Path != "/api/v1/sites" && r.URL.Path != "/api/v1/wifi" && r.URL.Path != "/api/v1/upgrade" && r.URL.Path != "/api/v1/upgrade/stage" && r.URL.Path != "/api/v1/upgrade/activate" && r.URL.Path != "/api/v1/upgrade/rollback" && r.URL.Path != "/api/v1/backup" && r.URL.Path != "/api/v1/backup/restore" && r.URL.Path != "/api/v1/frr" && r.URL.Path != "/api/v1/frr/apply" && r.URL.Path != "/api/v1/frr/rollback" && r.URL.Path != "/api/v1/ztp/issue" && r.URL.Path != "/api/v1/ztp/redeem" && !strings.HasPrefix(r.URL.Path, "/api/v1/wireguard/") && r.URL.Path != "/api/v1/wireguard" && !strings.HasPrefix(r.URL.Path, "/api/v1/xray/") && r.URL.Path != "/api/v1/xray" && !strings.HasPrefix(r.URL.Path, "/api/v1/vault/") && r.URL.Path != "/api/v1/vault" && !strings.HasPrefix(r.URL.Path, "/api/v1/speedlab/") && r.URL.Path != "/api/v1/speedlab" && !strings.HasPrefix(r.URL.Path, "/api/v1/upnp/") && r.URL.Path != "/api/v1/upnp" && !strings.HasPrefix(r.URL.Path, "/api/v1/wake/") && r.URL.Path != "/api/v1/wake" {
		problem(w, r, 404, errorCategoryFromCode("not_found"), "not_found", "Unknown API operation.")
		return
	}
	readOnly := r.URL.Path == "/api/v1/overview" || r.URL.Path == "/api/v1/ready" || r.URL.Path == "/api/v1/readiness" || r.URL.Path == "/api/v1/doctor" || r.URL.Path == "/api/v1/topology" || r.URL.Path == "/api/v1/multiwan" || r.URL.Path == "/api/v1/qos" || r.URL.Path == "/api/v1/ups" || r.URL.Path == "/api/v1/sites" || r.URL.Path == "/api/v1/wifi" || r.URL.Path == "/api/v1/upgrade" || r.URL.Path == "/api/v1/backup" || (strings.HasPrefix(r.URL.Path, "/api/v1/wireguard/") && r.Method == http.MethodGet) || r.URL.Path == "/api/v1/wireguard" && r.Method == http.MethodGet || (strings.HasPrefix(r.URL.Path, "/api/v1/xray/") && r.Method == http.MethodGet) || r.URL.Path == "/api/v1/xray" && r.Method == http.MethodGet
	// Per-path method enforcement (G03): the blanket
	// outer GET/HEAD gate has been removed, so each
	// endpoint enforces its own method and body contract.
	if readOnly {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			problem(w, r, 405, errorCategoryFromCode("method_not_allowed"), "method_not_allowed", "This endpoint is read-only.")
			return
		}
		if r.ContentLength != 0 || len(r.TransferEncoding) != 0 {
			problem(w, r, 400, errorCategoryFromCode("invalid_request"), "invalid_request", "Request bodies are not accepted on read-only endpoints.")
			return
		}
		if r.URL.ForceQuery || r.URL.RawPath != "" {
			problem(w, r, 400, errorCategoryFromCode("invalid_request"), "invalid_request", "API path is not canonical.")
			return
		}
		// G06: bounded collection parameters (limit, after)
		// are whitelisted on read-only endpoints. Unknown
		// query keys are ignored so callers cannot smuggle
		// additional parameters.
		_ = ParseCollectionParams(r.URL.RawQuery)
	}
	value := r.Header.Get("Authorization")
	token, ok := strings.CutPrefix(value, "Bearer ")
	hash := sha256.Sum256([]byte(token))
	authorized := ok && len(r.Header.Values("Authorization")) == 1 && subtle.ConstantTimeCompare(hash[:], h.tokenHash[:]) == 1
	user := identity.User{Username: "bootstrap", Role: identity.ReadOnly}
	machineHealth := healthauth.Verify(h.updaterHealthKey, r, time.Now())
	if h.identity != nil && h.identity.Enrolled() && !machineHealth {
		required := identity.Observe
		if !readOnly {
			required = identity.ConfigureNetwork
		}
		if r.URL.Path == "/api/v1/backup/restore" {
			required = identity.ManageBackups
		}
		u, err := h.identity.Authorize(token, required)
		authorized = ok && len(r.Header.Values("Authorization")) == 1 && err == nil
		if err == nil {
			user = u
		}
	}
	if r.URL.Path == "/api/v1/backup/restore" && (h.identity == nil || !h.identity.Enrolled()) {
		authorized = false
	}
	if !authorized {
		if machineHealth {
			authorized = true
		}
	}
	if !authorized {
		w.Header().Set("WWW-Authenticate", "Bearer realm=\"DuckOS\"")
		problem(w, r, 401, errorCategoryFromCode("unauthorized"), "unauthorized", "Enter the installation access token.")
		return
	}
	select {
	case h.slots <- struct{}{}:
		defer func() { <-h.slots }()
	default:
		problem(w, r, 503, errorCategoryFromCode("api_busy"), "api_busy", "Observation capacity is busy; retry shortly.")
		return
	}
	if strings.HasPrefix(r.URL.Path, "/api/v1/wireguard/") || r.URL.Path == "/api/v1/wireguard" {
		if h.assertionKey == "" {
			problem(w, r, 503, errorCategoryFromCode("agent_unavailable"), "agent_unavailable", "Agent assertion key is not configured.")
			return
		}
		h.handleWireGuardProxy(w, r, identity.ForwardedToken(h.assertionKey, user))
		return
	}
	if strings.HasPrefix(r.URL.Path, "/api/v1/xray/") || r.URL.Path == "/api/v1/xray" {
		if h.assertionKey == "" {
			problem(w, r, 503, errorCategoryFromCode("agent_unavailable"), "agent_unavailable", "Agent assertion key is not configured.")
			return
		}
		h.handleXrayProxy(w, r, identity.ForwardedToken(h.assertionKey, user))
		return
	}
	if strings.HasPrefix(r.URL.Path, "/api/v1/vault/") || r.URL.Path == "/api/v1/vault" {
		if h.assertionKey == "" {
			problem(w, r, 503, errorCategoryFromCode("agent_unavailable"), "agent_unavailable", "Agent assertion key is not configured.")
			return
		}
		h.handleVaultProxy(w, r, identity.ForwardedToken(h.assertionKey, user))
		return
	}
	if strings.HasPrefix(r.URL.Path, "/api/v1/speedlab/") || r.URL.Path == "/api/v1/speedlab" {
		if h.assertionKey == "" {
			problem(w, r, 503, errorCategoryFromCode("agent_unavailable"), "agent_unavailable", "Agent assertion key is not configured.")
			return
		}
		h.handleSpeedLabProxy(w, r, identity.ForwardedToken(h.assertionKey, user))
		return
	}
	if strings.HasPrefix(r.URL.Path, "/api/v1/upnp/") || r.URL.Path == "/api/v1/upnp" {
		if h.assertionKey == "" {
			problem(w, r, 503, errorCategoryFromCode("agent_unavailable"), "agent_unavailable", "Agent assertion key is not configured.")
			return
		}
		h.handleUPnPProxy(w, r, identity.ForwardedToken(h.assertionKey, user))
		return
	}
	if strings.HasPrefix(r.URL.Path, "/api/v1/wake/") || r.URL.Path == "/api/v1/wake" {
		if h.assertionKey == "" {
			problem(w, r, 503, errorCategoryFromCode("agent_unavailable"), "agent_unavailable", "Agent assertion key is not configured.")
			return
		}
		h.handleWakeProxy(w, r, identity.ForwardedToken(h.assertionKey, user))
		return
	}
	if r.URL.Path == "/api/v1/ready" {
		if err := h.engine.Ready(r.Context()); err != nil {
			problem(w, r, 503, errorCategoryFromCode("agent_unavailable"), "agent_unavailable", "Privileged Agent is unavailable or returned incompatible state.")
			return
		}
		writeJSON(w, 200, map[string]string{"status": "ready", "schemaVersion": model.SchemaVersion, "buildVersion": buildinfo.Version, "buildCommit": buildinfo.Commit})
		return
	}
	if r.URL.Path == "/api/v1/readiness" {
		snap, err := h.engine.Readiness(r.Context())
		if err != nil {
			problem(w, r, 503, errorCategoryFromCode("agent_unavailable"), "agent_unavailable", "Privileged Agent is unavailable or returned incompatible state.")
			return
		}
		writeJSON(w, 200, snap)
		return
	}
	if r.URL.Path == "/api/v1/doctor" {
		snap, err := h.engine.Doctor(r.Context())
		if err != nil {
			problem(w, r, 503, errorCategoryFromCode("agent_unavailable"), "agent_unavailable", "Privileged Agent is unavailable or returned incompatible state.")
			return
		}
		writeJSON(w, 200, snap)
		return
	}
	if r.URL.Path == "/api/v1/topology" {
		graph, err := h.engine.Topology(r.Context())
		if err != nil {
			problem(w, r, 503, errorCategoryFromCode("agent_unavailable"), "agent_unavailable", "Privileged Agent is unavailable or returned incompatible state.")
			return
		}
		writeJSON(w, 200, graph)
		return
	}
	if r.URL.Path == "/api/v1/multiwan" {
		t, sel, health, err := h.engine.MultiWAN(r.Context())
		if err != nil {
			problem(w, r, 503, errorCategoryFromCode("agent_unavailable"), "agent_unavailable", "Privileged Agent is unavailable or returned incompatible state.")
			return
		}
		writeJSON(w, 200, map[string]any{
			"topology":   t,
			"selections": sel,
			"health":     health,
		})
		return
	}
	if r.URL.Path == "/api/v1/qos" {
		st, err := h.engine.QoS(r.Context())
		if err != nil {
			problem(w, r, 503, errorCategoryFromCode("agent_unavailable"), "agent_unavailable", "Privileged Agent is unavailable or returned incompatible state.")
			return
		}
		writeJSON(w, 200, st)
		return
	}
	if r.URL.Path == "/api/v1/qos/apply" {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", "POST")
			problem(w, r, 405, errorCategoryFromCode("method_not_allowed"), "method_not_allowed", "QoS apply requires POST.")
			return
		}
		defer func() { _ = r.Body.Close() }()
		data, err := io.ReadAll(io.LimitReader(r.Body, engine.QoSMaxBytes+1))
		if err != nil {
			problem(w, r, 400, errorCategoryFromCode("invalid_body"), "invalid_body", "QoS apply body could not be read.")
			return
		}
		if len(data) > engine.QoSMaxBytes {
			problem(w, r, 413, errorCategoryFromCode("payload_too_large"), "payload_too_large", "QoS apply body exceeds the cap.")
			return
		}
		var p qos.Plan
		dec := json.NewDecoder(strings.NewReader(string(data)))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&p); err != nil {
			problem(w, r, 400, errorCategoryFromCode("invalid_plan"), "invalid_plan", "QoS plan could not be decoded.")
			return
		}
		if err := p.Valid(); err != nil {
			problem(w, r, 400, errorCategoryFromCode("invalid_plan"), "invalid_plan", "QoS plan did not validate.")
			return
		}
		out, err := h.engine.QoSApply(r.Context(), p)
		if err != nil {
			problem(w, r, 503, errorCategoryFromCode("qos_apply_failed"), "qos_apply_failed", err.Error())
			return
		}
		writeJSON(w, 200, out)
		return
	}
	if r.URL.Path == "/api/v1/qos/rollback" {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", "POST")
			problem(w, r, 405, errorCategoryFromCode("method_not_allowed"), "method_not_allowed", "QoS rollback requires POST.")
			return
		}
		out, err := h.engine.QoSRollback(r.Context())
		if err != nil {
			problem(w, r, 503, errorCategoryFromCode("qos_rollback_failed"), "qos_rollback_failed", err.Error())
			return
		}
		writeJSON(w, 200, out)
		return
	}
	if r.URL.Path == "/api/v1/ups" {
		st, err := h.engine.UPS(r.Context())
		if err != nil {
			problem(w, r, 503, errorCategoryFromCode("agent_unavailable"), "agent_unavailable", "Privileged Agent is unavailable or returned incompatible state.")
			return
		}
		writeJSON(w, 200, st)
		return
	}
	if r.URL.Path == "/api/v1/ups/probe" {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", "POST")
			problem(w, r, 405, errorCategoryFromCode("method_not_allowed"), "method_not_allowed", "UPS probe requires POST.")
			return
		}
		if err := h.engine.UPSProbe(r.Context()); err != nil {
			problem(w, r, 503, errorCategoryFromCode("ups_probe_failed"), "ups_probe_failed", err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.URL.Path == "/api/v1/migrate" {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", "POST")
			problem(w, r, 405, errorCategoryFromCode("method_not_allowed"), "method_not_allowed", "Migrate analysis requires POST.")
			return
		}
		defer func() { _ = r.Body.Close() }()
		data, err := io.ReadAll(io.LimitReader(r.Body, engine.MigrateMaxBytes+1))
		if err != nil {
			problem(w, r, 400, errorCategoryFromCode("invalid_body"), "invalid_body", "Migrate body could not be read.")
			return
		}
		if len(data) > engine.MigrateMaxBytes {
			problem(w, r, 413, errorCategoryFromCode("payload_too_large"), "payload_too_large", "Migrate body exceeds the cap.")
			return
		}
		if len(data) == 0 {
			problem(w, r, 400, errorCategoryFromCode("empty_source"), "empty_source", "Migrate body is empty.")
			return
		}
		src := migrate.Source{Filename: r.Header.Get("X-DuckOS-Migrate-Filename"), Format: migrate.FormatKind(r.Header.Get("X-DuckOS-Migrate-Format")), Provenance: r.Header.Get("X-DuckOS-Migrate-Provenance")}
		plan, err := h.engine.Migrate(r.Context(), src, data)
		if err != nil {
			problem(w, r, 422, errorCategoryFromCode("migrate_failed"), "migrate_failed", err.Error())
			return
		}
		writeJSON(w, 200, plan)
		return
	}
	if r.URL.Path == "/api/v1/sites" {
		st, err := h.engine.Sites(r.Context())
		if err != nil {
			problem(w, r, 503, errorCategoryFromCode("agent_unavailable"), "agent_unavailable", "Privileged Agent is unavailable or returned incompatible state.")
			return
		}
		writeJSON(w, 200, st)
		return
	}
	if r.URL.Path == "/api/v1/wifi" {
		st, err := h.engine.Wifi(r.Context())
		if err != nil {
			problem(w, r, 503, errorCategoryFromCode("agent_unavailable"), "agent_unavailable", "Privileged Agent is unavailable or returned incompatible state.")
			return
		}
		writeJSON(w, 200, st)
		return
	}
	if r.URL.Path == "/api/v1/upgrade" {
		data, err := h.engine.Upgrade(r.Context())
		if err != nil {
			problem(w, r, 503, errorCategoryFromCode("agent_unavailable"), "agent_unavailable", "Privileged Agent is unavailable or returned incompatible state.")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = w.Write(data)
		return
	}
	if r.URL.Path == "/api/v1/upgrade/stage" {
		// DV07 production-path: signed update stage.
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", "POST")
			problem(w, r, 405, errorCategoryFromCode("method_not_allowed"), "method_not_allowed", "Stage endpoint accepts POST only.")
			return
		}
		if r.ContentLength > engine.UpgradeStageMaxBytes {
			problem(w, r, 413, errorCategoryFromCode("payload_too_large"), "payload_too_large", "Stage payload exceeds the public cap.")
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, engine.UpgradeStageMaxBytes+1))
		if err != nil {
			problem(w, r, 400, errorCategoryFromCode("invalid_request"), "invalid_request", "Could not read stage payload.")
			return
		}
		if len(body) > engine.UpgradeStageMaxBytes {
			problem(w, r, 413, errorCategoryFromCode("payload_too_large"), "payload_too_large", "Stage payload exceeds the public cap.")
			return
		}
		data, err := h.engine.UpgradeStage(r.Context(), body)
		if err != nil {
			problem(w, r, 503, errorCategoryFromCode("agent_unavailable"), "agent_unavailable", "Privileged Agent is unavailable or returned incompatible state.")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = w.Write(data)
		return
	}
	if r.URL.Path == "/api/v1/upgrade/activate" {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", "POST")
			problem(w, r, 405, errorCategoryFromCode("method_not_allowed"), "method_not_allowed", "Activate endpoint accepts POST only.")
			return
		}
		body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<16))
		data, err := h.engine.UpgradeActivate(r.Context(), body)
		if err != nil {
			problem(w, r, 503, errorCategoryFromCode("agent_unavailable"), "agent_unavailable", "Privileged Agent is unavailable or returned incompatible state.")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = w.Write(data)
		return
	}
	if r.URL.Path == "/api/v1/upgrade/rollback" {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", "POST")
			problem(w, r, 405, errorCategoryFromCode("method_not_allowed"), "method_not_allowed", "Rollback endpoint accepts POST only.")
			return
		}
		body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<16))
		data, err := h.engine.UpgradeRollback(r.Context(), body)
		if err != nil {
			problem(w, r, 503, errorCategoryFromCode("agent_unavailable"), "agent_unavailable", "Privileged Agent is unavailable or returned incompatible state.")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = w.Write(data)
		return
	}
	if r.URL.Path == "/api/v1/frr" {
		// DV09 production-path: GET-only status.
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", "GET")
			problem(w, r, 405, errorCategoryFromCode("method_not_allowed"), "method_not_allowed", "FRR endpoint accepts GET only.")
			return
		}
		data, err := h.engine.FRR(r.Context())
		if err != nil {
			problem(w, r, 503, errorCategoryFromCode("agent_unavailable"), "agent_unavailable", "Privileged Agent is unavailable or returned incompatible state.")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = w.Write(data)
		return
	}
	if r.URL.Path == "/api/v1/frr/apply" {
		// DV09 production-path: stage + reload.
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", "POST")
			problem(w, r, 405, errorCategoryFromCode("method_not_allowed"), "method_not_allowed", "FRR apply endpoint accepts POST only.")
			return
		}
		if r.ContentLength > engine.FRRApplyMaxBytes {
			problem(w, r, 413, errorCategoryFromCode("payload_too_large"), "payload_too_large", "FRR config exceeds the public cap.")
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, engine.FRRApplyMaxBytes+1))
		if err != nil {
			problem(w, r, 400, errorCategoryFromCode("invalid_request"), "invalid_request", "Could not read FRR config.")
			return
		}
		if len(body) > engine.FRRApplyMaxBytes {
			problem(w, r, 413, errorCategoryFromCode("payload_too_large"), "payload_too_large", "FRR config exceeds the public cap.")
			return
		}
		data, err := h.engine.FRRApply(r.Context(), body)
		if err != nil {
			problem(w, r, 503, errorCategoryFromCode("agent_unavailable"), "agent_unavailable", "Privileged Agent is unavailable or returned incompatible state.")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = w.Write(data)
		return
	}
	if r.URL.Path == "/api/v1/frr/rollback" {
		// DV09 production-path: rollback to previous.
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", "POST")
			problem(w, r, 405, errorCategoryFromCode("method_not_allowed"), "method_not_allowed", "FRR rollback endpoint accepts POST only.")
			return
		}
		body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<16))
		data, err := h.engine.FRRRollback(r.Context(), body)
		if err != nil {
			problem(w, r, 503, errorCategoryFromCode("agent_unavailable"), "agent_unavailable", "Privileged Agent is unavailable or returned incompatible state.")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = w.Write(data)
		return
	}
	if r.URL.Path == "/api/v1/ztp/issue" || r.URL.Path == "/api/v1/ztp/redeem" {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", "POST")
			problem(w, r, 405, errorCategoryFromCode("method_not_allowed"), "method_not_allowed", "ZTP endpoint accepts POST only.")
			return
		}
		if r.ContentLength > engine.ZTPMaxBytes {
			problem(w, r, 413, errorCategoryFromCode("payload_too_large"), "payload_too_large", "ZTP payload exceeds the public cap.")
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, engine.ZTPMaxBytes+1))
		if err != nil {
			problem(w, r, 400, errorCategoryFromCode("invalid_request"), "invalid_request", "Could not read ZTP payload.")
			return
		}
		if len(body) > engine.ZTPMaxBytes {
			problem(w, r, 413, errorCategoryFromCode("payload_too_large"), "payload_too_large", "ZTP payload exceeds the public cap.")
			return
		}
		var data []byte
		if r.URL.Path == "/api/v1/ztp/issue" {
			data, err = h.engine.ZTPIssue(r.Context(), body)
		} else {
			data, err = h.engine.ZTPRedeem(r.Context(), body)
		}
		if err != nil {
			problem(w, r, 503, errorCategoryFromCode("agent_unavailable"), "agent_unavailable", "Privileged Agent is unavailable or returned incompatible state.")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = w.Write(data)
		return
	}
	if r.URL.Path == "/api/v1/backup" {
		// G06: only the bounded query keys (id) are
		// whitelisted; everything else is rejected before
		// reaching the Agent so callers cannot smuggle
		// additional parameters.
		id := ""
		for k, vs := range r.URL.Query() {
			if k != "id" {
				problem(w, r, 400, errorCategoryFromCode("invalid_request"), "invalid_request", "Backup endpoint accepts only the id query parameter.")
				return
			}
			if len(vs) > 0 {
				id = vs[0]
			}
		}
		data, err := h.engine.Backup(r.Context(), id)
		if err != nil {
			problem(w, r, 503, errorCategoryFromCode("agent_unavailable"), "agent_unavailable", "Privileged Agent is unavailable or returned incompatible state.")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = w.Write(data)
		return
	}
	if r.URL.Path == "/api/v1/backup/restore" {
		// DV08 production-path: the public route
		// reaches the Agent's BackupRestoreHost,
		// which drives the production Executor +
		// JournalStore. The API carries the body
		// verbatim (already canonical JSON from the
		// caller); the Agent owns the lifecycle and
		// the durable journal entry.
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", "POST")
			problem(w, r, 405, errorCategoryFromCode("method_not_allowed"), "method_not_allowed", "Restore endpoint accepts POST only.")
			return
		}
		if r.ContentLength > engine.BackupRestoreMaxBytes {
			problem(w, r, 413, errorCategoryFromCode("payload_too_large"), "payload_too_large", "Restore payload exceeds the public cap.")
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, engine.BackupRestoreMaxBytes+1))
		if err != nil {
			problem(w, r, 400, errorCategoryFromCode("invalid_request"), "invalid_request", "Could not read restore payload.")
			return
		}
		if len(body) > engine.BackupRestoreMaxBytes {
			problem(w, r, 413, errorCategoryFromCode("payload_too_large"), "payload_too_large", "Restore payload exceeds the public cap.")
			return
		}
		data, err := h.engine.BackupRestore(r.Context(), body)
		if err != nil {
			if errors.Is(err, engine.ErrInvalidRestore) {
				problem(w, r, 400, errorCategoryFromCode("invalid_request"), "invalid_request", "Restore requires logical IDs, selected relative targets and an expected revision.")
				return
			}
			if errors.Is(err, state.ErrConflict) || errors.Is(err, state.ErrPending) {
				problem(w, r, 409, errorCategoryFromCode("revision_conflict"), "revision_conflict", "Restore admission conflicts with current configuration.")
				return
			}
			if errors.Is(err, engine.ErrRestoreRefused) {
				problem(w, r, 422, errorCategoryFromCode("restore_refused"), "restore_refused", "Restore was rejected or rolled back.")
				return
			}
			problem(w, r, 503, errorCategoryFromCode("agent_unavailable"), "agent_unavailable", "Privileged Agent is unavailable or returned incompatible state.")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = w.Write(data)
		return
	}
	data, err := h.engine.Overview(r.Context())
	if err != nil {
		problem(w, r, 503, errorCategoryFromCode("agent_unavailable"), "agent_unavailable", "Privileged Agent is unavailable or returned incompatible state.")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)
	_, _ = w.Write(data)
}
func (h *Handler) serveWeb(w http.ResponseWriter, r *http.Request) {
	clean := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if clean == "." || clean == "" {
		clean = "index.html"
	}
	// os.Root prevents symlinks in the asset tree escaping the immutable release.
	f, err := h.webRoot.Open(filepath.FromSlash(clean))
	if err == nil {
		info, e := f.Stat()
		_ = f.Close()
		if e != nil || info.IsDir() {
			problem(w, r, 404, errorCategoryFromCode("not_found"), "not_found", "Asset not found.")
			return
		}
		h.web.ServeHTTP(w, r)
		return
	}
	if !errors.Is(err, os.ErrNotExist) || path.Ext(clean) != "" || strings.HasPrefix(clean, "assets/") {
		problem(w, r, 404, errorCategoryFromCode("not_found"), "not_found", "Asset not found.")
		return
	}
	// Only extensionless frontend navigation gets the SPA entry document.
	index, e := h.webRoot.Open("index.html")
	if e != nil {
		problem(w, r, 503, errorCategoryFromCode("ui_unavailable"), "ui_unavailable", "Dashboard build is unavailable.")
		return
	}
	defer index.Close()
	info, e := index.Stat()
	if e != nil {
		problem(w, r, 503, errorCategoryFromCode("ui_unavailable"), "ui_unavailable", "Dashboard build is unavailable.")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	http.ServeContent(w, r, "index.html", info.ModTime(), index)
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

// problem emits a structured JSON error response (G15). The
// error envelope carries a stable category, a human-readable
// message that is safe for log/audit but never carries
// secrets, and a per-call request id that ties every error
// back to a single observation. UI/CLI localise the message
// by category; the category itself is language-neutral.
func problem(w http.ResponseWriter, r *http.Request, status int, category ErrorCategory, code, message string) {
	requestID := r.Header.Get("X-Request-Id")
	if requestID == "" {
		requestID = newRequestID()
	}
	w.Header().Set("X-Request-Id", requestID)
	writeJSON(w, status, map[string]any{
		"error": map[string]any{
			"category":  category,
			"code":      code,
			"message":   message,
			"requestId": requestID,
		},
	})
}

func newRequestID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "duckos-err"
	}
	return hex.EncodeToString(b[:])
}

// handleWireGuardProxy proxies the public /api/v1/wireguard/*
// surface to the privileged Agent
// /agent/v1/wireguard/wireguard/v1/* IPC through
// Engine.WireGuardProxy. The handler is the canonical
// bridge between the public loopback and the Agent's
// ConfigureNetwork-gated mutation surface.
//
// DV04-WG production-path contract:
//
//	PUBLIC -> handleWireGuardProxy -> Engine.WireGuardProxy
//	       -> Agent /agent/v1/wireguard/wireguard/v1/*
//	       -> WireguardHost -> wireguard.API ->
//	       KernelAdapter -> /sbin/ip + /usr/bin/wg
func (h *Handler) handleWireGuardProxy(w http.ResponseWriter, r *http.Request, token string) {
	if r.ContentLength > 1<<20 {
		problem(w, r, 413, errorCategoryFromCode("payload_too_large"), "payload_too_large", "WireGuard payload exceeds 1 MiB.")
		return
	}
	var body io.Reader
	if r.Body != nil {
		body = io.LimitReader(r.Body, 1<<20)
	}
	status, data, contentType, err := h.engine.WireGuardProxy(r.Context(), r.Method, r.URL.Path, token, body)
	if err != nil {
		problem(w, r, 503, errorCategoryFromCode("agent_unavailable"), "agent_unavailable", err.Error())
		return
	}
	if contentType != "" {
		w.Header().Set("Content-Type", contentType)
	} else {
		w.Header().Set("Content-Type", "application/json")
	}
	w.WriteHeader(status)
	if len(data) > 0 {
		_, _ = w.Write(data)
	}
}

// handleXrayProxy proxies the public /api/v1/xray/*
// surface to the privileged Agent
// /agent/v1/xray/xray/v1/* IPC through Engine.XrayProxy.
//
// DV04-XRAY production-path contract:
//
//	PUBLIC -> handleXrayProxy -> Engine.XrayProxy
//	       -> Agent /agent/v1/xray/xray/v1/* -> XrayHost
//	       -> xray.API -> RenderConfig + Runtime.Apply
//	       -> /tmp/duckos-test/xray/xray run -c <path>
func (h *Handler) handleXrayProxy(w http.ResponseWriter, r *http.Request, token string) {
	if r.ContentLength > 1<<20 {
		problem(w, r, 413, errorCategoryFromCode("payload_too_large"), "payload_too_large", "Xray payload exceeds 1 MiB.")
		return
	}
	var body io.Reader
	if r.Body != nil {
		body = io.LimitReader(r.Body, 1<<20)
	}
	status, data, contentType, err := h.engine.XrayProxy(r.Context(), r.Method, r.URL.Path, token, body)
	if err != nil {
		problem(w, r, 503, errorCategoryFromCode("agent_unavailable"), "agent_unavailable", err.Error())
		return
	}
	if contentType != "" {
		w.Header().Set("Content-Type", contentType)
	} else {
		w.Header().Set("Content-Type", "application/json")
	}
	w.WriteHeader(status)
	if len(data) > 0 {
		_, _ = w.Write(data)
	}
}

// handleVaultProxy proxies the public /api/v1/vault/*
// request to the privileged Agent
// /agent/v1/vault/vault/v1/* IPC through Engine.VaultProxy.
//
// DV04-VAULT production-path contract:
//
//	PUBLIC -> handleVaultProxy -> Engine.VaultProxy
//	        -> Agent /agent/v1/vault/vault/v1/* -> VaultHost
//	        -> vault.API -> VaultLifecycle.Put / List / Revoke
//
// The handler never reads the request body itself; the body is
// forwarded to the Agent unchanged. Secret bytes that reach
// the Agent are wiped before the Agent returns the Reference
// (see internal/vault/api.go). The handler enforces the Owner
// (secrets.write) permission at the Agent boundary, so this
// layer only needs to keep the public envelope intact.
func (h *Handler) handleVaultProxy(w http.ResponseWriter, r *http.Request, token string) {
	// Bounded body. The vault handler re-bounds at 16 KiB; this
	// guard short-circuits oversized requests at the public
	// boundary so we do not waste an Agent round-trip.
	if r.ContentLength > 16<<10 {
		problem(w, r, 413, errorCategoryFromCode("payload_too_large"), "payload_too_large", "Vault payload exceeds 16 KiB.")
		return
	}
	var body io.Reader
	if r.Body != nil {
		body = io.LimitReader(r.Body, 16<<10)
	}
	status, data, contentType, err := h.engine.VaultProxy(r.Context(), r.Method, r.URL.Path, token, body)
	if err != nil {
		problem(w, r, 503, errorCategoryFromCode("agent_unavailable"), "agent_unavailable", err.Error())
		return
	}
	if contentType != "" {
		w.Header().Set("Content-Type", contentType)
	} else {
		w.Header().Set("Content-Type", "application/json")
	}
	w.WriteHeader(status)
	if len(data) > 0 {
		_, _ = w.Write(data)
	}
}

// handleSpeedLabProxy proxies the public /api/v1/speedlab/*
// request to the privileged Agent /agent/v1/speedlab/* IPC
// through Engine.SpeedLabProxy.
//
// DV04-SPEEDLAB production-path contract:
//
//	PUBLIC -> handleSpeedLabProxy -> Engine.SpeedLabProxy
//	        -> Agent /agent/v1/speedlab/speedlab/v1/* -> SpeedLabHost
//	        -> speedlab.Manager -> LinuxHost
//
// The public API never opens a network connection itself;
// only the Agent collector is allowed to dial.
func (h *Handler) handleSpeedLabProxy(w http.ResponseWriter, r *http.Request, token string) {
	// Bounded body. The speedlab handler re-bounds at 4 KiB;
	// this guard short-circuits oversized requests at the
	// public boundary so we do not waste an Agent round-trip.
	if r.ContentLength > 4<<10 {
		problem(w, r, 413, errorCategoryFromCode("payload_too_large"), "payload_too_large", "Speed Lab payload exceeds 4 KiB.")
		return
	}
	var body io.Reader
	if r.Body != nil {
		body = io.LimitReader(r.Body, 4<<10)
	}
	status, data, contentType, err := h.engine.SpeedLabProxy(r.Context(), r.Method, r.URL.Path, token, body)
	if err != nil {
		problem(w, r, 503, errorCategoryFromCode("agent_unavailable"), "agent_unavailable", err.Error())
		return
	}
	if contentType != "" {
		w.Header().Set("Content-Type", contentType)
	} else {
		w.Header().Set("Content-Type", "application/json")
	}
	w.WriteHeader(status)
	if len(data) > 0 {
		_, _ = w.Write(data)
	}
}

// handleUPnPProxy proxies the public /api/v1/upnp/* request
// to the privileged Agent /agent/v1/upnp/* IPC through
// Engine.UPnPProxy.
//
// P065 production-path contract:
//
//	PUBLIC -> handleUPnPProxy -> Engine.UPnPProxy
//	       -> Agent /agent/v1/upnp/upnp/v1/* -> UPnPHost
//	       -> upnp.Runtime -> NftablesBackend
//
// The public API never opens a network listener or
// mutates nftables; only the Agent is allowed to.
func (h *Handler) handleUPnPProxy(w http.ResponseWriter, r *http.Request, token string) {
	if r.ContentLength > 4<<10 {
		problem(w, r, 413, errorCategoryFromCode("payload_too_large"), "payload_too_large", "UPnP payload exceeds 4 KiB.")
		return
	}
	var body io.Reader
	if r.Body != nil {
		body = io.LimitReader(r.Body, 4<<10)
	}
	status, data, contentType, err := h.engine.UPnPProxy(r.Context(), r.Method, r.URL.Path, token, body)
	if err != nil {
		problem(w, r, 503, errorCategoryFromCode("agent_unavailable"), "agent_unavailable", err.Error())
		return
	}
	if contentType != "" {
		w.Header().Set("Content-Type", contentType)
	} else {
		w.Header().Set("Content-Type", "application/json")
	}
	w.WriteHeader(status)
	if len(data) > 0 {
		_, _ = w.Write(data)
	}
}

// handleWakeProxy proxies the public /api/v1/wake/* request
// to the privileged Agent /agent/v1/wake/* IPC through
// Engine.WakeProxy.
//
// P065 production-path contract:
//
//	PUBLIC -> handleWakeProxy -> Engine.WakeProxy
//	        -> Agent /agent/v1/wake/wake/v1/* -> WakeHost
//	        -> wake.Manager -> standard WOL magic packet
//
// The public API never opens a raw socket; only the Agent
// is allowed to dial UDP/9.
func (h *Handler) handleWakeProxy(w http.ResponseWriter, r *http.Request, token string) {
	if r.ContentLength > 4<<10 {
		problem(w, r, 413, errorCategoryFromCode("payload_too_large"), "payload_too_large", "Wake payload exceeds 4 KiB.")
		return
	}
	var body io.Reader
	if r.Body != nil {
		body = io.LimitReader(r.Body, 4<<10)
	}
	status, data, contentType, err := h.engine.WakeProxy(r.Context(), r.Method, r.URL.Path, token, body)
	if err != nil {
		problem(w, r, 503, errorCategoryFromCode("agent_unavailable"), "agent_unavailable", err.Error())
		return
	}
	if contentType != "" {
		w.Header().Set("Content-Type", contentType)
	} else {
		w.Header().Set("Content-Type", "application/json")
	}
	w.WriteHeader(status)
	if len(data) > 0 {
		_, _ = w.Write(data)
	}
}
