//go:build linux

// agent/fw_host.go: the typed firewall rule management IPC
// surface (beta.4, journal-first rewrite).
//
//	PUBLIC -> Engine.FirewallProxy -> Agent IPC
//	       -> FirewallRulesService -> safeapply.Worker
//	       -> safeapply.Controller (own durable journal)
//	       -> fwrulesAdapter -> atomic nft batch (inet duckos_fw)
//
// This closes the "two transaction engines" gap: firewall
// mutations no longer apply inline. Every mutation composes
// a full-ruleset candidate and admits it through the same
// safeapply architecture the network configuration uses:
// durable journal, known-good current revision, validation
// before apply, bounded nft execution, readback verification,
// confirmation window with automatic rollback, crash recovery
// on startup, and idempotent re-recovery.
//
// V1 safety boundary unchanged: FORWARD rules only, so a
// UI-authored rule cannot cut management access.
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/fwrules"
	"github.com/CrazyDuckLab/DuckOS/internal/identity"
	"github.com/CrazyDuckLab/DuckOS/internal/nftlaunch"
	"github.com/CrazyDuckLab/DuckOS/internal/safeapply"
	"github.com/CrazyDuckLab/DuckOS/internal/state"
)

// FirewallRulesPath is the IPC route prefix.
const FirewallRulesPath = "/agent/v1/fwrules"

// FirewallConfirmWindow is the confirmation window used for
// rule-set admissions (bounded like the network path).
const FirewallConfirmWindow = 300 * time.Second

// fwrulesAdapter is the safeapply.Adapter over the owned
// nftables table. current/candidate documents are fwrules
// wire JSON.
type fwrulesAdapter struct {
	nftBin   string
	stageDir string
	// readbackFn is injectable for tests; nil means the real
	// nft readback.
	readbackFn func(nftBin string) ([]json.RawMessage, bool, error)
}

func (a *fwrulesAdapter) readback() ([]json.RawMessage, bool, error) {
	if a.readbackFn != nil {
		return a.readbackFn(a.nftBin)
	}
	return readbackFwrTable(a.nftBin)
}

func (a *fwrulesAdapter) decode(doc json.RawMessage) ([]fwrules.Rule, error) {
	if len(doc) == 0 {
		return nil, nil
	}
	return fwrules.LoadWire(doc)
}

// Preflight validates the candidate without touching the
// host: full model validation plus render (which re-checks
// the render-critical fields).
func (a *fwrulesAdapter) Preflight(_ context.Context, _ /*current*/, candidate json.RawMessage) error {
	rules, err := a.decode(candidate)
	if err != nil {
		return err
	}
	_, err = fwrules.Render(rules)
	return err
}

// Apply renders one atomic batch and loads it.
func (a *fwrulesAdapter) Apply(_ context.Context, _ /*current*/, candidate json.RawMessage) error {
	rules, err := a.decode(candidate)
	if err != nil {
		return err
	}
	batch, err := fwrules.Render(rules)
	if err != nil {
		return err
	}
	return a.runNFT(batch)
}

// Verify reads back the kernel and compares the full semantic
// state against the candidate: enabled rules present ⇔ owned
// table present with exactly those rules; empty ruleset ⇔
// owned table absent. A failed observation is an error, never
// a successful empty state; the controller treats a Verify
// error as apply failure and rolls back automatically.
func (a *fwrulesAdapter) Verify(_ context.Context, candidate json.RawMessage) error {
	rules, err := a.decode(candidate)
	if err != nil {
		return err
	}
	objs, absent, err := a.readback()
	if err != nil {
		return fmt.Errorf("fwrules: verify failed: kernel readback error: %w", err)
	}
	enabled := fwrules.EnabledRules(rules)
	if len(enabled) == 0 {
		if !absent {
			return fmt.Errorf("fwrules: verify failed: owned table still present after empty apply")
		}
		return nil
	}
	if absent {
		return fmt.Errorf("fwrules: verify failed: owned table absent after apply")
	}
	return fwrules.VerifySnapshot(enabled, objs)
}

