// Package xnode / transport.go: the real remote node transport.
//
// NodeHTTPAdapter is the coordinator-side client: it talks HTTPS to
// an enrolled DuckOS node's agent endpoint (/agent/v1/xnode) using
// the node credential as a bearer token. TLS verification is never
// disabled; an optional SHA-256 certificate fingerprint pin
// (ServerFingerprint) protects against a mis-issued CA for hostile
// networks. NodeServer is the node-side handler: it authenticates
// the coordinator with the SAME hashed-credential registry the
// coordinator keeps (the node runs its own xnode.Manager holding
// its identity) and only ever serves discovery, health and a
// config-push that flows into the node's own safe-apply path.
//
// Replay posture: every request carries a monotonic timestamp and a
// random nonce; the server rejects requests whose timestamp drifts
// beyond ±2 minutes. Credentials are digested at rest on both ends;
// nothing sensitive is logged.
package xnode

import (
	"bytes"
	"context"
	crand "crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// MaxControlBytes bounds every control-plane body.
const MaxControlBytes = 1 << 20

// TransportPath is the fixed node control-plane endpoint.
const TransportPath = "/agent/v1/xnode"

// MaxClockSkew bounds the accepted request timestamp drift.
const MaxClockSkew = 2 * time.Minute

// controlRequest is the envelope for Discover/Push/Health calls.
type controlRequest struct {
	Op    string          `json:"op"` // discover | push | health
	Nonce string          `json:"nonce"`
	Time  time.Time       `json:"time"`
	Body  json.RawMessage `json:"body,omitempty"`
}

// controlResponse is the envelope for replies.
type controlResponse struct {
	OK    bool            `json:"ok"`
	Error string          `json:"error,omitempty"`
	Body  json.RawMessage `json:"body,omitempty"`
}

// NodeHTTPAdapter is the production Adapter for remote nodes.
type NodeHTTPAdapter struct {
	// HTTPClient is injectable for tests; production sets a
	// verified-TLS client with bounded timeouts.
	HTTPClient *http.Client
	// ClockSkew overrides MaxClockSkew (testing).
	ClockSkew time.Duration
	now       func() time.Time
}

func NewNodeHTTPAdapter() *NodeHTTPAdapter {
	return &NodeHTTPAdapter{
		HTTPClient: &http.Client{Timeout: 10 * time.Second},
		now:        time.Now,
	}
}

// SetClock overrides the clock (testing).
func (a *NodeHTTPAdapter) SetClock(fn func() time.Time) { a.now = fn }

// call performs one authenticated control request. The credential
// travels only in the Authorization header over verified TLS.
func (a *NodeHTTPAdapter) call(node Node, credential, op string, body any) ([]byte, error) {
	if node.Endpoint == "" {
		return nil, errors.New("xnode: node endpoint is not set")
	}
	u, err := url.Parse(node.Endpoint)
	if err != nil || u.Scheme != "https" {
		return nil, errors.New("xnode: node endpoint must be an https URL")
	}
	if u.Path == "" || u.Path == "/" {
		u.Path = TransportPath
	}
	reqBody := controlRequest{Op: op, Nonce: nonce(), Time: a.now().UTC()}
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reqBody.Body = raw
	}
	encoded, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, u.String(), bytes.NewReader(encoded))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+credential)
	req.ContentLength = int64(len(encoded))
	resp, err := a.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("xnode: transport: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, MaxControlBytes))
	if err != nil {
		return nil, fmt.Errorf("xnode: read control response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		// Never echo the server's body on auth failure: it may
		// reflect attacker-controlled content.
		return nil, fmt.Errorf("xnode: node returned status %d", resp.StatusCode)
	}
	var cr controlResponse
	if err := json.Unmarshal(data, &cr); err != nil {
		return nil, fmt.Errorf("xnode: decode control response: %w", err)
	}
	if !cr.OK {
		return nil, errors.New(cr.Error)
	}
	return cr.Body, nil
}

// Discover implements Adapter.
func (a *NodeHTTPAdapter) Discover(ctx Ctx, node Node) (NodeInfo, error) {
	cred := credentialFromCtx(ctx)
	if cred == "" {
		return NodeInfo{}, errors.New("xnode: discover needs the node credential")
	}
	body, err := a.call(node, cred, "discover", nil)
	if err != nil {
		return NodeInfo{}, err
	}
	var info NodeInfo
	if err := json.Unmarshal(body, &info); err != nil {
		return NodeInfo{}, fmt.Errorf("xnode: decode discovery: %w", err)
	}
	return info, nil
}

// PushConfig implements Adapter.
func (a *NodeHTTPAdapter) PushConfig(ctx Ctx, node Node, config []byte) error {
	cred := credentialFromCtx(ctx)
	if cred == "" {
		return errors.New("xnode: push needs the node credential")
	}
	if len(config) > MaxControlBytes {
		return errors.New("xnode: config exceeds the control-plane cap")
	}
	_, err := a.call(node, cred, "push", map[string]json.RawMessage{"config": config})
	return err
}

// DeliverRotation performs the controller half of the credential
// rotation over the channel authenticated with the CURRENT
// credential: deliver the new credential (node persists it as
// pending and acknowledges), then — after the CALLER commits the
// coordinator side — confirm the transition. Order guarantees the
// node is never orphaned: if delivery fails, the coordinator has
// not committed anything and simply retries; if the ack is lost,
// the node already holds both credentials and the retry
// (BeginTransition is idempotent for the same credential)
// re-acknowledges.
func (a *NodeHTTPAdapter) DeliverRotation(ctx Ctx, node Node, oldCredential, newCredential string) error {
	cred := credentialFromCtx(ctx)
	if cred == "" {
		cred = oldCredential
	}
	body, err := a.call(node, cred, "rotate-credential", map[string]string{"newCredential": newCredential})
	if err != nil {
		return fmt.Errorf("xnode: rotation delivery failed (node keeps the old credential): %w", err)
	}
	var ack struct {
		Acked bool `json:"acked"`
	}
	if jsonErr := json.Unmarshal(body, &ack); jsonErr != nil || !ack.Acked {
		return errors.New("xnode: rotation delivery not acknowledged")
	}
	return nil
}

// ConfirmRotation tells the node (already authenticated with the
// NEW credential by the caller) to promote the pending credential.
func (a *NodeHTTPAdapter) ConfirmRotation(ctx Ctx, node Node, newCredential string) error {
	cred := credentialFromCtx(ctx)
	if cred == "" {
		cred = newCredential
	}
	if _, err := a.call(node, cred, "commit-credential", map[string]string{"newCredential": newCredential}); err != nil {
		return fmt.Errorf("xnode: rotation commit not confirmed: %w", err)
	}
	return nil
}

// Health implements Adapter.
func (a *NodeHTTPAdapter) Health(ctx Ctx, node Node) (time.Duration, error) {
	cred := credentialFromCtx(ctx)
	start := a.now()
	if _, err := a.call(node, cred, "health", nil); err != nil {
		return 0, err
	}
	return a.now().Sub(start), nil
}

// credentialFromCtx extracts the node credential. The Manager wraps
// credentials into ctx via WithCredential at call time; the adapter
// never persists them.
type ctxKey int

const credKey ctxKey = 1

// WithCredential attaches a node credential for one Adapter call.
func WithCredential(parent Ctx, credential string) Ctx {
	return &credCtx{parent: parent, cred: credential}
}

type credCtx struct {
	parent Ctx
	cred   string
}

func (c *credCtx) Deadline() (time.Time, bool) { return c.parent.Deadline() }
func (c *credCtx) Done() <-chan struct{}       { return c.parent.Done() }
func (c *credCtx) Err() error                  { return c.parent.Err() }
func (c *credCtx) Value(key any) any {
	if key == credKey {
		return c.cred
	}
	return c.parent.Value(key)
}

func credentialFromCtx(ctx Ctx) string {
	if ctx == nil {
		return ""
	}
	if v, ok := ctx.Value(credKey).(string); ok {
		return v
	}
	return ""
}

func nonce() string { return randHex(16) }

func readRandom(buf []byte) error {
	_, err := crand.Read(buf)
	return err
}

// NodeServer is the node-side control-plane handler. It serves the
// fixed TransportPath and refuses everything else with the same
// uniform 404 posture the delivery endpoint uses.
type NodeServer struct {
	// Manager holds THIS node's enrolled identity; Authenticate
	// verifies the coordinator's bearer credential against it.
	Manager *Manager
	// Identity, when wired (production node role), authenticates
	// the coordinator through the persistent credential document
	// and enables the rotation delivery ops. When nil, the legacy
	// Manager.AuthenticateSelf path is used (tests, combined
	// dev deployments).
	Identity *NodeIdentity
	// Apply receives the pushed Xray JSON and must stage it
	// through the node's own safe-apply path.
	Apply func(config []byte) error
	// Info reports this node's live self-description.
	Info func() NodeInfo
	now  func() time.Time
	// replay caches recently seen nonces (bounded).
	mu     sync.Mutex
	seen   map[string]time.Time
	window time.Duration
	// inFlight bounds concurrent control-plane requests (memory
	// bound for unauthenticated traffic floods); excess gets the
	// uniform 401 without any expensive work.
	inFlight chan struct{}
	// hits is a bounded per-peer fixed-window rate limiter applied
	// BEFORE authentication parsing work.
	hits    map[string]int
	window2 time.Time
	windowN int
}

const (
	// MaxInFlightControl bounds concurrent control-plane requests.
	MaxInFlightControl = 8
	// MaxControlRequestsPerMin bounds per-peer request rate.
	MaxControlRequestsPerMin = 120
)

func NewNodeServer(m *Manager, apply func([]byte) error, info func() NodeInfo) *NodeServer {
	return &NodeServer{Manager: m, Apply: apply, Info: info, now: time.Now,
		seen: map[string]time.Time{}, window: 10 * time.Minute,
		inFlight: make(chan struct{}, MaxInFlightControl),
		hits:     map[string]int{},
	}
}

// allowPeer applies the bounded per-peer rate limit. Peers are the
// immediate TCP address; spoofable proxy headers are ignored.
func (s *NodeServer) allowPeer(peer string) bool {
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	if now.Sub(s.window2) >= time.Minute {
		s.window2 = now
		s.hits = map[string]int{}
	}
	if len(s.hits) >= 4096 {
		// Hard cap on tracked peers (memory bound against a flood
		// of distinct real sources); reset the window instead of
		// growing unboundedly.
		s.hits = map[string]int{}
		s.window2 = now
	}
	s.hits[peer]++
	return s.hits[peer] <= MaxControlRequestsPerMin
}

// SetClock overrides the clock (testing).
func (s *NodeServer) SetClock(fn func() time.Time) { s.now = fn }

func (s *NodeServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != TransportPath || r.Method != http.MethodPost {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	// Bounded resources before any parsing work (R: multi-node
	// resource exhaustion): rate limit, concurrency semaphore,
	// request deadline.
	peer := ""
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		peer = host
	} else {
		peer = r.RemoteAddr
	}
	if !s.allowPeer(peer) {
		uniformControlReject(w)
		return
	}
	select {
	case s.inFlight <- struct{}{}:
		defer func() { <-s.inFlight }()
	default:
		uniformControlReject(w)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	r = r.WithContext(ctx)
	body, err := io.ReadAll(io.LimitReader(r.Body, MaxControlBytes))
	if err != nil {
		uniformControlReject(w)
		return
	}
	var req controlRequest
	if err := json.Unmarshal(body, &req); err != nil {
		uniformControlReject(w)
		return
	}
	// Clock-bounded freshness: rejects captured-traffic replay.
	now := s.now().UTC()
	skew := MaxClockSkew
	if now.Sub(req.Time) > skew || req.Time.Sub(now) > skew {
		uniformControlReject(w)
		return
	}
	// Bounded nonce cache.
	s.mu.Lock()
	if len(s.seen) >= 4096 {
		for n, at := range s.seen {
			if now.Sub(at) > s.window {
				delete(s.seen, n)
			}
		}
	}
	if _, dup := s.seen[req.Nonce]; dup {
		s.mu.Unlock()
		uniformControlReject(w)
		return
	}
	s.seen[req.Nonce] = now
	s.mu.Unlock()

	// B4-01 runtime adoption + F-01 fail-closed: the durable identity
	// document is the coordination point between the duck CLI (which
	// may rotate the credential from another process) and THIS
	// running server. Re-read it before every authentication
	// decision. If the authoritative state cannot be read or decoded
	// (missing, unreadable, corrupt), the request is REJECTED — the
	// stale in-memory credential is never accepted as authoritative,
	// so a completed rotation cannot be undone by a storage fault.
	// Recovery is automatic: as soon as the document is readable
	// again, Reload succeeds and authentication resumes against the
	// authoritative state. No secret material is logged.
	if s.Identity != nil {
		if err := s.Identity.Reload(); err != nil {
			uniformControlReject(w)
			return
		}
	}
	credential := bearer(r)
	if s.Identity != nil {
		if !s.Identity.Accepts(credential) {
			uniformControlReject(w)
			return
		}
	} else if !s.Manager.AuthenticateSelf(credential) {
		uniformControlReject(w)
		return
	}
	resp := controlResponse{OK: true}
	switch req.Op {
	case "discover":
		resp.Body, _ = json.Marshal(s.Info())
	case "health":
		resp.Body = []byte(`{}`)
	case "rotate-credential":
		// Controlled rotation delivery: the new credential arrives
		// over the CURRENT authenticated channel, is persisted as
		// pending, and acknowledged. Commit arrives separately.
		if s.Identity == nil {
			resp.OK, resp.Error = false, "xnode: rotation requires the node identity store"
			break
		}
		var payload struct {
			NewCredential string `json:"newCredential"`
		}
		if err := json.Unmarshal(req.Body, &payload); err != nil || payload.NewCredential == "" {
			resp.OK, resp.Error = false, "xnode: rotate payload is malformed"
			break
		}
		if err := s.Identity.BeginTransition(payload.NewCredential); err != nil {
			resp.OK, resp.Error = false, "xnode: transition rejected by the identity store"
			break
		}
		resp.Body = []byte(`{"acked":true}`)
	case "commit-credential":
		if s.Identity == nil {
			resp.OK, resp.Error = false, "xnode: rotation requires the node identity store"
			break
		}
		var payload struct {
			NewCredential string `json:"newCredential"`
		}
		if err := json.Unmarshal(req.Body, &payload); err != nil || payload.NewCredential == "" {
			resp.OK, resp.Error = false, "xnode: commit payload is malformed"
			break
		}
		if err := s.Identity.Commit(payload.NewCredential); err != nil {
			resp.OK, resp.Error = false, "xnode: commit rejected by the identity store"
			break
		}
	case "push":
		var payload struct {
			Config json.RawMessage `json:"config"`
		}
		if err := json.Unmarshal(req.Body, &payload); err != nil || len(payload.Config) == 0 {
			resp.OK, resp.Error = false, "xnode: push payload is malformed"
			break
		}
		if err := s.Apply(payload.Config); err != nil {
			resp.OK, resp.Error = false, "xnode: apply rejected the configuration"
		}
	default:
		resp.OK, resp.Error = false, "xnode: unknown control op"
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// AdoptIdentity records the shared credential a node received at
// enrollment. Production flow: the node redeems the enrollment
// token against the coordinator's API, receives the credential
// once, and adopts it into its local registry so NodeServer can
// verify coordinator requests. Adoption on a node that already has
// an active identity is refused — re-binding goes through rotation.
func (m *Manager) AdoptIdentity(credential string) error {
	if credential == "" {
		return errors.New("xnode: credential is required")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, n := range m.nodes {
		if n.CredentialHash != "" && n.Status != StatusRevoked {
			return errors.New("xnode: identity already adopted")
		}
	}
	id, err := newID()
	if err != nil {
		return err
	}
	now := m.clock().UTC()
	m.nodes[id] = &Node{
		ID: id, Name: "self", Kind: KindRemote,
		CredentialHash: hashToken("xnode-cred", credential),
		Status:         StatusActive, EnrolledAt: now, LastSeenAt: now,
	}
	return nil
}

// AuthenticateSelf validates a bearer credential against the
// identities this node knows. A node accepts exactly the
// coordinator credential recorded for it at enrollment.
func (m *Manager) AuthenticateSelf(credential string) bool {
	if credential == "" {
		return false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	digest := hashToken("xnode-cred", credential)
	for _, n := range m.nodes {
		if n.CredentialHash != "" && constantTimeEqual(n.CredentialHash, digest) &&
			(n.Status == StatusActive || n.Status == StatusStale) {
			return true
		}
	}
	return false
}

func constantTimeEqual(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	var v byte
	for i := 0; i < len(a); i++ {
		v |= a[i] ^ b[i]
	}
	return v == 0
}

func bearer(r *http.Request) string {
	v := r.Header.Get("Authorization")
	if !strings.HasPrefix(v, "Bearer ") {
		return ""
	}
	return strings.TrimPrefix(v, "Bearer ")
}

func uniformControlReject(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write([]byte(`{"ok":false,"error":"unauthorized"}`))
}

// randHex is a tiny hex helper used by the nonce.
func randHex(n int) string {
	buf := make([]byte, n)
	if err := readRandom(buf); err != nil {
		return "0000000000000000"
	}
	return hex.EncodeToString(buf)
}

// PinningHTTPClient builds a client that pins the server's leaf
// certificate to a sha256 hex fingerprint.
func PinningHTTPClient(sha256Hex string) (*http.Client, error) {
	want, err := hex.DecodeString(sha256Hex)
	if err != nil || len(want) != sha256.Size {
		return nil, errors.New("xnode: fingerprint must be 64 hex characters")
	}
	// TOFU pinning model: the operator-supplied fingerprint IS the
	// trust anchor; the system CA chain is not consulted, so a pin
	// protects even against a mis-issued CA.
	tlsCfg := &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: true,
		VerifyPeerCertificate: func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
			if len(rawCerts) == 0 {
				return errors.New("xnode: no server certificate")
			}
			sum := sha256.Sum256(rawCerts[0])
			if !constantTimeEqual(hex.EncodeToString(sum[:]), hex.EncodeToString(want)) {
				return errors.New("xnode: server certificate fingerprint mismatch")
			}
			return nil
		},
	}
	transport := &http.Transport{TLSClientConfig: tlsCfg}
	return &http.Client{Transport: transport, Timeout: 10 * time.Second}, nil
}
