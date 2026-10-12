// Regression test for B-005 (BETA.2): the production
// KernelAdapter wires a WGTyper (LauncherClient → duck-wg-launcher
// over the Unix socket). When a.Typer is set, Adapter.ApplyServer
// and Adapter.RemoveServer must dispatch to the launcher path so
// the kernel operations do not require /sbin/ip or /usr/bin/wg
// (the agent runs with CapabilityBoundingSet= empty).
//
// Before this fix, the public API → API.upsertServer → Adapter.ApplyServer
// path always used the legacy Runner, which exec'd /sbin/ip directly;
// in production this returned "RTNETLINK answers: Operation not
// permitted" because the agent has no capabilities.

package wireguard

import (
	"context"
	"errors"
	"sync"
	"testing"
)

// fakeTyper records the WGTyper invocations the production adapter
// performs when a.Typer is set. The fake refuses to record private
// key bytes so the test asserts the boundary by construction.
type fakeTyper struct {
	mu          sync.Mutex
	linkAdd     []string
	linkDel     []string
	addrAdd     []string
	linkUp      []string
	setConfKey  []byte
	setConfPort int
	setConfPeer int
	err         error
}

func (f *fakeTyper) LinkAdd(ifname string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	f.linkAdd = append(f.linkAdd, ifname)
	return nil
}

func (f *fakeTyper) LinkDel(ifname string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	f.linkDel = append(f.linkDel, ifname)
	return nil
}

func (f *fakeTyper) AddrAdd(ifname, prefix string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	f.addrAdd = append(f.addrAdd, ifname+":"+prefix)
	return nil
}

func (f *fakeTyper) AddrDel(ifname, prefix string) error { return nil }

func (f *fakeTyper) LinkUp(ifname string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	f.linkUp = append(f.linkUp, ifname)
	return nil
}

func (f *fakeTyper) LinkDown(ifname string) error { return nil }

func (f *fakeTyper) SetConf(ifname string, listenPort int, privateKey []byte, peers []PeerConf) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	f.setConfKey = append([]byte(nil), privateKey...)
	f.setConfPort = listenPort
	f.setConfPeer = len(peers)
	return nil
}

// TestAdapterApplyServerDispatchesToLauncherWhenTyperSet pins the
// production dispatch contract: when a.Typer is set, ApplyServer
// must use the launcher path and NOT the exec-based Runner. The
// fakeRunner is wired but must remain untouched.
func TestAdapterApplyServerDispatchesToLauncherWhenTyperSet(t *testing.T) {
	server, priv := validServerFixture()
	_, peerPub, err := GenerateServerKey()
	if err != nil {
		t.Fatalf("GenerateServerKey: %v", err)
	}
	server.Peers = []Peer{validPeerFixture(server.ID, peerPub)}
	typer := &fakeTyper{}
	runner := &fakeRunner{peers: map[string]bool{}}
	fs := &fakeFilesystem{}
	adapter := &KernelAdapter{
		Resolver: staticResolver{key: priv},
		Runner:   runner, // present, but should not be touched
		Typer:    typer,  // production wiring
		Files:    fs,
	}
	if err := adapter.ApplyServer(context.Background(), server); err != nil {
		t.Fatalf("ApplyServer: %v", err)
	}
	if len(runner.commands) != 0 {
		t.Fatalf("Runner.Run was invoked %d times; production must dispatch to Typer", len(runner.commands))
	}
	if len(typer.linkAdd) != 1 || typer.linkAdd[0] != server.Interface {
		t.Fatalf("Typer.LinkAdd not invoked correctly: %v", typer.linkAdd)
	}
	if len(typer.addrAdd) == 0 {
		t.Fatal("Typer.AddrAdd was not invoked")
	}
	if len(typer.linkUp) != 1 || typer.linkUp[0] != server.Interface {
		t.Fatalf("Typer.LinkUp not invoked: %v", typer.linkUp)
	}
	if typer.setConfPort != server.ListenPort {
		t.Fatalf("Typer.SetConf port = %d, want %d", typer.setConfPort, server.ListenPort)
	}
	if want := len(server.Peers); typer.setConfPeer != want {
		t.Fatalf("Typer.SetConf peer count = %d, want %d", typer.setConfPeer, want)
	}
	if len(typer.setConfKey) != len(priv) {
		t.Fatalf("Typer.SetConf key bytes = %d, want %d", len(typer.setConfKey), len(priv))
	}
	for i := range typer.setConfKey {
		if typer.setConfKey[i] != priv[i] {
			t.Fatalf("Typer.SetConf key byte %d differs", i)
		}
	}
	if _, ok := fs.files["/etc/wireguard/"+server.Interface+".conf"]; !ok {
		t.Fatal("config was not persisted under /etc/wireguard/<if>.conf")
	}
}

