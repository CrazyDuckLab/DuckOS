package identity

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestForwardedAccessRequiresValidSignatureAndRole(t *testing.T) {
	user := User{Username: "owner", Role: Owner}
	token := ForwardedToken("secret-a", user)
	if _, err := (ForwardedAccess{Secret: "secret-b"}).Authorize(token, ConfigureNetwork); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("wrong secret got %v, want ErrUnauthorized", err)
	}
	if _, err := (ForwardedAccess{Secret: "secret-a"}).Authorize(token, ConfigureNetwork); err != nil {
		t.Fatalf("valid owner assertion rejected: %v", err)
	}
	reader := ForwardedToken("secret-a", User{Username: "reader", Role: ReadOnly})
	if _, err := (ForwardedAccess{Secret: "secret-a"}).Authorize(reader, ConfigureNetwork); !errors.Is(err, ErrForbidden) {
		t.Fatalf("read-only assertion got %v, want ErrForbidden", err)
	}
}

func TestForwardedAccessRejectsTamperAndExpiredClaims(t *testing.T) {
	now := time.Unix(1000, 0).UTC()
	token := forwardedTokenAt("secret-a", User{Username: "reader", Role: ReadOnly}, now)
	raw, _ := strings.CutPrefix(token, forwardedPrefix)
	payload, sig, _ := strings.Cut(raw, ".")
	data, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		t.Fatal(err)
	}
	var claim forwardedClaim
	if err := json.Unmarshal(data, &claim); err != nil {
		t.Fatal(err)
	}
	claim.Role = Owner
	tampered, _ := json.Marshal(claim)
	tamperedToken := forwardedPrefix + base64.RawURLEncoding.EncodeToString(tampered) + "." + sig
	if _, err := (ForwardedAccess{Secret: "secret-a", Now: func() time.Time { return now }}).Authorize(tamperedToken, ConfigureNetwork); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("tampered role got %v, want ErrUnauthorized", err)
	}
	if _, err := (ForwardedAccess{Secret: "secret-a", Now: func() time.Time { return now.Add(time.Minute) }}).Authorize(token, Observe); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("expired assertion got %v, want ErrUnauthorized", err)
	}
}
