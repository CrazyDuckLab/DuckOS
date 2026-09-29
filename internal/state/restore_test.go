package state

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
)

func TestH09AuthoritativeReservationSurvivesRestart(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	validate := func(json.RawMessage) error { return nil }
	s, err := Open(dir, []byte(`{"version":1}`), validate)
	if err != nil {
		t.Fatal(err)
	}
	st, err := s.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	request := json.RawMessage(`{"test":"reservation"}`)
	id := strings.Repeat("1", 32)
	if err := s.BeginRestore(id, strings.Repeat("f", 64), digest(request), request); !errors.Is(err, ErrConflict) {
		t.Fatalf("invented revision accepted: %v", err)
	}
	if err := s.BeginRestore(id, st.Current.ID, digest(request), request); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Prepare(st.Current.ID, []byte(`{"version":2}`), "owner", "concurrent"); !errors.Is(err, ErrPending) {
		t.Fatalf("reservation did not serialize config writer: %v", err)
	}
	s.Close()
	s, err = OpenExisting(dir, validate)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.BeginRestore(strings.Repeat("2", 32), st.Current.ID, digest(request), request); !errors.Is(err, ErrPending) {
		t.Fatal("restart lost reservation")
	}
	if err := s.FinishRestore(id, digest(request)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Prepare(st.Current.ID, []byte(`{"version":2}`), "owner", "after restore"); err != nil {
		t.Fatal(err)
	}
}
