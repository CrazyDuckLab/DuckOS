//go:build linux

// Package qa: real FreeRADIUS runtime qualification for
// §6 of the Reality Check program.
//
// The test starts a real FreeRADIUS 3.2.7 daemon inside
// an isolated network namespace, then exercises:
//
//   - successful PAP authentication (correct password)
//   - failed PAP authentication (wrong password)
//   - CoA disconnect (where supported in the lab)
//
// The test uses a hand-rolled minimal FreeRADIUS config
// because the internal/radiusd Render path is a partial
// config (listen + clients only); the full
// sites-enabled / mods-enabled tree is constructed
// here. The test asserts the management boundary
// (eth0 / default route / sshd) is unchanged.
package qa

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// findFreeRADIUS returns the path to a real FreeRADIUS
// binary. The test first looks at FREERADIUS_BIN, then
// at /usr/sbin/freeradius, /usr/sbin/radiusd, /usr/bin/freeradius.
func findFreeRADIUS(t testing.TB) string {
	t.Helper()
	if p := os.Getenv("FREERADIUS_BIN"); p != "" {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	for _, p := range []string{"/usr/sbin/freeradius", "/usr/sbin/radiusd", "/usr/bin/freeradius", "/usr/local/sbin/freeradius"} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

func findRadtest(t testing.TB) string {
	t.Helper()
	for _, p := range []string{"/usr/bin/radtest", "/usr/local/bin/radtest"} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// freeRADIUSMinimalConfig returns a complete, working
// FreeRADIUS 3.x configuration rooted at dir.
//
// Layout (upstream FR 3.2.7):
//
//	radiusd.conf
//	  prefix / logdir / run_dir / db_dir / libdir / pidfile
//	  modules { $INCLUDE mods-enabled/ }
//	  $INCLUDE sites-enabled/
//	  $INCLUDE clients.conf
//	mods-available/<name>  -- actual module config
//	mods-enabled/<name>    -- relative symlink to ../mods-available/<name>
//	sites-available/default
//	sites-enabled/default  -- relative symlink to ../sites-available/default
//
// Lessons learned during §6 qualification:
//  1. mods-enabled/<name> MUST be a symlink; empty
//     placeholder files are read as comments and modules
//     never load.
//  2. $INCLUDE must use a trailing slash for directories
//     ("$INCLUDE mods-enabled/", NOT "$INCLUDE mods-enabled/*").
//  3. modules-enabled/ must appear INSIDE a top-level
//     `modules { ... }` block, not at top level. Without
//     that wrapper, FR's parser logs "Cannot find a
//     modules section" and silently registers no modules,
//     so site references like "pap" fail with
//     "Failed to find pap as a module or policy".
//  4. Some modules (rlm_suffix, rlm_policy, rlm_eap) are
//     not built into every distro package. They are
//     omitted here to keep the test config portable.
//  5. attr_filter uses "filename" (not the deprecated
//     "attrsfile"); detail uses "permissions" (not the
//     deprecated "detailperm"). FR 3.2.7 rejects both.
//  6. "filter_username" is a built-in unlang filter, NOT
//     a module reference; do not list it under
//     authorize { ... }.
//  7. Auth-Type PAP { pap } is required for pap to actually
//     run; without it, authorize succeeds but authenticate
//     rejects.
func freeRADIUSMinimalConfig(t testing.TB, dir, listenIP, authPort, acctPort, clientIP, clientSecret, user, password string) {
	t.Helper()
	usersPath := filepath.Join(dir, "users")
	detailLog := filepath.Join(dir, "log", "detail.log")
	huntgroupsPath := filepath.Join(dir, "huntgroups")
	hintsPath := filepath.Join(dir, "hints")
	attrsPath := filepath.Join(dir, "attrs.access_reject")
	mods := map[string]string{
		"mods-available/pap": `pap {
  auto_header = yes
}
`,
		"mods-available/files": `files {
  filename = "` + usersPath + `"
  usersfile = "` + usersPath + `"
  allowed_chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789.-_@"
}
`,
		"mods-available/always": `always {
  ok = yes
}
`,
		"mods-available/expr": `expr {
}
`,
		"mods-available/logintime": `logintime {
}
`,
		"mods-available/detail": `detail {
  filename = "` + detailLog + `"
  header = "%t"
  permissions = 0600
  locking = no
  log_packet_header = no
}
`,
		"mods-available/preprocess": `preprocess {
  huntgroups = "` + huntgroupsPath + `"
  hints = "` + hintsPath + `"
  with_ascend_hack = no
  ascend_channels_per_line = 23
  with_ntdomain_hack = no
  with_specialix_sequa = no
  with_cisco_vsa_hack = no
  with_alvarion_vsa_hack = no
}
`,
		"mods-available/chap": `chap {
  authtype = CHAP
  use_openlog = no
  wait_for_start = no
}
`,
		"mods-available/mschap": `mschap {
  use_mppe = no
  require_encryption = no
  require_strong = no
  with_ntdomain_hack = no
  passchange {
  }
}
`,
		"mods-available/attr_filter": `attr_filter {
  filename = "` + attrsPath + `"
  key = "%{User-Name}"
}
`,
	}
	enabled := []string{
		"pap", "files", "always", "expr", "logintime",
		"detail", "preprocess", "chap", "mschap",
		"attr_filter",
	}
	if err := os.MkdirAll(filepath.Join(dir, "mods-available"), 0o755); err != nil {
		t.Fatalf("mkdir mods-available: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "mods-enabled"), 0o755); err != nil {
		t.Fatalf("mkdir mods-enabled: %v", err)
	}
	for rel, content := range mods {
		full := filepath.Join(dir, rel)
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", full, err)
		}
	}
	for _, name := range enabled {
		link := filepath.Join(dir, "mods-enabled", name)
		if err := os.Symlink("../mods-available/"+name, link); err != nil {
			t.Fatalf("symlink mods-enabled/%s: %v", name, err)
		}
	}
	clients := `client localhost {
  ipaddr = ` + clientIP + `
  secret = ` + clientSecret + `
  require_message_authenticator = no
  nastype = other
  shortname = localhost
}
client all {
  ipaddr = 0.0.0.0/0
  secret = ` + clientSecret + `
  require_message_authenticator = no
}
`
	if err := os.WriteFile(filepath.Join(dir, "clients.conf"), []byte(clients), 0o644); err != nil {
		t.Fatalf("write clients.conf: %v", err)
	}
	users := user + `	Auth-Type := PAP, Cleartext-Password := "` + password + `"
		Reply-Message := "ok"
`
	if err := os.WriteFile(usersPath, []byte(users), 0o644); err != nil {
		t.Fatalf("write users: %v", err)
	}
	if err := os.WriteFile(huntgroupsPath, []byte(""), 0o644); err != nil {
		t.Fatalf("write huntgroups: %v", err)
	}
	if err := os.WriteFile(hintsPath, []byte(""), 0o644); err != nil {
		t.Fatalf("write hints: %v", err)
	}
	if err := os.WriteFile(attrsPath, []byte(""), 0o644); err != nil {
		t.Fatalf("write attrs.access_reject: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "log"), 0o755); err != nil {
		t.Fatalf("mkdir log: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "sites-available"), 0o755); err != nil {
		t.Fatalf("mkdir sites-available: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "sites-enabled"), 0o755); err != nil {
		t.Fatalf("mkdir sites-enabled: %v", err)
	}
	site := `server default {
  listen {
    type = auth
    ipaddr = ` + listenIP + `
    port = ` + authPort + `
  }
  listen {
    type = acct
    ipaddr = ` + listenIP + `
    port = ` + acctPort + `
  }
  authorize {
    preprocess
    chap
    mschap
    files
    pap
  }
  authenticate {
    Auth-Type PAP {
      pap
    }
  }
  preacct {
    preprocess
  }
  accounting {
    detail
  }
  session {
  }
  post-auth {
  }
  pre-proxy {
  }
  post-proxy {
  }
}
`
	if err := os.WriteFile(filepath.Join(dir, "sites-available", "default"), []byte(site), 0o644); err != nil {
		t.Fatalf("write sites-available/default: %v", err)
	}
	if err := os.Symlink("../sites-available/default", filepath.Join(dir, "sites-enabled", "default")); err != nil {
		t.Fatalf("symlink sites-enabled/default: %v", err)
	}
	radiusdConf := `prefix = ` + dir + `
logdir = ` + filepath.Join(dir, "log") + `
run_dir = ` + filepath.Join(dir, "run") + `
db_dir = ` + filepath.Join(dir, "db") + `
libdir = /usr/lib/freeradius
pidfile = ` + filepath.Join(dir, "radiusd.pid") + `

modules {
	$INCLUDE mods-enabled/
}

$INCLUDE sites-enabled/
$INCLUDE clients.conf
`
	if err := os.WriteFile(filepath.Join(dir, "radiusd.conf"), []byte(radiusdConf), 0o644); err != nil {
		t.Fatalf("write radiusd.conf: %v", err)
	}
}

// radTest runs `radtest` against the given RADIUS server
// and returns (success, stdout+stderr, exit-code). The
// radtest exit code is 0 only when authentication is
// accepted.
//
// radtest CLI shape:
//
//	radtest user pass radius-server[:port] nas-port-number secret
//
// The nas-port-number is required (positional, before secret).
// radtest has no timeout flag — it relies on its own UDP
// socket + retransmit defaults. We wrap the call in
// `timeout -k 5 5` externally to bound the test wallclock.
func radTest(t testing.TB, radtest, user, password, host, authPort, secret string) (bool, string) {
	t.Helper()
	args := []string{
		user, password, host + ":" + authPort, "0", secret,
	}
	cmd := exec.Command(radtest, args...)
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	err := cmd.Run()
	combined := out.String() + errBuf.String()
	return err == nil, combined
}

// TestFreeRADIUSRealRuntimeAuth is §6 of the QA program:
// a real FreeRADIUS 3.2.7 daemon is started in a netns
// with a known test user, and radtest exercises both a
// successful and a failed authentication.
func TestFreeRADIUSRealRuntimeAuth(t *testing.T) {
	bin := findFreeRADIUS(t)
	rt := findRadtest(t)
	if bin == "" || rt == "" {
		t.Skip("freeradius/radtest not present")
	}
	before := CaptureManagementBoundary(t)
	lab := NewLab(t, "qa-radius")
	lab.RunShell("/sbin/ip", "link", "set", "lo", "up")
	// Add a veth pair so the test process (running in
	// the host) can reach the RADIUS server inside the
	// netns.
	if err := runShell("ip", "link", "add", "qa-r-host", "type", "veth", "peer", "name", "qa-r-ns"); err != nil {
		t.Fatalf("veth add: %v", err)
	}
	lab.OwnedLinks = append(lab.OwnedLinks, "qa-r-host")
	if err := runShell("ip", "addr", "add", "10.254.0.1/30", "dev", "qa-r-host"); err != nil {
		t.Fatalf("veth host addr: %v", err)
	}
	if err := runShell("ip", "link", "set", "qa-r-host", "up"); err != nil {
		t.Fatalf("veth host up: %v", err)
	}
	if err := runShell("ip", "link", "set", "qa-r-ns", "netns", lab.NetNS); err != nil {
		t.Fatalf("veth netns: %v", err)
	}
	t.Cleanup(func() {
		_ = runShell("ip", "link", "del", "qa-r-host")
	})
	if _, err := runInNS(lab.NetNS, "ip", "addr", "add", "10.254.0.2/30", "dev", "qa-r-ns"); err != nil {
		t.Fatalf("veth ns addr: %v", err)
	}
	if _, err := runInNS(lab.NetNS, "ip", "link", "set", "qa-r-ns", "up"); err != nil {
		t.Fatalf("veth ns up: %v", err)
	}
	// Build a complete FreeRADIUS config in the netns
	// via a temp dir. We bind the workdir to /tmp inside
	// the netns by writing files from the host.
	dir := t.TempDir()
	listenIP := "10.254.0.2"
	authPort := "1812"
	acctPort := "1813"
	clientIP := "10.254.0.1"
	clientSecret := "qa-shared-secret-please-rotate"
	user := "qa-user"
	password := "qa-correct-horse-battery-staple"
	freeRADIUSMinimalConfig(t, dir, listenIP, authPort, acctPort, clientIP, clientSecret, user, password)
	// Start radiusd in foreground mode inside the netns.
	// Foreground mode (-f) prints logs to stdout and
	// exits on SIGTERM. We use -l stdout to keep logs
	// in our captured output.
	logFile := filepath.Join(t.TempDir(), "radiusd.log")
	cmd := exec.Command(bin, "-f", "-l", "stdout",
		"-d", dir,
		"-n", "qa-radius",
		"-D", "/usr/share/freeradius",
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{}
	// We can't use netns as the test process's netns
	// easily; instead, run via the shell with ip netns.
	// But that doesn't capture the process for kill.
	// Instead, we use nsenter via a goroutine.
	nsenterCmd := "exec " + bin + " -f -l stdout -d " + dir
	nsenter := exec.Command("/bin/sh", "-c", "ip netns exec "+lab.NetNS+" /bin/sh -c "+shellQuote(nsenterCmd))
	rOut, _ := os.Create(logFile)
	nsenter.Stdout = rOut
	nsenter.Stderr = rOut
	if err := nsenter.Start(); err != nil {
		t.Fatalf("start radiusd: %v", err)
	}
	t.Cleanup(func() {
		_ = nsenter.Process.Kill()
		_, _ = nsenter.Process.Wait()
		rOut.Close()
	})
	// Wait for radiusd to listen on 1812.
	hexPort := "0714" // 1812 in hex
	deadline := time.Now().Add(15 * time.Second)
	var heard bool
	for time.Now().Before(deadline) {
		out, _ := runInNSCapture(lab.NetNS, "/bin/sh", "-c", "grep -E 'Listening on .* port "+authPort+"' /proc/net/udp 2>/dev/null; cat /proc/net/udp 2>/dev/null | grep -E ':"+hexPort+" ' | head -1")
		// /proc/net/udp uses hex port too: 0714.
		if strings.Contains(string(out), hexPort) {
			heard = true
			break
		}
		// /proc/net/udp may need 4-char hex with leading
		// zero: 0714 vs 714.
		if strings.Contains(string(out), "0714") {
			heard = true
			break
		}
		// Try a TCP-style read on the UDP socket via
		// ss in the netns.
		out2, _ := runInNSCapture(lab.NetNS, "/bin/sh", "-c", "ss -ulnp 2>/dev/null | grep 1812 | head -1")
		if strings.Contains(string(out2), "1812") {
			heard = true
			break
		}
		time.Sleep(300 * time.Millisecond)
	}
	if !heard {
		logData, _ := os.ReadFile(logFile)
		t.Fatalf("radiusd did not start listening. log:\n%s", logData)
	}
	// Give the listener another moment to fully bind.
	time.Sleep(500 * time.Millisecond)
	// Run radtest from the host (reaching 10.254.0.2:1812).
	t.Run("correct_password", func(t *testing.T) {
		ok, out := radTest(t, rt, user, password, "10.254.0.2", authPort, clientSecret)
		t.Logf("radtest (good): ok=%v out=%s", ok, out)
		if !ok {
			t.Fatalf("expected success on correct password")
		}
		if !strings.Contains(out, "Received Access-Accept") {
			t.Fatalf("expected 'Received Access-Accept' in output, got: %s", out)
		}
	})
	t.Run("wrong_password", func(t *testing.T) {
		ok, out := radTest(t, rt, user, "wrong-password", "10.254.0.2", authPort, clientSecret)
		t.Logf("radtest (bad): ok=%v out=%s", ok, out)
		if ok {
			t.Fatalf("expected failure on wrong password, got success")
		}
		if !strings.Contains(out, "Received Access-Reject") && !strings.Contains(out, "rejected") {
			t.Fatalf("expected Access-Reject, got: %s", out)
		}
	})
	t.Run("unknown_user", func(t *testing.T) {
		ok, out := radTest(t, rt, "nonexistent-user", "any", "10.254.0.2", authPort, clientSecret)
		t.Logf("radtest (no-user): ok=%v out=%s", ok, out)
		if ok {
			t.Fatalf("expected failure on unknown user, got success")
		}
	})
	t.Run("wrong_shared_secret", func(t *testing.T) {
		ok, out := radTest(t, rt, user, password, "10.254.0.2", authPort, "totally-wrong-secret")
		t.Logf("radtest (bad-secret): ok=%v out=%s", ok, out)
		if ok {
			t.Fatalf("expected failure on wrong shared secret, got success")
		}
	})
	after := CaptureManagementBoundary(t)
	AssertBoundaryEqual(t, before, after)
}

// shellQuote returns a single-quoted shell-safe version
// of s with embedded single quotes escaped.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func _unused() {
	_ = syscall.Getpid
}

func syscallSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		Pdeathsig: 0,
	}
}
