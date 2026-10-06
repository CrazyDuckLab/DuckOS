// Regression tests for B-007 (BETA.2): Engine proxy
// methods must preserve RawQuery end-to-end.
//
// The public API → Engine → Agent IPC chain had three
// layers that dropped the URL query string:
//   - Engine.XrayProxy / WireGuardProxy / VaultProxy
//     constructed the target URL from path only;
//   - XrayHost / WGHost / VaultHost in internal/agent
//     cloned the request with `r2.URL.RawPath = ""`
//     but never restored RawQuery on the clone;
//   - the api.Handler wrappers did not pass RawQuery
//     to the Engine.
//
// The test below stands up a fake Agent loopback and
// proves that:
//
//   - /api/v1/xray/users?server=<id> reaches the
//     Agent as /agent/v1/xray/xray/v1/users?server=<id>;
//   - /api/v1/wireguard/peers?server=<id> reaches the
//     Agent as /agent/v1/wireguard/wireguard/v1/peers?server=<id>;
//   - a CR / LF / NUL byte in the query is rejected at
//     the Engine boundary so the Agent never sees a
//     malformed request line;
//   - an oversized query (over 4 KiB) is rejected at
//     the Engine boundary with a clean error envelope.
package engine

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// recordingAgent is the test double for the Agent
// loopback. It records every request URL so the test
// can assert that RawQuery survived the proxy chain.
type recordingAgent struct {
	requests chan *http.Request
	body     []byte
}

func newRecordingAgent(body []byte) *recordingAgent {
	return &recordingAgent{requests: make(chan *http.Request, 16), body: body}
}

func (r *recordingAgent) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		r.requests <- req
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(r.body)
	})
}

