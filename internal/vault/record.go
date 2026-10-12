// Package vault implements the secret-record boundary, separate from canonical
// configuration. Storage ownership and installation key lifecycle are separate.
package vault

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"log/slog"
	"regexp"
)

const MaxSecretBytes = 16 << 10
const MaxRecordBytes = 24 << 10

var ErrRecord = errors.New("secret record is invalid or unavailable")
var opaqueID = regexp.MustCompile(`^[0-9a-f]{32}$`)

type Reference struct {
	ID       string `json:"id"`
	Revision string `json:"revision"`
	Purpose  string `json:"purpose"`
	KeyID    string `json:"keyId"`
}

func (r Reference) valid() bool {
	if !opaqueID.MatchString(r.ID) || !opaqueID.MatchString(r.Revision) || !opaqueID.MatchString(r.KeyID) {
		return false
	}
	switch r.Purpose {
	case "wireguard-private", "wireguard-preshared", "subscription-token", "xray-credential", "netbird-setup", "tls-private",
		// BETA.2 P065 typed vault binding for Xray TLS / REALITY.
		// "tls-certificate" carries a PEM-encoded X.509 certificate
		// for Xray Server.TLSCertRef. Distinct from "tls-private"
		// so a cert and a key cannot be interchanged.
		// "reality-private" carries the 32-byte X25519 private key
		// for Xray Server.RealityKeyRef. Semantically distinct
		// from "wireguard-private" (which also carries a 32-byte
		// Curve25519 key) so an operator cannot bind a WireGuard
		// identity into a REALITY server by accident.
		// "reality-shortid" carries the 8-byte / 16-hex-char short
		// ID for Xray Server.ShortIDRef. The bytes never travel in
		// the protocol body.
		"tls-certificate", "reality-private", "reality-shortid":
		return true
	default:
		return false
	}
}

type record struct {
	Version    int       `json:"version"`
	Reference  Reference `json:"reference"`
	Salt       []byte    `json:"salt"`
	Ciphertext []byte    `json:"ciphertext"`
}

type Secret struct{ data []byte }

// Reveal returns an owned copy for an authorized consumer. It must not be logged.
func (s *Secret) Reveal() []byte              { return bytes.Clone(s.data) }
func (s *Secret) Clear()                      { clear(s.data); s.data = nil }
func (s Secret) String() string               { return "[secret]" }
func (s Secret) GoString() string             { return s.String() }
func (s Secret) LogValue() slog.Value         { return slog.StringValue(s.String()) }
func (s Secret) MarshalJSON() ([]byte, error) { return nil, ErrRecord }

func contextBytes(ref Reference) []byte {
	raw, _ := json.Marshal(ref)
	return append([]byte("DuckOS/vault/record/v1\x00"), raw...)
}

func recordCipher(master, salt, context []byte) (cipher.AEAD, error) {
	if len(master) != 32 || len(salt) != 32 || bytes.Equal(master, make([]byte, 32)) {
		return nil, ErrRecord
	}
	key, err := hkdf.Key(sha256.New, master, salt, string(context), 32)
	if err != nil {
		return nil, ErrRecord
	}
	defer clear(key)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, ErrRecord
	}
	return cipher.NewGCMWithRandomNonce(block)
}

func Seal(master []byte, ref Reference, plaintext []byte) ([]byte, error) {
	if !ref.valid() || len(plaintext) == 0 || len(plaintext) > MaxSecretBytes {
		return nil, ErrRecord
	}
	salt := make([]byte, 32)
	if _, err := rand.Read(salt); err != nil {
		return nil, ErrRecord
	}
	context := contextBytes(ref)
	aead, err := recordCipher(master, salt, context)
	if err != nil {
		return nil, ErrRecord
	}
	r := record{Version: 1, Reference: ref, Salt: salt, Ciphertext: aead.Seal(nil, nil, plaintext, context)}
	return json.Marshal(r)
}

func Open(master []byte, expected Reference, raw []byte) (*Secret, error) {
	if !expected.valid() || len(raw) == 0 || len(raw) > MaxRecordBytes {
		return nil, ErrRecord
	}
	var r record
	if json.Unmarshal(raw, &r) != nil || r.Version != 1 || r.Reference != expected || len(r.Salt) != 32 || len(r.Ciphertext) < 29 || len(r.Ciphertext) > MaxSecretBytes+28 {
		return nil, ErrRecord
	}
	// Only the canonical encoding is persisted. Exact re-encoding also rejects
	// duplicate/unknown fields and alternate ambiguous representations.
	canonical, err := json.Marshal(r)
	if err != nil || !bytes.Equal(canonical, raw) {
		return nil, ErrRecord
	}
	context := contextBytes(expected)
	aead, err := recordCipher(master, r.Salt, context)
	if err != nil {
		return nil, ErrRecord
	}
	plaintext, err := aead.Open(nil, nil, r.Ciphertext, context)
	if err != nil {
		return nil, ErrRecord
	}
	return &Secret{data: plaintext}, nil
}
