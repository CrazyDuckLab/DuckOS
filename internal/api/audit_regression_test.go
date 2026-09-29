package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/CrazyDuckLab/DuckOS/internal/engine"
	"github.com/CrazyDuckLab/DuckOS/internal/identity"
	"github.com/CrazyDuckLab/DuckOS/internal/state"
)

type auditConfigEngine struct {
	*testEngine
	cfg *engine.Config
}

func (e *auditConfigEngine) Config() *engine.Config { return e.cfg }

func TestAuditConfiguredJournalRemainsAuthenticatedAndFailClosed(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	s, err := state.Open(dir, []byte(`{"privateKey":"synthetic-not-for-DTO"}`), func(json.RawMessage) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	cfg, err := (&engine.Engine{}).EnableConfig(s)
	if err != nil {
		t.Fatal(err)
	}
	h := handler(t, &testEngine{})
	h.engine = &auditConfigEngine{testEngine: &testEngine{}, cfg: cfg}
	idDir := t.TempDir()
	if err = os.Chmod(idDir, 0700); err != nil {
		t.Fatal(err)
	}
	identities, err := identity.Open(idDir)
	if err != nil {
		t.Fatal(err)
	}
	defer identities.Close()
	h.EnableIdentity(identities)
	if _, err = identities.Enroll("owner", "Synthetic audit password 2026!"); err != nil {
		t.Fatal(err)
	}
	token, _, err := identities.Login("owner", "Synthetic audit password 2026!")
	if err != nil {
		t.Fatal(err)
	}
	for _, operation := range []string{"apply", "confirm/" + strings.Repeat("a", 32), "rollback/" + strings.Repeat("a", 32)} {
		for _, authorized := range []bool{false, true} {
			r := httptest.NewRequest("POST", "http://127.0.0.1:8080/api/v1/config/"+operation, strings.NewReader(`{}`))
			want := 401
			if authorized {
				r.Header.Set("Authorization", "Bearer "+token)
				want = 503
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != want {
				t.Errorf("%s auth=%v: got %d want %d", operation, authorized, w.Code, want)
			}
		}
	}
	r := httptest.NewRequest("GET", "http://127.0.0.1:8080/api/v1/config/status", nil)
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 200 || strings.Contains(w.Body.String(), "synthetic-not-for-DTO") {
		t.Fatal("status unavailable or leaked private configuration")
	}
}

func TestAuditConfigRequiresAuthentication(t *testing.T) {
	h := handler(t, &testEngine{})
	for _, path := range []string{"status", "diff", "apply", "confirm/" + strings.Repeat("a", 32), "rollback/" + strings.Repeat("a", 32)} {
		method := "POST"
		if path == "status" {
			method = "GET"
		}
		r := httptest.NewRequest(method, "http://127.0.0.1:8080/api/v1/config/"+path, nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 401 {
			t.Errorf("%s: got %d, want 401", path, w.Code)
		}
	}
}
func TestAuditObjectsAcceptSessionNotBootstrap(t *testing.T) {
	h := newListerHandler(t)
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
	if _, err = s.Enroll("owner", "Synthetic audit password 2026!"); err != nil {
		t.Fatal(err)
	}
	token, _, err := s.Login("owner", "Synthetic audit password 2026!")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		token string
		want  int
	}{{token, 200}, {testToken, 401}, {"", 401}} {
		r := httptest.NewRequest("GET", "http://127.0.0.1:8080/api/v1/objects/network.route", nil)
		r.Header.Set("Authorization", "Bearer "+tc.token)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Errorf("got %d want %d", w.Code, tc.want)
		}
	}
}

type auditBrokenBody struct{}

func (auditBrokenBody) Read(p []byte) (int, error) { return 0, errors.New("synthetic read failure") }
func (auditBrokenBody) Close() error               { return nil }
func TestAuditBoundedBodyPropagatesFailure(t *testing.T) {
	r := httptest.NewRequest("POST", "/", strings.NewReader(strings.Repeat(" ", 32)))
	if _, err := readBoundedBody(r, 16); err == nil {
		t.Error("oversized body accepted")
	}
	r.Body = auditBrokenBody{}
	if _, err := readBoundedBody(r, 16); err == nil {
		t.Error("read failure swallowed")
	}
	r.Body = io.NopCloser(strings.NewReader("{}"))
	if b, err := readBoundedBody(r, 16); err != nil || string(b) != "{}" {
		t.Fatal("valid body rejected")
	}
}
