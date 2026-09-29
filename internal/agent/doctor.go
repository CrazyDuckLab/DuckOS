package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/doctor"
)

const DoctorPath = "/agent/v1/doctor"

// DoctorObserver is the engine-facing source of structured diagnostic
// snapshots. The implementation must return a Snapshot that passes
// doctor.Snapshot.Valid(); the Agent refuses to publish a snapshot
// that fails the bound or redaction contract.
type DoctorObserver interface {
	Doctor(context.Context) (doctor.Snapshot, error)
}

// DoctorMaxBytes is the Agent IPC cap for Doctor payloads. It is
// twice the doctor.MaxBytes policy cap so a generous cap, but well
// below the Agent response cap for other observation surfaces.
const DoctorMaxBytes = 1 << 20

// DoctorTimeout is the bound the Agent places on a single Doctor
// call. Sources that run longer than this are abandoned and the
// snapshot is returned with a Subsystem entry that explains the
// timeout.
const DoctorTimeout = 6 * time.Second

// ServeDoctor is the Agent IPC handler for the Doctor surface.
// The handler is bounded, redacted and concurrency-capped: a
// single slow Source cannot poison the rest of the response, and
// the Agent publishes the snapshot only after the implementation
// has confirmed the bound contract.
func (h *Handler) ServeDoctor(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		problem(w, 405, "method_not_allowed")
		return
	}
	if r.URL.RawQuery != "" || r.URL.ForceQuery || r.ContentLength != 0 || len(r.URL.Path) > len(DoctorPath)+16 {
		problem(w, 400, "invalid_request")
		return
	}
	if !strings.HasSuffix(r.URL.Path, DoctorPath) {
		problem(w, 404, "unknown_operation")
		return
	}
	select {
	case h.slots <- struct{}{}:
		defer func() { <-h.slots }()
	default:
		problem(w, 503, "agent_busy")
		return
	}
	h.serveDoctorLocked(w, r)
}

func (h *Handler) serveDoctorLocked(w http.ResponseWriter, r *http.Request) {
	host := h.doctor
	if host == nil {
		if observer, ok := h.observer.(DoctorObserver); ok {
			host = observer
		}
	}
	if host == nil {
		problem(w, 503, "doctor_unavailable")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), DoctorTimeout)
	defer cancel()
	snap, err := host.Doctor(ctx)
	if err != nil {
		problem(w, 503, "doctor_unavailable")
		return
	}
	if err := snap.Valid(); err != nil {
		problem(w, 503, "doctor_invalid")
		return
	}
	data, err := json.Marshal(snap)
	if err != nil || len(data) > DoctorMaxBytes {
		problem(w, 503, "doctor_limit")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// DecodeDoctor is the Engine-side helper that validates an Agent
// Doctor response. The function refuses partial, oversized or
// future-schema payloads before they can reach the API surface.
func DecodeDoctor(data []byte) (doctor.Snapshot, error) {
	if len(data) == 0 {
		return doctor.Snapshot{}, errors.New("doctor: empty payload")
	}
	if len(data) > DoctorMaxBytes {
		return doctor.Snapshot{}, fmt.Errorf("doctor: payload exceeds %d bytes", DoctorMaxBytes)
	}
	var snap doctor.Snapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return doctor.Snapshot{}, fmt.Errorf("doctor: %w", err)
	}
	if err := snap.Valid(); err != nil {
		return doctor.Snapshot{}, fmt.Errorf("doctor: %w", err)
	}
	return snap, nil
}
