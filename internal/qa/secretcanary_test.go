package qa

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewCanaryUniqueAndHasMarker(t *testing.T) {
	a := NewCanary("vault")
	b := NewCanary("vault")
	if a.Value == b.Value {
		t.Fatalf("canary values collided: %s", a.Value)
	}
	if a.Marker != "DUCKQA-VAULT" {
		t.Fatalf("unexpected marker: %q", a.Marker)
	}
	// Value must be a long pure hex string so the support
	// redactor treats it as a real secret.
	if len(a.Value) != 64 {
		t.Fatalf("expected 64-char hex value, got %d: %q", len(a.Value), a.Value)
	}
	for _, c := range a.Value {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Fatalf("non-hex rune in value: %q", a.Value)
		}
	}
}

func TestScanForLeakDetectsValue(t *testing.T) {
	dir := TempDir(t)
	c := NewCanary("wireguard")
	if err := os.WriteFile(filepath.Join(dir, "leaky.txt"), []byte("the secret is "+c.Value+" ok"), 0o600); err != nil {
		t.Fatal(err)
	}
	found := ScanForLeak(dir, c)
	if len(found) == 0 {
		t.Fatal("expected leak detection")
	}
}

func TestScanForLeakDetectsMarker(t *testing.T) {
	dir := TempDir(t)
	c := NewCanary("radius")
	if err := os.WriteFile(filepath.Join(dir, "leaky.txt"), []byte("marker only: "+c.Marker), 0o600); err != nil {
		t.Fatal(err)
	}
	found := ScanForLeak(dir, c)
	if len(found) == 0 {
		t.Fatal("expected marker detection")
	}
}

func TestScanForLeakClean(t *testing.T) {
	dir := TempDir(t)
	if err := os.WriteFile(filepath.Join(dir, "clean.txt"), []byte("hello world"), 0o600); err != nil {
		t.Fatal(err)
	}
	c := NewCanary("xray")
	if found := ScanForLeak(dir, c); len(found) != 0 {
		t.Fatalf("unexpected leak: %v", found)
	}
}

func TestContainsLeak(t *testing.T) {
	c := NewCanary("acme")
	if !ContainsLeak("this contains "+c.Marker, c) {
		t.Fatal("expected leak detection in string")
	}
	if !ContainsLeak("this contains "+c.Value, c) {
		t.Fatal("expected value detection in string")
	}
	if ContainsLeak("clean text", c) {
		t.Fatal("false positive leak detection")
	}
}

func TestCanaryMustSeedFile(t *testing.T) {
	dir := TempDir(t)
	c := NewCanary("vault")
	p := c.MustSeedFile(t, dir, "secret.txt")
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != c.Value {
		t.Fatalf("seeded file does not contain canary value")
	}
}
