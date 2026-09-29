// G17 / DV08 production-path bridge. The Agent owns
// the durable JournalStore and the Executor; the
// restore host exposes a single IPC route that runs
// the canonical validate -> snapshot -> apply -> commit
// lifecycle. The host wires the Snapshotter + Restorer
// callbacks from the IPC payload (selected keys +
// backup root + live root) so the public caller does
// not have to inject Go callbacks over the wire.
//
// The contract mirrors DV04-XRAY: the public /api/v1/
// backup/restore route reaches the Agent through the
// Engine's Unix-socket client; the Agent runs the
// Executor; the durable journal entry is observable
// from an independent process (cross-process kill is
// exercised in tests/layer_l/dv08_production_test.go).
package agent

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/backup"
)

// BackupRestorePath is the IPC route the Agent
// exposes for restore execution. POST only.
const BackupRestorePath = "/agent/v1/backup/restore"

// BackupRestoreMaxBytes caps the restore request
// payload (selected keys + paths).
const BackupRestoreMaxBytes = 64 * 1024

// backupRestoreRequest is the wire shape of a
// restore. SourceRevision and LiveRevision must
// match (the audit gate requireSafe enforces this).
// SelectedKeys is the subset the caller wants
// restored; BackupRoot carries the files; LiveRoot
// is the destination.
type backupRestoreRequest struct {
	BackupID       string   `json:"backupId"`
	SourceRevision string   `json:"sourceRevision"`
	LiveRevision   string   `json:"liveRevision"`
	SelectedKeys   []string `json:"selectedKeys"`
	BackupRoot     string   `json:"backupRoot"`
	LiveRoot       string   `json:"liveRoot"`
}

// backupRestoreResponse is the wire shape of the
// restore outcome. The EntryID lets the caller
// observe the durable journal row (the independent
// oracle verifies it from a separate process).
type backupRestoreResponse struct {
	Version   int                   `json:"version"`
	UpdatedAt time.Time             `json:"updatedAt"`
	Outcome   backup.RestoreOutcome `json:"outcome"`
	EntryID   string                `json:"entryId"`
	Errors    []string              `json:"errors,omitempty"`
}

// RestoreExecutor is the production runtime the
// host drives. The Agent composition root wires a
// real *backup.Executor; the host never invents a
// default — a nil executor returns 503 just like
// every other privileged surface.
type RestoreExecutor interface {
	Execute(ctx context.Context, entry backup.RestoreEntry, plan backup.RestorePlan, sourceRevision, liveRevision string, snap backup.Snapshotter, restore backup.Restorer) (backup.RestorerResult, error)
}

// BackupRestoreHost wraps a RestoreExecutor + a
// JournalStore (so it can build canonical entry IDs
// the production Executor accepts).
type BackupRestoreHost struct {
	mu         sync.Mutex
	executor   RestoreExecutor
	store      *backup.JournalStore
	clock      func() time.Time
	backupRoot string
	liveRoot   string
}

// NewBackupRestoreHost returns a host bound to the
// production Executor and JournalStore. Either nil
// is a programming error: the host returns nil so the
// Agent composition root refuses to register it.
func NewBackupRestoreHost(exec RestoreExecutor, store *backup.JournalStore) *BackupRestoreHost {
	if exec == nil || store == nil {
		return nil
	}
	return &BackupRestoreHost{executor: exec, store: store, clock: func() time.Time { return time.Now().UTC() }}
}

func (h *BackupRestoreHost) SetTrustedRoots(backupRoot, liveRoot string) error {
	backupRoot, err := validateTrustedRoot(backupRoot)
	if err != nil {
		return err
	}
	liveRoot, err = validateTrustedRoot(liveRoot)
	if err != nil {
		return err
	}
	h.backupRoot = backupRoot
	h.liveRoot = liveRoot
	return nil
}

