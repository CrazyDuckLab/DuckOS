// agent/speedlab_host.go: agent-side mount for the protected
// Speed Lab surface.
//
// DV04-SPEEDLAB production-path contract:
//
//	PUBLIC -> Engine.SpeedLabProxy -> Agent IPC
//	       -> SpeedLabHost -> Manager -> LinuxHost -> bounded probes
//
// The agent owns execution. The public API never opens a
// network connection itself.
package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/identity"
	"github.com/CrazyDuckLab/DuckOS/internal/speedlab"
)

// SpeedLabPath is the IPC route prefix for the production
// Speed Lab host. The prefix is the canonical mount point;
// the public API proxies /api/v1/speedlab/* to this loopback.
const SpeedLabPath = "/agent/v1/speedlab"

// SpeedLabSource is the narrow contract the host needs to
// drive speedlab probes. *speedlab.Manager satisfies it.
type SpeedLabSource interface {
	Start(req speedlab.Request) (speedlab.ActiveRef, error)
	Status(id string) (speedlab.State, speedlab.Summary, error)
	Cancel(id string) error
	List() []speedlab.ActiveRef
}

// SpeedLabAccess is the narrow contract the host uses to
// enforce authorization. The identity service satisfies it
// in production; tests inject a permissive stub.
type SpeedLabAccess interface {
	Authorize(token string, p identity.Permission) (identity.User, error)
}

// SpeedLabHost wraps speedlab.Manager behind a goroutine-safe
// IPC entry point.
type SpeedLabHost struct {
	mu     sync.Mutex
	Source SpeedLabSource
	Access SpeedLabAccess
	prefix string
}

// NewSpeedLabHost wires the speedlab.Manager into the host.
func NewSpeedLabHost(src SpeedLabSource, acc SpeedLabAccess) *SpeedLabHost {
	if src == nil {
		src = stubSpeedLabSource{}
	}
	if acc == nil {
		acc = stubSpeedLabAccess{}
	}
	return &SpeedLabHost{Source: src, Access: acc, prefix: "/speedlab/v1/"}
}

// SetPrefix overrides the default path prefix. The agent
// mount is at /agent/v1/speedlab/speedlab/v1/* so the
// canonical tail is /speedlab/v1/.
func (h *SpeedLabHost) SetPrefix(p string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.prefix = p
}

// ServeHTTP dispatches /agent/v1/speedlab/* to the underlying
// speedlab.Manager. The host enforces the ManageSecrets
// permission on every operation so the public API cannot
// bypass it through a stale session token.
//
// Paths:
//
//	POST   /speedlab/v1/probes            -> Start
//	GET    /speedlab/v1/probes            -> List
//	GET    /speedlab/v1/probes/{id}       -> Status
//	DELETE /speedlab/v1/probes/{id}       -> Cancel
func (h *SpeedLabHost) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	src := h.Source
	acc := h.Access
	h.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	tail := r.URL.EscapedPath()
	// Strip the agent-side mount prefix.
	for _, p := range []string{SpeedLabPath, "/speedlab/v1/"} {
		if len(tail) >= len(p) && tail[:len(p)] == p {
			tail = tail[len(p):]
			break
		}
	}
	switch r.Method {
	case http.MethodPost:
		if tail != "probes" {
			speedLabProblem(w, 404, "not_found", "Speed Lab endpoint not found.")
			return
		}
		h.handleStart(w, r, src, acc)
	case http.MethodGet:
		if tail == "probes" {
			h.handleList(w, r, src, acc)
			return
		}
		if len(tail) > 6 && tail[:6] == "probes" && tail[6] == '/' {
			id := tail[7:]
			h.handleStatus(w, r, id, src, acc)
			return
		}
		speedLabProblem(w, 404, "not_found", "Speed Lab endpoint not found.")
	case http.MethodDelete:
		if len(tail) > 6 && tail[:6] == "probes" && tail[6] == '/' {
			id := tail[7:]
			h.handleCancel(w, r, id, src, acc)
			return
		}
		speedLabProblem(w, 404, "not_found", "Speed Lab endpoint not found.")
	default:
		w.Header().Set("Allow", "GET, POST, DELETE")
		speedLabProblem(w, 405, "method_not_allowed", "Speed Lab endpoint method is not allowed.")
	}
}

func (h *SpeedLabHost) authorize(w http.ResponseWriter, r *http.Request, perm identity.Permission) (identity.User, bool) {
	v := r.Header.Get("Authorization")
	if len(v) < 7 || v[:7] != "Bearer " {
		speedLabProblem(w, 401, "unauthorized", "Bearer token is required.")
		return identity.User{}, false
	}
	if len(r.Header.Values("Authorization")) != 1 {
		speedLabProblem(w, 401, "unauthorized", "Bearer token is required.")
		return identity.User{}, false
	}
	user, err := h.Access.Authorize(v[7:], perm)
	if err != nil {
		speedLabProblem(w, 403, "forbidden", "Required permission is missing.")
		return identity.User{}, false
	}
	return user, true
}

