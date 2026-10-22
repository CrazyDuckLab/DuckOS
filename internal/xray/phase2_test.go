// Package xray / phase2_test.go: REALITY manager, quotas, routing
// policies, insights, backup/restore.
package xray

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestRealityManager_Lifecycle(t *testing.T) {
	m := NewRealityManager()
	rec, priv, err := m.Generate("edge-1", "vault-1")
	if err != nil {
		t.Fatal(err)
	}
	if priv == "" || rec.PublicKey == "" || strings.Contains(rec.KeyVaultRef, priv) {
		t.Fatalf("generate: %+v", rec)
	}
	// The record must never carry the private key.
	js, _ := json.Marshal(m.List())
	if strings.Contains(string(js), priv) {
		t.Fatal("private key leaked into inventory")
	}
	if err := ValidateRealityDestination("yahoo.com:443"); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"localhost:443", "127.0.0.1:443", "no-port"} {
		if err := ValidateRealityDestination(bad); err == nil {
			t.Errorf("bad destination %q accepted", bad)
		}
	}
	sid1, err := m.AddShortID(rec.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.AddShortID(rec.ID); err != nil {
		t.Fatal(err)
	}
	server := xcenterRealityServerHelper(t)
	preview, err := m.Preview(rec.ID, server)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.ShortIDs) != 2 || preview.PublicKey != rec.PublicKey {
		t.Fatalf("preview: %+v", preview)
	}
	if err := m.RetireShortID(rec.ID, sid1.Hex); err != nil {
		t.Fatal(err)
	}
	preview, _ = m.Preview(rec.ID, server)
	if len(preview.ShortIDs) != 1 {
		t.Fatalf("retired short id still active: %+v", preview)
	}
	// Rotation produces a new record; old becomes rotating.
	newRec, newPriv, err := m.Rotate(rec.ID, "vault-2")
	if err != nil {
		t.Fatal(err)
	}
	if newPriv == "" || newRec.PublicKey == rec.PublicKey {
		t.Fatal("rotation did not produce fresh material")
	}
	list := m.List()
	states := map[string]string{}
	for _, r := range list {
		states[r.ID] = r.State
	}
	if states[rec.ID] != "rotating" || states[newRec.ID] != "active" {
		t.Fatalf("rotation states: %v", states)
	}
}

func xcenterRealityServerHelper(t *testing.T) Server {
	t.Helper()
	s := Server{
		ID: "99999999999999999999999999999999", Name: "reality-preview",
		Protocol: ProtocolVLESS, Listen: "0.0.0.0:443", Port: 443,
		Transport: TransportTCP, Security: SecurityReality,
		RealityDest: "yahoo.com:443", RealitySNI: []string{"yahoo.com"},
		Enabled: true,
	}
	return s
}

func TestQuotaEngine(t *testing.T) {
	hist := NewUsageHistory()
	suspended := false
	e := NewEngine(hist)
	e.Suspend = func(kind, id string) error { suspended = true; return nil }
	now := time.Now().UTC()
	e.SetClock(func() time.Time { return now })
	q := Quota{ID: "q1", ScopeKind: "user", ScopeID: "user-1", LimitBytes: 1000,
		Reset: ResetDaily, AutoSuspend: true}
	if err := e.SetQuota(q); err != nil {
		t.Fatal(err)
	}
	// Warn zone.
	ds, err := e.Observe("user", "user-1", 850, "stats:26.3.27")
	if err != nil || len(ds) != 1 || ds[0].State != "warn" {
		t.Fatalf("warn decision: %+v err=%v", ds, err)
	}
	if ds[0].Enforced {
		t.Fatal("warn must not be enforced")
	}
	// Exhausted next day: reset window keeps daily quotas separate.
	now = now.Add(25 * time.Hour)
	ds, _ = e.Observe("user", "user-1", 1200, "stats:26.3.27")
	if ds[0].State != "exhausted" {
		t.Fatalf("exhausted decision: %+v", ds[0])
	}
	if !ds[0].Enforced || !suspended {
		t.Fatal("auto-suspend did not enforce")
	}
	// Unattributable sample refused.
	if err := hist.Record("user-1", UsageSample{At: now, Bytes: 5}); err == nil {
		t.Fatal("sample without source accepted")
	}
}

