//go:build linux

package xray

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
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
	// resolver is the bounded vault-backed Resolver the
	// runtime calls from Apply to materialize TLS cert /
	// key files and to inline REALITY private key + short
	// ids into the rendered config. A nil resolver means
	// the runtime cannot bring up TLS / REALITY inbounds
	// and Apply will fail them with a typed error. The
	// agent composition root MUST bind a resolver at
	// startup (production: *VaultLifecycle.XrayResolver()).
	resolver Resolver
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

// SetResolver binds a vault-backed Resolver to the runtime.
// The caller MUST call SetResolver BEFORE the first Apply
// when the catalog contains any TLS / REALITY inbounds.
// The resolver is consumed at Apply time only; runtime
// configuration is otherwise unaffected.
func (r *Runtime) SetResolver(res Resolver) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.resolver = res
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

// ApplyWith is the production entry point used by the
// agent host. It renders the catalog to a Config,
// resolves the typed vault references via the bound
// Resolver, and applies the resulting (materialized)
// Config. Apply(ctx, cfg) is the legacy path used by
// tests and by callers that already provide a fully
// materialised Config (security=none).
//
// The host MUST bind a Resolver via SetResolver before
// calling ApplyWith when the catalog has any TLS or
// REALITY inbounds; ApplyWith returns ErrBackend when the
// resolver is missing.
func (r *Runtime) ApplyWith(ctx context.Context, servers []Server, users []User) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.recovery {
		return errors.New("xray: recovery required")
	}
	cfg, err := RenderRuntimeConfig(servers, users)
	if err != nil {
		return ErrConfig
	}
	if needsMaterializationServers(servers) && r.resolver == nil {
		return ErrBackend
	}
	if r.resolver != nil {
		mcfg, err := Materialize(ctx, cfg, r.stage, servers, users, r.resolver)
		if err != nil {
			return err
		}
		cfg = mcfg
	}
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
	// Reject any inbound whose realised render still
	// carries the TLS / REALITY placeholder markers
	// when no resolver is bound. A missing resolver is
	// treated as an explicit, typed failure rather than
	// a silent xray -test error so they cannot be
	// distinguished at the IPC layer.
	if r.resolver == nil && needsMaterialization(raw) {
		return ErrConfig
	}
	candidate := filepath.Join(r.stage, "candidate.json")
	if err = writePrivate(candidate, raw); err != nil {
		return ErrConfig
	}
	defer os.Remove(candidate)
	checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	check := exec.CommandContext(checkCtx, r.binary, "run", "-c", candidate, "-test")
	// DEBUG-LIVE-2026-10-08 round 2: capture xray -test
	// output for live diagnostics. Re-removed after the
	// two-blocker root-cause is closed.
	var checkOut, checkStderr bytes.Buffer
	check.Stdout = &checkOut
	check.Stderr = &checkStderr
	err = check.Run()
	cancel()
	if err != nil {
		log.Printf("xray: configuration is invalid: xray -test failed: stdout=%s stderr=%s", checkOut.String(), checkStderr.String())
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

// needsMaterialization reports whether the rendered
// Config still carries TLS or REALITY placeholder
// markers that require runtime materialization. The
// runtime uses this guard so a missing resolver fails
// fast with a typed ErrConfig rather than letting xray
// -test surface a misleading parser error.
func needsMaterialization(raw []byte) bool {
	return bytesContains(raw, []byte("resolved at apply time")) ||
		bytesContains(raw, []byte("\"security\": \"tls\"")) ||
		bytesContains(raw, []byte("\"security\": \"reality\""))
}

// needsMaterializationServers is the catalog-level
// counterpart: any server whose Security is tls or
// reality forces ApplyWith to require a Resolver.
func needsMaterializationServers(servers []Server) bool {
	for _, s := range servers {
		if s.Security == SecurityTLS || s.Security == SecurityReality {
			return true
		}
	}
	return false
}

func bytesContains(haystack, needle []byte) bool {
	if len(needle) == 0 {
		return true
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if string(haystack[i:i+len(needle)]) == string(needle) {
			return true
		}
	}
	return false
}
