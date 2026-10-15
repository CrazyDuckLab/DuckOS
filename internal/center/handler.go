// Package center wires the Xray Center subsystems (nodes, groups,
// delivery tokens, policies, stats, backup, diagnostics) into one
// authenticated loopback HTTP surface mounted under the existing
// xray API prefix (/xray/v1/center/*), so the public proxy exposes
// it at /api/v1/xray/center/* with the same RBAC and audit path and
// no new privileged surface.
package center

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/identity"
	"github.com/CrazyDuckLab/DuckOS/internal/persist"
	"github.com/CrazyDuckLab/DuckOS/internal/subscription"
	"github.com/CrazyDuckLab/DuckOS/internal/xnode"
	"github.com/CrazyDuckLab/DuckOS/internal/xray"
)

// Access mirrors the xray.API authorisation contract.
type Access interface {
	Authorize(token string, p identity.Permission) (identity.User, error)
}

// DeliveryServicePrincipal is the dedicated least-privilege
// identity the public delivery daemon uses. It is admitted to
// exactly ONE operation — content preview — regardless of any role
// its forwarded token carries; a leaked delivery token cannot read
// the fleet, mutate nodes, issue tokens or touch backups.
const DeliveryServicePrincipal = "duck-api-delivery"

// Handler serves /xray/v1/center/*. The zero-value services are
// optional: a nil subsystem returns a typed 501 so the surface
// honestly reports what is not wired instead of failing obscurely.
type Handler struct {
	Access Access
	Prefix string
	// Nodes is the node registry (nil = node management disabled).
	Nodes *xnode.Manager
	// Groups is the subscription group store.
	Groups *subscription.GroupStore
	// Tokens is the delivery token store.
	Tokens *subscription.TokenStore
	// Policies is the durable policy set.
	Policies *persist.File[[]xray.Policy]
	// Stats returns the traffic ledger snapshot and staleness.
	Stats func() (map[string]uint64, time.Time, bool)
	// Render renders subscription content: kind is "group" or
	// "subscription" (profile-based xray subscription).
	Render func(kind, subID string, format subscription.Format) ([]byte, error)
	// Export builds a backup document from the live catalog.
	Export func(includeSecrets bool) (xray.Backup, error)
	// PreviewRestore validates a backup against the live catalog.
	PreviewRestore func(b xray.Backup) ([]xray.RestoreEntry, error)
	// Events is the insight log.
	Events *xray.EventLog
	// Diagnose runs the structured diagnostics for a subject.
	Diagnose func(subject string) ([]xray.Diagnostic, error)
	// StorageStatus reports per-domain degraded diagnoses (empty
	// map = all healthy).
	StorageStatus func() map[string]string
	// ReconcileStorage resolves commit uncertainty against the
	// authoritative disk snapshot.
	ReconcileStorage func() (string, error)
	// authorizedUser is an internal seam for tests; production
	// leaves it nil and Authorize supplies the user.
	authorizedUser func() (identity.User, error)
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.Access == nil {
		problem(w, 500, "center_unavailable", "Xray Center is not enabled.")
		return
	}
	user := identity.User{}
	var err error
	if h.authorizedUser != nil {
		user, err = h.authorizedUser()
	} else {
		user, err = h.Access.Authorize(extractToken(r), identity.ConfigureNetwork)
	}
	if err != nil {
		problem(w, 403, "forbidden", "Network write permission is required.")
		return
	}
	rest := r.URL.Path
	for _, p := range []string{h.Prefix, "/" + h.Prefix, "/xray/v1/" + h.Prefix} {
		if strings.HasPrefix(rest, p) {
			rest = strings.TrimPrefix(rest, p)
			break
		}
	}
	rest = strings.TrimPrefix(rest, "/")
	// Least privilege: the delivery service principal may render
	// content and nothing else. Every other center operation is
	// refused for it even when its token carries broad roles.
	if user.Username == DeliveryServicePrincipal {
		if !strings.HasPrefix(rest, "preview/") {
			problem(w, 403, "forbidden", "The delivery principal may only render subscription content.")
			return
		}
	}
	switch {
	case strings.HasPrefix(rest, "nodes"):
		h.serveNodes(w, r, strings.TrimPrefix(rest, "nodes"))
	case strings.HasPrefix(rest, "groups"):
		h.serveGroups(w, r, strings.TrimPrefix(rest, "groups"))
	case strings.HasPrefix(rest, "policies"):
		h.servePolicies(w, r)
	case strings.HasPrefix(rest, "stats"):
		h.serveStats(w, r)
	case strings.HasPrefix(rest, "storage"):
		h.serveStorage(w, r, strings.TrimPrefix(rest, "storage"))
	case strings.HasPrefix(rest, "tokens"):
		h.serveTokens(w, r, strings.TrimPrefix(rest, "tokens"))
	case strings.HasPrefix(rest, "preview"):
		h.servePreview(w, r, strings.TrimPrefix(rest, "preview"))
	case strings.HasPrefix(rest, "backup"):
		h.serveBackup(w, r, strings.TrimPrefix(rest, "backup"))
	case strings.HasPrefix(rest, "events"):
		h.serveEvents(w, r)
	case strings.HasPrefix(rest, "diagnostics"):
		h.serveDiagnostics(w, r)
	default:
		problem(w, 404, "not_found", "Unknown Xray Center operation.")
	}
}

