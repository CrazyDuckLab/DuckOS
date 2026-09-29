package wireguard

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/CrazyDuckLab/DuckOS/internal/identity"
)

// Access is the narrow contract the WireGuard API uses for
// authorisation. The implementation is provided by the identity
// service; the API never inspects tokens directly.
type Access interface {
	Authorize(token string, p identity.Permission) (identity.User, error)
}

// CatalogPersistence is the narrow contract the WireGuard API uses to
// record server and peer revisions. The catalog is the only durable
// source of truth for these records.
type CatalogPersistence interface {
	UpsertServer(ctx context.Context, server Server) error
	RemoveServer(ctx context.Context, id string) error
	UpsertPeer(ctx context.Context, peer Peer) error
	RemovePeer(ctx context.Context, id string) error
	Server(ctx context.Context, id string) (Server, error)
	Peer(ctx context.Context, id string) (Peer, error)
	ListServers(ctx context.Context) ([]Server, error)
	ListPeers(ctx context.Context, serverID string) ([]Peer, error)
}

// API is the loopback HTTP surface for the WireGuard service. The
// service is enabled on the Agent loopback alongside the network
// service. It never logs key bytes, even on the failure path.
type API struct {
	mu      sync.Mutex
	Access  Access
	Catalog CatalogPersistence
	Adapter *KernelAdapter
	Prefix  string
}

// ServeHTTP dispatches /wireguard/v1/{servers,peers} and refuses
// anything else.
func (a *API) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if a.Access == nil || a.Catalog == nil || a.Adapter == nil {
		problem(w, 500, "wireguard_unavailable", "WireGuard service is not enabled.")
		return
	}
	path := r.URL.Path
	switch {
	case strings.HasPrefix(path, a.Prefix+"servers"):
		a.serveServers(w, r)
	case strings.HasPrefix(path, a.Prefix+"peers"):
		a.servePeers(w, r)
	default:
		problem(w, 404, "not_found", "Unknown wireguard operation.")
	}
}

