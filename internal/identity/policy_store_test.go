// identity/policy_store_test.go pins the durable
// per-object policy overlay (B05). The store MUST
// persist Set/Delete through to disk so a restart picks
// up the overlay, MUST fsync the parent directory after
// rename, and MUST skip malformed lines on reload.
package identity

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// policyGrant returns a canonical Grant the tests can
// mutate. The grant is internally consistent so the
// in-memory Policy's Set accepts it.
func policyGrant(role Role, kind, id string, perm Permission, decision Decision, reason string) Grant {
	return Grant{
		Role:       role,
		Object:     ObjectKey{Kind: kind, ID: id},
		Permission: perm,
		Decision:   decision,
		Reason:     reason,
	}
}

// marshalGrant is a tiny helper that returns the JSON
// encoding of a Grant so the test stays self-contained.
func marshalGrant(g Grant) ([]byte, error) { return json.Marshal(g) }

// roleOwner / roleHelpdesk are aliases so the tests do
// not have to import internal identity aliases.
const (
	roleOwner    Role = "owner"
	roleHelpdesk Role = "helpdesk"
)

// TestPolicyStorePersistsAcrossRestart proves the
// overlay survives an Agent restart.
func TestPolicyStorePersistsAcrossRestart(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenPolicyStore(dir, NewPolicy())
	if err != nil {
		t.Fatal(err)
	}
	g := policyGrant(roleHelpdesk, "network.firewall.rule", "rule-100", ConfigureNetwork, DecisionAllow, "test override")
	if err := s.Set(g); err != nil {
		t.Fatal(err)
	}
	// Reopen the store and verify the grant survives.
	s2, err := OpenPolicyStore(dir, NewPolicy())
	if err != nil {
		t.Fatal(err)
	}
	all := s2.Policy().All()
	if len(all) != 1 {
		t.Fatalf("grants after restart: %d, want 1", len(all))
	}
	if all[0].Object.ID != "rule-100" {
		t.Fatalf("Object.ID: %q, want rule-100", all[0].Object.ID)
	}
}

// TestPolicyStoreFileModeIs600 pins the durable
// authorisation-trail permission contract.
func TestPolicyStoreFileModeIs600(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenPolicyStore(dir, NewPolicy())
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Set(policyGrant(roleOwner, "qos.class", "voice", ConfigureNetwork, DecisionAllow, "test override")); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(dir, "policy-overlay.log"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode: %v, want 0600", info.Mode().Perm())
	}
}

// TestPolicyStoreDeletePersists proves Delete also
// writes through to disk.
func TestPolicyStoreDeletePersists(t *testing.T) {
	dir := t.TempDir()
	s, _ := OpenPolicyStore(dir, NewPolicy())
	g := policyGrant(roleHelpdesk, "network.firewall.rule", "rule-100", ConfigureNetwork, DecisionAllow, "test override")
	if err := s.Set(g); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(g.Role, g.Object, g.Permission); err != nil {
		t.Fatal(err)
	}
	s2, _ := OpenPolicyStore(dir, NewPolicy())
	if len(s2.Policy().All()) != 0 {
		t.Fatalf("grant did not survive delete")
	}
}

// TestPolicyStoreMalformedLinesFailClosed proves the
// reload path FAILS CLOSED when the overlay contains a
// malformed line. The overlay is an authorisation
// anchor: silently dropping a deny rule on corruption
// is a permissive-fallback vulnerability (audit DV05,
// high severity). The store must refuse to load so the
// production composition root blocks authorization
// until the operator restores a known-good file.
func TestPolicyStoreMalformedLinesFailClosed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "policy-overlay.log")
	good := policyGrant(roleHelpdesk, "network.firewall.rule", "rule-100", ConfigureNetwork, DecisionAllow, "test override")
	line, _ := marshalGrant(good)
	body := append([]byte("{not-json\n"), line...)
	body = append(body, '\n')
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := OpenPolicyStore(dir, NewPolicy())
	if err == nil {
		// Defensive: if the store somehow succeeded,
		// force a Close so the test does not leak.
		_ = s
		t.Fatalf("reload with corrupt overlay must FAIL CLOSED; got nil error")
	}
	// Restore the overlay to the single good line; a
	// fresh OpenPolicyStore must succeed and surface
	// the good grant.
	if err := os.WriteFile(path, append(line, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	s2, err := OpenPolicyStore(dir, NewPolicy())
	if err != nil {
		t.Fatalf("reload of restored overlay: %v", err)
	}
	if got := s2.Policy().All(); len(got) != 1 {
		t.Fatalf("grants: %d, want 1", len(got))
	}
}

// TestPolicyStoreContentSHA256PinsOverlay proves the
// integrity anchor is deterministic across reloads so the
// audit trail has a stable reference.
func TestPolicyStoreContentSHA256PinsOverlay(t *testing.T) {
	dir := t.TempDir()
	s, _ := OpenPolicyStore(dir, NewPolicy())
	if err := s.Set(policyGrant(roleOwner, "qos.class", "voice", ConfigureNetwork, DecisionAllow, "test override")); err != nil {
		t.Fatal(err)
	}
	first, err := s.ContentSHA256()
	if err != nil {
		t.Fatal(err)
	}
	if first == "" {
		t.Fatal("empty digest")
	}
	// Reopen and confirm the digest is stable.
	s2, _ := OpenPolicyStore(dir, NewPolicy())
	second, err := s2.ContentSHA256()
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("digest changed across reload: %q != %q", first, second)
	}
}
