// G08 CLI provenance torture regression.
package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestProvenanceCmd_HelpWhenMissingFlags(t *testing.T) {
	if err := provenanceCmd([]string{}); err == nil {
		t.Fatalf("expected error when --kind/--id missing")
	}
	if err := provenanceCmd([]string{"--kind", "vlan"}); err == nil {
		t.Fatalf("expected error when --id missing")
	}
	if err := provenanceCmd([]string{"--help"}); err != nil {
		t.Fatalf("--help must be benign, got %v", err)
	}
}

func TestProvenanceCmd_RejectsInvalidID(t *testing.T) {
	if err := provenanceCmd([]string{"--kind", "vlan", "--id", "bad id!"}); err == nil {
		t.Fatalf("expected error for invalid id")
	}
}

func TestProvenanceCmd_RoundTripsJSON(t *testing.T) {
	srv := newUnixSocketServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("kind") != "vlan" || r.URL.Query().Get("id") != "vlan-10" {
			http.Error(w, "bad query", http.StatusBadRequest)
			return
		}
		body := `{"version":1,"updatedAt":"2026-01-01T00:00:00Z","ownership":{"generator":"template","manager":"operator","source":"vpn-defaults","firstSeen":"2026-01-01T00:00:00Z","updatedAt":"2026-01-01T00:00:00Z"},"history":[{"kind":"created","actor":"agent","at":"2026-01-01T00:00:00Z"}]}`
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	}))
	t.Setenv("DUCK_TOKEN_FILE", writeTestToken(t))
	if err := runProvenanceAgainstAgent(srv); err != nil {
		t.Fatalf("provenanceCmd: %v", err)
	}
}

func TestProvenanceCmd_RoundTripsTable(t *testing.T) {
	srv := newUnixSocketServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := `{"version":1,"updatedAt":"2026-01-01T00:00:00Z","ownership":{"generator":"template","manager":"operator","source":"vpn-defaults","firstSeen":"2026-01-01T00:00:00Z","updatedAt":"2026-01-01T00:00:00Z"},"history":[{"kind":"created","actor":"agent","at":"2026-01-01T00:00:00Z"}]}`
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	}))
	t.Setenv("DUCK_TOKEN_FILE", writeTestToken(t))
	if err := runProvenanceAgainstAgent(srv, "--output", "table"); err != nil {
		t.Fatalf("provenanceCmd table: %v", err)
	}
}

func TestProvenanceCmd_HTTPError(t *testing.T) {
	srv := newUnixSocketServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"x"}`, http.StatusServiceUnavailable)
	}))
	t.Setenv("DUCK_TOKEN_FILE", writeTestToken(t))
	if err := runProvenanceAgainstAgent(srv); err == nil {
		t.Fatalf("expected HTTP error")
	}
}

func TestProvenanceCmd_EmptyOwnershipAndHistory(t *testing.T) {
	srv := newUnixSocketServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"version":1,"updatedAt":"2026-01-01T00:00:00Z"}`)
	}))
	t.Setenv("DUCK_TOKEN_FILE", writeTestToken(t))
	if err := runProvenanceAgainstAgent(srv); err != nil {
		t.Fatalf("provenanceCmd: %v", err)
	}
}

// TestProvenanceCmd_RejectsAgentSocketMissing verifies the
// fail-closed behaviour from H10: when --agent-socket is
// not provided the CLI refuses the /agent/v1/* request
// rather than silently falling through to the public
// HTTP server.
func TestProvenanceCmd_RejectsAgentSocketMissing(t *testing.T) {
	t.Setenv("DUCK_AGENT_SOCKET", "")
	if err := runProvenanceAgainstPublic("http://127.0.0.1:1"); err == nil {
		t.Fatalf("missing agent-socket must yield explicit error")
	}
}

func TestProvenanceCmd_UnknownOutputFormat(t *testing.T) {
	t.Setenv("DUCK_TOKEN_FILE", writeTestToken(t))
	if err := runProvenanceAgainstPublic("http://127.0.0.1:1", "--output", "xml"); err == nil {
		t.Fatalf("unknown format must be rejected")
	}
}

func TestProvenanceResponse_RoundTripJSON(t *testing.T) {
	in := provenanceResponse{
		Version:   1,
		UpdatedAt: time.Unix(0, 0).UTC(),
		Ownership: &provenanceOwnership{
			Generator: "template",
			Manager:   "operator",
			Tags:      []string{"a", "b"},
			FirstSeen: time.Unix(0, 0).UTC(),
			UpdatedAt: time.Unix(0, 0).UTC(),
		},
		History: []provenanceEvent{{Kind: "created", Actor: "agent", At: time.Unix(0, 0).UTC()}},
	}
	b, err := json.Marshal(&in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back provenanceResponse
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back.Ownership == nil || back.Ownership.Generator != "template" {
		t.Fatalf("ownership drift: %+v", back.Ownership)
	}
	if len(back.History) != 1 || back.History[0].Kind != "created" {
		t.Fatalf("history drift: %+v", back.History)
	}
}

// runProvenanceAgainstAgent wires the supplied Unix-socket
// httptest server into DUCK_AGENT_SOCKET and invokes the
// provenance CLI command.
func runProvenanceAgainstAgent(srv *httptest.Server, extra ...string) error {
	addr := srv.Listener.Addr().String()
	_ = os.Setenv("DUCK_AGENT_SOCKET", addr)
	args := []string{
		"--endpoint", "http://127.0.0.1:1",
		"--allow-insecure",
		"--agent-socket", addr,
		"--kind", "vlan",
		"--id", "vlan-10",
	}
	args = append(args, extra...)
	return provenanceCmd(args)
}

// runProvenanceAgainstPublic invokes the provenance CLI
// targeting only the public HTTP endpoint. Used by tests
// that must NOT require a live agent.
func runProvenanceAgainstPublic(endpoint string, extra ...string) error {
	args := []string{
		"--endpoint", endpoint,
		"--allow-insecure",
		"--kind", "vlan",
		"--id", "vlan-10",
	}
	args = append(args, extra...)
	return provenanceCmd(args)
}

// writeTestToken writes a synthetic bearer token to a
// temporary file and returns the path so DUCK_TOKEN_FILE
// can point at it.
func writeTestToken(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := dir + "/token"
	if err := os.WriteFile(path, []byte(strings.Repeat("a", 40)), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
