//go:build linux

// nftables lifecycle regression tests (beta.3 hardening).
//
// These tests exercise the rule lifecycle against the real
// render path and the real backend state machine with
// validateOnly (no kernel mutation, no nft binary required
// beyond a fake path). Kernel-level behaviour is NOT claimed
// verified here; live qualification remains pending.
package upnp

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// ExternalIPString exposes the internal host for batch assertions.
func (m Mapping) ExternalIPString() string { return m.InternalHost }

func mappingA() Mapping {
	return Mapping{ID: "pcp-tcp-192.168.10.50-5000", NetworkRef: "lan-1", Protocol: ProtoPCP, Transport: TransportTCP,
		ClientIP: "192.168.10.50", InternalHost: "192.168.10.50", ExternalPort: 5000, InternalPort: 5000}
}

func mappingB() Mapping {
	return Mapping{ID: "nat-pmp-udp-192.168.10.51-8080", NetworkRef: "lan-1", Protocol: ProtoNATPMP, Transport: TransportUDP,
		ClientIP: "192.168.10.51", InternalHost: "192.168.10.51", ExternalPort: 8080, InternalPort: 8080}
}

func newValidatingBackend(t *testing.T) *NftablesBackend {
	t.Helper()
	dir := t.TempDir()
	b := &NftablesBackend{binPath: "/bin/true", stagingDir: dir, validateOnly: true}
	return b
}

func lastBatch(t *testing.T, b *NftablesBackend) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(b.stagingDir, "duckos-upnp.batch.nft"))
	if err != nil {
		t.Fatalf("batch read: %v", err)
	}
	return string(data)
}

func TestRenderTableIsSelfContained(t *testing.T) {
	data, err := renderTable([]Mapping{mappingA()})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	text := string(data)
	// The add/delete/add idiom must be present so the batch is
	// correct whether or not the table exists and clears stale
	// rules from a previous run.
	if strings.Count(text, "table inet duckos_upnp") < 3 {
		t.Fatalf("not self-contained: %s", text)
	}
	if !strings.Contains(text, "delete table inet duckos_upnp") {
		t.Fatal("missing delete table")
	}
	if strings.Count(text, "add rule") != 1 {
		t.Fatalf("rule count: %s", text)
	}
}

func TestRemoveMappingAKeepsMappingB(t *testing.T) {
	a, b := mappingA(), mappingB()
	backend := newValidatingBackend(t)
	if err := backend.Add(a); err != nil {
		t.Fatalf("add A: %v", err)
	}
	if err := backend.Add(b); err != nil {
		t.Fatalf("add B: %v", err)
	}
	if err := backend.Remove(a.ID); err != nil {
		t.Fatalf("remove A: %v", err)
	}
	text := lastBatch(t, backend)
	if strings.Contains(text, a.ExternalIPString()) {
		t.Fatalf("removed mapping still rendered: %s", text)
	}
	if !strings.Contains(text, b.InternalHost) {
		t.Fatalf("surviving mapping lost: %s", text)
	}
	if strings.Count(text, "add rule") != 1 {
		t.Fatalf("rule count after removal: %s", text)
	}
}

func TestRemoveUnknownIDIsIdempotent(t *testing.T) {
	b := newValidatingBackend(t)
	if err := b.Remove("nonexistent"); err != nil {
		t.Fatalf("unknown remove: %v", err)
	}
	if len(b.active) != 0 {
		t.Fatalf("active changed: %v", b.active)
	}
}

func TestRenewalReplacesInsteadOfDuplicating(t *testing.T) {
	b := newValidatingBackend(t)
	a := mappingA()
	if err := b.Add(a); err != nil {
		t.Fatalf("add: %v", err)
	}
	a.ExpiresAt = a.ExpiresAt.Add(0) // renewal is the same tuple
	if err := b.Add(a); err != nil {
		t.Fatalf("renewal: %v", err)
	}
	if got := len(b.active); got != 1 {
		t.Fatalf("renewal duplicated: %d", got)
	}
	if strings.Count(lastBatch(t, b), "add rule") != 1 {
		t.Fatalf("batch has duplicate rule: %s", lastBatch(t, b))
	}
}

func TestFailedNftLeavesActiveUntouched(t *testing.T) {
	dir := t.TempDir()
	// /bin/false exists on Linux and always exits non-zero.
	b := &NftablesBackend{binPath: "/bin/false", stagingDir: dir}
	a := mappingA()
	if err := b.Add(a); err == nil {
		t.Fatal("expected nft failure")
	}
	if len(b.active) != 0 {
		t.Fatalf("active mutated on failure: %v", b.active)
	}
	// A later successful Add must still include the mapping.
	b.validateOnly = true
	if err := b.Add(a); err != nil {
		t.Fatalf("recovery add: %v", err)
	}
	if len(b.active) != 1 {
		t.Fatalf("active after recovery: %v", b.active)
	}
}

func TestConcurrentAddRemoveConsistent(t *testing.T) {
	b := newValidatingBackend(t)
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			m := mappingA()
			m.ID = m.ID + string(rune('a'+i%26))
			m.ExternalPort = 20000 + i
			_ = b.Add(m)
		}(i)
		go func(i int) {
			defer wg.Done()
			m := mappingA()
			m.ID = m.ID + string(rune('a'+i%26))
			m.ExternalPort = 20000 + i
			_ = b.Remove(m.ID)
		}(i)
	}
	wg.Wait()
	// Whatever the interleaving, the rendered batch must equal
	// the active set exactly: no duplicates, no ghosts.
	text := lastBatch(t, b)
	rules := strings.Count(text, "add rule")
	if rules != len(b.active) {
		t.Fatalf("batch rules %d != active %d", rules, len(b.active))
	}
	seen := map[string]bool{}
	for _, m := range b.active {
		if seen[m.ID] {
			t.Fatalf("duplicate id in active set: %s", m.ID)
		}
		seen[m.ID] = true
	}
}

func TestRuleOrderIsDeterministic(t *testing.T) {
	a, b := mappingA(), mappingB()
	data, err := renderTable([]Mapping{b, a})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	text := string(data)
	if strings.Index(text, "nat-pmp-udp") > strings.Index(text, "pcp-tcp") {
		t.Fatalf("rule order not sorted by id:\n%s", text)
	}
}
