package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/network"
	"github.com/CrazyDuckLab/DuckOS/internal/safeapply"
	"github.com/CrazyDuckLab/DuckOS/internal/state"
)

const NetworkPrefix = "/agent/v1/network/"

type NetworkStatus struct {
	Version  int                `json:"version"`
	Current  state.Revision     `json:"current"`
	Pending  *state.Transaction `json:"pending"`
	Outcomes []state.Outcome    `json:"outcomes"`
	Job      safeapply.Job      `json:"job"`
	// Startup is the second-stage cold-boot state. "pending" means the Agent
	// is still capturing live kernel state; "ready" means the live state
	// matches the durable intent and the service is admitting work;
	// "recovery_required" means the live state diverged and an operator
	// must intervene. The status field never silently swaps to "ready"
	// without a successful kernel capture/compare cycle.
	Startup  string `json:"startup"`
	StartErr string `json:"startupError,omitempty"`
}

type NetworkApply struct {
	RequestID        string          `json:"requestId"`
	ExpectedRevision string          `json:"expectedRevision"`
	Candidate        json.RawMessage `json:"candidate"`
	Actor            string          `json:"actor"`
	Reason           string          `json:"reason"`
	ConfirmSeconds   int             `json:"confirmSeconds"`
	CreatedAt        time.Time       `json:"createdAt"`
}

// startupState is a tiny atomic state carrier for the second-stage gate so
// that status reads never compete with the rest of the request-handling
// mutex. The state is set once during StartupVerify and only ever advances
// to "ready" or "recovery_required".
type startupState struct {
	value atomic.Pointer[startupValue]
}

type startupValue struct {
	state string
	err   string
}

func newStartupState() *startupState {
	s := &startupState{}
	v := &startupValue{state: "pending"}
	s.value.Store(v)
	return s
}

func (s *startupState) State() string { return s.value.Load().state }
func (s *startupState) Error() string { return s.value.Load().err }
func (s *startupState) set(state, errMsg string) {
	s.value.Store(&startupValue{state: state, err: errMsg})
}

// NetworkService is explicitly installed only after Agent ownership enrollment
// and startup recovery succeed. It does not create a store or choose an adapter.
// The API process must never construct or supply the privileged adapter.
type NetworkService struct {
	mu          sync.Mutex
	closed      bool
	controlBusy bool
	controls    sync.WaitGroup
	controller  *safeapply.Controller
	worker      *safeapply.Worker
	slots       chan struct{}
	startup     *startupState
}

func NewNetworkService(controller *safeapply.Controller) (*NetworkService, error) {
	if controller == nil {
		return nil, errors.New("network controller required")
	}
	return &NetworkService{
		controller: controller,
		worker:     safeapply.NewWorker(controller),
		slots:      make(chan struct{}, 2),
		startup:    newStartupState(),
	}, nil
}

// EnableNetwork is startup-only; the caller closes the service before its store.
func (h *Handler) EnableNetwork(service *NetworkService) { h.network = service }

// StartupVerify is the second stage of the cold-boot model. It must run after
// the pre-networkd oneshot (RecoverRoutingArtifacts) has restored files and
// after networkd has started, but before the service admits any apply. The
// Agent captures the live private route tables and policy rules, compares
// them against the journaled "before" intent, and refuses to admit work if
// they diverge. A non-nil kernel is required; the call is the only path that
// can transition the service from "pending" to "ready" or
// "recovery_required".
func (s *NetworkService) StartupVerify(ctx context.Context, kernel network.BootKernel, enrollment network.Enrollment) error {
	if kernel == nil {
		return errors.New("startup verify requires an enrolled host kernel")
	}
	if s.startup.State() == "ready" {
		return nil
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return errors.New("network service is closed")
	}
	s.mu.Unlock()
	snap, err := s.controller.Status()
	if err != nil {
		s.startup.set("recovery_required", err.Error())
		return err
	}
	before, err := network.Decode(snap.Current.Config)
	if err != nil {
		s.startup.set("recovery_required", err.Error())
		return err
	}
	if err := network.ValidateRuntimeOwnership(ctx, kernel, before, enrollment); err != nil {
		s.startup.set("recovery_required", err.Error())
		return err
	}
	s.startup.set("ready", "")
	return nil
}

func (s *NetworkService) Close() error {
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
	s.controls.Wait()
	s.worker.Close()
	return s.controller.Close()
}

// MarkRecovery is the public entry point used by the wiring layer when an
// out-of-band check (boot guard, management verifier, etc.) refuses the
// service. It transitions the service to recovery_required without going
// through the boot-kernel gate. The call is idempotent and never moves
// the state backwards.
func (s *NetworkService) MarkRecovery(err error) {
	if err == nil {
		return
	}
	s.startup.set("recovery_required", err.Error())
}

func (s *NetworkService) status() (NetworkStatus, error) {
	job, snap, err := s.worker.Status()
	if err != nil {
		fmt.Fprintf(os.Stderr, "agent: status worker.Status err=%v\n", err)
		return NetworkStatus{}, err
	}
	return NetworkStatus{
		Version:  1,
		Current:  snap.Current,
		Pending:  snap.Pending,
		Outcomes: snap.Outcomes,
		Job:      job,
		Startup:  s.startup.State(),
		StartErr: s.startup.Error(),
	}, nil
}

