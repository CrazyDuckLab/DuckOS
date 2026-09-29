// Package capture owns the bounded packet-capture request model
// the V1 contract requires. A Capture is a short-lived
// observation: the operator specifies an interface, a BPF
// filter, a duration and a packet cap, and the Agent returns a
// PCAP file plus a redacted audit record.
//
// The package is split into platform-specific collectors:
// capture_linux.go shells out to a fixed, audited tcpdump path
// with a hard byte and packet cap; capture_other.go returns an
// explicit refusal. The model layer (this file) is portable.
package capture

import (
	"errors"
	"regexp"
	"strings"
	"sync"
	"time"
)

const SchemaVersion = "1"

// Hard caps. The Capture surface is intentionally small: an
// operator can ask for at most a few hundred thousand packets
// over a few minutes, and the resulting PCAP must fit inside a
// 32 MiB envelope.
const (
	MaxDuration     = 5 * time.Minute
	MaxPackets      = 1 << 17 // 131072
	MaxBytes        = 32 << 20
	MaxFilterBytes  = 256
	MaxInterfaceLen = 16
	MaxRequestID    = 64
	MaxConcurrent   = 2
	MinDuration     = 1 * time.Second
)

// Request is the operator-facing input. The Validate method
// enforces the bound contract before any Linux process is started.
type Request struct {
	ID        string        `json:"id"`
	Link      string        `json:"link"`
	Filter    string        `json:"filter,omitempty"`
	Duration  time.Duration `json:"duration"`
	MaxBytes  int           `json:"maxBytes"`
	Reason    string        `json:"reason,omitempty"`
	Operator  string        `json:"operator,omitempty"`
	CreatedAt time.Time     `json:"createdAt"`
}

// Validate rejects any Request that exceeds the policy cap or
// contains characters that could break out of the audited
// argument vector. BPF filters are a real attack surface: a
// shell metacharacter would let a request smuggle commands
// past the fixed tcpdump path.
func (r *Request) Validate() error {
	if r == nil {
		return errors.New("capture: request is nil")
	}
	if !idPattern.MatchString(r.ID) {
		return errors.New("capture: id is not canonical")
	}
	if r.Link == "" || len(r.Link) > MaxInterfaceLen || !linkPattern.MatchString(r.Link) {
		return errors.New("capture: link is not canonical")
	}
	if r.Filter != "" {
		if len(r.Filter) > MaxFilterBytes {
			return errors.New("capture: filter exceeds size cap")
		}
		if !bpfSafe(r.Filter) {
			return errors.New("capture: filter contains unsafe characters")
		}
	}
	if r.Duration < MinDuration || r.Duration > MaxDuration {
		return errors.New("capture: duration out of bounds")
	}
	if r.MaxBytes <= 0 || r.MaxBytes > MaxBytes {
		return errors.New("capture: maxBytes out of bounds")
	}
	if r.Operator != "" && (len(r.Operator) > 64 || !userPattern.MatchString(r.Operator)) {
		return errors.New("capture: operator is not canonical")
	}
	if r.CreatedAt.IsZero() {
		return errors.New("capture: createdAt is required")
	}
	return nil
}

