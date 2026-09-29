package api

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/CrazyDuckLab/DuckOS/internal/clientexport"
	"github.com/CrazyDuckLab/DuckOS/internal/engine"
	"github.com/CrazyDuckLab/DuckOS/internal/identity"
)

type exportProvider func(context.Context, string) (*clientexport.Document, error)

func (f exportProvider) Export(ctx context.Context, id string) (*clientexport.Document, error) {
	return f(ctx, id)
}

func TestClientExportTransportIdentityAndExactPayload(t *testing.T) {
	h := handler(t, &testEngine{})
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	auth, err := identity.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer auth.Close()
	h.EnableIdentity(auth)
	const password = "Synthetic export transport password 42!"
	if _, err := auth.Enroll("owner", password); err != nil {
		t.Fatal(err)
	}
	owner, _, err := auth.Login("owner", password)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := auth.CreateUser(owner, "reader", password, identity.ReadOnly); err != nil {
		t.Fatal(err)
	}
	reader, _, err := auth.Login("reader", password)
	if err != nil {
		t.Fatal(err)
	}
	const id = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	const revision = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	payload := "https://vpn.example.invalid/s?token=synthetic%2Bsecret"
	calls := 0
	service, err := clientexport.NewService(auth, map[string]clientexport.Provider{"subscription": exportProvider(func(context.Context, string) (*clientexport.Document, error) {
		calls++
		return clientexport.SubscriptionURL(id, revision, payload)
	})})
	if err != nil {
		t.Fatal(err)
	}
	e := engine.New("/nonexistent/export-test.sock", "TEST")
	e.EnableClientExports(service)
	h.engine = e
	request := func(token, method, suffix, body, origin string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "http://127.0.0.1:8080/api/v1/clients/export"+suffix, strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("Content-Type", "application/json")
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	const body = `{"id":"` + id + `","kind":"subscription","action":"qr"}`
	for _, token := range []string{testToken, "invalid", reader} {
		w := request(token, "POST", "", body, "")
		if w.Code != 401 && w.Code != 403 {
			t.Fatal("unauthorized export", w.Code)
		}
		if calls != 0 || strings.Contains(w.Body.String(), payload) {
			t.Fatal("unauthorized provider resolution or disclosure")
		}
	}
	for _, action := range []string{"qr", "copy", "download"} {
		w := request(owner, "POST", "", strings.Replace(body, `"qr"`, `"`+action+`"`, 1), "")
		var got struct {
			Payload  string `json:"payload"`
			Filename string `json:"filename"`
		}
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &got) != nil || got.Payload != payload || got.Filename != "subscription.txt" {
			t.Fatal("export transport mismatch", w.Code)
		}
		if w.Header().Get("Cache-Control") != "no-store, private" || w.Header().Get("Referrer-Policy") != "no-referrer" {
			t.Fatal("missing secret response headers")
		}
		payload += "-rotated"
	}
	for _, item := range []struct{ method, suffix, body, origin string }{
		{"GET", "", "", ""},
		{"POST", "?token=forbidden", body, ""},
		{"POST", "", body, "https://evil.example.invalid"},
		{"POST", "", `{"id":"` + id + `","kind":"subscription","action":"qr","secret":"unexpected"}`, ""},
	} {
		w := request(owner, item.method, item.suffix, item.body, item.origin)
		if w.Code < 400 || calls != 3 {
			t.Fatal("invalid request reached provider", w.Code)
		}
	}
}