func TestPolicyCompileAndConflicts(t *testing.T) {
	policies := []Policy{
		{ID: "p1", Name: "block-ads", Priority: 10, Domains: []string{"ads.example.com"}, Action: PolicyBlock, Enabled: true},
		{ID: "p2", Name: "bank-direct", Priority: 20, CIDRs: []string{"203.0.113.0/24"}, Action: PolicyDirect, Enabled: true},
		{ID: "p3", Name: "proxy-fallback", Priority: 30, Ports: []string{"443"}, Action: PolicyProxy, OutboundTag: "proxy-eu", Enabled: true},
		{ID: "p4", Name: "lan-direct", Priority: 5, InboundTags: []string{"in-lan"}, Action: PolicyDirect, Enabled: true},
	}
	rules, err := Compile(policies)
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 4 {
		t.Fatalf("rules: %d", len(rules))
	}
	// Deterministic order: priority then id.
	if rules[0].InboundTag[0] != "in-lan" || rules[1].OutboundTag != "block" {
		t.Fatalf("order: %+v", rules)
	}
	// Conflicting domain selectors refuse compilation.
	conflict := append(policies, Policy{ID: "p5", Name: "dupe", Priority: 1,
		Domains: []string{"ads.example.com"}, Action: PolicyDirect, Enabled: true})
	if _, err := Compile(conflict); err == nil {
		t.Fatal("conflicting policies compiled")
	}
	// Mixed-family duplicate CIDR is a warning only.
	warn := []Policy{
		{ID: "a", Name: "A", Priority: 1, CIDRs: []string{"203.0.113.5/32"}, Action: PolicyDirect, Enabled: true},
		{ID: "b", Name: "B", Priority: 2, CIDRs: []string{"2001:db8::/32"}, Action: PolicyBlock, Enabled: true},
	}
	if _, err := Compile(warn); err != nil {
		t.Fatalf("mixed family refused: %v", err)
	}
	// Render integration: block outbound appears; rules referencing
	// an existing outbound pass, unknown tags fail.
	local := append([]Policy(nil), policies...)
	local[2] = Policy{ID: "p3", Name: "proxy-fallback", Priority: 30, Ports: []string{"443"}, Action: PolicyDirect, Enabled: true}
	cfg := &Config{}
	if err := RenderPoliciesInto(cfg, local); err != nil {
		t.Fatal(err)
	}
	foundBlock := false
	for _, ob := range cfg.Outbounds {
		if ob.Tag == "block" {
			foundBlock = true
		}
	}
	if !foundBlock || len(cfg.Routing.Rules) != 4 {
		t.Fatalf("render integration: %+v", cfg)
	}
	// A proxy action pointing at an existing outbound tag is fine.
	withProxy := &Config{Outbounds: []Outbound{{Tag: "proxy-eu", Protocol: "freedom"}}}
	okProxy := []Policy{{ID: "px", Name: "PX", Priority: 1, Ports: []string{"443"}, Action: PolicyProxy, OutboundTag: "proxy-eu", Enabled: true}}
	if err := RenderPoliciesInto(withProxy, okProxy); err != nil {
		t.Fatalf("existing outbound rejected: %v", err)
	}
	bad := []Policy{{ID: "x", Name: "X", Priority: 1, Domains: []string{"d.com"}, Action: PolicyOutbound, OutboundTag: "ghost", Enabled: true}}
	if err := RenderPoliciesInto(&Config{}, bad); err == nil {
		t.Fatal("unknown outbound accepted")
	}
	// Port range validation.
	if err := (Policy{ID: "y", Name: "Y", Ports: []string{"a-b"}, Action: PolicyBlock}).Validate(); err == nil {
		t.Fatal("non-numeric port range accepted")
	}
}

