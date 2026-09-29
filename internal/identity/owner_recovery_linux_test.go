package identity

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestOwnerRecoveryLifecycle(t *testing.T) {
	s, path := serviceTest(t)
	token := ownerTest(t, s)
	reader, err := s.CreateUser(token, "reader", testPassword, ReadOnly)
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(path, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	backups := filepath.Join(t.TempDir(), "backups")
	if _, err = recoverOwner(path, backups); err == nil {
		t.Fatal("running service must block offline reset")
	}
	s.Close()
	backup, err := recoverOwner(path, backups)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := os.ReadFile(filepath.Join(backup, "state.json"))
	if err != nil || !bytes.Equal(saved, before) {
		t.Fatal("backup is not exact", err)
	}
	var manifest struct {
		Source string
		SHA256 map[string]string
	}
	raw, _ := os.ReadFile(filepath.Join(backup, "manifest.json"))
	if err = json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(before)
	if manifest.Source != path || manifest.SHA256["state.json"] != hex.EncodeToString(sum[:]) {
		t.Fatal("invalid backup manifest")
	}
	for _, name := range []string{"state.json", "writer.lock", "manifest.json"} {
		fi, err := os.Stat(filepath.Join(backup, name))
		if err != nil || fi.Mode().Perm() != 0600 {
			t.Fatal("backup permissions", name, err)
		}
	}
	// Repeating reset while enrollment is pending remains safe.
	second, err := recoverOwner(path, backups)
	if err != nil || second == backup {
		t.Fatal("second reset", err)
	}
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if s.Enrolled() {
		t.Fatal("recovery must reopen enrollment")
	}
	if _, _, err = s.Login("owner", testPassword); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("old password accepted", err)
	}
	if _, err = s.Current(token); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("old token accepted", err)
	}
	if _, _, err = s.Login("reader", testPassword); err != nil {
		t.Fatal("unrelated user lost", err)
	}
	if _, err = s.Enroll("reader", testPassword); err == nil {
		t.Fatal("existing username overwritten")
	}
	if _, err = s.Enroll("new-owner", testPassword); err != nil {
		t.Fatal(err)
	}
	newToken, _, err := s.Login("new-owner", testPassword)
	if err != nil {
		t.Fatal(err)
	}
	users, err := s.Users(newToken)
	if err != nil || len(users) != 2 {
		t.Fatal("users not preserved", err)
	}
	if users[0].ID != reader.ID {
		t.Fatal("reader identity changed")
	}
	if _, err = s.Enroll("third-owner", testPassword); !errors.Is(err, ErrForbidden) {
		t.Fatal("normal enrollment reopened")
	}
	s.Close()
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, _, err = s.Login("new-owner", testPassword); err != nil {
		t.Fatal("new credentials not durable", err)
	}
}

func TestOwnerRecoveryBackupFailureAndIsolation(t *testing.T) {
	s, path := serviceTest(t)
	ownerTest(t, s)
	s.Close()
	before, _ := os.ReadFile(filepath.Join(path, "state.json"))
	parent := t.TempDir()
	// These sentinels stand for independent journals/artifacts. Recovery is given
	// only the identity path and cannot traverse siblings or execute services.
	for _, name := range []string{"network-state.json", "enrollment.json", "config.json", "nftables", "networkd", "routes"} {
		if err := os.WriteFile(filepath.Join(parent, name), []byte(name), 0600); err != nil {
			t.Fatal(err)
		}
	}
	blocked := filepath.Join(parent, "blocked")
	os.WriteFile(blocked, []byte("not a directory"), 0600)
	if _, err := recoverOwner(path, blocked); err == nil {
		t.Fatal("backup failure accepted")
	}
	after, _ := os.ReadFile(filepath.Join(path, "state.json"))
	if !bytes.Equal(before, after) {
		t.Fatal("mutated without backup")
	}
	backups := filepath.Join(parent, "backups")
	// Simulate interruption immediately before canonical rename.
	os.WriteFile(filepath.Join(path, "owner-recovery.tmp"), []byte("partial"), 0600)
	if _, err := recoverOwner(path, backups); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"network-state.json", "enrollment.json", "config.json", "nftables", "networkd", "routes"} {
		raw, err := os.ReadFile(filepath.Join(parent, name))
		if err != nil || string(raw) != name {
			t.Fatal("unrelated state changed", name, err)
		}
	}
}

func TestOwnerRecoveryRejectsUnsafeBackupAndFiles(t *testing.T) {
	s, path := serviceTest(t)
	ownerTest(t, s)
	s.Close()
	before, _ := os.ReadFile(filepath.Join(path, "state.json"))
	base := t.TempDir()
	link := filepath.Join(base, "link")
	os.Symlink(t.TempDir(), link)
	if _, err := recoverOwner(path, link); err == nil {
		t.Fatal("symlink backup accepted")
	}
	os.Symlink(filepath.Join(path, "state.json"), filepath.Join(path, "state.tmp"))
	if _, err := recoverOwner(path, filepath.Join(base, "backups")); err == nil {
		t.Fatal("symlink store entry accepted")
	}
	after, _ := os.ReadFile(filepath.Join(path, "state.json"))
	if !bytes.Equal(before, after) {
		t.Fatal("unsafe recovery mutated state")
	}
}

func TestOwnerRecoveryNonRootGate(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("non-root invocation verified by CLI production witness")
	}
	if _, err := RecoverOwner("/does-not-exist", "/does-not-exist"); err == nil || err.Error() != "Owner recovery requires effective root" {
		t.Fatal(err)
	}
}
