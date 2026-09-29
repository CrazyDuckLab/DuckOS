package identity

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"unicode/utf8"
)

const passwordIterations = 600000

type passwordHash struct {
	Algorithm  string `json:"algorithm"`
	Iterations int    `json:"iterations"`
	Salt       string `json:"salt"`
	Digest     string `json:"digest"`
}

func passwordValid(password string) bool {
	return utf8.ValidString(password) && utf8.RuneCountInString(password) >= 15 && utf8.RuneCountInString(password) <= 128 && len(password) <= 512
}
func hashPassword(password string) (passwordHash, error) {
	if !passwordValid(password) {
		return passwordHash{}, errors.New("password requires 15–128 characters, at most 512 bytes")
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return passwordHash{}, err
	}
	key, err := pbkdf2.Key(sha256.New, password, salt, passwordIterations, 32)
	if err != nil {
		return passwordHash{}, err
	}
	return passwordHash{Algorithm: "pbkdf2-sha256", Iterations: passwordIterations, Salt: base64.RawStdEncoding.EncodeToString(salt), Digest: base64.RawStdEncoding.EncodeToString(key)}, nil
}
func validHash(h passwordHash) bool {
	s, e := base64.RawStdEncoding.DecodeString(h.Salt)
	d, f := base64.RawStdEncoding.DecodeString(h.Digest)
	return h.Algorithm == "pbkdf2-sha256" && h.Iterations == passwordIterations && e == nil && f == nil && len(s) == 16 && len(d) == 32
}
func verifyPassword(h passwordHash, password string) bool {
	if !validHash(h) || len(password) > 512 {
		return false
	}
	salt, _ := base64.RawStdEncoding.DecodeString(h.Salt)
	want, _ := base64.RawStdEncoding.DecodeString(h.Digest)
	key, err := pbkdf2.Key(sha256.New, password, salt, h.Iterations, 32)
	return err == nil && subtle.ConstantTimeCompare(key, want) == 1
}
