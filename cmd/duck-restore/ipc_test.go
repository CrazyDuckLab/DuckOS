//go:build linux

package main

import (
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/agent"
)

func TestH09UnauthorizedRestorePeerHelper(t *testing.T) {
	socket := os.Getenv("DUCKOS_RESTORE_DENIED_SOCKET")
	if socket == "" {
		t.Skip("subprocess helper")
	}
	if os.Geteuid() == 0 {
		t.Fatal("negative peer must be unprivileged")
	}
	conn, err := net.DialTimeout("unix", socket, time.Second)
	if err != nil {
		t.Fatal("fixture must reach socket to exercise SO_PEERCRED", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(time.Second))
	_, _ = conn.Write([]byte("GET / HTTP/1.1\r\nHost: restore\r\n\r\n"))
	var b [128]byte
	n, _ := conn.Read(b[:])
	if n != 0 {
		t.Fatalf("unauthorized peer received service response: %q", b[:n])
	}
}

func TestH09RestoreRejectsUnauthorizedPeer(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root only to create unprivileged peer")
	}
	dir, err := os.MkdirTemp("/tmp", "duck-restore-peer-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	if err := os.Chmod(dir, 0755); err != nil {
		t.Fatal(err)
	}
	socket := filepath.Join(dir, "restore.sock")
	listener, err := agent.Listen(socket, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	// Deliberately permit connect to prove the credential gate independently
	// of the stricter production socket mode 0600 / directory mode 0700.
	if err := os.Chmod(socket, 0666); err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("authorized")) })}
	defer server.Close()
	go server.Serve(listener)
	conn, err := net.Dial("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	_ = conn.SetDeadline(time.Now().Add(time.Second))
	_, _ = conn.Write([]byte("GET / HTTP/1.0\r\n\r\n"))
	raw, err := io.ReadAll(conn)
	conn.Close()
	if err != nil || len(raw) == 0 {
		t.Fatal("authorized root rejected", err)
	}
	binary, err := os.ReadFile(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	helper := filepath.Join(dir, "peer-test")
	if err := os.WriteFile(helper, binary, 0755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(helper, "-test.run=^TestH09UnauthorizedRestorePeerHelper$", "-test.timeout=10s")
	cmd.Env = append(os.Environ(), "DUCKOS_RESTORE_DENIED_SOCKET="+socket)
	cmd.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: 65534, Gid: 65534}}
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("unauthorized peer test: %v %s", err, out)
	}
}
