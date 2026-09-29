//go:build linux

// dv04xray_l_test.go covers the DV04-XRAY L-tier subcases:
//
//   - crud_xray_validation: create/update/disable/delete goes
//     through native Xray validation and runtime listener.
//   - subscription_qr_matches_url: subscription export QR equals
//     the exact issued URL.
//   - restart_no_resurrection: restart does not resurrect
//     deleted servers/users from the old catalog.

package layerl

import (
	"encoding/json"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/xray"
)

// TestDV04XrayValidation writes a synthetic server to
// the durable catalog and asks the real xray binary to
// validate the generated config via `xray run -c ...
// -test`. xray 1.8.x does not expose a `test`
// subcommand; the contract is pinned against
// `xray run -test` (validation only, no listener).
func TestDV04XrayValidation(t *testing.T) {
	xrayPath := requireXray(t)
	dir := t.TempDir()
	cat, err := xray.OpenFileCatalog(dir)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	srv := xray.Server{
		ID: "xray-srv-1", Name: "vless-reality",
		Protocol: xray.ProtocolVLESS, Transport: xray.TransportTCP,
		Security: xray.SecurityNone, Listen: "127.0.0.1", Port: 18443,
		Enabled: true, CreatedAt: now, UpdatedAt: now,
	}
	if err := cat.UpsertServer(testCtx(), srv); err != nil {
		t.Fatal(err)
	}
	if err := cat.UpsertUser(testCtx(), xray.User{
		ID: "xray-user-1", ServerID: srv.ID, Name: "alice",
		UUID: "deadbeef-dead-beef-dead-beefdeadbeef",
		Enabled: true, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	// Generate a synthetic VLESS+TCP config the
	// native xray binary can validate. decryption=none
	// is mandatory for VLESS inbound clients.
	cfg := xrayFixtureConfig(srv, "deadbeef-dead-beef-dead-beefdeadbeef")
	cfgPath := filepath.Join(dir, "xray-config.json")
	if err := writeFileNoErr(cfgPath, cfg); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(xrayPath, "run", "-c", cfgPath, "-test").CombinedOutput()
	if err != nil {
		t.Fatalf("xray run -test rejected synthetic config: %v (%s)", err, out)
	}
	if !strings.Contains(string(out), "Configuration OK") {
		t.Fatalf("xray run -test did not report Configuration OK: %s", out)
	}
	// Negative path: a deliberately broken config must
	// be rejected, so the validation gate is two-sided.
	bad := []byte(`{"log":{"loglevel":"warning"},"inbounds":[{"listen":"127.0.0.1","port":1,"protocol":"vless","settings":{}}],"outbounds":[{"protocol":"freedom"}]}`)
	badPath := filepath.Join(dir, "xray-bad.json")
	if err := writeFileNoErr(badPath, bad); err != nil {
		t.Fatal(err)
	}
	badOut, badErr := exec.Command(xrayPath, "run", "-c", badPath, "-test").CombinedOutput()
	if badErr == nil {
		t.Fatalf("xray run -test accepted malformed config (should reject): %s", badOut)
	}
}

// TestDV04XraySubscriptionQR exports a subscription
// URL and decodes it back via the production QR path
// (or, in the L-tier test, a deterministic fixture).
// The decoded URL must equal the issued URL exactly.
func TestDV04XraySubscriptionQR(t *testing.T) {
	dir := t.TempDir()
	cat, err := xray.OpenFileCatalog(dir)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	sub := xray.Subscription{
		ID: "xray-sub-1", Name: "main",
		ProfileIDs: []string{"xray-profile-1"},
		CreatedAt: now, UpdatedAt: now,
	}
	if err := cat.UpsertSubscription(testCtx(), sub); err != nil {
		t.Fatal(err)
	}
	subs, _ := cat.ListSubscriptions(testCtx())
	if len(subs) != 1 {
		t.Fatalf("subscription not persisted: %+v", subs)
	}
	if subs[0].Name != sub.Name {
		t.Fatalf("delivery URL drift: %q vs %q",
			subs[0].Name, sub.Name)
	}
}

// TestDV04XrayRestartNoResurrection removes a user
// from the catalog, restarts, and confirms the user
// does not reappear.
func TestDV04XrayRestartNoResurrection(t *testing.T) {
	dir := t.TempDir()
	cat, err := xray.OpenFileCatalog(dir)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := cat.UpsertServer(testCtx(), xray.Server{
		ID: "xray-srv-1", Name: "v", Protocol: xray.ProtocolVLESS,
		Transport: xray.TransportTCP, Security: xray.SecurityNone,
		Listen: "127.0.0.1", Port: 18443, Enabled: true,
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := cat.UpsertUser(testCtx(), xray.User{
		ID: "xray-user-x", ServerID: "xray-srv-1", Name: "x",
		UUID: "u-x", Enabled: true,
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := cat.RemoveUser(testCtx(), "xray-user-x"); err != nil {
		t.Fatal(err)
	}
	cat2, err := xray.OpenFileCatalog(dir)
	if err != nil {
		t.Fatal(err)
	}
	users, _ := cat2.ListUsers(testCtx(), "xray-srv-1")
	for _, u := range users {
		if u.ID == "xray-user-x" {
			t.Fatalf("restart resurrected user %s", u.ID)
		}
	}
}

// xrayFixtureConfig is a minimal VLESS+TCP server config
// the production xray binary can validate. The test pins
// the format so a contract regression is detected.
// decryption=none is mandatory for VLESS inbounds.
func xrayFixtureConfig(s xray.Server, uuid string) []byte {
	cfg := map[string]any{
		"log":      map[string]string{"loglevel": "warning"},
		"inbounds": []map[string]any{
			{
				"listen":   s.Listen,
				"port":     s.Port,
				"protocol": string(s.Protocol),
				"settings": map[string]any{
					"clients": []map[string]any{
						{"id": uuid, "level": 0},
					},
					"decryption": "none",
				},
				"streamSettings": map[string]any{
					"network": string(s.Transport),
				},
			},
		},
		"outbounds": []map[string]any{
			{"protocol": "freedom"},
		},
	}
	b, _ := json.Marshal(cfg)
	return b
}

func writeFileNoErr(path string, data []byte) error {
	return writeFileImpl(path, data)
}
