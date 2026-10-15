// Package subscription / delivery_test.go: security negative and
// positive tests for the public subscription delivery endpoint.
package subscription

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func deliveryFixture(t *testing.T) (*TokenStore, *DeliveryHandler, *[]string) {
	t.Helper()
	served := []string{}
	store := NewTokenStore()
	h := NewDeliveryHandler(store, func(subID string) ([]byte, error) {
		if subID != "live-sub" {
			return nil, errorsNew("subscription: not found")
		}
		served = append(served, subID)
		return []byte("cHJvZmlsZS1saW5rCg=="), nil
	})
	return store, h, &served
}

func errorsNew(s string) error { return &simpleError{s} }

type simpleError struct{ msg string }

func (e *simpleError) Error() string { return e.msg }

func TestDelivery_HappyPath(t *testing.T) {
	store, h, _ := deliveryFixture(t)
	token, rec, err := store.Issue("live-sub", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Revoked || rec.TokenHash == "" {
		t.Fatalf("record malformed: %+v", rec)
	}
	if strings.Contains(rec.TokenHash, token) || token == "" {
		t.Fatal("token digest must not embed the token and token must be non-empty")
	}
	req := httptest.NewRequest(http.MethodGet, "/sub/"+token, nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d body %s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("Content-Type"); got != "text/plain; charset=utf-8" {
		t.Fatalf("content type %q", got)
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("payload must not be cacheable")
	}
}

func TestDelivery_UniformFailure(t *testing.T) {
	store, h, _ := deliveryFixture(t)
	tok, _, _ := store.Issue("gone-sub", time.Hour)
	cases := []string{
		"/sub/unknown-token-value",
		"/sub/" + tok,          // render refuses unknown sub
		"/sub/a/b/" + tok,      // nested path
		"/sub/",                // empty token
		"/admin/dashboard",     // admin-looking surface
		"/api/v1/xray/servers", // API path must not exist here
		"/sub/" + strings.Repeat("x", 200),
	}
	var first string
	for i, path := range cases {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		body := w.Body.String()
		if w.Code != http.StatusNotFound {
			t.Errorf("case %d (%s): status %d", i, path, w.Code)
		}
		if first == "" {
			first = body
		} else if body != first {
			t.Errorf("case %d: non-uniform failure body", i)
		}
	}
	// POST is refused with the same uniform response.
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/sub/anything", nil))
	if w.Code != http.StatusNotFound || w.Body.String() != first {
		t.Fatal("POST failure must be uniform 404")
	}
}

func TestDelivery_RotationAndRevocation(t *testing.T) {
	store, h, _ := deliveryFixture(t)
	oldTok, _, _ := store.Issue("live-sub", time.Hour)
	newTok, _, err := store.Rotate("live-sub", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := store.Lookup(oldTok); ok {
		t.Fatal("old token survived rotation")
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/sub/"+newTok, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("new token failed: %d", w.Code)
	}
	if n, err := store.RevokeSubscription("live-sub"); err != nil || n < 1 {
		t.Fatalf("revocation count %d", n)
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/sub/"+newTok, nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("revoked token still serves: %d", w.Code)
	}
}

func TestDelivery_Expiry(t *testing.T) {
	store := NewTokenStore()
	tok, _, _ := store.Issue("live-sub", -time.Minute) // already expired
	if _, ok := store.Lookup(tok); ok {
		t.Fatal("expired token resolved")
	}
}

func TestDelivery_RateLimit(t *testing.T) {
	store, h, _ := deliveryFixture(t)
	tok, _, _ := store.Issue("live-sub", time.Hour)
	var okStatus, throttled int
	for i := 0; i < 60; i++ {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/sub/"+tok, nil))
		if w.Code == http.StatusOK {
			okStatus++
		} else {
			throttled++
		}
	}
	if okStatus > 30 {
		t.Fatalf("rate limit not enforced: %d ok", okStatus)
	}
	if throttled == 0 {
		t.Fatal("no request was throttled")
	}
}

func TestTokenStore_Concurrent(t *testing.T) {
	store := NewTokenStore()
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tok, _, _ := store.Issue("live-sub", time.Hour)
			store.Lookup(tok)
			store.TokenCount("live-sub")
		}()
	}
	wg.Wait()
}
