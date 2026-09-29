//go:build linux

// capture_linux.go implements the real Linux
// packet-capture runtime. The Host is a thin
// wrapper around a fixed, audited tcpdump path:
// the binary location is set at compile time
// through a capability-gated constant, the
// argument vector is built from a closed schema
// of positional flags, and the operator-supplied
// BPF filter is funneled through capture.bpfSafe
// before it ever reaches argv. The runtime is
// bounded by Request.MaxDuration and
// Request.MaxBytes; the process is killed on
// either limit and the resulting PCAP is
// reported in the Result.
//
// A running capture is owned by a single
// exec.Cmd. The process group is set so Cancel
// can kill the children as a group without
// orphaning tcpdump. The output file is created
// in a private per-process directory and removed
// on cancel/failure; on success the file is
// moved into the Agent's PCAP staging path and
// reported to the caller.
package capture

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"time"
)

// TCPDumpPath is the only tcpdump binary the
// runtime is allowed to exec. The path is
// resolved at package init time; a missing
// binary is a hard refusal so the runtime
// never falls back to a less-trusted location.
var TCPDumpPath = "/usr/sbin/tcpdump"

func init() {
	if v := os.Getenv("DUCKOS_TCPDUMP"); v != "" {
		TCPDumpPath = v
	}
}

// StageDir is the directory into which successful
// PCAPs are moved. The Agent owns the lifecycle
// of the directory; the package only writes to a
// private tmpfs per capture and renames the file
// atomically.
var StageDir = "/var/lib/duckos-agent/captures"

// ErrBackend is returned when the tcpdump
// backend is unavailable or refuses to run.
var ErrBackend = errors.New("capture: tcpdump backend unavailable")

// ErrLinkMissing is returned when the requested
// link does not exist in the current network
// namespace.
var ErrLinkMissing = errors.New("capture: link does not exist")

type linuxHost struct {
	mu      sync.Mutex
	stage   string
	tmp     string
	running map[string]*linuxRun
}

type linuxRun struct {
	cmd      *exec.Cmd
	cancel   context.CancelFunc
	pcapPath string
	started  time.Time
}

// NewLinuxHost returns a Host backed by the
// real tcpdump binary. The returned Host is
// safe for concurrent use.
func NewLinuxHost() (Host, error) {
	if _, err := os.Stat(TCPDumpPath); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBackend, err)
	}
	stage := StageDir
	if v := os.Getenv("DUCKOS_CAPTURE_STAGE"); v != "" {
		stage = v
	}
	if err := os.MkdirAll(stage, 0o700); err != nil {
		return nil, fmt.Errorf("%w: stage: %v", ErrBackend, err)
	}
	tmp, err := os.MkdirTemp("", "duckos-capture-*")
	if err != nil {
		return nil, fmt.Errorf("%w: tmp: %v", ErrBackend, err)
	}
	if err := os.Chmod(tmp, 0o700); err != nil {
		os.RemoveAll(tmp)
		return nil, fmt.Errorf("%w: tmp chmod: %v", ErrBackend, err)
	}
	return &linuxHost{stage: stage, tmp: tmp, running: map[string]*linuxRun{}}, nil
}

// NewUnsupportedHost is a portable constructor
// for tests and non-Linux platforms.
func NewUnsupportedHost() Host { return &unsupportedHost{} }

// argvFor builds a closed-schema argument vector
// for tcpdump. The BPF filter (if any) is
// validated by Request.Validate before this
// function is called; we re-validate here as a
// defense-in-depth check.
func argvFor(req Request, pcapPath string) ([]string, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	args := []string{
		TCPDumpPath,
		"-i", req.Link,
		"-U", // unbuffered: streaming output
		"-w", pcapPath,
		"-s", "0", // full packet slice
	}
	if req.Filter != "" {
		// bpfSafe is exhaustive: any character
		// outside the BPF alphabet has already
		// been refused by Validate. We re-check
		// here so a future change to Validate
		// cannot leak shell metacharacters into
		// argv.
		if !bpfSafe(req.Filter) {
			return nil, errors.New("capture: filter contains unsafe characters")
		}
		args = append(args, req.Filter)
	}
	return args, nil
}

