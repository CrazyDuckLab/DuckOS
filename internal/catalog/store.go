package catalog

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct {
	mu       sync.Mutex
	db       *sql.DB
	conn     *sql.Conn
	root     *os.Root
	lock     *os.File
	closed   bool
	poisoned bool
}

func Initialize(path string) (*Store, error) {
	entries, err := os.ReadDir(path)
	if err != nil || len(entries) != 0 {
		return nil, ErrUnavailable
	}
	return open(path, true)
}
func Open(path string) (*Store, error) { return open(path, false) }

func open(path string, initialize bool) (*Store, error) {
	root, abs, err := privateRoot(path)
	if err != nil {
		return nil, ErrUnavailable
	}
	s := &Store{root: root}
	ok := false
	defer func() {
		if !ok {
			s.Close()
		}
	}()
	dir, err := root.Open(".")
	if err != nil {
		return nil, ErrUnavailable
	}
	entries, err := dir.ReadDir(8)
	dir.Close()
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, ErrUnavailable
	}
	for _, entry := range entries {
		if entry.Name() != "writer.lock" && entry.Name() != "catalog.db" && entry.Name() != "catalog.db-journal" {
			return nil, ErrUnavailable
		}
		if _, err := privateFile(root, entry.Name(), false); err != nil {
			return nil, ErrUnavailable
		}
	}
	if len(entries) > 3 {
		return nil, ErrUnavailable
	}
	var original os.FileInfo
	if !initialize {
		original, err = existingHeader(root)
		if err != nil {
			return nil, ErrUnavailable
		}
	}
	if _, err := privateFile(root, "writer.lock", true); err != nil {
		return nil, ErrUnavailable
	}
	s.lock, err = root.OpenFile("writer.lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, ErrUnavailable
	}
	if err := syscall.Flock(int(s.lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return nil, ErrUnavailable
	}
	if initialize {
		f, err := root.OpenFile("catalog.db", os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
		if err != nil {
			return nil, ErrUnavailable
		}
		original, err = f.Stat()
		f.Close()
		if err != nil {
			return nil, ErrUnavailable
		}
	}
	u := url.URL{Scheme: "file", Path: filepath.Join(abs, "catalog.db")}
	query := u.Query()
	query.Set("mode", "rw")
	query.Add("_pragma", "trusted_schema(0)")
	u.RawQuery = query.Encode()
	s.db, err = sql.Open("sqlite", u.String())
	if err != nil {
		return nil, ErrUnavailable
	}
	s.db.SetMaxOpenConns(1)
	s.db.SetMaxIdleConns(1)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	s.conn, err = s.db.Conn(ctx)
	if err != nil {
		return nil, ErrUnavailable
	}
	current, err := privateFile(root, "catalog.db", false)
	if err != nil || !os.SameFile(original, current) {
		return nil, ErrUnavailable
	}
	if !initialize {
		if err := s.verifySchema(ctx); err != nil {
			return nil, ErrUnavailable
		}
	}
	settings := []string{
		"PRAGMA page_size=4096", "PRAGMA max_page_count=8192", "PRAGMA journal_mode=DELETE",
		"PRAGMA synchronous=EXTRA", "PRAGMA fullfsync=ON", "PRAGMA foreign_keys=ON",
		"PRAGMA cache_size=-2048", "PRAGMA mmap_size=0", "PRAGMA temp_store=MEMORY",
		"PRAGMA busy_timeout=1000", "PRAGMA trusted_schema=OFF",
	}
	for _, statement := range settings {
		if _, err := s.conn.ExecContext(ctx, statement); err != nil {
			return nil, ErrUnavailable
		}
	}
	if initialize {
		tx, err := s.conn.BeginTx(ctx, nil)
		if err != nil {
			return nil, ErrUnavailable
		}
		defer tx.Rollback()
		for _, name := range []string{"clients", "client_history", "catalog_audit"} {
			if _, err := tx.ExecContext(ctx, schema[name]); err != nil {
				return nil, ErrUnavailable
			}
		}
		if _, err := tx.ExecContext(ctx, "PRAGMA application_id=1146438475"); err != nil {
			return nil, ErrUnavailable
		}
		if _, err := tx.ExecContext(ctx, "PRAGMA user_version=1"); err != nil {
			return nil, ErrUnavailable
		}
		if err := tx.Commit(); err != nil {
			return nil, ErrUnavailable
		}
		d, err := root.Open(".")
		if err != nil {
			return nil, ErrUnavailable
		}
		err = d.Sync()
		d.Close()
		if err != nil {
			return nil, ErrUnavailable
		}
	}
	if err := s.verifySchema(ctx); err != nil {
		return nil, ErrUnavailable
	}
	for name, expected := range map[string]int{"page_size": 4096, "max_page_count": 8192, "synchronous": 3, "foreign_keys": 1, "trusted_schema": 0, "cache_size": -2048, "mmap_size": 0} {
		var actual int
		if s.conn.QueryRowContext(ctx, "PRAGMA "+name).Scan(&actual) != nil || actual != expected {
			return nil, ErrUnavailable
		}
	}
	var journal string
	if s.conn.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&journal) != nil || journal != "delete" {
		return nil, ErrUnavailable
	}
	if err := s.verifyClients(ctx); err != nil {
		return nil, ErrUnavailable
	}
	ok = true
	return s, nil
}

func (s *Store) verifySchema(ctx context.Context) error {
	var app, version int
	if s.conn.QueryRowContext(ctx, "PRAGMA application_id").Scan(&app) != nil || app != applicationID || s.conn.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version) != nil || version != schemaVersion {
		return ErrUnavailable
	}
	var integrity string
	if s.conn.QueryRowContext(ctx, "PRAGMA quick_check(1)").Scan(&integrity) != nil || integrity != "ok" {
		return ErrUnavailable
	}
	rows, err := s.conn.QueryContext(ctx, "SELECT name,type,sql FROM sqlite_schema WHERE sql IS NOT NULL")
	if err != nil {
		return ErrUnavailable
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var name, kind, definition string
		if rows.Scan(&name, &kind, &definition) != nil || kind != "table" || schema[name] == "" || schema[name] != definition {
			return ErrUnavailable
		}
		count++
	}
	if rows.Err() != nil || count != len(schema) {
		return ErrUnavailable
	}
	return nil
}

func (s *Store) verifyClients(ctx context.Context) error {
	rows, err := s.conn.QueryContext(ctx, "SELECT id,revision,document FROM clients LIMIT 1025")
	if err != nil {
		return ErrUnavailable
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var id, raw string
		var revision int64
		if rows.Scan(&id, &revision, &raw) != nil {
			return ErrUnavailable
		}
		client, err := decodeClient(raw)
		if err != nil || client.ID != id || revision < 1 || revision > 2147483647 {
			return ErrUnavailable
		}
		count++
	}
	if rows.Err() != nil || count > MaxClients {
		return ErrUnavailable
	}
	return nil
}

func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	var err error
	if s.conn != nil {
		err = s.conn.Close()
	}
	if s.db != nil {
		if e := s.db.Close(); err == nil {
			err = e
		}
	}
	if s.lock != nil {
		if e := s.lock.Close(); err == nil {
			err = e
		}
	}
	if s.root != nil {
		if e := s.root.Close(); err == nil {
			err = e
		}
	}
	return err
}