func (h *Handler) serveNetwork(w http.ResponseWriter, r *http.Request) {
	if h.network == nil {
		problem(w, 503, "network_not_enrolled")
		return
	}
	if r.URL.RawPath != "" || r.URL.RawQuery != "" || r.URL.ForceQuery {
		problem(w, 400, "invalid_request")
		return
	}
	path := r.URL.Path
	method := http.MethodPost
	if path == NetworkPrefix+"status" {
		method = http.MethodGet
	}
	if path != NetworkPrefix+"status" && path != NetworkPrefix+"apply" && path != NetworkPrefix+"cancel-admission" && path != NetworkPrefix+"confirm" && path != NetworkPrefix+"rollback" {
		problem(w, 404, "unknown_operation")
		return
	}
	if r.Method != method {
		w.Header().Set("Allow", method)
		problem(w, 405, "method_not_allowed")
		return
	}
	s := h.network
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	default:
		problem(w, 503, "network_busy")
		return
	}
	if method == http.MethodGet {
		if r.ContentLength != 0 || len(r.TransferEncoding) != 0 {
			problem(w, 400, "invalid_request")
			return
		}
		status, err := s.status()
		if err != nil {
			problem(w, 503, "network_recovery_required")
			return
		}
		networkJSON(w, 200, status)
		return
	}
	if r.Header.Get("Content-Type") != "application/json" || r.Header.Get("Content-Encoding") != "" {
		problem(w, 415, "invalid_content_type")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, state.MaxConfigBytes+4096)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if path == NetworkPrefix+"apply" || path == NetworkPrefix+"cancel-admission" {
		var input NetworkApply
		if d.Decode(&input) != nil || d.Decode(new(any)) != io.EOF || !state.ValidTransactionID(input.RequestID) || input.ConfirmSeconds < 120 || input.ConfirmSeconds > 900 {
			problem(w, 400, "invalid_network_request")
			return
		}
		age := time.Since(input.CreatedAt)
		if input.CreatedAt.IsZero() || age < -5*time.Second || (path == NetworkPrefix+"apply" && age > time.Minute) {
			problem(w, 409, "network_request_expired")
			return
		}
		if _, err := network.Decode(input.Candidate); err != nil {
			problem(w, 422, "invalid_network_configuration")
			return
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.closed || s.controlBusy {
			problem(w, 503, "network_busy")
			return
		}
		if s.startup.State() != "ready" {
			problem(w, 503, "network_startup_incomplete")
			return
		}
		if path == NetworkPrefix+"cancel-admission" {
			if err := s.worker.RejectAdmission(input.ExpectedRevision, input.RequestID, input.Candidate, input.Actor, input.Reason); err != nil {
				problem(w, 409, "network_admission_unresolved")
				return
			}
			networkJSON(w, 200, map[string]string{"transactionId": input.RequestID, "status": "rejected"})
			return
		}
		job, err := s.worker.SubmitRequest(input.ExpectedRevision, input.RequestID, input.Candidate, input.Actor, input.Reason, time.Duration(input.ConfirmSeconds)*time.Second)
		if err != nil {
			problem(w, 409, "network_admission_conflict")
			return
		}
		networkJSON(w, 202, job)
		return
	}
	var input struct {
		TransactionID string `json:"transactionId"`
	}
	if d.Decode(&input) != nil || d.Decode(new(any)) != io.EOF || !state.ValidTransactionID(input.TransactionID) {
		problem(w, 400, "invalid_network_request")
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.controlBusy {
		problem(w, 503, "network_busy")
		return
	}
	if s.startup.State() != "ready" {
		problem(w, 503, "network_startup_incomplete")
		return
	}
	status, err := s.status()
	if err != nil {
		problem(w, 503, "network_recovery_required")
		return
	}
	if status.Pending == nil || status.Pending.ID != input.TransactionID {
		problem(w, 409, "network_transaction_conflict")
		return
	}
	if path == NetworkPrefix+"confirm" && status.Pending.Phase != state.AwaitingConfirmation {
		problem(w, 409, "network_not_confirmable")
		return
	}
	s.controlBusy = true
	s.controls.Add(1)
	go func() {
		defer s.controls.Done()
		if path == NetworkPrefix+"confirm" {
			_ = s.controller.Confirm(input.TransactionID)
		} else {
			_ = s.controller.Rollback(input.TransactionID)
		}
		s.mu.Lock()
		s.controlBusy = false
		s.mu.Unlock()
	}()
	networkJSON(w, 202, map[string]string{"transactionId": input.TransactionID, "status": "admitted"})
}

func networkJSON(w http.ResponseWriter, code int, value any) {
	data, err := json.Marshal(value)
	if err != nil || len(data) > 1<<20 {
		problem(w, 503, "network_response_limit")
		return
	}
	w.WriteHeader(code)
	_, _ = w.Write(data)
}
