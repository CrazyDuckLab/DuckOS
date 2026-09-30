package agent

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/CrazyDuckLab/DuckOS/internal/firstrun"
	"github.com/CrazyDuckLab/DuckOS/internal/lifecycle"
	"github.com/CrazyDuckLab/DuckOS/internal/model"
	"net/http"
	"strings"
	"time"
)

const OverviewPath = "/agent/v1/overview"

type Observer interface {
	Overview(context.Context) (model.Overview, error)
}
type Handler struct {
	observer   Observer
	doctor     DoctorObserver
	topology   TopologyObserver
	multiwan   MultiWANObserver
	qos        QoSObservable
	ups        UPSObservable
	migrator   MigrateObservable
	sitesHost  SitesObservable
	wifiHost   WifiObservable
	readiness  ReadinessObserver
	provenance ProvenanceObserver
	firstrun   FirstRunObserver
	diagjobs   DiagJobObserver
	upgrade    UpgradeObserver
	backup     BackupObserver
	restore    interface {
		ServeRestore(http.ResponseWriter, *http.Request)
	}
	anycast   *AnycastHost
	ztp       *ZTPHost
	linuxObjs *LinuxObjectsHost
	wireguard *WireguardHost
	xray      *XrayHost
	vault     *VaultHost
	frr       *FRRHost
	slots     chan struct{}
	network   *NetworkService
}

func NewHandler(observer Observer) *Handler {
	return &Handler{observer: observer, slots: make(chan struct{}, 2)}
}

// EnableDoctor registers the Doctor source. The handler keeps a
// separate reference so an Observer that does not implement the
// DoctorObserver interface can still serve the Doctor endpoint
// when a privileged Agent has its own Doctor host.
func (h *Handler) EnableDoctor(host DoctorObserver) {
	h.doctor = host
}

// EnableTopology registers the Topology host. The handler keeps a
// separate reference so a non-Topology Observer can still serve
// the topology endpoint through a dedicated host.
func (h *Handler) EnableTopology(host TopologyObserver) {
	h.topology = host
}

// EnableMultiWAN registers the MultiWAN host. The handler keeps
// a separate reference so a non-MultiWAN Observer can still
// serve the multi-WAN endpoint through a dedicated host.
func (h *Handler) EnableMultiWAN(host MultiWANObserver) {
	h.multiwan = host
}

// EnableQoS registers the QoS host. The host is
// both the read-only source and the apply/rollback
// surface; the handler keeps a single reference so
// the IPC paths share the same state.
func (h *Handler) EnableQoS(host QoSObservable) {
	h.qos = host
}

// EnableUPS registers the UPS host. The host is
// the read-only source and the on-demand probe
// surface; the handler keeps a single reference
// so the IPC paths share the same state.
func (h *Handler) EnableUPS(host UPSObservable) {
	h.ups = host
}

// EnableMigrate registers the migration host.
// The migration surface is read-only: the
// analyser never executes the source commands
// and never mutates host state.
func (h *Handler) EnableMigrate(host MigrateObservable) {
	h.migrator = host
}

// EnableSites registers the sites host. The
// sites surface is read-only in the foundation;
// the Agent returns the bounded Topology, the
// per-site Health and the last probe timestamp.
func (h *Handler) EnableSites(host SitesObservable) {
	h.sitesHost = host
}

// EnableWifi registers the Wi-Fi host. The
// Wi-Fi surface is read-only in the foundation;
// the Agent returns the bounded Topology, the
// per-AP health, the host capability and the
// last probe timestamp.
func (h *Handler) EnableWifi(host WifiObservable) {
	h.wifiHost = host
}

// EnableReadiness registers the readiness host (G04 agent
// write side). The recorder is run by the agent composition
// root during boot; the host only exposes the recorded
// snapshot over IPC.
func (h *Handler) EnableReadiness(host ReadinessObserver) {
	h.readiness = host
}

