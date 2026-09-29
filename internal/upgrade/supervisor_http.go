package upgrade

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"time"
)

type SupervisorHTTP struct {
	Supervisor *Supervisor
	operation  sync.Mutex
}

func (h *SupervisorHTTP) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	fail := func(code int, message string) {
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": "upgrade_refused", "message": message}})
	}
	if r.URL.RawQuery != "" || r.URL.ForceQuery || r.URL.EscapedPath() != r.URL.Path {
		fail(400, "Invalid update request")
		return
	}
	if r.URL.Path == "/v1/status" && r.Method == "GET" {
		st := h.Supervisor.State()
		healthy := st.Active != "" && !st.RecoveryRequired && !h.Supervisor.Pending()
		if len(st.History) == 0 || st.History[len(st.History)-1].Result != "health-passed" {
			healthy = false
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"version": 1, "updatedAt": st.UpdatedAt, "currentRelease": st.Active, "previousRelease": st.Previous, "candidateRelease": st.Candidate, "healthy": healthy, "notes": []string{func() string {
			if st.RecoveryRequired {
				return "recovery_required"
			}
			return ""
		}()}})
		return
	}
	if r.Method != "POST" {
		fail(405, "POST required")
		return
	}
	if r.URL.Path != "/v1/stage" && r.URL.Path != "/v1/activate" && r.URL.Path != "/v1/rollback" {
		fail(404, "Unknown update operation")
		return
	}
	if !h.operation.TryLock() {
		fail(409, "Update operation in progress")
		return
	}
	if r.URL.Path == "/v1/stage" {
		defer h.operation.Unlock()
		var body struct {
			Operator string  `json:"operator"`
			Channel  Channel `json:"channel"`
			ID       string  `json:"releaseId"`
			Version  string  `json:"releaseVersion"`
			Arch     string  `json:"releaseArch"`
			Payload  string  `json:"payloadBase64"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<20))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&body) != nil || decoder.Decode(new(any)) != io.EOF {
			fail(400, "Invalid stage object")
			return
		}
		payload, err := base64.StdEncoding.DecodeString(body.Payload)
		if err != nil {
			fail(400, "Invalid package encoding")
			return
		}
		err = h.Supervisor.Stage(Plan{Channel: body.Channel, Release: Release{ID: body.ID, Version: body.Version, Arch: body.Arch, Channel: body.Channel}, Payload: payload, Operator: body.Operator})
		if err != nil {
			fail(422, "Signed candidate validation failed")
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"version": 1, "result": "staged"})
		return
	}
	var body struct {
		Reason string `json:"reason,omitempty"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil && err != io.EOF {
		h.operation.Unlock()
		fail(400, "Invalid operation object")
		return
	}
	if decoder.Decode(new(any)) != io.EOF {
		h.operation.Unlock()
		fail(400, "Invalid trailing input")
		return
	}
	rollback := r.URL.Path == "/v1/rollback"
	// The supervisor owns the operation lifetime. Agent restart and client
	// disconnect cannot cancel recovery. "accepted" never means "activated".
	go func() {
		defer h.operation.Unlock()
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		if rollback {
			_ = h.Supervisor.Rollback(ctx)
		} else {
			_ = h.Supervisor.Activate(ctx)
		}
	}()
	_ = json.NewEncoder(w).Encode(map[string]any{"version": 1, "result": "accepted"})
}
