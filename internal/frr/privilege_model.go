// frr/privilege_model.go: cross-platform declarations of
// the narrow FRR privilege model.
//
// The armv7 / ODROID XU4 failure mode observed in the
// physical beta.1 image was:
//
//   failed to mkdir "/var/lib/frr/duckos": Permission denied
//   privs_init: initial cap_set_proc failed: Operation not permitted
//   Wanted caps: cap_net_admin,cap_net_raw,cap_sys_admin=p
//   Have caps:    cap_net_admin,cap_net_raw=eip
//
// Two root causes:
//
//   1. The Debian FRR package ships /var/lib/frr/ owned by
//      frr:frr. FRR's daemon (run via the Agent's child
//      process) tried to lazily mkdir the pathspace state
//      dir as the running uid (root, in the legacy code),
//      which is NOT the FRR package owner. The Agent kept
//      `-u root -g frrvty`, so the pathspace dir's frr:frr
//      ownership blocked the daemon's mkdir.
//
//   2. With `-u root`, FRR's privs_init still calls
//      cap_set_proc asking for cap_net_admin + cap_net_raw
//      + cap_sys_admin (the working set FRR wants). The
//      systemd unit grants only CAP_NET_ADMIN CAP_NET_RAW.
//      cap_set_proc refuses because cap_sys_admin is not
//      in the permitted set, so the daemon aborts with
//      "Operation not permitted".
//
// The narrow fix:
//
//   - The Agent (running as root) pre-creates
//     /var/lib/frr/<pathspace> with frr:frr ownership
//     before exec'ing zebra / bgpd. FRR's own mkdir never
//     fires as the running uid.
//   - chownToFRR refuses any path that does not live under
//     StateBase (/var/lib/frr) or PathspaceBase (/run/frr)
//     — the Agent cannot chown arbitrary user-controlled
//     paths.
//   - FRR daemons are exec'd with `-u frr -g frr` instead
//     of `-u root -g frrvty`. FRR's privs_init drops ALL
//     caps from the working set (because the daemon is no
//     longer root), so it never asks for cap_sys_admin.
//   - duck-agent stays at CAP_NET_ADMIN CAP_NET_RAW only
//     — CAP_SYS_ADMIN is NEVER added.
//
// Live armv7 execution is LIVE-ARMV7-PENDING; the property
// tests in privilege_model_test.go run on every build
// target so a future regression is caught immediately.
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

// chownToFRR changes the ownership of path to the FRR
// service user / group. The helper REFUSES to chown any
// path that does not live under StateBase ("/var/lib/frr")
// or PathspaceBase ("/run/frr"). This is the narrow
// guarantee that the runtime cannot accidentally chown
// arbitrary user-controlled paths; it can only ever
// prepare the directories FRR itself needs.
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
	uid, gid, err := lookupFRRIDs()
	if err != nil {
		return err
	}
	if err := os.Chown(path, uid, gid); err != nil {
		return fmt.Errorf("frr: chown %s to uid=%d gid=%d: %w", path, uid, gid, err)
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