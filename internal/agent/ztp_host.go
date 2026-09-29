// agent/ztp_host.go wires the ZTP redemption manager into
// the Agent IPC loopback. The composition root registers
// a *ztp.Manager; the IPC layer exposes a bounded read view
// (count + per-record summary) and the Redeem surface the
// dashboard needs to bind a bootstrap device.
//
// C-blocker #10: ZTP wiring. The previous code path had
// no ZTP package at all; the audit classified the key
// lifecycle and durable redemption/replay protection as
// C-class software. The host now exposes the bounded
// view + Redeem surface so the dashboard can drive a
// bootstrap.
package agent

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/ztp"
)

// ZTPPath is the IPC route for the ZTP read view +
// Redeem surface.
const ZTPPath = "/agent/v1/ztp"

// ZTPIssuePath is the IPC route for authenticated
// administrative issuance.
const ZTPIssuePath = "/agent/v1/ztp/issue"

// ZTPMaxBytes is the IPC cap on ZTP responses.
const ZTPMaxBytes = 64 * 1024

// ZTPSource is the narrow contract the host needs.
// *ztp.Manager satisfies it; tests inject a fake.
type ZTPSource interface {
	Redeem(ctx context.Context, tok ztp.IssuedToken, observedNode string) (ztp.BootstrapRecord, error)
	Issue(ctx context.Context, req ztp.IssueRequest) (ztp.IssuedToken, error)
	All() []ztp.BootstrapRecord
	Counters() ztp.Counters
}

// ZTPHost wraps a ZTPSource. A nil source is replaced
// with a stub so the handler never panics on a missing
// dependency.
type ZTPHost struct {
	ZTPSource
	timeout time.Duration
}

// NewZTPHost wraps the source.
func NewZTPHost(src ZTPSource) *ZTPHost {
	if src == nil {
		src = stubZTP{}
	}
	return &ZTPHost{ZTPSource: src, timeout: 1500 * time.Millisecond}
}

// SetTimeout overrides the per-request ceiling.
func (h *ZTPHost) SetTimeout(d time.Duration) {
	if d <= 0 {
		d = 1500 * time.Millisecond
	}
	h.timeout = d
}

// ZTPStatus is the IPC read envelope.
type ZTPStatus struct {
	Version   int                   `json:"version"`
	UpdatedAt time.Time             `json:"updatedAt"`
	Records   []ztp.BootstrapRecord `json:"records"`
	Counters  ztp.Counters          `json:"counters"`
	Ready     bool                  `json:"ready"`
}

// ZTPRedeemRequest is the IPC Redeem envelope.
type ZTPRedeemRequest struct {
	Token        string    `json:"token"`
	Subject      string    `json:"subject,omitempty"`
	Scope        string    `json:"scope,omitempty"`
	Node         string    `json:"node,omitempty"`
	IssuedAt     time.Time `json:"issuedAt"`
	ExpiresAt    time.Time `json:"expiresAt"`
	MAC          string    `json:"mac"`
	ObservedNode string    `json:"observedNode,omitempty"`
}

type ZTPIssueRequest struct {
	Subject         string `json:"subject"`
	Scope           string `json:"scope,omitempty"`
	Node            string `json:"node,omitempty"`
	ValiditySeconds int64  `json:"validitySeconds,omitempty"`
}

type ZTPIssueResponse struct {
	Token ztp.IssuedToken `json:"token"`
	Error string          `json:"error,omitempty"`
}

// ZTPRedeemResponse is the IPC Redeem reply.
type ZTPRedeemResponse struct {
	Record ztp.BootstrapRecord `json:"record"`
	Error  string              `json:"error,omitempty"`
}

// ServeZTP dispatches GET (status) and POST (Redeem).
// GET returns the bounded read view; POST drives a
// single redemption.
func (h *ZTPHost) ServeZTP(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()
	if r.URL.EscapedPath() == ZTPIssuePath {
		h.serveIssue(w, r, ctx)
		return
	}
	switch r.Method {
	case http.MethodGet:
		h.serveStatus(w, ctx)
	case http.MethodPost:
		h.serveRedeem(w, r, ctx)
	default:
		w.Header().Set("Allow", "GET, POST")
		problem(w, 405, "method_not_allowed")
	}
}

