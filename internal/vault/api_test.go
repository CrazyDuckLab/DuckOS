package vault

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/identity"
)

// stubSource implements Source with a deterministic
// in-memory store. It records every Put / List / Revoke so
// the tests can assert the surface area and the caller's
// permissions at the same time.
type stubSource struct {
	mu      sync.Mutex
	records map[string]Reference
	puts    int
	lists   int
	revokes int
	failPut error
}

func newStubSource() *stubSource {
	return &stubSource{records: map[string]Reference{}}
}

func (s *stubSource) Put(id, purpose string, plaintext []byte) (Reference, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failPut != nil {
		return Reference{}, s.failPut
	}
	s.puts++
	rev := random32HexLocal()
	ref := Reference{
		ID:       id,
		Revision: rev,
		KeyID:    "00000000000000000000000000000001",
		Purpose:  purpose,
	}
	s.records[id] = ref
	return ref, nil
}

func (s *stubSource) List() ([]Reference, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lists++
	out := make([]Reference, 0, len(s.records))
	for _, r := range s.records {
		out = append(out, r)
	}
	return out, nil
}

func (s *stubSource) Revoke(id, revision, actor, reason string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.revokes++
	r, ok := s.records[id]
	if !ok {
		return errors.New("not found")
	}
	if r.Revision != revision {
		return errors.New("revision mismatch")
	}
	delete(s.records, id)
	return nil
}

// stubAccess implements Access with a single Owner bearer
// token. The token is opaque; the test sets Allowed to
// control whether the Owner permission is granted.
type stubAccess struct {
	Allowed bool
	Role    identity.Role
	User    string
}

func (a stubAccess) Authorize(token string, p identity.Permission) (identity.User, error) {
	if token == "" {
		return identity.User{}, errors.New("no token")
	}
	if !a.Allowed || !identity.Allowed(a.Role, p) {
		return identity.User{}, identity.ErrForbidden
	}
	return identity.User{Username: a.User, Role: a.Role}, nil
}

func random32Hex(t *testing.T) string {
	t.Helper()
	return random32HexLocal()
}

func random32HexLocal() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

func newAPI(t *testing.T, src Source, acc Access) *API {
	t.Helper()
	return &API{Source: src, Access: acc, Prefix: "/vault/v1/"}
}

func doRequest(t *testing.T, h http.Handler, method, path, token string, body []byte) (*httptest.ResponseRecorder, []byte) {
	t.Helper()
	r := httptest.NewRequest(method, path, bytes.NewReader(body))
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w, w.Body.Bytes()
}

// ----- happy path -----

func TestVaultPutHappy(t *testing.T) {
	src := newStubSource()
	acc := stubAccess{Allowed: true, Role: identity.Owner, User: "alice"}
	api := newAPI(t, src, acc)
	id := random32Hex(t)
	raw := []byte("a-32-byte-wg-private-key-mock---") // 31 bytes
	body := mustJSON(t, SecretPutRequest{ID: id, Purpose: "wireguard-private", Payload: base64.StdEncoding.EncodeToString(raw)})
	w, data := doRequest(t, api, http.MethodPost, "/vault/v1/keys", "good", body)
	if w.Code != http.StatusCreated {
		t.Fatalf("status: %d body=%s", w.Code, data)
	}
	var resp SecretPutResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		t.Fatalf("decode: %v body=%s", err, data)
	}
	if resp.VaultID != id {
		t.Fatalf("vaultId: got %s want %s", resp.VaultID, id)
	}
	if resp.Revision == "" || resp.KeyID == "" {
		t.Fatalf("reference incomplete: %+v", resp)
	}
	if resp.Purpose != "wireguard-private" {
		t.Fatalf("purpose: %s", resp.Purpose)
	}
	// Critical: the response body must not contain the secret.
	if bytes.Contains(data, raw) {
		t.Fatalf("response leaks secret bytes: %s", data)
	}
	if src.puts != 1 {
		t.Fatalf("source.Put calls: %d", src.puts)
	}
}

// ----- no-readback: the API never serves secret bytes -----

