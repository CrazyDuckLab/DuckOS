package upgrade

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func newTestVerifier(t *testing.T) (ed25519.PrivateKey, Ed25519Verifier) {
	t.Helper()
	priv, pub, err := GenerateSigningKey()
	if err != nil {
		t.Fatal(err)
	}
	return priv, Ed25519Verifier{Public: pub}
}

func buildManifest(t *testing.T, payload []byte) Manifest {
	t.Helper()
	sum := sha256.Sum256(payload)
	return Manifest{
		Format: FormatVer, Architecture: "amd64",
		SourceVersion: "1.0.0-rc.1", TargetVersion: "1.0.0",
		InputSchema: 1, OutputSchema: 1,
		RequiredSpace: int64(len(payload)), ReserveSpace: int64(len(payload)),
		CreatedAt:        "2026-01-01T00:00:00Z",
		PayloadAlgorithm: "sha256",
		PayloadDigest:    hex.EncodeToString(sum[:]),
		PayloadBytes:     int64(len(payload)),
	}
}

func signPackage(t *testing.T, manifestBytes []byte, priv ed25519.PrivateKey) []byte {
	t.Helper()
	return ed25519.Sign(priv, manifestBytes)
}

func buildPackage(t *testing.T, payload []byte) (*Package, []byte, ed25519.PrivateKey, Ed25519Verifier) {
	t.Helper()
	priv, verifier := newTestVerifier(t)
	manifest := buildManifest(t, payload)
	mb, _ := json.Marshal(manifest)
	sig := signPackage(t, mb, priv)
	body := append(append([]byte{}, sig...), mb...)
	header := Header{Magic: Magic, Format: FormatVer, BodyBytes: int64(len(body))}
	archive := append(header.Marshal(), body...)
	archive = append(archive, payload...)
	return nil, archive, priv, verifier
}

func TestGenerateSigningKeyProducesUsableVerifier(t *testing.T) {
	priv, pub, err := GenerateSigningKey()
	if err != nil {
		t.Fatal(err)
	}
	if len(priv) != ed25519.PrivateKeySize || len(pub) != ed25519.PublicKeySize {
		t.Fatal("key sizes are wrong")
	}
	verifier := Ed25519Verifier{Public: pub}
	if err := verifier.Verify([]byte("hello"), ed25519.Sign(priv, []byte("hello"))); err != nil {
		t.Fatal("verifier rejected its own signature", err)
	}
}

func TestManifestValidateRejectsInvalidInputs(t *testing.T) {
	manifest := buildManifest(t, []byte("payload"))
	if err := manifest.Validate(); err != nil {
		t.Fatal("baseline manifest was rejected", err)
	}
	bad := manifest
	bad.Format = 99
	if err := bad.Validate(); err == nil {
		t.Fatal("invalid format version was accepted")
	}
	bad = manifest
	bad.Architecture = "INVALID ARCH"
	if err := bad.Validate(); err == nil {
		t.Fatal("invalid architecture was accepted")
	}
	bad = manifest
	bad.PayloadAlgorithm = "md5"
	if err := bad.Validate(); err == nil {
		t.Fatal("invalid algorithm was accepted")
	}
	bad = manifest
	bad.PayloadDigest = "deadbeef"
	if err := bad.Validate(); err == nil {
		t.Fatal("invalid digest was accepted")
	}
	bad = manifest
	bad.PayloadBytes = -1
	if err := bad.Validate(); err == nil {
		t.Fatal("negative payload size was accepted")
	}
}

func TestDecodeRoundTrip(t *testing.T) {
	payload := []byte("the-payload-bytes")
	_, archive, _, verifier := buildPackage(t, payload)
	pkg, err := Decode(archive, verifier)
	if err != nil {
		t.Fatal(err)
	}
	if pkg.Manifest.Architecture != "amd64" {
		t.Fatal("architecture did not round-trip")
	}
	if string(pkg.Payload) != string(payload) {
		t.Fatal("payload did not round-trip")
	}
}

func TestDecodeRefusesBadSignature(t *testing.T) {
	payload := []byte("the-payload-bytes")
	_, archive, _, _ := buildPackage(t, payload)
	_, verifier := newTestVerifier(t)
	if _, err := Decode(archive, verifier); err == nil {
		t.Fatal("cross-key signature was accepted")
	}
}

