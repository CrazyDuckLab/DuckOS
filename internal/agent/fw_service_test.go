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
	"fmt"
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

type acceptAccess struct{}

func (acceptAccess) Authorize(string, identity.Permission) (identity.User, error) {
	return identity.User{}, nil
}

// fakeFwLauncher emulates the privileged nft launcher: validation
// always passes, apply records the batch into the stage file the
// readback seam reads back, and reads return a minimal owned-table
// JSON. Kernel behaviour itself is NOT claimed verified here —
// live qualification runs against the real launcher and kernel
// (fw_live_test.go).
type fakeFwLauncher struct{ stageDir string }

func (f fakeFwLauncher) Available() bool        { return true }
func (f fakeFwLauncher) Check([]byte) error     { return nil }
func (f fakeFwLauncher) CheckJSON([]byte) error { return nil }

func (f fakeFwLauncher) Apply(batch []byte) error {
	return os.WriteFile(filepath.Join(f.stageDir, "duckos-fw.batch.nft"), batch, 0o600)
}
func (f fakeFwLauncher) ApplyJSON(batch []byte) error { return f.Apply(batch) }

func (f fakeFwLauncher) ListTableJSON(_, _ string) ([]byte, error) {
	return []byte(`{"nftables":[{"metainfo":{}},{"rule":{}}]}`), nil
}
func (f fakeFwLauncher) ListRulesetJSON() ([]byte, error) {
	return []byte(`{"nftables":[]}`), nil
}

func newTestFwService(t *testing.T) (*FirewallRulesService, string) {
	t.Helper()
	dir := t.TempDir()
	// The journal path is a private, pre-existing directory.
	store := filepath.Join(dir, "store")
	if err := os.MkdirAll(store, 0o700); err != nil {
		t.Fatalf("store dir: %v", err)
	}
	stage := filepath.Join(dir, "stage")
	prevLauncher := nftClient
	nftClient = func() fwLauncherTransport { return fakeFwLauncher{stageDir: stage} }
	t.Cleanup(func() { nftClient = prevLauncher })
	svc, err := NewFirewallRulesService(
		store,
		stage,
		"",
		acceptAccess{},
	)
	if err != nil {
		t.Fatalf("service: %v", err)
	}
	// The fake nft cannot render kernel-accurate JSON readback,
	// so the readback seam synthesizes the owned-table snapshot
	// from the staged batch the adapter just applied.
	svc.adapter.readbackFn = func(string) ([]json.RawMessage, bool, error) {
		return fwrSnapshotFromStage(stage)
	}
	t.Cleanup(svc.Close)
	return svc, dir
}

// fwrSnapshotFromStage builds an nft JSON snapshot equivalent to
// the last staged batch, honoring the empty-state contract (an
// empty batch means the owned table is absent).
func fwrSnapshotFromStage(stageDir string) ([]json.RawMessage, bool, error) {
	raw, err := os.ReadFile(filepath.Join(stageDir, "duckos-fw.batch.nft"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, true, nil
		}
		return nil, false, err
	}
	var rules []json.RawMessage
	for _, line := range strings.Split(string(raw), "\n") {
		if !strings.HasPrefix(line, "add rule inet duckos_fw forward ") {
			continue
		}
		rule, err := fwrRuleObjectFromLine(line)
		if err != nil {
			return nil, false, err
		}
		rules = append(rules, rule)
	}
	if len(rules) == 0 {
		return nil, true, nil
	}
	objs := []json.RawMessage{
		json.RawMessage(`{"table":{"family":"inet","name":"duckos_fw"}}`),
		json.RawMessage(`{"chain":{"family":"inet","table":"duckos_fw","name":"forward","type":"filter","hook":"forward","prio":0,"policy":"accept"}}`),
	}
	return append(objs, rules...), false, nil
}

// fwrRuleObjectFromLine translates one rendered `add rule` line
// into the nft JSON shape VerifySnapshot decodes.
func fwrRuleObjectFromLine(line string) (json.RawMessage, error) {
	var exprs []map[string]any
	add := func(expr map[string]any) { exprs = append(exprs, expr) }
	tokens := strings.Fields(strings.TrimPrefix(line, "add rule inet duckos_fw forward "))
	for i := 0; i < len(tokens); i++ {
		switch tokens[i] {
		case "ip":
			field := tokens[i+1]
			i += 2
			proto := map[string]any{"protocol": "ip", "field": field + "addr"}
			add(map[string]any{"match": map[string]any{"op": "==", "left": map[string]any{"payload": proto}, "right": tokens[i]}})
		case "iifname", "oifname":
			key := tokens[i]
			value := strings.Trim(tokens[i+1], "\"")
			i += 1
			add(map[string]any{"match": map[string]any{"op": "==", "left": map[string]any{"meta": map[string]any{"key": key}}, "right": value}})
		case "meta":
			// meta l4proto <p>
			i += 2
			add(map[string]any{"match": map[string]any{"op": "==", "left": map[string]any{"meta": map[string]any{"key": "l4proto"}}, "right": tokens[i]}})
		case "tcp", "udp":
			// <proto> dport <n>
			proto := tokens[i]
			i += 2
			add(map[string]any{"match": map[string]any{"op": "==", "left": map[string]any{"payload": map[string]any{"protocol": proto, "field": "dport"}}, "right": json.Number(tokens[i])}})
		case "accept", "drop", "reject":
			add(map[string]any{"verdict": map[string]any{"verdict": tokens[i]}})
		case "comment":
			i += 1
			add(map[string]any{"comment": strings.Trim(tokens[i], "\"")})
		default:
			return nil, fmt.Errorf("unmodelled rule token %q", tokens[i])
		}
	}
	body, err := json.Marshal(map[string]any{"family": "inet", "table": "duckos_fw", "chain": "forward", "handle": 1, "expr": exprs})
	if err != nil {
		return nil, err
	}
	return json.RawMessage(`{"rule":` + string(body) + `}`), nil
}

// txIDOf returns the pending transaction id, empty while the
// worker has not yet picked the admission up.
func txIDOf(t *testing.T, svc *FirewallRulesService) string {
	t.Helper()
	job, _, err := svc.worker.Status()
	if err != nil {
		return ""
	}
	return job.TransactionID
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
	// The transaction id is assigned once the worker starts the
	// apply; poll the status until it is visible.
	deadline := time.Now().Add(5 * time.Second)
	for {
		st := doReq(t, svc, http.MethodGet, "/agent/v1/fwrules/fwrules/v1/status", "")
		if id := txIDOf(t, svc); id != "" {
			job.TransactionID = id
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("tx id never assigned: %s", st.Body.String())
		}
		time.Sleep(50 * time.Millisecond)
	}
	// Wait briefly for the worker to apply the batch.
	deadline = time.Now().Add(5 * time.Second)
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
	// Roll the transaction back explicitly; the id appears once
	// the worker reaches the confirmation window.
	deadline := time.Now().Add(5 * time.Second)
	for {
		if id := txIDOf(t, svc); id != "" {
			job.TransactionID = id
		}
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
			ID   string   `json:"id"`
			Role string   `json:"role"`
			DHCP string   `json:"dhcpClient"`
			Addr []string `json:"addresses"`
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
	if strings.Count(string(cand3), `"id":"wan-up1"`) != 1 {
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
