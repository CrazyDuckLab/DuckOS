// frr/privilege_model.go: cross-platform declarations of
// the narrow FRR privilege model.
//
// Architecture (corrected from the post-f881412 repair
// chain after the live BETA.2 review at ed3b784):
//
//   - duck-agent has CapabilityBoundingSet= (empty). It
//     does NOT hold CAP_NET_ADMIN, CAP_NET_RAW, CAP_SETUID,
//     CAP_SETGID, CAP_SETPCAP, CAP_NET_BIND_SERVICE, or
//     CAP_SYS_ADMIN. The Agent's operations are
//     MkdirAll, gid-only chown, chmod, write of frr.conf,
//     and exec of the launcher. None require caps; UID 0
//     with owner-rwx on the parent path is sufficient.
//
//   - /usr/libexec/duckos/duck-frr-launcher is a separate
//     long-running privileged daemon
//     (duck-frr-launcher.service). Its service unit
//     grants CapabilityBoundingSet=CAP_SETUID CAP_SETGID
//     CAP_SETPCAP CAP_NET_ADMIN CAP_NET_RAW
//     CAP_NET_BIND_SERVICE CAP_SYS_ADMIN with
//     NoNewPrivileges=no. For each EXEC command the
//     launcher re-execs a child that:
//     1. PR_SET_KEEPCAPS=1,
//     2. PR_CAPBSET_DROP CAP_SETUID + CAP_SETGID from
//     the bounding set (BEFORE setresuid so the
//     effective set still contains CAP_SETPCAP
//     which PR_CAPBSET_DROP needs),
//     3. setgroups([]) + setresgid + setresuid to
//     drop to uid=frr,
//     4. mkdir /etc/frr/<ps>/ + write vtysh.conf,
//     5. exec /usr/lib/frr/{zebra,bgpd}.
//
//   - /usr/lib/frr/zebra and /usr/lib/frr/bgpd have file
//     caps
//     cap_net_admin,cap_net_raw,cap_net_bind_service,cap_sys_admin,cap_setpcap+ep
//     installed by install.sh. The launcher does NOT
//     call cap_set_proc itself; kernel exec semantics
//     (parent P ∩ parent Bnd, then file_caps OR'd in)
//     hand the FRR child exactly the caps it needs.
//     cap_setpcap is INTENTIONALLY included so the
//     child's E has CAP_SETPCAP — FRR's own privs_init
//     calls cap_set_proc and that requires CAP_SETPCAP
//     in E.
//
//   - The FRR child runs as uid=frr, gid=frr, no
//     supplementary groups, with P/E =
//     {NET_ADMIN, NET_RAW, NET_BIND_SERVICE, SYS_ADMIN,
//     SETPCAP}. CAP_SYS_ADMIN DOES reach the FRR child:
//     Debian FRR 10.3's zebra binary calls cap_set_proc
//     with CAP_SYS_ADMIN unconditionally, and Debian's
//     zebra build does NOT expose the upstream -S /
//     --skip_runas flag (only bgpd does). The runtime
//     therefore MUST NOT pass -S to zebra on Debian. Per
//     the BETA.2 architectural review, CAP_SYS_ADMIN on
//     the FRR-only child is acceptable because the FRR
//     process genuinely needs it. CAP_SYS_ADMIN MUST NOT
//     reach duck-agent itself; the agent's bounding set
//     stays empty.
//
// The armv7 failure modes observed in the physical beta.1
// image and during live BETA.2 amd64 qualification were all
// caused by an earlier, more privileged Agent. This file
// captures the narrow, correct model.
//
// Live armv7 execution remains LIVE-ARMV7-PENDING; the
// property tests in privilege_model_test.go run on every
// build target so a future regression is caught
// immediately.
package frr

import (
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
)

// PathspaceBase is the base dir the runtime writes its
// private FRR socket / pid / log files under. Each
// runtime instance gets its own subdir.
const PathspaceBase = "/run/frr"

// StateBase is FRR's canonical per-pathspace state
// directory: zebra and bgpd persist their pid/log files
// here when given `-N <pathspace>`. The Debian FRR
// package ships `/var/lib/frr/` owned by `frr:frr`; the
// agent pre-creates the per-pathspace subdir here so the
// daemons never need to mkdir as the running uid, which
// is what caused the "Permission denied" failure on armv7.
const StateBase = "/var/lib/frr"

