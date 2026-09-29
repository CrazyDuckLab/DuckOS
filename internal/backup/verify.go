package backup

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"errors"
)

// Verifier is the contract a signature implementation must
// satisfy. The reference implementation is the Ed25519
// production path; tests use a StaticVerifier.
type Verifier interface {
	Verify(manifest []byte, signature []byte) error
}

// Ed25519Verifier verifies a detached Ed25519 signature over
// the canonical manifest. The public key is the 32-byte Ed25519
// verification key. The signature is the 64-byte Ed25519
// detached signature.
type Ed25519Verifier struct {
	Public ed25519.PublicKey
}

// Verify returns nil when the signature is valid, an error
// otherwise.
func (v Ed25519Verifier) Verify(manifest, signature []byte) error {
	if len(v.Public) != ed25519.PublicKeySize {
		return errors.New("backup: invalid verifier public key")
	}
	if len(signature) != ed25519.SignatureSize {
		return errors.New("backup: signature has the wrong size")
	}
	if !ed25519.Verify(v.Public, manifest, signature) {
		return errors.New("backup: signature did not verify")
	}
	return nil
}

// StaticVerifier is a test-only verifier that accepts exactly
// one signature. It is never instantiated in production code.
type StaticVerifier struct {
	Expected []byte
}

// Verify returns nil when the supplied signature matches the
// expected value.
func (s StaticVerifier) Verify(_ []byte, signature []byte) error {
	if len(signature) != len(s.Expected) {
		return errors.New("backup: static signature size mismatch")
	}
	for i := range signature {
		if signature[i] != s.Expected[i] {
			return errors.New("backup: static signature did not match")
		}
	}
	return nil
}

// Inspect returns the manifest encoded in the envelope. The
// inspector never touches the payload: a partially-truncated
// archive is still safe to inspect as long as the manifest is
// intact.
func Inspect(envelope *Envelope) (Manifest, error) {
	if envelope == nil {
		return Manifest{}, errors.New("backup: envelope is nil")
	}
	if string(envelope.Header) != MagicHeader {
		return Manifest{}, errors.New("backup: invalid magic header")
	}
	return DecodeManifest(envelope.ManifestJSON)
}

// VerifyEnvelope runs the signature check on a sealed envelope.
// The function is the single entry point the API uses before
// handing an envelope to the restorer.
func VerifyEnvelope(envelope *Envelope, v Verifier) error {
	if envelope == nil {
		return errors.New("backup: envelope is nil")
	}
	if string(envelope.Header) != MagicHeader {
		return errors.New("backup: invalid magic header")
	}
	manifest, err := DecodeManifest(envelope.ManifestJSON)
	if err != nil {
		return err
	}
	if err := v.Verify(envelope.ManifestJSON, envelope.Signature); err != nil {
		return err
	}
	envelope.Manifest = manifest
	return nil
}

// VerifyDigest checks the supplied bytes against a canonical
// hex sha256. The function is the single helper the restorer
// uses to validate the per-entry payload before writing it to
// disk.
func VerifyDigest(body []byte, expected string) error {
	if len(expected) != 64 {
		return errors.New("backup: digest has the wrong size")
	}
	sum := sha256.Sum256(body)
	got := hex.EncodeToString(sum[:])
	if got != expected {
		return errors.New("backup: digest mismatch")
	}
	return nil
}
