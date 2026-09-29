package agent

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"time"
)

// UpdaterProxy grants no write access to the release store. The only transport
// is the installation-owned local Unix socket; IPC requests contain no paths.
type UpdaterProxy struct{ client *http.Client }

func NewUpdaterProxy(socket string) *UpdaterProxy {
	return &UpdaterProxy{client: &http.Client{Timeout: 90 * time.Second, Transport: &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{Timeout: 3 * time.Second}).DialContext(ctx, "unix", socket)
	}}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}
func (p *UpdaterProxy) Status(ctx context.Context) (UpgradeStatus, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", "http://updater/v1/status", nil)
	res, err := p.client.Do(req)
	if err != nil {
		return UpgradeStatus{}, err
	}
	defer res.Body.Close()
	var status UpgradeStatus
	if res.StatusCode != 200 || json.NewDecoder(io.LimitReader(res.Body, 65536)).Decode(&status) != nil {
		return status, errors.New("updater status unavailable")
	}
	return status, nil
}
func (p *UpdaterProxy) forward(ctx context.Context, w http.ResponseWriter, r *http.Request, path string) {
	if r.Method != "POST" || r.URL.RawQuery != "" || r.URL.ForceQuery {
		writeUpgradeProblem(w, 400, "invalid_request", "Invalid update operation.")
		return
	}
	req, err := http.NewRequestWithContext(ctx, "POST", "http://updater/v1/"+path, http.MaxBytesReader(w, r.Body, UpgradeStageMaxBytes))
	if err != nil {
		writeUpgradeProblem(w, 400, "invalid_request", "Invalid request.")
		return
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := p.client.Do(req)
	if err != nil {
		writeUpgradeProblem(w, 503, "upgrade_unavailable", "Updater unavailable.")
		return
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 65537))
	if err != nil || len(body) > 65536 || !json.Valid(body) {
		writeUpgradeProblem(w, 503, "upgrade_unavailable", "Updater response invalid.")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(res.StatusCode)
	_, _ = w.Write(body)
}
func (p *UpdaterProxy) Stage(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	p.forward(ctx, w, r, "stage")
}
func (p *UpdaterProxy) Activate(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	p.forward(ctx, w, r, "activate")
}
func (p *UpdaterProxy) Rollback(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	p.forward(ctx, w, r, "rollback")
}
