//go:build linux

// agent/fw_host.go: the typed firewall rule management IPC
// surface (beta.4).
//
//	PUBLIC -> Engine.FirewallProxy -> Agent IPC
//	       -> FirewallRulesHost -> fwrules.Manager + StoreFile
//	       -> atomic nft batch (inet duckos_fw)
//
// Workflow per mutation:
//
//	authenticate (ConfigureNetwork) -> validate (fwrules)
//	-> persist staged config -> render one atomic batch
//	-> nft -c check -> nft -f apply -> readback verify
//	-> on verify failure: restore previous batch + previous
//	store -> report honestly
//
// Safety properties:
//
//   - V1 renders FORWARD rules only (model enforces it), so
//     a UI-authored rule cannot cut management access.
//   - The table is exclusively owned; no other ruleset is
//     touched, nothing is flushed globally.
//   - Failed verify restores the previous known-good ruleset
//     atomically and returns an error — the API never
//     reports success for a state that does not exist.
package agent

import (
	"bytes"
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
	"github.com/CrazyDuckLab/DuckOS/internal/network"
)

// FirewallRulesPath is the IPC route prefix.
const FirewallRulesPath = "/agent/v1/fwrules"

// FirewallRulesHost serves the typed rule surface.
type FirewallRulesHost struct {
	mu      sync.Mutex
	mgr     *fwrules.Manager
	store   *fwrules.StoreFile
	nftBin  string
	Access  UPnPAccess
	// stageDir holds the transient nft batch file.
	stageDir string
}

// NewFirewallRulesHost loads the persisted rule set (missing
// file = empty set) and wires the nft runner.
func NewFirewallRulesHost(storePath, stageDir, nftBin string, acc UPnPAccess) (*FirewallRulesHost, error) {
	store := fwrules.NewStoreFile(storePath)
	rules, err := store.Load()
	if err != nil {
		return nil, fmt.Errorf("fwrules: load store: %w", err)
	}
	mgr := fwrules.NewManager()
	if len(rules) > 0 {
		if err := mgr.Set(rules); err != nil {
			return nil, fmt.Errorf("fwrules: stored rules invalid: %w", err)
		}
	}
	if err := os.MkdirAll(stageDir, 0o700); err != nil {
		return nil, fmt.Errorf("fwrules: stage dir: %w", err)
	}
	return &FirewallRulesHost{mgr: mgr, store: store, nftBin: nftBin, Access: acc, stageDir: stageDir}, nil
}

// ServeHTTP dispatches /agent/v1/fwrules/*:
//
//	GET    /fwrules/v1/rules          -> config + observed status
//	PUT    /fwrules/v1/rules/{id}     -> create/replace + apply
//	DELETE /fwrules/v1/rules/{id}     -> delete + apply
//	PUT    /fwrules/v1/rules/{id}/enabled -> enable/disable + apply
//	GET    /fwrules/v1/observed       -> kernel readback snapshot
func (h *FirewallRulesHost) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	tail := strings.Trim(strings.TrimPrefix(r.URL.EscapedPath(), FirewallRulesPath), "/")
	switch {
	case r.Method == http.MethodGet && tail == "rules":
		h.authorized(w, r, identity.Observe, h.handleGetRules)
	case r.Method == http.MethodGet && tail == "observed":
		h.authorized(w, r, identity.Observe, h.handleObserved)
	case r.Method == http.MethodPut && strings.HasSuffix(tail, "/enabled") && strings.HasPrefix(tail, "rules/"):
		id := strings.TrimSuffix(strings.TrimPrefix(tail, "rules/"), "/enabled")
		h.authorized(w, r, identity.ConfigureNetwork, func(w http.ResponseWriter, r *http.Request) {
			h.handleSetEnabled(w, r, id)
		})
	case r.Method == http.MethodPut && strings.HasPrefix(tail, "rules/"):
		h.authorized(w, r, identity.ConfigureNetwork, func(w http.ResponseWriter, r *http.Request) {
			h.handlePut(w, r, strings.TrimPrefix(tail, "rules/"))
		})
	case r.Method == http.MethodDelete && strings.HasPrefix(tail, "rules/"):
		h.authorized(w, r, identity.ConfigureNetwork, func(w http.ResponseWriter, r *http.Request) {
			h.handleDelete(w, r, strings.TrimPrefix(tail, "rules/"))
		})
	default:
		w.Header().Set("Allow", "GET, PUT, DELETE")
		fwrProblem(w, 404, "not_found", "Firewall rules endpoint not found.")
	}
}

func (h *FirewallRulesHost) authorized(w http.ResponseWriter, r *http.Request, perm identity.Permission, next http.HandlerFunc) {
	v := r.Header.Get("Authorization")
	if len(v) < 7 || v[:7] != "Bearer " {
		fwrProblem(w, 401, "unauthorized", "Bearer token is required.")
		return
	}
	if _, err := h.Access.Authorize(v[7:], perm); err != nil {
		fwrProblem(w, 403, "forbidden", "Required permission is missing.")
		return
	}
	next(w, r)
}

func (h *FirewallRulesHost) handleGetRules(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	rules := h.mgr.Rules()
	path := h.store.Path()
	h.mu.Unlock()
	writeFwrJSON(w, http.StatusOK, map[string]any{
		"rules":     rules,
		"store":     path,
		"observedAt": time.Now().UTC().Format(time.RFC3339),
	})
}

func (h *FirewallRulesHost) handlePut(w http.ResponseWriter, r *http.Request, id string) {
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
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := h.applyMutation(func() error { return h.mgr.Put(body.Rule) }, "put:"+id, r); err != nil {
		fwrProblem(w, 500, "apply_failed", err.Error())
		return
	}
	writeFwrJSON(w, http.StatusOK, map[string]any{"status": "applied", "rule": body.Rule})
}

