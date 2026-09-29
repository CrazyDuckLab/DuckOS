package state

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestOpenExistingNeverInitializesMissingJournal(t *testing.T) {
	path := t.TempDir()
	if err := os.Chmod(path, 0700); err != nil {
		t.Fatal(err)
	}
	validate := func(raw json.RawMessage) error {
		if !json.Valid(raw) {
			return errors.New("invalid")
		}
		return nil
	}
	if s, err := OpenExisting(path, validate); err == nil {
		s.Close()
		t.Fatal("initialized absent journal")
	}
	if _, err := os.Stat(filepath.Join(path, "state.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("created journal", err)
	}
	s, err := Open(path, json.RawMessage(`{"version":1}`), validate)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := s.Snapshot()
	s.Close()
	s, err = OpenExisting(path, validate)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := s.Snapshot()
	s.Close()
	if before.Current.ID != after.Current.ID {
		t.Fatal("reinitialized existing journal")
	}
	if err := os.Rename(filepath.Join(path, "state.json"), filepath.Join(path, "saved-state.json")); err != nil {
		t.Fatal(err)
	}
	if s, err := OpenExisting(path, validate); err == nil {
		s.Close()
		t.Fatal("replaced lost journal")
	}
}
