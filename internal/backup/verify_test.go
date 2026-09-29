package backup

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"
	"time"
)

func TestEd25519VerifierRoundTrip(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := Build([]Entry{newEntry("a", "config", "x")}, "h", "v", "", "admin", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	data, err := EncodeManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	sig := ed25519.Sign(priv, data)
	v := Ed25519Verifier{Public: pub}
	if err := v.Verify(data, sig); err != nil {
		t.Fatal(err)
	}
}

func TestEd25519VerifierRejectsBadSignature(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	bad := make([]byte, ed25519.SignatureSize)
	v := Ed25519Verifier{Public: pub}
	if err := v.Verify([]byte("manifest"), bad); err == nil {
		t.Fatal("zero signature accepted")
	}
}

func TestEd25519VerifierRejectsBadKey(t *testing.T) {
	v := Ed25519Verifier{Public: []byte("too-short")}
	if err := v.Verify([]byte("x"), make([]byte, 64)); err == nil {
		t.Fatal("invalid key accepted")
	}
}

func TestStaticVerifier(t *testing.T) {
	sig := []byte{1, 2, 3, 4}
	v := StaticVerifier{Expected: sig}
	if err := v.Verify([]byte("x"), sig); err != nil {
		t.Fatal(err)
	}
	if err := v.Verify([]byte("x"), []byte{1, 2, 3}); err == nil {
		t.Fatal("size mismatch accepted")
	}
	if err := v.Verify([]byte("x"), []byte{1, 2, 3, 5}); err == nil {
		t.Fatal("different signature accepted")
	}
}

func TestVerifyDigest(t *testing.T) {
	body := []byte("hello world")
	want := Digest(body)
	if err := VerifyDigest(body, want); err != nil {
		t.Fatal(err)
	}
	if err := VerifyDigest(body, "bad"); err == nil {
		t.Fatal("bad digest accepted")
	}
	if err := VerifyDigest([]byte("other"), want); err == nil {
		t.Fatal("mismatched body accepted")
	}
}

func TestInspectRejectsBadMagic(t *testing.T) {
	env := &Envelope{Header: []byte("wrong")}
	if _, err := Inspect(env); err == nil {
		t.Fatal("bad magic accepted")
	}
}

func TestVerifyEnvelopeRoundTrip(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	manifest, _ := Build([]Entry{newEntry("a", "config", "x")}, "h", "v", "", "admin", time.Now().UTC())
	data, _ := EncodeManifest(manifest)
	sig := ed25519.Sign(priv, data)
	env := &Envelope{Header: []byte(MagicHeader), Signature: sig, ManifestJSON: data}
	if err := VerifyEnvelope(env, Ed25519Verifier{Public: pub}); err != nil {
		t.Fatal(err)
	}
	if len(env.Manifest.Entries) != 1 {
		t.Fatal("manifest not populated after verification")
	}
}

func TestVerifyEnvelopeRejectsBadSignature(t *testing.T) {
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	manifest, _ := Build([]Entry{newEntry("a", "config", "x")}, "h", "v", "", "admin", time.Now().UTC())
	data, _ := EncodeManifest(manifest)
	env := &Envelope{Header: []byte(MagicHeader), Signature: make([]byte, 64), ManifestJSON: data}
	if err := VerifyEnvelope(env, Ed25519Verifier{Public: pub}); err == nil {
		t.Fatal("bad signature accepted")
	}
}

func TestVerifyEnvelopeRejectsBadMagic(t *testing.T) {
	env := &Envelope{Header: []byte("wrong")}
	if err := VerifyEnvelope(env, StaticVerifier{}); err == nil {
		t.Fatal("bad magic accepted")
	}
}

func TestVerifyEnvelopeRejectsMalformedManifest(t *testing.T) {
	env := &Envelope{Header: []byte(MagicHeader), Signature: []byte{}, ManifestJSON: []byte("not json")}
	if err := VerifyEnvelope(env, StaticVerifier{}); err == nil {
		t.Fatal("malformed manifest accepted")
	}
}
