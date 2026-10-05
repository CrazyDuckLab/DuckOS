//go:build linux

// cmd/duck-frr-launcher/main.go: narrow privilege dropper
// for the DuckOS FRR child process.
//
// Architecture (see internal/frr/privilege_model.go):
//
//   duck-agent holds NO capabilities in its bounding
//   set. The agent pre-creates /run/frr/<ps>,
//   /var/lib/frr/<ps>, /etc/frr/<ps>, chowns the gid to
//   frr, chmods 0775, then exec's THIS launcher.
//
//   The launcher is a small Go binary that:
//     1. holds CAP_SETUID + CAP_SETGID + CAP_SETPCAP via
//        file capabilities (no parent caps inherited),
//     2. PR_SET_KEEPCAPS = 1,
//     3. setuid(frr) / setgid(frr) / setgroups([]) — drop
//        to the canonical `frr` service user,
//     4. cap_set_proc({NET_ADMIN, NET_RAW, NET_BIND_SERVICE})
//        — narrow P/E/I to what FRR actually uses
//        (netlink, raw sockets, TCP/179 bind). CAP_SYS_ADMIN
//        is intentionally absent.
//     5. exec /usr/lib/frr/{zebra,bgpd} with `-S`
//        (--skip_runas) so FRR's privs_init does NOT call
//        cap_set_proc.
//
//   The FRR child runs as uid=frr, gid=frr, no
//   supplementary groups, with P/E = {NET_ADMIN, NET_RAW,
//   NET_BIND_SERVICE}. NO CAP_SYS_ADMIN. NO CAP_SETUID/
//   SETGID/SETPCAP. NO duck-agent capabilities.
//
//   install.sh sets:
//     setcap cap_net_admin,cap_net_raw,cap_net_bind_service+ep
//         /usr/lib/frr/{zebra,bgpd}
//     setcap cap_setuid,cap_setgid,cap_setpcap+ep
//         /usr/libexec/duckos/duck-frr-launcher
//
//   This binary stays as small as possible so its
//   attack surface matches its narrow job.

package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"strconv"
	"unsafe"

	"golang.org/x/sys/unix"
)

// frrUid / frrGid are passed in by the runtime via
// --frr-uid / --frr-gid. Default 0/0 forces lookup.
var (
	frrUidFlag = flag.Int("frr-uid", 0, "uid of the frr service user")
	frrGidFlag = flag.Int("frr-gid", 0, "gid of the frr service group")
)

// frrRunUser / frrRunGroup are the names we resolve via
// NSS when the uid/gid flags are zero.
const (
	frrRunUser  = "frr"
	frrRunGroup = "frr"
)

// capsToKeep is the narrow capability set we hand to the
// FRR child. CAP_SYS_ADMIN is intentionally absent.
var capsToKeep = []string{"net_admin", "net_raw", "net_bind_service"}

func main() {
	pathspace := flag.String("pathspace", "", "FRR logical namespace name (-N flag); required")
	frrBinary := flag.String("binary", "zebra", "FRR binary: zebra or bgpd")
	configPath := flag.String("config", "", "frr.conf path (default /var/lib/frr/<ps>/frr.conf)")
	zServAddr := flag.String("zebra-addr", "127.0.0.1", "zserv API bind address (-A)")
	logPath := flag.String("log", "", "redirect stderr to this file; empty = inherit")
	flag.Parse()

	if *pathspace == "" {
		die("frr-launcher: --pathspace is required")
	}
	if *configPath == "" {
		*configPath = "/var/lib/frr/" + *pathspace + "/frr.conf"
	}

	// Anything after `--` is appended to the FRR argv.
	// The runtime uses this to pass `-i <pidfile>`,
	// `-s <socket-buf-len>` and similar zebra/bgpd flags
	// without us hard-coding them.
	var passthrough []string
	flagEnd := false
	for _, arg := range os.Args[1:] {
		if flagEnd {
			passthrough = append(passthrough, arg)
			continue
		}
		if arg == "--" {
			flagEnd = true
			continue
		}
	}

	// Resolve frr uid / gid from /etc/passwd if not passed.
	uid := *frrUidFlag
	gid := *frrGidFlag
	if uid == 0 || gid == 0 {
		var err error
		uid, gid, err = lookupFrrIds()
		if err != nil {
			die(fmt.Sprintf("frr-launcher: lookup frr ids: %v", err))
		}
	}

	// Step 1: keep all caps across the upcoming setuid
	// transition. PR_SET_KEEPCAPS preserves P/E when
	// transitioning from uid 0 to a non-zero uid.
	if _, _, errno := unix.Syscall(unix.PR_SET_KEEPCAPS, 1, 0, 0); errno != 0 {
		die(fmt.Sprintf("frr-launcher: PR_SET_KEEPCAPS: %v", errno))
	}

	// Step 2: drop supplementary groups, then gid, then
	// uid. Order matters: groups first so setgid doesn't
	// fail, and keepcaps before any of these.
	if err := unix.Setgroups([]int{}); err != nil {
		die(fmt.Sprintf("frr-launcher: setgroups: %v", err))
	}
	if err := unix.Setresgid(gid, gid, gid); err != nil {
		die(fmt.Sprintf("frr-launcher: setresgid(%d): %v", gid, err))
	}
	if err := unix.Setresuid(uid, uid, uid); err != nil {
		die(fmt.Sprintf("frr-launcher: setresuid(%d): %v", uid, err))
	}

	// Step 3: narrow P/E/I to capsToKeep. CAP_SYS_ADMIN
	// is deliberately excluded.
	if err := applyCaps(capsToKeep); err != nil {
		die(fmt.Sprintf("frr-launcher: applyCaps: %v", err))
	}

	// Step 4: exec the FRR binary with `-S` (skip_runas).
	binPath := "/usr/lib/frr/" + *frrBinary
	args := []string{binPath, "-d",
		"-f", *configPath,
		"-A", *zServAddr,
		"-N", *pathspace,
		"-u", frrRunUser,
		"-g", frrRunGroup,
		"-S",
	}
	args = append(args, passthrough...)
	cmd := exec.Command(binPath, args[1:]...)
	cmd.Stdin = nil
	cmd.Stdout = nil
	if *logPath != "" {
		f, err := os.OpenFile(*logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o640)
		if err != nil {
			die(fmt.Sprintf("frr-launcher: open log %s: %v", *logPath, err))
		}
		// The fd is duplicated across exec, so the daemon
		// inherits it as stderr.
		cmd.Stderr = f
		defer f.Close()
	} else {
		cmd.Stderr = os.Stderr
	}
	// exec returns only on failure.
	if err := cmd.Run(); err != nil {
		die(fmt.Sprintf("frr-launcher: exec %s: %v", binPath, err))
	}
}

