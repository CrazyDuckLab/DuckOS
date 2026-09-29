package identity

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"strings"
	"time"
)

const forwardedPrefix = "duckos-forwarded-v1:"
const forwardedTTL = 30 * time.Second

type forwardedClaim struct {
	Username string `json:"username"`
	Role     Role   `json:"role"`
	IssuedAt int64  `json:"iat"`
	Expires  int64  `json:"exp"`
}

func ForwardedToken(secret string, u User) string {
	return forwardedTokenAt(secret, u, time.Now().UTC())
}

func forwardedTokenAt(secret string, u User, now time.Time) string {
	claim := forwardedClaim{Username: u.Username, Role: u.Role, IssuedAt: now.Unix(), Expires: now.Add(forwardedTTL).Unix()}
	data, _ := json.Marshal(claim)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(data)
	sig := mac.Sum(nil)
	return forwardedPrefix + base64.RawURLEncoding.EncodeToString(data) + "." + base64.RawURLEncoding.EncodeToString(sig)
}

type ForwardedAccess struct {
	Secret string
	Now    func() time.Time
}

func (a ForwardedAccess) Authorize(token string, p Permission) (User, error) {
	if a.Secret == "" {
		return User{}, ErrUnauthorized
	}
	raw, ok := strings.CutPrefix(token, forwardedPrefix)
	if !ok {
		return User{}, ErrUnauthorized
	}
	payload, sigText, ok := strings.Cut(raw, ".")
	if !ok {
		return User{}, ErrUnauthorized
	}
	data, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return User{}, ErrUnauthorized
	}
	sig, err := base64.RawURLEncoding.DecodeString(sigText)
	if err != nil {
		return User{}, ErrUnauthorized
	}
	mac := hmac.New(sha256.New, []byte(a.Secret))
	_, _ = mac.Write(data)
	if !hmac.Equal(sig, mac.Sum(nil)) {
		return User{}, ErrUnauthorized
	}
	var claim forwardedClaim
	if err := json.Unmarshal(data, &claim); err != nil {
		return User{}, ErrUnauthorized
	}
	if !usernamePattern.MatchString(claim.Username) || !validRole(claim.Role) {
		return User{}, ErrUnauthorized
	}
	now := time.Now().UTC()
	if a.Now != nil {
		now = a.Now().UTC()
	}
	if claim.IssuedAt <= 0 || claim.Expires <= claim.IssuedAt || now.Unix() < claim.IssuedAt-5 || now.Unix() > claim.Expires {
		return User{}, ErrUnauthorized
	}
	if !Allowed(claim.Role, p) {
		return User{}, ErrForbidden
	}
	return User{Username: claim.Username, Role: claim.Role}, nil
}
