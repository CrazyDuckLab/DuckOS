package vault

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"testing"
)

func fixture(t *testing.T) ([]byte, Reference) {
	t.Helper()
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	return key, Reference{ID: strings.Repeat("a", 32), Revision: strings.Repeat("b", 32), KeyID: strings.Repeat("c", 32), Purpose: "wireguard-private"}
}

func TestSecretRoundTripAndRedaction(t *testing.T) {
	key, ref := fixture(t)
	plaintext := []byte("synthetic private credential \x00\xff")
	first, err := Seal(key, ref, plaintext)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Seal(key, ref, plaintext)
	if err != nil || bytes.Equal(first, second) || bytes.Contains(first, plaintext) {
		t.Fatal("encryption did not randomize and conceal plaintext")
	}
	secret, err := Open(key, ref, first)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(secret.Reveal(), plaintext) {
		t.Fatal("plaintext mismatch")
	}
	copy := secret.Reveal()
	copy[0] ^= 1
	if !bytes.Equal(secret.Reveal(), plaintext) {
		t.Fatal("caller modified owned secret")
	}
	var logs bytes.Buffer
	slog.New(slog.NewJSONHandler(&logs, nil)).Info("record", "secret", secret)
	if strings.Contains(logs.String(), "synthetic") || strings.Contains(fmt.Sprintf("%v %+v %#v", secret, *secret, secret), "synthetic") {
		t.Fatal("secret logging disclosure")
	}
	if raw, err := json.Marshal(secret); err == nil || bytes.Contains(raw, []byte("synthetic")) {
		t.Fatal("implicit secret serialization")
	}
	secret.Clear()
	if len(secret.Reveal()) != 0 {
		t.Fatal("clear retained owned bytes")
	}
}

func TestSecretReferenceAndCiphertextAuthentication(t *testing.T) {
	key, ref := fixture(t)
	raw, err := Seal(key, ref, []byte("synthetic credential"))
	if err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*Reference){
		func(r *Reference) { r.ID = strings.Repeat("d", 32) },
		func(r *Reference) { r.Revision = strings.Repeat("d", 32) },
		func(r *Reference) { r.KeyID = strings.Repeat("d", 32) },
		func(r *Reference) { r.Purpose = "subscription-token" },
	} {
		other := ref
		mutate(&other)
		if secret, err := Open(key, other, raw); err == nil || secret != nil {
			t.Fatal("record substitution accepted")
		}
		var r record
		if err := json.Unmarshal(raw, &r); err != nil {
			t.Fatal(err)
		}
		r.Reference = other
		altered, _ := json.Marshal(r)
		if secret, err := Open(key, other, altered); err == nil || secret != nil {
			t.Fatal("reference tampering accepted")
		}
	}
	for _, mutate := range []func(*record){
		func(r *record) { r.Salt[0] ^= 1 },
		func(r *record) { r.Ciphertext[0] ^= 1 },
		func(r *record) { r.Ciphertext[len(r.Ciphertext)-1] ^= 1 },
		func(r *record) { r.Version++ },
	} {
		var r record
		if err := json.Unmarshal(raw, &r); err != nil {
			t.Fatal(err)
		}
		mutate(&r)
		altered, _ := json.Marshal(r)
		if secret, err := Open(key, ref, altered); err == nil || secret != nil {
			t.Fatal("encrypted record tampering accepted")
		}
	}
	wrong, _ := fixture(t)
	if secret, err := Open(wrong, ref, raw); err == nil || secret != nil {
		t.Fatal("wrong key accepted")
	}
	for _, invalid := range [][]byte{raw[:len(raw)-1], append([]byte(" "), raw...), bytes.Replace(raw, []byte(`"version":1`), []byte(`"version":1,"version":1`), 1), bytes.Repeat([]byte("x"), MaxRecordBytes+1)} {
		if secret, err := Open(key, ref, invalid); err == nil || secret != nil {
			t.Fatal("invalid encoding accepted")
		}
	}
}

func TestSecretReferenceAcceptsXrayTLSPurposes(t *testing.T) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	for _, purpose := range []string{"tls-certificate", "reality-private", "reality-shortid"} {
		ref := Reference{ID: strings.Repeat("a", 32), Revision: strings.Repeat("b", 32), KeyID: strings.Repeat("c", 32), Purpose: purpose}
		plaintext := []byte("synthetic xray material for " + purpose)
		raw, err := Seal(key, ref, plaintext)
		if err != nil {
			t.Fatalf("Seal(%s): %v", purpose, err)
		}
		secret, err := Open(key, ref, raw)
		if err != nil {
			t.Fatalf("Open(%s): %v", purpose, err)
		}
		if !bytes.Equal(secret.Reveal(), plaintext) {
			t.Fatalf("%s: plaintext mismatch", purpose)
		}
		// Reference validation must accept the new purpose.
		if !ref.valid() {
			t.Fatalf("%s: Reference.valid() false", purpose)
		}
	}
	// And the cross-purpose tamper check must still reject.
	real := Reference{ID: strings.Repeat("a", 32), Revision: strings.Repeat("b", 32), KeyID: strings.Repeat("c", 32), Purpose: "reality-private"}
	raw, err := Seal(key, real, []byte("reality key bytes"))
	if err != nil {
		t.Fatal(err)
	}
	tampered := Reference{ID: strings.Repeat("a", 32), Revision: strings.Repeat("b", 32), KeyID: strings.Repeat("c", 32), Purpose: "wireguard-private"}
	if secret, err := Open(key, tampered, raw); err == nil || secret != nil {
		t.Fatal("reality-private accepted as wireguard-private")
	}
}

func TestSecretLimits(t *testing.T) {
	key, ref := fixture(t)
	for _, size := range []int{1, MaxSecretBytes} {
		plain := bytes.Repeat([]byte{255}, size)
		raw, err := Seal(key, ref, plain)
		if err != nil || len(raw) > MaxRecordBytes {
			t.Fatal("valid record exceeded bounds", err)
		}
		secret, err := Open(key, ref, raw)
		if err != nil || !bytes.Equal(secret.Reveal(), plain) {
			t.Fatal("bounded round trip failed")
		}
	}
	for _, bad := range [][]byte{nil, make([]byte, MaxSecretBytes+1)} {
		if _, err := Seal(key, ref, bad); err == nil {
			t.Fatal("invalid plaintext length accepted")
		}
	}
	for _, bad := range [][]byte{nil, make([]byte, 31), make([]byte, 32), make([]byte, 33)} {
		if _, err := Seal(bad, ref, []byte("secret")); err == nil {
			t.Fatal("invalid master key accepted")
		}
	}
}

func FuzzOpenSecretRecord(f *testing.F) {
	key := bytes.Repeat([]byte{1}, 32)
	ref := Reference{ID: strings.Repeat("a", 32), Revision: strings.Repeat("b", 32), KeyID: strings.Repeat("c", 32), Purpose: "subscription-token"}
	raw, err := Seal(key, ref, []byte("synthetic credential"))
	if err != nil {
		f.Fatal(err)
	}
	f.Add(raw)
	f.Add([]byte(`{}`))
	f.Fuzz(func(t *testing.T, input []byte) {
		secret, err := Open(key, ref, input)
		if err != nil && secret != nil {
			t.Fatal("failed authentication returned plaintext")
		}
		if err == nil && (len(secret.Reveal()) == 0 || len(secret.Reveal()) > MaxSecretBytes) {
			t.Fatal("unbounded decrypted record")
		}
	})
}
