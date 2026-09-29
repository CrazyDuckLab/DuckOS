package main

import (
	"strings"
	"testing"
)

func TestDispatchRejectsMissingSubcommand(t *testing.T) {
	if err := dispatch(nil); err == nil {
		t.Fatal("missing subcommand was silently accepted")
	}
}

func TestDispatchRejectsUnknownSubcommand(t *testing.T) {
	if err := dispatch([]string{"bogus"}); err == nil {
		t.Fatal("unknown subcommand was silently accepted")
	}
}

func TestDispatchPrintsVersion(t *testing.T) {
	if err := dispatch([]string{"version"}); err != nil {
		t.Fatal(err)
	}
}

func TestDoctorSubcommandDispatches(t *testing.T) {
	if err := dispatch([]string{"doctor"}); err == nil {
		t.Fatal("dispatch accepted doctor without a reachable server")
	}
}
func TestTopologySubcommandDispatches(t *testing.T) {
	if err := dispatch([]string{"topology"}); err == nil {
		t.Fatal("dispatch accepted topology without a reachable server")
	}
}
func TestMultiwanSubcommandDispatches(t *testing.T) {
	if err := dispatch([]string{"multiwan"}); err == nil {
		t.Fatal("dispatch accepted multiwan without a reachable server")
	}
}
func TestDispatchPrintsHelp(t *testing.T) {
	if err := dispatch([]string{"help"}); err != nil {
		t.Fatal(err)
	}
}

func TestNetworkSubcommandRequiresOperation(t *testing.T) {
	if err := networkCmd(nil); err == nil {
		t.Fatal("network with no operation was accepted")
	}
}

func TestNetworkSubcommandRejectsUnknownOperation(t *testing.T) {
	if err := networkCmd([]string{"nope"}); err == nil {
		t.Fatal("network with unknown operation was accepted")
	}
}

func TestClientSubcommandRequiresOperation(t *testing.T) {
	if err := clientCmd(nil); err == nil {
		t.Fatal("client with no operation was accepted")
	}
}

func TestIdentitySubcommandRequiresOperation(t *testing.T) {
	if err := identityCmd(nil); err == nil {
		t.Fatal("identity with no operation was accepted")
	}
}

func TestIdentitySubcommandRejectsUnknownOperation(t *testing.T) {
	if err := identityCmd([]string{"bogus"}); err == nil {
		t.Fatal("identity with unknown operation was accepted")
	}
}

func TestIdentityEnrollRequiresUsername(t *testing.T) {
	if err := identityEnroll([]string{"--password", "0123456789012345"}); err == nil {
		t.Fatal("enroll without username was accepted")
	}
}

func TestIdentityEnrollRequiresStrongPassword(t *testing.T) {
	if err := identityEnroll([]string{"--username", "alice", "--password", "short"}); err == nil {
		t.Fatal("enroll with short password was accepted")
	}
}

func TestIdentityChangePasswordRequiresInputs(t *testing.T) {
	if err := identityChangePassword([]string{"--old-password", "x", "--new-password", "y"}); err == nil {
		// missing inputs should fail before we get to network
		t.Fatal("change-password with weak new password was accepted")
	}
}

func TestIdentityRevokeRequiresSessionID(t *testing.T) {
	if err := identityRevoke(nil); err == nil {
		t.Fatal("revoke without session id was accepted")
	}
}

func TestWireguardRequiresOperation(t *testing.T) {
	if err := wireguardCmd(nil); err == nil {
		t.Fatal("wireguard without operation was accepted")
	}
}

func TestWireguardServersRequiresOperation(t *testing.T) {
	if err := wireguardServers(nil); err == nil {
		t.Fatal("wireguard servers without operation was accepted")
	}
}

func TestWireguardServersCreateRequiresFile(t *testing.T) {
	if err := wireguardServersCreate(nil); err == nil {
		t.Fatal("wireguard servers create without --server was accepted")
	}
}

func TestWireguardServersDeleteRequiresID(t *testing.T) {
	if err := wireguardServersDelete(nil); err == nil {
		t.Fatal("wireguard servers delete without --server-id was accepted")
	}
}

func TestWireguardPeersRequiresOperation(t *testing.T) {
	if err := wireguardPeers(nil); err == nil {
		t.Fatal("wireguard peers without operation was accepted")
	}
}

func TestWireguardPeersAddRequiresFile(t *testing.T) {
	if err := wireguardPeersAdd(nil); err == nil {
		t.Fatal("wireguard peers add without --peer was accepted")
	}
}

func TestWireguardPeersDeleteRequiresID(t *testing.T) {
	if err := wireguardPeersDelete(nil); err == nil {
		t.Fatal("wireguard peers delete without --peer-id was accepted")
	}
}

func TestXrayRequiresOperation(t *testing.T) {
	if err := xrayCmd(nil); err == nil {
		t.Fatal("xray without operation was accepted")
	}
}

func TestXrayCRUDRequiresOperation(t *testing.T) {
	if err := xrayCRUD("servers", nil); err == nil {
		t.Fatal("xray servers without operation was accepted")
	}
}

func TestXrayCRUDRejectsUnknownOperation(t *testing.T) {
	if err := xrayCRUD("servers", []string{"bogus"}); err == nil {
		t.Fatal("xray servers with unknown operation was accepted")
	}
}

func TestXrayCRUDCreateRequiresFile(t *testing.T) {
	if err := xrayCRUD("servers", []string{"create"}); err == nil {
		t.Fatal("xray servers create without --file was accepted")
	}
}

func TestXrayCRUDDeleteRequiresID(t *testing.T) {
	if err := xrayCRUD("servers", []string{"delete"}); err == nil {
		t.Fatal("xray servers delete without --id was accepted")
	}
}

func TestLoadTokenRejectsWorldAccessible(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/token"
	if err := writeToken(path, "01234567890123456789012345678901"); err != nil {
		t.Fatal(err)
	}
	if err := chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadToken(path); err == nil {
		t.Fatal("world-accessible token was accepted")
	}
}

func TestLoadTokenAcceptsPrivateFile(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/token"
	if err := writeToken(path, "01234567890123456789012345678901"); err != nil {
		t.Fatal(err)
	}
	if err := chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	tok, err := loadToken(path)
	if err != nil {
		t.Fatal(err)
	}
	if tok != "01234567890123456789012345678901" {
		t.Fatal("token did not round-trip")
	}
}

func TestRenderJSONRejectsUnknownFormat(t *testing.T) {
	if err := render("xml", map[string]any{"x": 1}); err == nil {
		t.Fatal("unknown format was accepted")
	}
}

func TestRenderTextEmitsKeyValue(t *testing.T) {
	out, _ := captureOutput(func() error {
		return render("text", map[string]any{"name": "duck", "age": 1})
	})
	if !strings.Contains(out, "name:") || !strings.Contains(out, "duck") {
		t.Fatal("text output missing fields", out)
	}
}

func TestErrorMessageExtractsMessage(t *testing.T) {
	got := errorMessage(map[string]any{"error": map[string]any{"message": "nope"}})
	if got != "nope" {
		t.Fatal("did not extract error message", got)
	}
}

func TestErrorMessageHandlesMissingError(t *testing.T) {
	got := errorMessage(map[string]any{"status": "ok"})
	if !strings.Contains(got, "ok") {
		t.Fatal("missing error fallback wrong", got)
	}
}