// Restore re-applies the previous known-good ruleset (or
// removes the table when it was empty).
func (a *fwrulesAdapter) Restore(_ context.Context, before, _ /*candidate*/ json.RawMessage) error {
	rules, err := a.decode(before)
	if err != nil {
		return err
	}
	var batch []byte
	if len(rules) == 0 {
		batch = []byte("add table inet " + fwrules.TableName + "\ndelete table inet " + fwrules.TableName + "\n")
	} else {
		batch, err = fwrules.Render(rules)
		if err != nil {
			return err
		}
	}
	return a.runNFT(batch)
}

// runNFT stages nothing locally and execs nothing: the batch goes to
// the privileged duck-nft-launcher, which enforces the DuckOS table
// ownership allowlist (inet duckos_fw here) and performs the
// CAP_NET_ADMIN netlink work the zero-capability agent cannot.
// BETA.3 remediation: direct nft exec inside the agent failed with
// "Operation not permitted" because CapabilityBoundingSet= is empty
// by design. The check-then-load sequence and its error strings are
// preserved so the safeapply pipeline semantics are unchanged.
func (a *fwrulesAdapter) runNFT(batch []byte) error {
	client := nftClient()
	if err := client.Check(batch); err != nil {
		return fmt.Errorf("fwrules: nft check failed: %w", err)
	}
	if err := client.Apply(batch); err != nil {
		return fmt.Errorf("fwrules: nft load failed: %w", err)
	}
	return nil
}

// nftClient returns the launcher transport. Package-level var so
// tests can point it at a socket-backed fake launcher.
var nftClient = func() fwLauncherTransport { return nftlaunch.NewClient() }

// fwLauncherTransport is the launcher seam for the agent's nftables
// surfaces (firewall adapter + read-only observation hosts).
type fwLauncherTransport interface {
	Check(batch []byte) error
	Apply(batch []byte) error
	ListTableJSON(family, table string) ([]byte, error)
	ListRulesetJSON() ([]byte, error)
}

// readbackFwrTable lists the owned table through the privileged
// launcher, distinguishing the three observation outcomes fail-closed:
// (objects, table absent, error). A launcher-reported nft failure is
// only "absent" when nft itself reports the table does not exist;
// every other failure and every decode error surface as errors, never
// as an empty successful state.
func readbackFwrTable(nftBin string) ([]json.RawMessage, bool, error) {
	_ = nftBin // kept for seam compatibility; the launcher owns the nft path
	out, err := nftClient().ListTableJSON("inet", fwrules.TableName)
	if err != nil {
		if strings.Contains(err.Error(), "No such file") {
			return nil, true, nil
		}
		return nil, false, fmt.Errorf("fwrules: nft list failed: %w", err)
	}
	objs, err := fwrules.DecodeSnapshotObjects(out)
	if err != nil {
		return nil, false, fmt.Errorf("fwrules: kernel readback decode failed: %w", err)
	}
	return objs, false, nil
}

// FirewallRulesService is the journal-first firewall rule
// manager served over IPC.
type FirewallRulesService struct {
	mu         sync.Mutex
	closed     bool
	controller *safeapply.Controller
	worker     *safeapply.Worker
	Access     UPnPAccess
	storePath  string
	stageDir   string
	nftBin     string
	// adapter is the pipeline over the owned table; its
	// readbackFn seam is test-only.
	adapter *fwrulesAdapter
}

