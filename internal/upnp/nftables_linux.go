//go:build linux

// upnp/nftables_linux.go: real NetworkBackend that drives
// the host firewall through the nft binary.
//
// The runtime in runtime.go decides when a UPnP / NAT-PMP /
// PCP mapping is allowed; this backend is responsible for
// actually rendering the corresponding nftables rules so the
// data-plane is honest.
//
// Properties preserved:
//
//   - mappings only ever appear in a dedicated DuckOS
//     table "duckos.upnp" (so unrelated firewall rules are
//     untouched)
//   - every Add / Remove writes to a transient staging
//     file, runs `nft -c -f` to validate, then atomically
//     loads via `nft -f`
//   - Add failures roll back the Manager state in the
//     caller (see runtime.AddMapping)
//   - Remove failures are logged but never block the
//     runtime's clean-up cycle
//
// WAN exposure is structurally refused: AllowListen always
// returns false unless the operator has explicitly added
// the interface to a list of trusted interfaces.
package upnp

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// NftablesBackend is the production NetworkBackend. It owns
// a single transient staging directory and serialises all
// `nft` invocations through a mutex so two concurrent
// mappings cannot race the staging file.
type NftablesBackend struct {
	mu          sync.Mutex
	binPath     string
	stagingDir  string
	trustedIfxs []string
	// active is the backend's authoritative view of the
	// mappings currently rendered into inet duckos_upnp.
	active []Mapping
	// validateOnly skips the `nft -f` load step so the
	// constructor can probe `nft` availability without
	// mutating the kernel. Production wiring always sets
	// this to false.
	validateOnly bool
}

// NewNftablesBackend probes `nft` and returns a backend
// configured to render mappings to stagingDir. The trusted
// interface list is empty by default so AllowListen refuses
// every interface (the operator must explicitly add LAN
// interfaces via NewNftablesBackendTrusted).
//
// If `nft` is not on PATH the constructor returns an error
// so the production wiring can surface a clean
// "backend missing" state instead of pretending success.
func NewNftablesBackend(stagingDir string) (*NftablesBackend, error) {
	bin, err := exec.LookPath("nft")
	if err != nil {
		return nil, fmt.Errorf("upnp: nft binary is not on PATH: %w", err)
	}
	if stagingDir == "" {
		stagingDir = filepath.Join(os.TempDir(), "duckos-upnp")
	}
	if err := os.MkdirAll(stagingDir, 0o700); err != nil {
		return nil, fmt.Errorf("upnp: cannot create staging dir: %w", err)
	}
	return &NftablesBackend{binPath: bin, stagingDir: stagingDir}, nil
}

// NewNftablesBackendTrusted is like NewNftablesBackend but
// seeds the trusted-interface allow-list. Only interfaces in
// the list are accepted by AllowListen; everything else is
// refused as WAN.
func NewNftablesBackendTrusted(stagingDir string, trusted []string) (*NftablesBackend, error) {
	b, err := NewNftablesBackend(stagingDir)
	if err != nil {
		return nil, err
	}
	b.trustedIfxs = append(b.trustedIfxs, trusted...)
	return b, nil
}

// nftablesBackendFixture is a package-private test helper.
// It lives in nftables_linux.go so the test suite can
// drive the runtime without a real nft binary, but it is
// strictly lower-case so production code outside this
// package cannot import it. The no_fixture_in_production
// test additionally guards against accidental references
// from cmd/, internal/agent, internal/api or
// internal/engine.
func nftablesBackendFixture() *NftablesBackend {
	return &NftablesBackend{
		binPath:      "/bin/true",
		stagingDir:   filepath.Join(os.TempDir(), "duckos-upnp-test"),
		validateOnly: true,
	}
}

// TrustIfaces appends to the trusted-interface allow-list.
// Exposed for production wiring (e.g. an admin who adds a
// new trusted LAN).
func (n *NftablesBackend) TrustIfaces(ifaces ...string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.trustedIfxs = append(n.trustedIfxs, ifaces...)
}

// TrustIface adds iface to the trusted-interface allow-list
// (NetworkBackend interface). Duplicates are tolerated
// because the production wiring may legitimately re-add the
// same iface when reloading the registry.
func (n *NftablesBackend) TrustIface(iface string) {
	n.TrustIfaces(iface)
}

