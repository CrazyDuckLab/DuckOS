package network

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	serviceunit "github.com/CrazyDuckLab/DuckOS/packaging/systemd"
)

const dnsUnitName = "duck-dnsmasq.service"
const dnsUnitPath = "/etc/systemd/system/duck-dnsmasq.service"

// DNSServiceSnapshot records lifecycle only. Active is not DNS/DHCP readiness
// and enabling boot startup is outside this primitive's authority.
type DNSServiceSnapshot struct {
	Version int  `json:"version"`
	Active  bool `json:"active"`
	Enabled bool `json:"enabled"`
}

type DNSServiceHost struct {
	mu       sync.Mutex
	readUnit func() ([]byte, error)
	run      func(context.Context, []string) ([]byte, error)
}

func NewDNSServiceHost() (*DNSServiceHost, error) {
	if runtime.GOOS != "linux" || os.Geteuid() != 0 {
		return nil, errors.New("DNS service control requires privileged Linux Agent")
	}
	return &DNSServiceHost{readUnit: readDNSServiceUnit, run: runDNSService}, nil
}

func readDNSServiceUnit() ([]byte, error) {
	return readOwnedSystemUnit(dnsUnitName)
}

func readOwnedSystemUnit(name string) ([]byte, error) {
	if name != dnsUnitName && name != routingBootUnit {
		return nil, errors.New("unsupported owned service unit")
	}
	for _, path := range []string{"/etc", "/etc/systemd", "/etc/systemd/system"} {
		info, err := os.Lstat(path)
		if err != nil || !info.IsDir() || info.Mode().Perm()&0022 != 0 {
			return nil, errors.New("unsafe DNS service unit directory")
		}
		st, ok := info.Sys().(*syscall.Stat_t)
		if !ok || st.Uid != 0 {
			return nil, errors.New("DNS service directory is not root-owned")
		}
	}
	root, err := os.OpenRoot("/etc/systemd/system")
	if err != nil {
		return nil, err
	}
	defer root.Close()
	info, err := root.Lstat(name)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0022 != 0 || info.Size() > 32<<10 {
		return nil, errors.New("unsafe DNS service unit")
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || st.Uid != 0 || st.Nlink != 1 {
		return nil, errors.New("unsafe DNS unit ownership or links")
	}
	f, err := root.OpenFile(name, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return nil, errors.New("DNS service unit changed during read")
	}
	raw, err := io.ReadAll(io.LimitReader(f, (32<<10)+1))
	if err != nil || len(raw) > 32<<10 {
		return nil, errors.New("DNS service unit exceeds limit")
	}
	return raw, nil
}

func decodeDNSService(raw []byte) (DNSServiceSnapshot, error) {
	return decodeDNSServiceState(raw, false)
}

// Recovery may stop a failed, process-free unit, but never captures it as a
// stable before-state for a new transaction.
func decodeDNSServiceState(raw []byte, recovery bool) (DNSServiceSnapshot, error) {
	if len(raw) > 16<<10 {
		return DNSServiceSnapshot{}, errors.New("DNS service observation exceeds limit")
	}
	fields := map[string]string{}
	for _, line := range strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n") {
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return DNSServiceSnapshot{}, errors.New("invalid DNS service observation")
		}
		if _, exists := fields[key]; exists {
			return DNSServiceSnapshot{}, errors.New("duplicate DNS service field")
		}
		switch key {
		case "LoadState", "ActiveState", "SubState", "FragmentPath", "DropInPaths", "NeedDaemonReload", "UnitFileState", "MainPID", "User", "Group":
		default:
			return DNSServiceSnapshot{}, errors.New("unexpected DNS service field")
		}
		fields[key] = value
	}
	if len(fields) != 10 || fields["LoadState"] != "loaded" || fields["FragmentPath"] != dnsUnitPath || fields["DropInPaths"] != "" || fields["NeedDaemonReload"] != "no" || fields["User"] != "duck-dnsmasq" || fields["Group"] != "duck-dnsmasq" {
		return DNSServiceSnapshot{}, errors.New("DNS service ownership or loaded unit differs")
	}
	if fields["UnitFileState"] != "disabled" && fields["UnitFileState"] != "enabled" {
		return DNSServiceSnapshot{}, errors.New("unsupported DNS service enablement state")
	}
	pid, err := strconv.ParseUint(fields["MainPID"], 10, 32)
	if err != nil || strconv.FormatUint(pid, 10) != fields["MainPID"] {
		return DNSServiceSnapshot{}, errors.New("invalid DNS service process identity")
	}
	active := fields["ActiveState"] == "active" && fields["SubState"] == "running" && pid > 0
	inactive := fields["ActiveState"] == "inactive" && fields["SubState"] == "dead" && pid == 0
	failed := recovery && fields["ActiveState"] == "failed" && fields["SubState"] == "failed" && pid == 0
	if !active && !inactive && !failed {
		return DNSServiceSnapshot{}, errors.New("DNS service is failed or transitioning")
	}
	return DNSServiceSnapshot{Version: 1, Active: active, Enabled: fields["UnitFileState"] == "enabled"}, nil
}

