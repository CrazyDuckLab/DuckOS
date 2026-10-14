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
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/fwrules"
	"github.com/CrazyDuckLab/DuckOS/internal/identity"
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

// Verify reads back the kernel: rules present ⇔ owned table
// present with objects; empty ruleset ⇔ table absent. The
// controller treats a Verify error as apply failure and
// rolls back automatically.
func (a *fwrulesAdapter) Verify(_ context.Context, candidate json.RawMessage) error {
	rules, err := a.decode(candidate)
	if err != nil {
		return err
	}
	objs := readbackFwrTable(a.nftBin)
	if len(rules) == 0 {
		if len(objs) != 0 {
			return fmt.Errorf("fwrules: verify failed: table still present after empty apply")
		}
		return nil
	}
	if len(objs) == 0 {
		return fmt.Errorf("fwrules: verify failed: owned table absent after apply")
	}
	return nil
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

func (a *fwrulesAdapter) runNFT(batch []byte) error {
	stage := filepath.Join(a.stageDir, "duckos-fw.batch.nft")
	if err := os.WriteFile(stage, batch, 0o600); err != nil {
		return fmt.Errorf("fwrules: stage write failed: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	check := exec.CommandContext(ctx, a.nftBin, "-c", "-f", stage)
	var errOut strings.Builder
	check.Stderr = &errOut
	if err := check.Run(); err != nil {
		return fmt.Errorf("fwrules: nft check failed: %w: %s", err, strings.TrimSpace(errOut.String()))
	}
	load := exec.CommandContext(ctx, a.nftBin, "-f", stage)
	load.Stderr = &errOut
	if err := load.Run(); err != nil {
		return fmt.Errorf("fwrules: nft load failed: %w: %s", err, strings.TrimSpace(errOut.String()))
	}
	return nil
}

// readbackFwrTable lists the owned table; nil = absent/error.
func readbackFwrTable(nftBin string) []json.RawMessage {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, nftBin, "-j", "list", "table", "inet", fwrules.TableName).Output()
	if err != nil {
		return nil
	}
	objs, err := fwrules.DecodeSnapshotObjects(out)
	if err != nil {
		return nil
	}
	return objs
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
}

// NewFirewallRulesService opens the durable journal (creating
// the store with an empty ruleset on first boot) and wires
// the worker. Crash recovery of an interrupted transaction is
// the controller's startup behaviour (expire → restore).
func NewFirewallRulesService(storePath, stageDir, nftBin string, acc UPnPAccess) (*FirewallRulesService, error) {
	if err := os.MkdirAll(stageDir, 0o700); err != nil {
		return nil, fmt.Errorf("fwrules: stage dir: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(storePath), 0o700); err != nil {
		return nil, fmt.Errorf("fwrules: store dir: %w", err)
	}
	validate := func(raw json.RawMessage) error {
		_, err := fwrules.LoadWire(raw)
		return err
	}
	var store *state.Store
	var err error
	if _, statErr := os.Stat(storePath); statErr == nil {
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
	objs := readbackFwrTable(nftBin)
	empty := len(objs) == 0
	writeFwrJSON(w, http.StatusOK, map[string]any{
		"objects":    objs,
		"empty":      empty,
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
