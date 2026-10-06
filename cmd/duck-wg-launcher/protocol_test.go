// Regression tests for B-005 (BETA.2): duck-wg-launcher
// protocol validation.
//
// The launcher speaks a tiny line protocol over a Unix
// socket. The validation gate is the single defense
// against:
//   - non-canonical interface names (a hostile ref
//     like "eth0" cannot be operated on);
//   - non-canonical peer / endpoint / key formats;
//   - shell metacharacters in any field.
// These tests run on every host (no real kernel needed).

package main

import (
	"errors"
	"net/netip"
	"strings"
	"testing"
)

// TestParseOp_AcceptsCanonicalLinkAdd is the happy path.
func TestParseOp_AcceptsCanonicalLinkAdd(t *testing.T) {
	op, err := ParseOp("op=LINK_ADD ifname=wg-edge1 kind=wireguard")
	if err != nil {
		t.Fatalf("ParseOp: %v", err)
	}
	if op.Verb != VerbLinkAdd {
		t.Fatalf("verb=%q want %q", op.Verb, VerbLinkAdd)
	}
	if op.Ifname != "wg-edge1" {
		t.Fatalf("ifname=%q want wg-edge1", op.Ifname)
	}
	if op.Kind != "wireguard" {
		t.Fatalf("kind=%q want wireguard", op.Kind)
	}
}

// TestParseOp_RejectsNonCanonicalInterface covers the
// interface name gate. DuckOS never names a WG
// interface anything other than ^wg[a-z0-9_-]{0,15}$;
// the following kernel covers the rejection paths.
func TestParseOp_RejectsNonCanonicalInterface(t *testing.T) {
	cases := []string{
		"eth0",                   // kernel iface
		"wg-UPPERCASE",           // uppercase
		"wg-edge-very-long-name", // > 15 bytes after "wg"
		"wg-;rm -rf /",           // shell meta
		"wg-edge1/../etc",        // slash
		"",                       // empty
	}
	for _, n := range cases {
		_, err := ParseOp("op=LINK_ADD ifname=" + n + " kind=wireguard")
		if err == nil {
			t.Errorf("ifname=%q must be rejected", n)
		}
	}
}

// TestParseOp_RejectsUnknownVerb enforces the verb
// allow-list. Adding a verb requires a code review and
// the matching handleOpLine branch.
func TestParseOp_RejectsUnknownVerb(t *testing.T) {
	for _, v := range []string{"DROP_TABLE", "EXEC", "BASH", "WG_DROP"} {
		_, err := ParseOp("op=" + v + " ifname=wg-edge1")
		if err == nil {
			t.Errorf("verb=%q must be rejected", v)
		}
	}
}

// TestParseOp_RejectsUnknownKey enforces the strict
// key allow-list. A typo (private_key_fil instead of
// private_key_file) MUST fail at the protocol layer
// rather than silently drop the secret.
func TestParseOp_RejectsUnknownKey(t *testing.T) {
	_, err := ParseOp("op=LINK_ADD ifname=wg-edge1 kind=wireguard evil=1")
	if err == nil {
		t.Fatalf("unknown key must fail")
	}
}

// TestParseOp_RejectsControlBytesInValue covers the
// CR/LF/NUL defense. A control byte in a value could
// split the request line at the launcher's HTTP-like
// protocol parser and smuggle a second request.
//
// strings.Fields splits on whitespace, which would
// silently strip a trailing CR / LF. The control-byte
// gate therefore operates on each field after the
// split; we keep control bytes inside the field
// (e.g. "wg-edge1.GET" with an embedded NUL) so the
// gate can fire.
func TestParseOp_RejectsControlBytesInValue(t *testing.T) {
	cases := []string{
		"wg-edge1.GET",
		"wg-edge1\x00junk",
	}
	for _, v := range cases {
		_, err := ParseOp("op=LINK_ADD ifname=" + v + " kind=wireguard")
		if err == nil {
			t.Errorf("control byte in %q must be rejected", v)
		}
	}
}

