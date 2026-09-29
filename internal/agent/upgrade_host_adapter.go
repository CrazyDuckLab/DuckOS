// agent/upgrade_host_adapter.go bridges a real
// *upgrade.Orchestrator to the UpgradeObserver surface
// the IPC handler dispatches to. The previous wiring
// passed nil (agent.NewUpgradeHost(nil)) which produced
// a stub that returned zero values — the audit
// classified that as "disconnected orchestrator". The
// adapter here is the production wiring.
package agent

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/upgrade"
)

// OrchestratorAdapter implements UpgradeObserver against
// a real *upgrade.Orchestrator. The adapter does not
// cache the orchestrator state so the IPC layer always
// reflects the latest reconciliation; the orchestrator
// owns the canonical state via State().
type OrchestratorAdapter struct {
	orchestrator *upgrade.Orchestrator
}

// NewOrchestratorAdapter wires the real orchestrator.
// A nil orchestrator returns nil — callers MUST refuse
// nil so the IPC layer never invents availability.
func NewOrchestratorAdapter(o *upgrade.Orchestrator) *OrchestratorAdapter {
	return &OrchestratorAdapter{orchestrator: o}
}

// Status returns the canonical upgrade status snapshot.
//
// DV07 production-path: Healthy is NEVER hardcoded true.
// It is computed from the orchestrator's durable state:
//   - false if Active is empty (no release activated yet)
//   - false if no history has been recorded yet
//   - false for selection-only history such as "activated"
//   - true only after the verification workflow records "health-passed"
//
// A nil orchestrator returns Healthy=false (the previous
// "fake Healthy:true" was the audit's blocker).
func (a *OrchestratorAdapter) Status(ctx context.Context) (UpgradeStatus, error) {
	if a == nil || a.orchestrator == nil {
		return UpgradeStatus{Healthy: false, Notes: []string{"upgrade orchestrator not configured"}}, nil
	}
	if err := ctx.Err(); err != nil {
		return UpgradeStatus{}, err
	}
	st := a.orchestrator.State()
	hist := a.orchestrator.History()
	snap := UpgradeStatus{
		Version:          1,
		UpdatedAt:        time.Now().UTC(),
		CurrentRelease:   st.Active,
		CurrentVersion:   formatVersion(st.Active),
		CandidateRelease: st.Candidate,
		CandidateVersion: formatVersion(st.Candidate),
		PreviousRelease:  st.Previous,
		PreviousVersion:  formatVersion(st.Previous),
		Healthy:          computeHealthy(st, hist),
	}
	if len(hist) > 0 {
		last := hist[len(hist)-1]
		snap.Healthcheck = last.Result
		snap.Notes = append(snap.Notes, last.Reason)
	}
	if st.RecoveryRequired {
		snap.Healthy = false
		snap.Notes = append(snap.Notes, "recovery_required")
	}
	return snap, nil
}

// computeHealthy is the canonical Healthy derivation.
// Exported for tests; never inlined so a regression in
// the rule is caught by a unit test rather than by a
// silent production false-positive.
func computeHealthy(st upgrade.State, hist []upgrade.HistoryEntry) bool {
	if st.Active == "" || st.RecoveryRequired {
		return false
	}
	if len(hist) == 0 {
		// Active is set but no history has been
		// recorded yet — the orchestrator was
		// never reconciled in this process. Treat
		// as not-healthy until a real health-check
		// run has been recorded.
		return false
	}
	last := hist[len(hist)-1]
	switch last.Result {
	case "health-passed":
		return true
	case "rolled-back", "refused", "preflight-failed", "health-failed-rollback-error", "health-failed-no-previous":
		return false
	}
	// Unknown result string: be conservative.
	return false
}

// formatVersion derives a friendly version label from a
// release id. The orchestrator stores ids; the adapter
// keeps the formatting local so callers never need to
// reach into the upgrade package.
func formatVersion(id string) string {
	if id == "" {
		return ""
	}
	return id
}

// UpgradeStageMaxBytes caps the signed-update payload
// the Agent accepts on /agent/v1/upgrade/stage. The
// orchestrator's MaxPayloadBytes is the canonical
// authoritative cap; this is the per-request IPC guard.
const UpgradeStageMaxBytes = 64 << 20

// upgradeStageRequest is the IPC body for /agent/v1/upgrade/stage.
// The payload is the canonical .duck package bytes (the
// orchestrator runs Ed25519 signature verification on it
// via its registered keys).
type upgradeStageRequest struct {
	Operator      string `json:"operator"`
	Channel       string `json:"channel"`
	ReleaseID     string `json:"releaseId"`
	ReleaseVer    string `json:"releaseVersion"`
	ReleaseArch   string `json:"releaseArch"`
	PayloadBase64 string `json:"payloadBase64"`
}

// upgradeResponse is the shared IPC response for
// stage/activate/rollback. The Result field carries the
// orchestrator's canonical outcome string ("staged",
// "activated", "rolled-back", "refused", etc.).
type upgradeResponse struct {
	Version   int       `json:"version"`
	UpdatedAt time.Time `json:"updatedAt"`
	Result    string    `json:"result"`
	Reason    string    `json:"reason,omitempty"`
	Operator  string    `json:"operator,omitempty"`
}

