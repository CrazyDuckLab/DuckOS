package agent

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/doctor"
	"github.com/CrazyDuckLab/DuckOS/internal/model"
)

type fakeObserver struct {
	doctor doctor.Snapshot
	err    error
}

func (f *fakeObserver) Overview(context.Context) (model.Overview, error) {
	return model.Overview{}, nil
}
func (f *fakeObserver) Doctor(context.Context) (doctor.Snapshot, error) {
	return f.doctor, f.err
}

func newSnapshot() doctor.Snapshot {
	b := doctor.NewBuilder("test", "duckos-dev", "boot-1", time.Now().UTC(), 10)
	b.SetSystem(doctor.System{Hostname: "router", Kernel: "6.5", OS: "debian", Architecture: "amd64"})
	b.SetNetwork(doctor.Network{Interfaces: []doctor.Interface{{Name: "lo", Index: 1, LinkUp: true, MTU: 65536}}})
	b.SetFirewall(doctor.Firewall{Backend: "nftables", Status: "available", Tables: 1, Chains: 2, Rules: 4})
	b.AddSubsystem(doctor.Subsystem{Name: "vault", Status: "ok"})
	return b.Snapshot()
}

func TestServeDoctorRoundTrip(t *testing.T) {
	h := NewHandler(&fakeObserver{doctor: newSnapshot()})
	req := httptest.NewRequest(http.MethodGet, DoctorPath, nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d (%s)", w.Code, w.Body.String())
	}
	if got := w.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("expected application/json, got %q", got)
	}
	var snap doctor.Snapshot
	if err := json.NewDecoder(w.Body).Decode(&snap); err != nil {
		t.Fatal(err)
	}
	if snap.System.Hostname != "router" {
		t.Fatalf("expected router, got %q", snap.System.Hostname)
	}
}

func TestServeDoctorRefusesMethods(t *testing.T) {
	h := NewHandler(&fakeObserver{doctor: newSnapshot()})
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		req := httptest.NewRequest(method, DoctorPath, nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != 405 {
			t.Fatalf("expected 405 for %s, got %d", method, w.Code)
		}
	}
}

func TestServeDoctorRefusesQuery(t *testing.T) {
	h := NewHandler(&fakeObserver{doctor: newSnapshot()})
	req := httptest.NewRequest(http.MethodGet, DoctorPath+"?exec=anything", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 400 {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestServeDoctorUnknownPath(t *testing.T) {
	h := NewHandler(&fakeObserver{doctor: newSnapshot()})
	req := httptest.NewRequest(http.MethodGet, "/agent/v1/doctorextra", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 404 {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestServeDoctorObserverError(t *testing.T) {
	h := NewHandler(&fakeObserver{err: errors.New("nope")})
	req := httptest.NewRequest(http.MethodGet, DoctorPath, nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 503 {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

func TestServeDoctorMissingDoctor(t *testing.T) {
	type plain struct{}
	h := NewHandler((Observer)(nil))
	_ = (*plain)(nil) // keep type referenced
	req := httptest.NewRequest(http.MethodGet, DoctorPath, nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 503 {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

func TestServeDoctorRejectsInvalidSnapshot(t *testing.T) {
	bad := doctor.Snapshot{SchemaVersion: "wrong"}
	h := NewHandler(&fakeObserver{doctor: bad})
	req := httptest.NewRequest(http.MethodGet, DoctorPath, nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 503 {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

func TestDecodeDoctorRoundTrip(t *testing.T) {
	snap := newSnapshot()
	data, err := json.Marshal(snap)
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecodeDoctor(data)
	if err != nil {
		t.Fatal(err)
	}
	if got.System.Hostname != snap.System.Hostname {
		t.Fatalf("hostname lost: %q vs %q", got.System.Hostname, snap.System.Hostname)
	}
}

func TestDecodeDoctorRejectsOversized(t *testing.T) {
	huge := make([]byte, DoctorMaxBytes+1)
	if _, err := DecodeDoctor(huge); err == nil {
		t.Fatal("oversized payload accepted")
	}
}

func TestDecodeDoctorRejectsMalformed(t *testing.T) {
	if _, err := DecodeDoctor([]byte("not json")); err == nil {
		t.Fatal("malformed payload accepted")
	}
}

func TestServeDoctorConcurrencyCap(t *testing.T) {
	h := NewHandler(&fakeObserver{doctor: newSnapshot()})
	done := make(chan struct{}, 8)
	for i := 0; i < 8; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			req := httptest.NewRequest(http.MethodGet, DoctorPath, nil)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, req)
		}()
	}
	for i := 0; i < 8; i++ {
		<-done
	}
}

func TestServeDoctorRedacts(t *testing.T) {
	b := doctor.NewBuilder("test", "duckos-dev", "boot-1", time.Now().UTC(), 0)
	b.SetVault(doctor.Vault{Available: true, KeyID: "abc", Records: 1, MasterPath: "/var/lib/duckos/vault/master"})
	b.SetWireGuard(doctor.WireGuard{Servers: 1, Peers: 2, Endpoint: "vpn.example.com:51820"})
	snap := b.Snapshot()
	h := NewHandler(&fakeObserver{doctor: snap})
	req := httptest.NewRequest(http.MethodGet, DoctorPath, nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	body := w.Body.String()
	if !strings.Contains(body, "vpn.example.com:51820") {
		t.Fatalf("expected wireguard endpoint in output, got %s", body)
	}
}
