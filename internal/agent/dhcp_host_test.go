package agent

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/CrazyDuckLab/DuckOS/internal/dhcpsnoop"
)

func TestDhcpHostLeasesRoundTrip(t *testing.T) {
	h := NewDhcpHost(func() ([]dhcpsnoop.Lease, error) {
		return []dhcpsnoop.Lease{{MAC: "aa:bb:cc:dd:ee:ff", IP: "192.168.10.50", Hostname: "client-1", Trusted: true}}, nil
	}, stubUPnPAccess{}, "test-leases")
	req := httptest.NewRequest(http.MethodGet, "/agent/v1/dhcp/dhcp/v1/leases", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status: %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"aa:bb:cc:dd:ee:ff"`) || !strings.Contains(body, "test-leases") {
		t.Fatalf("body: %s", body)
	}
}

func TestDhcpHostRequiresAuth(t *testing.T) {
	h := NewDhcpHost(func() ([]dhcpsnoop.Lease, error) { return nil, nil }, stubUPnPAccess{}, "test")
	req := httptest.NewRequest(http.MethodGet, "/agent/v1/dhcp/dhcp/v1/leases", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 401 {
		t.Fatalf("unauthenticated status: %d", rec.Code)
	}
}

func TestDhcpHostReportsSourceErrorHonestly(t *testing.T) {
	h := NewDhcpHost(func() ([]dhcpsnoop.Lease, error) {
		return nil, errLeaseSourceDown
	}, stubUPnPAccess{}, "missing-file")
	req := httptest.NewRequest(http.MethodGet, "/agent/v1/dhcp/dhcp/v1/leases", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status: %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "lease source down") {
		t.Fatalf("error not surfaced: %s", rec.Body.String())
	}
}

var errLeaseSourceDown = &leaseSourceError{}

type leaseSourceError struct{}

func (*leaseSourceError) Error() string { return "lease source down" }