func (h *ZTPHost) serveIssue(w http.ResponseWriter, r *http.Request, ctx context.Context) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		problem(w, 405, "method_not_allowed")
		return
	}
	if r.URL.ForceQuery || r.ContentLength == 0 || len(r.TransferEncoding) != 0 {
		problem(w, 400, "invalid_request")
		return
	}
	var req ZTPIssueRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, ZTPMaxBytes)).Decode(&req); err != nil {
		problem(w, 400, "invalid_request")
		return
	}
	tok, err := h.Issue(ctx, ztp.IssueRequest{
		Subject:  req.Subject,
		Scope:    req.Scope,
		Node:     req.Node,
		Validity: time.Duration(req.ValiditySeconds) * time.Second,
	})
	resp := ZTPIssueResponse{Token: tok}
	if err != nil {
		resp.Error = err.Error()
	}
	if errors.Is(err, ztp.ErrNoProductionKey) {
		problem(w, 503, "no_production_key")
		return
	}
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		data, _ := json.Marshal(resp)
		_, _ = w.Write(data)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	data, _ := json.Marshal(resp)
	_, _ = w.Write(data)
}

func (h *ZTPHost) serveStatus(w http.ResponseWriter, _ context.Context) {
	resp := ZTPStatus{
		Version:   ztp.SchemaVersion,
		UpdatedAt: time.Now().UTC(),
		Records:   h.All(),
		Counters:  h.Counters(),
		Ready:     true,
	}
	data, err := json.Marshal(resp)
	if err != nil || len(data) > ZTPMaxBytes {
		problem(w, 503, "ztp_limit")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (h *ZTPHost) serveRedeem(w http.ResponseWriter, r *http.Request, ctx context.Context) {
	if r.URL.ForceQuery || r.ContentLength == 0 || len(r.TransferEncoding) != 0 {
		problem(w, 400, "invalid_request")
		return
	}
	var req ZTPRedeemRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, ZTPMaxBytes)).Decode(&req); err != nil {
		problem(w, 400, "invalid_request")
		return
	}
	tok := ztp.IssuedToken{
		Subject:   req.Subject,
		Scope:     req.Scope,
		Node:      req.Node,
		IssuedAt:  req.IssuedAt,
		ExpiresAt: req.ExpiresAt,
		MAC:       req.MAC,
	}
	record, err := h.Redeem(ctx, tok, req.ObservedNode)
	resp := ZTPRedeemResponse{Record: record}
	if err != nil {
		resp.Error = err.Error()
	}
	switch {
	case errors.Is(err, ztp.ErrNoProductionKey):
		problem(w, 503, "no_production_key")
		return
	case errors.Is(err, ztp.ErrTokenReplay):
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		data, _ := json.Marshal(resp)
		_, _ = w.Write(data)
		return
	case errors.Is(err, ztp.ErrTokenStale), errors.Is(err, ztp.ErrTokenMismatch), errors.Is(err, ztp.ErrCapacityExhausted):
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		data, _ := json.Marshal(resp)
		_, _ = w.Write(data)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	data, _ := json.Marshal(resp)
	_, _ = w.Write(data)
}

// stubZTP is the safe fallback when the privileged
// manager is not yet configured. Every Redeem call
// surfaces ErrNoProductionKey so the IPC layer never
// invents availability.
type stubZTP struct{}

func (stubZTP) Redeem(_ context.Context, _ ztp.IssuedToken, _ string) (ztp.BootstrapRecord, error) {
	return ztp.BootstrapRecord{}, ztp.ErrNoProductionKey
}
func (stubZTP) Issue(_ context.Context, _ ztp.IssueRequest) (ztp.IssuedToken, error) {
	return ztp.IssuedToken{}, ztp.ErrNoProductionKey
}
func (stubZTP) All() []ztp.BootstrapRecord { return []ztp.BootstrapRecord{} }
func (stubZTP) Counters() ztp.Counters     { return ztp.Counters{} }
