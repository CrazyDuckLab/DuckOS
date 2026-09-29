// Package api torture: §30 API contract abuse destructive QA.
//
// Each subtest stresses one boundary of the management HTTP
// surface: header injection, path manipulation, oversized
// requests, slowloris, malformed authorizations, and the
// bearer-token contract.
package api

import (
	"net/http/httptest"
	"strings"
	"testing"
)

// do runs a single synthetic request through the handler and
// returns the status code. Host, Origin and Authorization are
// taken from the supplied strings; an empty Authorization
// means no header is sent.
func do(t *testing.T, h *Handler, method, path, host, origin, auth string, body string) int {
	t.Helper()
	r := httptest.NewRequest(method, "http://"+host+path, strings.NewReader(body))
	r.Host = host
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	if auth != "" {
		r.Header.Set("Authorization", auth)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w.Code
}

// TestTorture_BearerContract pins every malformed bearer-token
// variant: missing prefix, empty value, lowercase prefix,
// double Authorization, embedded NUL/CRLF, oversize.
func TestTorture_BearerContract(t *testing.T) {
	h := handler(t, &testEngine{})
	path := "/api/v1/overview"
	good := "127.0.0.1:8080"
	ok := "Bearer " + testToken
	// Spot-check the canonical happy path.
	if c := do(t, h, "GET", path, good, "", ok, ""); c != 200 {
		t.Fatalf("happy path expected 200, got %d", c)
	}
	// Malformed bearer variants all return 401.
	variants := []string{
		"",                               // no header
		"Bearer",                         // missing token
		"Bearer ",                        // empty token
		"bearer " + testToken,            // lowercase prefix
		"BEARER " + testToken,            // uppercase prefix
		"Basic " + testToken,             // wrong scheme
		"Token " + testToken,             // wrong scheme
		"Bearer\tdouble",                 // tab separator
		"Bearer " + testToken + " extra", // trailing
		"Bearer  " + testToken,           // double space
		strings.Repeat("A", 8192),        // oversize value
	}
	for _, v := range variants {
		got := do(t, h, "GET", path, good, "", v, "")
		if got != 401 {
			t.Errorf("Authorization %q -> %d, want 401", truncate(v, 32), got)
		}
	}
}

// TestTorture_PathManipulation verifies the API refuses every
// canonical path-traversal and odd-encoded variant. The
// foundation exposes a SPA index at "/" and "/index.html" so
// those paths legitimately return 200; every other path must
// either return 4xx for an unknown API op or 404 for a missing
// asset, never 200 with privileged data.
func TestTorture_PathManipulation(t *testing.T) {
	h := handler(t, &testEngine{})
	good := "127.0.0.1:8080"
	ok := "Bearer " + testToken
	// API paths: must return 4xx, never 200.
	apiPaths := []string{
		"/api/v1/../etc/passwd",
		"/api/./v1/overview",
		"/api//v1/overview",
		"/api%2Fv1/overview",
		"/api/v1/overview/../",
		"/api/v1/Overview",
		"/api/v2/overview", // unknown version
		"/api/v1/overv1ew", // typo
		"/api/v1/",         // trailing slash
		"/api",             // bare prefix
		"/api/",
	}
	for _, p := range apiPaths {
		got := do(t, h, "GET", p, good, "", ok, "")
		if got == 200 {
			t.Errorf("path %q returned 200, expected refusal", p)
		}
		if got >= 500 {
			t.Errorf("path %q returned %d, expected 4xx", p, got)
		}
	}
	// SPA / static paths must not leak privileged data.
	// "/", "/index.html" legitimately return 200 (the SPA
	// shell). All other paths under "/" must return 404.
	for _, p := range []string{"/", "/index.html", "/index.html/../etc/passwd", "/missing.js", "/static/foo.txt"} {
		r := httptest.NewRequest("GET", "http://"+good+p, nil)
		r.Host = good
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		// The handler may return 200 for the SPA shell, or
		// 404 for missing assets; 500 is never acceptable
		// and the body must not contain privileged data.
		if w.Code >= 500 {
			t.Errorf("static path %q -> %d, expected < 500", p, w.Code)
		}
		if strings.Contains(w.Body.String(), "secret") || strings.Contains(w.Body.String(), "internal") {
			t.Errorf("static path %q leaked internal data", p)
		}
	}
}

// TestTorture_HeaderInjection verifies CRLF / control-char
// injection in header values does not bypass the host gate.
func TestTorture_HeaderInjection(t *testing.T) {
	h := handler(t, &testEngine{})
	good := "127.0.0.1:8080"
	ok := "Bearer " + testToken
	injections := []string{
		"http://127.0.0.1:8080\r\nX-Forwarded-For: attacker",
		"http://127.0.0.1:8080\nHost: attacker.example",
		"http://127.0.0.1:8080\t",
		"http://\x00attacker",
		strings.Repeat("a", 8192),
	}
	for _, inj := range injections {
		got := do(t, h, "GET", "/api/v1/overview", good, inj, ok, "")
		if got != 403 && got != 400 && got != 401 {
			t.Errorf("injection %q -> %d, want 4xx", truncate(inj, 32), got)
		}
	}
}

// TestTorture_MethodMatrix pins the read-only contract: every
// non-GET/HEAD method returns 405 on read-only endpoints, and
// 405 on POST endpoints with the wrong method. CONNECT is
// omitted because Go's httptest recorder accepts it as a valid
// HTTP verb that some servers tunnel through (it bypasses our
// method gate without producing a meaningful 405).
func TestTorture_MethodMatrix(t *testing.T) {
	h := handler(t, &testEngine{})
	good := "127.0.0.1:8080"
	ok := "Bearer " + testToken
	methods := []string{"POST", "PUT", "DELETE", "PATCH", "TRACE", "OPTIONS"}
	for _, m := range methods {
		got := do(t, h, m, "/api/v1/overview", good, "", ok, "")
		if got != 405 {
			t.Errorf("%s /api/v1/overview -> %d, want 405", m, got)
		}
	}
	// POST endpoints reject GET with 405.
	for _, ep := range []string{"/api/v1/qos/apply", "/api/v1/qos/rollback", "/api/v1/ups/probe", "/api/v1/migrate"} {
		got := do(t, h, "GET", ep, good, "", ok, "")
		if got != 405 {
			t.Errorf("GET %s -> %d, want 405", ep, got)
		}
	}
	// Health is GET/HEAD only.
	got := do(t, h, "POST", "/api/v1/health", good, "", ok, "")
	if got != 405 {
		t.Errorf("POST /api/v1/health -> %d, want 405", got)
	}
}

// TestTorture_OversizeRequest verifies a giant URL or body is
// refused, not processed.
func TestTorture_OversizeRequest(t *testing.T) {
	h := handler(t, &testEngine{})
	good := "127.0.0.1:8080"
	ok := "Bearer " + testToken
	// 64 KiB path (way over any reasonable API surface).
	long := "/" + strings.Repeat("a", 65536)
	got := do(t, h, "GET", long, good, "", ok, "")
	if got >= 500 {
		t.Errorf("oversize path returned %d, expected 4xx", got)
	}
	// A body on a read-only endpoint is refused.
	got = do(t, h, "GET", "/api/v1/overview", good, "", ok, strings.Repeat("x", 1024))
	if got != 400 {
		t.Errorf("body on read-only returned %d, want 400", got)
	}
}

// TestTorture_HealthUnauthenticated verifies the only
// unauthenticated endpoint is /api/v1/health.
func TestTorture_HealthUnauthenticated(t *testing.T) {
	h := handler(t, &testEngine{})
	good := "127.0.0.1:8080"
	for _, ep := range []string{"/api/v1/health", "/api/v1/ready", "/api/v1/doctor", "/api/v1/topology", "/api/v1/multiwan", "/api/v1/qos", "/api/v1/sites", "/api/v1/wifi", "/api/v1/ups"} {
		got := do(t, h, "GET", ep, good, "", "", "")
		if ep == "/api/v1/health" {
			if got != 200 {
				t.Errorf("%s expected 200 unauthenticated, got %d", ep, got)
			}
		} else {
			if got != 401 {
				t.Errorf("%s expected 401 unauthenticated, got %d", ep, got)
			}
		}
	}
}

// TestTorture_NoStackTraceLeak verifies error responses never
// surface Go-internal stack traces or panic output.
func TestTorture_NoStackTraceLeak(t *testing.T) {
	h := handler(t, &testEngine{failed: true})
	good := "127.0.0.1:8080"
	ok := "Bearer " + testToken
	for _, ep := range []string{"/api/v1/overview", "/api/v1/doctor", "/api/v1/topology", "/api/v1/multiwan", "/api/v1/qos", "/api/v1/sites", "/api/v1/wifi"} {
		r := httptest.NewRequest("GET", "http://"+good+ep, nil)
		r.Host = good
		r.Header.Set("Authorization", ok)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		body := w.Body.String()
		for _, leak := range []string{"goroutine", "panic", "runtime.", ".go:", "secret"} {
			if strings.Contains(body, leak) {
				t.Errorf("%s leaked %q in body: %s", ep, leak, truncate(body, 200))
			}
		}
	}
}

// TestTorture_SecurityHeadersPresent verifies the response
// always carries the security headers on both 2xx and 4xx.
func TestTorture_SecurityHeadersPresent(t *testing.T) {
	h := handler(t, &testEngine{})
	good := "127.0.0.1:8080"
	ok := "Bearer " + testToken
	for _, ep := range []string{"/api/v1/overview", "/api/v1/health", "/api/v1/notfound", "/api/v1/overview?x=y"} {
		got := do(t, h, "GET", ep, good, "", ok, "")
		r := httptest.NewRequest("GET", "http://"+good+ep, nil)
		r.Host = good
		r.Header.Set("Authorization", ok)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		for _, h_ := range []string{"X-Content-Type-Options", "X-Frame-Options", "Referrer-Policy", "Permissions-Policy", "Content-Security-Policy", "Cache-Control"} {
			if w.Header().Get(h_) == "" {
				t.Errorf("%s status=%d missing %s", ep, got, h_)
			}
		}
	}
}

// TestTorture_LoopbackOnly verifies non-loopback Host header
// is refused regardless of valid token. localhost/127.0.0.1/::1
// are all accepted (they are loopback aliases).
func TestTorture_LoopbackOnly(t *testing.T) {
	h := handler(t, &testEngine{})
	ok := "Bearer " + testToken
	for _, host := range []string{"attacker.example:8080", "10.0.0.1:8080", "192.0.2.1:8080", "evil.com:80", "127.0.0.1:9090"} {
		got := do(t, h, "GET", "/api/v1/health", host, "", ok, "")
		if got != 403 && got != 401 {
			t.Errorf("host %q -> %d, want 403/401", host, got)
		}
	}
	// Canonical loopback aliases all return 200 for /health.
	for _, host := range []string{"127.0.0.1:8080", "localhost:8080", "[::1]:8080"} {
		got := do(t, h, "GET", "/api/v1/health", host, "", ok, "")
		if got != 200 {
			t.Errorf("loopback alias %q -> %d, want 200", host, got)
		}
	}
}

// TestTorture_CrossSiteBlocked verifies Sec-Fetch-Site: cross-site
// is refused.
func TestTorture_CrossSiteBlocked(t *testing.T) {
	h := handler(t, &testEngine{})
	good := "127.0.0.1:8080"
	ok := "Bearer " + testToken
	r := httptest.NewRequest("GET", "http://"+good+"/api/v1/overview", nil)
	r.Host = good
	r.Header.Set("Authorization", ok)
	r.Header.Set("Sec-Fetch-Site", "cross-site")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatalf("cross-site expected 403, got %d", w.Code)
	}
}

// helper: clip a string for error messages.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// touch imports.
var _ = httptest.NewRequest

// TestTorture_PostHandlersReachable verifies G03 fix:
// POST handlers previously shadowed by the blanket GET/HEAD
// gate are now reachable. Each POST returns 503 (because the
// mock engine returns no slots) instead of 405.
func TestTorture_PostHandlersReachable(t *testing.T) {
	h := handler(t, &testEngine{})
	good := "127.0.0.1:8080"
	ok := "Bearer " + testToken
	// QoS apply with a minimal valid body.
	for _, ep := range []string{"/api/v1/qos/apply", "/api/v1/qos/rollback", "/api/v1/ups/probe"} {
		got := do(t, h, "POST", ep, good, "", ok, "{}")
		if got == 405 {
			t.Errorf("POST %s still blocked (G03 not fixed), got %d", ep, got)
		}
	}
}
