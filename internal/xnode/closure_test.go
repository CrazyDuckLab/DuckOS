// Package xnode / closure_test.go: F-01 semantic identity validation
// regressions for the final security closure (2026-10-24 round).
package xnode

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func closureControl(s *NodeServer, credential, op, nonce string) int {
	raw, _ := json.Marshal(controlRequest{Op: op, Nonce: nonce, Time: time.Now()})
	r := httptest.NewRequest("POST", TransportPath, bytes.NewReader(raw))
	r.Header.Set("Authorization", "Bearer "+credential)
	r.RemoteAddr = "192.0.2.1:10000"
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	return w.Code
}

// F-01: a syntactically valid JSON identity document that could never
// have been produced by the identity writer must fail the
// authentication boundary closed — pending state without an adopted
// current identity, malformed digest formats and pending
// plaintext/digest mismatches authenticate nothing, and no stale
// cached credential is used instead. Recovery is automatic once a
// valid authoritative document is restored.
func TestClosureSemanticallyInvalidIdentityFailsClosed(t *testing.T) {
	cases := map[string]NodeIdentityDocument{
		"orphan-pending":             {PendingHash: nodeCredentialHash("closure-old")},
		"pending-integrity-mismatch": {CurrentHash: nodeCredentialHash("closure-new"), PendingHash: nodeCredentialHash("closure-old"), PendingCredential: "closure-does-not-match"},
		"invalid-current-hash":       {CurrentHash: "invalid-digest", PendingHash: nodeCredentialHash("closure-old")},
		"invalid-pending-hash":       {CurrentHash: nodeCredentialHash("closure-new"), PendingHash: "DEADBEEF"},
		"plaintext-without-pending":  {CurrentHash: nodeCredentialHash("closure-new"), PendingCredential: "closure-orphan-plaintext"},
	}
	for name, doc := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			local, err := OpenNodeIdentity(dir)
			if err != nil {
				t.Fatal(err)
			}
			if err := local.Adopt("closure-new"); err != nil {
				t.Fatal(err)
			}
			s := NewNodeServer(nil, func([]byte) error { return nil },
				func() NodeInfo { return NodeInfo{Name: "closure"} })
			s.Identity = local
			path := filepath.Join(dir, "node-identity.json")
			raw, err := json.Marshal(doc)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, raw, 0o600); err != nil {
				t.Fatal(err)
			}
			if code := closureControl(s, "closure-old", "health", "old"); code != 401 {
				t.Errorf("invalid identity accepted the pending credential: %d", code)
			}
			if code := closureControl(s, "closure-new", "health", "new"); code != 401 {
				t.Errorf("invalid identity admitted authentication: %d", code)
			}
			// Recovery: restoring a VALID authoritative document
			// re-establishes authentication automatically.
			valid, err := json.Marshal(NodeIdentityDocument{CurrentHash: nodeCredentialHash("closure-new")})
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, valid, 0o600); err != nil {
				t.Fatal(err)
			}
			if code := closureControl(s, "closure-new", "health", "recovered"); code != 200 {
				t.Fatalf("no automatic recovery after a valid document was restored: %d", code)
			}
		})
	}
}

// F-01: legitimate historical formats stay valid — in particular the
// hash-only pending document (no stored plaintext) written by earlier
// protocol versions must keep authenticating both slots.
func TestClosureLegacyHashOnlyPendingRemainsValid(t *testing.T) {
	dir := t.TempDir()
	local, err := OpenNodeIdentity(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := local.Adopt("closure-current"); err != nil {
		t.Fatal(err)
	}
	s := NewNodeServer(nil, func([]byte) error { return nil },
		func() NodeInfo { return NodeInfo{Name: "legacy"} })
	s.Identity = local
	path := filepath.Join(dir, "node-identity.json")
	// Hash-only pending: exactly what the writer produces minus the
	// plaintext field (the supported recovery shape).
	legacy := NodeIdentityDocument{
		CurrentHash: nodeCredentialHash("closure-current"),
		PendingHash: nodeCredentialHash("closure-pending"),
	}
	raw, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if code := closureControl(s, "closure-current", "health", "c"); code != 200 {
		t.Fatalf("current slot rejected on a valid hash-only pending document: %d", code)
	}
	if code := closureControl(s, "closure-pending", "health", "p"); code != 200 {
		t.Fatalf("pending slot rejected on a valid hash-only pending document: %d", code)
	}
}
