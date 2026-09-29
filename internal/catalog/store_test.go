package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/vault"
)

const actorID = "cccccccccccccccccccccccccccccccc"

func directory(t *testing.T) string {
	t.Helper()
	p := t.TempDir()
	if err := os.Chmod(p, 0700); err != nil {
		t.Fatal(err)
	}
	return p
}
func draft(id string) Client { return Client{ID: id, Name: "Phone", Kind: "wireguard"} }

func TestCatalogSQLitePersistenceAndOptimisticRevisions(t *testing.T) {
	path := directory(t)
	if s, err := Open(path); err == nil {
		s.Close()
		t.Fatal("missing database initialized")
	}
	s, err := Initialize(path)
	if err != nil {
		t.Fatal("initialize", err)
	}
	ctx := context.Background()
	c := draft(strings.Repeat("a", 32))
	first, err := s.Save(ctx, actorID, 0, c)
	if err != nil || first.Revision != 1 {
		t.Fatal("create", err)
	}
	if _, err := s.Save(ctx, actorID, 0, c); !errors.Is(err, ErrConflict) {
		t.Fatal("duplicate create accepted", err)
	}
	c.Name = "Phone'; DROP TABLE clients; --"
	second, err := s.Save(ctx, actorID, first.Revision, c)
	if err != nil || second.Revision != 2 {
		t.Fatal("update", err)
	}
	if _, err := s.Save(ctx, actorID, first.Revision, c); !errors.Is(err, ErrConflict) {
		t.Fatal("stale update accepted")
	}
	if other, err := Open(path); err == nil {
		other.Close()
		t.Fatal("concurrent writer accepted")
	}
	var histories, audits int
	if s.conn.QueryRowContext(ctx, "SELECT count(*) FROM client_history").Scan(&histories) != nil || histories != 1 {
		t.Fatal("history mismatch")
	}
	if s.conn.QueryRowContext(ctx, "SELECT count(*) FROM catalog_audit").Scan(&audits) != nil || audits != 2 {
		t.Fatal("audit mismatch")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(path, "catalog.db"))
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("database permissions")
	}
	s, err = Open(path)
	if err != nil {
		t.Fatal("reopen", err)
	}
	defer s.Close()
	got, err := s.Get(ctx, c.ID)
	if err != nil || got.Revision != 2 || got.Client.Name != c.Name {
		t.Fatal("reopen lost canonical metadata")
	}
	if _, err := Initialize(path); err == nil {
		t.Fatal("existing database reinitialized")
	}
}

func TestCatalogPaginationAndHistoryRetention(t *testing.T) {
	s, err := Initialize(directory(t))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	for i := 1; i <= 3; i++ {
		if _, err := s.Save(ctx, actorID, 0, draft(fmt.Sprintf("%032x", i))); err != nil {
			t.Fatal(err)
		}
	}
	page, err := s.List(ctx, "", 2)
	if err != nil || len(page) != 2 || page[0].Client.ID != fmt.Sprintf("%032x", 1) {
		t.Fatal("first page mismatch")
	}
	next, err := s.List(ctx, page[1].Client.ID, 2)
	if err != nil || len(next) != 1 || next[0].Client.ID != fmt.Sprintf("%032x", 3) {
		t.Fatal("next page mismatch")
	}
	entry := page[0]
	for i := 0; i < 35; i++ {
		entry.Client.Name = fmt.Sprintf("Phone %d", i)
		entry, err = s.Save(ctx, actorID, entry.Revision, entry.Client)
		if err != nil {
			t.Fatal(err)
		}
	}
	var count int
	if s.conn.QueryRowContext(ctx, "SELECT count(*) FROM client_history WHERE client_id=?", entry.Client.ID).Scan(&count) != nil || count != 32 {
		t.Fatal("history retention unbounded")
	}
	history, err := s.History(ctx, entry.Client.ID)
	if err != nil || len(history) != 32 || history[0].Revision != entry.Revision-1 {
		t.Fatal("history ordering mismatch")
	}
	changes, err := Changes(history[0].Client, entry.Client)
	if err != nil || len(changes) != 1 || changes[0] != "client.name.changed" {
		t.Fatal("semantic change mismatch")
	}
	for _, limit := range []int{0, 101} {
		if _, err := s.List(ctx, "", limit); err == nil {
			t.Fatal("invalid page limit accepted")
		}
	}
}

