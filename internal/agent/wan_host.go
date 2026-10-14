//go:build linux

// agent/wan_host.go: the beta.4 WAN management IPC surface.
//
//	PUBLIC -> Engine.WanProxy -> Agent IPC
//	       -> WanHost -> wan.Provider store + kernel observation
//
// Staging semantics (beta.4 milestone): configuration
// mutations validate completely and persist, and are
// reported as "staged", never as applied. Live WAN apply is
// a 🔴 РИСК operation that lands through the managed-routing
// enrollment flow and Safe Apply; the explicit apply verb
// returns 501 until that integration exists. Observation is
// read-only kernel truth.
package agent

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/identity"
	"github.com/CrazyDuckLab/DuckOS/internal/wan"
)

// WanPath is the IPC route prefix.
const WanPath = "/agent/v1/wan"

// WanStore is the durable provider list (JSON file).
type WanStore struct {
	mu   sync.Mutex
	path string
}

// NewWanStore persists providers to path atomically.
func NewWanStore(path string) *WanStore { return &WanStore{path: path} }

func (s *WanStore) Load() ([]wan.Provider, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if len(data) == 0 {
		return nil, nil
	}
	var doc struct {
		Version   int            `json:"version"`
		Providers []wan.Provider `json:"providers"`
	}
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&doc); err != nil {
		return nil, err
	}
	if doc.Version != 1 {
		return nil, errors.New("wan: unsupported store version")
	}
	for _, p := range doc.Providers {
		if err := wan.ValidProviderComplete(p); err != nil {
			return nil, err
		}
	}
	return doc.Providers, nil
}

