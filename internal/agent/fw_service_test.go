//go:build linux

// Journal-first firewall service tests. The nft binary is a
// shell-script fixture: it validates (exit 0) and records the
// batches it "applies", so the full safeapply flow (admit →
// apply → verify → confirm/rollback) runs without touching
// the host kernel. Kernel behaviour itself is NOT claimed
// verified here — live qualification remains pending.
package agent

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/fwrules"
	"github.com/CrazyDuckLab/DuckOS/internal/identity"
	"github.com/CrazyDuckLab/DuckOS/internal/wan"
)

// fakeNFT emulates: `nft -c -f <file>` (validate, ok) and
// `nft -f <file>` (apply, ok) plus `nft -j list table ...`
// (readback of the "kernel" table state kept in stateFile).
// mode file: "ok" | "fail-apply".
func writeFakeNFT(t *testing.T, dir, stateFile string) string {
	t.Helper()
	script := filepath.Join(dir, "nft")
	content := `#!/bin/sh
STATE="` + stateFile + `"
case "$1" in
  -c) exit 0 ;;
  -f)
    if [ -f "$STATE.fail" ]; then exit 1; fi
    cp "$2" "$STATE.batch"
    exit 0 ;;
  -j)
    if [ ! -f "$STATE.batch" ]; then exit 1; fi
    # reply with a minimal nftables JSON containing one object
    printf '{"nftables":[{"metainfo":{}},{"rule":{}}]}'
    exit 0 ;;
esac
exit 2
`
	if err := os.WriteFile(script, []byte(content), 0o755); err != nil {
		t.Fatalf("write nft fixture: %v", err)
	}
	return script
}

type acceptAccess struct{}

func (acceptAccess) Authorize(string, identity.Permission) (identity.User, error) {
	return identity.User{}, nil
}

func newTestFwService(t *testing.T) (*FirewallRulesService, string) {
	t.Helper()
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "kernel-state")
	// The journal path is a private, pre-existing directory.
	store := filepath.Join(dir, "store")
	if err := os.MkdirAll(store, 0o700); err != nil {
		t.Fatalf("store dir: %v", err)
	}
	svc, err := NewFirewallRulesService(
		store,
		filepath.Join(dir, "stage"),
		writeFakeNFT(t, dir, stateFile),
		acceptAccess{},
	)
	if err != nil {
		t.Fatalf("service: %v", err)
	}
	t.Cleanup(svc.Close)
	return svc, dir
}

func fwRuleJSON(id string) []byte {
	rule := fwrules.Rule{ID: id, Enabled: true, Action: "accept", Direction: "forward", Protocol: "tcp", DstPort: 8443}
	body, _ := json.Marshal(map[string]any{"rule": rule})
	return body
}

func doReq(t *testing.T, svc *FirewallRulesService, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var reader *strings.Reader
	if body == "" {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Authorization", "Bearer test")
	rec := httptest.NewRecorder()
	svc.ServeHTTP(rec, req)
	return rec
}

func TestFirewallServiceAdmitApplyConfirm(t *testing.T) {
	svc, _ := newTestFwService(t)
	rec := doReq(t, svc, http.MethodPut, "/agent/v1/fwrules/fwrules/v1/rules/allow-lan-web", string(fwRuleJSON("allow-lan-web")))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("admit status: %d body=%s", rec.Code, rec.Body.String())
	}
	var job struct {
		TransactionID string `json:"transactionId"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &job); err != nil || job.TransactionID == "" {
		t.Fatalf("job: %v %s", err, rec.Body.String())
	}
	// Wait briefly for the worker to apply the batch.
	deadline := time.Now().Add(5 * time.Second)
	for {
		st := doReq(t, svc, http.MethodGet, "/agent/v1/fwrules/fwrules/v1/status", "")
		if !bytes.Contains(st.Body.Bytes(), []byte(`"applied"`)) && !strings.Contains(st.Body.String(), "awaiting-confirmation") {
			if time.Now().After(deadline) {
				t.Fatalf("tx never reached confirm window: %s", st.Body.String())
			}
			time.Sleep(50 * time.Millisecond)
			continue
		}
		break
	}
	// Confirm.
	rec = doReq(t, svc, http.MethodPost, "/agent/v1/fwrules/fwrules/v1/confirm", `{"transactionId":"`+job.TransactionID+`"}`)
	if rec.Code != 200 {
		t.Fatalf("confirm: %d %s", rec.Code, rec.Body.String())
	}
	// Status must show the rule as current.
	st := doReq(t, svc, http.MethodGet, "/agent/v1/fwrules/fwrules/v1/status", "")
	if !strings.Contains(st.Body.String(), "allow-lan-web") {
		t.Fatalf("rule not in journal: %s", st.Body.String())
	}
}

func TestFirewallServiceRollback(t *testing.T) {
	svc, _ := newTestFwService(t)
	rec := doReq(t, svc, http.MethodPut, "/agent/v1/fwrules/fwrules/v1/rules/r1", string(fwRuleJSON("r1")))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("admit: %d %s", rec.Code, rec.Body.String())
	}
	var job struct {
		TransactionID string `json:"transactionId"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &job)
	// Roll the transaction back explicitly.
	deadline := time.Now().Add(5 * time.Second)
	for {
		rec = doReq(t, svc, http.MethodPost, "/agent/v1/fwrules/fwrules/v1/rollback", `{"transactionId":"`+job.TransactionID+`"}`)
		if rec.Code == 200 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("rollback never accepted: %d %s", rec.Code, rec.Body.String())
		}
		time.Sleep(50 * time.Millisecond)
	}
	st := doReq(t, svc, http.MethodGet, "/agent/v1/fwrules/fwrules/v1/status", "")
	if strings.Contains(st.Body.String(), `"id": "r1"`) || strings.Contains(st.Body.String(), `"r1","enabled"`) {
		t.Fatalf("rolled-back rule still current: %s", st.Body.String())
	}
}

