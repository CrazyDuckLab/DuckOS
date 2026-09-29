package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/migrate"
)

// MigratePath is the Agent IPC path for the
// migration analysis surface. The Agent is
// read-only: the package never executes the
// source commands and never mutates host
// state.
const MigratePath = "/agent/v1/migrate"

// MigrateMaxBytes is the Agent IPC cap for
// migration payloads. The cap matches the
// source-byte cap; the Agent never inflates
// it.
const MigrateMaxBytes = migrate.MaxSourceBytes + migrate.MaxPlanBytes + 1024

// MigrateTimeout is the bound the Agent places
// on a single migration analysis. The analyser
// is in-process; the bound is a safety net for
// very large sources.
const MigrateTimeout = 6 * time.Second

// MigrateObserver is the Agent-facing read-only
// source of migration data. The implementation
// owns the classifier; tests can substitute a
// fake.
type MigrateObserver interface {
	Migrate(ctx context.Context, req migrate.Source, body []byte) (migrate.Plan, error)
}

// MigrateObservable is the alias used by the
// Handler; the package exposes a single
// Agent-facing contract.
type MigrateObservable = MigrateObserver

// ServeMigrate is the Agent IPC handler for
// the migration surface. The handler is
// bounded, validated and concurrency capped.
// The request body is the source bytes; an
// empty body is rejected.
func (h *Handler) ServeMigrate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		problem(w, 405, "method_not_allowed")
		return
	}
	if r.URL.RawQuery != "" || r.URL.ForceQuery {
		problem(w, 400, "invalid_request")
		return
	}
	if !strings.HasSuffix(r.URL.Path, MigratePath) {
		problem(w, 404, "unknown_operation")
		return
	}
	select {
	case h.slots <- struct{}{}:
		defer func() { <-h.slots }()
	default:
		problem(w, 503, "agent_busy")
		return
	}
	host, ok := h.migrator.(MigrateObserver)
	if !ok || host == nil {
		problem(w, 503, "migrate_unavailable")
		return
	}
	defer func() { _ = r.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(r.Body, migrate.MaxSourceBytes+1))
	if err != nil {
		problem(w, 400, "invalid_body")
		return
	}
	if len(body) > migrate.MaxSourceBytes {
		problem(w, 413, "payload_too_large")
		return
	}
	if len(body) == 0 {
		problem(w, 400, "empty_source")
		return
	}
	// The filename and format hint are passed
	// through headers. The handler is a thin
	// pass-through: the body is the source.
	filename := strings.TrimSpace(r.Header.Get("X-DuckOS-Migrate-Filename"))
	formatHint := strings.TrimSpace(r.Header.Get("X-DuckOS-Migrate-Format"))
	src := migrate.Source{Filename: filename, Format: migrate.FormatKind(formatHint), Provenance: r.Header.Get("X-DuckOS-Migrate-Provenance")}
	ctx, cancel := context.WithTimeout(r.Context(), MigrateTimeout)
	defer cancel()
	plan, err := host.Migrate(ctx, src, body)
	if err != nil {
		// A non-fatal classification failure
		// (empty source, format unknown) is
		// reported as 422 with a structured
		// body. A true internal failure is 503.
		if errors.Is(err, migrate.ErrSourceTooLarge) {
			problem(w, 413, "payload_too_large")
			return
		}
		if errors.Is(err, migrate.ErrSourceEmpty) {
			payload, _ := json.Marshal(map[string]any{"error": map[string]string{"code": "migrate_empty_source", "message": err.Error()}})
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Cache-Control", "no-store")
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = w.Write(payload)
			return
		}
		problem(w, 503, "migrate_failed")
		return
	}
	data, err := plan.Encode()
	if err != nil || len(data) > migrate.MaxPlanBytes {
		problem(w, 503, "migrate_limit")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// MigrateFile is a small helper that reads a
// file from disk and submits it to the Agent
// IPC. The helper is used by the duck-agent
// boot-time recovery path; the Agent never
// imports files from disk directly.
func MigrateFile(ctx context.Context, host MigrateObserver, filename string, format string, body []byte) (migrate.Plan, error) {
	src := migrate.Source{Filename: filename, Format: migrate.FormatKind(format)}
	return host.Migrate(ctx, src, body)
}

// bytesReader is a tiny helper used in tests
// to construct an io.Reader from a byte slice.
// The helper is local to the package so the
// test does not need to import bytes.
func bytesReader(b []byte) io.Reader { return bytes.NewReader(b) }