func (h *DNSServiceHost) capture(ctx context.Context) (DNSServiceSnapshot, error) {
	return h.captureState(ctx, false)
}

func (h *DNSServiceHost) captureState(ctx context.Context, recovery bool) (DNSServiceSnapshot, error) {
	unit, err := h.readUnit()
	if err != nil || !bytes.Equal(unit, []byte(serviceunit.DNSMasq())) {
		return DNSServiceSnapshot{}, errors.New("DNS service unit differs from installed build")
	}
	raw, err := h.run(ctx, []string{"--no-pager", "--no-ask-password", "--all", "show", dnsUnitName, "--property=LoadState,ActiveState,SubState,FragmentPath,DropInPaths,NeedDaemonReload,UnitFileState,MainPID,User,Group"})
	if err != nil {
		return DNSServiceSnapshot{}, errors.New("DNS service inventory unavailable")
	}
	return decodeDNSServiceState(raw, recovery)
}
func (h *DNSServiceHost) Capture(ctx context.Context) (DNSServiceSnapshot, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.capture(ctx)
}

// SetActive never enables, reloads, resets failures or controls another unit.
// Missing startup gates or a failed start are not converted to successful state.
func (h *DNSServiceHost) SetActive(parent context.Context, before DNSServiceSnapshot, active bool) (DNSServiceSnapshot, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if before.Version != 1 {
		return DNSServiceSnapshot{}, errors.New("invalid DNS service before-state")
	}
	ctx, cancel := context.WithTimeout(parent, 25*time.Second)
	defer cancel()
	live, err := h.capture(ctx)
	if err != nil {
		return DNSServiceSnapshot{}, err
	}
	if live != before {
		return DNSServiceSnapshot{}, errors.New("DNS service changed before lifecycle operation")
	}
	if live.Active == active {
		return live, nil
	}
	operation := "stop"
	if active {
		operation = "start"
	}
	if _, err := h.run(ctx, []string{"--no-pager", "--no-ask-password", operation, dnsUnitName}); err != nil {
		return DNSServiceSnapshot{}, errors.New("DNS service outcome uncertain; recovery required")
	}
	after, err := h.capture(ctx)
	if err != nil || after.Active != active || after.Enabled != before.Enabled {
		return DNSServiceSnapshot{}, errors.New("DNS service did not reach requested lifecycle state")
	}
	return after, nil
}

// QuiesceForRecovery stops the owned service before restoring DNS artifacts.
// The caller must hold transaction ownership and supply the durable before-state.
// It neither changes enablement nor clears a failed unit's diagnostic state.
// Once configuration is restored, a separate verified start may be attempted;
// start-rate limits and unresolved failures must not be bypassed automatically.
func (h *DNSServiceHost) QuiesceForRecovery(parent context.Context, before DNSServiceSnapshot) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if before.Version != 1 {
		return errors.New("invalid DNS service recovery before-state")
	}
	ctx, cancel := context.WithTimeout(parent, 25*time.Second)
	defer cancel()
	live, err := h.captureState(ctx, true)
	if err != nil || live.Enabled != before.Enabled {
		return errors.New("DNS service recovery ownership or enablement differs")
	}
	if _, err := h.run(ctx, []string{"--no-pager", "--no-ask-password", "stop", dnsUnitName}); err != nil {
		return errors.New("DNS service recovery stop outcome uncertain")
	}
	after, err := h.captureState(ctx, true)
	if err != nil || after.Active || after.Enabled != before.Enabled {
		return errors.New("DNS service recovery could not verify quiescence")
	}
	return nil
}

func runDNSService(parent context.Context, args []string) ([]byte, error) {
	if runtime.GOOS != "linux" || os.Geteuid() != 0 {
		return nil, errors.New("DNS service operation unavailable")
	}
	ctx, cancel := context.WithTimeout(parent, 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/usr/bin/systemctl", args...)
	cmd.Env = []string{"PATH=/usr/sbin:/usr/bin", "LANG=C", "LC_ALL=C", "SYSTEMD_COLORS=0"}
	cmd.Dir = "/"
	cmd.Stderr = io.Discard
	cmd.WaitDelay = time.Second
	var output dnsServiceOutput
	cmd.Stdout = &output
	if err := cmd.Run(); err != nil {
		return nil, errors.New("DNS service command failed or exceeded limits")
	}
	return output.Bytes(), nil
}

type dnsServiceOutput struct{ bytes.Buffer }

func (b *dnsServiceOutput) Write(p []byte) (int, error) {
	if len(p) > (16<<10)-b.Len() {
		return 0, errors.New("DNS service output exceeds limit")
	}
	return b.Buffer.Write(p)
}
