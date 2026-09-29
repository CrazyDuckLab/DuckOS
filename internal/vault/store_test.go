package vault

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/CrazyDuckLab/DuckOS/internal/state"
)

func privateDirectory(t *testing.T) string {
	t.Helper()
	path := t.TempDir()
	if err := os.Chmod(path, 0700); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestVaultDurableImmutableRevisions(t *testing.T) {
	key, ref := fixture(t)
	path := privateDirectory(t)
	if s, err := OpenStore(path, key); err == nil {
		s.Close()
		t.Fatal("missing vault silently initialized")
	}
	s, err := InitializeStore(path, key)
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprintf("%v %#v", s, s) != "[private vault store] [private vault store]" {
		t.Fatal("vault diagnostics were not redacted")
	}
	if _, err := json.Marshal(s); err == nil {
		t.Fatal("vault implicitly serialized")
	}
	first, err := s.Put(ref.ID, ref.Purpose, []byte("synthetic-old-private-key"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.Put(ref.ID, ref.Purpose, []byte("synthetic-new-private-key"))
	if err != nil || first.Revision == second.Revision {
		t.Fatal("rotation overwrote immutable reference", err)
	}
	if other, err := OpenStore(path, key); err == nil {
		other.Close()
		t.Fatal("concurrent writer accepted")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if secret, err := s.Get(first); err == nil || secret != nil {
		t.Fatal("closed store exported secret")
	}
	raw, err := os.ReadFile(filepath.Join(path, "state.json"))
	if err != nil || bytes.Contains(raw, []byte("synthetic-old")) || bytes.Contains(raw, []byte("synthetic-new")) {
		t.Fatal("persistent history exposed plaintext")
	}
	s, err = OpenStore(path, key)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for reference, expected := range map[Reference]string{first: "synthetic-old-private-key", second: "synthetic-new-private-key"} {
		secret, err := s.Get(reference)
		if err != nil || string(secret.Reveal()) != expected {
			t.Fatal("reopened vault lost revision")
		}
		secret.Clear()
	}
}

func TestVaultWrongKeyAndCorruptionDoNotReinitialize(t *testing.T) {
	key, ref := fixture(t)
	path := privateDirectory(t)
	s, err := InitializeStore(path, key)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Put(ref.ID, ref.Purpose, []byte("synthetic-private-key")); err != nil {
		t.Fatal(err)
	}
	s.Close()
	before, err := os.ReadFile(filepath.Join(path, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	wrong, _ := fixture(t)
	if bad, err := OpenStore(path, wrong); err == nil {
		bad.Close()
		t.Fatal("wrong installation key opened vault")
	}
	if bad, err := InitializeStore(path, key); err == nil {
		bad.Close()
		t.Fatal("existing vault reinitialized")
	}
	after, _ := os.ReadFile(filepath.Join(path, "state.json"))
	if !bytes.Equal(before, after) {
		t.Fatal("failed open modified encrypted data")
	}
	if err := os.WriteFile(filepath.Join(path, "state.json"), []byte("truncated"), 0600); err != nil {
		t.Fatal(err)
	}
	if bad, err := OpenStore(path, key); err == nil {
		bad.Close()
		t.Fatal("corrupt vault reinitialized")
	}
}

func TestVaultInterruptedCreationRollsBackOnOpen(t *testing.T) {
	key, ref := fixture(t)
	path := privateDirectory(t)
	s, err := InitializeStore(path, key)
	if err != nil {
		t.Fatal(err)
	}
	ref.KeyID = s.db.KeyID
	envelope, err := Seal(key, ref, []byte("synthetic-uncommitted"))
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(database{Version: 1, KeyID: s.db.KeyID, Records: []storedRecord{{Reference: ref, Envelope: envelope}}})
	snap, err := s.state.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.state.Prepare(snap.Current.ID, data, "vault", "secret.create"); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = OpenStore(path, key)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if secret, err := s.Get(ref); err == nil || secret != nil {
		t.Fatal("pending secret implicitly committed")
	}
	snap, err = s.state.Snapshot()
	if err != nil || snap.Pending != nil {
		t.Fatal("pending vault journal not reconciled")
	}
}

func TestVaultDurabilityFailureDeniesSubsequentReads(t *testing.T) {
	key, ref := fixture(t)
	path := privateDirectory(t)
	s, err := InitializeStore(path, key)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	first, err := s.Put(ref.ID, ref.Purpose, []byte("synthetic-private-key"))
	if err != nil {
		t.Fatal(err)
	}
	// Closing the underlying durable store injects a persistence failure without
	// changing any real host state or relying on filesystem timing.
	if err := s.state.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Put(ref.ID, ref.Purpose, []byte("replacement")); err == nil {
		t.Fatal("failed persistence acknowledged")
	}
	if secret, err := s.Get(first); err == nil || secret != nil {
		t.Fatal("uncertain durable state still exported secrets")
	}
}

func TestVaultDatabaseBounds(t *testing.T) {
	key, ref := fixture(t)
	if _, err := decodeDatabase(key, ref.KeyID, bytes.Repeat([]byte("x"), state.MaxConfigBytes+1)); err == nil {
		t.Fatal("oversized database accepted")
	}
}