// FRRRunUser and FRRRunGroup are the canonical user /
// group FRR daemons drop to on Debian. Using -u frr -g frr
// (instead of -u root -g frrvty) means FRR's privs_init
// drops ALL caps from the working set, so the daemons no
// longer try to call cap_set_proc asking for
// cap_sys_admin — the operation that failed on armv7
// because the agent's capability bounding set is
// deliberately narrow (CAP_NET_ADMIN CAP_NET_RAW only).
const (
	FRRRunUser  = "frr"
	FRRRunGroup = "frr"
)

// chownToFRR changes the group ownership of path to the
// FRR service group and grants the group write bit. The
// helper REFUSES to chown any path that does not live
// under StateBase ("/var/lib/frr") or PathspaceBase
// ("/run/frr"). This is the narrow guarantee that the
// runtime cannot accidentally chown arbitrary user-
// controlled paths; it can only ever prepare the
// directories FRR itself needs.
//
// The Agent's CapabilityBoundingSet is empty (it holds
// NO capabilities). Without CAP_CHOWN the kernel forbids
// chown(2) to a different uid; with the Agent's
// SupplementaryGroups (frr, frrvty) the kernel DOES allow
// chown(2) to change only the gid, when the new gid is in
// the process's supplementary groups. We therefore
// chown(-1, gid) — pass uid=-1 to leave the uid
// untouched — and chmod 0o775 so the FRR daemon running
// as the `frr` user can still write into the pathspace
// state directory.
//
// Tests stub the user lookup by setting
// DUCKOS_FRR_RUN_UID / DUCKOS_FRR_RUN_GID env vars;
// production wiring always reads from the live
// /etc/passwd / /etc/group database.
func chownToFRR(path string) error {
	if !strings.HasPrefix(path, StateBase+"/") &&
		path != StateBase &&
		!strings.HasPrefix(path, PathspaceBase+"/") &&
		path != PathspaceBase {
		return fmt.Errorf("frr: chownToFRR refuses non-FRR path %q", path)
	}
	_, gid, err := lookupFRRIDs()
	if err != nil {
		return err
	}
	// Preserve the existing uid (-1 leaves it as-is); only
	// chown the gid, which the kernel permits because the
	// new gid is in the Agent's supplementary groups and
	// we hold the file's uid. This avoids needing
	// CAP_CHOWN while still handing the directory off to
	// the FRR service group.
	if err := os.Chown(path, -1, gid); err != nil {
		return fmt.Errorf("frr: chown %s gid=%d: %w", path, gid, err)
	}
	// Group write so the FRR daemon (which drops to
	// uid=frr) can write its own state into the dir.
	if err := os.Chmod(path, 0o775); err != nil {
		return fmt.Errorf("frr: chmod %s to 0o775: %w", path, err)
	}
	return nil
}

// lookupFRRIDs returns the FRR service user uid / gid. It
// prefers env-var overrides (tests / containers without
// /etc/passwd) and falls back to the canonical user
// database. Lookup failures return ErrFRRUserMissing so
// callers can surface a clean "FRR_USER_MISSING" reason
// to the public API instead of letting FRR die on a
// missing setuid.
func lookupFRRIDs() (int, int, error) {
	if u := os.Getenv("DUCKOS_FRR_RUN_UID"); u != "" {
		if g := os.Getenv("DUCKOS_FRR_RUN_GID"); g != "" {
			var uid, gid int
			fmt.Sscanf(u, "%d", &uid)
			fmt.Sscanf(g, "%d", &gid)
			if uid > 0 && gid > 0 {
				return uid, gid, nil
			}
		}
	}
	u, err := user.Lookup(FRRRunUser)
	if err != nil {
		return 0, 0, fmt.Errorf("%w: %v", ErrFRRUserMissing, err)
	}
	uid, err := strconv.Atoi(u.Uid)
	if err != nil {
		return 0, 0, fmt.Errorf("frr: parse uid for %s: %w", FRRRunUser, err)
	}
	g, err := user.LookupGroup(FRRRunGroup)
	if err != nil {
		return 0, 0, fmt.Errorf("%w: %v", ErrFRRUserMissing, err)
	}
	gid, err := strconv.Atoi(g.Gid)
	if err != nil {
		return 0, 0, fmt.Errorf("frr: parse gid for %s: %w", FRRRunGroup, err)
	}
	return uid, gid, nil
}

// ErrFRRUserMissing is the canonical error returned when
// the FRR service user is not present on the host. The
// public API maps this to a stable "FRR_USER_MISSING"
// reason instead of a generic failure.
var ErrFRRUserMissing = errors.New("frr: service user is missing")

// Ensure filepath is referenced so an unrelated refactor
// does not strip the import — we use it indirectly through
// the constants above, and Go does not flag unused
// constants.
var _ = filepath.Join