// EnableProvenance registers the ownership/provenance
// host (G08). The host wraps the lifecycle graph so the
// same dependency/ownership/provenance model is shared
// across Agent, API and CLI.
func (h *Handler) EnableProvenance(host ProvenanceObserver) {
	h.provenance = host
}

// EnableFirstRun registers the first-run persistence
// host (G18). The host exposes the on-disk status over
// IPC so the API and CLI can render an honest pending /
// completed badge. Writes remain privileged.
func (h *Handler) EnableFirstRun(host FirstRunObserver) {
	h.firstrun = host
}

// EnableDiagJobs registers the contextual diagnostic job
// host (G19). The store is owned by the Agent composition
// root; the API and CLI consume the read-only IPC view.
func (h *Handler) EnableDiagJobs(host DiagJobObserver) {
	h.diagjobs = host
}

// EnableUpgrade registers the upgrade activation host
// (G16). The host exposes a read-only status over IPC so
// the API and CLI can render an honest current/staged/
// previous release view. Materialize / Activate remain
// Agent-privileged.
func (h *Handler) EnableUpgrade(host UpgradeObserver) {
	h.upgrade = host
}

// EnableBackup registers the portable/selective restore
// bridge (G17). The host exposes a read-only backup
// catalog + restore plan over IPC; actual restore
// execution remains Agent-privileged.
func (h *Handler) EnableBackup(host BackupObserver) {
	h.backup = host
}

// EnableFRR registers the production FRR runtime
// (DV09 production-path). The host wraps a real
// *frr.Runtime + Manager so the public /api/v1/frr
// routes reach a real vtysh / zebra / bgpd over IPC.
// A nil host (no runtime wired) means the IPC route
// returns 503 frr_unavailable, never a fake 200.
func (h *Handler) EnableFRR(host *FRRHost) {
	h.frr = host
}

// EnableBackupRestore registers the production
// restore executor (DV08 production-path). The host
// wraps a real *backup.Executor + JournalStore so the
// public /api/v1/backup/restore route reaches a real
// validate -> snapshot -> apply -> commit lifecycle
// over IPC. A nil host (no executor wired) means the
// IPC route returns 503, never a fake 200.
func (h *Handler) EnableBackupRestore(host interface {
	ServeRestore(http.ResponseWriter, *http.Request)
}) {
	h.restore = host
}

// EnableAnycast registers the read-only anycast evidence
// view (G24 anycast). The host exposes the per-VIP
// evidence (desired, health, advertisement) over IPC.
// The privileged store stays in-process.
func (h *Handler) EnableAnycast(host AnycastSource) {
	h.anycast = NewAnycastHost(host)
}

// EnableZTP registers the ZTP redemption manager. The
// host exposes the bounded read view + Redeem surface
// so the dashboard can drive a bootstrap device.
func (h *Handler) EnableZTP(host ZTPSource) {
	h.ztp = NewZTPHost(host)
}

// EnableLinuxObjects registers the read-only Linux object source
// used by the public /api/v1/objects/{kind} listers. The Agent owns
// the runtime probes; the API/Engine consume only bounded object
// projections over Unix IPC.
func (h *Handler) EnableLinuxObjects(host *LinuxObjectsHost) {
	h.linuxObjs = host
}

// EnableWireguard registers the production WireGuard
// host. The host owns the durable catalog + the
// kernel adapter; the loopback IPC dispatches every
// mutation through the authorisation gate before the
// underlying wireguard.API runs.
func (h *Handler) EnableWireguard(host *WireguardHost) {
	h.wireguard = host
}

// EnableXray registers the production Xray host. The
// host owns the durable catalog + the runtime; the
// loopback IPC dispatches every mutation through the
// authorisation gate before the underlying xray.API
// runs. On every successful mutation the API re-renders
// the config and applies it through the runtime so the
// daemon tracks the catalog state.
func (h *Handler) EnableXray(host *XrayHost) {
	h.xray = host
}