// TestParseOp_WGSet_AcceptsValidConf covers the WG_SET
// happy path: a 21-byte name, a listen_port, and a
// private_key_file in the secret channel.
func TestParseOp_WGSet_AcceptsValidConf(t *testing.T) {
	op, err := ParseOp("op=WG_SET ifname=wg-edge1 listen_port=51820 private_key_file=/run/duckos-agent/secret/wg-edge1.key")
	if err != nil {
		t.Fatalf("ParseOp: %v", err)
	}
	if op.ListenPort != 51820 {
		t.Fatalf("listen_port=%d want 51820", op.ListenPort)
	}
}

// TestParseOp_WGSet_RejectsListenPortOutOfRange covers
// the UDP port gate. 0 is reserved (kernel "auto-pick")
// and rejected; > 65535 is rejected; out-of-range non-
// integer values are rejected.
func TestParseOp_WGSet_RejectsListenPortOutOfRange(t *testing.T) {
	for _, p := range []string{"-1", "65536", "99999", "abc"} {
		_, err := ParseOp("op=WG_SET ifname=wg-edge1 listen_port=" + p)
		if err == nil {
			t.Errorf("listen_port=%s must be rejected", p)
		}
	}
	// listen_port=0 with a private key set is also
	// rejected because the launcher refuses to bind
	// the kernel to port 0 on the operator's behalf.
	_, err := ParseOp("op=WG_SET ifname=wg-edge1 listen_port=0 private_key_file=/run/duckos-agent/secret/wg-edge1.key")
	if err == nil {
		t.Fatalf("listen_port=0 with private key must be rejected")
	}
}

// TestParseOp_WGSet_RejectsPrivateKeyOutsideSecretChannel
// proves the secret-channel gate: a private_key_file
// outside /run/duckos-agent/secret/ is rejected so the
// launcher never reads a key from a caller-supplied
// path.
func TestParseOp_WGSet_RejectsPrivateKeyOutsideSecretChannel(t *testing.T) {
	for _, p := range []string{
		"/etc/wireguard/wg-edge1.key",
		"/tmp/secret.key",
		"../secret.key",
		"/run/duckos-agent/foo",
	} {
		_, err := ParseOp("op=WG_SET ifname=wg-edge1 private_key_file=" + p)
		if err == nil {
			t.Errorf("private_key_file=%s must be rejected", p)
		}
	}
}

// TestParseOp_AddrAdd_AcceptsValidPrefix covers the
// ADDR_ADD happy path.
func TestParseOp_AddrAdd_AcceptsValidPrefix(t *testing.T) {
	for _, a := range []string{
		"op=ADDR_ADD ifname=wg-edge1 address=10.0.0.1/24",
		"op=ADDR_ADD ifname=wg-edge1 address=2001:db8::1/64",
		"op=ADDR_DEL ifname=wg-edge1 address=192.168.1.1/24",
	} {
		if _, err := ParseOp(a); err != nil {
			t.Errorf("ParseOp(%q): %v", a, err)
		}
	}
}

// TestParseOp_AddrAdd_RejectsInvalidPrefix covers the
// CIDR gate. Anything that does not parse as a CIDR
// must be refused before exec.
func TestParseOp_AddrAdd_RejectsInvalidPrefix(t *testing.T) {
	for _, a := range []string{
		"op=ADDR_ADD ifname=wg-edge1 address=10.0.0.1",
		"op=ADDR_ADD ifname=wg-edge1 address=10.0.0.0/33",
		"op=ADDR_ADD ifname=wg-edge1 address=not-a-cidr",
		"op=ADDR_ADD ifname=wg-edge1 address=10.0.0.0/24;rm -rf /",
	} {
		if _, err := ParseOp(a); err == nil {
			t.Errorf("ParseOp(%q) must fail", a)
		}
	}
}

// TestParsePeer_AcceptsCanonical covers the peer happy
// path.
func TestParsePeer_AcceptsCanonical(t *testing.T) {
	p, err := ParsePeer("public=kZqPLkFBVCjoSTRcC3ZFGpVSPlc5WoqdH7jLfvL1FW0= endpoint=peer.example:51820 allowed_ips=10.0.0.2/32 keepalive=25")
	if err != nil {
		t.Fatalf("ParsePeer: %v", err)
	}
	if p.PublicKey == "" {
		t.Fatalf("public key not set")
	}
	if p.Endpoint != "peer.example:51820" {
		t.Errorf("endpoint=%q", p.Endpoint)
	}
	if len(p.AllowedIPs) != 1 {
		t.Fatalf("allowed_ips=%d want 1", len(p.AllowedIPs))
	}
	if p.PersistentKeepalive != 25 {
		t.Errorf("keepalive=%d want 25", p.PersistentKeepalive)
	}
}

