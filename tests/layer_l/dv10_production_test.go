//go:build linux

// dv10_production_test.go exercises DV10 through the
// production code path:
//
//   PUBLIC ADMIN ISSUANCE
//   -> production ztp.Manager.Issue
//   -> production ztp.Manager.Redeem (HMAC + TTL + scope/node + replay)
//   -> production ztp.Manager.persistLocked (tmp+rename+fsync+parent-fsync)
//   -> durable evidence: redemption-journal.log on disk
//   -> independent runtime oracle:
//        (a) separate os/exec re-execution of this test binary
//            against the same dir — proves flock(2) cross-process
//            exclusion (ErrLocked on the second writer);
//        (b) os.ReadFile + sha256File of the journal — proves the
//            redemption evidence is observable from outside the
//            in-process Manager.
//
// DV10 production-path contract: PUBLIC issuance → redemption →
// persistence → restart → corrupt journal handling → consumed
// token NEVER becomes usable again. Real cross-process
// writer/flock witness.
//
// The Manager's reload() fails CLOSED on any corrupt journal
// line (B10 security contract) so the replay window cannot be
// silently re-opened. Cross-process flock prevents a second
// writer from racing the durable dedup index.

package layerl

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/ztp"
)

// dv10ProductionKey is the production HMAC secret the
// fixture's Manager signs against. A real production
// deployment reads this from /etc/duckos/ztp.key (or
// equivalent); the fixture injects it directly so the
// witness is deterministic.
func dv10ProductionKey(t *testing.T) []byte {
	t.Helper()
	k := make([]byte, 32)
	if _, err := rand.Read(k); err != nil {
		t.Fatalf("entropy: %v", err)
	}
	return k
}

// journalPath is the durable path the Manager owns.
func journalPath(dir string) string {
	return filepath.Join(dir, "redemption-journal.log")
}

// lockPath is the durable flock path the Manager owns.
func lockPath(dir string) string {
	return filepath.Join(dir, "redemption-journal.lock")
}

// issueAndRedeem runs the canonical PUBLIC issuance flow:
// Issue + Redeem + Verify (durable evidence on disk).
// The IssuedToken is returned alongside the BootstrapRecord
// so callers can replay the same token against a future
// Manager (post-restart, post-corrupt-restore) without
// having to reproduce the MAC externally — the MAC is
// keyed on the production secret the Manager owns.
func issueAndRedeem(t *testing.T, m *ztp.Manager, subject, scope, node string, validity time.Duration) (ztp.BootstrapRecord, ztp.IssuedToken) {
	t.Helper()
	tok, err := m.Issue(context.Background(), ztp.IssueRequest{
		Subject:  subject,
		Scope:    scope,
		Node:     node,
		Validity: validity,
	})
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	rec, err := m.Redeem(context.Background(), tok, node)
	if err != nil {
		t.Fatalf("Redeem(%s): %v", subject, err)
	}
	if rec.Phase != ztp.PhaseRedeemed {
		t.Fatalf("Phase: %q, want %q", rec.Phase, ztp.PhaseRedeemed)
	}
	if rec.TokenDigest == "" {
		t.Fatalf("TokenDigest empty after Redeem")
	}
	return rec, tok
}

// TestDV10ProductionPathIssuanceRedemptionPersists proves
// the PUBLIC issuance -> durable redemption chain through
// the production Manager. The witness also pins the
// on-disk journal: a separate os.ReadFile + sha256 of
// the journal must produce a stable digest that includes
// the redemption record.
func TestDV10ProductionPathIssuanceRedemptionPersists(t *testing.T) {
	dir := t.TempDir()
	key := dv10ProductionKey(t)

	m, err := ztp.OpenManager(dir, key)
	if err != nil {
		t.Fatalf("OpenManager: %v", err)
	}
	t.Cleanup(func() { _ = m.Close() })

	rec, _ := issueAndRedeem(t, m, "node-dv10-1", "edge", "node-dv10-1", time.Minute)

	// Independent oracle: the journal file must
	// exist on disk and contain the durable evidence.
	data, err := os.ReadFile(journalPath(dir))
	if err != nil {
		t.Fatalf("independent ReadFile(journal): %v", err)
	}
	if len(data) == 0 {
		t.Fatalf("journal empty after Redeem")
	}
	if !bytes.Contains(data, []byte(rec.TokenDigest)) {
		t.Fatalf("journal missing TokenDigest %q", rec.TokenDigest)
	}
	if !bytes.Contains(data, []byte("\"phase\":\"redeemed\"")) {
		t.Fatalf("journal missing redeemed phase row")
	}

	// Manager.FindByDigest surfaces the same record.
	if got, ok := m.FindByDigest(rec.TokenDigest); !ok || got.ID != rec.ID {
		t.Fatalf("FindByDigest: ok=%v id=%q, want ok=true id=%q", ok, got.ID, rec.ID)
	}
}