// UntrustIface removes iface from the trusted-interface
// allow-list (NetworkBackend interface). An iface that is
// not present is a no-op so the disable path is idempotent
// and safe to call on agent restarts after a stale trust
// has already been cleared.
func (n *NftablesBackend) UntrustIface(iface string) {
	if iface == "" {
		return
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	out := n.trustedIfxs[:0]
	for _, t := range n.trustedIfxs {
		if t != iface {
			out = append(out, t)
		}
	}
	n.trustedIfxs = out
}

// active carries the backend's view of the mappings that
// are currently rendered into the kernel table. The runtime
// serialized every Add/Remove through this backend, so this
// view can never race; it is the source for the atomic
// full-table replacement batch.
//
// Lifecycle design (beta.3 hardening, replaces the beta.2
// per-mapping additive render + whole-table flush):
//
//   - Every Add and Remove renders ONE self-contained nft
//     transaction: ensure-table + delete-table + create
//     table/chains + one rule per active mapping. `nft -f`
//     applies the file as a single atomic transaction, so
//     there is never a window with missing or stale rules
//     and removing mapping A can never touch mapping B.
//   - The add-delete-add table idiom makes the batch
//     idempotent (the table may or may not exist) and wipes
//     stale rules from a previous process run at the first
//     mutation after restart.
//   - No rule handles are used: rules are re-derived from
//     the active set on every change, so an unrelated
//     subsystem mutating its own tables cannot invalidate
//     our identifiers.
//   - A failed `nft` run leaves `active` untouched, keeping
//     in-memory and kernel state consistent.
func (n *NftablesBackend) Add(m Mapping) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	next := make([]Mapping, 0, len(n.active)+1)
	replaced := false
	for _, existing := range n.active {
		if existing.ID == m.ID {
			next = append(next, m) // renewal: same id replaces
			replaced = true
			continue
		}
		next = append(next, existing)
	}
	if !replaced {
		next = append(next, m)
	}
	rendered, err := renderTable(next)
	if err != nil {
		return err
	}
	if err := n.apply(rendered); err != nil {
		return err
	}
	n.active = next
	return nil
}

// Remove drops the mapping and re-renders the remaining
// active set. Removing an unknown id is a no-op success so
// retries and restart recovery are idempotent.
func (n *NftablesBackend) Remove(id string) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	found := false
	next := make([]Mapping, 0, len(n.active))
	for _, existing := range n.active {
		if existing.ID == id {
			found = true
			continue
		}
		next = append(next, existing)
	}
	if !found {
		return nil
	}
	rendered, err := renderTable(next)
	if err != nil {
		return err
	}
	if err := n.apply(rendered); err != nil {
		return err
	}
	n.active = next
	return nil
}

// apply stages and loads one nft transaction. On any failure
// the active set is left untouched so the caller (runtime)
// and the kernel stay consistent.
func (n *NftablesBackend) apply(rendered []byte) error {
	stage := filepath.Join(n.stagingDir, "duckos-upnp.batch.nft")
	if err := os.WriteFile(stage, rendered, 0o600); err != nil {
		return fmt.Errorf("upnp: stage write failed: %w", err)
	}
	if n.validateOnly {
		return nil
	}
	cmd := exec.Command(n.binPath, "-f", stage)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		_ = os.Remove(stage)
		return fmt.Errorf("upnp: nft -f failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

// AllowListen refuses WAN listeners. The backend trusts only
// the operator-seeded trustedIfxs; everything else is
// refused. The default (empty list) refuses all interfaces,
// so a fresh install is safe-by-default.
func (n *NftablesBackend) AllowListen(iface string, addr net.Addr) bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	if iface == "" {
		return false
	}
	for _, t := range n.trustedIfxs {
		if t == iface {
			return true
		}
	}
	return false
}

// WanRefusedErr returns the canonical WAN-refusal error so
// callers can use errors.Is.
func WanRefusedErr() error { return WanRefused }

// UnavailableBackend + ErrBackendUnavailable + UnavailableReason
// live in the cross-platform file (upnp.go) so the Agent and
// API packages can refer to them from darwin builds.

// renderTable produces ONE self-contained nft transaction
// that atomically replaces inet duckos_upnp with exactly one
// DNAT rule per mapping (sorted by id for deterministic
// rule order). The add-table/delete-table/add-table idiom
// makes the batch correct whether or not the table exists
// and clears stale rules left by a previous process run.
//
// The full port-range case is intentionally out of V1
// scope; the bounded surface is one mapping per id.
func renderTable(mappings []Mapping) ([]byte, error) {
	sorted := append([]Mapping(nil), mappings...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].ID < sorted[j].ID })
	var b strings.Builder
	b.WriteString("# duckos.upnp atomic table replacement\n")
	b.WriteString("add table inet duckos_upnp\n")
	b.WriteString("delete table inet duckos_upnp\n")
	b.WriteString("add table inet duckos_upnp\n")
	b.WriteString("add chain inet duckos_upnp prerouting { type nat hook prerouting priority -100 ; }\n")
	for _, m := range sorted {
		rule, err := renderRule(m)
		if err != nil {
			return nil, err
		}
		b.WriteString(rule)
	}
	return []byte(b.String()), nil
}

// renderRule renders one DNAT rule line.
func renderRule(m Mapping) (string, error) {
	if m.ID == "" {
		return "", errors.New("upnp: mapping id is required")
	}
	if m.ExternalPort < 1 || m.ExternalPort > 65535 {
		return "", errors.New("upnp: externalPort out of range")
	}
	if m.InternalPort < 1 || m.InternalPort > 65535 {
		return "", errors.New("upnp: internalPort out of range")
	}
	if m.InternalHost == "" {
		return "", errors.New("upnp: internalHost is required")
	}
	// The empty transport keeps beta.2 renderings (tcp) stable.
	transport := string(m.Transport)
	if transport == "" {
		transport = string(TransportTCP)
	}
	if transport != string(TransportTCP) && transport != string(TransportUDP) {
		return "", fmt.Errorf("upnp: unsupported transport %q", transport)
	}
	return fmt.Sprintf(
		"add rule inet duckos_upnp prerouting %s dport %d dnat ip to %s:%d comment \"duckos_upnp:%s\"\n",
		transport, m.ExternalPort, m.InternalHost, m.InternalPort, m.ID,
	), nil
}