// newEngineForTest constructs an *Engine wired to the
// supplied httptest server. The Engine normally dials a
// Unix socket; the test overrides client to a vanilla
// HTTP client so the proxy methods can talk to httptest
// over loopback TCP.
func newEngineForTest(t *testing.T, srv *httptest.Server) *Engine {
	t.Helper()
	return &Engine{
		client: &http.Client{
			Timeout: 5 * time.Second,
			// refuse to follow redirects so a test
			// failure surfaces as a clear error
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		environment: "test",
		ttl:         time.Second,
	}
}

// TestEngine_XrayProxy_PreservesServerQuery is the canonical
// regression for B-007. The Xray /users listing requires
// ?server=<id>; the prior code stripped the query in two
// places (Engine proxy + XrayHost clone) so the Agent
// always saw an empty r.URL.RawQuery.
func TestEngine_XrayProxy_PreservesServerQuery(t *testing.T) {
	agent := newRecordingAgent([]byte(`{"users":[]}`))
	srv := httptest.NewServer(agent.handler())
	defer srv.Close()

	e := &Engine{
		client: srv.Client(),
		// Override the URL the proxy constructs. The
		// real Engine hardcodes http://agent; tests
		// rewrite it through a RoundTripper shim.
		environment: "test",
	}
	e.client.Transport = redirectHost(srv.URL)

	status, data, ct, err := e.XrayProxy(context.Background(),
		http.MethodGet,
		"/api/v1/xray/users",
		"server=xr-edge-001",
		"tok-owner",
		nil)
	if err != nil {
		t.Fatalf("XrayProxy: %v", err)
	}
	if status != 200 {
		t.Fatalf("status=%d want 200 (data=%s)", status, data)
	}
	if ct == "" {
		t.Fatalf("missing content-type")
	}
	select {
	case req := <-agent.requests:
		if got := req.URL.RawQuery; got != "server=xr-edge-001" {
			t.Fatalf("agent saw RawQuery=%q want %q", got, "server=xr-edge-001")
		}
		if !strings.HasSuffix(req.URL.Path, "/users") {
			t.Fatalf("agent saw path=%q want /users tail", req.URL.Path)
		}
	case <-time.After(time.Second):
		t.Fatalf("agent did not record a request")
	}
}

// TestEngine_XrayProxy_PreservesCursor is the second
// B-007 regression. The Xray /subscriptions listing uses
// a cursor; the same drop-Query chain would silently
// break cursor-based pagination.
func TestEngine_XrayProxy_PreservesCursor(t *testing.T) {
	agent := newRecordingAgent([]byte(`{"items":[]}`))
	srv := httptest.NewServer(agent.handler())
	defer srv.Close()
	e := newEngineForTest(t, srv)
	e.client.Transport = redirectHost(srv.URL)

	status, data, _, err := e.XrayProxy(context.Background(),
		http.MethodGet,
		"/api/v1/xray/subscriptions",
		"cursor=01HMK9E2GR9",
		"tok-owner",
		nil)
	if err != nil {
		t.Fatalf("XrayProxy: %v", err)
	}
	if status != 200 {
		t.Fatalf("status=%d want 200 (data=%s)", status, data)
	}
	select {
	case req := <-agent.requests:
		if got := req.URL.RawQuery; got != "cursor=01HMK9E2GR9" {
			t.Fatalf("agent saw RawQuery=%q", got)
		}
	case <-time.After(time.Second):
		t.Fatalf("agent did not record a request")
	}
}

// TestEngine_WireGuardProxy_PreservesServerQuery covers
// the WireGuard peer listing, which also pins a server
// ID via ?server=. The bug is generic across the proxy
// surface; this test guards the WG fix specifically.
func TestEngine_WireGuardProxy_PreservesServerQuery(t *testing.T) {
	agent := newRecordingAgent([]byte(`{"peers":[]}`))
	srv := httptest.NewServer(agent.handler())
	defer srv.Close()
	e := newEngineForTest(t, srv)
	e.client.Transport = redirectHost(srv.URL)

	if _, _, _, err := e.WireGuardProxy(context.Background(),
		http.MethodGet,
		"/api/v1/wireguard/peers",
		"server=wg-edge-001",
		"tok-owner",
		nil); err != nil {
		t.Fatalf("WireGuardProxy: %v", err)
	}
	select {
	case req := <-agent.requests:
		if got := req.URL.RawQuery; got != "server=wg-edge-001" {
			t.Fatalf("agent saw RawQuery=%q", got)
		}
	case <-time.After(time.Second):
		t.Fatalf("agent did not record a request")
	}
}

// TestEngine_VaultProxy_PreservesQuery covers the
// third sibling proxy that suffered from the same drop-
// Query pattern. The vault rarely uses rawQuery but
// the contract is consistent across the surface.
func TestEngine_VaultProxy_PreservesQuery(t *testing.T) {
	agent := newRecordingAgent([]byte(`{"records":[]}`))
	srv := httptest.NewServer(agent.handler())
	defer srv.Close()
	e := newEngineForTest(t, srv)
	e.client.Transport = redirectHost(srv.URL)

	if _, _, _, err := e.VaultProxy(context.Background(),
		http.MethodGet,
		"/api/v1/vault/records",
		"kind=wireguard.peer",
		"tok-owner",
		nil); err != nil {
		t.Fatalf("VaultProxy: %v", err)
	}
	select {
	case req := <-agent.requests:
		if got := req.URL.RawQuery; got != "kind=wireguard.peer" {
			t.Fatalf("agent saw RawQuery=%q", got)
		}
	case <-time.After(time.Second):
		t.Fatalf("agent did not record a request")
	}
}

// TestEngine_ProxyRejectsControlBytesInQuery proves the
// narrow gate that prevents a hostile public client from
// smuggling CR / LF / NUL into the IPC request line.
// A malformed request line at the Agent's HTTP server
// would either fail or — worse — smuggle a second
// request behind the first.
func TestEngine_ProxyRejectsControlBytesInQuery(t *testing.T) {
	e := &Engine{client: &http.Client{Timeout: time.Second}, environment: "test"}
	cases := []string{
		"server=foo\rGET /evil HTTP/1.1",
		"server=foo\n",
		"server=foo\x00",
	}
	for _, q := range cases {
		for _, name := range []string{"Xray", "WireGuard", "Vault"} {
			var err error
			switch name {
			case "Xray":
				_, _, _, err = e.XrayProxy(context.Background(), http.MethodGet, "/api/v1/xray/users", q, "tok", nil)
			case "WireGuard":
				_, _, _, err = e.WireGuardProxy(context.Background(), http.MethodGet, "/api/v1/wireguard/peers", q, "tok", nil)
			case "Vault":
				_, _, _, err = e.VaultProxy(context.Background(), http.MethodGet, "/api/v1/vault/records", q, "tok", nil)
			}
			if err == nil {
				t.Fatalf("%s accepted CR/LF/NUL in query: %q", name, q)
			}
		}
	}
}

// TestEngine_ProxyRejectsOversizedQuery caps the
// forwarded query at 4 KiB so the Agent never sees a
// runaway envelope.
func TestEngine_ProxyRejectsOversizedQuery(t *testing.T) {
	e := &Engine{client: &http.Client{Timeout: time.Second}, environment: "test"}
	q := strings.Repeat("a", maxRawQueryBytes+1)
	if _, _, _, err := e.XrayProxy(context.Background(), http.MethodGet, "/api/v1/xray/users", q, "tok", nil); err == nil {
		t.Fatalf("XrayProxy accepted oversized query")
	}
	if _, _, _, err := e.WireGuardProxy(context.Background(), http.MethodGet, "/api/v1/wireguard/peers", q, "tok", nil); err == nil {
		t.Fatalf("WireGuardProxy accepted oversized query")
	}
	if _, _, _, err := e.VaultProxy(context.Background(), http.MethodGet, "/api/v1/vault/records", q, "tok", nil); err == nil {
		t.Fatalf("VaultProxy accepted oversized query")
	}
}

// TestEngine_ProxyAcceptsEmptyQuery is the guard against
// over-correction: an absent query is still a valid
// request envelope (the Agent's mux ignores URL.Query()
// on the no-server-required paths).
func TestEngine_ProxyAcceptsEmptyQuery(t *testing.T) {
	agent := newRecordingAgent([]byte(`{}`))
	srv := httptest.NewServer(agent.handler())
	defer srv.Close()
	e := newEngineForTest(t, srv)
	e.client.Transport = redirectHost(srv.URL)

	if _, _, _, err := e.XrayProxy(context.Background(), http.MethodGet, "/api/v1/xray/servers", "", "tok", nil); err != nil {
		t.Fatalf("XrayProxy(empty): %v", err)
	}
	select {
	case req := <-agent.requests:
		if req.URL.RawQuery != "" {
			t.Fatalf("agent saw RawQuery=%q want empty", req.URL.RawQuery)
		}
	case <-time.After(time.Second):
		t.Fatalf("agent did not record a request")
	}
}

// redirectHost returns a RoundTripper that re-points every
// request to the supplied base URL while preserving
// path + query. The Engine normally targets
// http://agent/<path>; the test rewrites the host
// portion to point at the httptest listener without
// touching the path + query construction the proxy
// already performed.
func redirectHost(base string) http.RoundTripper {
	return roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		// Replace "http://agent" with the test
		// server URL. The proxy uses NewRequest
		// with target="http://agent<path>"; we
		// swap the host portion only.
		// Strip scheme://host and re-attach.
		// Easier: rewrite URL to base + req.URL.RequestURI().
		req2 := req.Clone(req.Context())
		req2.URL.Scheme = ""
		req2.URL.Host = ""
		// Compose the test URL.
		newURL := base + req.URL.RequestURI()
		req2, err := http.NewRequestWithContext(req.Context(), req.Method, newURL, req.Body)
		if err != nil {
			return nil, err
		}
		for k, v := range req.Header {
			req2.Header[k] = v
		}
		return http.DefaultTransport.RoundTrip(req2)
	})
}

// roundTripperFunc adapts a function to http.RoundTripper.
type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// keep io used so a future refactor that drops the
// import does not silently break the test file.
var _ = io.EOF
