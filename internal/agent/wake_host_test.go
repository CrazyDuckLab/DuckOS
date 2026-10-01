//go:build linux

// agent/wake_host_test.go: bounded tests for the
// Wake-on-LAN agent host.
//
// Wake only ever reports PACKET_SENT — never DEVICE_ONLINE.
// The host enforces the authorisation gate (ConfigureNetwork)
// and refuses non-canonical MACs up front so the agent's
// underlying manager never sees them.
package agent

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/CrazyDuckLab/DuckOS/internal/identity"
	"github.com/CrazyDuckLab/DuckOS/internal/wake"
)

type fakeWakeSource struct {
	mu      sync.Mutex
	lastRes wake.Result
	calls   int
}

func (f *fakeWakeSource) Send(o wake.SendOpts) (wake.Result, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if err := o.Validate(); err != nil {
		return wake.Result{}, err
	}
	f.lastRes = wake.Result{
		Target:      o.Target,
		Broadcast:   "255.255.255.255:9",
		Port:        9,
		Packets:     3,
		Bytes:       306,
		SentAt:      wake.Result{}.SentAt,
		Status:      wake.StatusSent,
		Observation: wake.ObservationPacketSent,
	}
	return f.lastRes, nil
}

type fakeWakeAccess struct {
	grant identity.Permission
}

func (f *fakeWakeAccess) Authorize(_ string, p identity.Permission) (identity.User, error) {
	if p == f.grant {
		return identity.User{}, nil
	}
	return identity.User{}, errors.New("forbidden")
}

func TestWakeHost_Send(t *testing.T) {
	src := &fakeWakeSource{}
	host := NewWakeHost(src, &fakeWakeAccess{grant: identity.ConfigureNetwork})
	body, _ := json.Marshal(map[string]any{"target": "AA:BB:CC:DD:EE:FF", "port": 9, "packets": 3})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, WakePath+"/send", ioReader(body))
	req.Header.Set("Authorization", "Bearer t")
	host.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d body=%s", rec.Code, rec.Body.String())
	}
	if src.calls != 1 {
		t.Fatalf("expected 1 send, got %d", src.calls)
	}
	if src.lastRes.Observation != wake.ObservationPacketSent {
		t.Fatalf("expected PACKET_SENT, got %s", src.lastRes.Observation)
	}
}

func TestWakeHost_RejectsInvalidMAC(t *testing.T) {
	src := &fakeWakeSource{}
	host := NewWakeHost(src, &fakeWakeAccess{grant: identity.ConfigureNetwork})
	body, _ := json.Marshal(map[string]any{"target": "not-a-mac"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, WakePath+"/send", ioReader(body))
	req.Header.Set("Authorization", "Bearer t")
	host.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestWakeHost_NoBearer(t *testing.T) {
	src := &fakeWakeSource{}
	host := NewWakeHost(src, &fakeWakeAccess{grant: identity.ConfigureNetwork})
	body, _ := json.Marshal(map[string]any{"target": "AA:BB:CC:DD:EE:FF"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, WakePath+"/send", ioReader(body))
	host.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestWakeHost_AuthRequiresConfigure(t *testing.T) {
	src := &fakeWakeSource{}
	host := NewWakeHost(src, &fakeWakeAccess{grant: identity.Observe})
	body, _ := json.Marshal(map[string]any{"target": "AA:BB:CC:DD:EE:FF"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, WakePath+"/send", ioReader(body))
	req.Header.Set("Authorization", "Bearer t")
	host.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestWakeHost_UnknownPath(t *testing.T) {
	src := &fakeWakeSource{}
	host := NewWakeHost(src, &fakeWakeAccess{grant: identity.ConfigureNetwork})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, WakePath+"/unknown", nil)
	req.Header.Set("Authorization", "Bearer t")
	host.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestWakeHost_MethodNotAllowed(t *testing.T) {
	src := &fakeWakeSource{}
	host := NewWakeHost(src, &fakeWakeAccess{grant: identity.ConfigureNetwork})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, WakePath+"/send", nil)
	req.Header.Set("Authorization", "Bearer t")
	host.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound && rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 404/405, got %d", rec.Code)
	}
}

func wakeReader(b []byte) io.Reader { return strings.NewReader(string(b)) }

var _ = ioReader // alias kept for clarity; ioReader defined in upnp_host_test.go.