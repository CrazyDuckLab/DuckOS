package healthauth

import (
	"crypto/ed25519"
	"crypto/rand"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAssertionExactScopeAndExpiry(t *testing.T) {
	pub, key, _ := ed25519.GenerateKey(rand.Reader)
	now := time.Now()
	for _, tc := range []struct {
		method, path, remote string
		offset               time.Duration
		want                 bool
	}{
		{"GET", Path, "127.0.0.1:42", 0, true}, {"GET", Path, "[::1]:42", 0, true},
		{"POST", Path, "127.0.0.1:42", 0, false}, {"GET", "/api/v1/overview", "127.0.0.1:42", 0, false},
		{"GET", Path, "198.51.100.1:42", 0, false}, {"GET", Path, "127.0.0.1:42", 15 * time.Second, false},
	} {
		r := httptest.NewRequest(tc.method, "http://localhost"+tc.path, nil)
		r.RemoteAddr = tc.remote
		r.Header.Set("Authorization", "Bearer "+Sign(key, now))
		if Verify(pub, r, now.Add(tc.offset)) != tc.want {
			t.Fatal(tc)
		}
	}
	r := httptest.NewRequest("GET", "http://localhost"+Path, nil)
	r.RemoteAddr = "127.0.0.1:42"
	r.Header.Add("Authorization", "Bearer "+Sign(key, now))
	r.Header.Add("Authorization", "Bearer "+Sign(key, now))
	if Verify(pub, r, now) {
		t.Fatal("duplicate authorization accepted")
	}
}

func TestRootOwnedHealthKeyProvisioning(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("root-owned key provisioning requires root")
	}
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	private, public := filepath.Join(dir, "key"), filepath.Join(dir, "pub")
	if err := Provision(private, public); err != nil {
		t.Fatal(err)
	}
	key, err := LoadPrivate(private)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(key)
	pub, err := LoadPublic(public)
	if err != nil || !key.Public().(ed25519.PublicKey).Equal(pub) {
		t.Fatal("key mismatch", err)
	}
	if err := Provision(private, public); err == nil {
		t.Fatal("existing keys overwritten")
	}
	if err := os.Chmod(private, 0640); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadPrivate(private); err == nil {
		t.Fatal("group-readable private key accepted")
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(public, link); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadPublic(link); err == nil {
		t.Fatal("symlink public key accepted")
	}
}
