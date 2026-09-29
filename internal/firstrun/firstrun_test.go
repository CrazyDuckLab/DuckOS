// G18 first-run persistence torture regression.
package firstrun

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	return NewStore(filepath.Join(dir, "firstrun.json"))
}

func TestStore_LoadReturnsNotInitialised(t *testing.T) {
	s := newStore(t)
	if _, err := s.Load(); !errors.Is(err, ErrNotInitialised) {
		t.Fatalf("expected ErrNotInitialised, got %v", err)
	}
}

func TestStore_WriteAndLoadRoundTrip(t *testing.T) {
	s := newStore(t)
	if _, err := s.MarkComplete(State{}, "secret-token-1234567890", "router", "en", "dark", "router-1", []string{"language"}); err != nil {
		t.Fatalf("MarkComplete: %v", err)
	}
	loaded, err := s.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !loaded.Completed() {
		t.Fatalf("state must be completed")
	}
	if loaded.Mode != "router" || loaded.Locale != "en" || loaded.Theme != "dark" {
		t.Fatalf("drift: %+v", loaded)
	}
	if len(loaded.Steps) != 1 || loaded.Steps[0] != "language" {
		t.Fatalf("steps drift: %+v", loaded.Steps)
	}
	if len(loaded.AdminTokenHash) != 64 {
		t.Fatalf("adminTokenHash must be 64 hex chars, got %d", len(loaded.AdminTokenHash))
	}
	if loaded.AdminTokenHash != HashToken("secret-token-1234567890") {
		t.Fatalf("adminTokenHash drift")
	}
}

func TestStore_MarkCompleteAppendsSteps(t *testing.T) {
	s := newStore(t)
	first, err := s.MarkComplete(State{}, "tok", "easy", "en", "light", "host", []string{"language", "mode"})
	if err != nil {
		t.Fatalf("first MarkComplete: %v", err)
	}
	second, err := s.MarkComplete(first, "tok", "easy", "en", "light", "host", []string{"hostname"})
	if err != nil {
		t.Fatalf("second MarkComplete: %v", err)
	}
	if !second.CompletedAt.Equal(first.CompletedAt) {
		t.Fatalf("CompletedAt must be preserved across updates")
	}
	// Steps are deduped + sorted: hostname, language, mode
	if want := []string{"hostname", "language", "mode"}; !equalStrings(second.Steps, want) {
		t.Fatalf("steps drift: got %v want %v", second.Steps, want)
	}
}

func TestStore_WriteRejectsInvalidMode(t *testing.T) {
	s := newStore(t)
	if _, err := s.MarkComplete(State{}, "tok", "invalid-mode", "en", "dark", "h", nil); err == nil {
		t.Fatalf("invalid mode must be rejected")
	}
}

func TestStore_WriteRejectsInvalidTheme(t *testing.T) {
	s := newStore(t)
	if _, err := s.MarkComplete(State{}, "tok", "router", "en", "rainbow", "h", nil); err == nil {
		t.Fatalf("invalid theme must be rejected")
	}
}

func TestStore_WriteRejectsInvalidLocale(t *testing.T) {
	s := newStore(t)
	if _, err := s.MarkComplete(State{}, "tok", "router", "klingon", "dark", "h", nil); err == nil {
		t.Fatalf("invalid locale must be rejected")
	}
}

func TestStore_WriteRejectsInvalidStep(t *testing.T) {
	s := newStore(t)
	if _, err := s.MarkComplete(State{}, "tok", "router", "en", "dark", "h", []string{"bad step!"}); err == nil {
		t.Fatalf("invalid step must be rejected")
	}
}

func TestStore_WriteRejectsTooManySteps(t *testing.T) {
	s := newStore(t)
	steps := make([]string, 0, 65)
	for i := 0; i < 65; i++ {
		steps = append(steps, "step-"+itoa(i))
	}
	if _, err := s.MarkComplete(State{}, "tok", "router", "en", "dark", "h", steps); err == nil {
		t.Fatalf("too many steps must be rejected")
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	digits := []byte{}
	for i > 0 {
		digits = append([]byte{byte('0' + i%10)}, digits...)
		i /= 10
	}
	return string(digits)
}

func TestStore_DecodeRejectsUnknownVersion(t *testing.T) {
	raw := `{"version":99,"updatedAt":"2026-01-01T00:00:00Z"}`
	if _, err := Decode([]byte(raw)); err == nil {
		t.Fatalf("unknown version must be rejected")
	}
}

func TestStore_DecodeRejectsUnknownFields(t *testing.T) {
	raw := `{"version":1,"updatedAt":"2026-01-01T00:00:00Z","mystery":true}`
	if _, err := Decode([]byte(raw)); err == nil {
		t.Fatalf("unknown fields must be rejected")
	}
}

func TestStore_DecodeRejectsOversizedDocument(t *testing.T) {
	raw := []byte(`{"version":1,"updatedAt":"2026-01-01T00:00:00Z","pad":"` + strings.Repeat("x", maxDocumentBytes+1) + `"}`)
	if _, err := Decode(raw); err == nil {
		t.Fatalf("oversized document must be rejected")
	}
}

func TestStore_AtomicWriteLeavesNoTemp(t *testing.T) {
	s := newStore(t)
	if _, err := s.MarkComplete(State{}, "tok", "router", "en", "dark", "h", []string{"language"}); err != nil {
		t.Fatalf("MarkComplete: %v", err)
	}
	if _, err := os.Stat(s.path + ".tmp"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("temp file must not survive a successful write")
	}
}

func TestStore_PathIsEmptyRejected(t *testing.T) {
	s := &Store{path: ""}
	if _, err := s.Load(); err == nil {
		t.Fatalf("empty path must be rejected")
	}
	if err := s.Write(State{Version: SchemaVersion}); err == nil {
		t.Fatalf("Write with empty path must be rejected")
	}
}

func TestStore_CreatedAtStampedOnFirstCompletion(t *testing.T) {
	s := newStore(t)
	before := time.Now().UTC()
	state, err := s.MarkComplete(State{}, "tok", "router", "en", "dark", "h", []string{"language"})
	if err != nil {
		t.Fatalf("MarkComplete: %v", err)
	}
	if state.CompletedAt.Before(before) {
		t.Fatalf("CompletedAt must be >= before-call time")
	}
}

func TestIsValidMode(t *testing.T) {
	for _, m := range []string{"easy", "full", "router", "server", "custom"} {
		if !IsValidMode(m) {
			t.Errorf("%q must be valid", m)
		}
	}
	if IsValidMode("nonsense") {
		t.Errorf("nonsense must be invalid")
	}
}

func TestIsValidLocale(t *testing.T) {
	for _, l := range []string{"en", "ru", "de", "es"} {
		if !IsValidLocale(l) {
			t.Errorf("%q must be valid", l)
		}
	}
	if IsValidLocale("xx") {
		t.Errorf("xx must be invalid")
	}
}

func TestHashTokenIsStable(t *testing.T) {
	a := HashToken("hello")
	b := HashToken("hello")
	if a != b {
		t.Fatalf("hash drift: %s vs %s", a, b)
	}
	if c := HashToken("world"); c == a {
		t.Fatalf("different tokens must hash differently")
	}
	if len(a) != 64 {
		t.Fatalf("hash must be 64 hex chars, got %d", len(a))
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
