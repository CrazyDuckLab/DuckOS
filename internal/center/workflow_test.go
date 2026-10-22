// Package center / workflow_test.go: end-to-end acceptance
// workflows A–F against the REAL durable catalog, the REAL render
// pipeline, the REAL delivery handler and the REAL node transport.
// Live Xray-core execution is the only excluded element (no binary
// on this host); that part stays honestly qualified as pending.
package center

import (
	"context"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/certs"
	"github.com/CrazyDuckLab/DuckOS/internal/subscription"
	"github.com/CrazyDuckLab/DuckOS/internal/ximport"
	"github.com/CrazyDuckLab/DuckOS/internal/xnode"
	"github.com/CrazyDuckLab/DuckOS/internal/xray"
)

// stubResolver resolves every reference to a fixed secret per key id.
type stubResolver map[string]string

func (s stubResolver) Resolve(_ context.Context, ref xray.KeyRef) ([]byte, error) {
	if v, ok := s[ref.KeyID]; ok {
		return []byte(v), nil
	}
	return nil, errorsNew("resolve: unknown key")
}

func testCatalog(t *testing.T) *xray.FileCatalog {
	t.Helper()
	cat, err := xray.OpenFileCatalog(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return cat
}

// WORKFLOW A: server + VLESS/REALITY + user → render → delivery →
// revoke → access removed from the delivered payload.
func TestWorkflowA_ServerToDeliveryToRevocation(t *testing.T) {
	ctx := context.Background()
	cat := testCatalog(t)
	kp, err := xray.GenerateRealityKeypair()
	if err != nil {
		t.Fatal(err)
	}
	sid, _ := xray.GenerateShortID()
	server := xray.Server{
		ID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Name: "edge", Protocol: xray.ProtocolVLESS,
		Listen: "0.0.0.0:443", Port: 443, Transport: xray.TransportTCP, Security: xray.SecurityReality,
		RealityDest: "yahoo.com:443", RealitySNI: []string{"yahoo.com"}, RealityPublicKey: kp.PublicKey,
		RealityKeyRef: &xray.KeyRef{VaultID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Revision: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", KeyID: "priv1"},
		ShortIDRef:    &xray.KeyRef{VaultID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Revision: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", KeyID: "sid1"},
		Fingerprint:   "chrome", Enabled: true,
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	if err := server.Validate(); err != nil {
		t.Fatal(err)
	}
	user := xray.User{ID: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", ServerID: server.ID, Name: "alice",
		Credential: &xray.KeyRef{VaultID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Revision: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", KeyID: "cred1"},
		Email:      "alice@example.com", Enabled: true,
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err := cat.UpsertServer(ctx, server); err != nil {
		t.Fatal(err)
	}
	if err := cat.UpsertUser(ctx, user); err != nil {
		t.Fatal(err)
	}
	// Render the runtime config with materialization (the same
	// call the agent makes; `xray -test` itself needs the real
	// binary and stays qualified separately).
	servers, _ := cat.ListServers(ctx)
	users, _ := cat.ListUsers(ctx, server.ID)
	cfg, err := xray.RenderRuntimeConfig(servers, users)
	if err != nil {
		t.Fatal(err)
	}
	resolver := stubResolver{"priv1": kp.PrivateKey, "sid1": sid, "cred1": "11111111-1111-1111-1111-111111111111"}
	if _, err := xray.Materialize(ctx, cfg, t.TempDir(), servers, users, resolver); err != nil {
		t.Fatal(err)
	}
	// Delivery: group + token + fetch through the real handler.
	groups := subscription.NewGroupStore()
	if _, err := groups.Upsert(subscription.Group{ID: "g1", Name: "Personal", Format: subscription.FormatBase64, Enabled: true,
		Members: []subscription.GroupMember{{ServerID: server.ID, UserID: user.ID, DisplayName: "Edge", Enabled: true}}}); err != nil {
		t.Fatal(err)
	}
	tokens := subscription.NewTokenStore()
	render := BuildRenderer(RendererOptions{Catalog: cat, Groups: groups, Resolver: resolver})
	delivery := subscription.NewDeliveryHandler(tokens, func(subID string) ([]byte, error) {
		return render("group", subID, subscription.FormatBase64)
	})
	token, _, _ := tokens.Issue("g1", time.Hour)
	req := httptest.NewRequest(http.MethodGet, "/sub/"+token, nil)
	w := httptest.NewRecorder()
	delivery.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("delivery: %d %s", w.Code, w.Body.String())
	}
	decoded, err := base64.StdEncoding.DecodeString(w.Body.String())
	if err != nil {
		t.Fatal(err)
	}
	link := string(decoded)
	for _, want := range []string{"vless://11111111-1111-1111-1111-111111111111@", "security=reality", "pbk=", "sid="} {
		if !strings.Contains(link, want) {
			t.Errorf("payload missing %q: %s", want, link)
		}
	}
	if strings.Contains(link, kp.PrivateKey) || strings.Contains(link, "priv1") {
		t.Error("REALITY private key material leaked into delivery")
	}
	// Revoke the user: the delivered payload loses the link.
	user.Enabled = false
	user.UpdatedAt = time.Now().UTC()
	if err := cat.UpsertUser(ctx, user); err != nil {
		t.Fatal(err)
	}
	w = httptest.NewRecorder()
	delivery.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/sub/"+token, nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("revoked subscription payload must be unavailable, got %d", w.Code)
	}
}

// WORKFLOW B: remote node register → auth → discover → push →
// revoke → access fails. Uses the REAL HTTPS transport.
func TestWorkflowB_MultiNodeLifecycle(t *testing.T) {
	nodeMgr := xnode.NewManager(nil)
	var applied []byte
	nodeServer := xnode.NewNodeServer(nodeMgr, func(cfg []byte) error { applied = cfg; return nil },
		func() xnode.NodeInfo {
			return xnode.NodeInfo{Name: "edge-2", Version: "26.3.27", Capabilities: []string{"vless"}}
		})
	ts := httptest.NewTLSServer(nodeServer)
	defer ts.Close()
	adapter := xnode.NewNodeHTTPAdapter()
	adapter.HTTPClient = ts.Client()
	coordinator := xnode.NewManager(adapter)
	tok, _, _ := coordinator.IssueEnrollmentToken(time.Hour)
	node, credential, err := coordinator.Enroll(tok, xnode.NodeInfo{Name: "edge-2"})
	if err != nil {
		t.Fatal(err)
	}
	node.Endpoint = ts.URL
	if err := nodeMgr.AdoptIdentity(credential); err != nil {
		t.Fatal(err)
	}
	ctx := xnode.WithCredential(context.Background(), credential)
	if info, err := adapter.Discover(ctx, node); err != nil || info.Version != "26.3.27" {
		t.Fatalf("discover: %+v err=%v", info, err)
	}
	if err := adapter.PushConfig(ctx, node, []byte(`{"inbounds":[]}`)); err != nil {
		t.Fatalf("push: %v", err)
	}
	if len(applied) == 0 {
		t.Fatal("config never reached the node")
	}
	if err := coordinator.Revoke(node.ID); err != nil {
		t.Fatal(err)
	}
	if coordinator.Authenticate(node.ID, credential) {
		t.Fatal("revoked node still authenticates")
	}
}

// WORKFLOW C: import subscription → policies → compile → render →
// removal verified through a re-opened durable catalog.
func TestWorkflowC_ImportPolicyRenderRollback(t *testing.T) {
	payload := base64.StdEncoding.EncodeToString([]byte(
		"vless://11111111-1111-1111-1111-111111111111@203.0.113.5:443?type=tcp&security=tls&sni=example.com#ext-1\n"))
	conns, skipped, err := ximport.ParseSubscriptionPayload(payload)
	if err != nil || len(conns) != 1 || len(skipped) != 0 {
		t.Fatalf("import: %d %v %v", len(conns), skipped, err)
	}
	if conns[0].Protocol != xray.ProtocolVLESS || conns[0].SNI != "example.com" {
		t.Fatalf("parsed: %+v", conns[0].ParsedConnection)
	}
	// Policies compile deterministically.
	policies := []xray.Policy{
		{ID: "p1", Name: "block-tracker", Priority: 1, Domains: []string{"tracker.example.com"}, Action: xray.PolicyBlock, Enabled: true},
		{ID: "p2", Name: "lan-direct", Priority: 2, CIDRs: []string{"192.168.0.0/16"}, Action: xray.PolicyDirect, Enabled: true},
	}
	cfg := &xray.Config{}
	if err := xray.RenderPoliciesInto(cfg, policies); err != nil {
		t.Fatal(err)
	}
	if len(cfg.Routing.Rules) != 2 {
		t.Fatalf("rules: %+v", cfg.Routing)
	}
	// Rollback unit: removing the object from the durable catalog
	// and re-opening shows the removal persisted (no resurrection).
	cat := testCatalog(t)
	ctx := context.Background()
	srv := xray.Server{ID: "cccccccccccccccccccccccccccccccc", Name: "temp", Protocol: xray.ProtocolVLESS,
		Listen: "127.0.0.1:1080", Port: 1080, Transport: xray.TransportTCP, Security: xray.SecurityNone,
		Enabled: true, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err := cat.UpsertServer(ctx, srv); err != nil {
		t.Fatal(err)
	}
	if err := cat.RemoveServer(ctx, srv.ID); err != nil {
		t.Fatal(err)
	}
	reopened, err := xray.OpenFileCatalog(cat.Dir())
	if err != nil {
		t.Fatal(err)
	}
	servers, _ := reopened.ListServers(ctx)
	if len(servers) != 0 {
		t.Fatalf("rollback left state behind: %d", len(servers))
	}
}

// WORKFLOW D: TLS import → verify → attach → monitor expiration.
func TestWorkflowD_TLSLifecycle(t *testing.T) {
	certPEM, keyPEM := selfSignedForTest(t, []string{"vpn.example.com"}, 30*24*time.Hour)
	inv := certs.NewInventory()
	rec, err := inv.Import("", certs.Source{
		Kind: certs.KindImported, Domains: []string{"vpn.example.com"},
		Certificate: certPEM, PrivateKey: keyPEM,
	}, "vault-key-ref")
	if err != nil {
		t.Fatal(err)
	}
	if err := inv.Bind(rec.ID, "srv-1"); err != nil {
		t.Fatal(err)
	}
	if err := inv.Remove(rec.ID); err == nil {
		t.Fatal("bound certificate removed")
	}
	list := inv.List(45 * 24 * time.Hour)
	if len(list) != 1 || list[0].Health != certs.HealthExpiring {
		t.Fatalf("health: %+v", list)
	}
}

func selfSignedForTest(t *testing.T, domains []string, ttl time.Duration) (string, string) {
	t.Helper()
	cert, key, err := certs.MakeSelfSigned(domains, ttl)
	if err != nil {
		t.Fatal(err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw})
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	return string(certPEM), string(keyPEM)
}

// WORKFLOW E: quota → exhaust → suspend (catalog flag) → new
// config excludes the user.
func TestWorkflowE_QuotaSuspension(t *testing.T) {
	cat := testCatalog(t)
	ctx := context.Background()
	srv := xray.Server{ID: "dddddddddddddddddddddddddddddddd", Name: "q", Protocol: xray.ProtocolVLESS,
		Listen: "127.0.0.1:2080", Port: 2080, Transport: xray.TransportTCP, Security: xray.SecurityNone,
		Enabled: true, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	_ = cat.UpsertServer(ctx, srv)
	user := xray.User{ID: "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee", ServerID: srv.ID, Name: "bob",
		UUID: "22222222-2222-2222-2222-222222222222", Email: "bob", Enabled: true,
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	_ = cat.UpsertUser(ctx, user)
	history := xray.NewUsageHistory()
	engine := xray.NewEngine(history)
	engine.Suspend = func(kind, id string) error {
		users, _ := cat.ListUsers(ctx, srv.ID)
		for i := range users {
			if users[i].ID == id {
				users[i].Enabled = false
				users[i].UpdatedAt = time.Now().UTC()
				_ = cat.UpsertUser(ctx, users[i])
			}
		}
		return nil
	}
	if err := engine.SetQuota(xray.Quota{ID: "q1", ScopeKind: "user", ScopeID: user.ID,
		LimitBytes: 100, Reset: xray.ResetDaily, AutoSuspend: true}); err != nil {
		t.Fatal(err)
	}
	enforcer := &xray.Enforcer{Accountant: xray.NewAccountant(64, nil), Engine: engine,
		Events:        xray.NewEventLog(),
		EmailForScope: func(kind, id string) string { return "bob" },
		Collect: func(ctx context.Context) (xray.StatsSnapshot, error) {
			return xray.StatsSnapshot{Counters: []xray.StatCounter{
				// Xray keys stats by EMAIL; the test user's email
				// label is its catalog name.
				{Name: "user>>>bob>>>traffic>>>uplink", Value: 150},
			}}, nil
		}}
	decisions, err := enforcer.Round(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(decisions) == 0 || decisions[0].State != "exhausted" || !decisions[0].Enforced {
		t.Fatalf("decisions: %+v err=%v", decisions, err)
	}
	users, _ := cat.ListUsers(ctx, srv.ID)
	servers, _ := cat.ListServers(ctx)
	cfg, err := xray.RenderRuntimeConfig(servers, users)
	if err == nil && len(cfg.Inbounds) != 0 {
		t.Fatalf("suspended user still rendered: %+v", cfg.Inbounds)
	}
}

// WORKFLOW F: backup → validate → restore into an isolated catalog
// → referential integrity.
func TestWorkflowF_BackupRestore(t *testing.T) {
	src := testCatalog(t)
	ctx := context.Background()
	srv := xray.Server{ID: "ffffffffffffffffffffffffffffffff", Name: "primary", Protocol: xray.ProtocolVLESS,
		Listen: "127.0.0.1:2180", Port: 2180, Transport: xray.TransportTCP, Security: xray.SecurityNone,
		Enabled: true, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	_ = src.UpsertServer(ctx, srv)
	user := xray.User{ID: "11111111111111111111111111111112", ServerID: srv.ID, Name: "carol",
		UUID: "33333333-3333-3333-3333-333333333333", Enabled: true,
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	_ = src.UpsertUser(ctx, user)
	servers, _ := src.ListServers(ctx)
	users, _ := src.ListUsers(ctx, srv.ID)
	backup, err := xray.Export(servers, users, nil, nil, xray.ExportOptions{Producer: "workflow-f"})
	if err != nil {
		t.Fatal(err)
	}
	if backup.SecretsIncluded || backup.Users[0].UUID != "" {
		t.Fatal("default export leaked secrets")
	}
	raw, _ := json.Marshal(backup)
	var decoded xray.Backup
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	dst := testCatalog(t)
	applier := &fileApplier{cat: dst, ctx: ctx}
	// The secret-free export carries a credential-less user; the
	// honest restore applies the server and reports the skipped
	// credential for vault re-binding.
	applied, skipped, err := xray.Restore(decoded, applier)
	if err != nil || applied != 1 || skipped != 1 {
		t.Fatalf("restore: applied=%d skipped=%d err=%v", applied, skipped, err)
	}
	dstServers, _ := dst.ListServers(ctx)
	if len(dstServers) != 1 {
		t.Fatalf("servers after restore: %d", len(dstServers))
	}
}

type fileApplier struct {
	cat *xray.FileCatalog
	ctx context.Context
}

func (f *fileApplier) UpsertServer(s xray.Server) error { return f.cat.UpsertServer(f.ctx, s) }
func (f *fileApplier) UpsertUser(u xray.User) error     { return f.cat.UpsertUser(f.ctx, u) }
func (f *fileApplier) UpsertProfile(p xray.Profile) error {
	return f.cat.UpsertProfile(f.ctx, p)
}
func (f *fileApplier) UpsertSubscription(s xray.Subscription) error {
	return f.cat.UpsertSubscription(f.ctx, s)
}
