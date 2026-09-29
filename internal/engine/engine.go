// Package engine owns observation use cases and a bounded, short-lived cache.
// It never executes commands or opens Linux configuration files.
package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/agent"
	"github.com/CrazyDuckLab/DuckOS/internal/chaos"
	"github.com/CrazyDuckLab/DuckOS/internal/clientexport"
	"github.com/CrazyDuckLab/DuckOS/internal/doctor"
	"github.com/CrazyDuckLab/DuckOS/internal/frr"
	"github.com/CrazyDuckLab/DuckOS/internal/migrate"
	"github.com/CrazyDuckLab/DuckOS/internal/model"
	"github.com/CrazyDuckLab/DuckOS/internal/multiwan"
	"github.com/CrazyDuckLab/DuckOS/internal/qos"
	"github.com/CrazyDuckLab/DuckOS/internal/restore"
	"github.com/CrazyDuckLab/DuckOS/internal/sites"
	"github.com/CrazyDuckLab/DuckOS/internal/topology"
	"github.com/CrazyDuckLab/DuckOS/internal/ups"
	"github.com/CrazyDuckLab/DuckOS/internal/wifi"
)

type Engine struct {
	restoreMu     sync.Mutex
	exports       *clientexport.Service
	client        *http.Client
	environment   string
	ttl           time.Duration
	mu            sync.Mutex
	cached        []byte
	expires       time.Time
	inflight      chan struct{}
	doctorMu      sync.Mutex
	doctorCache   doctor.Snapshot
	doctorExpiry  time.Time
	readiness     *chaos.ReadinessStore
	config        *Config
	objectListers map[string]ObjectLister
}

// SetReadiness wires the readiness store that the Engine
// uses for the Readiness() snapshot. It is called by the
// agent composition root once on boot.
func (e *Engine) SetReadiness(store *chaos.ReadinessStore) {
	e.readiness = store
}

// SetConfig wires the canonical Config coordinator that
// the public API handler uses for Config().Prepare,
// Config().Confirm and Config().Rollback. The Engine
// retains the previous value across the assignment so a
// restart path that re-runs EnableConfig can swap the
// coordinator without dropping concurrent callers.
func (e *Engine) SetConfig(c *Config) {
	e.config = c
}

// Config returns the canonical config transaction journal
// (G01 Safe Apply public contract). Returns nil when the
// engine composition root has not enabled config.
func (e *Engine) Config() *Config { return e.config }

func New(socket, environment string) *Engine {
	transport := &http.Transport{Proxy: nil, DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{Timeout: time.Second}).DialContext(ctx, "unix", socket)
	}, MaxIdleConns: 2, MaxIdleConnsPerHost: 2, MaxConnsPerHost: 2, IdleConnTimeout: 30 * time.Second, ResponseHeaderTimeout: 8 * time.Second, MaxResponseHeaderBytes: 8192, DisableCompression: true}
	return &Engine{client: &http.Client{Transport: transport, Timeout: 9 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, environment: environment, ttl: 2 * time.Second}
}

func (e *Engine) Overview(ctx context.Context) ([]byte, error) { return e.overview(ctx, false) }

// Ready probes the actual IPC boundary rather than trusting a previous cache hit.
func (e *Engine) Ready(ctx context.Context) error { _, err := e.overview(ctx, true); return err }

// Doctor fetches the structured diagnostic snapshot from the
// privileged Agent. The cache is short (2 s) but is invalidated on
// every call: a Doctor dump is intentionally cheap to take and
// operators use it to compare a stuck system with a working one.
func (e *Engine) Doctor(ctx context.Context) (doctor.Snapshot, error) {
	e.doctorMu.Lock()
	if !e.doctorCache.GeneratedAt.IsZero() && time.Now().Before(e.doctorExpiry) {
		snap := e.doctorCache
		e.doctorMu.Unlock()
		return snap, nil
	}
	e.doctorMu.Unlock()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://agent"+agent.DoctorPath, nil)
	if err != nil {
		return doctor.Snapshot{}, err
	}
	resp, err := e.client.Do(req)
	if err != nil {
		return doctor.Snapshot{}, errors.New("Agent is unavailable")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "application/json" {
		return doctor.Snapshot{}, errors.New("Agent rejected doctor request")
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, agent.DoctorMaxBytes+1))
	if err != nil {
		return doctor.Snapshot{}, errors.New("Agent response read failed")
	}
	snap, err := agent.DecodeDoctor(data)
	if err != nil {
		return doctor.Snapshot{}, err
	}
	e.doctorMu.Lock()
	e.doctorCache = snap
	e.doctorExpiry = time.Now().Add(e.ttl)
	e.doctorMu.Unlock()
	return snap, nil
}