func TestVaultNeverReadsBackSecretBytes(t *testing.T) {
	src := newStubSource()
	acc := stubAccess{Allowed: true, Role: identity.Owner, User: "alice"}
	api := newAPI(t, src, acc)
	id := random32Hex(t)
	raw := []byte("CENSORED-secret-bytes-CENSORED----")
	body := mustJSON(t, SecretPutRequest{ID: id, Purpose: "wireguard-private", Payload: base64.StdEncoding.EncodeToString(raw)})
	_, _ = doRequest(t, api, http.MethodPost, "/vault/v1/keys", "good", body)

	// List must not include any byte from raw.
	_, listData := doRequest(t, api, http.MethodGet, "/vault/v1/keys", "good", nil)
	if bytes.Contains(listData, raw) {
		t.Fatalf("list leaks secret: %s", listData)
	}

	// Get by id must not include any byte from raw.
	_, getData := doRequest(t, api, http.MethodGet, "/vault/v1/keys/"+id, "good", nil)
	if bytes.Contains(getData, raw) {
		t.Fatalf("get leaks secret: %s", getData)
	}

	// Delete must not include any byte from raw.
	revRef := src.records[id]
	_, delData := doRequest(t, api, http.MethodDelete, "/vault/v1/keys/"+id+"?revision="+revRef.Revision, "good", nil)
	if bytes.Contains(delData, raw) {
		t.Fatalf("delete leaks secret: %s", delData)
	}
}

// ----- unauthenticated -----

func TestVaultUnauthenticated(t *testing.T) {
	src := newStubSource()
	acc := stubAccess{Allowed: true, Role: identity.Owner, User: "alice"}
	api := newAPI(t, src, acc)
	id := random32Hex(t)
	body := mustJSON(t, SecretPutRequest{ID: id, Purpose: "wireguard-private", Payload: base64.StdEncoding.EncodeToString(bytes.Repeat([]byte("x"), 31))})
	w, _ := doRequest(t, api, http.MethodPost, "/vault/v1/keys", "", body)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status: %d", w.Code)
	}
	if src.puts != 0 {
		t.Fatalf("source must not be touched on unauth: %d puts", src.puts)
	}
}

// ----- non-Owner -----

func TestVaultNonOwner(t *testing.T) {
	src := newStubSource()
	acc := stubAccess{Allowed: true, Role: identity.NetworkAdmin, User: "bob"}
	api := newAPI(t, src, acc)
	id := random32Hex(t)
	body := mustJSON(t, SecretPutRequest{ID: id, Purpose: "wireguard-private", Payload: base64.StdEncoding.EncodeToString(bytes.Repeat([]byte("x"), 31))})
	w, _ := doRequest(t, api, http.MethodPost, "/vault/v1/keys", "good", body)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status: %d", w.Code)
	}
	if src.puts != 0 {
		t.Fatalf("source must not be touched by non-Owner: %d puts", src.puts)
	}
	// Same gate for list / get / delete.
	if w, _ = doRequest(t, api, http.MethodGet, "/vault/v1/keys", "good", nil); w.Code != http.StatusForbidden {
		t.Fatalf("list status: %d", w.Code)
	}
	if w, _ = doRequest(t, api, http.MethodGet, "/vault/v1/keys/"+id, "good", nil); w.Code != http.StatusForbidden {
		t.Fatalf("get status: %d", w.Code)
	}
	if w, _ = doRequest(t, api, http.MethodDelete, "/vault/v1/keys/"+id+"?revision=00000000000000000000000000000001", "good", nil); w.Code != http.StatusForbidden {
		t.Fatalf("delete status: %d", w.Code)
	}
}

// ----- oversized payload -----

func TestVaultOversizedPayload(t *testing.T) {
	src := newStubSource()
	acc := stubAccess{Allowed: true, Role: identity.Owner, User: "alice"}
	api := newAPI(t, src, acc)
	id := random32Hex(t)
	// 17 KiB > 16 KiB ceiling
	big := bytes.Repeat([]byte{0x42}, 17<<10)
	body := mustJSON(t, SecretPutRequest{ID: id, Purpose: "wireguard-private", Payload: base64.StdEncoding.EncodeToString(big)})
	w, _ := doRequest(t, api, http.MethodPost, "/vault/v1/keys", "good", body)
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status: %d", w.Code)
	}
	if src.puts != 0 {
		t.Fatalf("source must not be touched on oversized: %d puts", src.puts)
	}
}

// ----- empty payload -----

func TestVaultEmptyPayload(t *testing.T) {
	src := newStubSource()
	acc := stubAccess{Allowed: true, Role: identity.Owner, User: "alice"}
	api := newAPI(t, src, acc)
	id := random32Hex(t)
	body := mustJSON(t, SecretPutRequest{ID: id, Purpose: "wireguard-private", Payload: ""})
	w, _ := doRequest(t, api, http.MethodPost, "/vault/v1/keys", "good", body)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status: %d", w.Code)
	}
}

