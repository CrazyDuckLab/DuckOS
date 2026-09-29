//go:build linux

package xray

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

var XrayPath = "/usr/local/bin/xray"
var StageDir = "/var/lib/duckos-agent/xray"
var ErrBackend = errors.New("xray: backend unavailable")
var ErrConfig = errors.New("xray: configuration is invalid")
var ErrActive = errors.New("xray: no active process")

func init() {
	if v := os.Getenv("DUCKOS_XRAY"); v != "" {
		XrayPath = v
	}
}

// Runtime serializes native validation, process replacement and recovery.
// Xray has no supported SIGUSR1 configuration-reload contract: a replacement
// is successful only after the new process owns its expected listener sockets.
type Runtime struct {
	mu                    sync.Mutex
	stage, binary, active string
	cmd                   *exec.Cmd
	done                  chan struct{}
	recovery              bool
}

func NewRuntime() (*Runtime, error) {
	if _, err := os.Stat(XrayPath); err != nil {
		return nil, ErrBackend
	}
	stage := StageDir
	if v := os.Getenv("DUCKOS_XRAY_STAGE"); v != "" {
		stage = v
	}
	if err := os.MkdirAll(stage, 0700); err != nil {
		return nil, ErrBackend
	}
	return &Runtime{stage: stage, binary: XrayPath}, nil
}

type State struct {
	Active           bool
	PID              int
	Started          time.Time
	LastPath         string
	RecoveryRequired bool
}

func (r *Runtime) aliveLocked() bool {
	if r.cmd == nil {
		return false
	}
	select {
	case <-r.done:
		return false
	default:
		return true
	}
}
func (r *Runtime) State() State {
	r.mu.Lock()
	defer r.mu.Unlock()
	s := State{Active: r.aliveLocked(), LastPath: r.active, RecoveryRequired: r.recovery}
	if s.Active {
		s.PID = r.cmd.Process.Pid
	}
	return s
}

func (r *Runtime) Apply(ctx context.Context, cfg Config) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.applyLocked(ctx, cfg)
}
func (r *Runtime) applyLocked(ctx context.Context, cfg Config) error {
	if r.recovery {
		return errors.New("xray: recovery required")
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		return ErrConfig
	}
	candidate := filepath.Join(r.stage, "candidate.json")
	if err = writePrivate(candidate, raw); err != nil {
		return ErrConfig
	}
	defer os.Remove(candidate)
	checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	check := exec.CommandContext(checkCtx, r.binary, "run", "-c", candidate, "-test")
	// Native parser output may include credentials: never return or log it.
	err = check.Run()
	cancel()
	if err != nil {
		return ErrConfig
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	active := filepath.Join(r.stage, "active.json")
	previous, readErr := os.ReadFile(active)
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		return ErrConfig
	}
	if err = r.stopLocked(); err != nil {
		r.recovery = true
		return err
	}
	if len(previous) > 0 {
		if err = writePrivate(filepath.Join(r.stage, "previous.json"), previous); err != nil {
			return r.recoverLocked(previous, ErrConfig)
		}
	}
	if err = writePrivate(active, raw); err == nil {
		err = r.startLocked(ctx, cfg, active)
	}
	if err != nil {
		return r.recoverLocked(previous, err)
	}
	r.active = active
	return nil
}

