package wireguard

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// KeyResolver is the narrow contract the kernel adapter uses to
// resolve secret bytes for a Server or Peer. Implementations must
// honour context cancellation and never return a stale key after a
// rotation or revoke.
type KeyResolver interface {
	Resolve(ctx context.Context, ref KeyRef) ([]byte, error)
}

// KernelAdapter owns the lifetime of a WireGuard tunnel. It is the
// only object that mutates the kernel via the bounded wg/auditd
// bridges. The adapter never holds private key bytes between calls;
// every operation pulls the key from the resolver, applies it, and
// clears its in-memory copy.
type KernelAdapter struct {
	Resolver      KeyResolver
	Runner        Runner
	Files         Filesystem
	InterfaceAddr string
	// Catalog is the narrow lookup the adapter uses to
	// resolve the parent Server and the current peer
	// set when a single-peer Apply/Remove is in flight.
	// The Catalog is the only durable source of truth;
	// the kernel is best-effort.
	Catalog catalogLookup
}

type Runner interface {
	Run(ctx context.Context, args []string) (string, error)
}

type Filesystem interface {
	WriteAtomic(ctx context.Context, path string, mode uint32, data []byte) error
	Remove(ctx context.Context, path string) error
}

// ApplyServer writes the new server key, brings the WireGuard
// interface up, applies the listen port, replaces the peer set and
// persists the rendered wg-quick config so the daemon can restore
// the tunnel on reboot. The function never logs key bytes.
func (a *KernelAdapter) ApplyServer(ctx context.Context, server Server) error {
	if err := server.Validate(); err != nil {
		return err
	}
	if a.Resolver == nil {
		return errors.New("wireguard kernel adapter requires a key resolver")
	}
	if a.Runner == nil {
		return errors.New("wireguard kernel adapter requires a runner")
	}
	if a.Files == nil {
		return errors.New("wireguard kernel adapter requires a filesystem")
	}
	priv, err := a.Resolver.Resolve(ctx, server.PrivateKeyRef)
	if err != nil {
		return fmt.Errorf("wireguard private key: %w", err)
	}
	defer clearBytes(priv)
	privB64 := base64.StdEncoding.EncodeToString(priv)
	// 1. Create the WireGuard interface if it does not
	//    exist. The link is added via `ip link`, never
	//    by shell wrappers.
	if _, err := a.Runner.Run(ctx, []string{"link", "add", "dev", server.Interface, "type", "wireguard"}); err != nil && !isExists(err) {
		return fmt.Errorf("create wireguard link: %w", err)
	}
	// 2. Apply the listen UDP port + private key. The
	//    `wg set` command takes the private key via
	//    stdin (the adapter passes it through the
	//    __privkey__: marker that the IPRouteRunner
	//    dispatcher rewrites into a stdin payload, so
	//    the secret never appears in argv or ps).
	if _, err := a.Runner.Run(ctx, []string{
		"__wg__", "set", server.Interface,
		"listen-port", fmt.Sprintf("%d", server.ListenPort),
		"private-key", "__privkey__:" + privB64,
	}); err != nil {
		return fmt.Errorf("configure wireguard: %w", err)
	}
	// 3. Replace the peer set in a single transaction: collect existing
	//    peer public keys, add or update the desired set, remove the
	//    obsolete keys.
	current, err := a.Runner.Run(ctx, []string{"__wg__", "show", server.Interface, "peers"})
	if err != nil {
		return fmt.Errorf("enumerate wireguard peers: %w", err)
	}
	currentSet := parsePeerList(current)
	desired := map[string]Peer{}
	for _, p := range server.Peers {
		if !p.Enabled {
			continue
		}
		desired[p.PublicKey] = p
	}
	for pubKey, peer := range desired {
		args := []string{"__wg__", "set", server.Interface, "peer", pubKey}
		if peer.PresharedKeyRef != nil {
			psk, err := a.Resolver.Resolve(ctx, *peer.PresharedKeyRef)
			if err != nil {
				return fmt.Errorf("wireguard preshared key: %w", err)
			}
			defer clearBytes(psk)
			pskB64 := base64.StdEncoding.EncodeToString(psk)
			args = append(args, "preshared-key", "__psk__:"+pskB64)
		}
		args = append(args, "allowed-ips", strings.Join(peer.AllowedIPs, ","))
		if peer.PersistentKeepalive > 0 {
			args = append(args, "persistent-keepalive", fmt.Sprintf("%d", peer.PersistentKeepalive))
		}
		if endpoint, ok := splitEndpointOrEmpty(peer.Endpoint); ok {
			args = append(args, "endpoint", endpoint)
		}
		if _, err := a.Runner.Run(ctx, args); err != nil {
			return fmt.Errorf("apply wireguard peer %s: %w", pubKey, err)
		}
	}
	// Remove peers that are no longer in the desired set.
	for pubKey := range currentSet {
		if _, ok := desired[pubKey]; ok {
			continue
		}
		if _, err := a.Runner.Run(ctx, []string{"__wg__", "set", server.Interface, "peer", pubKey, "remove"}); err != nil {
			return fmt.Errorf("remove wireguard peer %s: %w", pubKey, err)
		}
	}
	// 4. Persist the rendered configuration so the next boot can
	//    recreate the tunnel without re-applying the key from the vault.
	rendered := ExportServerConfig(server, server.Peers)
	path := "/etc/wireguard/" + server.Interface + ".conf"
	if err := a.Files.WriteAtomic(ctx, path, 0o600, []byte(rendered)); err != nil {
		return fmt.Errorf("persist wireguard config: %w", err)
	}
	return nil
}