func (a *API) serveServers(w http.ResponseWriter, r *http.Request) {
	if _, err := a.Access.Authorize(extractToken(r), identity.ConfigureNetwork); err != nil {
		problem(w, 403, "forbidden", "Network write permission is required.")
		return
	}
	id := strings.TrimPrefix(r.URL.Path, a.Prefix+"servers")
	switch {
	case id == "" && r.Method == http.MethodGet:
		servers, err := a.Catalog.ListServers(r.Context())
		if err != nil {
			problem(w, 503, "wireguard_unavailable", "WireGuard catalog is unavailable.")
			return
		}
		writeJSON(w, 200, servers)
	case id == "" && r.Method == http.MethodPost:
		var server Server
		if err := decodeJSON(r, &server); err != nil {
			problem(w, 400, "invalid_request", "Invalid WireGuard server payload.")
			return
		}
		if err := a.upsertServer(r.Context(), server); err != nil {
			problem(w, 422, "invalid_server", err.Error())
			return
		}
		writeJSON(w, 201, server)
	case strings.HasSuffix(id, "/export") && r.Method == http.MethodGet:
		// /wireguard/v1/servers/{id}/export returns the
		// rendered wg-quick configuration with the private
		// key redacted. Authorization is the same
		// ConfigureNetwork gate as the rest of the server
		// surface; secret export is intentionally NOT
		// available here.
		serverID := strings.TrimSuffix(strings.TrimPrefix(id, "/"), "/export")
		server, err := a.Catalog.Server(r.Context(), serverID)
		if err != nil {
			problem(w, 404, "not_found", "WireGuard server not found.")
			return
		}
		peers, err := a.Catalog.ListPeers(r.Context(), serverID)
		if err != nil {
			problem(w, 503, "wireguard_unavailable", "WireGuard catalog rejected the peer listing.")
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(ExportServerConfig(server, peers)))
	case id != "" && r.Method == http.MethodGet:
		server, err := a.Catalog.Server(r.Context(), strings.TrimPrefix(id, "/"))
		if err != nil {
			problem(w, 404, "not_found", "WireGuard server not found.")
			return
		}
		writeJSON(w, 200, server)
	case id != "" && r.Method == http.MethodPut:
		var server Server
		if err := decodeJSON(r, &server); err != nil {
			problem(w, 400, "invalid_request", "Invalid WireGuard server payload.")
			return
		}
		if server.ID != strings.TrimPrefix(id, "/") {
			problem(w, 409, "conflict", "Server ID does not match the URL.")
			return
		}
		if err := a.upsertServer(r.Context(), server); err != nil {
			problem(w, 422, "invalid_server", err.Error())
			return
		}
		writeJSON(w, 200, server)
	case id != "" && r.Method == http.MethodDelete:
		if err := a.Adapter.RemoveServer(r.Context(), Server{Interface: serverInterfaceByID(id, a.Catalog)}); err != nil {
			problem(w, 503, "wireguard_unavailable", err.Error())
			return
		}
		if err := a.Catalog.RemoveServer(r.Context(), strings.TrimPrefix(id, "/")); err != nil {
			problem(w, 503, "wireguard_unavailable", "WireGuard catalog rejected the remove.")
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	default:
		w.Header().Set("Allow", "GET, POST, PUT, DELETE")
		problem(w, 405, "method_not_allowed", "Method is not supported for this server.")
	}
}

func (a *API) servePeers(w http.ResponseWriter, r *http.Request) {
	if _, err := a.Access.Authorize(extractToken(r), identity.ConfigureNetwork); err != nil {
		problem(w, 403, "forbidden", "Network write permission is required.")
		return
	}
	id := strings.TrimPrefix(r.URL.Path, a.Prefix+"peers")
	switch {
	case id == "" && r.Method == http.MethodGet:
		// List all peers for the supplied server. The
		// caller pins the server with ?server=ID so the
		// catalog never returns the global peer list.
		serverID := r.URL.Query().Get("server")
		if serverID == "" {
			problem(w, 400, "invalid_request", "WireGuard peer listing requires ?server=ID.")
			return
		}
		peers, err := a.Catalog.ListPeers(r.Context(), serverID)
		if err != nil {
			problem(w, 503, "wireguard_unavailable", "WireGuard catalog rejected the peer listing.")
			return
		}
		writeJSON(w, 200, peers)
	case id == "" && r.Method == http.MethodPost:
		var peer Peer
		if err := decodeJSON(r, &peer); err != nil {
			problem(w, 400, "invalid_request", "Invalid WireGuard peer payload.")
			return
		}
		if err := peer.Validate(peer.ServerID); err != nil {
			problem(w, 422, "invalid_peer", err.Error())
			return
		}
		if err := a.upsertPeer(r.Context(), peer); err != nil {
			problem(w, 422, "invalid_peer", err.Error())
			return
		}
		writeJSON(w, 201, peer)
	case id != "" && r.Method == http.MethodGet:
		peer, err := a.Catalog.Peer(r.Context(), strings.TrimPrefix(id, "/"))
		if err != nil {
			problem(w, 404, "not_found", "WireGuard peer not found.")
			return
		}
		writeJSON(w, 200, peer)
	case id != "" && r.Method == http.MethodPut:
		var peer Peer
		if err := decodeJSON(r, &peer); err != nil {
			problem(w, 400, "invalid_request", "Invalid WireGuard peer payload.")
			return
		}
		if peer.ID != strings.TrimPrefix(id, "/") {
			problem(w, 409, "conflict", "Peer ID does not match the URL.")
			return
		}
		if err := peer.Validate(peer.ServerID); err != nil {
			problem(w, 422, "invalid_peer", err.Error())
			return
		}
		if err := a.upsertPeer(r.Context(), peer); err != nil {
			problem(w, 422, "invalid_peer", err.Error())
			return
		}
		writeJSON(w, 200, peer)
	case id != "" && r.Method == http.MethodDelete:
		peerID := strings.TrimPrefix(id, "/")
		// The kernel adapter needs both the
		// serverID (to find the interface) and the
		// public key (to identify the peer). The
		// catalog is the source of truth.
		existing, err := a.Catalog.Peer(r.Context(), peerID)
		if err != nil {
			problem(w, 404, "not_found", "WireGuard peer not found.")
			return
		}
		if err := a.Adapter.RemovePeer(r.Context(), existing); err != nil {
			problem(w, 503, "wireguard_unavailable", err.Error())
			return
		}
		if err := a.Catalog.RemovePeer(r.Context(), peerID); err != nil {
			problem(w, 503, "wireguard_unavailable", "WireGuard catalog rejected the remove.")
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	default:
		w.Header().Set("Allow", "GET, POST, PUT, DELETE")
		problem(w, 405, "method_not_allowed", "Method is not supported for this peer.")
	}
}

// upsertPeer applies the peer to the kernel and persists it
// in the catalog. The two operations are kept ordered: the
// kernel is updated first so a partial write to the catalog
// never advertises a peer that the kernel cannot honour.
func (a *API) upsertPeer(ctx context.Context, peer Peer) error {
	if err := peer.Validate(peer.ServerID); err != nil {
		return err
	}
	if err := a.Adapter.ApplyPeer(ctx, peer); err != nil {
		return err
	}
	return a.Catalog.UpsertPeer(ctx, peer)
}

func (a *API) upsertServer(ctx context.Context, server Server) error {
	if err := server.Validate(); err != nil {
		return err
	}
	if err := a.Adapter.ApplyServer(ctx, server); err != nil {
		return err
	}
	return a.Catalog.UpsertServer(ctx, server)
}

func serverInterfaceByID(id string, catalog CatalogPersistence) string {
	id = strings.TrimPrefix(id, "/")
	server, err := catalog.Server(context.Background(), id)
	if err != nil {
		return ""
	}
	return server.Interface
}

func extractToken(r *http.Request) string {
	v := r.Header.Get("Authorization")
	if !strings.HasPrefix(v, "Bearer ") {
		return ""
	}
	return strings.TrimPrefix(v, "Bearer ")
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func problem(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}

func decodeJSON(r *http.Request, v any) error {
	if r.ContentLength > 1<<20 {
		return errors.New("payload exceeds 1 MiB")
	}
	if r.Method == http.MethodGet {
		return errors.New("GET requests cannot contain bodies")
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("read body: %w", err)
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}
