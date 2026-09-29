// Test helpers shared across the Duck CLI package. The
// CLI commands that target /agent/v1/* now go through the
// private Agent Unix socket transport. Tests spin up a
// short-lived httptest server on a temporary Unix socket
// so the dispatch matches production.
package main

import (
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// newUnixSocketServer starts an httptest server on a
// temporary Unix socket. The returned *httptest.Server
// speaks HTTP over the socket; callers wire the path
// into DUCK_AGENT_SOCKET so the CLI dispatches to it.
//
// macOS has a 104-byte limit on Unix socket paths so the
// helper uses os.TempDir() directly with a short socket
// name to stay under the limit.
func newUnixSocketServer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()
	base := os.TempDir()
	sock := filepath.Join(base, "duckos-test-agent.sock")
	// Clean up any stale socket from a previous run.
	_ = os.Remove(sock)
	listener, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen unix: %v", err)
	}
	srv := &httptest.Server{
		Listener: listener,
		Config:   &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second},
	}
	srv.Start()
	t.Cleanup(func() {
		srv.Close()
		_ = os.Remove(sock)
	})
	return srv
}