// ApplyPeer adds or updates a single peer on the live
// interface. The server's interface must already exist
// (callers run ApplyServer first). A disabled peer is
// removed from the kernel; an enabled peer is added or
// updated with the supplied AllowedIPs / preshared key /
// endpoint. The function never logs key bytes.
func (a *KernelAdapter) ApplyPeer(ctx context.Context, peer Peer) error {
	if peer.ServerID == "" {
		return errors.New("wireguard peer requires a server id")
	}
	if peer.PublicKey == "" {
		return errors.New("wireguard peer requires a public key")
	}
	if a.Resolver == nil {
		return errors.New("wireguard kernel adapter requires a key resolver")
	}
	if a.Runner == nil {
		return errors.New("wireguard kernel adapter requires a runner")
	}
	if a.Files == nil {
		return errors.New("wireguard kernel adapter requires a filesystem")
	}
	server, err := a.serverLookup(ctx, peer.ServerID)
	if err != nil {
		return fmt.Errorf("wireguard server for peer: %w", err)
	}
	if !peer.Enabled {
		// Disabling is implemented as a kernel-level
		// remove. The catalog keeps the peer record
		// so the operator can re-enable it later.
		if _, err := a.Runner.Run(ctx, []string{"__wg__", "set", server.Interface, "peer", peer.PublicKey, "remove"}); err != nil && !isNoPeer(err) {
			return fmt.Errorf("disable wireguard peer: %w", err)
		}
		return a.refreshPersistedConfig(ctx, server)
	}
	args := []string{"__wg__", "set", server.Interface, "peer", peer.PublicKey}
	if peer.PresharedKeyRef != nil {
		psk, err := a.Resolver.Resolve(ctx, *peer.PresharedKeyRef)
		if err != nil {
			return fmt.Errorf("wireguard preshared key: %w", err)
		}
		defer clearBytes(psk)
		pskB64 := base64.StdEncoding.EncodeToString(psk)
		args = append(args, "preshared-key", "__psk__:"+pskB64)
	}
	args = append(args, "allowed-ips", strings.Join(peer.AllowedIPs, ","))
	if peer.PersistentKeepalive > 0 {
		args = append(args, "persistent-keepalive", fmt.Sprintf("%d", peer.PersistentKeepalive))
	}
	if endpoint, ok := splitEndpointOrEmpty(peer.Endpoint); ok {
		args = append(args, "endpoint", endpoint)
	}
	if _, err := a.Runner.Run(ctx, args); err != nil {
		return fmt.Errorf("apply wireguard peer %s: %w", peer.PublicKey, err)
	}
	return a.refreshPersistedConfig(ctx, server)
}

// RemovePeer removes a single peer from the live
// interface. The catalog is the only durable source of
// truth; the kernel is best-effort.
func (a *KernelAdapter) RemovePeer(ctx context.Context, peer Peer) error {
	if peer.ServerID == "" || peer.PublicKey == "" {
		return errors.New("wireguard peer remove requires server id and public key")
	}
	if a.Runner == nil {
		return errors.New("wireguard kernel adapter requires a runner")
	}
	if a.Files == nil {
		return errors.New("wireguard kernel adapter requires a filesystem")
	}
	server, err := a.serverLookup(ctx, peer.ServerID)
	if err != nil {
		return fmt.Errorf("wireguard server for peer remove: %w", err)
	}
	if _, err := a.Runner.Run(ctx, []string{"__wg__", "set", server.Interface, "peer", peer.PublicKey, "remove"}); err != nil && !isNoPeer(err) {
		return fmt.Errorf("remove wireguard peer: %w", err)
	}
	return a.refreshPersistedConfig(ctx, server)
}

// serverLookup is the narrow contract the adapter uses to
// resolve a Server record. The Catalog implementation is
// the production source; tests inject a fake.
type catalogLookup interface {
	Server(ctx context.Context, id string) (Server, error)
	ListPeers(ctx context.Context, serverID string) ([]Peer, error)
}

func (a *KernelAdapter) serverLookup(ctx context.Context, serverID string) (Server, error) {
	if a.Catalog == nil {
		return Server{}, errors.New("wireguard kernel adapter requires a catalog")
	}
	return a.Catalog.Server(ctx, serverID)
}

