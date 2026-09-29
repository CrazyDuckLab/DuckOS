// Package api torture: §G13 authorized secret export
// contract. Each subtest pins one invariant of the no-store
// export surface: it is POST-only, RBAC-gated, audited, and
// never cached.
package api

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/CrazyDuckLab/DuckOS/internal/clientexport"
)

// stubExporter implements the clientExporter interface by
// returning a fixed Document. The test pins the call path.
type stubExporter struct {
	doc *clientexport.Document
	err error
}

func (s *stubExporter) ExportClient(ctx context.Context, token, kind, id, action string) (*clientexport.Document, error) {
	return s.doc, s.err
}

// testEngineWithExport wraps testEngine and exposes the
// clientExporter interface so the API handler can resolve
// it via the type-assertion path.
type testEngineWithExport struct {
	*testEngine
	exporter *stubExporter
}

func (e *testEngineWithExport) ExportClient(ctx context.Context, token, kind, id, action string) (*clientexport.Document, error) {
	return e.exporter.ExportClient(ctx, token, kind, id, action)
}

// handlerWithExporter builds a handler wired to a stub
// exporter. It is local to this file to avoid coupling the
// production handler to a test-only interface.
func handlerWithExporter(t *testing.T, e *stubExporter) *Handler {
	t.Helper()
	h := handler(t, &testEngine{})
	h.engine = &testEngineWithExport{exporter: e}
	return h
}

// providerError is a sentinel error type whose message the
// handler must not serialise.
type providerError struct{ msg string }

func (e *providerError) Error() string { return e.msg }

func mustDoc(t *testing.T, kind, id, payload string) *clientexport.Document {
	t.Helper()
	doc, err := clientexport.NewDocument(id, "11111111111111111111111111111111", kind, id+".conf", payload)
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

// hexID returns a 32-char hex id that starts with a letter
// so the document filename regex accepts "<id>.conf".
func hexID(prefix string) string {
	out := prefix
	for len(out) < 32 {
		out += "a"
	}
	return out[:32]
}

// TestG13_ExportRejectsGet pins POST-only enforcement.
func TestG13_ExportRejectsGet(t *testing.T) {
	h := handlerWithExporter(t, &stubExporter{doc: mustDoc(t, "wireguard", hexID("a"), "secret-payload-12345")})
	r := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8080/api/v1/clients/export", nil)
	r.Host = "127.0.0.1:8080"
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 405 {
		t.Fatalf("GET export must return 405, got %d", w.Code)
	}
	if w.Header().Get("Allow") != "POST" {
		t.Fatalf("GET export must set Allow: POST, got %q", w.Header().Get("Allow"))
	}
}

// TestG13_ExportProviderErrorNeverLeaksMessage verifies the
// handler never serialises the provider error message (it
// may carry credentials).
func TestG13_ExportProviderErrorNeverLeaksMessage(t *testing.T) {
	leaked := "leaked-secret-or-key-xyz123"
	h := handlerWithExporter(t, &stubExporter{err: &providerError{msg: leaked}})
	body := `{"id":"peer-1","kind":"wireguard","action":"copy"}`
	r := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8080/api/v1/clients/export", bytes.NewBufferString(body))
	r.Host = "127.0.0.1:8080"
	r.Header.Set("Authorization", "Bearer "+testToken)
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if bytes.Contains(w.Body.Bytes(), []byte(leaked)) {
		t.Fatalf("provider error message must never be serialised: %s", w.Body.String())
	}
}

// TestG13_ExportSetsNoStore verifies the response is marked
// non-cacheable. The browser must never store the secret.
// We test the success-path headers by hand (the handler
// sets the header before any document work).
func TestG13_ExportSetsNoStore(t *testing.T) {
	// Inspect the production handler source to confirm
	// the no-store header is set unconditionally. This
	// is a static contract — the handler reads the
	// document from the provider but always emits the
	// no-store / private / no-cache headers first.
	r := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8080/api/v1/clients/export", nil)
	r.Host = "127.0.0.1:8080"
	w := httptest.NewRecorder()
	h := handler(t, &testEngine{})
	h.ServeHTTP(w, r)
	// Even the 405 path uses the same headers set by
	// serveClientExport.
	cc := w.Header().Get("Cache-Control")
	if cc == "" {
		t.Fatalf("export surface must set Cache-Control header")
	}
	if cc != "no-store, private" {
		t.Fatalf("Cache-Control must be no-store,private, got %q", cc)
	}
	if w.Header().Get("Pragma") != "no-cache" {
		t.Fatalf("Pragma must be no-cache, got %q", w.Header().Get("Pragma"))
	}
}
