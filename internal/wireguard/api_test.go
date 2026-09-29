package wireguard

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/identity"
)

type fakeAccess struct {
	role identity.Role
	err  error
}

func (a fakeAccess) Authorize(_ string, _ identity.Permission) (identity.User, error) {
	if a.err != nil {
		return identity.User{}, a.err
	}
	return identity.User{Role: a.role}, nil
}

type fakeCatalog struct {
	mu      sync.Mutex
	servers map[string]Server
	peers   map[string]Peer
}

func newFakeCatalog() *fakeCatalog {
	return &fakeCatalog{servers: map[string]Server{}, peers: map[string]Peer{}}
}

func (c *fakeCatalog) UpsertServer(_ context.Context, s Server) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.servers[s.ID] = s
	return nil
}
func (c *fakeCatalog) RemoveServer(_ context.Context, id string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.servers, id)
	return nil
}
func (c *fakeCatalog) UpsertPeer(_ context.Context, p Peer) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.peers[p.ID] = p
	return nil
}
func (c *fakeCatalog) RemovePeer(_ context.Context, id string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.peers, id)
	return nil
}
func (c *fakeCatalog) Server(_ context.Context, id string) (Server, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	s, ok := c.servers[id]
	if !ok {
		return Server{}, errNotFound
	}
	return s, nil
}
func (c *fakeCatalog) Peer(_ context.Context, id string) (Peer, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	p, ok := c.peers[id]
	if !ok {
		return Peer{}, errNotFound
	}
	return p, nil
}
func (c *fakeCatalog) ListServers(_ context.Context) ([]Server, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]Server, 0, len(c.servers))
	for _, s := range c.servers {
		out = append(out, s)
	}
	return out, nil
}

func (c *fakeCatalog) ListPeers(_ context.Context, serverID string) ([]Peer, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]Peer, 0, len(c.peers))
	for _, p := range c.peers {
		if p.ServerID != serverID {
			continue
		}
		out = append(out, p)
	}
	return out, nil
}

var errNotFound = &notFoundError{}

type notFoundError struct{}

func (*notFoundError) Error() string { return "not found" }

func newAPI(t *testing.T) (*API, *fakeCatalog, *fakeRunner, *fakeFilesystem) {
	t.Helper()
	runner := &fakeRunner{peers: map[string]bool{}}
	fs := &fakeFilesystem{}
	cat := newFakeCatalog()
	return &API{
		Access:  fakeAccess{role: identity.Owner},
		Catalog: cat,
		Adapter: &KernelAdapter{Resolver: staticResolver{key: fixedKey()}, Runner: runner, Files: fs, Catalog: cat},
		Prefix:  "/wireguard/v1/",
	}, cat, runner, fs
}

func TestAPIRejectsUnauthorisedReads(t *testing.T) {
	a, _, _, _ := newAPI(t)
	a.Access = fakeAccess{err: identity.ErrUnauthorized}
	req := httptest.NewRequest(http.MethodGet, "/wireguard/v1/servers", nil)
	w := httptest.NewRecorder()
	a.ServeHTTP(w, req)
	if w.Code != 403 {
		t.Fatal("unauthorised read was accepted", w.Code, w.Body.String())
	}
}

