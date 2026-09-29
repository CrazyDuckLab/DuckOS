package api

import (
	"crypto/ed25519"
	"crypto/rand"
	"github.com/CrazyDuckLab/DuckOS/internal/healthauth"
	"github.com/CrazyDuckLab/DuckOS/internal/identity"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestUpdaterReadinessEnrolledProductionAuth(t *testing.T) {
	engine := &testEngine{}
	h := handler(t, engine)
	pub, key, _ := ed25519.GenerateKey(rand.Reader)
	h.EnableUpdaterHealthKey(pub)
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
	if _, err := auth.Enroll("owner", "Synthetic enrolled health test 2026!"); err != nil {
		t.Fatal(err)
	}
	session, _, err := auth.Login("owner", "Synthetic enrolled health test 2026!")
	if err != nil {
		t.Fatal(err)
	}
	call := func(path, token, remote string) int {
		r := httptest.NewRequest("GET", "http://127.0.0.1:8080"+path, nil)
		r.RemoteAddr = remote
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w.Code
	}
	assertion := healthauth.Sign(key, time.Now())
	_, wrong, _ := ed25519.GenerateKey(rand.Reader)
	for _, tc := range []struct {
		name, path, token, remote string
		want                      int
	}{
		{"session readiness", healthauth.Path, session, "127.0.0.1:1000", 200},
		{"session management", "/api/v1/overview", session, "127.0.0.1:1000", 200},
		{"bootstrap readiness", healthauth.Path, testToken, "127.0.0.1:1000", 401},
		{"bootstrap management", "/api/v1/overview", testToken, "127.0.0.1:1000", 401},
		{"updater readiness", healthauth.Path, assertion, "127.0.0.1:1000", 200},
		{"updater not management", "/api/v1/overview", assertion, "127.0.0.1:1000", 401},
		{"wrong key", healthauth.Path, healthauth.Sign(wrong, time.Now()), "127.0.0.1:1000", 401},
		{"tampered", healthauth.Path, assertion + "x", "127.0.0.1:1000", 401},
		{"expired", healthauth.Path, healthauth.Sign(key, time.Now().Add(-time.Minute)), "127.0.0.1:1000", 401},
		{"future", healthauth.Path, healthauth.Sign(key, time.Now().Add(time.Minute)), "127.0.0.1:1000", 401},
		{"remote peer", healthauth.Path, assertion, "192.0.2.2:1000", 401},
		{"query not scoped", healthauth.Path + "?extra=1", assertion, "127.0.0.1:1000", 401},
		{"raw key denied", healthauth.Path, strings.Repeat("a", 64), "127.0.0.1:1000", 401},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := call(tc.path, tc.token, tc.remote); got != tc.want {
				t.Fatalf("HTTP %d want %d", got, tc.want)
			}
		})
	}
	engine.failed = true
	if got := call(healthauth.Path, assertion, "127.0.0.1:1000"); got != 503 {
		t.Fatalf("unhealthy runtime accepted: %d", got)
	}
}