// NewFirewallRulesService opens the durable journal (creating
// the store with an empty ruleset on first boot) and wires
// the worker. Crash recovery of an interrupted transaction is
// the controller's startup behaviour (expire → restore).
func NewFirewallRulesService(storePath, stageDir, nftBin string, acc UPnPAccess) (*FirewallRulesService, error) {
	if err := os.MkdirAll(stageDir, 0o700); err != nil {
		return nil, fmt.Errorf("fwrules: stage dir: %w", err)
	}
	if err := os.MkdirAll(storePath, 0o700); err != nil {
		return nil, fmt.Errorf("fwrules: store dir: %w", err)
	}
	validate := func(raw json.RawMessage) error {
		_, err := fwrules.LoadWire(raw)
		return err
	}
	var store *state.Store
	var err error
	// The store path IS the private journal directory; first boot
	// creates it and its empty baseline, every later boot opens
	// the existing journal or fails closed.
	if _, statErr := os.Stat(filepath.Join(storePath, "state.json")); statErr == nil {
		store, err = state.OpenExisting(storePath, validate)
	} else {
		empty, _ := json.Marshal(fwrules.Wire{Version: 1, Rules: []fwrules.Rule{}})
		store, err = state.Open(storePath, empty, validate)
	}
	if err != nil {
		return nil, fmt.Errorf("fwrules: open store: %w", err)
	}
	adapter := &fwrulesAdapter{nftBin: nftBin, stageDir: stageDir}
	controller, err := safeapply.Open(store, adapter)
	if err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("fwrules: open controller: %w", err)
	}
	return &FirewallRulesService{
		controller: controller,
		worker:     safeapply.NewWorker(controller),
		Access:     acc,
		storePath:  storePath,
		stageDir:   stageDir,
		nftBin:     nftBin,
		adapter:    adapter,
	}, nil
}

// Close stops the worker/controller (agent shutdown path).
func (s *FirewallRulesService) Close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	s.mu.Unlock()
	s.worker.Close()
	_ = s.controller.Close()
}

// currentDoc returns the journaled current ruleset and its
// revision id.
func (s *FirewallRulesService) currentDoc() (json.RawMessage, string, error) {
	snap, err := s.controller.Status()
	if err != nil {
		return nil, "", err
	}
	return snap.Current.Config, snap.Current.ID, nil
}

// ServeHTTP dispatches /agent/v1/fwrules/*:
//
//	GET  /fwrules/v1/status             -> current ruleset + transaction state
//	PUT  /fwrules/v1/rules/{id}         -> compose candidate (create/replace) + admit
//	PUT  /fwrules/v1/rules/{id}/enabled -> compose candidate (toggle) + admit
//	DEL  /fwrules/v1/rules/{id}         -> compose candidate (delete) + admit
//	GET  /fwrules/v1/observed           -> kernel readback snapshot
//	POST /fwrules/v1/confirm            -> confirm the pending transaction
//	POST /fwrules/v1/rollback           -> roll back the pending transaction
func (s *FirewallRulesService) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	tail := strings.Trim(strings.TrimPrefix(r.URL.EscapedPath(), FirewallRulesPath), "/")
	// The engine proxy composes FirewallRulesPath +
	// "/fwrules/v1/" + verb; strip the version segment so the
	// canonical verb ("status", "rules/<id>", "confirm", ...)
	// reaches the switch.
	if strings.HasPrefix(tail, "fwrules/v1/") {
		tail = strings.TrimPrefix(tail, "fwrules/v1/")
	}
	switch {
	case r.Method == http.MethodGet && tail == "status":
		s.authorized(w, r, identity.Observe, s.handleStatus)
	case r.Method == http.MethodGet && tail == "observed":
		s.authorized(w, r, identity.Observe, s.handleObserved)
	case r.Method == http.MethodPost && tail == "confirm":
		s.authorized(w, r, identity.ConfigureNetwork, s.handleConfirm)
	case r.Method == http.MethodPost && tail == "rollback":
		s.authorized(w, r, identity.ConfigureNetwork, s.handleRollback)
	case r.Method == http.MethodPut && strings.HasSuffix(tail, "/enabled") && strings.HasPrefix(tail, "rules/"):
		id := strings.TrimSuffix(strings.TrimPrefix(tail, "rules/"), "/enabled")
		s.authorized(w, r, identity.ConfigureNetwork, func(w http.ResponseWriter, r *http.Request) {
			s.handleToggle(w, r, id)
		})
	case r.Method == http.MethodPut && strings.HasPrefix(tail, "rules/"):
		s.authorized(w, r, identity.ConfigureNetwork, func(w http.ResponseWriter, r *http.Request) {
			s.handlePut(w, r, strings.TrimPrefix(tail, "rules/"))
		})
	case r.Method == http.MethodDelete && strings.HasPrefix(tail, "rules/"):
		s.authorized(w, r, identity.ConfigureNetwork, func(w http.ResponseWriter, r *http.Request) {
			s.handleDelete(w, r, strings.TrimPrefix(tail, "rules/"))
		})
	default:
		w.Header().Set("Allow", "GET, PUT, DELETE, POST")
		fwrProblem(w, 404, "not_found", "Firewall rules endpoint not found.")
	}
}