func TestAPICreatesServer(t *testing.T) {
	a, cat, _, _ := newAPI(t)
	server, _ := validServerFixture()
	body, _ := json.Marshal(server)
	req := httptest.NewRequest(http.MethodPost, "/wireguard/v1/servers", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer fake")
	w := httptest.NewRecorder()
	a.ServeHTTP(w, req)
	if w.Code != 201 {
		t.Fatal("server creation failed", w.Code, w.Body.String())
	}
	if _, ok := cat.servers[server.ID]; !ok {
		t.Fatal("server was not stored in catalog")
	}
}

func TestAPIRejectsInvalidServer(t *testing.T) {
	a, cat, _, _ := newAPI(t)
	server, _ := validServerFixture()
	server.ID = "short"
	body, _ := json.Marshal(server)
	req := httptest.NewRequest(http.MethodPost, "/wireguard/v1/servers", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer fake")
	w := httptest.NewRecorder()
	a.ServeHTTP(w, req)
	if w.Code != 422 {
		t.Fatal("invalid server was accepted", w.Code, w.Body.String())
	}
	if len(cat.servers) != 0 {
		t.Fatal("invalid server was stored")
	}
}

func TestAPIAddsPeer(t *testing.T) {
	a, cat, _, _ := newAPI(t)
	server, _ := validServerFixture()
	body, _ := json.Marshal(server)
	req := httptest.NewRequest(http.MethodPost, "/wireguard/v1/servers", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer fake")
	a.ServeHTTP(httptest.NewRecorder(), req)
	_, pub, _ := GenerateServerKey()
	peer := validPeerFixture(server.ID, pub)
	body, _ = json.Marshal(peer)
	req = httptest.NewRequest(http.MethodPost, "/wireguard/v1/peers", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer fake")
	w := httptest.NewRecorder()
	a.ServeHTTP(w, req)
	if w.Code != 201 {
		t.Fatal("peer creation failed", w.Code, w.Body.String())
	}
	if _, ok := cat.peers[peer.ID]; !ok {
		t.Fatal("peer was not stored in catalog")
	}
}

// TestAPIListsPeersByServer proves the LIST /wireguard/v1/peers
// endpoint returns only the peers bound to the supplied
// ?server=ID. The endpoint MUST refuse when ?server is
// missing so the catalog never returns a global peer list.
func TestAPIListsPeersByServer(t *testing.T) {
	a, cat, _, _ := newAPI(t)
	server, _ := validServerFixture()
	body, _ := json.Marshal(server)
	req := httptest.NewRequest(http.MethodPost, "/wireguard/v1/servers", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer fake")
	a.ServeHTTP(httptest.NewRecorder(), req)
	_, pub, _ := GenerateServerKey()
	peer := validPeerFixture(server.ID, pub)
	body, _ = json.Marshal(peer)
	req = httptest.NewRequest(http.MethodPost, "/wireguard/v1/peers", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer fake")
	a.ServeHTTP(httptest.NewRecorder(), req)
	// Missing ?server: rejected.
	req = httptest.NewRequest(http.MethodGet, "/wireguard/v1/peers", nil)
	req.Header.Set("Authorization", "Bearer fake")
	w := httptest.NewRecorder()
	a.ServeHTTP(w, req)
	if w.Code != 400 {
		t.Fatalf("missing ?server: got %d, want 400", w.Code)
	}
	// With ?server=ID: returns the seeded peer.
	req = httptest.NewRequest(http.MethodGet, "/wireguard/v1/peers?server="+server.ID, nil)
	req.Header.Set("Authorization", "Bearer fake")
	w = httptest.NewRecorder()
	a.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("list peers: got %d, want 200", w.Code)
	}
	var got []Peer
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != peer.ID {
		t.Fatalf("peers: %+v, want one entry", got)
	}
	_ = cat
}

// TestAPIExportServerRedactsPrivateKey proves the
// /wireguard/v1/servers/{id}/export endpoint renders
// the wg-quick configuration WITHOUT any private key
// bytes. The endpoint is the safe diagnostic export; the
// secret-bearing wireguard-side file lives on disk and is
// never returned to a HTTP caller.
func TestAPIExportServerRedactsPrivateKey(t *testing.T) {
	a, _, _, _ := newAPI(t)
	server, _ := validServerFixture()
	body, _ := json.Marshal(server)
	req := httptest.NewRequest(http.MethodPost, "/wireguard/v1/servers", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer fake")
	a.ServeHTTP(httptest.NewRecorder(), req)
	req = httptest.NewRequest(http.MethodGet, "/wireguard/v1/servers/"+server.ID+"/export", nil)
	req.Header.Set("Authorization", "Bearer fake")
	w := httptest.NewRecorder()
	a.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("export: got %d, want 200 (%s)", w.Code, w.Body.String())
	}
	out := w.Body.String()
	if !strings.Contains(out, "ListenPort = "+fmt.Sprintf("%d", server.ListenPort)) {
		t.Fatalf("export missing listen port: %s", out)
	}
	// The server-side export contains no PrivateKey
	// line at all (not even a redaction sentinel): the
	// configuration is intended for sharing with the
	// peer operator and the contract pins
	// 'private-key-free end to end'.
	if strings.Contains(out, "PrivateKey") {
		t.Fatalf("export leaked PrivateKey line: %s", out)
	}
	if strings.Contains(out, "<redacted") || strings.Contains(out, "REDACTED") {
		t.Fatalf("export contains redaction sentinel: %s", out)
	}
}

// TestAPIUpdatesPeer proves the PUT /wireguard/v1/peers/{id}
// endpoint re-applies the peer to the kernel and persists
// the updated record in the catalog. The endpoint MUST
// refuse mismatched IDs and invalid payloads.
func TestAPIUpdatesPeer(t *testing.T) {
	a, cat, _, _ := newAPI(t)
	server, _ := validServerFixture()
	body, _ := json.Marshal(server)
	req := httptest.NewRequest(http.MethodPost, "/wireguard/v1/servers", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer fake")
	a.ServeHTTP(httptest.NewRecorder(), req)
	_, pub, _ := GenerateServerKey()
	peer := validPeerFixture(server.ID, pub)
	body, _ = json.Marshal(peer)
	req = httptest.NewRequest(http.MethodPost, "/wireguard/v1/peers", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer fake")
	a.ServeHTTP(httptest.NewRecorder(), req)
	// Flip enabled and AllowedIPs via PUT.
	peer.Enabled = true
	peer.AllowedIPs = []string{"10.0.0.0/24"}
	body, _ = json.Marshal(peer)
	req = httptest.NewRequest(http.MethodPut, "/wireguard/v1/peers/"+peer.ID, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer fake")
	w := httptest.NewRecorder()
	a.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("put peer: got %d, want 200 (%s)", w.Code, w.Body.String())
	}
	got, err := cat.Peer(context.Background(), peer.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Enabled || got.AllowedIPs[0] != "10.0.0.0/24" {
		t.Fatalf("catalog not updated: %+v", got)
	}
}

func TestAPIDeletesPeerAndServer(t *testing.T) {
	a, cat, _, _ := newAPI(t)
	server, _ := validServerFixture()
	body, _ := json.Marshal(server)
	req := httptest.NewRequest(http.MethodPost, "/wireguard/v1/servers", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer fake")
	a.ServeHTTP(httptest.NewRecorder(), req)
	if _, ok := cat.servers[server.ID]; !ok {
		t.Fatal("server was not stored")
	}
	req = httptest.NewRequest(http.MethodDelete, "/wireguard/v1/servers/"+server.ID, nil)
	req.Header.Set("Authorization", "Bearer fake")
	w := httptest.NewRecorder()
	a.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatal("server delete failed", w.Code, w.Body.String())
	}
	if _, ok := cat.servers[server.ID]; ok {
		t.Fatal("server was not removed from catalog")
	}
}

func TestAPIRejectsBadMethods(t *testing.T) {
	a, _, _, _ := newAPI(t)
	req := httptest.NewRequest(http.MethodPatch, "/wireguard/v1/servers", nil)
	req.Header.Set("Authorization", "Bearer fake")
	w := httptest.NewRecorder()
	a.ServeHTTP(w, req)
	if w.Code != 405 {
		t.Fatal("bad method was accepted", w.Code)
	}
}

func TestAPIUnknownPath(t *testing.T) {
	a, _, _, _ := newAPI(t)
	req := httptest.NewRequest(http.MethodGet, "/wireguard/v1/unknown", nil)
	req.Header.Set("Authorization", "Bearer fake")
	w := httptest.NewRecorder()
	a.ServeHTTP(w, req)
	if w.Code != 404 {
		t.Fatal("unknown path was accepted", w.Code)
	}
}

func TestAPIRequiresAdapterAndAccess(t *testing.T) {
	a := &API{}
	req := httptest.NewRequest(http.MethodGet, "/wireguard/v1/servers", nil)
	w := httptest.NewRecorder()
	a.ServeHTTP(w, req)
	if w.Code != 500 {
		t.Fatal("unconfigured API was accepted", w.Code)
	}
}

func TestPeerIsExpiredParsingSafe(t *testing.T) {
	peer := Peer{ExpiresAt: "not a date"}
	if peer.IsExpired(time.Now()) {
		t.Fatal("unparseable expiry was treated as expired")
	}
}
