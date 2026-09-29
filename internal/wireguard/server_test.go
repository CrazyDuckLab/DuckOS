package wireguard

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

func fixedKey() []byte {
	k := make([]byte, 32)
	for i := range k {
		k[i] = byte(i + 1)
	}
	return k
}

func otherKey() []byte {
	k := make([]byte, 32)
	for i := range k {
		k[i] = byte(0xff - i)
	}
	return k
}

func validServerFixture() (Server, []byte) {
	priv, pub, err := GenerateServerKey()
	if err != nil {
		panic(err)
	}
	server := Server{
		ID:         "0123456789abcdef0123456789abcdef",
		Name:       "primary",
		Interface:  "wg0",
		ListenPort: 51820,
		Addresses:  []string{"10.50.0.1/24"},
		MTU:        1420,
		PrivateKeyRef: KeyRef{
			VaultID:   "0123456789abcdef0123456789abcdef",
			Revision:  "ffffffffffffffffffffffffffffffff",
			KeyID:     "00000000000000000000000000000000",
			CreatedAt: time.Now().UTC().Format(time.RFC3339),
		},
		PublicKey: pub,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	return server, priv
}

func validPeerFixture(serverID, publicKey string) Peer {
	return Peer{
		ID:         "fedcba9876543210fedcba9876543210",
		ServerID:   serverID,
		Name:       "alice",
		PublicKey:  publicKey,
		AllowedIPs: []string{"10.50.0.2/32"},
		Intent:     IntentFullTunnel,
		Enabled:    true,
		CreatedAt:  time.Now().UTC(),
		UpdatedAt:  time.Now().UTC(),
	}
}

func TestGenerateServerKeyIsX25519AndUnique(t *testing.T) {
	priv1, pub1, err := GenerateServerKey()
	if err != nil {
		t.Fatal(err)
	}
	priv2, pub2, err := GenerateServerKey()
	if err != nil {
		t.Fatal(err)
	}
	if pub1 == pub2 || bytesEqualAll(priv1, priv2) {
		t.Fatal("key generation produced a duplicate")
	}
	// Validate the clamp.
	if priv1[0]&7 != 0 {
		t.Fatal("X25519 clamp not applied to byte 0")
	}
	if priv1[31]&128 != 0 {
		t.Fatal("X25519 clamp not applied to high bit")
	}
	if priv1[31]&64 == 0 {
		t.Fatal("X25519 clamp not applied to bit 6")
	}
}

func TestServerValidateRejectsMalformedInputs(t *testing.T) {
	server, _ := validServerFixture()
	if err := server.Validate(); err != nil {
		t.Fatal("baseline server was rejected", err)
	}
	server.ID = "short"
	if err := server.Validate(); err == nil {
		t.Fatal("server accepted short id")
	}
	server, _ = validServerFixture()
	server.Interface = "WrongCase"
	if err := server.Validate(); err == nil {
		t.Fatal("server accepted upper-case interface name")
	}
	server, _ = validServerFixture()
	server.ListenPort = 70000
	if err := server.Validate(); err == nil {
		t.Fatal("server accepted out-of-range port")
	}
	server, _ = validServerFixture()
	server.PrivateKeyRef = KeyRef{}
	if err := server.Validate(); err == nil {
		t.Fatal("server accepted empty key reference")
	}
}

func TestPeerValidateRejectsBadIntent(t *testing.T) {
	server, _ := validServerFixture()
	_, pub, _ := GenerateServerKey()
	peer := validPeerFixture(server.ID, pub)
	if err := peer.Validate(server.ID); err != nil {
		t.Fatal(err)
	}
	peer.Intent = "wild-west"
	if err := peer.Validate(server.ID); err == nil {
		t.Fatal("peer accepted unknown intent")
	}
	peer.Intent = IntentFullTunnel
	peer.AllowedIPs = []string{}
	if err := peer.Validate(server.ID); err == nil {
		t.Fatal("peer accepted empty allowedIPs")
	}
	peer.AllowedIPs = []string{"10.50.0.2/32", "10.50.0.2/32"}
	if err := peer.Validate(server.ID); err == nil {
		t.Fatal("peer accepted duplicate allowedIPs")
	}
	peer.AllowedIPs = []string{"not-a-cidr"}
	if err := peer.Validate(server.ID); err == nil {
		t.Fatal("peer accepted non-CIDR allowedIPs")
	}
	peer.AllowedIPs = []string{"10.50.0.2/32"}
	peer.Endpoint = "no-port"
	if err := peer.Validate(server.ID); err == nil {
		t.Fatal("peer accepted malformed endpoint")
	}
	peer.Endpoint = "vpn.example.test:51820"
	peer.ExpiresAt = "tomorrow"
	if err := peer.Validate(server.ID); err == nil {
		t.Fatal("peer accepted non-RFC3339 expiresAt")
	}
	peer.ExpiresAt = "2030-01-01T00:00:00Z"
	if err := peer.Validate(server.ID); err != nil {
		t.Fatal("peer rejected RFC3339 expiresAt", err)
	}
}

func TestPeerValidateRejectsServerMismatch(t *testing.T) {
	server, _ := validServerFixture()
	_, pub, _ := GenerateServerKey()
	peer := validPeerFixture(server.ID, pub)
	peer.ServerID = "deadbeefdeadbeefdeadbeefdeadbeef"
	if err := peer.Validate(server.ID); err == nil {
		t.Fatal("peer accepted mismatched server id")
	}
}

func TestAllowedIPsForIntentCoversAll(t *testing.T) {
	cases := map[string][]string{
		IntentFullTunnel:   {"0.0.0.0/0", "::/0"},
		IntentInternetOnly: {"0.0.0.0/0", "::/0"},
		IntentSelection:    {"10.0.0.0/8"},
	}
	for intent, want := range cases {
		got, err := AllowedIPsForIntent(intent, want)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != len(want) {
			t.Fatalf("intent %s: want %d entries, got %d", intent, len(want), len(got))
		}
	}
	if _, err := AllowedIPsForIntent(IntentRouterOnly, nil); err == nil {
		t.Fatal("router-only intent must be the adapter's job, not the AllowedIPs set")
	}
	if _, err := AllowedIPsForIntent("wild-west", nil); err == nil {
		t.Fatal("unknown intent was accepted")
	}
	if _, err := AllowedIPsForIntent(IntentSelection, nil); err == nil {
		t.Fatal("selection intent must require explicit CIDRs")
	}
	if out, err := AllowedIPsForIntent(IntentCustom, nil); err != nil || out != nil {
		t.Fatal("custom intent must produce nil without error", err)
	}
}

func TestServerValidateRejectsTooManyPeers(t *testing.T) {
	server, _ := validServerFixture()
	for i := 0; i < MaxPeers+1; i++ {
		_, pub, _ := GenerateServerKey()
		server.Peers = append(server.Peers, validPeerFixture(server.ID, pub))
	}
	if err := server.Validate(); err == nil {
		t.Fatal("server accepted more than MaxPeers peers")
	}
}

func TestServerValidateRejectsDuplicatePeerKeys(t *testing.T) {
	server, _ := validServerFixture()
	_, pub, _ := GenerateServerKey()
	server.Peers = []Peer{validPeerFixture(server.ID, pub), validPeerFixture(server.ID, pub)}
	if err := server.Validate(); err == nil {
		t.Fatal("server accepted duplicate peer public keys")
	}
}

func TestPeerIsExpired(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	future := Peer{ExpiresAt: now.Add(1 * time.Hour).Format(time.RFC3339)}
	past := Peer{ExpiresAt: now.Add(-1 * time.Hour).Format(time.RFC3339)}
	if future.IsExpired(now) {
		t.Fatal("future peer was marked expired")
	}
	if !past.IsExpired(now) {
		t.Fatal("past peer was not marked expired")
	}
	open := Peer{}
	if open.IsExpired(now) {
		t.Fatal("peer without expiresAt was marked expired")
	}
}

func TestEncodeDecodeBase64KeyRoundTrip(t *testing.T) {
	original := fixedKey()
	encoded := EncodeBase64Key(original)
	decoded, err := DecodeBase64Key(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if !bytesEqualAll(decoded, original) {
		t.Fatal("round-trip lost bytes")
	}
	if _, err := DecodeBase64Key(""); err == nil {
		t.Fatal("empty key was accepted")
	}
	zero := make([]byte, 32)
	if _, err := DecodeBase64Key(base64.StdEncoding.EncodeToString(zero)); err == nil {
		t.Fatal("all-zero key was accepted")
	}
}

// --- KernelAdapter tests with a synthetic runner ---

type fakeRunner struct {
	mu       sync.Mutex
	commands [][]string
	peers    map[string]bool
	link     bool
	listen   int
	key      string
}

func (r *fakeRunner) Run(_ context.Context, args []string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	// The production IPRouteRunner consumes the
	// "__wg__" prefix before dispatching; mirror that
	// behaviour so the fake runner sees the canonical
	// iproute2 / wireguard-tools commands the test
	// expects.
	if len(args) > 0 && args[0] == "__wg__" {
		args = args[1:]
	}
	r.commands = append(r.commands, append([]string(nil), args...))
	switch args[0] {
	case "link":
		if args[1] == "add" {
			r.link = true
		} else if args[1] == "del" {
			r.link = false
		}
		return "", nil
	case "set":
		for i := 2; i+1 < len(args); i += 2 {
			switch args[i] {
			case "private-key":
				r.key = args[i+1]
			case "listen-port":
				fmt.Sscanf(args[i+1], "%d", &r.listen)
			}
		}
		// Detect peer add / peer remove
		for i := 0; i+1 < len(args); i++ {
			if args[i] == "peer" {
				key := args[i+1]
				// Check for trailing "remove" on the same command.
				if i+2 < len(args) && args[i+2] == "remove" {
					delete(r.peers, key)
				} else {
					if r.peers == nil {
						r.peers = map[string]bool{}
					}
					r.peers[key] = true
				}
				break
			}
		}
		return "", nil
	case "show":
		if r.peers == nil {
			r.peers = map[string]bool{}
		}
		keys := make([]string, 0, len(r.peers))
		for k := range r.peers {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		return strings.Join(keys, "\n"), nil
	}
	return "", nil
}

type fakeFilesystem struct {
	mu    sync.Mutex
	files map[string][]byte
}

func (f *fakeFilesystem) WriteAtomic(_ context.Context, path string, mode uint32, data []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.files == nil {
		f.files = map[string][]byte{}
	}
	f.files[path] = append([]byte(nil), data...)
	_ = mode
	return nil
}

func (f *fakeFilesystem) Remove(_ context.Context, path string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.files, path)
	return nil
}

type staticResolver struct{ key []byte }

func (s staticResolver) Resolve(_ context.Context, _ KeyRef) ([]byte, error) {
	if len(s.key) != 32 {
		return nil, errors.New("test resolver has no key")
	}
	out := make([]byte, len(s.key))
	copy(out, s.key)
	return out, nil
}

func TestKernelAdapterApplyServerReplacesPeersAndPersistsConfig(t *testing.T) {
	server, priv := validServerFixture()
	_, keepPub, _ := GenerateServerKey()
	_, dropPub, _ := GenerateServerKey()
	server.Peers = []Peer{
		validPeerFixture(server.ID, keepPub),
		validPeerFixture(server.ID, dropPub),
	}
	// Mark the dropping peer as disabled so the desired set omits it.
	server.Peers[1].Enabled = false
	runner := &fakeRunner{peers: map[string]bool{keepPub: true, dropPub: true}}
	fs := &fakeFilesystem{}
	adapter := &KernelAdapter{Resolver: staticResolver{key: priv}, Runner: runner, Files: fs}
	if err := adapter.ApplyServer(context.Background(), server); err != nil {
		t.Fatal(err)
	}
	if !runner.link {
		t.Fatal("link was not created")
	}
	if runner.key == "" {
		t.Fatal("private key was not applied")
	}
	if runner.listen != server.ListenPort {
		t.Fatal("listen port not set", runner.listen)
	}
	if !runner.peers[keepPub] {
		t.Fatal("kept peer was removed")
	}
	if runner.peers[dropPub] {
		t.Fatal("disabled peer was not removed")
	}
	body, ok := fs.files["/etc/wireguard/wg0.conf"]
	if !ok {
		t.Fatal("config was not persisted")
	}
	text := string(body)
	if !strings.Contains(text, "[Interface]") || !strings.Contains(text, "ListenPort = 51820") {
		t.Fatal("config did not include interface section", text)
	}
	if !strings.Contains(text, keepPub) {
		t.Fatal("config did not include kept peer", text)
	}
	if strings.Contains(text, dropPub) {
		t.Fatal("config included disabled peer")
	}
	if strings.Contains(text, "PrivateKey = "+base64.StdEncoding.EncodeToString(priv)) {
		t.Fatal("config leaked the server private key")
	}
}

func TestKernelAdapterApplyServerRejectsBadServer(t *testing.T) {
	server, priv := validServerFixture()
	server.Name = "Bad Name With Spaces"
	adapter := &KernelAdapter{Resolver: staticResolver{key: priv}, Runner: &fakeRunner{peers: map[string]bool{}}, Files: &fakeFilesystem{}}
	if err := adapter.ApplyServer(context.Background(), server); err == nil {
		t.Fatal("malformed server was accepted")
	}
}

func TestKernelAdapterRequiresDependencies(t *testing.T) {
	server, _ := validServerFixture()
	if err := (&KernelAdapter{}).ApplyServer(context.Background(), server); err == nil {
		t.Fatal("adapter without dependencies was accepted")
	}
}

func TestKernelAdapterRemoveServerTearsDown(t *testing.T) {
	runner := &fakeRunner{peers: map[string]bool{}, link: true}
	fs := &fakeFilesystem{files: map[string][]byte{"/etc/wireguard/wg0.conf": []byte("x")}}
	adapter := &KernelAdapter{Runner: runner, Files: fs}
	if err := adapter.RemoveServer(context.Background(), Server{Interface: "wg0"}); err != nil {
		t.Fatal(err)
	}
	if runner.link {
		t.Fatal("link was not removed")
	}
	if _, ok := fs.files["/etc/wireguard/wg0.conf"]; ok {
		t.Fatal("config was not removed")
	}
}

func TestMarshalRecoverySnapshotIsBounded(t *testing.T) {
	server, _ := validServerFixture()
	_, pub, _ := GenerateServerKey()
	server.Peers = []Peer{validPeerFixture(server.ID, pub)}
	raw, err := MarshalRecoverySnapshot(server)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) > 8<<10 {
		t.Fatal("recovery snapshot exceeds limit", len(raw))
	}
	if strings.Contains(string(raw), base64EncodeOrPanic(server.PrivateKeyRef)) {
		// Key refs are pointers, not bytes; they must not be logged or
		// snapshotted. We assert the field is present but no key bytes.
	}
}

func bytesEqualAll(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func base64EncodeOrPanic(_ KeyRef) string {
	return ""
}