// ----- bad purpose / bad id / malformed base64 -----

func TestVaultInvalidPurpose(t *testing.T) {
	src := newStubSource()
	acc := stubAccess{Allowed: true, Role: identity.Owner, User: "alice"}
	api := newAPI(t, src, acc)
	id := random32Hex(t)
	body := mustJSON(t, SecretPutRequest{ID: id, Purpose: "totally-unsupported", Payload: base64.StdEncoding.EncodeToString(bytes.Repeat([]byte("x"), 31))})
	w, _ := doRequest(t, api, http.MethodPost, "/vault/v1/keys", "good", body)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status: %d", w.Code)
	}
}

// TestVaultAcceptsXrayTLSPurposes locks in BETA.2 P065 typed
// vault binding for Xray TLS / REALITY: each new purpose must
// be accepted by the handler so the operator can store the
// matching secret bytes through the public API.
func TestVaultAcceptsXrayTLSPurposes(t *testing.T) {
	src := newStubSource()
	acc := stubAccess{Allowed: true, Role: identity.Owner, User: "alice"}
	api := newAPI(t, src, acc)
	for _, purpose := range []string{"tls-certificate", "reality-private", "reality-shortid"} {
		id := random32Hex(t)
		body := mustJSON(t, SecretPutRequest{
			ID:      id,
			Purpose: purpose,
			Payload: base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0xab}, 32)),
		})
		w, _ := doRequest(t, api, http.MethodPost, "/vault/v1/keys", "good", body)
		if w.Code != http.StatusCreated {
			t.Fatalf("purpose %s: status %d body %s", purpose, w.Code, w.Body.String())
		}
		var resp SecretPutResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("purpose %s: unmarshal: %v", purpose, err)
		}
		if resp.Purpose != purpose {
			t.Fatalf("purpose %s: resp.Purpose = %s", purpose, resp.Purpose)
		}
	}
}

func TestVaultInvalidID(t *testing.T) {
	src := newStubSource()
	acc := stubAccess{Allowed: true, Role: identity.Owner, User: "alice"}
	api := newAPI(t, src, acc)
	body := mustJSON(t, SecretPutRequest{ID: "not-hex", Purpose: "wireguard-private", Payload: base64.StdEncoding.EncodeToString(bytes.Repeat([]byte("x"), 31))})
	w, _ := doRequest(t, api, http.MethodPost, "/vault/v1/keys", "good", body)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status: %d", w.Code)
	}
}

func TestVaultBadBase64(t *testing.T) {
	src := newStubSource()
	acc := stubAccess{Allowed: true, Role: identity.Owner, User: "alice"}
	api := newAPI(t, src, acc)
	id := random32Hex(t)
	body := mustJSON(t, SecretPutRequest{ID: id, Purpose: "wireguard-private", Payload: "!!!not-base64!!!"})
	w, _ := doRequest(t, api, http.MethodPost, "/vault/v1/keys", "good", body)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status: %d", w.Code)
	}
}

// ----- rotation: same id, different revision -----

func TestVaultRotation(t *testing.T) {
	src := newStubSource()
	acc := stubAccess{Allowed: true, Role: identity.Owner, User: "alice"}
	api := newAPI(t, src, acc)
	id := random32Hex(t)
	for i := 0; i < 3; i++ {
		raw := bytes.Repeat([]byte{byte('a' + i)}, 32)
		body := mustJSON(t, SecretPutRequest{ID: id, Purpose: "wireguard-private", Payload: base64.StdEncoding.EncodeToString(raw)})
		w, data := doRequest(t, api, http.MethodPost, "/vault/v1/keys", "good", body)
		if w.Code != http.StatusCreated {
			t.Fatalf("rotation %d: status %d body=%s", i, w.Code, data)
		}
		var resp SecretPutResponse
		if err := json.Unmarshal(data, &resp); err != nil {
			t.Fatalf("rotation %d: decode %v", i, err)
		}
		if resp.VaultID != id {
			t.Fatalf("rotation %d: id changed: %s vs %s", i, resp.VaultID, id)
		}
	}
	// Last write wins.
	refs, _ := src.List()
	if len(refs) != 1 {
		t.Fatalf("expected 1 record after rotation, got %d", len(refs))
	}
}

// ----- agent unavailable (nil source) -----