func TestDecodeRefusesBadPayloadDigest(t *testing.T) {
	priv, _ := newTestVerifier(t)
	payload := []byte("the-payload-bytes")
	manifest := buildManifest(t, []byte("the-OTHER-payload-bytes"))
	mb, _ := json.Marshal(manifest)
	sig := signPackage(t, mb, priv)
	body := append(append([]byte{}, sig...), mb...)
	header := Header{Magic: Magic, Format: FormatVer, BodyBytes: int64(len(body))}
	archive := append(header.Marshal(), body...)
	archive = append(archive, payload...)
	verifier := Ed25519Verifier{Public: mustDerivePublic(priv)}
	if _, err := Decode(archive, verifier); err == nil {
		t.Fatal("package with mismatched payload digest was accepted")
	}
}

func TestDecodeRefusesBadMagic(t *testing.T) {
	payload := []byte("the-payload-bytes")
	_, archive, _, verifier := buildPackage(t, payload)
	archive = append([]byte("WRONGARC!"), archive[len(Magic):]...)
	if _, err := Decode(archive, verifier); err == nil {
		t.Fatal("package with bad magic was accepted")
	}
}

func TestReleaseStoreRefusesReservedID(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	s, err := NewReleaseStore(dir, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	meta := ReleaseMeta{ID: string(ReleaseActive), Architecture: "amd64"}
	if err := s.Materialize(meta, []byte("payload")); err == nil {
		t.Fatal("reserved id was accepted")
	}
}

func TestReleaseStoreMaterializeAndActivate(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	s, err := NewReleaseStore(dir, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte("the-payload-bytes")
	meta := ReleaseMeta{ID: "v1.0.0", Architecture: "amd64", SourceVersion: "1.0.0-rc.1", TargetVersion: "1.0.0"}
	if err := s.Materialize(meta, payload); err != nil {
		t.Fatal(err)
	}
	if err := s.VerifyFingerprint("v1.0.0"); err != nil {
		t.Fatal("fingerprint mismatch", err)
	}
	if err := s.Activate("v1.0.0"); err != nil {
		t.Fatal(err)
	}
	target, err := os.Readlink(filepath.Join(dir, string(ReleaseActive)))
	if err != nil || target != "v1.0.0" {
		t.Fatal("Active symlink was not set", target, err)
	}
	// Materialising again should refuse to overwrite.
	if err := s.Materialize(meta, payload); err == nil {
		t.Fatal("overwrite was accepted")
	}
}

func TestReleaseStoreRefusesToRemoveActiveOrPrevious(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	s, _ := NewReleaseStore(dir, 1<<20)
	mustMaterialize(t, s, "v1.0.0", []byte("a"))
	mustMaterialize(t, s, "v1.0.1", []byte("b"))
	if err := s.Activate("v1.0.1"); err != nil {
		t.Fatal(err)
	}
	if err := s.Activate("v1.0.0"); err != nil {
		t.Fatal(err)
	}
	if err := s.Remove("v1.0.1"); err == nil {
		t.Fatal("removed the active release")
	}
	if err := s.Remove("v1.0.0"); err == nil {
		t.Fatal("removed the previous release")
	}
}

func TestReleaseStoreRefusesPayloadExceedingSizeCap(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	s, _ := NewReleaseStore(dir, 4)
	if err := s.Materialize(ReleaseMeta{ID: "v1.0.0", Architecture: "amd64"}, []byte("hello")); err == nil {
		t.Fatal("oversized payload was accepted")
	}
}

func TestReleaseStoreVerifyFingerprintDetectsTampering(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	s, _ := NewReleaseStore(dir, 1<<20)
	mustMaterialize(t, s, "v1.0.0", []byte("payload"))
	payloadPath := filepath.Join(dir, "v1.0.0", "payload")
	if err := os.WriteFile(payloadPath, []byte("tampered"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := s.VerifyFingerprint("v1.0.0"); err == nil {
		t.Fatal("tampered release passed fingerprint check")
	}
}

func TestReleaseStoreListExcludesReservedNames(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	s, _ := NewReleaseStore(dir, 1<<20)
	mustMaterialize(t, s, "v1.0.0", []byte("payload"))
	mustMaterialize(t, s, "v1.0.1", []byte("payload"))
	_ = s.Activate("v1.0.0")
	list, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range list {
		if name == string(ReleaseActive) || name == string(ReleasePrevious) {
			t.Fatal("list returned reserved name", name)
		}
	}
}

func mustMaterialize(t *testing.T, s *ReleaseStore, id string, payload []byte) {
	t.Helper()
	meta := ReleaseMeta{ID: id, Architecture: "amd64", SourceVersion: "1.0.0-rc.1", TargetVersion: id}
	if err := s.Materialize(meta, payload); err != nil {
		t.Fatal(err)
	}
}

func mustDerivePublic(priv ed25519.PrivateKey) ed25519.PublicKey {
	return priv.Public().(ed25519.PublicKey)
}

var _ = rand.Read
var _ = context.Background
var _ = errors.New