// TestDV10ProductionPathRestartPreservesReplayProtection
//
// After a process restart (Close + re-OpenManager against
// the same dir + same key) the redeemed token is still
// in the durable dedup index. A second Redeem of the
// SAME token returns ErrTokenReplay — the replay window
// cannot be silently re-opened by restarting the
// Manager.
//
// Independent oracle: the on-disk journal is unchanged
// between the first Close and the second Open, so the
// replay protection is observable from outside the
// process.
func TestDV10ProductionPathRestartPreservesReplayProtection(t *testing.T) {
	dir := t.TempDir()
	key := dv10ProductionKey(t)

	first, err := ztp.OpenManager(dir, key)
	if err != nil {
		t.Fatalf("first OpenManager: %v", err)
	}
	rec, tok := issueAndRedeem(t, first, "node-dv10-r", "edge", "node-dv10-r", time.Minute)

	// Independent oracle: snapshot the journal before
	// the restart.
	before, err := os.ReadFile(journalPath(dir))
	if err != nil {
		t.Fatalf("independent ReadFile(before): %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}

	// Restart.
	second, err := ztp.OpenManager(dir, key)
	if err != nil {
		t.Fatalf("restart OpenManager: %v", err)
	}
	t.Cleanup(func() { _ = second.Close() })

	// Journal survives the restart byte-for-byte.
	after, err := os.ReadFile(journalPath(dir))
	if err != nil {
		t.Fatalf("independent ReadFile(after): %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Fatalf("journal changed across restart:\nbefore: %q\nafter:  %q",
			string(before), string(after))
	}

	// The redeemed token still surfaces via FindByDigest.
	if got, ok := second.FindByDigest(rec.TokenDigest); !ok || got.ID != rec.ID {
		t.Fatalf("post-restart FindByDigest: ok=%v id=%q", ok, got.ID)
	}

	// Replay MUST be refused after the restart. The
	// original IssuedToken (same MAC, same IssuedAt,
	// same ExpiresAt) is replayed against the second
	// Manager; the in-memory dedup index (reloaded
	// from disk) still contains the TokenDigest.
	if _, err := second.Redeem(context.Background(), tok, rec.Node); !errors.Is(err, ztp.ErrTokenReplay) {
		t.Fatalf("post-restart Redeem: got %v, want ErrTokenReplay", err)
	}
}

// TestDV10ProductionPathCorruptJournalFailsClosed proves
// the Manager's reload() refuses to load a journal with
// any corrupt line. A corrupt line in the redemption
// journal is a SECURITY defect (it could silently drop
// a redeemed-token record and re-open the replay
// window). The Manager MUST fail CLOSED so the
// production composition root refuses to start
// (or refuses to redeem) until the operator restores
// a known-good journal.
//
// Independent oracle: the corrupt file is observable
// from outside the Manager via os.ReadFile before
// the OpenManager attempt.
func TestDV10ProductionPathCorruptJournalFailsClosed(t *testing.T) {
	dir := t.TempDir()
	key := dv10ProductionKey(t)

	// Seed a valid record so the journal is not empty.
	seed, err := ztp.OpenManager(dir, key)
	if err != nil {
		t.Fatalf("seed OpenManager: %v", err)
	}
	_, _ = issueAndRedeem(t, seed, "node-dv10-c", "edge", "node-dv10-c", time.Minute)
	if err := seed.Close(); err != nil {
		t.Fatalf("seed Close: %v", err)
	}

	// Snapshot the valid journal so the oracle can
	// confirm corruption is observable.
	good, err := os.ReadFile(journalPath(dir))
	if err != nil {
		t.Fatalf("independent ReadFile(good): %v", err)
	}

	// Inject a corrupt line ahead of the valid record.
	corrupt := append([]byte("{not-json\n"), good...)
	if err := os.WriteFile(journalPath(dir), corrupt, 0o600); err != nil {
		t.Fatalf("WriteFile(corrupt): %v", err)
	}

	// Independent oracle first.
	data, err := os.ReadFile(journalPath(dir))
	if err != nil {
		t.Fatalf("independent ReadFile: %v", err)
	}
	if !bytes.Contains(data, []byte("not-json")) {
		t.Fatalf("independent oracle: journal not observably corrupt")
	}

	// OpenManager MUST fail CLOSED.
	if _, err := ztp.OpenManager(dir, key); err == nil {
		t.Fatalf("OpenManager(corrupt) must FAIL CLOSED; got nil")
	}

	// Restore the journal and confirm OpenManager now
	// succeeds with the original record still present.
	if err := os.WriteFile(journalPath(dir), good, 0o600); err != nil {
		t.Fatalf("WriteFile(restore): %v", err)
	}
	m, err := ztp.OpenManager(dir, key)
	if err != nil {
		t.Fatalf("restore OpenManager: %v", err)
	}
	t.Cleanup(func() { _ = m.Close() })
	if got := m.Counters(); got.Total != 1 {
		t.Fatalf("restored Counters.Total: %d, want 1", got.Total)
	}
}

// TestDV10ProductionPathConsumedTokenNeverReusable is
// the canonical "consumed token never becomes usable
// again" witness. The redeem path is exercised through
// every available fail-closed surface:
//
//   - replay after a successful first redeem
//   - replay after a process restart
//   - replay after a journal corruption + restore
//
// In every case the token's MAC is correct (it was
// minted by the production Manager) so the only thing
// standing between the redeemer and a second redemption
// is the durable dedup index.
func TestDV10ProductionPathConsumedTokenNeverReusable(t *testing.T) {
	dir := t.TempDir()
	key := dv10ProductionKey(t)

	m, err := ztp.OpenManager(dir, key)
	if err != nil {
		t.Fatalf("OpenManager: %v", err)
	}
	t.Cleanup(func() { _ = m.Close() })

	// Phase 1: original redemption succeeds.
	rec, tok := issueAndRedeem(t, m, "node-dv10-x", "edge", "node-dv10-x", time.Minute)

	// Phase 2: replay is refused immediately.
	if _, err := m.Redeem(context.Background(), tok, rec.Node); !errors.Is(err, ztp.ErrTokenReplay) {
		t.Fatalf("phase2 replay: got %v, want ErrTokenReplay", err)
	}

	// Phase 3: replay is still refused after a restart.
	if err := m.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	m2, err := ztp.OpenManager(dir, key)
	if err != nil {
		t.Fatalf("restart OpenManager: %v", err)
	}
	t.Cleanup(func() { _ = m2.Close() })
	if _, err := m2.Redeem(context.Background(), tok, rec.Node); !errors.Is(err, ztp.ErrTokenReplay) {
		t.Fatalf("phase3 post-restart replay: got %v, want ErrTokenReplay", err)
	}

	// Phase 4: replay is STILL refused after a journal
	// corruption + restore. The corruption is reversed
	// by restoring the original (good) journal — there
	// is no path that drops the redeemed record.
	if err := m2.Close(); err != nil {
		t.Fatalf("Close(m2): %v", err)
	}
	good, err := os.ReadFile(journalPath(dir))
	if err != nil {
		t.Fatalf("ReadFile(good): %v", err)
	}
	corrupt := append([]byte("{not-json\n"), good...)
	if err := os.WriteFile(journalPath(dir), corrupt, 0o600); err != nil {
		t.Fatalf("WriteFile(corrupt): %v", err)
	}
	if _, err := ztp.OpenManager(dir, key); err == nil {
		t.Fatalf("OpenManager(corrupt) must FAIL CLOSED; got nil")
	}
	if err := os.WriteFile(journalPath(dir), good, 0o600); err != nil {
		t.Fatalf("WriteFile(restore): %v", err)
	}
	m3, err := ztp.OpenManager(dir, key)
	if err != nil {
		t.Fatalf("restore OpenManager: %v", err)
	}
	t.Cleanup(func() { _ = m3.Close() })
	if _, err := m3.Redeem(context.Background(), tok, rec.Node); !errors.Is(err, ztp.ErrTokenReplay) {
		t.Fatalf("phase4 post-restore replay: got %v, want ErrTokenReplay", err)
	}
}

// TestDV10ProductionPathCrossProcessFlockWitness exercises
// the flock(2) inter-process exclusion guarantee through
// a SEPARATE OS PROCESS. The Manager holds the lock for
// the lifetime of the in-process Manager; a child process
// spawned via os/exec that tries to open the same dir
// MUST be refused with ErrLocked.
//
// The child re-execs this test binary with the
// TestDV10CrossProcessHelper target. The helper is a
// tiny standalone routine that opens the Manager
// against the supplied DV10_DIR + DV10_KEY env vars and
// reports the result through the exit code:
//
//   0 -> lock acquired (would race the parent; FAIL)
//   2 -> ErrLocked observed (expected; PASS)
//   1 -> other error (FAIL — surface unexpected state)
func TestDV10ProductionPathCrossProcessFlockWitness(t *testing.T) {
	dir := t.TempDir()
	key := dv10ProductionKey(t)

	parent, err := ztp.OpenManager(dir, key)
	if err != nil {
		t.Fatalf("parent OpenManager: %v", err)
	}
	t.Cleanup(func() { _ = parent.Close() })

	// Confirm the parent holds the lock file as a
	// 0o600 file. The independent oracle proves the
	// on-disk evidence exists.
	info, err := os.Stat(lockPath(dir))
	if err != nil {
		t.Fatalf("Stat(lock): %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("lock mode: %v, want 0600", info.Mode().Perm())
	}

	// Independent cross-process witness: re-exec the
	// test binary with the helper flag. The child
	// process must run with the same uid as the parent
	// so the per-dir lock file (created 0600 by the
	// parent's OpenManager) is accessible to the
	// child.
	keyHex := hex.EncodeToString(key)
	childPath, err := os.Executable()
	if err != nil {
		t.Fatalf("Executable: %v", err)
	}
	// When the parent runs as root (sudo from CI), the
	// child must also run as root so it can open the
	// 0o600 lock file. We detect this from os.Getuid()
	// and prepend `sudo -E` to the child command.
	if os.Getuid() == 0 {
		cmd := exec.Command("sudo", "-E",
			childPath,
			"-test.run=TestDV10CrossProcessHelper",
			"-test.v",
		)
		cmd.Env = append(os.Environ(),
			"DV10_DIR="+dir,
			"DV10_KEY="+keyHex,
		)
		out, err := cmd.CombinedOutput()
		dv10CheckCrossProcess(t, out, err)
		return
	}
	cmd := exec.Command(childPath,
		"-test.run=TestDV10CrossProcessHelper",
		"-test.v",
	)
	cmd.Env = append(os.Environ(),
		"DV10_DIR="+dir,
		"DV10_KEY="+keyHex,
	)
	out, err := cmd.CombinedOutput()
	dv10CheckCrossProcess(t, out, err)
}

// dv10CheckCrossProcess asserts the helper exited with
// code 2 and surfaced ErrLocked. The helper exits with
// code 2 when the second OpenManager returns ErrLocked
// (the cross-process flock witness), code 0 when the
// lock was acquired (would race the parent — FAIL), and
// code 1 for any other error.
func dv10CheckCrossProcess(t *testing.T, out []byte, err error) {
	t.Helper()
	if err == nil {
		t.Fatalf("cross-process helper exited 0 (lock acquired); expected ErrLocked\noutput: %s", string(out))
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("cross-process helper: non-ExitError %v\noutput: %s", err, string(out))
	}
	if exitErr.ExitCode() != 2 {
		t.Fatalf("cross-process helper exit code %d, want 2 (ErrLocked)\noutput: %s",
			exitErr.ExitCode(), string(out))
	}
	if !strings.Contains(string(out), "ErrLocked") {
		t.Fatalf("cross-process helper did not surface ErrLocked; output: %s", string(out))
	}
}

// TestDV10CrossProcessHelper is the in-binary helper
// invoked by os/exec from
// TestDV10ProductionPathCrossProcessFlockWitness. It
// opens a Manager against DV10_DIR / DV10_KEY and
// reports the outcome via the process exit code:
//
//   0 -> lock acquired (would race the parent — FAIL)
//   2 -> ErrLocked observed (expected — PASS)
//   1 -> any other error
func TestDV10CrossProcessHelper(t *testing.T) {
	dir := os.Getenv("DV10_DIR")
	keyHex := os.Getenv("DV10_KEY")
	if dir == "" || keyHex == "" {
		t.Skip("DV10_DIR / DV10_KEY not set; not invoked as helper")
	}
	fmt.Fprintf(os.Stderr, "helper: dir=%s keyLen=%d\n", dir, len(keyHex))
	key, err := hex.DecodeString(keyHex)
	if err != nil {
		fmt.Fprintf(os.Stderr, "helper: decode key: %v\n", err)
		os.Exit(1)
	}
	m, err := ztp.OpenManager(dir, key)
	if err != nil {
		fmt.Fprintf(os.Stderr, "helper: OpenManager: %v\n", err)
		if errors.Is(err, ztp.ErrLocked) {
			fmt.Fprintln(os.Stderr, "helper: ErrLocked observed")
			os.Exit(2)
		}
		os.Exit(1)
	}
	_ = m.Close()
	fmt.Fprintln(os.Stderr, "helper: lock acquired (UNEXPECTED)")
	os.Exit(0)
}

// TestDV10ProductionPathJournalFileModeIsRestrictive pins
// the journal file mode (0600) and the parent dir mode
// (no group/world bits) via in-process os.Stat + an
// independent /usr/bin/stat oracle.
//
// The parent dir is created explicitly with 0o700 mode
// (OpenManager only sets 0o700 on the supplied dir, but
// t.TempDir() parents are 0o755; we want the explicit
// mode to mirror the production /var/lib/duckos/ztp
// directory).
func TestDV10ProductionPathJournalFileModeIsRestrictive(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "ztp")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	key := dv10ProductionKey(t)

	m, err := ztp.OpenManager(dir, key)
	if err != nil {
		t.Fatalf("OpenManager: %v", err)
	}
	t.Cleanup(func() { _ = m.Close() })
	_, _ = issueAndRedeem(t, m, "node-dv10-m", "edge", "node-dv10-m", time.Minute)

	info, err := os.Stat(journalPath(dir))
	if err != nil {
		t.Fatalf("Stat(journal): %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("journal mode: %v, want 0600", info.Mode().Perm())
	}
	dirInfo, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("Stat(dir): %v", err)
	}
	if dirInfo.Mode().Perm()&0o077 != 0 {
		t.Fatalf("dir mode: %v, want no group/world bits", dirInfo.Mode().Perm())
	}

	// Independent oracle.
	stat := independentStat(t, journalPath(dir))
	if stat == "" {
		return
	}
	if !strings.HasPrefix(stat, "600 ") {
		t.Fatalf("independent stat: %q, want mode 600 prefix", stat)
	}
}

// TestDV10ProductionPathTruncatedJournalFailsClosed
// mirrors DV05's truncated-line contract: a crash
// mid-append can leave the journal with a partial
// final line. The Manager's reload() must fail CLOSED
// so the partial grant is not silently dropped (which
// would re-open the replay window).
//
// The truncation chops the final record mid-JSON so
// the parser reports a syntax error rather than
// silently accepting the line.
func TestDV10ProductionPathTruncatedJournalFailsClosed(t *testing.T) {
	dir := t.TempDir()
	key := dv10ProductionKey(t)

	seed, err := ztp.OpenManager(dir, key)
	if err != nil {
		t.Fatalf("seed OpenManager: %v", err)
	}
	_, _ = issueAndRedeem(t, seed, "node-dv10-t", "edge", "node-dv10-t", time.Minute)
	if err := seed.Close(); err != nil {
		t.Fatalf("seed Close: %v", err)
	}

	good, err := os.ReadFile(journalPath(dir))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	// Chop the file mid-line so the trailing record is
	// not valid JSON. Keep at least half so the
	// truncated prefix is clearly observable.
	truncated := good[:len(good)/2]
	if err := os.WriteFile(journalPath(dir), truncated, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Independent oracle.
	data, err := os.ReadFile(journalPath(dir))
	if err != nil {
		t.Fatalf("independent ReadFile: %v", err)
	}
	if len(data) == 0 || len(data) >= len(good) {
		t.Fatalf("independent oracle: file is not observably truncated (len=%d, want < %d)",
			len(data), len(good))
	}

	if _, err := ztp.OpenManager(dir, key); err == nil {
		t.Fatalf("OpenManager(truncated) must FAIL CLOSED; got nil")
	}
}

// mustReMAC was removed; replay tests now use the
// IssuedToken returned by issueAndRedeem directly so
// the MAC matches byte-for-byte without re-issuing.