func TestEventLogRetention(t *testing.T) {
	log := NewEventLog()
	base := time.Now().UTC()
	log.clock = func() time.Time { return base }
	if err := log.Record(Event{Kind: EventRestart, Source: "agent"}); err == nil {
		t.Fatal("event without timestamp accepted")
	}
	if err := log.Record(Event{At: base, Kind: EventRestart, Source: ""}); err == nil {
		t.Fatal("event without source accepted")
	}
	for i := 0; i < 1500; i++ {
		kind := EventConfigChange
		if i%2 == 0 {
			kind = EventRestart
		}
		if err := log.Record(Event{At: base.Add(time.Duration(i) * time.Minute), Kind: kind,
			Subject: "srv", Source: "agent", Message: "observed"}); err != nil {
			t.Fatal(err)
		}
	}
	if log.Count() > MaxEvents {
		t.Fatalf("ring exceeded cap: %d", log.Count())
	}
	restarts := log.Query([]EventKind{EventRestart}, "srv", time.Time{}, 0)
	if len(restarts) == 0 {
		t.Fatal("query returned nothing")
	}
	// Newest first.
	if !restarts[0].At.After(restarts[len(restarts)-1].At) {
		t.Fatal("query order is not newest-first")
	}
	since := log.Query(nil, "", base.Add(1490*time.Minute), 0)
	if len(since) == 0 || since[0].At.Before(base.Add(1490*time.Minute)) {
		t.Fatalf("since filter: %+v", since)
	}
}

func TestBackupExportRestore(t *testing.T) {
	srv := tortureValidServerFixture()
	srv.CreatedAt = time.Now().UTC()
	srv.UpdatedAt = srv.CreatedAt
	u := User{ID: "11111111111111111111111111111111", ServerID: srv.ID, Name: "primary-user",
		UUID: "11111111-1111-1111-1111-111111111111", Enabled: true,
		Credential: &KeyRef{VaultID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Revision: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", KeyID: "cccccccccccccccccccccccccccccccc"},
		CreatedAt:  srv.CreatedAt, UpdatedAt: srv.CreatedAt}
	// Default export strips inline secrets.
	b, err := Export([]Server{srv}, []User{u}, nil, nil, ExportOptions{Producer: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if b.SecretsIncluded || b.Users[0].UUID != "" {
		t.Fatal("default export leaked inline credential")
	}
	// Opt-in export keeps them.
	secret, err := Export([]Server{srv}, []User{u}, nil, nil, ExportOptions{IncludeSecrets: true, Producer: "test"})
	if err != nil || secret.Users[0].UUID != u.UUID {
		t.Fatalf("opt-in export: %v", err)
	}
	if err := ValidateBackup(b); err != nil {
		t.Fatalf("validation: %v", err)
	}
	// Referential integrity.
	broken := b
	broken.Users = []User{{ID: u.ID, ServerID: "ffffffffffffffffffffffffffffffff", Name: "orphan", UUID: "", Enabled: true}}
	if err := ValidateBackup(broken); err == nil {
		t.Fatal("orphan user passed validation")
	}
	// Version gate.
	oldVersion := b
	oldVersion.Version = 0
	if err := ValidateBackup(oldVersion); err == nil {
		t.Fatal("wrong version accepted")
	}
	// Preview and transactional restore.
	live := func() ([]Server, []User, []Profile, []Subscription) {
		return nil, nil, nil, nil
	}
	preview, err := RestorePreview(b, live)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview) != 2 {
		t.Fatalf("preview: %+v", preview)
	}
	applied := map[string]bool{}
	n, _, err := Restore(b, &stubApplier{applied: applied})
	if err != nil || n != 2 {
		t.Fatalf("restore: n=%d err=%v", n, err)
	}
	if !applied["server:"+srv.ID] || !applied["user:"+u.ID] {
		t.Fatalf("applied: %v", applied)
	}
}

type stubApplier struct{ applied map[string]bool }

func (s *stubApplier) UpsertServer(sv Server) error  { s.applied["server:"+sv.ID] = true; return nil }
func (s *stubApplier) UpsertUser(u User) error       { s.applied["user:"+u.ID] = true; return nil }
func (s *stubApplier) UpsertProfile(p Profile) error { s.applied["profile:"+p.ID] = true; return nil }
func (s *stubApplier) UpsertSubscription(x Subscription) error {
	s.applied["subscription:"+x.ID] = true
	return nil
}
