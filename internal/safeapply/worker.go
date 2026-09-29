package safeapply

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/state"
)

// Job is bounded public progress, not a copy of host recovery evidence. The
// request ID is shared with Engine intent; TransactionID becomes that same ID
// in the durable Agent journal once preflight/capture has succeeded. A restarted process
// must report its journal, never pretend that an in-memory request survived.
type Job struct {
	RequestID         string `json:"requestId"`
	TransactionID     string `json:"transactionId,omitempty"`
	CandidateRevision string `json:"candidateRevision,omitempty"`
	Phase             string `json:"phase"`
	Error             string `json:"error,omitempty"`
}
type Worker struct {
	mu         sync.Mutex
	controller *Controller
	job        *Job
	active     bool
	closed     bool
	done       chan struct{}
}

func NewWorker(c *Controller) *Worker { return &Worker{controller: c} }

// RejectAdmission is serialized with SubmitRequest. Its durable tombstone
// prevents a delayed apply with this identity from entering the adapter.
func (w *Worker) RejectAdmission(expected, id string, candidate json.RawMessage, actor, reason string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed || w.active || w.controller == nil {
		return state.ErrConflict
	}
	return w.controller.store.RejectRequest(expected, id, candidate, actor, reason)
}

// Submit never ties a dangerous operation's lifetime to an HTTP connection.
// Only one job may be in flight, and an unresolved durable transaction blocks
// admission. Clients must poll; they must not blindly retry a mutation POST.
func (w *Worker) Submit(expected string, candidate json.RawMessage, actor, reason string, window time.Duration) (Job, error) {
	return w.SubmitRequest(expected, "", candidate, actor, reason, window)
}

func (w *Worker) SubmitRequest(expected, requestID string, candidate json.RawMessage, actor, reason string, window time.Duration) (Job, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed || w.active {
		return Job{}, errors.New("apply worker unavailable or busy")
	}
	if w.controller == nil {
		return Job{}, errors.New("apply controller unavailable")
	}
	if requestID != "" && !state.ValidTransactionID(requestID) {
		return Job{}, errors.New("invalid apply request identity")
	}
	if len(candidate) > state.MaxConfigBytes || !json.Valid(candidate) || len(actor) == 0 || len(actor) > 128 || len(reason) == 0 || len(reason) > 512 || window < 2*time.Minute || window > 15*time.Minute {
		return Job{}, errors.New("invalid apply request")
	}
	snapshot, err := w.controller.store.Snapshot()
	if err != nil {
		return Job{}, err
	}
	if snapshot.Pending != nil {
		return Job{}, state.ErrPending
	}
	if snapshot.Current.ID != expected {
		return Job{}, state.ErrConflict
	}
	if requestID == "" {
		var id [16]byte
		if _, err = rand.Read(id[:]); err != nil {
			return Job{}, err
		}
		requestID = hex.EncodeToString(id[:])
	}
	for _, outcome := range snapshot.Outcomes {
		if outcome.TransactionID == requestID {
			return Job{}, state.ErrConflict
		}
	}
	job := Job{RequestID: requestID, Phase: "preflight"}
	w.job = &job
	w.active = true
	w.done = make(chan struct{})
	candidate = append(json.RawMessage(nil), candidate...)
	go func() {
		tx, err := w.controller.ApplyRequest(expected, requestID, candidate, actor, reason, window)
		w.mu.Lock()
		defer w.mu.Unlock()
		if err != nil {
			fmt.Fprintf(os.Stderr, "safeapply: ApplyRequest failed requestID=%s err=%q\n", requestID, err.Error())
			w.job.Phase = "failed"
			// Store/validator errors may contain private paths or input. Public
			// progress must not expose arbitrary error strings across IPC.
			w.job.Error = "apply did not complete; inspect transaction state before retrying"
		} else {
			w.job.Phase = "awaiting-confirmation"
			w.job.TransactionID = tx.ID
			w.job.CandidateRevision = tx.Candidate.ID
		}
		w.active = false
		close(w.done)
	}()
	return job, nil
}

// Status remains responsive while a bounded Linux adapter is doing work.
// Durable phases take precedence over the provisional in-memory progress.
func (w *Worker) Status() (Job, state.Snapshot, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.controller == nil {
		return Job{}, state.Snapshot{}, errors.New("apply controller unavailable")
	}
	var job Job
	if w.job != nil {
		job = *w.job
	}
	snapshot, err := w.controller.store.Snapshot()
	if err != nil {
		return job, state.Snapshot{}, err
	}
	if snapshot.Pending != nil && (job.TransactionID == "" || job.TransactionID == snapshot.Pending.ID) {
		job.Phase = string(snapshot.Pending.Phase)
		job.TransactionID = snapshot.Pending.ID
	} else if job.Phase == "awaiting-confirmation" {
		// Absence from bounded retention does not prove success or failure.
		job.Phase = "resolved"
		for _, outcome := range snapshot.Outcomes {
			if outcome.TransactionID == job.TransactionID {
				job.Phase = outcome.Result
				break
			}
		}
	}
	if snapshot.Pending != nil {
		snapshot.Pending.Recovery = nil
	}
	return job, snapshot, nil
}

// Close stops admission and waits for the bounded adapter before the caller
// closes the controller/store. Process death instead uses startup recovery.
func (w *Worker) Close() {
	w.mu.Lock()
	w.closed = true
	done := w.done
	active := w.active
	w.mu.Unlock()
	if active {
		<-done
	}
}
