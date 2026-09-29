package network

import (
	"os"
	"testing"
)

// PrivateTempDir creates a temp directory that the production
// artifact directory check will accept. t.TempDir() creates a
// directory with mode 0775 on Go 1.27, which OpenArtifactDirectory
// refuses because the group write bit is set. The helper chmods
// the result to 0700 so callers can pass it directly to
// OpenArtifactDirectory without breaking the production safety
// check.
func PrivateTempDir(t *testing.T) string {
	t.Helper()
	d := t.TempDir()
	if err := os.Chmod(d, 0o700); err != nil {
		t.Fatal(err)
	}
	return d
}
