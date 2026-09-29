package catalog

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

func (s *Store) Get(ctx context.Context, id string) (Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.poisoned {
		return Entry{}, ErrUnavailable
	}
	if !identifier.MatchString(id) {
		return Entry{}, ErrInput
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	var revision int64
	var raw string
	err := s.conn.QueryRowContext(ctx, "SELECT revision,document FROM clients WHERE id=?", id).Scan(&revision, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return Entry{}, ErrNotFound
	}
	if err != nil {
		return Entry{}, ErrUnavailable
	}
	client, err := decodeClient(raw)
	if err != nil || client.ID != id {
		return Entry{}, ErrUnavailable
	}
	return Entry{Revision: revision, Client: client}, nil
}

func (s *Store) List(ctx context.Context, after string, limit int) ([]Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.poisoned {
		return nil, ErrUnavailable
	}
	if (after != "" && !identifier.MatchString(after)) || limit < 1 || limit > 100 {
		return nil, ErrInput
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	rows, err := s.conn.QueryContext(ctx, "SELECT id,revision,document FROM clients WHERE id>? ORDER BY id LIMIT ?", after, limit)
	if err != nil {
		return nil, ErrUnavailable
	}
	defer rows.Close()
	result := []Entry{}
	for rows.Next() {
		var id, raw string
		var revision int64
		if rows.Scan(&id, &revision, &raw) != nil {
			return nil, ErrUnavailable
		}
		client, err := decodeClient(raw)
		if err != nil || client.ID != id {
			return nil, ErrUnavailable
		}
		result = append(result, Entry{Revision: revision, Client: client})
	}
	if rows.Err() != nil {
		return nil, ErrUnavailable
	}
	return result, nil
}

// Save changes desired metadata only. expected=0 creates a client. The Engine
// must coordinate dangerous changes with service transactions before activation.
func (s *Store) Save(ctx context.Context, actor string, expected int64, client Client) (Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.poisoned {
		return Entry{}, ErrUnavailable
	}
	if !identifier.MatchString(actor) || expected < 0 || expected >= 2147483647 || client.Validate() != nil {
		return Entry{}, ErrInput
	}
	raw, err := json.Marshal(client)
	if err != nil || len(raw) > maxDocumentBytes {
		return Entry{}, ErrInput
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	tx, err := s.conn.BeginTx(ctx, nil)
	if err != nil {
		return Entry{}, ErrUnavailable
	}
	defer tx.Rollback()
	action := "client.create"
	if expected == 0 {
		var count int
		if tx.QueryRowContext(ctx, "SELECT count(*) FROM clients").Scan(&count) != nil {
			return Entry{}, ErrUnavailable
		}
		if count >= MaxClients {
			return Entry{}, ErrCapacity
		}
		result, err := tx.ExecContext(ctx, "INSERT INTO clients(id,revision,document) VALUES(?,1,?) ON CONFLICT(id) DO NOTHING", client.ID, string(raw))
		if err != nil {
			s.poisoned = true
			return Entry{}, ErrUnavailable
		}
		n, err := result.RowsAffected()
		if err != nil {
			s.poisoned = true
			return Entry{}, ErrUnavailable
		}
		if n != 1 {
			return Entry{}, ErrConflict
		}
	} else {
		action = "client.update"
		var previous string
		if err := tx.QueryRowContext(ctx, "SELECT document FROM clients WHERE id=? AND revision=?", client.ID, expected).Scan(&previous); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return Entry{}, ErrConflict
			}
			return Entry{}, ErrUnavailable
		}
		old, err := decodeClient(previous)
		if err != nil || old.ID != client.ID || old.Kind != client.Kind {
			return Entry{}, ErrInput
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO client_history(client_id,revision,document) VALUES(?,?,?)", client.ID, expected, previous); err != nil {
			s.poisoned = true
			return Entry{}, ErrUnavailable
		}
		result, err := tx.ExecContext(ctx, "UPDATE clients SET revision=?,document=? WHERE id=? AND revision=?", expected+1, string(raw), client.ID, expected)
		if err != nil {
			s.poisoned = true
			return Entry{}, ErrUnavailable
		}
		if affected, err := result.RowsAffected(); err != nil || affected != 1 {
			s.poisoned = true
			return Entry{}, ErrUnavailable
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM client_history WHERE client_id=? AND revision NOT IN (SELECT revision FROM client_history WHERE client_id=? ORDER BY revision DESC LIMIT 32)", client.ID, client.ID); err != nil {
			s.poisoned = true
			return Entry{}, ErrUnavailable
		}
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO catalog_audit(actor,client_id,action,revision,at) VALUES(?,?,?,?,?)", actor, client.ID, action, expected+1, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		s.poisoned = true
		return Entry{}, ErrUnavailable
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM catalog_audit WHERE sequence NOT IN (SELECT sequence FROM catalog_audit ORDER BY sequence DESC LIMIT 1024)"); err != nil {
		s.poisoned = true
		return Entry{}, ErrUnavailable
	}
	if err := tx.Commit(); err != nil {
		s.poisoned = true
		return Entry{}, ErrUnavailable
	}
	// Return a decoded owned value rather than aliasing the caller's credential.
	owned, _ := decodeClient(string(raw))
	return Entry{Revision: expected + 1, Client: owned}, nil
}