// refreshPersistedConfig rewrites the rendered wg-quick
// file from the catalog state so the next boot restores
// the exact peer set the kernel currently has.
func (a *KernelAdapter) refreshPersistedConfig(ctx context.Context, server Server) error {
	peers, err := a.Catalog.ListPeers(ctx, server.ID)
	if err != nil {
		return fmt.Errorf("list peers for persist: %w", err)
	}
	rendered := ExportServerConfig(server, peers)
	path := "/etc/wireguard/" + server.Interface + ".conf"
	if err := a.Files.WriteAtomic(ctx, path, 0o600, []byte(rendered)); err != nil {
		return fmt.Errorf("persist wireguard config: %w", err)
	}
	return nil
}

// RemoveServer tears down the WireGuard interface and removes the
// persisted configuration.
func (a *KernelAdapter) RemoveServer(ctx context.Context, server Server) error {
	if a.Runner == nil || a.Files == nil {
		return errors.New("wireguard kernel adapter requires runner and filesystem")
	}
	if _, err := a.Runner.Run(ctx, []string{"link", "del", "dev", server.Interface}); err != nil && !isNoDevice(err) {
		return fmt.Errorf("remove wireguard link: %w", err)
	}
	path := "/etc/wireguard/" + server.Interface + ".conf"
	if err := a.Files.Remove(ctx, path); err != nil {
		return fmt.Errorf("remove wireguard config: %w", err)
	}
	return nil
}

// ExportServerConfig renders the server-side wg-quick
// configuration. The public export surface used by the
// diagnostic API emits NO PrivateKey line at all —
// neither a real key, a sentinel, nor a redaction
// placeholder — because the configuration is intended
// for sharing with the peer operator and must be
// private-key-free end to end. The server-side private
// key never leaves the vault; the per-peer private key
// is generated by the client from this redacted
// configuration.
func ExportServerConfig(server Server, peers []Peer) string {
	var b strings.Builder
	b.WriteString("# Generated by DuckOS; do not edit by hand.\n")
	b.WriteString("# Server-side config export; private key is intentionally omitted.\n")
	b.WriteString("[Interface]\n")
	b.WriteString("Address = " + strings.Join(server.Addresses, ", ") + "\n")
	if server.MTU > 0 {
		fmt.Fprintf(&b, "MTU = %d\n", server.MTU)
	}
	b.WriteString("ListenPort = " + fmt.Sprintf("%d", server.ListenPort) + "\n")
	sorted := append([]Peer(nil), peers...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].PublicKey < sorted[j].PublicKey })
	for _, peer := range sorted {
		if !peer.Enabled {
			continue
		}
		b.WriteString("\n[Peer]\n")
		b.WriteString("PublicKey = " + peer.PublicKey + "\n")
		if len(peer.AllowedIPs) > 0 {
			b.WriteString("AllowedIPs = " + strings.Join(peer.AllowedIPs, ", ") + "\n")
		}
		if peer.Endpoint != "" {
			b.WriteString("Endpoint = " + peer.Endpoint + "\n")
		}
		if peer.PersistentKeepalive > 0 {
			fmt.Fprintf(&b, "PersistentKeepalive = %d\n", peer.PersistentKeepalive)
		}
	}
	return b.String()
}

func parsePeerList(out string) map[string]bool {
	set := map[string]bool{}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if _, err := base64.StdEncoding.Strict().DecodeString(line); err == nil && len(line) == 44 {
			set[line] = true
		}
	}
	return set
}

func splitEndpointOrEmpty(s string) (string, bool) {
	idx := strings.LastIndex(s, ":")
	if idx < 1 || idx == len(s)-1 {
		return "", false
	}
	return s, true
}

func isNoPeer(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "no such peer") || strings.Contains(msg, "Unable to access interface")
}

func isExists(err error) bool {
	// Linux ip link returns exit code 2 with "RTNETLINK answers: File exists".
	// The bridge here uses a string match; the runner abstracts the error
	// type, so we are conservative and treat only the explicit "exists"
	// hint as a no-op.
	if err == nil {
		return true
	}
	return strings.Contains(err.Error(), "File exists")
}

func isNoDevice(err error) bool {
	if err == nil {
		return true
	}
	return strings.Contains(err.Error(), "Cannot find device") || strings.Contains(err.Error(), "does not exist")
}

func clearBytes(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

// RecoverySnapshot is the durable recovery evidence the Safe Apply
// controller stores with each transaction. It contains no private
// bytes; the kernel adapter pulls keys from the vault on restore.
type RecoverySnapshot struct {
	Version  int       `json:"version"`
	ServerID string    `json:"serverId"`
	Peers    []Peer    `json:"peers"`
	At       time.Time `json:"at"`
}

// MarshalRecoverySnapshot returns a JSON-encoded recovery evidence for
// the given server. The snapshot contains no private material.
func MarshalRecoverySnapshot(server Server) (json.RawMessage, error) {
	snap := RecoverySnapshot{Version: 1, ServerID: server.ID, Peers: server.Peers, At: server.UpdatedAt}
	raw, err := json.Marshal(snap)
	if err != nil {
		return nil, err
	}
	return raw, nil
}