func TestCatalogRejectsCorruptionAndUnknownSchemaWithoutReset(t *testing.T) {
	for _, kind := range []string{"version", "trigger", "truncated", "symlink", "public"} {
		t.Run(kind, func(t *testing.T) {
			path := directory(t)
			s, err := Initialize(path)
			if err != nil {
				t.Fatal(err)
			}
			if kind == "version" {
				if _, err := s.conn.ExecContext(context.Background(), "PRAGMA user_version=99"); err != nil {
					t.Fatal(err)
				}
			}
			if kind == "trigger" {
				if _, err := s.conn.ExecContext(context.Background(), "CREATE TRIGGER foreign_trigger AFTER INSERT ON clients BEGIN DELETE FROM clients; END"); err != nil {
					t.Fatal(err)
				}
			}
			s.Close()
			filename := filepath.Join(path, "catalog.db")
			if kind == "truncated" {
				if err := os.WriteFile(filename, []byte("SQLite format 3\x00partial"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if kind == "public" {
				if err := os.Chmod(filename, 0644); err != nil {
					t.Fatal(err)
				}
			}
			if kind == "symlink" {
				target := filepath.Join(directory(t), "database")
				if err := os.Rename(filename, target); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, filename); err != nil {
					t.Fatal(err)
				}
			}
			before, err := os.ReadFile(filename)
			if err != nil {
				t.Fatal(err)
			}
			if bad, err := Open(path); err == nil {
				bad.Close()
				t.Fatal("unsafe database opened")
			}
			after, err := os.ReadFile(filename)
			if err != nil || string(before) != string(after) {
				t.Fatal("failed open changed database")
			}
		})
	}
}

func TestCatalogMetadataValidationAndExpiry(t *testing.T) {
	c := draft(strings.Repeat("a", 32))
	if c.ExportEligible(time.Now()) {
		t.Fatal("draft treated as eligible")
	}
	c.Enabled = true
	if c.Validate() == nil {
		t.Fatal("unprovisioned enabled client accepted")
	}
	c.ProfileID = strings.Repeat("b", 32)
	c.Credential = &vault.Reference{ID: c.ID, Revision: strings.Repeat("d", 32), KeyID: strings.Repeat("e", 32), Purpose: "wireguard-private"}
	c.ExpiresAt = "2026-09-09T12:00:00Z"
	if c.Validate() != nil || !c.ExportEligible(time.Date(2026, 9, 9, 11, 59, 59, 0, time.UTC)) || c.ExportEligible(time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)) {
		t.Fatal("expiry boundary incorrect")
	}
	c.Credential.ID = strings.Repeat("f", 32)
	if c.Validate() == nil {
		t.Fatal("another client's credential accepted")
	}
}

func TestCatalogConcurrentUpdatesHaveOneWinner(t *testing.T) {
	s, err := Initialize(directory(t))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	client := draft(strings.Repeat("a", 32))
	if _, err := s.Save(context.Background(), actorID, 0, client); err != nil {
		t.Fatal(err)
	}
	var workers sync.WaitGroup
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			_, err := s.Save(context.Background(), actorID, 1, client)
			results <- err
		}()
	}
	workers.Wait()
	close(results)
	wins, conflicts := 0, 0
	for err := range results {
		if err == nil {
			wins++
		} else if errors.Is(err, ErrConflict) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if wins != 1 || conflicts != 1 {
		t.Fatal("optimistic revision did not serialize writers")
	}
}

func TestCatalogAuditFailureRollsBackMetadataAndHistory(t *testing.T) {
	s, err := Initialize(directory(t))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	client := draft(strings.Repeat("a", 32))
	if _, err := s.Save(ctx, actorID, 0, client); err != nil {
		t.Fatal(err)
	}
	// A test-only SQLite trigger injects an audit write failure after the update.
	if _, err := s.conn.ExecContext(ctx, "CREATE TRIGGER reject_audit BEFORE INSERT ON catalog_audit BEGIN SELECT RAISE(ABORT,'injected audit failure'); END"); err != nil {
		t.Fatal(err)
	}
	client.Name = "Changed"
	if _, err := s.Save(ctx, actorID, 1, client); err == nil {
		t.Fatal("failed audit acknowledged")
	}
	if _, err := s.Get(ctx, client.ID); !errors.Is(err, ErrUnavailable) {
		t.Fatal("failed writer continued serving")
	}
	var revision, history int
	if s.conn.QueryRowContext(ctx, "SELECT revision FROM clients WHERE id=?", client.ID).Scan(&revision) != nil || revision != 1 {
		t.Fatal("failed transaction changed metadata")
	}
	if s.conn.QueryRowContext(ctx, "SELECT count(*) FROM client_history").Scan(&history) != nil || history != 0 {
		t.Fatal("failed transaction changed history")
	}
}

func TestCatalogClientCountLimit(t *testing.T) {
	s, err := Initialize(directory(t))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	tx, err := s.conn.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < MaxClients; i++ {
		client := draft(fmt.Sprintf("%032x", i))
		raw, _ := json.Marshal(client)
		if _, err := tx.ExecContext(ctx, "INSERT INTO clients(id,revision,document) VALUES(?,1,?)", client.ID, string(raw)); err != nil {
			tx.Rollback()
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Save(ctx, actorID, 0, draft(strings.Repeat("f", 32))); !errors.Is(err, ErrCapacity) {
		t.Fatal("client limit not enforced", err)
	}
	if _, err := s.Get(ctx, fmt.Sprintf("%032x", 0)); err != nil {
		t.Fatal("capacity rejection poisoned valid catalog")
	}
}
