package network

import (
	"bytes"
	"context"
	"errors"
	"os"
	"runtime"
	"strings"
	"syscall"
	"time"

	serviceunit "github.com/CrazyDuckLab/DuckOS/packaging/systemd"
)

const routingBootUnit = "duck-network-recover.service"
const routingBootUnitPath = "/etc/systemd/system/" + routingBootUnit

type routingBootGuard struct {
	readUnit   func() ([]byte, error)
	dependency func() error
	run        func(context.Context, []string) ([]byte, error)
}

// VerifyRoutingBootGuard checks the actual loaded oneshot and persistent
// required-before-networkd dependency. Merely installing a unit is insufficient.
func VerifyRoutingBootGuard(ctx context.Context) error {
	if runtime.GOOS != "linux" || os.Geteuid() != 0 {
		return errors.New("routing boot verification requires privileged Linux Agent")
	}
	g := routingBootGuard{readUnit: func() ([]byte, error) { return readOwnedSystemUnit(routingBootUnit) }, dependency: checkRoutingBootDependency, run: runDNSService}
	return g.verify(ctx)
}

func checkRoutingBootDependency() error {
	const directory = "/etc/systemd/system/systemd-networkd.service.requires"
	info, err := os.Lstat(directory)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0022 != 0 {
		return errors.New("persistent networkd recovery dependency is absent or unsafe")
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || st.Uid != 0 {
		return errors.New("networkd recovery dependency directory is not root-owned")
	}
	info, err = os.Lstat(directory + "/" + routingBootUnit)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		return errors.New("persistent required recovery unit link is absent")
	}
	st, ok = info.Sys().(*syscall.Stat_t)
	if !ok || st.Uid != 0 {
		return errors.New("networkd recovery dependency link is not root-owned")
	}
	target, err := os.Readlink(directory + "/" + routingBootUnit)
	if err != nil || target != routingBootUnitPath && target != "../"+routingBootUnit {
		return errors.New("networkd recovery dependency points outside the owned unit")
	}
	return nil
}

func (g routingBootGuard) verify(parent context.Context) error {
	ctx, cancel := context.WithTimeout(parent, 15*time.Second)
	defer cancel()
	raw, err := g.readUnit()
	if err != nil || !bytes.Equal(raw, []byte(serviceunit.NetworkRecovery())) {
		return errors.New("boot recovery unit differs from installed build")
	}
	if err := g.dependency(); err != nil {
		return err
	}
	properties := "LoadState,ActiveState,SubState,FragmentPath,DropInPaths,NeedDaemonReload,UnitFileState,ExecMainStatus,User,Group"
	raw, err = g.run(ctx, []string{"--no-pager", "--no-ask-password", "--all", "show", routingBootUnit, "--property=" + properties})
	if err != nil {
		return errors.New("boot recovery service observation unavailable")
	}
	f, err := strictUnitProperties(raw, properties)
	if err != nil {
		return err
	}
	// systemd's oneshot services settle into ActiveState=inactive +
	// SubState=dead once ExecMainStatus reports the terminal code. The
	// brief "active"/"exited" pair only exists in the few milliseconds
	// between the process exiting and the manager marking the unit
	// dead, so requiring exactly that pair is a race that fails
	// verification on every settled boot. A successful terminal exit
	// (ExecMainStatus=0) with either SubState=exited (transient) or
	// SubState=dead (settled) is the correct invariant.
	recoverOk := f["LoadState"] == "loaded" &&
		(f["SubState"] == "exited" || f["SubState"] == "dead") &&
		(f["ActiveState"] == "active" || f["ActiveState"] == "inactive") &&
		f["FragmentPath"] == routingBootUnitPath &&
		f["DropInPaths"] == "" &&
		f["NeedDaemonReload"] == "no" &&
		f["UnitFileState"] == "enabled" &&
		f["ExecMainStatus"] == "0" &&
		f["User"] == "root" &&
		f["Group"] == "root"
	if !recoverOk {
		return errors.New("boot recovery service is not verified for this boot")
	}
	properties = "LoadState,ActiveState,NeedDaemonReload,Requires,After"
	raw, err = g.run(ctx, []string{"--no-pager", "--no-ask-password", "--all", "show", "systemd-networkd.service", "--property=" + properties})
	if err != nil {
		return errors.New("networkd boot ordering observation unavailable")
	}
	f, err = strictUnitProperties(raw, properties)
	if err != nil {
		return err
	}
	has := func(value string) bool {
		for _, name := range strings.Fields(value) {
			if name == routingBootUnit {
				return true
			}
		}
		return false
	}
	// systemd-networkd.service is "inactive" when no interface is
	// currently managed by networkd (e.g. before the first Safe Apply
	// adopts eth1/eth2, or on a host whose only link is the externally
	// managed eth0). The boot ordering invariant under test is the
	// Requires=/After= wiring that places duck-network-recover before
	// networkd, not the runtime activeness of networkd itself. Accept
	// either active or inactive so the guard does not falsely reject a
	// pre-first-Apply host.
	if f["LoadState"] != "loaded" || (f["ActiveState"] != "active" && f["ActiveState"] != "inactive") || f["NeedDaemonReload"] != "no" || !has(f["Requires"]) || !has(f["After"]) {
		return errors.New("networkd lacks the required boot recovery ordering")
	}
	return nil
}

func strictUnitProperties(raw []byte, properties string) (map[string]string, error) {
	if len(raw) > 16<<10 {
		return nil, errors.New("service observation exceeds limit")
	}
	allowed := map[string]bool{}
	for _, key := range strings.Split(properties, ",") {
		allowed[key] = true
	}
	f := map[string]string{}
	for _, line := range strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n") {
		key, value, ok := strings.Cut(line, "=")
		if !ok || !allowed[key] {
			return nil, errors.New("unexpected service observation field")
		}
		if _, duplicate := f[key]; duplicate {
			return nil, errors.New("duplicate service observation field")
		}
		f[key] = value
	}
	if len(f) != len(allowed) {
		return nil, errors.New("incomplete service observation")
	}
	return f, nil
}