func (s *FirewallRulesService) authorized(w http.ResponseWriter, r *http.Request, perm identity.Permission, next http.HandlerFunc) {
	v := r.Header.Get("Authorization")
	if len(v) < 7 || v[:7] != "Bearer " {
		fwrProblem(w, 401, "unauthorized", "Bearer token is required.")
		return
	}
	if _, err := s.Access.Authorize(v[7:], perm); err != nil {
		fwrProblem(w, 403, "forbidden", "Required permission is missing.")
		return
	}
	next(w, r)
}

// admit composes a candidate from the journaled current doc
// by applying mut, then submits it to the journal-first
// pipeline. The journal is the store of record: no separate
// intent file exists to drift.
func (s *FirewallRulesService) admit(w http.ResponseWriter, r *http.Request, op string, mut func([]fwrules.Rule) ([]fwrules.Rule, error)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		fwrProblem(w, 503, "unavailable", "Firewall rules service is closed.")
		return
	}
	current, revision, err := s.currentDoc()
	if err != nil {
		fwrProblem(w, 503, "journal_unavailable", err.Error())
		return
	}
	rules, err := fwrules.LoadWire(current)
	if err != nil {
		fwrProblem(w, 500, "journal_corrupt", err.Error())
		return
	}
	next, err := mut(rules)
	if err != nil {
		fwrProblem(w, 400, "invalid_request", err.Error())
		return
	}
	candidate, err := json.Marshal(fwrules.Wire{Version: 1, Rules: next})
	if err != nil {
		fwrProblem(w, 500, "compose_failed", err.Error())
		return
	}
	actor := "api:" + clientAddrOf(r)
	job, err := s.worker.SubmitRequest(revision, "", candidate, actor, op, FirewallConfirmWindow)
	if err != nil {
		fwrProblem(w, 409, "admission_conflict", err.Error())
		return
	}
	log.Printf("fwrules: admitted %s (tx=%s rules=%d by %s)", op, job.TransactionID, len(next), actor)
	writeFwrJSON(w, http.StatusAccepted, map[string]any{
		"status":        "admitted",
		"transactionId": job.TransactionID,
		"phase":         job.Phase,
		"note":          "change is applied and verified; it rolls back automatically unless confirmed within the window",
	})
}

