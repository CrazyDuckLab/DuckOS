//go:build linux

// fakefrr is a test helper that stands in for a real FRR
// daemon. After self-re-exec, it presents:
//
//   /proc/<pid>/cmdline  = "/usr/lib/frr/<bin> -N <pathspace>"
//   /proc/<pid>/comm     = <bin>          (PR_SET_NAME)
//
// Usage:
//
//	fakefrr <bin> <pathspace> [pidfile]
//
// argv[0] is exactly /usr/lib/frr/<bin> so the launcher's
// isFRRProcess identity gate (which checks
// /proc/<pid>/cmdline for "/usr/lib/frr/<bin>") accepts
// the process.

package main

import (
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: fakefrr <bin> <pathspace> [pidfile]")
		os.Exit(2)
	}

	// First entry: argv[0] is the fakefrr binary path.
	// Re-exec ourselves with argv[0] rewritten so the
	// kernel-managed cmdline matches the FRR identity.
	if !strings.HasPrefix(os.Args[0], "/usr/lib/frr/") {
		bin := os.Args[1]
		ps := os.Args[2]
		pidfile := ""
		if len(os.Args) >= 4 {
			pidfile = os.Args[3]
		}
		newArgv0 := "/usr/lib/frr/" + bin
		argv := []string{newArgv0, "-N", ps, "--pidfile", pidfile}
		self, err := os.Executable()
		if err != nil {
			fmt.Fprintln(os.Stderr, "fakefrr: os.Executable:", err)
			os.Exit(1)
		}
		if err := syscall.Exec(self, argv, os.Environ()); err != nil {
			fmt.Fprintln(os.Stderr, "fakefrr: exec:", err)
			os.Exit(1)
		}
		// unreachable
	}

	// Post-exec. argv[0] = /usr/lib/frr/<bin>.
	bin := strings.TrimPrefix(os.Args[0], "/usr/lib/frr/")
	pidfile := ""
	for i := 1; i < len(os.Args); i++ {
		if os.Args[i] == "--pidfile" && i+1 < len(os.Args) {
			pidfile = os.Args[i+1]
		}
	}

	// Set comm = <bin> (truncated to 15 chars by kernel).
	nameBytes := append([]byte(bin), 0)
	if len(nameBytes) < 16 {
		nameBytes = append(nameBytes, make([]byte, 16-len(nameBytes))...)
	}
	if _, _, errno := syscall.Syscall(syscall.SYS_PRCTL, syscall.PR_SET_NAME, uintptr(unsafe.Pointer(&nameBytes[0])), 0); errno != 0 {
		fmt.Fprintln(os.Stderr, "fakefrr: PR_SET_NAME errno=", errno)
	}

	if pidfile != "" {
		if err := os.WriteFile(pidfile, []byte(strconv.Itoa(os.Getpid())+"\n"), 0o644); err != nil {
			fmt.Fprintln(os.Stderr, "fakefrr: pidfile write:", err)
		}
	}

	sigCh := make(chan os.Signal, 4)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-sigCh
		if pidfile != "" {
			_ = os.Remove(pidfile)
		}
		os.Exit(0)
	}()

	time.Sleep(600 * time.Second)
	if pidfile != "" {
		_ = os.Remove(pidfile)
	}
}