// EnableVault registers the production vault host. The host
// owns the protected secret lifecycle (Put / List / Revoke);
// the loopback IPC dispatches every request through the
// authorisation gate (ManageSecrets) before the underlying
// vault.API runs. Secret bytes never leave the agent.
func (h *Handler) EnableVault(host *VaultHost) {
	h.vault = host
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if strings.HasPrefix(r.URL.Path, NetworkPrefix) {
		h.serveNetwork(w, r)
		return
	}
	if r.URL.EscapedPath() == DoctorPath {
		h.ServeDoctor(w, r)
		return
	}
	if r.URL.EscapedPath() == TopologyPath {
		h.ServeTopology(w, r)
		return
	}
	if r.URL.EscapedPath() == MultiWANPath {
		h.ServeMultiWAN(w, r)
		return
	}
	if r.URL.EscapedPath() == QoSPath {
		h.ServeQoS(w, r)
		return
	}
	if r.URL.EscapedPath() == QoSApplyPath {
		h.ServeQoSApply(w, r)
		return
	}
	if r.URL.EscapedPath() == QoSRollbackPath {
		h.ServeQoSRollback(w, r)
		return
	}
	if r.URL.EscapedPath() == UPSPath {
		h.ServeUPS(w, r)
		return
	}
	if r.URL.EscapedPath() == UPSProbePath {
		h.ServeUPSProbe(w, r)
		return
	}
	if r.URL.EscapedPath() == MigratePath {
		h.ServeMigrate(w, r)
		return
	}
	if r.URL.EscapedPath() == SitesPath {
		h.ServeSites(w, r)
		return
	}
	if r.URL.EscapedPath() == WifiPath {
		h.ServeWifi(w, r)
		return
	}
	if r.URL.EscapedPath() == ReadinessPath {
		h.serveReadiness(w, r)
		return
	}
	if r.URL.EscapedPath() == ProvenancePath {
		h.serveProvenance(w, r)
		return
	}
	if r.URL.EscapedPath() == FirstRunPath {
		h.serveFirstRun(w, r)
		return
	}
	if r.URL.EscapedPath() == DiagJobPath {
		h.serveDiagJobs(w, r)
		return
	}
	if r.URL.EscapedPath() == UpgradePath {
		h.serveUpgrade(w, r)
		return
	}
	if r.URL.EscapedPath() == UpgradeStagePath {
		h.serveUpgradeStage(w, r)
		return
	}
	if r.URL.EscapedPath() == UpgradeActivatePath {
		h.serveUpgradeActivate(w, r)
		return
	}
	if r.URL.EscapedPath() == UpgradeRollbackPath {
		h.serveUpgradeRollback(w, r)
		return
	}
	if r.URL.EscapedPath() == BackupPath {
		h.serveBackup(w, r)
		return
	}
	if r.URL.EscapedPath() == BackupRestorePath || r.URL.EscapedPath() == BackupRestorePath+"/reconcile" {
		h.serveBackupRestore(w, r)
		return
	}
	if r.URL.EscapedPath() == AnycastPath {
		if h.anycast == nil {
			NewAnycastHost(nil).ServeAnycast(w, r)
			return
		}
		h.anycast.ServeAnycast(w, r)
		return
	}
	if r.URL.EscapedPath() == ZTPPath || r.URL.EscapedPath() == ZTPIssuePath {
		if h.ztp == nil {
			NewZTPHost(nil).ServeZTP(w, r)
			return
		}
		h.ztp.ServeZTP(w, r)
		return
	}
	if strings.HasPrefix(r.URL.EscapedPath(), LinuxObjectsPathPrefix) {
		if h.linuxObjs == nil {
			NewLinuxObjectsHost(nil).ServeHTTP(w, r)
			return
		}
		h.linuxObjs.ServeHTTP(w, r)
		return
	}
	if strings.HasPrefix(r.URL.EscapedPath(), WireguardPath) {
		if h.wireguard == nil {
			NewWireguardHost(nil, nil).ServeHTTP(w, r)
			return
		}
		h.wireguard.ServeHTTP(w, r)
		return
	}
	if strings.HasPrefix(r.URL.EscapedPath(), "/agent/v1/frr") {
		if h.frr == nil {
			writeFRRProblem(w, 503, "frr_unavailable", "FRR runtime is not configured.")
			return
		}
		h.frr.ServeFRR(w, r)
		return
	}
	if strings.HasPrefix(r.URL.EscapedPath(), XrayPath) {
		if h.xray == nil {
			NewXrayHost(nil, nil, nil).ServeHTTP(w, r)
			return
		}
		h.xray.ServeHTTP(w, r)
		return
	}
	if strings.HasPrefix(r.URL.EscapedPath(), VaultPath) {
		if h.vault == nil {
			NewVaultHost(nil, nil).ServeHTTP(w, r)
			return
		}
		h.vault.ServeHTTP(w, r)
		return
	}
	if r.URL.EscapedPath() != OverviewPath {
		problem(w, 404, "unknown_operation")
		return
	}
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		problem(w, 405, "method_not_allowed")
		return
	}
	if r.URL.RawQuery != "" || r.URL.ForceQuery || r.ContentLength != 0 || len(r.TransferEncoding) != 0 {
		problem(w, 400, "invalid_request")
		return
	}
	select {
	case h.slots <- struct{}{}:
		defer func() { <-h.slots }()
	default:
		problem(w, 503, "agent_busy")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	result, err := h.observer.Overview(ctx)
	if err != nil {
		problem(w, 503, "observation_unavailable")
		return
	}
	data, err := json.Marshal(result)
	if err != nil || len(data) > model.MaxResponseBytes {
		problem(w, 503, "observation_limit")
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
func problem(w http.ResponseWriter, status int, code string) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": code, "message": "Agent observation request could not be completed."}})
}

// serveReadiness renders the readiness snapshot over IPC.
// The endpoint is gated by the readiness observer: when no
// host is registered the endpoint returns 503 with a
// structured error so the UI can render a "readiness not
// configured" badge honestly.
func (h *Handler) serveReadiness(w http.ResponseWriter, r *http.Request) {
	if h.readiness == nil {
		problem(w, 503, "readiness_unavailable")
		return
	}
	if readinessHost, ok := h.readiness.(*ReadinessHost); ok {
		readinessHost.ServeReadiness(w, r)
		return
	}
	// Fallback: the observer satisfies our snapshot shape.
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		problem(w, 405, "method_not_allowed")
		return
	}
	if r.URL.RawQuery != "" || r.URL.ForceQuery || r.ContentLength != 0 || len(r.TransferEncoding) != 0 {
		problem(w, 400, "invalid_request")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 1500*time.Millisecond)
	defer cancel()
	rows, err := h.readiness.Snapshot(ctx)
	if err != nil {
		problem(w, 503, "readiness_unavailable")
		return
	}
	data, err := json.Marshal(map[string]any{"version": 1, "updatedAt": time.Now().UTC(), "features": rows})
	if err != nil {
		problem(w, 503, "readiness_limit")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// serveBackup renders the G17 backup catalog / restore
// plan over IPC.
func (h *Handler) serveBackup(w http.ResponseWriter, r *http.Request) {
	if h.backup == nil {
		NewBackupHost(nil).ServeBackup(w, r)
		return
	}
	if host, ok := h.backup.(*BackupHost); ok {
		host.ServeBackup(w, r)
		return
	}
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		problem(w, 405, "method_not_allowed")
		return
	}
	if r.URL.ForceQuery || r.ContentLength != 0 || len(r.TransferEncoding) != 0 {
		problem(w, 400, "invalid_request")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 1500*time.Millisecond)
	defer cancel()
	id := r.URL.Query().Get("id")
	if id == "" {
		backups, err := h.backup.List(ctx)
		if err != nil {
			problem(w, 503, "backup_unavailable")
			return
		}
		resp := map[string]any{"version": 1, "updatedAt": time.Now().UTC(), "backups": backups}
		data, _ := json.Marshal(resp)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
		return
	}
	plan, err := h.backup.Plan(ctx, id)
	if err != nil {
		problem(w, 503, "backup_plan_unavailable")
		return
	}
	resp := map[string]any{"version": 1, "updatedAt": time.Now().UTC(), "plan": plan}
	data, _ := json.Marshal(resp)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// serveBackupRestore runs the production restore
// lifecycle (DV08 production-path). A nil host
// returns 503; the agent composition root refuses to
// register a host without a real Executor + JournalStore.
func (h *Handler) serveBackupRestore(w http.ResponseWriter, r *http.Request) {
	if h.restore == nil {
		writeRestoreProblem(w, 503, "backup_restore_unavailable", "Backup restore is not enabled.")
		return
	}
	h.restore.ServeRestore(w, r)
}

// upgradeOrchestrator returns the OrchestratorAdapter
// behind h.upgrade if one is wired. Used by the stage /
// activate / rollback IPC routes. A nil observer (or an
// observer that is NOT an OrchestratorAdapter — e.g. a
// stub UpgradeHost) yields 503 upgrade_unavailable.
type upgradeMutator interface {
	Stage(context.Context, http.ResponseWriter, *http.Request)
	Activate(context.Context, http.ResponseWriter, *http.Request)
	Rollback(context.Context, http.ResponseWriter, *http.Request)
}

func (h *Handler) upgradeOrchestrator() upgradeMutator {
	if h.upgrade == nil {
		return nil
	}
	if oa, ok := h.upgrade.(upgradeMutator); ok {
		return oa
	}
	return nil
}

// serveUpgradeStage accepts a base64-encoded signed .duck
// package and runs the production orchestrator's Stage
// (preflight + materialize + Candidate).
func (h *Handler) serveUpgradeStage(w http.ResponseWriter, r *http.Request) {
	oa := h.upgradeOrchestrator()
	if oa == nil {
		writeUpgradeProblem(w, 503, "upgrade_unavailable", "Upgrade orchestrator is not configured.")
		return
	}
	oa.Stage(r.Context(), w, r)
}

// serveUpgradeActivate promotes Candidate -> Active.
func (h *Handler) serveUpgradeActivate(w http.ResponseWriter, r *http.Request) {
	oa := h.upgradeOrchestrator()
	if oa == nil {
		writeUpgradeProblem(w, 503, "upgrade_unavailable", "Upgrade orchestrator is not configured.")
		return
	}
	oa.Activate(r.Context(), w, r)
}

// serveUpgradeRollback promotes Previous -> Active.
func (h *Handler) serveUpgradeRollback(w http.ResponseWriter, r *http.Request) {
	oa := h.upgradeOrchestrator()
	if oa == nil {
		writeUpgradeProblem(w, 503, "upgrade_unavailable", "Upgrade orchestrator is not configured.")
		return
	}
	oa.Rollback(r.Context(), w, r)
}

// serveUpgrade renders the G16 upgrade status over IPC.
func (h *Handler) serveUpgrade(w http.ResponseWriter, r *http.Request) {
	if h.upgrade == nil {
		// Fall back to the stub host so the endpoint is
		// always observable.
		NewUpgradeHost(nil).ServeUpgrade(w, r)
		return
	}
	if host, ok := h.upgrade.(*UpgradeHost); ok {
		host.ServeUpgrade(w, r)
		return
	}
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		problem(w, 405, "method_not_allowed")
		return
	}
	if r.URL.ForceQuery || r.ContentLength != 0 || len(r.TransferEncoding) != 0 || r.URL.RawQuery != "" {
		problem(w, 400, "invalid_request")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 1500*time.Millisecond)
	defer cancel()
	status, err := h.upgrade.Status(ctx)
	resp := UpgradeStatus{Version: 1, UpdatedAt: time.Now().UTC(), Healthy: false}
	if err == nil {
		resp = status
		if resp.Version == 0 {
			resp.Version = 1
		}
		if resp.UpdatedAt.IsZero() {
			resp.UpdatedAt = time.Now().UTC()
		}
	} else {
		resp.Notes = []string{"upgrade status unavailable: " + err.Error()}
	}
	data, err := json.Marshal(resp)
	if err != nil {
		problem(w, 503, "upgrade_limit")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// serveDiagJobs renders the G19 diagnostic job list over
// IPC. The endpoint is gated by the diagjobs observer;
// missing host returns 503.
func (h *Handler) serveDiagJobs(w http.ResponseWriter, r *http.Request) {
	if h.diagjobs == nil {
		problem(w, 503, "diagjobs_unavailable")
		return
	}
	if host, ok := h.diagjobs.(*DiagJobHost); ok {
		host.ServeDiagJobs(w, r)
		return
	}
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		problem(w, 405, "method_not_allowed")
		return
	}
	if r.URL.ForceQuery || r.ContentLength != 0 || len(r.TransferEncoding) != 0 {
		problem(w, 400, "invalid_request")
		return
	}
	resp := map[string]any{"version": 1, "updatedAt": time.Now().UTC()}
	if id := r.URL.Query().Get("id"); id != "" {
		if j, ok := h.diagjobs.Get(id); ok {
			resp["jobs"] = []any{j}
		}
	} else {
		resp["jobs"] = h.diagjobs.List()
	}
	data, err := json.Marshal(resp)
	if err != nil {
		problem(w, 503, "diagjobs_limit")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// serveFirstRun renders the G18 first-run status over IPC.
func (h *Handler) serveFirstRun(w http.ResponseWriter, r *http.Request) {
	if h.firstrun == nil {
		problem(w, 503, "firstrun_unavailable")
		return
	}
	if host, ok := h.firstrun.(*FirstRunHost); ok {
		host.ServeFirstRun(w, r)
		return
	}
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		problem(w, 405, "method_not_allowed")
		return
	}
	if r.URL.ForceQuery || r.ContentLength != 0 || len(r.TransferEncoding) != 0 || r.URL.RawQuery != "" {
		problem(w, 400, "invalid_request")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 1500*time.Millisecond)
	defer cancel()
	state, err := h.firstrun.Status(ctx)
	resp := map[string]any{"version": 1, "updatedAt": time.Now().UTC()}
	if err == nil {
		resp["completed"] = state.Completed()
		resp["state"] = state
	} else if errors.Is(err, firstrun.ErrNotInitialised) {
		resp["pending"] = true
	} else {
		problem(w, 503, "firstrun_unavailable")
		return
	}
	data, err := json.Marshal(resp)
	if err != nil {
		problem(w, 503, "firstrun_limit")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// serveProvenance renders the combined ownership +
// provenance view over IPC (G08). The endpoint is gated
// by the provenance observer; missing host returns 503.
func (h *Handler) serveProvenance(w http.ResponseWriter, r *http.Request) {
	if h.provenance == nil {
		problem(w, 503, "provenance_unavailable")
		return
	}
	if host, ok := h.provenance.(*ProvenanceHost); ok {
		host.ServeProvenance(w, r)
		return
	}
	// Generic fallback: the observer satisfies our shape.
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		problem(w, 405, "method_not_allowed")
		return
	}
	if r.URL.ForceQuery || r.ContentLength != 0 || len(r.TransferEncoding) != 0 {
		problem(w, 400, "invalid_request")
		return
	}
	kind := r.URL.Query().Get("kind")
	id := r.URL.Query().Get("id")
	if (kind == "") != (id == "") {
		problem(w, 400, "invalid_request")
		return
	}
	resp := map[string]any{"version": 1, "updatedAt": time.Now().UTC()}
	if kind != "" && id != "" {
		obj := lifecycle.Object{Kind: lifecycle.ObjectKind(kind), ID: id}
		if obj.Valid() {
			if rec, ok := h.provenance.Ownership(obj); ok {
				resp["ownership"] = rec
			}
			resp["history"] = h.provenance.Provenance(obj)
		}
	}
	data, err := json.Marshal(resp)
	if err != nil {
		problem(w, 503, "provenance_limit")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
