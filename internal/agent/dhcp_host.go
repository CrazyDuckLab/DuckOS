// agent/dhcp_host.go: agent-side read-only DHCP lease
// observation surface.
//
//	PUBLIC -> Engine.DhcpProxy -> Agent IPC
//	       -> DhcpHost -> dnsmasq lease file
//
// The host is strictly read-only (Observe permission): lease
// mutation belongs to the DHCP server, not to the API. The
// lease file is read whole with a hard size bound so a
// runaway lease file cannot exhaust the agent.
package agent

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/dhcpsnoop"
	"github.com/CrazyDuckLab/DuckOS/internal/identity"
)

// DhcpPath is the IPC route prefix for the DHCP host.
const DhcpPath = "/agent/v1/dhcp"

// MaxLeaseFileBytes bounds the lease file read. A dnsmasq
// lease line is ~100 bytes; 4 MiB covers ~40k leases.
const MaxLeaseFileBytes = 4 << 20

// LeaseReader is the narrow source the host consumes.
type LeaseReader func() ([]dhcpsnoop.Lease, error)

// DhcpHost serves the read-only lease view.
type DhcpHost struct {
	mu     sync.Mutex
	read   LeaseReader
	Access UPnPAccess // reuse the narrow Authorize contract
	source string
}

// NewDhcpHost wires the host to a lease reader. source is a
// human-readable description of the backing store (surfaced
// verbatim in the state document).
func NewDhcpHost(read LeaseReader, acc UPnPAccess, source string) *DhcpHost {
	return &DhcpHost{read: read, Access: acc, source: source}
}

// FileLeaseReader returns a LeaseReader over a dnsmasq
// lease file at path.
func FileLeaseReader(path string) LeaseReader {
	return func() ([]dhcpsnoop.Lease, error) {
		f, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		buf := make([]byte, MaxLeaseFileBytes+1)
		n, err := f.Read(buf)
		if err != nil && n == 0 {
			return nil, err
		}
		if n > MaxLeaseFileBytes {
			n = MaxLeaseFileBytes
		}
		return dhcpsnoop.ParseDnsmasqLeases(buf[:n]), nil
	}
}

// ServeHTTP dispatches /agent/v1/dhcp/*:
//
//	GET /dhcp/v1/leases -> { leases, source, observedAt }
func (h *DhcpHost) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	tail := strings.TrimPrefix(r.URL.EscapedPath(), DhcpPath)
	tail = strings.Trim(tail, "/")
	switch {
	case r.Method == http.MethodGet && tail == "leases":
		h.handleLeases(w, r)
	default:
		w.Header().Set("Allow", "GET")
		dhcpProblem(w, 404, "not_found", "DHCP endpoint not found.")
	}
}

func (h *DhcpHost) handleLeases(w http.ResponseWriter, r *http.Request) {
	v := r.Header.Get("Authorization")
	if len(v) < 7 || v[:7] != "Bearer " {
		dhcpProblem(w, 401, "unauthorized", "Bearer token is required.")
		return
	}
	if _, err := h.Access.Authorize(v[7:], identity.Observe); err != nil {
		dhcpProblem(w, 403, "forbidden", "Required permission is missing.")
		return
	}
	h.mu.Lock()
	read := h.read
	source := h.source
	h.mu.Unlock()
	leases, err := read()
	out := struct {
		Leases     []dhcpsnoop.Lease `json:"leases"`
		Source     string            `json:"source"`
		ObservedAt string            `json:"observedAt"`
		Error      string            `json:"error,omitempty"`
	}{
		Source:     source,
		ObservedAt: time.Now().UTC().Format(time.RFC3339),
	}
	if err != nil {
		// The observation is honest: report the failure, keep
		// the API shape stable.
		out.Error = err.Error()
		out.Leases = []dhcpsnoop.Lease{}
	} else {
		if leases == nil {
			leases = []dhcpsnoop.Lease{}
		}
		out.Leases = leases
	}
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(out)
}

func dhcpProblem(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]string{"code": code, "message": message},
	})
}
