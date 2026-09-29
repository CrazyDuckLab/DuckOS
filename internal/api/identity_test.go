package api

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/CrazyDuckLab/DuckOS/internal/identity"
)

func TestIdentityAPIEnrollmentLoginAndRevocation(t *testing.T) {
	h := handler(t, &testEngine{})
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	s, err := identity.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	h.EnableIdentity(s)
	request := func(method, path, token, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "http://127.0.0.1:8080"+path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	if w := request("GET", "/api/v1/auth/status", "", ""); w.Code != 200 || !strings.Contains(w.Body.String(), `"enrolled":false`) {
		t.Fatal(w.Body.String())
	}
	body := `{"username":"owner","password":"Synthetic identity test password 42!"}`
	if w := request("POST", "/api/v1/auth/enroll", "wrong", body); w.Code != 401 {
		t.Fatal(w.Code)
	}
	if w := request("POST", "/api/v1/auth/enroll", testToken, body); w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := request("GET", "/api/v1/overview", testToken, ""); w.Code != 401 {
		t.Fatal("bootstrap still authorized after enrollment")
	}
	w := request("POST", "/api/v1/auth/login", "", body)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var login struct {
		Token string `json:"token"`
	}
	if err = json.Unmarshal(w.Body.Bytes(), &login); err != nil || len(login.Token) != 64 {
		t.Fatal("missing opaque token")
	}
	if w = request("GET", "/api/v1/overview", login.Token, ""); w.Code != 200 {
		t.Fatal(w.Code)
	}
	if w = request("GET", "/api/v1/auth/users", login.Token, ""); w.Code != 200 || strings.Contains(w.Body.String(), "password") || strings.Contains(w.Body.String(), "digest") {
		t.Fatal("unsafe users response")
	}
	if w = request("POST", "/api/v1/auth/logout", login.Token, `{}`); w.Code != 200 {
		t.Fatal(w.Code)
	}
	if w = request("GET", "/api/v1/overview", login.Token, ""); w.Code != 401 {
		t.Fatal("revoked token accepted")
	}
}
func TestIdentityAPIOriginMethodsAndBodyLimits(t *testing.T) {
	h := handler(t, &testEngine{})
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	s, err := identity.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	h.EnableIdentity(s)
	for _, tc := range []struct {
		method, path, origin, content, body string
		want                                int
	}{
		{"POST", "/api/v1/auth/enroll", "https://attacker.test", "application/json", `{}`, 403},
		{"POST", "/api/v1/auth/login", "", "text/plain", `{}`, 415},
		{"POST", "/api/v1/auth/login", "", "application/json", `{"command":"anything"}`, 400},
		{"POST", "/api/v1/auth/login", "", "application/json", `{} {}`, 400},
		{"POST", "/api/v1/auth/login", "", "application/json", strings.Repeat("x", 5000), 400},
		{"GET", "/api/v1/auth/login", "", "application/json", "", 405},
		{"GET", "/api/v1/auth/me?token=secret", "", "application/json", "", 400},
		{"GET", "/api/v1/auth/not-real", "", "application/json", "", 404},
	} {
		r := httptest.NewRequest(tc.method, "http://127.0.0.1:8080"+tc.path, strings.NewReader(tc.body))
		r.Header.Set("Origin", tc.origin)
		r.Header.Set("Content-Type", tc.content)
		r.Header.Set("Authorization", "Bearer "+testToken)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Fatalf("%s: %d want %d", tc.path, w.Code, tc.want)
		}
	}
}
