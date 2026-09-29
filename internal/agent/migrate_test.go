package agent

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/migrate"
)

// fakeMigrator records the call. The analyser
// itself is exercised through the migrate
// package's tests; this file covers the Agent
// IPC surface.
type fakeMigrator struct {
	err error
}

func (f *fakeMigrator) Migrate(_ context.Context, _ migrate.Source, body []byte) (migrate.Plan, error) {
	if f.err != nil {
		return migrate.Plan{}, f.err
	}
	return migrate.Analyse(migrate.Source{Filename: "test"}, body, time.Unix(1, 0))
}

func TestServeMigrateRoundtrip(t *testing.T) {
	srv := NewHandler(nil)
	srv.EnableMigrate(&fakeMigrator{})
	body := []byte("/ip address\nadd address=192.0.2.1/24 interface=ether1\n")
	r := httptest.NewRequest("POST", MigratePath, bytes.NewReader(body))
	r.Header.Set("X-DuckOS-Migrate-Filename", "router.rsc")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d (%s)", w.Code, w.Body.String())
	}
	plan, err := migrate.ParsePlan(w.Body.Bytes())
	if err != nil {
		t.Fatalf("parse plan: %v", err)
	}
	if plan.Format != migrate.FormatMikroTik {
		t.Fatalf("expected mikrotik, got %s", plan.Format)
	}
}

func TestServeMigrateEmptyBody(t *testing.T) {
	srv := NewHandler(nil)
	srv.EnableMigrate(&fakeMigrator{})
	r := httptest.NewRequest("POST", MigratePath, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	if w.Code != 400 {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestServeMigrateOversized(t *testing.T) {
	srv := NewHandler(nil)
	srv.EnableMigrate(&fakeMigrator{})
	body := bytes.Repeat([]byte("a\n"), migrate.MaxSourceBytes/2+2)
	r := httptest.NewRequest("POST", MigratePath, bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	if w.Code != 413 && w.Code != 400 {
		t.Fatalf("expected 413 or 400, got %d", w.Code)
	}
}

func TestServeMigrateMethodNotAllowed(t *testing.T) {
	srv := NewHandler(nil)
	srv.EnableMigrate(&fakeMigrator{})
	r := httptest.NewRequest("GET", MigratePath, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	if w.Code != 405 {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestServeMigrateWithoutHost(t *testing.T) {
	srv := NewHandler(nil)
	body := []byte("/ip address\nadd address=192.0.2.1/24\n")
	r := httptest.NewRequest("POST", MigratePath, bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	if w.Code != 503 {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

func TestServeMigrateEmptySource(t *testing.T) {
	srv := NewHandler(nil)
	srv.EnableMigrate(&fakeMigrator{})
	// The body has bytes but they are all
	// whitespace; the analyser returns
	// ErrSourceEmpty.
	body := []byte("\n\n   \n")
	r := httptest.NewRequest("POST", MigratePath, bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	if w.Code != 422 {
		t.Fatalf("expected 422, got %d (%s)", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "migrate_empty_source") {
		t.Fatalf("expected structured error, got %q", w.Body.String())
	}
}

func TestServeMigrateInternalError(t *testing.T) {
	srv := NewHandler(nil)
	srv.EnableMigrate(&fakeMigrator{err: errors.New("boom")})
	body := []byte("/ip address\nadd address=192.0.2.1/24\n")
	r := httptest.NewRequest("POST", MigratePath, bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	if w.Code != 503 {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

func TestMigrateFileHelper(t *testing.T) {
	m := &fakeMigrator{}
	p, err := MigrateFile(context.Background(), m, "test.rsc", "mikrotik", []byte("/ip address\nadd address=192.0.2.1/24\n"))
	if err != nil {
		t.Fatalf("migrate file: %v", err)
	}
	if p.Format != migrate.FormatMikroTik {
		t.Fatalf("expected mikrotik, got %s", p.Format)
	}
}

func TestBytesReaderHelper(t *testing.T) {
	r := bytesReader([]byte("hello"))
	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(data) != "hello" {
		t.Fatalf("expected hello, got %q", string(data))
	}
}