// Recovery is independent of a cancelled API request. Failure leaves an
// explicit recovery-required state, never a successful desired-state claim.
func (r *Runtime) recoverLocked(previous []byte, cause error) error {
	if err := r.stopLocked(); err != nil {
		r.recovery = true
		return errors.New("xray: recovery required; process stop failed")
	}
	active := filepath.Join(r.stage, "active.json")
	if len(previous) == 0 {
		if err := os.Remove(active); err != nil && !errors.Is(err, os.ErrNotExist) {
			r.recovery = true
		}
		r.active = ""
		return cause
	}
	var cfg Config
	if json.Unmarshal(previous, &cfg) != nil || writePrivate(active, previous) != nil {
		r.recovery = true
		return errors.New("xray: recovery required; previous configuration unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := r.startLocked(ctx, cfg, active); err != nil {
		r.recovery = true
		return errors.New("xray: recovery required; previous runtime failed")
	}
	r.active = active
	return cause
}

func (r *Runtime) startLocked(ctx context.Context, cfg Config, path string) error {
	cmd := exec.Command(r.binary, "run", "-c", path)
	cmd.Env = []string{"PATH=/usr/local/bin:/usr/bin:/bin"}
	// A crashed Agent must not leave an untracked, credential-bearing daemon.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pdeathsig: syscall.SIGKILL}
	if err := cmd.Start(); err != nil {
		return ErrBackend
	}
	done := make(chan struct{})
	r.cmd = cmd
	r.done = done
	go func() { _ = cmd.Wait(); close(done) }()
	timeout := time.NewTimer(3 * time.Second)
	defer timeout.Stop()
	tick := time.NewTicker(20 * time.Millisecond)
	defer tick.Stop()
	stable := time.Now().Add(150 * time.Millisecond)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-done:
			return ErrBackend
		case <-timeout.C:
			return errors.New("xray: listener readiness failed")
		case <-tick.C:
			if time.Now().After(stable) && ownsListeners(cmd.Process.Pid, cfg.Inbounds) {
				select {
				case <-done:
					return ErrBackend
				default:
					return nil
				}
			}
		}
	}
}

// Inspect only sockets owned by this child, not an unrelated process that
// happens to listen on a configured port. All supported transports use TCP.
func ownsListeners(pid int, inbounds []Inbound) bool {
	if len(inbounds) == 0 {
		return true
	}
	root := fmt.Sprintf("/proc/%d", pid)
	entries, err := os.ReadDir(filepath.Join(root, "fd"))
	if err != nil {
		return false
	}
	sockets := map[string]bool{}
	for _, entry := range entries {
		target, err := os.Readlink(filepath.Join(root, "fd", entry.Name()))
		if err == nil && strings.HasPrefix(target, "socket:[") {
			sockets[strings.TrimSuffix(strings.TrimPrefix(target, "socket:["), "]")] = true
		}
	}
	ports := map[int]bool{}
	for _, name := range []string{"tcp", "tcp6"} {
		raw, err := os.ReadFile(filepath.Join(root, "net", name))
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(raw), "\n") {
			fields := strings.Fields(line)
			if len(fields) < 10 || fields[3] != "0A" || !sockets[fields[9]] {
				continue
			}
			addr := strings.Split(fields[1], ":")
			if len(addr) != 2 {
				continue
			}
			port, err := strconv.ParseInt(addr[1], 16, 32)
			if err == nil {
				ports[int(port)] = true
			}
		}
	}
	for _, in := range inbounds {
		if !ports[in.Port] {
			return false
		}
	}
	return true
}

func (r *Runtime) stopLocked() error {
	if r.cmd == nil {
		return nil
	}
	if r.aliveLocked() {
		if err := syscall.Kill(-r.cmd.Process.Pid, syscall.SIGTERM); err != nil && !errors.Is(err, syscall.ESRCH) {
			return ErrBackend
		}
		select {
		case <-r.done:
		case <-time.After(2 * time.Second):
			if err := syscall.Kill(-r.cmd.Process.Pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
				return ErrBackend
			}
			select {
			case <-r.done:
			case <-time.After(time.Second):
				return ErrBackend
			}
		}
	}
	r.cmd = nil
	r.done = nil
	return nil
}
func (r *Runtime) Stop() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.stopLocked()
}
func (r *Runtime) Rollback() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	path := filepath.Join(r.stage, "previous.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		return ErrConfig
	}
	var cfg Config
	if err = json.Unmarshal(raw, &cfg); err != nil {
		return ErrConfig
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err = r.applyLocked(ctx, cfg); err != nil {
		return err
	}
	return os.Remove(path)
}

func writePrivate(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".xray-")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if err = f.Chmod(0600); err != nil {
		return err
	}
	if _, err = f.Write(data); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = os.Rename(f.Name(), path); err != nil {
		return err
	}
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}