// TestAdapterRemoveServerDispatchesToLauncherWhenTyperSet pins the
// production dispatch contract for the teardown path.
func TestAdapterRemoveServerDispatchesToLauncherWhenTyperSet(t *testing.T) {
	typer := &fakeTyper{}
	runner := &fakeRunner{}
	fs := &fakeFilesystem{files: map[string][]byte{"/etc/wireguard/wg0.conf": []byte("x")}}
	adapter := &KernelAdapter{
		Runner: runner,
		Typer:  typer,
		Files:  fs,
	}
	if err := adapter.RemoveServer(context.Background(), Server{Interface: "wg0"}); err != nil {
		t.Fatalf("RemoveServer: %v", err)
	}
	if len(runner.commands) != 0 {
		t.Fatalf("Runner.Run was invoked %d times; production must dispatch to Typer", len(runner.commands))
	}
	if len(typer.linkDel) != 1 || typer.linkDel[0] != "wg0" {
		t.Fatalf("Typer.LinkDel not invoked: %v", typer.linkDel)
	}
	if _, ok := fs.files["/etc/wireguard/wg0.conf"]; ok {
		t.Fatal("config was not removed")
	}
}

// TestAdapterApplyServerLauncherErrorPropagates ensures the new
// dispatch surfaces launcher errors verbatim so the API can return
// the typed envelope to the caller.
func TestAdapterApplyServerLauncherErrorPropagates(t *testing.T) {
	server, priv := validServerFixture()
	_, peerPub, err := GenerateServerKey()
	if err != nil {
		t.Fatalf("GenerateServerKey: %v", err)
	}
	server.Peers = []Peer{validPeerFixture(server.ID, peerPub)}
	typer := &fakeTyper{err: errors.New("launcher refused")}
	adapter := &KernelAdapter{
		Resolver: staticResolver{key: priv},
		Runner:   &fakeRunner{},
		Typer:    typer,
		Files:    &fakeFilesystem{},
	}
	if err := adapter.ApplyServer(context.Background(), server); err == nil {
		t.Fatal("launcher error was not propagated")
	}
}

