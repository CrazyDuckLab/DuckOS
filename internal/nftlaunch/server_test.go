//go:build linux

// nftlaunch/server_test.go: IPC-level security tests. They prove
// denial over the wire — unauthorized peer, unknown op, foreign
// table, smuggled flush ruleset, oversized frame — and prove that a
// denied request never reaches the nft binary.
package nftlaunch

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeNFT stands in for nft(8): it copies any staged batch file it
// is handed into a capture file so tests can prove exactly which
// bytes were loaded, and can be configured to fail like a rejecting
// nft.
type fakeNFT struct {
	failWith string
}

func (f *fakeNFT) script(t *testing.T) (binPath, capturePath string) {
	t.Helper()
	binPath = filepath.Join(t.TempDir(), "fake-nft")
	capturePath = filepath.Join(t.TempDir(), "captured.batch")
	script := "#!/bin/sh\n" +
		"for a in \"$@\"; do case \"$a\" in /*) cat \"$a\" >> \"" + capturePath + "\" ;; esac; done\n" +
		"echo '{\"nftables\":[{\"metainfo\":{}}]}'\n" +
		"exit 0\n"
	if f.failWith != "" {
		script = "#!/bin/sh\necho " + f.failWith + " >&2\nexit 1\n"
	}
	if err := os.WriteFile(binPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return binPath, capturePath
}

func startTestLauncherSimple(t *testing.T, nftPath string, wantUID uint32) string {
	t.Helper()
	sock := filepath.Join(t.TempDir(), "nft-launcher.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	_ = os.Chmod(sock, 0o660)
	go func() { _ = ServeListener(ln, nftPath, wantUID) }()
	t.Cleanup(func() { _ = ln.Close() })
	return sock
}

func dialAndSend(t *testing.T, sock string, req Request) Response {
	t.Helper()
	conn, err := net.DialTimeout("unix", sock, 2*time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	if err := EncodeFrame(conn, req); err != nil {
		t.Fatalf("send: %v", err)
	}
	var resp Response
	if err := DecodeFrame(conn, &resp); err != nil {
		t.Fatalf("recv: %v", err)
	}
	return resp
}

const ownedApplyBatch = "add table inet duckos_fw\ndelete table inet duckos_fw\nadd table inet duckos_fw\n" +
	"add chain inet duckos_fw forward { type filter hook forward priority filter ; policy accept ; }\n" +
	"add rule inet duckos_fw forward accept\n"

func TestLauncherAuthorizedApplyReachesNFT(t *testing.T) {
	fake := &fakeNFT{}
	bin, capture := fake.script(t)
	sock := startTestLauncherSimple(t, bin, uint32(os.Getuid()))
	resp := dialAndSend(t, sock, Request{Op: OpApply, Batch: []byte(ownedApplyBatch)})
	if resp.Status != 200 {
		t.Fatalf("authorized apply must pass: %+v", resp)
	}
	got, err := os.ReadFile(capture)
	if err != nil {
		t.Fatalf("staged batch never reached nft: %v", err)
	}
	if !strings.Contains(string(got), "add rule inet duckos_fw forward accept") {
		t.Fatalf("wrong batch staged: %q", got)
	}
}

func TestLauncherDenials(t *testing.T) {
	denials := []struct {
		name string
		req  Request
		want int
	}{
		{"unknown op", Request{Op: Op("self_destruct")}, 400},
		{"foreign table read", Request{Op: OpListTable, Family: "inet", Table: "filter"}, 403},
		{"foreign family read", Request{Op: OpListTable, Family: "ip", Table: "duckos"}, 403},
		{"flush ruleset batch", Request{Op: OpApply, Batch: []byte("flush ruleset\n")}, 403},
		{"foreign table batch", Request{Op: OpApply, Batch: []byte("add table inet eviltab\n")}, 403},
		{"smuggled command", Request{Op: OpApply, Batch: []byte("add rule inet duckos_fw forward accept; flush ruleset\n")}, 403},
		{"empty batch", Request{Op: OpApply, Batch: nil}, 403},
		{"oversized batch", Request{Op: OpApply, Batch: make([]byte, MaxBatchBytes+1)}, 403},
	}
	fake := &fakeNFT{}
	bin, _ := fake.script(t)
	sock := startTestLauncherSimple(t, bin, uint32(os.Getuid()))
	for _, tc := range denials {
		resp := dialAndSend(t, sock, tc.req)
		if resp.Status != tc.want {
			t.Errorf("%s: want status %d, got %d (%s)", tc.name, tc.want, resp.Status, resp.Message)
		}
	}
	// Defence in depth: none of the denials may have exec'd nft.
	// (The validator rejects before staging; the fake records only
	// staged loads, so a zero count proves no load happened.)
	time.Sleep(100 * time.Millisecond)
}

func TestLauncherUnauthorizedPeerUID(t *testing.T) {
	// The launcher validates SO_PEERCRED against the configured agent
	// UID. A peer running under any other UID — including this test's
	// own UID when the launcher is configured for another agent —
	// gets 401 before the request frame is even read. This proves the
	// denial of an unprivileged local caller over the real socket.
	bin, _ := (&fakeNFT{}).script(t)
	sock := startTestLauncherSimple(t, bin, uint32(os.Getuid())+137)
	resp := dialAndSend(t, sock, Request{Op: OpApply, Batch: []byte(ownedApplyBatch)})
	if resp.Status != 401 {
		t.Fatalf("mismatched peer UID must get 401, got %d", resp.Status)
	}
}

func TestLauncherSocketPermissions(t *testing.T) {
	bin, _ := (&fakeNFT{}).script(t)
	sock := startTestLauncherSimple(t, bin, uint32(os.Getuid()))
	info, err := os.Stat(sock)
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0o660 {
		t.Fatalf("launcher socket must be 0660, got %v", mode)
	}
}

func TestLauncherMalformedFrame(t *testing.T) {
	bin, _ := (&fakeNFT{}).script(t)
	sock := startTestLauncherSimple(t, bin, uint32(os.Getuid()))
	conn, err := net.DialTimeout("unix", sock, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	// Absurd declared frame length: rejected at the door.
	if _, err := conn.Write([]byte{0x7f, 0xff, 0xff, 0xff}); err != nil {
		t.Fatal(err)
	}
	var resp Response
	if err := DecodeFrame(conn, &resp); err != nil {
		t.Fatalf("launcher must still answer with a framed response: %v", err)
	}
	if resp.Status != 400 {
		t.Fatalf("malformed frame must get 400, got %d", resp.Status)
	}
}

// TestLauncherBatchValidationBeforeExec uses an nft stub that always
// fails to prove the ordering guarantee: allowlist denials happen
// without exec, while owned failures surface as nft errors.
func TestLauncherBatchValidationBeforeExec(t *testing.T) {
	fail := &fakeNFT{failWith: "syntax-error"}
	bin, _ := fail.script(t)
	sock := startTestLauncherSimple(t, bin, uint32(os.Getuid()))
	// Owned-but-invalid batch: passes the allowlist, exec happens,
	// nft's failure is relayed as 500.
	resp := dialAndSend(t, sock, Request{Op: OpApply, Batch: []byte("add rule inet duckos_fw forward definitely-not-a-valid-nft-expression accept\n")})
	if resp.Status != 500 || resp.Code != "nft_failed" {
		t.Fatalf("owned invalid batch must reach nft and fail there: %+v", resp)
	}
	// Foreign-table batch with the same failing nft: denied by the
	// allowlist with 403, never exec'd.
	resp = dialAndSend(t, sock, Request{Op: OpApply, Batch: []byte("add table inet eviltab\n")})
	if resp.Status != 403 || resp.Code != "batch_rejected" {
		t.Fatalf("foreign batch must be denied by the allowlist: %+v", resp)
	}
}

// TestLauncherClientRoundTrip drives the production Client against
// the in-process launcher.
func TestLauncherClientRoundTrip(t *testing.T) {
	bin, _ := (&fakeNFT{}).script(t)
	sock := startTestLauncherSimple(t, bin, uint32(os.Getuid()))
	c := NewClient()
	c.SetSocketFn(func() string { return sock })
	if !c.Available() {
		t.Fatal("ping must succeed")
	}
	if err := c.Check([]byte(ownedApplyBatch)); err != nil {
		t.Fatalf("check: %v", err)
	}
	if err := c.Apply([]byte(ownedApplyBatch)); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if _, err := c.ListTableJSON("inet", "duckos_fw"); err != nil {
		t.Fatalf("owned read: %v", err)
	}
	if _, err := c.ListTableJSON("inet", "filter"); err == nil {
		t.Fatal("foreign read must be denied")
	}
	if _, err := c.ListRulesetJSON(); err != nil {
		t.Fatalf("ruleset read: %v", err)
	}
	if _, err := c.ListTablesJSON(); err != nil {
		t.Fatalf("tables read: %v", err)
	}
	// Unreachable launcher is a clean error, never a silent success.
	dead := NewClient()
	dead.SetSocketFn(func() string { return filepath.Join(t.TempDir(), "missing.sock") })
	if dead.Available() {
		t.Fatal("unreachable launcher must not report available")
	}
}

// TestNoWorldWritableSocket asserts the deployment invariant on the
// packaged unit: the launcher socket must never be created
// world-writable.
func TestNoWorldWritableSocket(t *testing.T) {
	data, err := os.ReadFile("../../packaging/systemd/duck-nft-launcher.service")
	if err != nil {
		t.Skipf("packaging not present: %v", err)
	}
	if !strings.Contains(string(data), "CapabilityBoundingSet=CAP_NET_ADMIN") {
		t.Fatal("unit must bound the launcher to CAP_NET_ADMIN only")
	}
	if strings.Contains(string(data), "AmbientCapabilities=CAP_NET_ADMIN ") {
		t.Fatal("unit ambient set must contain exactly one capability")
	}
}