func (s *WanStore) Save(providers []wan.Provider) error {
	data, err := json.MarshalIndent(struct {
		Version   int            `json:"version"`
		Providers []wan.Provider `json:"providers"`
	}{Version: 1, Providers: providers}, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// WanHost serves the WAN surface.
type WanHost struct {
	mu     sync.Mutex
	store  *WanStore
	Access UPnPAccess
}

// NewWanHost wires the host.
func NewWanHost(store *WanStore, acc UPnPAccess) (*WanHost, error) {
	if _, err := store.Load(); err != nil {
		return nil, err
	}
	return &WanHost{store: store, Access: acc}, nil
}

// ServeHTTP dispatches /agent/v1/wan/*:
//
//	GET    /wan/v1/providers       -> staged configuration
//	PUT    /wan/v1/providers/{id}  -> validate + stage
//	DELETE /wan/v1/providers/{id}  -> remove from staging
//	POST   /wan/v1/apply           -> 501 (pending Safe Apply integration)
//	GET    /wan/v1/status          -> kernel observation
func (h *WanHost) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	tail := strings.Trim(strings.TrimPrefix(r.URL.EscapedPath(), WanPath), "/")
	switch {
	case r.Method == http.MethodGet && tail == "providers":
		h.authorized(w, r, identity.Observe, h.handleGetProviders)
	case r.Method == http.MethodGet && tail == "status":
		h.authorized(w, r, identity.Observe, h.handleStatus)
	case r.Method == http.MethodPut && strings.HasPrefix(tail, "providers/"):
		id := strings.TrimPrefix(tail, "providers/")
		h.authorized(w, r, identity.ConfigureNetwork, func(w http.ResponseWriter, r *http.Request) {
			h.handlePut(w, r, id)
		})
	case r.Method == http.MethodDelete && strings.HasPrefix(tail, "providers/"):
		id := strings.TrimPrefix(tail, "providers/")
		h.authorized(w, r, identity.ConfigureNetwork, func(w http.ResponseWriter, r *http.Request) {
			h.handleDelete(w, r, id)
		})
	case r.Method == http.MethodPost && tail == "apply":
		h.authorized(w, r, identity.ConfigureNetwork, func(w http.ResponseWriter, r *http.Request) {
			fwrProblem(w, 501, "not_implemented",
				"Live WAN apply is pending the managed-routing Safe Apply integration; the configuration is staged only.")
		})
	default:
		w.Header().Set("Allow", "GET, PUT, DELETE, POST")
		fwrProblem(w, 404, "not_found", "WAN endpoint not found.")
	}
}

func (h *WanHost) authorized(w http.ResponseWriter, r *http.Request, perm identity.Permission, next http.HandlerFunc) {
	v := r.Header.Get("Authorization")
	if len(v) < 7 || v[:7] != "Bearer " {
		fwrProblem(w, 401, "unauthorized", "Bearer token is required.")
		return
	}
	if _, err := h.Access.Authorize(v[7:], perm); err != nil {
		fwrProblem(w, 403, "forbidden", "Required permission is missing.")
		return
	}
	next(w, r)
}

func (h *WanHost) providers() []wan.Provider {
	ps, err := h.store.Load()
	if err != nil {
		return nil
	}
	return ps
}

func (h *WanHost) handleGetProviders(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	defer h.mu.Unlock()
	ps := h.providers()
	if ps == nil {
		ps = []wan.Provider{}
	}
	writeFwrJSON(w, http.StatusOK, map[string]any{
		"providers":  ps,
		"applied":    false,
		"note":       "staged configuration; live apply is pending the managed-routing integration",
		"observedAt": time.Now().UTC().Format(time.RFC3339),
	})
}

func (h *WanHost) handlePut(w http.ResponseWriter, r *http.Request, id string) {
	defer r.Body.Close()
	var body struct {
		Provider wan.Provider `json:"provider"`
	}
	dec := json.NewDecoder(io.LimitReader(r.Body, 64<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil {
		fwrProblem(w, 400, "invalid_request", "Provider body is not valid JSON.")
		return
	}
	if body.Provider.ID == "" {
		body.Provider.ID = id
	}
	if body.Provider.ID != id {
		fwrProblem(w, 400, "invalid_request", "Provider id does not match the URL.")
		return
	}
	if err := wan.ValidProviderComplete(body.Provider); err != nil {
		fwrProblem(w, 400, "invalid_request", err.Error())
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	ps := h.providers()
	replaced := false
	for i := range ps {
		if ps[i].ID == id {
			ps[i] = body.Provider
			replaced = true
			break
		}
	}
	if !replaced {
		ps = append(ps, body.Provider)
	}
	if err := h.store.Save(ps); err != nil {
		fwrProblem(w, 500, "persist_failed", err.Error())
		return
	}
	log.Printf("wan: staged provider %s mode=%s (by %s)", id, body.Provider.Mode, clientAddrOf(r))
	writeFwrJSON(w, http.StatusOK, map[string]any{
		"status": "staged",
		"applied": false,
	})
}

func (h *WanHost) handleDelete(w http.ResponseWriter, r *http.Request, id string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	ps := h.providers()
	out := ps[:0]
	found := false
	for _, p := range ps {
		if p.ID == id {
			found = true
			continue
		}
		out = append(out, p)
	}
	if !found {
		writeFwrJSON(w, http.StatusOK, map[string]any{"status": "absent"})
		return
	}
	if err := h.store.Save(out); err != nil {
		fwrProblem(w, 500, "persist_failed", err.Error())
		return
	}
	log.Printf("wan: removed staged provider %s (by %s)", id, clientAddrOf(r))
	writeFwrJSON(w, http.StatusOK, map[string]any{"status": "removed"})
}

func (h *WanHost) handleStatus(w http.ResponseWriter, r *http.Request) {
	link := r.URL.Query().Get("link")
	obs := wan.Observe(link)
	if obs == nil {
		obs = []wan.Observation{}
	}
	writeFwrJSON(w, http.StatusOK, map[string]any{
		"observations": obs,
		"observedAt":   time.Now().UTC().Format(time.RFC3339),
	})
}