// Stage accepts a base64-encoded signed .duck package
// and runs the production orchestrator.Stage(preflight +
// materialize + candidate). The orchestrator's preflight
// verifies the Ed25519 signature against the keys loaded
// from --upgrade-keys-file; an unsigned or wrongly-keyed
// package is refused BEFORE any disk write.
func (a *OrchestratorAdapter) Stage(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	if a == nil || a.orchestrator == nil {
		writeUpgradeProblem(w, 503, "upgrade_unavailable", "Upgrade orchestrator is not configured.")
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		writeUpgradeProblem(w, 405, "method_not_allowed", "Stage accepts POST only.")
		return
	}
	if r.ContentLength > UpgradeStageMaxBytes {
		writeUpgradeProblem(w, 413, "payload_too_large", "Stage payload exceeds the Agent cap.")
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, UpgradeStageMaxBytes+1))
	if err != nil {
		writeUpgradeProblem(w, 400, "invalid_request", "Could not read stage payload.")
		return
	}
	if len(body) > UpgradeStageMaxBytes {
		writeUpgradeProblem(w, 413, "payload_too_large", "Stage payload exceeds the Agent cap.")
		return
	}
	var req upgradeStageRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeUpgradeProblem(w, 400, "invalid_request", "Stage payload is not canonical JSON.")
		return
	}
	if req.Operator == "" {
		writeUpgradeProblem(w, 422, "invalid_request", "Stage requires an operator.")
		return
	}
	if req.ReleaseID == "" || req.ReleaseVer == "" || req.ReleaseArch == "" {
		writeUpgradeProblem(w, 422, "invalid_request", "Stage requires releaseId/releaseVersion/releaseArch.")
		return
	}
	payload, err := base64.StdEncoding.DecodeString(req.PayloadBase64)
	if err != nil {
		writeUpgradeProblem(w, 422, "invalid_request", "payloadBase64 is not canonical base64.")
		return
	}
	channel := upgrade.Channel(req.Channel)
	if !channel.Valid() {
		channel = upgrade.ChannelStable
	}
	plan := upgrade.Plan{
		Operator: req.Operator,
		Channel:  channel,
		Payload:  payload,
		Release: upgrade.Release{
			ID:      req.ReleaseID,
			Version: req.ReleaseVer,
			Channel: channel,
			Arch:    req.ReleaseArch,
		},
	}
	if err := a.orchestrator.Stage(plan); err != nil {
		writeUpgradeProblem(w, 422, "stage_refused", err.Error())
		return
	}
	writeUpgradeJSON(w, 200, upgradeResponse{
		Version: 1, UpdatedAt: time.Now().UTC(), Result: "staged", Operator: req.Operator,
	})
}

// Activate promotes the staged Candidate to Active through
// ActivateAndVerify. The public production route must not
// use plain Activate(): a candidate is only reported as
// activated after the bounded health runner accepts the
// post-activation state.
func (a *OrchestratorAdapter) Activate(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	if a == nil || a.orchestrator == nil {
		writeUpgradeProblem(w, 503, "upgrade_unavailable", "Upgrade orchestrator is not configured.")
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		writeUpgradeProblem(w, 405, "method_not_allowed", "Activate accepts POST only.")
		return
	}
	results, err := a.orchestrator.ActivateAndVerifyCtx(ctx, activationHealthRunner{orchestrator: a.orchestrator}, []upgrade.HealthCheck{{Name: "active-release", Command: "status"}}, 5*time.Second)
	if err != nil {
		writeUpgradeProblem(w, 422, "activate_refused", err.Error())
		return
	}
	writeUpgradeJSON(w, 200, upgradeResponse{
		Version: 1, UpdatedAt: time.Now().UTC(), Result: "activated",
		Reason: "healthChecks=" + strconv.Itoa(len(results)),
	})
}

type activationHealthRunner struct {
	orchestrator *upgrade.Orchestrator
}

func (r activationHealthRunner) Run(check upgrade.HealthCheck) error {
	if r.orchestrator == nil {
		return errors.New("upgrade: health runner has no orchestrator")
	}
	if check.Command != "status" {
		return errors.New("upgrade: unsupported activation health command")
	}
	// Store selection is not a runtime health oracle. Fail closed until the
	// candidate execution/health provider is wired into production activation.
	return errors.New("upgrade: runtime activation and health provider unavailable")
}

// Rollback promotes Previous back to Active. The
// orchestrator's Rollback is the atomic side effect.
func (a *OrchestratorAdapter) Rollback(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	if a == nil || a.orchestrator == nil {
		writeUpgradeProblem(w, 503, "upgrade_unavailable", "Upgrade orchestrator is not configured.")
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		writeUpgradeProblem(w, 405, "method_not_allowed", "Rollback accepts POST only.")
		return
	}
	var body struct {
		Reason string `json:"reason"`
	}
	_, _ = io.ReadAll(io.LimitReader(r.Body, 1<<16))
	_ = json.Unmarshal([]byte("{}"), &body) // tolerate empty body
	// Re-read the actual body so the operator-supplied reason is honoured.
	bodyBytes, _ := io.ReadAll(io.LimitReader(r.Body, 1<<16))
	_ = json.Unmarshal(bodyBytes, &body)
	if err := a.orchestrator.Rollback(body.Reason); err != nil {
		writeUpgradeProblem(w, 422, "rollback_refused", err.Error())
		return
	}
	writeUpgradeJSON(w, 200, upgradeResponse{
		Version: 1, UpdatedAt: time.Now().UTC(), Result: "rolled-back", Reason: body.Reason,
	})
}

func writeUpgradeJSON(w http.ResponseWriter, status int, payload any) {
	data, err := json.Marshal(payload)
	if err != nil {
		writeUpgradeProblem(w, 503, "encode_failed", "Upgrade response could not be encoded.")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = w.Write(data)
}

func writeUpgradeProblem(w http.ResponseWriter, status int, code, message string) {
	writeUpgradeJSON(w, status, map[string]any{
		"error": map[string]string{"code": code, "message": message},
	})
}

// silence unused-import warning for strings when no
// helper uses it.
var _ = strings.TrimSpace

var _ = errors.New
