// agent/wake_host.go: agent-side mount for the bounded
// Wake-on-LAN surface.
//
// P065 production-path contract:
//
//	PUBLIC -> Engine.WakeProxy -> Agent IPC
//	       -> WakeHost -> Manager -> standard WOL magic packet
//
// The host only ever reports PACKET_SENT (never
// DEVICE_ONLINE) so the public UI cannot mislead the
// operator into believing a device actually woke.
package agent

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"sync"

	"github.com/CrazyDuckLab/DuckOS/internal/identity"
	"github.com/CrazyDuckLab/DuckOS/internal/wake"
)

// WakePath is the IPC route prefix for the production
// Wake host.
const WakePath = "/agent/v1/wake"

// WakeSource is the narrow contract the host needs.
// *wake.Manager satisfies it.
type WakeSource interface {
	Send(o wake.SendOpts) (wake.Result, error)
}

// WakeAccess is the narrow contract the host uses to
// enforce authorization.
type WakeAccess interface {
	Authorize(token string, p identity.Permission) (identity.User, error)
}

// WakeHost wraps wake.Manager behind a goroutine-safe IPC.
type WakeHost struct {
	mu     sync.Mutex
	Source WakeSource
	Access WakeAccess
}

// NewWakeHost wires the wake.Manager into the host.
func NewWakeHost(src WakeSource, acc WakeAccess) *WakeHost {
	if src == nil {
		src = stubWakeSource{}
	}
	if acc == nil {
		acc = stubWakeAccess{}
	}
	return &WakeHost{Source: src, Access: acc}
}

// ServeHTTP dispatches /agent/v1/wake/* to the underlying
// wake.Manager. Wake requires ManageServices (it's an
// outbound mutation against the LAN).
//
// Paths:
//
//	POST /wake/v1/send    -> Send
func (h *WakeHost) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	src := h.Source
	acc := h.Access
	h.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	tail := r.URL.EscapedPath()
	for _, p := range []string{WakePath, "/wake/v1/"} {
		if len(tail) >= len(p) && tail[:len(p)] == p {
			tail = tail[len(p):]
			break
		}
	}
	if r.Method == http.MethodPost && tail == "send" {
		h.handleSend(w, r, src, acc)
		return
	}
	w.Header().Set("Allow", "POST")
	wakeProblem(w, 404, "not_found", "Wake endpoint not found.")
}

func (h *WakeHost) authorize(w http.ResponseWriter, r *http.Request, perm identity.Permission) (identity.User, bool) {
	v := r.Header.Get("Authorization")
	if len(v) < 7 || v[:7] != "Bearer " {
		wakeProblem(w, 401, "unauthorized", "Bearer token is required.")
		return identity.User{}, false
	}
	if len(r.Header.Values("Authorization")) != 1 {
		wakeProblem(w, 401, "unauthorized", "Bearer token is required.")
		return identity.User{}, false
	}
	user, err := h.Access.Authorize(v[7:], perm)
	if err != nil {
		wakeProblem(w, 403, "forbidden", "Required permission is missing.")
		return identity.User{}, false
	}
	return user, true
}

func (h *WakeHost) handleSend(w http.ResponseWriter, r *http.Request, src WakeSource, acc WakeAccess) {
	if _, ok := h.authorize(w, r, identity.ConfigureNetwork); !ok {
		return
	}
	defer r.Body.Close()
	var body struct {
		Target    string `json:"target"`
		Broadcast string `json:"broadcast"`
		Port      int    `json:"port"`
		Packets   int    `json:"packets"`
		Source    string `json:"source"`
		Reason    string `json:"reason"`
	}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil {
		wakeProblem(w, 400, "invalid_request", "Wake body is not valid JSON.")
		return
	}
	opts := wake.SendOpts{
		Target:  body.Target,
		Source:  body.Source,
		Reason:  body.Reason,
		Port:    body.Port,
		Packets: body.Packets,
	}
	if body.Broadcast != "" {
		addr, err := net.ResolveUDPAddr("udp", body.Broadcast)
		if err != nil {
			wakeProblem(w, 400, "invalid_request", "Wake broadcast is not a valid UDP address.")
			return
		}
		opts.Broadcast = addr
	}
	res, err := src.Send(opts)
	if err != nil {
		wakeProblem(w, 400, "invalid_request", err.Error())
		return
	}
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(res)
}

// wakeProblem writes an RFC-7807 problem document.
func wakeProblem(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]string{"code": code, "message": message},
	})
}

// Compile-time check.
var _ WakeSource = (*wake.Manager)(nil)

type stubWakeSource struct{}

func (stubWakeSource) Send(wake.SendOpts) (wake.Result, error) {
	return wake.Result{}, errors.New("wake: agent is unavailable")
}

type stubWakeAccess struct{}

func (stubWakeAccess) Authorize(string, identity.Permission) (identity.User, error) {
	return identity.User{}, errors.New("identity service unavailable")
}