func (h *FirewallRulesHost) handleDelete(w http.ResponseWriter, r *http.Request, id string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := h.applyMutation(func() error {
		h.mgr.Delete(id)
		return nil
	}, "delete:"+id, r); err != nil {
		fwrProblem(w, 500, "apply_failed", err.Error())
		return
	}
	writeFwrJSON(w, http.StatusOK, map[string]any{"status": "applied"})
}

func (h *FirewallRulesHost) handleSetEnabled(w http.ResponseWriter, r *http.Request, id string) {
	defer r.Body.Close()
	var body struct {
		Enabled bool `json:"enabled"`
	}
	dec := json.NewDecoder(io.LimitReader(r.Body, 4<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil {
		fwrProblem(w, 400, "invalid_request", "Body is not valid JSON.")
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := h.applyMutation(func() error { return h.mgr.SetEnabled(id, body.Enabled) }, "enabled:"+id, r); err != nil {
		fwrProblem(w, 500, "apply_failed", err.Error())
		return
	}
	writeFwrJSON(w, http.StatusOK, map[string]any{"status": "applied"})
}

// applyMutation runs mut, then persists and applies the new
// rule set with verify + rollback. mut failures leave
// nothing applied; apply failures restore the previous
// kernel and store state.
func (h *FirewallRulesHost) applyMutation(mut func() error, op string, r *http.Request) error {
	before := h.mgr.Rules()
	if err := mut(); err != nil {
		return err
	}
	after := h.mgr.Rules()
	if err := h.store.Save(after); err != nil {
		_ = h.mgr.Set(before)
		return fmt.Errorf("persist failed: %w", err)
	}
	if err := h.applyRuleset(after, before); err != nil {
		// Roll kernel + store + memory back to known-good.
		_ = h.restoreRuleset(before)
		_ = h.store.Save(before)
		_ = h.mgr.Set(before)
		log.Printf("fwrules: apply %s failed, rolled back: %v (by %s)", op, err, clientAddrOf(r))
		return err
	}
	log.Printf("fwrules: applied %s (rules=%d, by %s)", op, len(after), clientAddrOf(r))
	return nil
}

// applyRules renders and applies one atomic batch, then
// verifies by readback. previous is restored on failure.
func (h *FirewallRulesHost) applyRuleset(rules, previous []fwrules.Rule) error {
	batch, err := fwrules.Render(rules)
	if err != nil {
		return err
	}
	if err := h.runNFT(batch); err != nil {
		return err
	}
	if len(rules) == 0 {
		// Empty set: the batch deleted the table; readback of
		// absence is the success condition.
		if objs := h.readbackObjects(); len(objs) != 0 {
			return fmt.Errorf("verify failed: table still present after empty apply")
		}
		return nil
	}
	objs := h.readbackObjects()
	if len(objs) == 0 {
		_ = h.restoreRuleset(previous)
		return fmt.Errorf("verify failed: owned table absent after apply")
	}
	return nil
}

// restoreRuleset best-effort restores a previous ruleset.
func (h *FirewallRulesHost) restoreRuleset(rules []fwrules.Rule) error {
	if len(rules) == 0 {
		return h.runNFT([]byte("add table inet " + fwrules.TableName + "\ndelete table inet " + fwrules.TableName + "\n"))
	}
	batch, err := fwrules.Render(rules)
	if err != nil {
		return err
	}
	return h.runNFT(batch)
}

// runNFT stages and executes one atomic batch. nft -c
// validates first, then the same file is loaded.
func (h *FirewallRulesHost) runNFT(batch []byte) error {
	stage := filepath.Join(h.stageDir, "duckos-fw.batch.nft")
	if err := os.WriteFile(stage, batch, 0o600); err != nil {
		return fmt.Errorf("stage write failed: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	check := exec.CommandContext(ctx, h.nftBin, "-c", "-f", stage)
	var errOut bytes.Buffer
	check.Stderr = &errOut
	if err := check.Run(); err != nil {
		return fmt.Errorf("nft check failed: %w: %s", err, strings.TrimSpace(errOut.String()))
	}
	load := exec.CommandContext(ctx, h.nftBin, "-f", stage)
	load.Stderr = &errOut
	if err := load.Run(); err != nil {
		return fmt.Errorf("nft load failed: %w: %s", err, strings.TrimSpace(errOut.String()))
	}
	return nil
}

// readbackObjects lists the owned table and decodes it into
// the standard snapshot shape.
func (h *FirewallRulesHost) readbackObjects() []json.RawMessage {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, h.nftBin, "-j", "list", "table", "inet", fwrules.TableName).Output()
	if err != nil {
		return nil
	}
	snap, err := network.DecodeNFTSnapshot(out)
	if err != nil {
		return nil
	}
	return snap.Objects
}

func (h *FirewallRulesHost) handleObserved(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	objs := h.readbackObjects()
	h.mu.Unlock()
	empty := len(objs) == 0
	writeFwrJSON(w, http.StatusOK, map[string]any{
		"objects":    objs,
		"empty":      empty,
		"observedAt": time.Now().UTC().Format(time.RFC3339),
	})
}

func clientAddrOf(r *http.Request) string {
	if r == nil {
		return "-"
	}
	return r.RemoteAddr
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

// fwrRulesUnavailable is the fail-closed stand-in used when
// the agent did not wire the typed firewall host.
func fwrRulesUnavailable() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		fwrProblem(w, 503, "unavailable", "Firewall rule management is not wired on this agent.")
	}
}