func TestComposeWANCandidateStaticAndDHCP(t *testing.T) {
	current, err := os.ReadFile("testdata/wan-current.json")
	if err != nil {
		// Fall back to a minimal inline fixture.
		current = []byte(`{"version":1,"interfaces":[{"id":"mgmt","name":"eth-mgmt","role":"lan","kind":"physical","mtu":1500,"addresses":["192.168.1.10/24"],"dhcpClient":"none","management":true}],"routes":[],"firewall":{"masquerade":[],"forwards":[],"rules":[]},"dns":{"upstreams":[],"records":[]}}`)
	}
	// Static.
	// The static address must be a prefix that contains the
	// gateway (a /32 has no connected subnet for the gateway).
	p := wanProviderFixture("up1", "static", "eth-wan", "203.0.113.2/24", "203.0.113.1")
	cand, err := composeWANCandidate(current, p)
	if err != nil {
		t.Fatalf("static compose: %v", err)
	}
	var cfg struct {
		Interfaces []struct {
			ID   string `json:"id"`
			Role string `json:"role"`
			DHCP string `json:"dhcpClient"`
			Addr string `json:"addresses"`
		} `json:"interfaces"`
		Routes []struct {
			Destination string `json:"destination"`
			Gateway     string `json:"gateway"`
		} `json:"routes"`
	}
	if err := json.Unmarshal(cand, &cfg); err != nil {
		t.Fatalf("candidate json: %v", err)
	}
	foundIface, foundRoute := false, false
	for _, i := range cfg.Interfaces {
		if i.ID == "wan-up1" && i.Role == "wan" && i.DHCP == "none" {
			foundIface = true
		}
	}
	for _, r := range cfg.Routes {
		if r.Destination == "0.0.0.0/0" && r.Gateway == "203.0.113.1" {
			foundRoute = true
		}
	}
	if !foundIface || !foundRoute {
		t.Fatalf("static composition incomplete: %s", cand)
	}
	// DHCP mode: no addresses, DHCP client on.
	p2 := wanProviderFixture("up2", "dhcp", "eth-wan2", "", "")
	cand2, err := composeWANCandidate(current, p2)
	if err != nil {
		t.Fatalf("dhcp compose: %v", err)
	}
	if !strings.Contains(string(cand2), `"dhcpClient":"ipv4"`) {
		t.Fatalf("dhcp client not set: %s", cand2)
	}
	// Re-composition replaces the previous entries instead of
	// duplicating them.
	cand3, err := composeWANCandidate(cand, p)
	if err != nil {
		t.Fatalf("recompose: %v", err)
	}
	if strings.Count(string(cand3), `"wan-up1"`) != 1 {
		t.Fatalf("duplicate composition: %s", cand3)
	}
	// PPPoE/L2TP cannot be applied yet.
	p3 := wanProviderFixture("up3", "pppoe", "eth-wan3", "", "")
	if _, err := composeWANCandidate(current, p3); err == nil {
		t.Fatal("pppoe apply must be refused")
	}
}

func wanProviderFixture(id, mode, link, address, gateway string) wan.Provider {
	p := wan.Provider{
		ID: id, Name: id, Link: link, Mode: wan.Mode(mode), Enabled: true,
		MAC: wan.MACConfig{Source: wan.MACSourceHardware},
	}
	if mode == "static" {
		p.Static = &wan.StaticConfig{Address: address, Gateway: gateway}
	}
	return p
}
