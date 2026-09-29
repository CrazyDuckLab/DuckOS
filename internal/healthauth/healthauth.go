// Package healthauth authenticates local updater readiness probes only.
// Its signing key is independent of user sessions and Agent assertions.
package healthauth

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"time"
)

const Path = "/api/v1/ready"
const prefix = "duckos-updater-health-v1:"
const ttl = 15 * time.Second

type claim struct {
	Audience string
	Method   string
	Path     string
	Issued   int64
	Expires  int64
	Nonce    string
}

func Sign(key ed25519.PrivateKey, now time.Time) string {
	if len(key) != ed25519.PrivateKeySize {
		return ""
	}
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return ""
	}
	raw, _ := json.Marshal(claim{"duckos-api-readiness", http.MethodGet, Path, now.Unix(), now.Add(ttl).Unix(), base64.RawURLEncoding.EncodeToString(nonce)})
	return prefix + base64.RawURLEncoding.EncodeToString(raw) + "." + base64.RawURLEncoding.EncodeToString(ed25519.Sign(key, raw))
}

func Verify(key ed25519.PublicKey, r *http.Request, now time.Time) bool {
	if len(key) != ed25519.PublicKeySize || r.Method != http.MethodGet || r.URL.Path != Path || r.URL.RawPath != "" || r.URL.RawQuery != "" || r.URL.ForceQuery || r.ContentLength != 0 || len(r.TransferEncoding) != 0 || len(r.Header.Values("Authorization")) != 1 {
		return false
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil || !net.ParseIP(host).IsLoopback() {
		return false
	}
	text, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "+prefix)
	if !ok || len(text) > 1024 {
		return false
	}
	body, sig, ok := strings.Cut(text, ".")
	if !ok {
		return false
	}
	raw, err := base64.RawURLEncoding.DecodeString(body)
	if err != nil {
		return false
	}
	signature, err := base64.RawURLEncoding.DecodeString(sig)
	if err != nil || !ed25519.Verify(key, raw, signature) {
		return false
	}
	var c claim
	if json.Unmarshal(raw, &c) != nil {
		return false
	}
	nonce, err := base64.RawURLEncoding.DecodeString(c.Nonce)
	return err == nil && len(nonce) == 16 && c.Audience == "duckos-api-readiness" && c.Method == r.Method && c.Path == r.URL.Path && c.Expires-c.Issued == int64(ttl/time.Second) && c.Issued > 0 && now.Unix() >= c.Issued-2 && now.Unix() < c.Expires
}