// ServeRestore runs the canonical restore lifecycle.
// POST only; the body is a backupRestoreRequest; the
// response is a backupRestoreResponse.
func (h *BackupRestoreHost) ServeRestore(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.executor == nil {
		writeRestoreProblem(w, 503, "backup_restore_unavailable", "Backup restore is not enabled.")
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		writeRestoreProblem(w, 405, "method_not_allowed", "Restore endpoint accepts POST only.")
		return
	}
	if r.ContentLength > BackupRestoreMaxBytes {
		writeRestoreProblem(w, 413, "payload_too_large", "Restore payload exceeds the Agent cap.")
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, BackupRestoreMaxBytes+1))
	if err != nil {
		writeRestoreProblem(w, 400, "invalid_request", "Could not read restore payload.")
		return
	}
	if len(body) > BackupRestoreMaxBytes {
		writeRestoreProblem(w, 413, "payload_too_large", "Restore payload exceeds the Agent cap.")
		return
	}
	var req backupRestoreRequest
	dec := json.NewDecoder(strings.NewReader(string(body)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeRestoreProblem(w, 400, "invalid_request", "Restore payload is not canonical JSON.")
		return
	}
	if err := validateRestoreRequest(&req); err != nil {
		writeRestoreProblem(w, 422, "invalid_restore", err.Error())
		return
	}
	if err := h.validateRequestRoots(req.BackupRoot, req.LiveRoot); err != nil {
		writeRestoreProblem(w, 422, "invalid_restore", err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	plan := backup.RestorePlan{
		BackupID:    req.BackupID,
		Adds:        append([]string(nil), req.SelectedKeys...),
		SafeToApply: true,
	}
	entryID := buildRestoreEntryID(req.BackupID, h.clock())
	entry := backup.RestoreEntry{
		ID:             entryID,
		BackupID:       req.BackupID,
		Outcome:        backup.OutcomeStarted,
		SourceRevision: req.SourceRevision,
		LiveRevision:   req.LiveRevision,
		StartedAt:      h.clock(),
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	snap := fileSnapshotter(req.LiveRoot, req.SelectedKeys)
	rest := fileRestorer(req.BackupRoot, req.LiveRoot, req.SelectedKeys)
	result, execErr := h.executor.Execute(ctx, entry, plan, req.SourceRevision, req.LiveRevision, snap, rest)
	out := backupRestoreResponse{
		Version:   1,
		UpdatedAt: h.clock(),
		EntryID:   entryID,
	}
	if execErr != nil {
		out.Outcome = backup.OutcomeFailed
		out.Errors = append(out.Errors, execErr.Error())
		writeRestoreJSON(w, 422, out)
		return
	}
	out.Outcome = result.Entry.Outcome
	writeRestoreJSON(w, 200, out)
}

// fileSnapshotter captures the bytes of every
// selected key under liveRoot so the rollback path
// (which the Executor does NOT trigger on its own;
// the rollback is the operator's responsibility) can
// put the live state back. The snapshot is opaque to
// the Executor.
func fileSnapshotter(liveRoot string, keys []string) backup.Snapshotter {
	return func(ctx context.Context) (any, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		snap := map[string]fileSnapshot{}
		for _, k := range keys {
			path := filepath.Join(liveRoot, k)
			data, err := os.ReadFile(path)
			if err != nil {
				if os.IsNotExist(err) {
					snap[k] = fileSnapshot{}
					continue
				}
				return nil, err
			}
			snap[k] = fileSnapshot{Exists: true, Data: data}
		}
		return snap, nil
	}
}

type fileSnapshot struct {
	Exists bool
	Data   []byte
}

// fileRestorer applies the restore (DirectionApply):
// copy each selected key from backupRoot to liveRoot,
// creating parent directories with 0700 mode. The
// rollback direction restores the captured pre-apply
// bytes and removes files that did not exist before
// this restore attempt.
func fileRestorer(backupRoot, liveRoot string, keys []string) backup.Restorer {
	return func(ctx context.Context, snapshot any, direction backup.RestoreDirection) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		switch direction {
		case backup.DirectionApply:
			for _, k := range keys {
				src := filepath.Join(backupRoot, k)
				dst := filepath.Join(liveRoot, k)
				data, err := os.ReadFile(src)
				if err != nil {
					return err
				}
				if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
					return err
				}
				if err := os.WriteFile(dst, data, 0o600); err != nil {
					return err
				}
			}
		case backup.DirectionRollback:
			snap, ok := snapshot.(map[string]fileSnapshot)
			if !ok {
				return errors.New("restore snapshot has unexpected shape")
			}
			for _, k := range keys {
				dst := filepath.Join(liveRoot, k)
				before := snap[k]
				if !before.Exists {
					if err := os.Remove(dst); err != nil && !os.IsNotExist(err) {
						return err
					}
					continue
				}
				if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
					return err
				}
				if err := os.WriteFile(dst, before.Data, 0o600); err != nil {
					return err
				}
			}
		default:
			return errors.New("unsupported restore direction")
		}
		return nil
	}
}