func die(msg string) {
	fmt.Fprintln(os.Stderr, msg)
	os.Exit(1)
}

// lookupFrrIds resolves frr uid / gid via os/user. This
// avoids re-implementing NSS.
func lookupFrrIds() (uid, gid int, err error) {
	u, err := user.Lookup(frrRunUser)
	if err != nil {
		return 0, 0, fmt.Errorf("lookup %s: %w", frrRunUser, err)
	}
	uid, err = strconv.Atoi(u.Uid)
	if err != nil {
		return 0, 0, fmt.Errorf("parse uid %s: %w", u.Uid, err)
	}
	gid, err = strconv.Atoi(u.Gid)
	if err != nil {
		return 0, 0, fmt.Errorf("parse gid %s: %w", u.Gid, err)
	}
	return uid, gid, nil
}

// applyCaps sets P/E/I to the union of the named
// capabilities. The launcher's file caps include CAP_SETPCAP
// so the kernel accepts this cap_set call.
//
//   PR_SET_KEEPCAPS=1 was issued in main(), so the caller's
//   permitted/effective sets survived the privilege drop.
//   We then replace them with capsToKeep, dropping
//   CAP_SETUID + CAP_SETGID + CAP_SETPCAP (the launcher's
//   own caps that the child does not need).
//
//   We use LINUX_CAPABILITY_VERSION_3 with two uint32s per
//   set (the 64-bit caps format). golang.org/x/sys/unix
//   only exposes V1/V2 via CapUserData, so we issue the
//   syscall directly.
func applyCaps(names []string) error {
	var lo, hi uint32
	for _, n := range names {
		v, ok := capByName(n)
		if !ok {
			return fmt.Errorf("unknown capability %q", n)
		}
		if v < 32 {
			lo |= 1 << uint(v)
		} else if v < 64 {
			hi |= 1 << uint(v-32)
		} else {
			return fmt.Errorf("capability %q is beyond caps 64-bit set", n)
		}
	}
	type hdrT struct {
		Version uint32
		Pid     int32
	}
	type dataT struct {
		effective0, effective1   uint32
		permitted0, permitted1   uint32
		inheritable0, inheritable1 uint32
	}
	hdr := hdrT{Version: 0x20080522, Pid: 0}
	data := dataT{
		effective0:   lo,
		effective1:   hi,
		permitted0:   lo,
		permitted1:   hi,
		inheritable0: lo,
		inheritable1: hi,
	}
	_, _, errno := unix.Syscall(
		unix.SYS_CAPSET,
		uintptr(unsafe.Pointer(&hdr)),
		uintptr(unsafe.Pointer(&data)),
		0,
	)
	if errno != 0 {
		return errno
	}
	return nil
}

// capByName is the Linux cap name -> number map. Kept
// inline for binary size; values are the canonical
// /usr/include/linux/capability.h definitions.
func capByName(name string) (int, bool) {
	m := map[string]int{
		"chown":            0,
		"dac_override":     1,
		"dac_read_search":  2,
		"fowner":           3,
		"fsetid":           4,
		"kill":             5,
		"setgid":           6,
		"setuid":           7,
		"setpcap":          8,
		"linux_immutable":  9,
		"net_bind_service": 10,
		"net_broadcast":    11,
		"net_admin":        12,
		"net_raw":          13,
		"ipc_lock":         14,
		"ipc_owner":        15,
		"sys_module":       16,
		"sys_rawio":        17,
		"sys_chroot":       18,
		"sys_ptrace":       19,
		"sys_pacct":        20,
		"sys_admin":        21,
		"sys_boot":         22,
		"sys_nice":         23,
		"sys_resource":     24,
		"sys_time":         25,
		"sys_tty_config":   26,
		"mknod":            27,
		"lease":            28,
		"audit_write":      29,
		"audit_control":    30,
		"setfcap":          31,
	}
	v, ok := m[name]
	return v, ok
}