func (s *FirewallRulesService) handlePut(w http.ResponseWriter, r *http.Request, id string) {
	defer r.Body.Close()
	var body struct {
		Rule fwrules.Rule `json:"rule"`
	}
	dec := json.NewDecoder(io.LimitReader(r.Body, 64<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil {
		fwrProblem(w, 400, "invalid_request", "Rule body is not valid JSON.")
		return
	}
	if body.Rule.ID == "" {
		body.Rule.ID = id
	}
	if body.Rule.ID != id {
		fwrProblem(w, 400, "invalid_request", "Rule id does not match the URL.")
		return
	}
	if err := body.Rule.Valid(); err != nil {
		fwrProblem(w, 400, "invalid_request", err.Error())
		return
	}
	s.admit(w, r, "put:"+id, func(rules []fwrules.Rule) ([]fwrules.Rule, error) {
		mgr := fwrules.NewManager()
		if err := mgr.Set(rules); err != nil {
			return nil, err
		}
		if err := mgr.Put(body.Rule); err != nil {
			return nil, err
		}
		return mgr.Rules(), nil
	})
}

func (s *FirewallRulesService) handleDelete(w http.ResponseWriter, r *http.Request, id string) {
	s.admit(w, r, "delete:"+id, func(rules []fwrules.Rule) ([]fwrules.Rule, error) {
		mgr := fwrules.NewManager()
		if err := mgr.Set(rules); err != nil {
			return nil, err
		}
		mgr.Delete(id)
		return mgr.Rules(), nil
	})
}

func (s *FirewallRulesService) handleToggle(w http.ResponseWriter, r *http.Request, id string) {
	defer r.Body.Close()
	var body struct {
		Enabled *bool `json:"enabled"`
	}
	dec := json.NewDecoder(io.LimitReader(r.Body, 4<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil || body.Enabled == nil {
		fwrProblem(w, 400, "invalid_request", "Body must be {\"enabled\":bool}.")
		return
	}
	enabled := *body.Enabled
	s.admit(w, r, fmt.Sprintf("enabled:%s=%v", id, enabled), func(rules []fwrules.Rule) ([]fwrules.Rule, error) {
		mgr := fwrules.NewManager()
		if err := mgr.Set(rules); err != nil {
			return nil, err
		}
		if err := mgr.SetEnabled(id, enabled); err != nil {
			return nil, err
		}
		return mgr.Rules(), nil
	})
}

func (s *FirewallRulesService) handleStatus(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		fwrProblem(w, 503, "unavailable", "Firewall rules service is closed.")
		return
	}
	current, revision, err := s.currentDoc()
	if err != nil {
		fwrProblem(w, 503, "journal_unavailable", err.Error())
		return
	}
	rules, err := fwrules.LoadWire(current)
	if err != nil {
		fwrProblem(w, 500, "journal_corrupt", err.Error())
		return
	}
	job, _, err := s.worker.Status()
	if err != nil {
		fwrProblem(w, 503, "journal_unavailable", err.Error())
		return
	}
	if rules == nil {
		rules = []fwrules.Rule{}
	}
	writeFwrJSON(w, http.StatusOK, map[string]any{
		"rules":         rules,
		"revision":      revision,
		"transaction":   job,
		"confirmWindow": int(FirewallConfirmWindow.Seconds()),
		"observedAt":    time.Now().UTC().Format(time.RFC3339),
	})
}

func (s *FirewallRulesService) handleObserved(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	nftBin := s.nftBin
	s.mu.Unlock()
	objs, absent, err := readbackFwrTable(nftBin)
	if err != nil {
		writeFwrJSON(w, http.StatusBadGateway, map[string]any{
			"error":      map[string]string{"code": "observation_failed", "message": err.Error()},
			"observedAt": time.Now().UTC().Format(time.RFC3339),
		})
		return
	}
	writeFwrJSON(w, http.StatusOK, map[string]any{
		"objects":    objs,
		"absent":     absent,
		"empty":      absent || len(objs) == 0,
		"observedAt": time.Now().UTC().Format(time.RFC3339),
	})
}

func (s *FirewallRulesService) handleConfirm(w http.ResponseWriter, r *http.Request) {
	s.txControl(w, r, true)
}

func (s *FirewallRulesService) handleRollback(w http.ResponseWriter, r *http.Request) {
	s.txControl(w, r, false)
}

func (s *FirewallRulesService) txControl(w http.ResponseWriter, r *http.Request, confirm bool) {
	defer r.Body.Close()
	var body struct {
		TransactionID string `json:"transactionId"`
	}
	dec := json.NewDecoder(io.LimitReader(r.Body, 4<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil || !state.ValidTransactionID(body.TransactionID) {
		fwrProblem(w, 400, "invalid_request", "transactionId is required.")
		return
	}
	job, _, err := s.worker.Status()
	if err != nil || job.TransactionID != body.TransactionID {
		fwrProblem(w, 409, "transaction_conflict", "No matching pending transaction.")
		return
	}
	if confirm {
		if err := s.controller.Confirm(body.TransactionID); err != nil {
			fwrProblem(w, 409, "not_confirmable", err.Error())
			return
		}
	} else {
		if err := s.controller.Rollback(body.TransactionID); err != nil {
			fwrProblem(w, 409, "not_rollbackable", err.Error())
			return
		}
	}
	log.Printf("fwrules: %s tx=%s (by %s)", map[bool]string{true: "confirmed", false: "rolled back"}[confirm], body.TransactionID, clientAddrOf(r))
	writeFwrJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func writeFwrJSON(w http.ResponseWriter, status int, body any) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func fwrProblem(w http.ResponseWriter, status int, code, message string) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]string{"code": code, "message": message},
	})
}

func clientAddrOf(r *http.Request) string {
	if r == nil {
		return "-"
	}
	return r.RemoteAddr
}
