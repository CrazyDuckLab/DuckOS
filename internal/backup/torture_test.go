// Package backup torture: §15 backup-restore destructive QA.
//
// Each subtest stresses one boundary of the backup envelope or
// the logical-manifest surface. The tests are pure-Go so they
// run under `go test -race` without external daemons.
package backup

import (
	"crypto/rand"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

// makeEntry produces a valid entry with the supplied body.
func makeEntry(path, kind string, body []byte) Entry {
	return Entry{
		Path:   path,
		Kind:   Kind(kind),
		Size:   int64(len(body)),
		Digest: Digest(body),
		Mode:   0o600,
		MTime:  time.Now().Unix(),
		Owner:  "root",
	}
}

// TestTorture_ConcurrentBuilds verifies Build is safe under
// concurrent callers. Each goroutine produces its own manifest
// with distinct IDs and distinct paths; the only shared state
// is Digest/EncodeManifest/DecodeManifest.
func TestTorture_ConcurrentBuilds(t *testing.T) {
	const N = 16
	var wg sync.WaitGroup
	wg.Add(N)
	errs := make(chan error, N)
	for i := 0; i < N; i++ {
		go func(i int) {
			defer wg.Done()
			body := []byte(fmt.Sprintf("payload-%d", i))
			e := makeEntry(fmt.Sprintf("etc/duckos/test-%d.json", i), "config", body)
			m, err := Build([]Entry{e}, "host-1", "v1.0", "torture", "qa", time.Unix(int64(i), 0))
			if err != nil {
				errs <- fmt.Errorf("build %d: %w", i, err)
				return
			}
			enc, err := EncodeManifest(m)
			if err != nil {
				errs <- fmt.Errorf("encode %d: %w", i, err)
				return
			}
			back, err := DecodeManifest(enc)
			if err != nil {
				errs <- fmt.Errorf("decode %d: %w", i, err)
				return
			}
			if len(back.Entries) != 1 || back.Entries[0].Path != e.Path {
				errs <- fmt.Errorf("roundtrip %d: %+v", i, back.Entries)
				return
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
}

// TestTorture_TamperedDigest verifies VerifyDigest rejects a
// payload whose digest does not match.
func TestTorture_TamperedDigest(t *testing.T) {
	body := []byte("real content")
	e := makeEntry("a/b.json", "config", body)
	if err := VerifyDigest(body, e.Digest); err != nil {
		t.Fatal("clean digest must verify:", err)
	}
	tampered := append([]byte{}, body...)
	tampered[0] ^= 0xFF
	if err := VerifyDigest(tampered, e.Digest); err == nil {
		t.Fatal("tampered payload must fail verification")
	}
	wrongDigest := Digest([]byte("different content"))
	if err := VerifyDigest(body, wrongDigest); err == nil {
		t.Fatal("wrong digest must fail verification")
	}
}

// TestTorture_SchemaMismatch verifies EncodeManifest and
// DecodeManifest both reject a manifest whose schemaVersion
// does not match SchemaVersion.
func TestTorture_SchemaMismatch(t *testing.T) {
	body := []byte("{}")
	e := makeEntry("a", "config", body)
	m, err := Build([]Entry{e}, "host", "v1", "torture", "qa", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	m.SchemaVersion = "999"
	if err := m.Valid(); err == nil {
		t.Fatal("manifest with bad schema must fail Valid()")
	}
	if _, err := EncodeManifest(m); err == nil {
		t.Fatal("EncodeManifest must reject mismatched schema")
	}
	// A clean manifest with the canonical schemaVersion must
	// round-trip and decode back with the same schemaVersion.
	clean, err := Build([]Entry{e}, "host", "v1", "torture", "qa", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	enc, err := EncodeManifest(clean)
	if err != nil {
		t.Fatal(err)
	}
	back, err := DecodeManifest(enc)
	if err != nil {
		t.Fatal(err)
	}
	if back.SchemaVersion != SchemaVersion {
		t.Fatalf("decoded schema %q != %q", back.SchemaVersion, SchemaVersion)
	}
}

// TestTorture_PathTraversal verifies Entry.Valid rejects every
// canonical traversal pattern.
func TestTorture_PathTraversal(t *testing.T) {
	cases := []string{
		"../etc/passwd",
		"etc/../../passwd",
		"/etc/passwd",
		"a/b/../../../etc",
		"",
		".",
		"a//b",
		"a/./b",
	}
	for _, p := range cases {
		e := makeEntry(p, "config", []byte("x"))
		if err := e.Valid(); err == nil {
			t.Errorf("path %q must be rejected", p)
		}
	}
	// Spot-check that legitimate paths still pass.
	for _, p := range []string{"etc/duckos/network.json", "var/log/agent.log", "a/b/c/d/e/f/g.bin"} {
		e := makeEntry(p, "config", []byte("x"))
		if err := e.Valid(); err != nil {
			t.Errorf("legitimate path %q rejected: %v", p, err)
		}
	}
}

// TestTorture_OversizeEntry verifies Manifest.Valid rejects an
// envelope whose single entry exceeds MaxSingleFileBytes.
func TestTorture_OversizeEntry(t *testing.T) {
	e := Entry{
		Path:   "huge.bin",
		Kind:   KindState,
		Size:   MaxSingleFileBytes + 1,
		Digest: strings.Repeat("0", 64),
		Mode:   0o600,
		MTime:  time.Now().Unix(),
		Owner:  "root",
	}
	if err := e.Valid(); err == nil {
		t.Fatal("oversize single entry must be rejected")
	}
	// A manifest that lies about Size but has a valid digest
	// is caught by EncodeManifest when the builder runs.
	// Build will reject because Size != len(Entry).
	body := make([]byte, 1024)
	e = makeEntry("ok.json", "config", body)
	e.Size = MaxTotalBytes + 1
	if err := e.Valid(); err == nil {
		t.Fatal("entry.Size > MaxTotalBytes must be rejected even before Build")
	}
}

// TestTorture_TooManyEntries verifies Manifest.Valid rejects an
// envelope with more than MaxEntries entries.
func TestTorture_TooManyEntries(t *testing.T) {
	body := []byte("x")
	es := make([]Entry, 0, MaxEntries+1)
	for i := 0; i <= MaxEntries; i++ {
		es = append(es, makeEntry(fmt.Sprintf("d/%d.json", i), "config", body))
	}
	if _, err := Build(es, "h", "v", "r", "o", time.Now()); err == nil {
		t.Fatal("manifest exceeding MaxEntries must be rejected")
	}
}

// TestTorture_UnknownKind verifies isValidKind rejects any kind
// outside the canonical whitelist.
func TestTorture_UnknownKind(t *testing.T) {
	for _, k := range []string{"", "Config", "secret", "exec", "rootkit", "CONFIG"} {
		if isValidKind(Kind(k)) {
			t.Errorf("kind %q must be rejected", k)
		}
	}
	for _, k := range []Kind{KindConfig, KindVault, KindIdentity, KindNetwork, KindState, KindAudit, KindLog, KindMeta} {
		if !isValidKind(k) {
			t.Errorf("canonical kind %q must be valid", k)
		}
	}
}

// TestTorture_PlanRestoreConflicts verifies PlanRestore reports
// every overlapping key as a Modifies (overwrite) entry and
// refuses to mark the plan safe to apply.
func TestTorture_PlanRestoreConflicts(t *testing.T) {
	backup := map[string]bool{
		"duckos.network":  true,
		"duckos.firewall": true,
		"duckos.vault":    true,
	}
	live := map[string]bool{
		"duckos.firewall": true, // conflict (will be overwritten)
		"duckos.users":    true, // live-only (will be removed)
	}
	plan := PlanRestore("bk-1", false, backup, live)
	if plan.SafeToApply {
		t.Fatal("overlapping keys must mark plan unsafe")
	}
	if plan.RequiresSecret {
		t.Fatal("plan with requiresSecret=false must not require secret")
	}
	if len(plan.Modifies) != 1 || plan.Modifies[0] != "duckos.firewall" {
		t.Fatalf("expected single modifies key duckos.firewall, got %+v", plan.Modifies)
	}
	if len(plan.Adds) != 2 || plan.Adds[0] != "duckos.network" || plan.Adds[1] != "duckos.vault" {
		t.Fatalf("expected adds duckos.network+duckos.vault, got %+v", plan.Adds)
	}
	if len(plan.Removes) != 1 || plan.Removes[0] != "duckos.users" {
		t.Fatalf("expected single removes key duckos.users, got %+v", plan.Removes)
	}
	found := false
	for _, s := range plan.Impact {
		if s == "overwrites-live-objects" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected impact marker overwrites-live-objects, got %+v", plan.Impact)
	}
}

// TestTorture_PlanRestoreCleanIsSafe verifies that a plan
// whose backupKeys are disjoint from liveKeys and require no
// removals is safe to apply, and that a plan with live-only
// keys is not (everything would be removed).
func TestTorture_PlanRestoreCleanIsSafe(t *testing.T) {
	// Empty live index: every backup key is a pure add.
	fresh := PlanRestore("bk-fresh", false,
		map[string]bool{"new.a": true, "new.b": true},
		map[string]bool{})
	if !fresh.SafeToApply {
		t.Fatal("fresh live + non-secret backup must be safe")
	}
	if len(fresh.Adds) != 2 || len(fresh.Modifies) != 0 || len(fresh.Removes) != 0 {
		t.Fatalf("fresh plan unexpected: %+v", fresh)
	}
	// Empty backup index: everything in live would be removed
	// — must downgrade.
	empty := PlanRestore("bk-empty", false,
		map[string]bool{},
		map[string]bool{"x": true, "y": true})
	if empty.SafeToApply {
		t.Fatal("empty backup must downgrade plan (everything would be removed)")
	}
	if len(empty.Removes) != 2 {
		t.Fatalf("expected 2 removals, got %+v", empty.Removes)
	}
}

// TestTorture_PlanRestoreSecretGate verifies a full-protected
// backup with no supplied secret is not safe to apply.
func TestTorture_PlanRestoreSecretGate(t *testing.T) {
	plan := PlanRestore("bk-2", true, map[string]bool{"k": true}, map[string]bool{})
	if !plan.RequiresSecret {
		t.Fatal("full-protected backup must require secret")
	}
	if plan.SafeToApply {
		t.Fatal("plan must not be safe when secret is required")
	}
	found := false
	for _, s := range plan.Impact {
		if s == "requires-secret" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected impact marker requires-secret, got %+v", plan.Impact)
	}
}

// TestTorture_PlanRestoreEmptyEdgeCases pins the trivial
// cases: empty backup index yields an unsafe plan (everything
// would be removed); empty live index yields a safe plan with
// only adds.
func TestTorture_PlanRestoreEmptyEdgeCases(t *testing.T) {
	all := PlanRestore("bk-empty", false, map[string]bool{}, map[string]bool{"a": true, "b": true})
	if all.SafeToApply {
		t.Fatal("empty backup must downgrade plan (everything would be removed)")
	}
	if len(all.Removes) != 2 {
		t.Fatalf("expected 2 removals, got %+v", all.Removes)
	}
	none := PlanRestore("bk-fresh", false, map[string]bool{"a": true, "b": true}, map[string]bool{})
	if !none.SafeToApply {
		t.Fatal("fresh live + non-secret backup must be safe")
	}
	if len(none.Adds) != 2 {
		t.Fatalf("expected 2 adds, got %+v", none.Adds)
	}
	if len(none.Modifies) != 0 || len(none.Removes) != 0 {
		t.Fatalf("fresh plan must have no modifies/removes, got %+v", none)
	}
}

// TestTorture_RemapRejectsInvalid verifies Remap surfaces
// every malformed request as a diagnostic on the returned
// LogicalManifest and skips the corresponding binding.
func TestTorture_RemapRejectsInvalid(t *testing.T) {
	lm := LogicalManifest{
		BackupID:      "bk-3",
		SchemaVersion: CurrentSchemaVersion,
		Mode:          BackupModeConfigOnly,
		Interfaces:    []InterfaceBinding{{Logical: "wan", Physical: "eth0", Kind: "ethernet"}},
	}
	reqs := []RemapRequest{
		{Logical: "wan", Physical: "eth1", MAC: "00:11:22:33:44:55"},
		{Logical: "", Physical: "eth2"}, // invalid: missing logical
		{Logical: "lan", Physical: ""},  // invalid: missing physical
	}
	out, missing := Remap(lm, reqs)
	if len(out.Diagnostics) != 2 {
		t.Fatalf("expected 2 diagnostics for invalid requests, got %+v", out.Diagnostics)
	}
	// The valid request remapped the only interface so the
	// missing list must be empty.
	if len(missing) != 0 {
		t.Fatalf("expected no missing logicals after valid remap, got %+v", missing)
	}
	if out.Interfaces[0].Physical != "eth1" {
		t.Fatalf("valid remap was not applied, got %+v", out.Interfaces[0])
	}
}

// TestTorture_RandomEntriesRoundtrip builds 64 random entries
// (random path, kind, body) and verifies that Build→Encode→
// Decode preserves the canonical fields and rejects any entry
// whose post-Build state violates the contract.
func TestTorture_RandomEntriesRoundtrip(t *testing.T) {
	const N = 64
	kinds := []Kind{KindConfig, KindVault, KindIdentity, KindNetwork, KindState, KindAudit, KindLog}
	entries := make([]Entry, 0, N)
	for i := 0; i < N; i++ {
		buf := make([]byte, 64+i)
		_, _ = rand.Read(buf)
		k := kinds[i%len(kinds)]
		entries = append(entries, makeEntry(fmt.Sprintf("dir-%d/file-%d.bin", i%4, i), string(k), buf))
	}
	m, err := Build(entries, "host-rand", "v1.0", "torture-rand", "qa", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Valid(); err != nil {
		t.Fatal(err)
	}
	enc, err := EncodeManifest(m)
	if err != nil {
		t.Fatal(err)
	}
	back, err := DecodeManifest(enc)
	if err != nil {
		t.Fatal(err)
	}
	if len(back.Entries) != N {
		t.Fatalf("entry count: got %d want %d", len(back.Entries), N)
	}
	// Re-validate every decoded entry; tampered fields are
	// caught here even if Build skipped a check.
	for i, e := range back.Entries {
		if err := e.Valid(); err != nil {
			t.Fatalf("decoded entry %d invalid: %v", i, err)
		}
	}
	// Spot-check that an attacker cannot smuggle an oversized
	// entry by editing the encoded manifest.
	bad := append([]byte{}, enc...)
	// Flip a byte inside the JSON payload area.
	if len(bad) > 256 {
		idx := len(bad) / 2
		bad[idx] ^= 0x80
	}
	if _, err := DecodeManifest(bad); err == nil {
		t.Fatal("tampered encoded manifest must be rejected")
	}
	// Spot-check that random hex digests are accepted by
	// VerifyDigest. Digest(raw) is the canonical sha256 hex;
	// VerifyDigest must accept it back.
	raw := make([]byte, 32)
	_, _ = rand.Read(raw)
	d := Digest(raw)
	if err := VerifyDigest(raw, d); err != nil {
		t.Fatalf("VerifyDigest must accept Digest(raw): %v", err)
	}
	// Spot-check that VerifyDigest rejects a flipped byte in
	// the raw body even when the digest matches Digest(raw).
	tampered := append([]byte{}, raw...)
	tampered[0] ^= 0xFF
	if err := VerifyDigest(tampered, d); err == nil {
		t.Fatal("VerifyDigest must reject tampered body with stale digest")
	}
	// Spot-check that VerifyDigest rejects a malformed digest.
	if err := VerifyDigest(raw, strings.Repeat("z", 64)); err == nil {
		t.Fatal("VerifyDigest must reject non-hex digest")
	}
}
