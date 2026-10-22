// Package xray / reality.go: bounded REALITY key material
// generation. Key generation uses crypto/ecdh X25519 with the Go
// standard library CSPRNG; the private key never leaves the caller
// except as the return value that the operator stores into the
// vault. No private bytes are logged or rendered into canonical
// state.
package xray

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
)

// RealityKeypair is a freshly generated X25519 keypair in the exact
// encodings Xray-core consumes: both halves are 43-character
// unpadded URL-safe base64.
type RealityKeypair struct {
	PublicKey  string // share with clients (pbk link parameter)
	PrivateKey string
}

// GenerateRealityKeypair derives a new REALITY X25519 keypair.
// The private key must be stored into the vault under the
// "reality-private" purpose; the public key is canonical state
// (Server.RealityPublicKey).
func GenerateRealityKeypair() (RealityKeypair, error) {
	scheme := ecdh.X25519()
	priv, err := scheme.GenerateKey(rand.Reader)
	if err != nil {
		return RealityKeypair{}, fmt.Errorf("xray: generate reality keypair: %w", err)
	}
	pub := priv.PublicKey().Bytes()
	return RealityKeypair{
		PublicKey:  base64.RawURLEncoding.EncodeToString(pub),
		PrivateKey: base64.RawURLEncoding.EncodeToString(priv.Bytes()),
	}, nil
}

// MaxShortIDHex is the maximum short-id length in hex characters
// (8 bytes); xray-core rejects longer values.
const MaxShortIDHex = 16

// GenerateShortID returns a random short id as a lowercase hex
// string of 16 characters (8 random bytes). Short ids are not
// secret; they disambiguate clients on a shared REALITY inbound
// and are stored under the "reality-shortid" vault purpose.
func GenerateShortID() (string, error) {
	buf := make([]byte, MaxShortIDHex/2)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("xray: generate short id: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

// GenerateUUID returns a random RFC 4122 version 4 UUID in canonical
// lowercase form. Xray-core accepts any UUID string for VLESS/VMess
// clients, but the canonical form keeps share links and catalog
// diffs stable.
func GenerateUUID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("xray: generate uuid: %w", err)
	}
	buf[6] = (buf[6] & 0x0f) | 0x40
	buf[8] = (buf[8] & 0x3f) | 0x80
	h := hex.EncodeToString(buf)
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:32], nil
}

// GenerateCredential returns a random password secret of n bytes,
// base64url-encoded (no padding). Used for Trojan / Shadowsocks /
// SOCKS credentials. The value must be stored into the vault under
// the "xray-credential" purpose.
func GenerateCredential(n int) (string, error) {
	if n < 16 || n > 64 {
		return "", errors.New("xray: credential byte size must be 16..64")
	}
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("xray: generate credential: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