// TestIsExistsAcceptsBothIproute2ConflictWordings pins the
// launcher-path idempotency guarantee: when the agent re-applies
// a WireGuard server whose address is already on the kernel
// interface, AddrAdd must be treated as a no-op so the apply
// step succeeds (idempotent re-apply after restart). iproute2
// emits the conflict in two wordings across versions; both
// must be accepted.
func TestIsExistsAcceptsBothIproute2ConflictWordings(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, true},
		{"old wording", errors.New("RTNETLINK answers: File exists"), true},
		{"new wording", errors.New("Error: ipv4: Address already assigned."), true},
		{"launcher ownership gate", errors.New(`interface "wg-edge1" already exists`), true},
		{"unrelated", errors.New("some other error"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isExists(tc.err); got != tc.want {
				t.Fatalf("isExists(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

// B-011 regression: the production agent holds no capabilities, so
// ApplyPeer must never exec wg via the Runner when a Typer is wired.
// The launcher contract is a full WG_SET (replace_peers), so the peer
// delta is merged into the server's peer list and the whole setconf
// is re-applied through the privileged launcher.
func TestAdapterApplyPeerDispatchesToLauncherWhenTyperSet(t *testing.T) {
	server, priv := validServerFixture()
	_, peerPub, err := GenerateServerKey()
	if err != nil {
		t.Fatalf("GenerateServerKey: %v", err)
	}
	existing := validPeerFixture(server.ID, peerPub)
	server.Peers = []Peer{existing}
	dir := t.TempDir()
	cat, err := OpenFileCatalog(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := cat.UpsertServer(context.Background(), server); err != nil {
		t.Fatal(err)
	}
	if err := cat.UpsertPeer(context.Background(), existing); err != nil {
		t.Fatal(err)
	}
	_, newPub, err := GenerateServerKey()
	if err != nil {
		t.Fatalf("GenerateServerKey: %v", err)
	}
	added := validPeerFixture(server.ID, newPub)
	added.ID = "0123456789abcdef0123456789abcdef"
	added.Name = "bob"
	typer := &fakeTyper{}
	runner := &fakeRunner{peers: map[string]bool{}}
	fs := &fakeFilesystem{}
	adapter := &KernelAdapter{
		Resolver: staticResolver{key: priv},
		Runner:   runner, // present, but must not be touched
		Typer:    typer,  // production wiring
		Files:    fs,
		Catalog:  cat,
	}
	if err := adapter.ApplyPeer(context.Background(), added); err != nil {
		t.Fatalf("ApplyPeer: %v", err)
	}
	if len(runner.commands) != 0 {
		t.Fatalf("Runner.Run was invoked %d times; ApplyPeer must dispatch to Typer", len(runner.commands))
	}
	if typer.setConfPort != server.ListenPort {
		t.Fatalf("Typer.SetConf port = %d, want %d", typer.setConfPort, server.ListenPort)
	}
	// The rendered peer list must contain both the existing and the
	// newly added peer (full WG_SET replacement semantics).
	if typer.setConfPeer != 2 {
		t.Fatalf("Typer.SetConf peer count = %d, want 2 (existing + delta)", typer.setConfPeer)
	}
	if _, ok := fs.files["/etc/wireguard/"+server.Interface+".conf"]; !ok {
		t.Fatal("config was not persisted under /etc/wireguard/<if>.conf")
	}
}

// B-011 regression: disabling a peer through the launcher path must
// drop it from the rendered peer set (the kernel-level remove), not
// exec wg remove via the Runner.
func TestAdapterApplyPeerDisableDispatchesToLauncherWhenTyperSet(t *testing.T) {
	server, priv := validServerFixture()
	_, peerPub, err := GenerateServerKey()
	if err != nil {
		t.Fatalf("GenerateServerKey: %v", err)
	}
	existing := validPeerFixture(server.ID, peerPub)
	server.Peers = []Peer{existing}
	dir := t.TempDir()
	cat, err := OpenFileCatalog(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := cat.UpsertServer(context.Background(), server); err != nil {
		t.Fatal(err)
	}
	if err := cat.UpsertPeer(context.Background(), existing); err != nil {
		t.Fatal(err)
	}
	disabled := existing
	disabled.Enabled = false
	typer := &fakeTyper{}
	runner := &fakeRunner{peers: map[string]bool{}}
	fs := &fakeFilesystem{}
	adapter := &KernelAdapter{
		Resolver: staticResolver{key: priv},
		Runner:   runner,
		Typer:    typer,
		Files:    fs,
		Catalog:  cat,
	}
	if err := adapter.ApplyPeer(context.Background(), disabled); err != nil {
		t.Fatalf("ApplyPeer(disable): %v", err)
	}
	if len(runner.commands) != 0 {
		t.Fatalf("Runner.Run was invoked %d times on disable; must dispatch to Typer", len(runner.commands))
	}
	if typer.setConfPeer != 0 {
		t.Fatalf("Typer.SetConf peer count = %d, want 0 (disabled peer omitted)", typer.setConfPeer)
	}
}
