//go:build linux

// agent/upnp_host_test.go: bounded tests for the UPnP /
// NAT-PMP / PCP agent host.
//
// The agent owns execution. The host enforces the
// authorisation gate, the validate-before-apply boundary,
// and the OFF-by-default semantics. The runtime is
// exercised via the cross-platform upnp.RuntimeStub so the
// tests run on every host without requiring nftables.
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
	"github.com/CrazyDuckLab/DuckOS/internal/upnp"
)

type fakeUPnPSource struct {
	mu       sync.Mutex
	networks []upnp.Network
	mappings []upnp.Mapping
	revoked  []string
	disabled []string
}

func (f *fakeUPnPSource) EnableNetwork(net upnp.Network, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := net.Valid(); err != nil {
		return err
	}
	f.networks = append(f.networks, net)
	return nil
}
func (f *fakeUPnPSource) DisableNetwork(ref string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.disabled = append(f.disabled, ref)
	f.networks = nil
}
func (f *fakeUPnPSource) Networks() []upnp.Network {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]upnp.Network, len(f.networks))
	copy(out, f.networks)
	return out
}
func (f *fakeUPnPSource) Mappings() []upnp.Mapping {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]upnp.Mapping, len(f.mappings))
	copy(out, f.mappings)
	return out
}
func (f *fakeUPnPSource) RevokeMapping(id string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.revoked = append(f.revoked, id)
}
func (f *fakeUPnPSource) Stats() upnp.RuntimeStats {
	f.mu.Lock()
	defer f.mu.Unlock()
	return upnp.RuntimeStats{Networks: len(f.networks), Mappings: len(f.mappings)}
}

type fakeUPnPAccess struct {
	grant identity.Permission
}

func (f *fakeUPnPAccess) Authorize(_ string, p identity.Permission) (identity.User, error) {
	if p == f.grant {
		return identity.User{}, nil
	}
	return identity.User{}, errors.New("forbidden")
}

func TestUPnPHost_StateRequiresObserve(t *testing.T) {
	src := &fakeUPnPSource{}
	host := NewUPnPHost(src, &fakeUPnPAccess{grant: identity.ConfigureNetwork})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, UPnPPath+"/state", nil)
	req.Header.Set("Authorization", "Bearer t")
	host.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
	host2 := NewUPnPHost(src, &fakeUPnPAccess{grant: identity.Observe})
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, UPnPPath+"/state", nil)
	req2.Header.Set("Authorization", "Bearer t")
	host2.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec2.Code, rec2.Body.String())
	}
}

func TestUPnPHost_EnableAndDisable(t *testing.T) {
	src := &fakeUPnPSource{}
	host := NewUPnPHost(src, &fakeUPnPAccess{grant: identity.ConfigureNetwork})
	body, _ := json.Marshal(map[string]any{
		"network": map[string]any{
			"networkRef": "lan.home",
			"protocols":  []string{"upnp", "nat-pmp"},
			"ttlSec":     3600,
		},
		"iface": "eth0",
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, UPnPPath+"/networks/lan.home", ioReader(body))
	req.Header.Set("Authorization", "Bearer t")
	host.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d body=%s", rec.Code, rec.Body.String())
	}
	if len(src.networks) != 1 {
		t.Fatalf("expected 1 network, got %d", len(src.networks))
	}
	if src.networks[0].NetworkRef != "lan.home" {
		t.Fatalf("expected lan.home, got %s", src.networks[0].NetworkRef)
	}
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodDelete, UPnPPath+"/networks/lan.home", nil)
	req2.Header.Set("Authorization", "Bearer t")
	host.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusNoContent {
		t.Fatalf("expected 204 on disable, got %d", rec2.Code)
	}
	if len(src.disabled) != 1 {
		t.Fatalf("expected 1 disabled, got %d", len(src.disabled))
	}
}

func TestUPnPHost_EnableRejectsInvalidNetwork(t *testing.T) {
	src := &fakeUPnPSource{}
	host := NewUPnPHost(src, &fakeUPnPAccess{grant: identity.ConfigureNetwork})
	body, _ := json.Marshal(map[string]any{
		"network": map[string]any{
			"networkRef": "!!bogus!!",
			"protocols":  []string{"upnp"},
			"ttlSec":     3600,
		},
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, UPnPPath+"/networks/!!bogus!!", ioReader(body))
	req.Header.Set("Authorization", "Bearer t")
	host.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestUPnPHost_RevokeMapping(t *testing.T) {
	src := &fakeUPnPSource{mappings: []upnp.Mapping{{ID: "m1"}}}
	host := NewUPnPHost(src, &fakeUPnPAccess{grant: identity.ConfigureNetwork})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, UPnPPath+"/mappings/m1", nil)
	req.Header.Set("Authorization", "Bearer t")
	host.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec.Code)
	}
	if len(src.revoked) != 1 || src.revoked[0] != "m1" {
		t.Fatalf("expected revoke m1, got %v", src.revoked)
	}
}

func TestUPnPHost_NoBearer(t *testing.T) {
	src := &fakeUPnPSource{}
	host := NewUPnPHost(src, &fakeUPnPAccess{grant: identity.Observe})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, UPnPPath+"/state", nil)
	host.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestUPnPHost_MultipleAuthorizationHeaders(t *testing.T) {
	src := &fakeUPnPSource{}
	host := NewUPnPHost(src, &fakeUPnPAccess{grant: identity.Observe})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, UPnPPath+"/state", nil)
	req.Header.Add("Authorization", "Bearer t1")
	req.Header.Add("Authorization", "Bearer t2")
	host.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestUPnPHost_UnknownPath(t *testing.T) {
	src := &fakeUPnPSource{}
	host := NewUPnPHost(src, &fakeUPnPAccess{grant: identity.Observe})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, UPnPPath+"/unknown", nil)
	req.Header.Set("Authorization", "Bearer t")
	host.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestUPnPHost_MethodNotAllowed(t *testing.T) {
	src := &fakeUPnPSource{}
	host := NewUPnPHost(src, &fakeUPnPAccess{grant: identity.Observe})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch, UPnPPath+"/state", nil)
	req.Header.Set("Authorization", "Bearer t")
	host.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func ioReader(b []byte) io.Reader { return strings.NewReader(string(b)) }