/* — nodes — */

func (h *Handler) serveNodes(w http.ResponseWriter, r *http.Request, rest string) {
	if h.Nodes == nil {
		problem(w, 501, "nodes_unavailable", "Node management is not enabled.")
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodPost && r.Method != http.MethodDelete {
		w.Header().Set("Allow", "GET, POST, DELETE")
		problem(w, 405, "method_not_allowed", "Method is not supported for nodes.")
		return
	}
	switch {
	case r.Method == http.MethodGet && rest == "":
		writeJSON(w, 200, h.Nodes.List())
	case r.Method == http.MethodGet && rest == "/eligible":
		writeJSON(w, 200, h.Nodes.Eligible())
	case r.Method == http.MethodPost && rest == "/enroll-token":
		token, _, err := h.Nodes.IssueEnrollmentToken(time.Hour)
		if err != nil {
			problem(w, 503, "enroll_token_failed", err.Error())
			return
		}
		// The presentation token is returned exactly once over the
		// authenticated management channel.
		writeJSON(w, 201, map[string]string{"token": token})
	case r.Method == http.MethodPost && rest == "/local":
		var body struct {
			Name string `json:"name"`
		}
		if err := decodeBody(r, &body); err != nil {
			problem(w, 400, "invalid_request", "Invalid local node payload.")
			return
		}
		node, err := h.Nodes.EnrollLocal(body.Name)
		if err != nil {
			problem(w, 422, "invalid_node", err.Error())
			return
		}
		writeJSON(w, 201, node)
	case r.Method == http.MethodPost && strings.HasSuffix(rest, "/rotate"):
		id := strings.TrimSuffix(strings.TrimPrefix(rest, "/"), "/rotate")
		credential, err := h.Nodes.RotateCredential(id)
		if err != nil {
			switch {
			case strings.Contains(strings.ToLower(err.Error()), "uncertain"):
				problem(w, 503, "rotation_commit_uncertain", err.Error())
			case strings.Contains(strings.ToLower(err.Error()), "not committed"):
				problem(w, 503, "rotation_not_committed", err.Error())
			default:
				problem(w, 409, "rotate_failed", err.Error())
			}
			return
		}
		writeJSON(w, 200, map[string]string{"credential": credential})
	case r.Method == http.MethodPost && strings.HasSuffix(rest, "/suspend"):
		id := strings.TrimSuffix(strings.TrimPrefix(rest, "/"), "/suspend")
		if err := h.Nodes.Suspend(id); err != nil {
			problem(w, 409, "suspend_failed", err.Error())
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	case r.Method == http.MethodPost && strings.HasSuffix(rest, "/resume"):
		id := strings.TrimSuffix(strings.TrimPrefix(rest, "/"), "/resume")
		if err := h.Nodes.Resume(id); err != nil {
			problem(w, 409, "resume_failed", err.Error())
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	case r.Method == http.MethodDelete && rest != "":
		if err := h.Nodes.Revoke(strings.TrimPrefix(rest, "/")); err != nil {
			// A persistence failure must never look like a plain
			// not-found: the operator has to know the durable
			// state did not change.
			switch {
			case strings.Contains(strings.ToLower(err.Error()), "uncertain"):
				problem(w, 503, "revocation_commit_uncertain", err.Error())
			case strings.Contains(strings.ToLower(err.Error()), "not committed"):
				problem(w, 503, "revocation_not_committed", err.Error())
			default:
				problem(w, 404, "node_not_found", err.Error())
			}
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	default:
		problem(w, 404, "not_found", "Unknown node operation.")
	}
}

/* — groups — */

func (h *Handler) serveGroups(w http.ResponseWriter, r *http.Request, rest string) {
	if h.Groups == nil {
		problem(w, 501, "groups_unavailable", "Subscription groups are not enabled.")
		return
	}
	switch {
	case r.Method == http.MethodGet && rest == "":
		writeJSON(w, 200, h.Groups.List())
	case r.Method == http.MethodPost && rest == "":
		var g subscription.Group
		if err := decodeBody(r, &g); err != nil {
			problem(w, 400, "invalid_request", "Invalid group payload.")
			return
		}
		saved, err := h.Groups.Upsert(g)
		if err != nil {
			problem(w, 422, "invalid_group", err.Error())
			return
		}
		writeJSON(w, 201, saved)
	case r.Method == http.MethodDelete && rest != "":
		if err := h.Groups.Delete(strings.TrimPrefix(rest, "/")); err != nil {
			problem(w, 404, "group_not_found", err.Error())
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	default:
		problem(w, 404, "not_found", "Unknown group operation.")
	}
}

/* — policies — */

func (h *Handler) servePolicies(w http.ResponseWriter, r *http.Request) {
	if h.Policies == nil {
		problem(w, 501, "policies_unavailable", "Routing policies are not enabled.")
		return
	}
	switch r.Method {
	case http.MethodGet:
		policies := h.Policies.Get()
		rules, _ := xray.Compile(policies)
		conflicts := xray.DetectConflicts(policies)
		writeJSON(w, 200, map[string]any{
			"policies":  policies,
			"rules":     rules,
			"conflicts": conflicts,
		})
	case http.MethodPut:
		var policies []xray.Policy
		if err := decodeBody(r, &policies); err != nil {
			problem(w, 400, "invalid_request", "Invalid policy set payload.")
			return
		}
		if _, err := xray.Compile(policies); err != nil {
			problem(w, 422, "invalid_policies", err.Error())
			return
		}
		outcome, err := h.Policies.Update(func(p *[]xray.Policy) { *p = policies })
		if err != nil {
			code := "policies_persist_failed"
			if outcome == persist.CommitUncertain {
				code = "policies_commit_uncertain"
			}
			problem(w, 503, code, "Policy store rejected the update.")
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	default:
		w.Header().Set("Allow", "GET, PUT")
		problem(w, 405, "method_not_allowed", "Method is not supported for policies.")
	}
}

/* — stats — */

func (h *Handler) serveStats(w http.ResponseWriter, r *http.Request) {
	if h.Stats == nil {
		problem(w, 501, "stats_unavailable", "Traffic statistics are not enabled.")
		return
	}
	ledger, at, stale := h.Stats()
	if stale {
		writeJSON(w, 200, map[string]any{
			"stale":       true,
			"collectedAt": at,
			"note":        "last collection failed; values are not current",
		})
		return
	}
	writeJSON(w, 200, map[string]any{"stale": false, "collectedAt": at, "ledger": ledger})
}

/* — delivery tokens — */

func (h *Handler) serveTokens(w http.ResponseWriter, r *http.Request, rest string) {
	if h.Tokens == nil {
		problem(w, 501, "tokens_unavailable", "Delivery tokens are not enabled.")
		return
	}
	switch {
	case r.Method == http.MethodPost && rest == "":
		var body struct {
			SubID    string `json:"subId"`
			TTLHours int    `json:"ttlHours"`
		}
		if err := decodeBody(r, &body); err != nil || body.SubID == "" {
			problem(w, 400, "invalid_request", "Token issuance needs a subId.")
			return
		}
		ttl := time.Duration(body.TTLHours) * time.Hour
		token, rec, err := h.Tokens.Issue(body.SubID, ttl)
		if err != nil {
			problem(w, 503, tokenErrorCode(err), err.Error())
			return
		}
		writeJSON(w, 201, map[string]any{"token": token, "record": rec})
	case r.Method == http.MethodPost && strings.HasSuffix(rest, "/rotate"):
		var body struct {
			SubID    string `json:"subId"`
			TTLHours int    `json:"ttlHours"`
		}
		if err := decodeBody(r, &body); err != nil || body.SubID == "" {
			problem(w, 400, "invalid_request", "Rotation needs a subId.")
			return
		}
		token, rec, err := h.Tokens.Rotate(body.SubID, time.Duration(body.TTLHours)*time.Hour)
		if err != nil {
			problem(w, 503, tokenErrorCode(err), err.Error())
			return
		}
		writeJSON(w, 200, map[string]any{"token": token, "record": rec})
	case r.Method == http.MethodPost && strings.HasSuffix(rest, "/revoke"):
		var body struct {
			SubID string `json:"subId"`
		}
		if err := decodeBody(r, &body); err != nil || body.SubID == "" {
			problem(w, 400, "invalid_request", "Revocation needs a subId.")
			return
		}
		n, err := h.Tokens.RevokeSubscription(body.SubID)
		if err != nil {
			problem(w, 503, tokenErrorCode(err), err.Error())
			return
		}
		writeJSON(w, 200, map[string]int{"revoked": n})
	case r.Method == http.MethodGet && rest != "":
		subID := strings.TrimPrefix(rest, "/")
		writeJSON(w, 200, map[string]int{"active": h.Tokens.TokenCount(subID)})
	default:
		problem(w, 404, "not_found", "Unknown token operation.")
	}
}

/* — preview (content render) — */

func (h *Handler) servePreview(w http.ResponseWriter, r *http.Request, rest string) {
	if h.Render == nil {
		problem(w, 501, "preview_unavailable", "Content rendering is not enabled.")
		return
	}
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		problem(w, 405, "method_not_allowed", "Preview is read-only.")
		return
	}
	// Shape: /preview/<kind>/<id>?format=
	parts := strings.SplitN(strings.Trim(rest, "/"), "/", 2)
	if len(parts) != 2 || parts[1] == "" {
		problem(w, 400, "invalid_request", "Preview needs kind and id.")
		return
	}
	format := subscription.Format(r.URL.Query().Get("format"))
	if q := r.URL.Query().Get("format"); q != "" && !subscription.Formats[format] {
		problem(w, 422, "invalid_format", "Unsupported preview format.")
		return
	}
	if format == "" {
		format = subscription.FormatBase64
	}
	kind := parts[0]
	if kind != "group" && kind != "subscription" {
		problem(w, 404, "not_found", "Unknown preview kind.")
		return
	}
	payload, err := h.Render(kind, parts[1], format)
	if err != nil {
		problem(w, 404, "render_failed", "The requested content cannot be rendered.")
		return
	}
	w.Header().Set("Content-Type", format.ContentType())
	w.WriteHeader(200)
	_, _ = w.Write(payload)
}

/* — backup — */

func (h *Handler) serveBackup(w http.ResponseWriter, r *http.Request, rest string) {
	switch {
	case r.Method == http.MethodGet && rest == "" && h.Export != nil:
		// Default export never includes secrets; the query flag is
		// the explicit opt-in on an authenticated channel.
		backup, err := h.Export(r.URL.Query().Get("secrets") == "include")
		if err != nil {
			problem(w, 503, "backup_failed", err.Error())
			return
		}
		writeJSON(w, 200, backup)
	case r.Method == http.MethodPost && rest == "/preview" && h.PreviewRestore != nil:
		body, err := io.ReadAll(io.LimitReader(r.Body, 8<<20))
		if err != nil {
			problem(w, 400, "invalid_request", "Cannot read the backup document.")
			return
		}
		var b xray.Backup
		if err := json.Unmarshal(body, &b); err != nil {
			problem(w, 400, "invalid_request", "The document is not a backup.")
			return
		}
		preview, err := h.PreviewRestore(b)
		if err != nil {
			problem(w, 422, "invalid_backup", err.Error())
			return
		}
		writeJSON(w, 200, map[string]any{"preview": preview})
	default:
		problem(w, 404, "not_found", "Unknown backup operation.")
	}
}

/* — events / diagnostics — */

func (h *Handler) serveEvents(w http.ResponseWriter, r *http.Request) {
	if h.Events == nil {
		problem(w, 501, "events_unavailable", "The insight log is not enabled.")
		return
	}
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		problem(w, 405, "method_not_allowed", "Events are read-only.")
		return
	}
	q := r.URL.Query()
	var kinds []xray.EventKind
	for _, k := range q["kind"] {
		kinds = append(kinds, xray.EventKind(k))
	}
	var since time.Time
	if s := q.Get("since"); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			since = t
		}
	}
	limit := 0
	if v := q.Get("limit"); v != "" {
		fmt2 := 0
		for _, c := range v {
			if c < '0' || c > '9' {
				fmt2 = -1
				break
			}
			fmt2 = fmt2*10 + int(c-'0')
		}
		if fmt2 >= 0 {
			limit = fmt2
		}
	}
	writeJSON(w, 200, h.Events.Query(kinds, q.Get("subject"), since, limit))
}

func (h *Handler) serveDiagnostics(w http.ResponseWriter, r *http.Request) {
	if h.Diagnose == nil {
		problem(w, 501, "diagnostics_unavailable", "Diagnostics are not enabled.")
		return
	}
	subject := r.URL.Query().Get("subject")
	diags, err := h.Diagnose(subject)
	if err != nil {
		problem(w, 404, "diagnostics_failed", err.Error())
		return
	}
	writeJSON(w, 200, diags)
}

/* — shared helpers — */

func decodeBody(r *http.Request, v any) error {
	if r.ContentLength > 1<<20 {
		return errors.New("payload exceeds 1 MiB")
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		return err
	}
	return json.Unmarshal(body, v)
}

func extractToken(r *http.Request) string {
	v := r.Header.Get("Authorization")
	if !strings.HasPrefix(v, "Bearer ") {
		return ""
	}
	return strings.TrimPrefix(v, "Bearer ")
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func problem(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}

// serveStorage exposes the degraded-storage status and the
// controlled reconciliation entry point.
func (h *Handler) serveStorage(w http.ResponseWriter, r *http.Request, rest string) {
	switch {
	case r.Method == http.MethodGet && rest == "" && h.StorageStatus != nil:
		writeJSON(w, 200, map[string]any{"domains": h.StorageStatus()})
	case r.Method == http.MethodPost && strings.TrimPrefix(rest, "/") == "reconcile" && h.ReconcileStorage != nil:
		result, err := h.ReconcileStorage()
		if err != nil {
			problem(w, 409, "reconcile_failed", err.Error())
			return
		}
		writeJSON(w, 200, map[string]string{"result": result})
	default:
		problem(w, 404, "not_found", "Unknown storage operation.")
	}
}

// tokenErrorCode keeps the structured non-success codes distinct:
// an uncertain commit is a degraded-storage condition, not a plain
// persistence failure.
func tokenErrorCode(err error) string {
	if strings.Contains(strings.ToLower(err.Error()), "uncertain") {
		return "token_commit_uncertain"
	}
	return "token_persist_failed"
}
