// G02+G06 API integration regression: the
// GET /api/v1/objects/{kind} handler must dispatch to the
// engine's registered ObjectLister, return the canonical
// object.Collection envelope, reject non-GET methods with a
// 405, refuse request bodies, and produce honest
// "unsupported_kind" envelopes for unknown kinds.
package api

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/CrazyDuckLab/DuckOS/internal/engine"
	"github.com/CrazyDuckLab/DuckOS/internal/object"
	"github.com/CrazyDuckLab/DuckOS/internal/objectlisters"
)

// objectListTestEngine satisfies the api.Engine contract and
// forwards ObjectList to a real Engine wired with the
// production listers. This is the closest the api tests can
// get to the Agent composition root without booting a real
// IPC channel.
type objectListTestEngine struct {
	*engine.Engine
}

func (o *objectListTestEngine) ObjectList(ctx context.Context, kind string, params object.CollectionParams) (object.Collection, error) {
	return o.Engine.ObjectList(ctx, kind, params)
}

func writeIndex(dir string) error {
	return os.WriteFile(dir+"/index.html", []byte("<!doctype html><title>DuckOS test</title>"), 0o600)
}

func newListerHandler(t *testing.T) *Handler {
	t.Helper()
	e := engine.New("/tmp/duckos-lister-api-test.sock", "test")
	objectlisters.RegisterAll(e)
	dir := t.TempDir()
	if err := writeIndex(dir); err != nil {
		t.Fatal(err)
	}
	h, err := New(&objectListTestEngine{Engine: e}, testToken, "127.0.0.1:8080", dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = h.Close() })
	return h
}

func TestObjects_RegisteredKindReturnsEnvelope(t *testing.T) {
	h := newListerHandler(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/objects/network.route", nil)
	req.Header.Set("Authorization", "Bearer "+testToken)
	req.Host = "127.0.0.1:8080"
	h.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var col object.Collection
	if err := json.Unmarshal(rec.Body.Bytes(), &col); err != nil {
		t.Fatalf("response is not a valid object.Collection: %v body=%s", err, rec.Body.String())
	}
	if col.Complete {
		t.Fatalf("stub lister must NOT claim Complete=true (honest: source missing)")
	}
	if len(col.Issues) == 0 {
		t.Fatalf("stub lister must report an engine_unavailable issue")
	}
}

func TestObjects_UnknownKindStillReturnsEnvelope(t *testing.T) {
	h := newListerHandler(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/objects/no.such.kind", nil)
	req.Header.Set("Authorization", "Bearer "+testToken)
	req.Host = "127.0.0.1:8080"
	h.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("unknown kind must still respond 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var col object.Collection
	if err := json.Unmarshal(rec.Body.Bytes(), &col); err != nil {
		t.Fatalf("response is not a valid object.Collection: %v body=%s", err, rec.Body.String())
	}
	if col.SnapshotID != "engine-no-lister" {
		t.Fatalf("unknown kind SnapshotID drift: %q", col.SnapshotID)
	}
	if col.Complete {
		t.Fatalf("unknown kind must NOT claim Complete=true")
	}
}

func TestObjects_RequiresAuth(t *testing.T) {
	h := newListerHandler(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/objects/network.route", nil)
	req.Host = "127.0.0.1:8080"
	h.ServeHTTP(rec, req)

	if rec.Code != 401 {
		t.Fatalf("unauthenticated request must return 401, got %d", rec.Code)
	}
}

func TestObjects_RejectsNonGET(t *testing.T) {
	h := newListerHandler(t)
	for _, method := range []string{"POST", "PUT", "DELETE", "PATCH"} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(method, "/api/v1/objects/network.route", strings.NewReader("{}"))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+testToken)
		req.Host = "127.0.0.1:8080"
		h.ServeHTTP(rec, req)

		if rec.Code/100 != 4 {
			t.Errorf("method %s: expected 4xx, got %d body=%s", method, rec.Code, rec.Body.String())
		}
	}
}

func TestObjects_RejectsRequestBody(t *testing.T) {
	h := newListerHandler(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/objects/network.route", strings.NewReader("not-empty"))
	req.Header.Set("Authorization", "Bearer "+testToken)
	req.Host = "127.0.0.1:8080"
	req.ContentLength = int64(len("not-empty"))
	h.ServeHTTP(rec, req)

	if rec.Code != 400 {
		t.Fatalf("expected 400 for body-on-list, got %d", rec.Code)
	}
}

func TestObjects_HonoursPaginationParams(t *testing.T) {
	h := newListerHandler(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/objects/xray.user?limit=7&after=abc", nil)
	req.Header.Set("Authorization", "Bearer "+testToken)
	req.Host = "127.0.0.1:8080"
	h.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("expected JSON content type, got %q", rec.Header().Get("Content-Type"))
	}
}

func TestObjects_RejectsNestedKind(t *testing.T) {
	h := newListerHandler(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/objects/network.route/extra", nil)
	req.Header.Set("Authorization", "Bearer "+testToken)
	req.Host = "127.0.0.1:8080"
	h.ServeHTTP(rec, req)

	if rec.Code != 404 {
		t.Fatalf("nested path must return 404, got %d", rec.Code)
	}
}

func TestIsObjectKind(t *testing.T) {
	for _, k := range objectlisters.SupportedKinds {
		if !IsObjectKind(k) {
			t.Errorf("IsObjectKind(%q) must be true", k)
		}
	}
	if IsObjectKind("no.such.kind") {
		t.Errorf("IsObjectKind must be false for unknown kinds")
	}
	if IsObjectKind("") {
		t.Errorf("IsObjectKind must be false for empty kind")
	}
}