func TestVaultAgentUnavailable(t *testing.T) {
	api := &API{Source: nil, Access: stubAccess{Allowed: true, Role: identity.Owner, User: "alice"}, Prefix: "/vault/v1/"}
	w, _ := doRequest(t, api, http.MethodGet, "/vault/v1/keys", "good", nil)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status: %d", w.Code)
	}
}

// ----- unknown method on /keys -----

func TestVaultUnknownMethod(t *testing.T) {
	src := newStubSource()
	acc := stubAccess{Allowed: true, Role: identity.Owner, User: "alice"}
	api := newAPI(t, src, acc)
	w, _ := doRequest(t, api, http.MethodPatch, "/vault/v1/keys", "good", nil)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status: %d", w.Code)
	}
}

// ----- delete with wrong revision -----

func TestVaultDeleteWrongRevision(t *testing.T) {
	src := newStubSource()
	acc := stubAccess{Allowed: true, Role: identity.Owner, User: "alice"}
	api := newAPI(t, src, acc)
	id := random32Hex(t)
	body := mustJSON(t, SecretPutRequest{ID: id, Purpose: "wireguard-private", Payload: base64.StdEncoding.EncodeToString(bytes.Repeat([]byte("x"), 31))})
	_, _ = doRequest(t, api, http.MethodPost, "/vault/v1/keys", "good", body)
	w, _ := doRequest(t, api, http.MethodDelete, "/vault/v1/keys/"+id+"?revision=ffffffffffffffffffffffffffffffff", "good", nil)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status: %d", w.Code)
	}
}

// ----- delete with valid revision returns 204 -----

func TestVaultDeleteSuccess(t *testing.T) {
	src := newStubSource()
	acc := stubAccess{Allowed: true, Role: identity.Owner, User: "alice"}
	api := newAPI(t, src, acc)
	id := random32Hex(t)
	body := mustJSON(t, SecretPutRequest{ID: id, Purpose: "wireguard-private", Payload: base64.StdEncoding.EncodeToString(bytes.Repeat([]byte("x"), 31))})
	_, _ = doRequest(t, api, http.MethodPost, "/vault/v1/keys", "good", body)
	rev := src.records[id].Revision
	w, _ := doRequest(t, api, http.MethodDelete, "/vault/v1/keys/"+id+"?revision="+rev, "good", nil)
	if w.Code != http.StatusNoContent {
		t.Fatalf("status: %d", w.Code)
	}
	if len(src.records) != 0 {
		t.Fatalf("record still present after delete")
	}
}

// ----- restart persistence: Source state survives across
// the API lifecycle (the stub is process-local; production
// uses the durable store on disk via VaultLifecycle).

func TestVaultRestartPersistence(t *testing.T) {
	src := newStubSource()
	acc := stubAccess{Allowed: true, Role: identity.Owner, User: "alice"}

	// Phase 1: write.
	api1 := newAPI(t, src, acc)
	id := random32Hex(t)
	body := mustJSON(t, SecretPutRequest{ID: id, Purpose: "wireguard-private", Payload: base64.StdEncoding.EncodeToString(bytes.Repeat([]byte("x"), 31))})
	if w, _ := doRequest(t, api1, http.MethodPost, "/vault/v1/keys", "good", body); w.Code != http.StatusCreated {
		t.Fatalf("phase 1 put status: %d", w.Code)
	}

	// Phase 2: spin up a new API against the same Source,
	// simulating a restart. List must surface the record.
	api2 := newAPI(t, src, acc)
	w, data := doRequest(t, api2, http.MethodGet, "/vault/v1/keys", "good", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("phase 2 list status: %d", w.Code)
	}
	if !bytes.Contains(data, []byte(id)) {
		t.Fatalf("record id missing after restart: %s", data)
	}
}

// ----- bounded body: real http.MaxBytesReader -----

func TestVaultBoundedBody(t *testing.T) {
	src := newStubSource()
	acc := stubAccess{Allowed: true, Role: identity.Owner, User: "alice"}
	api := newAPI(t, src, acc)
	// 17 KiB > MaxRequestBytes
	huge := bytes.Repeat([]byte{0x20}, 17<<10)
	w, _ := doRequest(t, api, http.MethodPost, "/vault/v1/keys", "good", huge)
	if w.Code != http.StatusRequestEntityTooLarge && w.Code != http.StatusBadRequest {
		t.Fatalf("status: %d", w.Code)
	}
}

// ----- helpers -----

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// silence unused imports when test helpers are pruned.
var _ = strings.NewReader
var _ = io.Discard
var _ = time.Second