// TopologyMaxBytes is the Engine IPC cap for Topology payloads.
const TopologyMaxBytes = 80 << 10

// Topology fetches the relationship graph from the Agent. The
// Engine does not build the graph itself: the Agent is the
// privileged side that has access to the live interface, device
// catalog and tunnel state. The Engine just validates and caches
// the result.
func (e *Engine) Topology(ctx context.Context) (topology.Graph, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://agent"+agent.TopologyPath, nil)
	if err != nil {
		return topology.Graph{}, err
	}
	resp, err := e.client.Do(req)
	if err != nil {
		return topology.Graph{}, errors.New("Agent is unavailable")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "application/json" {
		return topology.Graph{}, errors.New("Agent rejected topology request")
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, TopologyMaxBytes+1))
	if err != nil {
		return topology.Graph{}, errors.New("Agent response read failed")
	}
	var g topology.Graph
	if err := json.Unmarshal(data, &g); err != nil {
		return topology.Graph{}, errors.New("topology decode failed")
	}
	if err := g.Valid(); err != nil {
		return topology.Graph{}, errors.New("topology validation failed")
	}
	return g, nil
}

// ReadinessMaxBytes is the Engine IPC cap for readiness payloads.
const ReadinessMaxBytes = 32 << 10

// readinessIPC is the small shape returned by the Agent's
// /agent/v1/readiness endpoint.
type readinessIPC struct {
	Version   int                      `json:"version"`
	UpdatedAt time.Time                `json:"updatedAt"`
	Features  []chaos.FeatureReadiness `json:"features"`
}

// fetchReadiness pulls the canonical readiness manifest
// from the privileged Agent. The Engine proxies the call
// and decodes the bounded envelope. A non-2xx response or
// a decode failure surfaces as an error so callers can
// render the gap honestly.
func (e *Engine) fetchReadiness(ctx context.Context) ([]chaos.FeatureReadiness, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://agent"+agent.ReadinessPath, nil)
	if err != nil {
		return nil, err
	}
	resp, err := e.client.Do(req)
	if err != nil {
		return nil, errors.New("Agent is unavailable")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "application/json" {
		return nil, errors.New("Agent rejected readiness request")
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, ReadinessMaxBytes+1))
	if err != nil {
		return nil, errors.New("Agent response read failed")
	}
	var msg readinessIPC
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, errors.New("readiness decode failed")
	}
	if msg.Version == 0 {
		return nil, errors.New("readiness envelope version is 0")
	}
	return msg.Features, nil
}

// MultiWANMaxBytes is the Engine IPC cap for Multi-WAN payloads.
const MultiWANMaxBytes = 80 << 10