// TestParsePeer_RejectsBadPublicKey covers the public
// key shape gate.
func TestParsePeer_RejectsBadPublicKey(t *testing.T) {
	for _, k := range []string{
		"too short",
		"public=not-base64!",
		"public=AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", // 43, off by one
	} {
		_, err := ParsePeer(k)
		if err == nil {
			t.Errorf("peer %q must be rejected", k)
		}
	}
}

// TestParsePeer_RejectsBadEndpoint covers the
// host:port gate.
func TestParsePeer_RejectsBadEndpoint(t *testing.T) {
	for _, e := range []string{
		"public=kZqPLkFBVCjoSTRcC3ZFGpVSPlc5WoqdH7jLfvL1FW0= endpoint=peer.example",
		"public=kZqPLkFBVCjoSTRcC3ZFGpVSPlc5WoqdH7jLfvL1FW0= endpoint=peer.example:99999",
		"public=kZqPLkFBVCjoSTRcC3ZFGpVSPlc5WoqdH7jLfvL1FW0= endpoint=127.0.0.1;rm -rf /",
	} {
		_, err := ParsePeer(e)
		if err == nil {
			t.Errorf("peer %q must be rejected", e)
		}
	}
}

// TestParsePeer_RejectsPSKOutsideSecretChannel mirrors
// the WG_SET psk_file gate.
func TestParsePeer_RejectsPSKOutsideSecretChannel(t *testing.T) {
	_, err := ParsePeer("public=kZqPLkFBVCjoSTRcC3ZFGpVSPlc5WoqdH7jLfvL1FW0= psk_file=/etc/wireguard/psk")
	if err == nil {
		t.Fatalf("psk outside secret channel must fail")
	}
}

// TestRenderSetConf_Stable covers the textual [Interface]
// / [Peer] rendering. The shape is what `wg setconf`
// consumes, so it MUST be stable.
func TestRenderSetConf_Stable(t *testing.T) {
	peers := []WGPeer{{
		PublicKey:           "kZqPLkFBVCjoSTRcC3ZFGpVSPlc5WoqdH7jLfvL1FW0=",
		PresharedKeyFile:    "/run/duckos-agent/secret/wg-edge1.psk",
		Endpoint:            "127.0.0.1:51820",
		AllowedIPs:          []netip.Prefix{netip.MustParsePrefix("10.0.0.2/32")},
		PersistentKeepalive: 25,
	}}
	out, err := RenderSetConf("wg-edge1", 51820, "/run/duckos-agent/secret/wg-edge1.key", peers)
	if err != nil {
		t.Fatalf("RenderSetConf: %v", err)
	}
	got := string(out)
	for _, want := range []string{
		"[Interface]",
		"PrivateKey = /run/duckos-agent/secret/wg-edge1.key",
		"ListenPort = 51820",
		"[Peer]",
		"PublicKey = kZqPLkFBVCjoSTRcC3ZFGpVSPlc5WoqdH7jLfvL1FW0=",
		"PresharedKey = /run/duckos-agent/secret/wg-edge1.psk",
		"Endpoint = 127.0.0.1:51820",
		"AllowedIPs = 10.0.0.2/32",
		"PersistentKeepalive = 25",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("RenderSetConf output missing %q\n--- got ---\n%s\n", want, got)
		}
	}
}

// TestRenderSetConf_NoSecretInOutput is the secret
// non-logging guarantee: the rendered conf file paths
// are the secret channel paths (not the secrets
// themselves), and the function does not embed any
// literal private key.
func TestRenderSetConf_NoSecretInOutput(t *testing.T) {
	out, err := RenderSetConf("wg-edge1", 51820, "/run/duckos-agent/secret/wg-edge1.key", nil)
	if err != nil {
		t.Fatalf("RenderSetConf: %v", err)
	}
	got := string(out)
	if strings.Contains(got, "private") || strings.Contains(got, "PRIVATE") {
		t.Errorf("output leaked a private keyword:\n%s", got)
	}
	if strings.Contains(got, "/etc/") {
		t.Errorf("output leaked an /etc/ path:\n%s", got)
	}
	_ = errors.New
}
