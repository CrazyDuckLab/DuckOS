// agent/firewall_host.go: read-only observation of the
// DuckOS-owned nftables table (inet/duckos).
//
//	PUBLIC -> Engine.FirewallProxy -> Agent IPC
//	       -> FirewallHost -> network.FirewallHost.Capture
//
// Strictly read-only (Observe). Rule mutation stays with the
// Safe Apply configuration path; this surface only reports
// observed kernel truth so the UI can replace its
// contract-gap stub with evidence.
package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/identity"
	"github.com/CrazyDuckLab/DuckOS/internal/network"
)

// FirewallPath is the IPC route prefix for the firewall
// observation host.
const FirewallPath = "/agent/v1/firewall"

// SnapshotProvider captures the owned nftables snapshot.
type SnapshotProvider func() (network.NFTSnapshot, error)

// FirewallHost serves the read-only firewall snapshot.
type FirewallHost struct {
	mu     sync.Mutex
	capfn  SnapshotProvider
	Access UPnPAccess
}

// NewFirewallHost wires the host. capfn is injectable for
// tests; production passes a live network.FirewallHost.
func NewFirewallHost(capfn SnapshotProvider, acc UPnPAccess) *FirewallHost {
	return &FirewallHost{capfn: capfn, Access: acc}
}

// LiveSnapshotProvider returns a SnapshotProvider backed by
// a real nft capture.
func LiveSnapshotProvider() SnapshotProvider {
	return func() (network.NFTSnapshot, error) {
		h, err := network.NewFirewallHost()
		if err != nil {
			return network.NFTSnapshot{}, err
		}
		return h.Capture(context.Background())
	}
}

// ServeHTTP dispatches /agent/v1/firewall/*:
//
//	GET /firewall/v1/observed -> { objects, empty, observedAt, error? }
//
// (beta.4: the typed rule surface moved to /agent/v1/fwrules;
// this host keeps the raw owned-table snapshot.)
func (h *FirewallHost) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	tail := strings.Trim(strings.TrimPrefix(r.URL.EscapedPath(), FirewallPath), "/")
	// The engine proxy composes FirewallPath + "/firewall/v1/"
	// + verb; strip the version segment so the canonical verb
	// reaches the switch.
	if strings.HasPrefix(tail, "firewall/v1/") {
		tail = strings.TrimPrefix(tail, "firewall/v1/")
	}
	switch {
	case r.Method == http.MethodGet && tail == "observed":
		h.handleRules(w, r)
	default:
		w.Header().Set("Allow", "GET")
		fwProblem(w, 404, "not_found", "Firewall endpoint not found.")
	}
}

func (h *FirewallHost) handleRules(w http.ResponseWriter, r *http.Request) {
	v := r.Header.Get("Authorization")
	if len(v) < 7 || v[:7] != "Bearer " {
		fwProblem(w, 401, "unauthorized", "Bearer token is required.")
		return
	}
	if _, err := h.Access.Authorize(v[7:], identity.Observe); err != nil {
		fwProblem(w, 403, "forbidden", "Required permission is missing.")
		return
	}
	h.mu.Lock()
	capfn := h.capfn
	h.mu.Unlock()
	snap, err := capfn()
	out := struct {
		Objects    []json.RawMessage `json:"objects"`
		Empty      bool              `json:"empty"`
		ObservedAt string            `json:"observedAt"`
		Error      string            `json:"error,omitempty"`
	}{Objects: []json.RawMessage{}, ObservedAt: time.Now().UTC().Format(time.RFC3339)}
	if err != nil {
		out.Error = err.Error()
	} else {
		out.Objects = snap.Objects
		out.Empty = len(snap.Objects) == 0
	}
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(out)
}

func fwProblem(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]string{"code": code, "message": message},
	})
}
