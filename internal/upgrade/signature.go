package upgrade

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"fmt"
)

// GenerateSigningKey returns a fresh Ed25519 key pair. The Agent
// stores the private key in the vault or in a dedicated signing-key
// file. The public key is published in the system fingerprint.
func GenerateSigningKey() (ed25519.PrivateKey, ed25519.PublicKey, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, errors.New("signing key generation unavailable")
	}
	return priv, pub, nil
}

// Ed25519Verifier is the production signature verifier. It refuses
// signatures of any length other than 64 and refuses any public key
// of any length other than 32.
type Ed25519Verifier struct {
	Public ed25519.PublicKey
}

func (v Ed25519Verifier) Verify(manifest, signature []byte) error {
	if len(signature) != ed25519.SignatureSize {
		return fmt.Errorf("signature length %d is invalid", len(signature))
	}
	if len(v.Public) != ed25519.PublicKeySize {
		return errors.New("verifier public key is invalid")
	}
	if !ed25519.Verify(v.Public, manifest, signature) {
		return errors.New("ed25519 signature does not verify")
	}
	return nil
}

// StaticVerifier is a test-only verifier that accepts the signatures
// minted by its companion signer. Production code never uses it.
type StaticVerifier struct {
	accept map[string]bool
}

func NewStaticVerifier(keys ...ed25519.PublicKey) *StaticVerifier {
	accept := map[string]bool{}
	for _, k := range keys {
		accept[string(k)] = true
	}
	return &StaticVerifier{accept: accept}
}

func (s *StaticVerifier) Verify(_ []byte, signature []byte) error {
	if !s.accept[string(signature)] {
		return errors.New("static verifier refused the signature")
	}
	return nil
}
