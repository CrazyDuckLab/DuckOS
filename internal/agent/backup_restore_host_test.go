package agent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CrazyDuckLab/DuckOS/internal/backup"
)

func TestFileRestorerPartialFailureRollsBackMutatedFiles(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "a.conf"), []byte("candidate"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dst, "a.conf"), []byte("before"), 0o600); err != nil {
		t.Fatal(err)
	}
	keys := []string{"a.conf", "missing.conf"}
	snapshot, err := fileSnapshotter(dst, keys)(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	restorer := fileRestorer(src, dst, keys)
	if err := restorer(context.Background(), snapshot, backup.DirectionApply); err == nil {
		t.Fatal("apply unexpectedly succeeded with a missing backup source")
	}
	if err := restorer(context.Background(), snapshot, backup.DirectionRollback); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dst, "a.conf"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "before" {
		t.Fatalf("partial restore did not roll back first file: %q", string(data))
	}
	if _, err := os.Stat(filepath.Join(dst, "missing.conf")); !os.IsNotExist(err) {
		t.Fatalf("rollback left a file that did not exist before: %v", err)
	}
}

func TestBackupRestoreHostRejectsRootsOutsideConfiguredTrust(t *testing.T) {
	store, err := backup.OpenJournalStore(privateTempDir(t))
	if err != nil {
		t.Fatal(err)
	}
	exec, err := backup.NewExecutor(store)
	if err != nil {
		t.Fatal(err)
	}
	host := NewBackupRestoreHost(exec, store)
	backupRoot := privateTempDir(t)
	liveRoot := privateTempDir(t)
	if err := host.SetTrustedRoots(backupRoot, liveRoot); err != nil {
		t.Fatal(err)
	}
	otherRoot := privateTempDir(t)
	body := `{"backupId":"backup-0001","sourceRevision":"rev-1","liveRevision":"rev-1","selectedKeys":["a.conf"],"backupRoot":"` + otherRoot + `","liveRoot":"` + liveRoot + `"}`
	req := httptest.NewRequest(http.MethodPost, BackupRestorePath, strings.NewReader(body))
	rec := httptest.NewRecorder()
	host.ServeRestore(rec, req)
	if rec.Code != 422 {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	if len(store.All()) != 0 {
		t.Fatal("untrusted-root request reached restore journal")
	}
}

func privateTempDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	return dir
}