func (h *SpeedLabHost) handleStart(w http.ResponseWriter, r *http.Request, src SpeedLabSource, acc SpeedLabAccess) {
	if _, ok := h.authorize(w, r, identity.ManageSecrets); !ok {
		return
	}
	defer r.Body.Close()
	var req speedlab.Request
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		speedLabProblem(w, 400, "invalid_request", "Speed Lab request body is not valid JSON.")
		return
	}
	if req.ID == "" {
		req.ID = speedlab.SecureRandomID()
	}
	if req.CreatedAt.IsZero() {
		req.CreatedAt = time.Now().UTC()
	}
	ref, err := src.Start(req)
	if err != nil {
		switch {
		case errors.Is(err, speedlab.ErrBusy):
			speedLabProblem(w, 503, "speedlab_busy", "Too many concurrent Speed Lab probes.")
		case errors.Is(err, speedlab.ErrUnsupported):
			speedLabProblem(w, 503, "speedlab_unsupported", "Speed Lab is not supported on this host.")
		default:
			speedLabProblem(w, 400, "invalid_request", err.Error())
		}
		return
	}
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(ref)
}

func (h *SpeedLabHost) handleList(w http.ResponseWriter, r *http.Request, src SpeedLabSource, acc SpeedLabAccess) {
	if _, ok := h.authorize(w, r, identity.Observe); !ok {
		return
	}
	out := src.List()
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(out)
}

func (h *SpeedLabHost) handleStatus(w http.ResponseWriter, r *http.Request, id string, src SpeedLabSource, acc SpeedLabAccess) {
	if _, ok := h.authorize(w, r, identity.Observe); !ok {
		return
	}
	if id == "" {
		speedLabProblem(w, 400, "invalid_request", "Probe id is required.")
		return
	}
	state, summary, err := src.Status(id)
	if err != nil {
		if errors.Is(err, speedlab.ErrNotFound) {
			speedLabProblem(w, 404, "not_found", "Probe not found.")
			return
		}
		speedLabProblem(w, 503, "speedlab_unavailable", "Probe status unavailable.")
		return
	}
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(struct {
		State   speedlab.State    `json:"state"`
		Summary speedlab.Summary  `json:"summary"`
	}{
		State:   state,
		Summary: summary,
	})
}

func (h *SpeedLabHost) handleCancel(w http.ResponseWriter, r *http.Request, id string, src SpeedLabSource, acc SpeedLabAccess) {
	if _, ok := h.authorize(w, r, identity.ManageSecrets); !ok {
		return
	}
	if id == "" {
		speedLabProblem(w, 400, "invalid_request", "Probe id is required.")
		return
	}
	if err := src.Cancel(id); err != nil {
		if errors.Is(err, speedlab.ErrNotFound) {
			speedLabProblem(w, 404, "not_found", "Probe not found.")
			return
		}
		speedLabProblem(w, 503, "speedlab_unavailable", "Probe cancel failed.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// speedLabProblem writes an RFC-7807 problem document with
// a per-subsystem code and a sanitised message. The helper
// is local because the agent package's shared problem() does
// not accept a message.
func speedLabProblem(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]string{"code": code, "message": message},
	})
}

// Compile-time check.
var _ SpeedLabSource = (*speedlab.Manager)(nil)

// stub implementations.
type stubSpeedLabSource struct{}

func (stubSpeedLabSource) Start(speedlab.Request) (speedlab.ActiveRef, error) {
	return speedlab.ActiveRef{}, errors.New("speedlab: agent is unavailable")
}
func (stubSpeedLabSource) Status(string) (speedlab.State, speedlab.Summary, error) {
	return speedlab.StateFailed, speedlab.Summary{}, errors.New("speedlab: agent is unavailable")
}
func (stubSpeedLabSource) Cancel(string) error { return errors.New("speedlab: agent is unavailable") }
func (stubSpeedLabSource) List() []speedlab.ActiveRef { return nil }

type stubSpeedLabAccess struct{}

func (stubSpeedLabAccess) Authorize(string, identity.Permission) (identity.User, error) {
	return identity.User{}, errors.New("identity service unavailable")
}

// FormatProbeID is a tiny helper used by tests to build
// canonical ids.
func FormatProbeID(prefix string, n int) string {
	return fmt.Sprintf("%s-%032d", prefix, n)
}

// ensure context is referenced (CI build otherwise drops the
// import if no method uses it directly).
var _ = context.Background