var (
	idPattern   = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,63}$`)
	linkPattern = regexp.MustCompile(`^[a-z][a-z0-9._-]{0,15}$`)
	userPattern = regexp.MustCompile(`^[a-z][a-z0-9._@-]{0,63}$`)
)

// bpfSafe enforces the BPF filter alphabet. tcpdump filters are
// a small subset of ASCII; any character outside that subset
// would be either a bug or an attempt to inject arguments.
func bpfSafe(s string) bool {
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == ' ' || r == '\t':
		case strings.ContainsRune("()[]{}<>.,:+-*/!&=|^~", r):
		default:
			return false
		}
	}
	return true
}

// Result is the Agent's response once a Capture has been
// recorded. The PCAP path is on the local Agent filesystem; the
// API is responsible for streaming it through the loopback
// boundary with a Content-Length equal to Result.Size.
type Result struct {
	ID         string    `json:"id"`
	Link       string    `json:"link"`
	Filter     string    `json:"filter,omitempty"`
	StartedAt  time.Time `json:"startedAt"`
	FinishedAt time.Time `json:"finishedAt"`
	Packets    uint64    `json:"packets"`
	Bytes      int64     `json:"bytes"`
	PCAPPath   string    `json:"pcapPath"`
	Status     string    `json:"status"`
	Error      string    `json:"error,omitempty"`
}

// State is the lifecycle of a Capture: pending, running, done,
// failed. The Agent refuses to start a Capture that is not
// pending and refuses to list more than MaxConcurrent captures.
type State string

const (
	StatePending State = "pending"
	StateRunning State = "running"
	StateDone    State = "done"
	StateFailed  State = "failed"
)

// Capture is the in-memory state a Host keeps for a single
// capture. The Host never serialises a Capture directly; it
// returns the appropriate Result.
type Capture struct {
	Request Request
	State   State
	Result  Result
}

// ActiveRef is what the API returns to a caller that started a
// capture. The Reference can be used to poll status or stream
// the resulting PCAP.
type ActiveRef struct {
	ID        string    `json:"id"`
	State     State     `json:"state"`
	StartedAt time.Time `json:"startedAt,omitempty"`
}

// Host is the contract a platform implementation must satisfy.
// The reference implementation lives in capture_linux.go; the
// non-Linux implementation refuses with an explicit reason.
type Host interface {
	Start(Request) (ActiveRef, error)
	Status(id string) (State, Result, error)
	Cancel(id string) error
	List() []ActiveRef
}

// Errors the Host returns. The API maps each to a stable HTTP
// code; the CLI prints the message.
var (
	ErrBusy        = errors.New("capture: too many concurrent captures")
	ErrNotFound    = errors.New("capture: not found")
	ErrUnsupported = errors.New("capture: not supported on this platform")
)

// Manager owns the in-flight set of Captures and enforces the
// concurrent cap. Manager is goroutine-safe.
type Manager struct {
	mu       sync.Mutex
	host     Host
	active   map[string]Capture
	sequence int
}

// NewManager wires a Host to the Manager. A nil Host produces
// a Manager that returns ErrUnsupported for every operation.
func NewManager(host Host) *Manager {
	if host == nil {
		host = &unsupportedHost{}
	}
	return &Manager{host: host, active: map[string]Capture{}}
}

// Start validates the Request, registers a pending capture and
// delegates to the Host. The call is non-blocking: the Host
// owns the actual lifecycle.
func (m *Manager) Start(req Request) (ActiveRef, error) {
	if err := req.Validate(); err != nil {
		return ActiveRef{}, err
	}
	m.mu.Lock()
	if len(m.active) >= MaxConcurrent {
		m.mu.Unlock()
		return ActiveRef{}, ErrBusy
	}
	now := time.Now().UTC()
	if req.CreatedAt.IsZero() {
		req.CreatedAt = now
	}
	capture := Capture{Request: req, State: StatePending, Result: Result{ID: req.ID, Link: req.Link, Filter: req.Filter, Status: string(StatePending)}}
	m.active[req.ID] = capture
	m.mu.Unlock()
	ref, err := m.host.Start(req)
	if err != nil {
		m.mu.Lock()
		delete(m.active, req.ID)
		m.mu.Unlock()
		return ActiveRef{}, err
	}
	m.mu.Lock()
	capture.State = StateRunning
	capture.Result.StartedAt = ref.StartedAt
	capture.Result.Status = string(StateRunning)
	m.active[req.ID] = capture
	m.mu.Unlock()
	return ref, nil
}

// Status returns the current State and Result of an active
// capture. A capture that is no longer in the active set is
// reported as ErrNotFound; the API and CLI translate that to
// "finished" by consulting the persistent store (out of scope
// for this foundation).
func (m *Manager) Status(id string) (State, Result, error) {
	m.mu.Lock()
	capture, ok := m.active[id]
	m.mu.Unlock()
	if !ok {
		hostState, hostResult, err := m.host.Status(id)
		if err != nil {
			return StateFailed, Result{}, err
		}
		return hostState, hostResult, nil
	}
	return capture.State, capture.Result, nil
}

// Cancel stops an in-flight capture. The Host is the single
// owner of the actual cancellation; the Manager only updates
// the local state.
func (m *Manager) Cancel(id string) error {
	if err := m.host.Cancel(id); err != nil {
		return err
	}
	m.mu.Lock()
	if capture, ok := m.active[id]; ok {
		capture.State = StateFailed
		capture.Result.Status = string(StateFailed)
		capture.Result.Error = "cancelled"
		capture.Result.FinishedAt = time.Now().UTC()
		delete(m.active, id)
	}
	m.mu.Unlock()
	return nil
}

// List returns the ActiveRef of every in-flight capture. The
// returned slice is a copy.
func (m *Manager) List() []ActiveRef {
	m.mu.Lock()
	out := make([]ActiveRef, 0, len(m.active))
	for _, c := range m.active {
		out = append(out, ActiveRef{ID: c.Request.ID, State: c.State, StartedAt: c.Result.StartedAt})
	}
	m.mu.Unlock()
	return out
}

type unsupportedHost struct{}

func (unsupportedHost) Start(Request) (ActiveRef, error) { return ActiveRef{}, ErrUnsupported }
func (unsupportedHost) Status(string) (State, Result, error) {
	return StateFailed, Result{}, ErrUnsupported
}
func (unsupportedHost) Cancel(string) error { return ErrUnsupported }
func (unsupportedHost) List() []ActiveRef   { return nil }