func (h *BackupRestoreHost) validateRequestRoots(backupRoot, liveRoot string) error {
	if h.backupRoot == "" && h.liveRoot == "" {
		return nil
	}
	backupRoot, err := validateTrustedRoot(backupRoot)
	if err != nil {
		return err
	}
	liveRoot, err = validateTrustedRoot(liveRoot)
	if err != nil {
		return err
	}
	if backupRoot != h.backupRoot || liveRoot != h.liveRoot {
		return errors.New("restore roots do not match configured trusted roots")
	}
	return nil
}

func validateTrustedRoot(root string) (string, error) {
	if root == "" {
		return "", errors.New("restore root is required")
	}
	clean, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return "", err
	}
	info, err := os.Lstat(clean)
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return "", errors.New("restore root must be a real directory, not a symlink")
	}
	return clean, nil
}

// validateRestoreRequest enforces the audit gates:
// canonical backup id (logicalIDPattern), selected
// keys non-empty, no empty roots, no path traversal.
func validateRestoreRequest(req *backupRestoreRequest) error {
	if !logicalIDPattern.MatchString(req.BackupID) {
		return errors.New("backupId is not canonical")
	}
	if req.SourceRevision == "" || req.LiveRevision == "" {
		return errors.New("sourceRevision and liveRevision are required")
	}
	if req.SourceRevision != req.LiveRevision {
		return errors.New("liveRevision does not match sourceRevision")
	}
	if len(req.SelectedKeys) == 0 {
		return errors.New("selectedKeys must list at least one key")
	}
	if req.BackupRoot == "" || req.LiveRoot == "" {
		return errors.New("backupRoot and liveRoot are required")
	}
	for _, k := range req.SelectedKeys {
		if k == "" {
			return errors.New("selectedKeys contains an empty key")
		}
		if strings.Contains(k, "..") || strings.HasPrefix(k, "/") {
			return errors.New("selectedKeys path traversal is refused")
		}
	}
	return nil
}

// logicalIDPattern is duplicated from the backup
// package so the host validates before constructing
// any internal state. Same regex as
// backup.LogicalManifest.Valid().
var logicalIDPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:-]{0,63}$`)

// buildRestoreEntryID is the canonical entry id the
// Executor (and the JournalStore's per-id dedup)
// accept. entryIDPattern in restore_lifecycle.go
// requires 8-64 chars of lowercase alnum + .-_;
// we synthesise <backup>-<yyyymmddhhmmss> so the id
// is unique per restore attempt.
func buildRestoreEntryID(backupID string, now time.Time) string {
	stamp := now.UTC().Format("20060102150405")
	return backupID + "-" + stamp
}

func writeRestoreJSON(w http.ResponseWriter, status int, payload any) {
	data, err := json.Marshal(payload)
	if err != nil {
		writeRestoreProblem(w, 503, "encode_failed", "Restore response could not be encoded.")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = w.Write(data)
}

func writeRestoreProblem(w http.ResponseWriter, status int, code, message string) {
	writeRestoreJSON(w, status, map[string]any{
		"error": map[string]string{"code": code, "message": message},
	})
}
