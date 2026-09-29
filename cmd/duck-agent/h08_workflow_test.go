//go:build linux

package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/CrazyDuckLab/DuckOS/internal/agent"
	"github.com/CrazyDuckLab/DuckOS/internal/upgrade"
)

// This covers production workflow persistence, not executable activation.
func TestH08ProductionWorkflowPersistence(t *testing.T) {
	dir := t.TempDir()
	priv, pub, err := upgrade.GenerateSigningKey()
	if err != nil {
		t.Fatal(err)
	}
	keys := filepath.Join(dir, "keys")
	if err := os.WriteFile(keys, []byte(hex.EncodeToString(pub)), 0600); err != nil {
		t.Fatal(err)
	}
	storeDir := filepath.Join(dir, "releases")
	o := enableUpgradeService(storeDir, keys)
	if o == nil {
		t.Fatal("production composition failed")
	}
	defer func() {
		if o != nil {
			_ = o.Close()
		}
	}()
	h := agent.NewHandler(nil)
	h.EnableUpgrade(agent.NewOrchestratorAdapter(o))
	srv, token := remediationPublicAPI(t, h, strings.Repeat("h", 64))
	call := func(path string, body any, want int) []byte {
		t.Helper()
		raw, _ := json.Marshal(body)
		req, _ := http.NewRequest("POST", srv.URL+"/api/v1/upgrade/"+path, bytes.NewReader(raw))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		out, _ := io.ReadAll(res.Body)
		if res.StatusCode != want {
			t.Fatalf("%s: HTTP %d: %s", path, res.StatusCode, out)
		}
		return out
	}
	payload := []byte("intentionally non-executable candidate")
	digest := sha256.Sum256(payload)
	m := upgrade.Manifest{Format: upgrade.FormatVer, Architecture: runtime.GOARCH, SourceVersion: "v1", TargetVersion: "v1", InputSchema: 1, OutputSchema: 1, PayloadAlgorithm: "sha256", PayloadDigest: hex.EncodeToString(digest[:]), PayloadBytes: int64(len(payload))}
	manifest, _ := json.Marshal(m)
	body := append(ed25519.Sign(priv, manifest), manifest...)
	raw := append((upgrade.Header{Magic: upgrade.Magic, Format: upgrade.FormatVer, BodyBytes: int64(len(body))}).Marshal(), body...)
	raw = append(raw, payload...)
	call("stage", map[string]any{"operator": "owner", "channel": "stable", "releaseId": "candidate", "releaseVersion": "v1", "releaseArch": runtime.GOARCH, "payloadBase64": base64.StdEncoding.EncodeToString(raw)}, 200)
	srv.Close()
	_ = o.Close()
	o = enableUpgradeService(storeDir, keys)
	if o == nil {
		t.Fatal("production reopen failed")
	}
	if st := o.State(); st.Candidate != "candidate" || st.Active != "" {
		t.Fatalf("lost staged candidate: %+v", st)
	}
	h = agent.NewHandler(nil)
	h.EnableUpgrade(agent.NewOrchestratorAdapter(o))
	srv, token = remediationPublicAPI(t, h, strings.Repeat("h", 64))
	call("activate", map[string]any{}, 503)
	if o.State().Active != "" {
		t.Fatal("failed first activation remained active")
	}
	srv.Close()
	_ = o.Close()
	o = enableUpgradeService(storeDir, keys)
	if o == nil {
		t.Fatal("failed activation recovery lost service")
	}
	if o.State().Active != "" || o.State().Candidate != "candidate" {
		t.Fatal("failed activation resurrected after reopen")
	}
}