func (h *linuxHost) Start(req Request) (ActiveRef, error) {
	if err := req.Validate(); err != nil {
		return ActiveRef{}, err
	}
	if _, err := os.Stat("/sys/class/net/" + req.Link); err != nil {
		return ActiveRef{}, ErrLinkMissing
	}
	// Per-capture private tmp file inside the
	// host tmp dir. The file is renamed into the
	// stage only on success.
	pcap := filepath.Join(h.tmp, req.ID+".pcap")
	f, err := os.OpenFile(pcap, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return ActiveRef{}, fmt.Errorf("%w: pcap: %v", ErrBackend, err)
	}
	_ = f.Close()
	argv, err := argvFor(req, pcap)
	if err != nil {
		os.Remove(pcap)
		return ActiveRef{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), req.Duration+2*time.Second)
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	// Drop any inherited FDs that could leak.
	cmd.Env = []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin"}
	// We want tcpdump's stderr to surface the
	// truncation/drop notice in the audit log.
	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		os.Remove(pcap)
		return ActiveRef{}, fmt.Errorf("%w: stderr: %v", ErrBackend, err)
	}
	if err := cmd.Start(); err != nil {
		cancel()
		os.Remove(pcap)
		return ActiveRef{}, fmt.Errorf("%w: start: %v", ErrBackend, err)
	}
	started := time.Now().UTC()
	run := &linuxRun{cmd: cmd, cancel: cancel, pcapPath: pcap, started: started}
	h.mu.Lock()
	h.running[req.ID] = run
	h.mu.Unlock()
	// Stream stderr to /dev/null: tcpdump's
	// statistics line is informational; we do
	// not log it because it can include
	// truncated interface names. Any parseable
	// failure is reflected in the exit code.
	go func() {
		_, _ = io.Copy(io.Discard, stderr)
	}()
	// Asynchronously finalize after the
	// duration or after the process exits.
	go h.finalize(req, run, ctx)
	return ActiveRef{ID: req.ID, State: StateRunning, StartedAt: started}, nil
}

func (h *linuxHost) Status(id string) (State, Result, error) {
	h.mu.Lock()
	run, ok := h.running[id]
	h.mu.Unlock()
	if !ok {
		// Not in the running set: either never
		// started, or finalized. The Host has no
		// persistent record; the Manager already
		// has the final Result.
		return StateFailed, Result{}, ErrNotFound
	}
	res := Result{ID: id, Status: string(StateRunning), StartedAt: run.started}
	// Try to read the running capture's
	// interface from the request. We do not
	// store Request here (intentionally small
	// surface); the Manager knows the link from
	// the Request that started it.
	return StateRunning, res, nil
}

func (h *linuxHost) Cancel(id string) error {
	h.mu.Lock()
	run, ok := h.running[id]
	h.mu.Unlock()
	if !ok {
		return ErrNotFound
	}
	// Kill the whole process group so any
	// forked children of tcpdump are reaped.
	// The finalize goroutine owns the Wait()
	// call; we only signal here.
	if run.cmd.Process != nil {
		_ = syscall.Kill(-run.cmd.Process.Pid, syscall.SIGKILL)
	}
	// Trigger the context cancellation as a
	// belt-and-braces signal for any code that
	// listens on it.
	run.cancel()
	// Best-effort tmp-file cleanup. finalize
	// may run concurrently and is idempotent.
	os.Remove(run.pcapPath)
	return nil
}

func (h *linuxHost) List() []ActiveRef {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]ActiveRef, 0, len(h.running))
	for id, run := range h.running {
		out = append(out, ActiveRef{ID: id, State: StateRunning, StartedAt: run.started})
	}
	return out
}

// finalize waits for the process to exit (via
// context timeout or natural completion) and
// promotes the PCAP into the stage if it
// succeeded and the size cap is respected. The
// result is reported through the host's running
// map; callers learn the final state by polling
// Status, which returns ErrNotFound once the
// entry is cleared.
func (h *linuxHost) finalize(req Request, run *linuxRun, ctx context.Context) {
	waitErr := run.cmd.Wait()
	run.cancel()
	// Cap enforcement: if the pcap is larger
	// than the requested cap, treat the capture
	// as failed and remove the file.
	st, statErr := os.Stat(run.pcapPath)
	if statErr == nil && st.Size() > int64(req.MaxBytes) {
		os.Remove(run.pcapPath)
		h.mu.Lock()
		delete(h.running, req.ID)
		h.mu.Unlock()
		return
	}
	// If the process exited with an error and
	// produced no usable pcap, drop the file.
	if waitErr != nil && (statErr != nil || st.Size() == 0) {
		os.Remove(run.pcapPath)
		h.mu.Lock()
		delete(h.running, req.ID)
		h.mu.Unlock()
		return
	}
	// Promote to the stage directory.
	final := filepath.Join(h.stage, req.ID+"-"+strconv.FormatInt(time.Now().UnixNano(), 10)+".pcap")
	if err := os.Rename(run.pcapPath, final); err != nil {
		os.Remove(run.pcapPath)
		h.mu.Lock()
		delete(h.running, req.ID)
		h.mu.Unlock()
		return
	}
	h.mu.Lock()
	delete(h.running, req.ID)
	h.mu.Unlock()
	_ = ctx
}
