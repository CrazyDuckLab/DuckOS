// Package persist tests: atomicity, round-trip, permission, repair
// semantics of the bounded JSON file store.
package persist

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

type doc struct {
	Items []string `json:"items"`
	N     int      `json:"n"`
}

func TestRoundTripAndUpdate(t *testing.T) {
	dir := t.TempDir()
	f, err := Open[doc](dir, "test.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Update(func(d *doc) { d.Items = []string{"a", "b"}; d.N = 2 }); err != nil {
		t.Fatal(err)
	}
	g, err := Open[doc](dir, "test.json")
	if err != nil {
		t.Fatal(err)
	}
	got := g.Get()
	if got.N != 2 || len(got.Items) != 2 {
		t.Fatalf("round trip: %+v", got)
	}
	info, err := os.Stat(filepath.Join(dir, "test.json"))
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("permissions %o, want 600", perm)
	}
}

func TestUpdateRollbackOnPersistFailure(t *testing.T) {
	dir := t.TempDir()
	f, _ := Open[doc](dir, "test.json")
	_ = f.Update(func(d *doc) { d.N = 1 })
	// Make the persist path fail: replace the target file with a
	// directory so the final rename cannot succeed.
	if err := os.Remove(filepath.Join(dir, "test.json")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "test.json"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := f.Update(func(d *doc) { d.N = 99 }); err == nil {
		t.Fatal("persist error expected")
	}
	if got := f.Get(); got.N != 1 {
		t.Fatalf("in-memory state diverged after failure: %+v", got)
	}
}

func TestFreshFileIsZeroValue(t *testing.T) {
	f, err := Open[doc](t.TempDir(), "fresh.json")
	if err != nil {
		t.Fatal(err)
	}
	if got := f.Get(); got.N != 0 || got.Items != nil {
		t.Fatalf("fresh state not zero: %+v", got)
	}
}

func TestConcurrentUpdates(t *testing.T) {
	f, _ := Open[doc](t.TempDir(), "c.json")
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = f.Update(func(d *doc) { d.N++ })
		}()
	}
	wg.Wait()
	if got := f.Get().N; got != 16 {
		t.Fatalf("lost updates: %d", got)
	}
}

func TestRejectsPathTraversal(t *testing.T) {
	if _, err := Open[doc](t.TempDir(), "../evil.json"); err == nil {
		t.Fatal("path traversal accepted")
	}
}