// MultiWAN fetches the multi-WAN plan, the most recent
// per-group selections and the per-member health from the
// Agent. The Engine proxies the call, validates the topology
// and passes through the read-only data.
func (e *Engine) MultiWAN(ctx context.Context) (multiwan.Topology, map[string]multiwan.Selection, map[string]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://agent"+agent.MultiWANPath, nil)
	if err != nil {
		return multiwan.Topology{}, nil, nil, err
	}
	resp, err := e.client.Do(req)
	if err != nil {
		return multiwan.Topology{}, nil, nil, errors.New("Agent is unavailable")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "application/json" {
		return multiwan.Topology{}, nil, nil, errors.New("Agent rejected multiwan request")
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, MultiWANMaxBytes+1))
	if err != nil {
		return multiwan.Topology{}, nil, nil, errors.New("Agent response read failed")
	}
	var payload struct {
		Topology   multiwan.Topology             `json:"topology"`
		Selections map[string]multiwan.Selection `json:"selections"`
		Health     map[string]string             `json:"health"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return multiwan.Topology{}, nil, nil, errors.New("multiwan decode failed")
	}
	if err := payload.Topology.Valid(); err != nil && len(payload.Topology.Groups) > 0 {
		return multiwan.Topology{}, nil, nil, errors.New("multiwan validation failed")
	}
	return payload.Topology, payload.Selections, payload.Health, nil
}

// QoSMaxBytes is the Engine IPC cap for QoS payloads.
// The cap matches the Agent IPC cap; the Engine
// never inflates it.
const QoSMaxBytes = agent.QoSMaxBytes

// QoS fetches the read-only QoS state from the
// Agent. The state is the topology, the last
// successful apply and the last error. The Engine
// validates the topology and re-validates the byte
// cap so a hostile Agent cannot inflate the
// payload.
func (e *Engine) QoS(ctx context.Context) (qos.Status, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://agent"+agent.QoSPath, nil)
	if err != nil {
		return qos.Status{}, err
	}
	resp, err := e.client.Do(req)
	if err != nil {
		return qos.Status{}, errors.New("Agent is unavailable")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "application/json" {
		return qos.Status{}, errors.New("Agent rejected qos request")
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, QoSMaxBytes+1))
	if err != nil {
		return qos.Status{}, errors.New("Agent response read failed")
	}
	var st qos.Status
	if err := json.Unmarshal(data, &st); err != nil {
		return qos.Status{}, errors.New("qos decode failed")
	}
	if err := st.Topology.Valid(); err != nil && len(st.Topology.Shapers) > 0 {
		return qos.Status{}, errors.New("qos validation failed")
	}
	return st, nil
}

// QoSApply submits a plan to the Agent. The plan
// is the bounded, validated contract the Engine
// forwards. The ApplyOutcome is the redacted
// summary the Engine returns to the API/CLI.
func (e *Engine) QoSApply(ctx context.Context, p qos.Plan) (qos.ApplyOutcome, error) {
	if err := p.Valid(); err != nil {
		return qos.ApplyOutcome{}, err
	}
	body, err := qos.PlanJSON(p)
	if err != nil {
		return qos.ApplyOutcome{}, err
	}
	if len(body) > QoSMaxBytes {
		return qos.ApplyOutcome{}, errors.New("qos: plan exceeds engine cap")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://agent"+agent.QoSApplyPath, bytes.NewReader(body))
	if err != nil {
		return qos.ApplyOutcome{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := e.client.Do(req)
	if err != nil {
		return qos.ApplyOutcome{}, errors.New("Agent is unavailable")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return qos.ApplyOutcome{}, decodeQoSError(resp)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, QoSMaxBytes+1))
	if err != nil {
		return qos.ApplyOutcome{}, errors.New("Agent response read failed")
	}
	var out qos.ApplyOutcome
	if err := json.Unmarshal(data, &out); err != nil {
		return qos.ApplyOutcome{}, errors.New("qos apply decode failed")
	}
	return out, nil
}

// QoSRollback asks the Agent to roll back the
// active plan. The Engine surfaces the Agent's
// outcome; the API and CLI never see raw tc state.
func (e *Engine) QoSRollback(ctx context.Context) (qos.ApplyOutcome, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://agent"+agent.QoSRollbackPath, nil)
	if err != nil {
		return qos.ApplyOutcome{}, err
	}
	resp, err := e.client.Do(req)
	if err != nil {
		return qos.ApplyOutcome{}, errors.New("Agent is unavailable")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return qos.ApplyOutcome{}, decodeQoSError(resp)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, QoSMaxBytes+1))
	if err != nil {
		return qos.ApplyOutcome{}, errors.New("Agent response read failed")
	}
	var out qos.ApplyOutcome
	if err := json.Unmarshal(data, &out); err != nil {
		return qos.ApplyOutcome{}, errors.New("qos rollback decode failed")
	}
	return out, nil
}

// decodeQoSError is a tiny helper that surfaces the
// Agent's structured error body. The Engine never
// returns the raw HTTP status; the operator sees a
// message that names the failing step.
func decodeQoSError(resp *http.Response) error {
	data, err := io.ReadAll(io.LimitReader(resp.Body, QoSMaxBytes))
	if err != nil {
		return errors.New("qos: agent returned an error")
	}
	var payload struct {
		Error map[string]string `json:"error"`
	}
	if err := json.Unmarshal(data, &payload); err == nil {
		if msg, ok := payload.Error["message"]; ok && msg != "" {
			return errors.New("qos: " + msg)
		}
	}
	return errors.New("qos: agent returned an error")
}

// UPSMaxBytes is the Engine IPC cap for UPS payloads.
// The cap matches the Agent IPC cap; the Engine
// never inflates it.
const UPSMaxBytes = agent.UPSMaxBytes

// UPS fetches the read-only UPS state from the
// Agent. The state is the topology, the per-device
// health, the per-device variables and the last
// probe timestamp. The Engine validates the
// topology and re-validates the byte cap so a
// hostile Agent cannot inflate the payload.
func (e *Engine) UPS(ctx context.Context) (ups.Status, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://agent"+agent.UPSPath, nil)
	if err != nil {
		return ups.Status{}, err
	}
	resp, err := e.client.Do(req)
	if err != nil {
		return ups.Status{}, errors.New("Agent is unavailable")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "application/json" {
		return ups.Status{}, errors.New("Agent rejected ups request")
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, UPSMaxBytes+1))
	if err != nil {
		return ups.Status{}, errors.New("Agent response read failed")
	}
	var st ups.Status
	if err := json.Unmarshal(data, &st); err != nil {
		return ups.Status{}, errors.New("ups decode failed")
	}
	if err := st.Topology.Valid(); err != nil && len(st.Topology.Devices) > 0 {
		return ups.Status{}, errors.New("ups validation failed")
	}
	return st, nil
}

// UPSProbe asks the Agent to run an on-demand
// probe of every UPS device. The Engine surfaces
// the Agent's outcome; the API and CLI never see
// the raw `upsc` error.
func (e *Engine) UPSProbe(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://agent"+agent.UPSProbePath, nil)
	if err != nil {
		return err
	}
	resp, err := e.client.Do(req)
	if err != nil {
		return errors.New("Agent is unavailable")
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNoContent {
		return nil
	}
	if resp.StatusCode == http.StatusServiceUnavailable {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, UPSMaxBytes))
		var payload struct {
			Error map[string]string `json:"error"`
		}
		if json.Unmarshal(data, &payload) == nil {
			if msg, ok := payload.Error["message"]; ok && msg != "" {
				return errors.New("ups: " + msg)
			}
		}
		return errors.New("ups: agent returned an error")
	}
	return fmt.Errorf("ups: unexpected status %d", resp.StatusCode)
}

// MigrateMaxBytes is the Engine IPC cap for
// migration payloads. The cap matches the
// Agent IPC cap; the Engine never inflates it.
const MigrateMaxBytes = agent.MigrateMaxBytes

// Migrate submits a migration source to the
// Agent. The Engine forwards the body
// verbatim; the Agent owns the source-size
// gate. The returned Plan is the bounded,
// validated contract the API surfaces to the
// GUI/CLI.
func (e *Engine) Migrate(ctx context.Context, src migrate.Source, body []byte) (migrate.Plan, error) {
	if len(body) > migrate.MaxSourceBytes {
		return migrate.Plan{}, errors.New("migrate: source exceeds engine cap")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://agent"+agent.MigratePath, bytes.NewReader(body))
	if err != nil {
		return migrate.Plan{}, err
	}
	if src.Filename != "" {
		req.Header.Set("X-DuckOS-Migrate-Filename", src.Filename)
	}
	if src.Format != "" && src.Format != migrate.FormatUnknown {
		req.Header.Set("X-DuckOS-Migrate-Format", string(src.Format))
	}
	if src.Provenance != "" {
		req.Header.Set("X-DuckOS-Migrate-Provenance", src.Provenance)
	}
	resp, err := e.client.Do(req)
	if err != nil {
		return migrate.Plan{}, errors.New("Agent is unavailable")
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, MigrateMaxBytes))
	if err != nil {
		return migrate.Plan{}, errors.New("Agent response read failed")
	}
	if resp.StatusCode == http.StatusOK {
		return migrate.ParsePlan(data)
	}
	// structured error
	var payload struct {
		Error map[string]string `json:"error"`
	}
	if json.Unmarshal(data, &payload) == nil {
		if msg, ok := payload.Error["message"]; ok && msg != "" {
			return migrate.Plan{}, errors.New("migrate: " + msg)
		}
	}
	return migrate.Plan{}, fmt.Errorf("migrate: unexpected status %d", resp.StatusCode)
}

// SitesMaxBytes is the Engine IPC cap for sites
// payloads. The cap matches the Agent IPC cap;
// the Engine never inflates it.
const SitesMaxBytes = agent.SitesMaxBytes

// Sites fetches the read-only sites state from
// the Agent. The state is the topology, the
// per-site health and the last probe timestamp.
func (e *Engine) Sites(ctx context.Context) (sites.Status, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://agent"+agent.SitesPath, nil)
	if err != nil {
		return sites.Status{}, err
	}
	resp, err := e.client.Do(req)
	if err != nil {
		return sites.Status{}, errors.New("Agent is unavailable")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "application/json" {
		return sites.Status{}, errors.New("Agent rejected sites request")
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, SitesMaxBytes+1))
	if err != nil {
		return sites.Status{}, errors.New("Agent response read failed")
	}
	var st sites.Status
	if err := json.Unmarshal(data, &st); err != nil {
		return sites.Status{}, errors.New("sites decode failed")
	}
	if err := st.Topology.Valid(); err != nil && len(st.Topology.Sites) > 0 {
		return sites.Status{}, errors.New("sites validation failed")
	}
	return st, nil
}

// AnycastMaxBytes is the Engine IPC cap for anycast
// responses; the Engine never inflates it past the Agent.
const AnycastMaxBytes = agent.AnycastMaxBytes

// Anycast fetches the read-only anycast view from the
// Agent. The view is the per-VIP evidence (desired,
// health, advertisement) plus the bounded counters.
// The Engine never invents availability: a 503 from the
// Agent is propagated as 'Agent is unavailable'.
func (e *Engine) Anycast(ctx context.Context) (agent.AnycastStatus, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://agent"+agent.AnycastPath, nil)
	if err != nil {
		return agent.AnycastStatus{}, err
	}
	resp, err := e.client.Do(req)
	if err != nil {
		return agent.AnycastStatus{}, errors.New("Agent is unavailable")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "application/json" {
		return agent.AnycastStatus{}, errors.New("Agent rejected anycast request")
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, AnycastMaxBytes+1))
	if err != nil {
		return agent.AnycastStatus{}, errors.New("Agent response read failed")
	}
	var st agent.AnycastStatus
	if err := json.Unmarshal(data, &st); err != nil {
		return agent.AnycastStatus{}, errors.New("anycast decode failed")
	}
	if st.Version != 1 {
		return agent.AnycastStatus{}, errors.New("anycast schema mismatch")
	}
	return st, nil
}

// WifiMaxBytes is the Engine IPC cap for Wi-Fi
// payloads.
const WifiMaxBytes = agent.WifiMaxBytes

// Wifi fetches the read-only Wi-Fi state from
// the Agent. The state is the topology, the
// per-AP health, the per-AP station count,
// the host capability and the last probe
// timestamp.
func (e *Engine) Wifi(ctx context.Context) (wifi.Status, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://agent"+agent.WifiPath, nil)
	if err != nil {
		return wifi.Status{}, err
	}
	resp, err := e.client.Do(req)
	if err != nil {
		return wifi.Status{}, errors.New("Agent is unavailable")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "application/json" {
		return wifi.Status{}, errors.New("Agent rejected wifi request")
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, WifiMaxBytes+1))
	if err != nil {
		return wifi.Status{}, errors.New("Agent response read failed")
	}
	var st wifi.Status
	if err := json.Unmarshal(data, &st); err != nil {
		return wifi.Status{}, errors.New("wifi decode failed")
	}
	if err := st.Topology.Valid(); err != nil && len(st.Topology.APs) > 0 {
		return wifi.Status{}, errors.New("wifi validation failed")
	}
	return st, nil
}

// Upgrade proxies the public /api/v1/upgrade read to the
// privileged Agent /agent/v1/upgrade IPC. The Engine is
// the only public caller; CLI commands reach the same
// boundary through /api/v1/upgrade, not via the direct
// Agent socket. B06 closes when this method exists and
// the CLI route uses it.
func (e *Engine) Upgrade(ctx context.Context) ([]byte, error) {
	if e == nil {
		return nil, errors.New("engine not configured")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://agent"+agent.UpgradePath, nil)
	if err != nil {
		return nil, err
	}
	resp, err := e.client.Do(req)
	if err != nil {
		return nil, errors.New("Agent is unavailable")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "application/json" {
		return nil, fmt.Errorf("Agent rejected upgrade request: HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, (1<<20)+1))
	if err != nil {
		return nil, errors.New("Agent upgrade response read failed")
	}
	if len(data) > 1<<20 {
		return nil, errors.New("Agent upgrade response exceeds size limit")
	}
	return data, nil
}

// Backup proxies the public /api/v1/backup read to the
// privileged Agent /agent/v1/backup IPC. The optional
// id selects a single backup plan; the empty id returns
// the catalog. B06 closes when this method exists and
// the CLI route uses it.
func (e *Engine) Backup(ctx context.Context, id string) ([]byte, error) {
	if e == nil {
		return nil, errors.New("engine not configured")
	}
	target := "http://agent" + agent.BackupPath
	if id != "" {
		target += "?id=" + backupIDEscape(id)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	resp, err := e.client.Do(req)
	if err != nil {
		return nil, errors.New("Agent is unavailable")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "application/json" {
		return nil, fmt.Errorf("Agent rejected backup request: HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, (1<<20)+1))
	if err != nil {
		return nil, errors.New("Agent backup response read failed")
	}
	if len(data) > 1<<20 {
		return nil, errors.New("Agent backup response exceeds size limit")
	}
	return data, nil
}

// backupIDEscape is a tiny RFC 3986 query-escape helper
// that the engine package can use without pulling in
// net/url for a single call site.
func backupIDEscape(s string) string {
	const hex = "0123456789ABCDEF"
	var b []byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_' || c == '.' || c == '~' {
			b = append(b, c)
		} else {
			b = append(b, '%', hex[c>>4], hex[c&0xF])
		}
	}
	return string(b)
}

// BackupRestoreMaxBytes is the Engine IPC cap for the
// restore request + response payload.
const BackupRestoreMaxBytes = 64 * 1024

// BackupRestore proxies a public /api/v1/backup/restore
// request to the privileged Agent
// /agent/v1/backup/restore IPC. The Engine carries the
// canonical payload (backupId + source/live revision +
// selected keys + roots) verbatim; the Agent's restore
// host owns the lifecycle. DV08 production-path: the
// Engine never invents availability — a nil Engine or
// an unreachable Agent surfaces a typed error envelope.
var ErrRestoreRefused = errors.New("restore refused")
var ErrInvalidRestore = errors.New("invalid restore request")

func (e *Engine) BackupRestore(ctx context.Context, payload []byte) ([]byte, error) {
	return e.restoreOperation(ctx, payload, false)
}
func (e *Engine) restoreOperation(ctx context.Context, payload []byte, reconcile bool) ([]byte, error) {
	if e == nil {
		return nil, errors.New("engine not configured")
	}
	if len(payload) > BackupRestoreMaxBytes {
		return nil, errors.New("Backup restore payload exceeds the Engine cap")
	}
	e.restoreMu.Lock()
	defer e.restoreMu.Unlock()
	if e.config == nil {
		return nil, errors.New("restore requires authoritative Engine config store")
	}
	var operation restore.Request
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&operation) != nil || decoder.Decode(new(any)) != io.EOF {
		return nil, ErrInvalidRestore
	}
	if err := operation.Validate(); err != nil {
		return nil, ErrInvalidRestore
	}
	canonical, _ := json.Marshal(operation)
	if err := e.config.store.BeginRestore(operation.ID, operation.Expected, operation.Digest(), canonical); err != nil {
		return nil, err
	}
	endpoint := agent.BackupRestorePath
	if reconcile {
		endpoint += "/reconcile"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"http://agent"+endpoint,
		bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := e.client.Do(req)
	if err != nil {
		return nil, errors.New("Agent is unavailable")
	}
	defer resp.Body.Close()
	if (resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusUnprocessableEntity) || resp.Header.Get("Content-Type") != "application/json" {
		return nil, fmt.Errorf("Agent rejected restore request: HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, BackupRestoreMaxBytes+1))
	if err != nil {
		return nil, errors.New("Agent restore response read failed")
	}
	if len(data) > BackupRestoreMaxBytes {
		return nil, errors.New("Agent restore response exceeds size limit")
	}
	var result restore.Result
	if json.Unmarshal(data, &result) != nil || result.ID != operation.ID || (result.Outcome != "applied" && result.Outcome != "rolled-back" && result.Outcome != "rejected") {
		return nil, errors.New("restore outcome uncertain")
	}
	if err := e.config.store.FinishRestore(operation.ID, operation.Digest()); err != nil {
		return nil, err
	}
	if result.Outcome != "applied" {
		return nil, ErrRestoreRefused
	}
	return data, nil
}

// WireGuardMaxBytes is the Engine IPC cap for WireGuard
// payloads (server / peer JSON + the rendered .conf).
const WireGuardMaxBytes = 1 << 20

// UpgradeStageMaxBytes is the Engine IPC cap for the
// signed-update Stage payload. The orchestrator's
// MaxPayloadBytes is the authoritative cap; this is the
// per-request IPC guard.
const UpgradeStageMaxBytes = 64 << 20

// UpgradeStage proxies the public /api/v1/upgrade/stage
// request to the privileged Agent
// /agent/v1/upgrade/stage IPC. DV07 production-path.
func (e *Engine) UpgradeStage(ctx context.Context, payload []byte) ([]byte, error) {
	return e.upgradeProxy(ctx, agent.UpgradeStagePath, payload)
}

// UpgradeActivate proxies the public
// /api/v1/upgrade/activate request to the Agent
// /agent/v1/upgrade/activate IPC.
func (e *Engine) UpgradeActivate(ctx context.Context, payload []byte) ([]byte, error) {
	return e.upgradeProxy(ctx, agent.UpgradeActivatePath, payload)
}

// UpgradeRollback proxies the public
// /api/v1/upgrade/rollback request to the Agent
// /agent/v1/upgrade/rollback IPC.
func (e *Engine) UpgradeRollback(ctx context.Context, payload []byte) ([]byte, error) {
	return e.upgradeProxy(ctx, agent.UpgradeRollbackPath, payload)
}

// FRRApplyMaxBytes is the Engine IPC cap for the
// FRR apply payload (canonical Config JSON). The
// frr.ApplyMaxBytes is the per-request Agent cap;
// this is the Engine-to-Agent bridge cap.
const FRRApplyMaxBytes = 1 << 20

// ZTPMaxBytes is the Engine IPC cap for issuance and
// redemption payloads.
const ZTPMaxBytes = agent.ZTPMaxBytes

// FRR proxies the public /api/v1/frr GET to the
// privileged Agent /agent/v1/frr IPC.
func (e *Engine) FRR(ctx context.Context) ([]byte, error) {
	if e == nil {
		return nil, errors.New("engine not configured")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://agent"+frr.FRRPath, nil)
	if err != nil {
		return nil, err
	}
	resp, err := e.client.Do(req)
	if err != nil {
		return nil, errors.New("Agent is unavailable")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "application/json" {
		return nil, fmt.Errorf("Agent rejected FRR request: HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, FRRApplyMaxBytes+1))
	if err != nil {
		return nil, errors.New("Agent FRR response read failed")
	}
	if len(data) > FRRApplyMaxBytes {
		return nil, errors.New("Agent FRR response exceeds size limit")
	}
	return data, nil
}

// FRRApply proxies the public /api/v1/frr/apply POST
// to the Agent /agent/v1/frr/apply IPC. DV09
// production-path.
func (e *Engine) FRRApply(ctx context.Context, payload []byte) ([]byte, error) {
	return e.frrProxy(ctx, frr.FRRApplyPath, payload)
}

// FRRRollback proxies the public /api/v1/frr/rollback
// POST to the Agent /agent/v1/frr/rollback IPC.
func (e *Engine) FRRRollback(ctx context.Context, payload []byte) ([]byte, error) {
	return e.frrProxy(ctx, frr.FRRRollbackPath, payload)
}

func (e *Engine) ZTPIssue(ctx context.Context, payload []byte) ([]byte, error) {
	return e.ztpProxy(ctx, agent.ZTPIssuePath, payload)
}

func (e *Engine) ZTPRedeem(ctx context.Context, payload []byte) ([]byte, error) {
	return e.ztpProxy(ctx, agent.ZTPPath, payload)
}

func (e *Engine) ztpProxy(ctx context.Context, path string, payload []byte) ([]byte, error) {
	if e == nil {
		return nil, errors.New("engine not configured")
	}
	if len(payload) > ZTPMaxBytes {
		return nil, errors.New("ZTP payload exceeds the Engine cap")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://agent"+path, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := e.client.Do(req)
	if err != nil {
		return nil, errors.New("Agent is unavailable")
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, int64(ZTPMaxBytes)+1))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 || resp.Header.Get("Content-Type") != "application/json" {
		return nil, fmt.Errorf("Agent rejected ZTP request: HTTP %d", resp.StatusCode)
	}
	if len(body) > ZTPMaxBytes {
		return nil, errors.New("Agent ZTP response exceeds size limit")
	}
	return body, nil
}

// frrProxy is the shared implementation for the
// FRR POST routes. Cap is FRRApplyMaxBytes (1 MiB).
// The non-200 response body is included in the error
// envelope so callers (and tests) can see why the
// Agent refused the request.
func (e *Engine) frrProxy(ctx context.Context, path string, payload []byte) ([]byte, error) {
	if e == nil {
		return nil, errors.New("engine not configured")
	}
	if len(payload) > FRRApplyMaxBytes {
		return nil, errors.New("FRR payload exceeds the Engine cap")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"http://agent"+path,
		bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := e.client.Do(req)
	if err != nil {
		return nil, errors.New("Agent is unavailable")
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, int64(FRRApplyMaxBytes)+1))
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "application/json" {
		return nil, fmt.Errorf("Agent rejected FRR request: HTTP %d body=%s", resp.StatusCode, string(body))
	}
	if len(body) > FRRApplyMaxBytes {
		return nil, errors.New("Agent FRR response exceeds size limit")
	}
	return body, nil
}

// upgradeProxy is the shared implementation for the
// three POST routes (stage / activate / rollback).
// stage has a larger cap; activate + rollback share the
// 1 MiB cap.
func (e *Engine) upgradeProxy(ctx context.Context, path string, payload []byte) ([]byte, error) {
	if e == nil {
		return nil, errors.New("engine not configured")
	}
	cap := 1 << 20
	if path == agent.UpgradeStagePath {
		cap = UpgradeStageMaxBytes
	}
	if len(payload) > cap {
		return nil, errors.New("Upgrade payload exceeds the Engine cap")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"http://agent"+path,
		bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := e.client.Do(req)
	if err != nil {
		return nil, errors.New("Agent is unavailable")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "application/json" {
		return nil, fmt.Errorf("Agent rejected upgrade request: HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, int64(cap)+1))
	if err != nil {
		return nil, errors.New("Agent upgrade response read failed")
	}
	if len(data) > cap {
		return nil, errors.New("Agent upgrade response exceeds size limit")
	}
	return data, nil
}

// WireGuardProxy proxies a public /api/v1/wireguard/*
// mutation to the privileged Agent
// /agent/v1/wireguard/wireguard/v1/* IPC. The Engine
// is the canonical bridge: it owns the Unix socket
// client, the size cap and the typed-error envelope.
//
// DV04-WG production-path contract:
//
//	PUBLIC API -> Engine.WireGuardProxy -> Agent IPC
//	              -> WireguardHost -> wireguard.API ->
//	              KernelAdapter -> /sbin/ip + /usr/bin/wg
//
// The method is the single point that maps a public
// request (method + path + body + token) to the Agent
// loopback. Errors propagate as-is so the public API
// can surface them in the same envelope shape the Agent
// returned (no rewriting of code/message).
func (e *Engine) WireGuardProxy(ctx context.Context, method, path, token string, body io.Reader) (status int, data []byte, contentType string, err error) {
	if e == nil {
		return 0, nil, "", errors.New("engine not configured")
	}
	// The Agent IPC path is /agent/v1/wireguard/wireguard/v1/{tail}
	// (the host preserves the wireguard.API prefix so the
	// Engine path mirrors the wireguard.API contract).
	tail := strings.TrimPrefix(path, "/api/v1/wireguard/")
	if tail == "" {
		tail = path
	}
	target := "http://agent" + agent.WireguardPath + "/wireguard/v1/" + strings.TrimPrefix(tail, "/")
	req, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return 0, nil, "", err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := e.client.Do(req)
	if err != nil {
		return 0, nil, "", errors.New("Agent is unavailable")
	}
	defer resp.Body.Close()
	data, err = io.ReadAll(io.LimitReader(resp.Body, WireGuardMaxBytes+1))
	if err != nil {
		return resp.StatusCode, nil, resp.Header.Get("Content-Type"), errors.New("Agent WireGuard response read failed")
	}
	if len(data) > WireGuardMaxBytes {
		return resp.StatusCode, nil, resp.Header.Get("Content-Type"), errors.New("Agent WireGuard response exceeds size limit")
	}
	return resp.StatusCode, data, resp.Header.Get("Content-Type"), nil
}

// XrayMaxBytes is the Engine IPC cap for Xray payloads
// (server / user / profile / subscription JSON + the
// rendered base64 subscription URL).
const XrayMaxBytes = 1 << 20

// XrayProxy proxies a public /api/v1/xray/* mutation
// to the privileged Agent /agent/v1/xray/xray/v1/*
// IPC.
//
// DV04-XRAY production-path contract:
//
//	PUBLIC API -> Engine.XrayProxy -> Agent IPC
//	           -> XrayHost -> xray.API ->
//	           RenderConfig + Runtime.Apply ->
//	           /tmp/duckos-test/xray/xray run -c <path>
func (e *Engine) XrayProxy(ctx context.Context, method, path, token string, body io.Reader) (status int, data []byte, contentType string, err error) {
	if e == nil {
		return 0, nil, "", errors.New("engine not configured")
	}
	tail := strings.TrimPrefix(path, "/api/v1/xray/")
	if tail == "" {
		tail = path
	}
	target := "http://agent" + agent.XrayPath + "/xray/v1/" + strings.TrimPrefix(tail, "/")
	req, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return 0, nil, "", err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := e.client.Do(req)
	if err != nil {
		return 0, nil, "", errors.New("Agent is unavailable")
	}
	defer resp.Body.Close()
	data, err = io.ReadAll(io.LimitReader(resp.Body, XrayMaxBytes+1))
	if err != nil {
		return resp.StatusCode, nil, resp.Header.Get("Content-Type"), errors.New("Agent Xray response read failed")
	}
	if len(data) > XrayMaxBytes {
		return resp.StatusCode, nil, resp.Header.Get("Content-Type"), errors.New("Agent Xray response exceeds size limit")
	}
	return resp.StatusCode, data, resp.Header.Get("Content-Type"), nil
}

func (e *Engine) overview(ctx context.Context, force bool) ([]byte, error) {
	for {
		e.mu.Lock()
		if !force && e.cached != nil && time.Now().Before(e.expires) {
			data := append([]byte(nil), e.cached...)
			e.mu.Unlock()
			return data, nil
		}
		if wait := e.inflight; wait != nil {
			e.mu.Unlock()
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-wait:
			}
			continue
		}
		e.inflight = make(chan struct{})
		e.mu.Unlock()
		data, err := e.fetch(ctx)
		e.mu.Lock()
		if err == nil {
			e.cached = append([]byte(nil), data...)
			e.expires = time.Now().Add(e.ttl)
		} else {
			e.cached = nil
		}
		close(e.inflight)
		e.inflight = nil
		e.mu.Unlock()
		return data, err
	}
}
func (e *Engine) fetch(ctx context.Context) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://agent"+agent.OverviewPath, nil)
	if err != nil {
		return nil, err
	}
	resp, err := e.client.Do(req)
	if err != nil {
		return nil, errors.New("Agent is unavailable")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "application/json" {
		return nil, errors.New("Agent rejected observation")
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, model.MaxResponseBytes+1))
	if err != nil {
		return nil, errors.New("Agent response read failed")
	}
	if _, err = model.DecodeOverview(data, e.environment, time.Now()); err != nil {
		return nil, errors.New("Agent returned an invalid observation")
	}
	return data, nil
}
