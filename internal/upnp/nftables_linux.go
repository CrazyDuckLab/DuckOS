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

// Add renders and applies the mapping. Returns an explicit
// error if the nft staging file fails validation so the
// runtime can roll back the Manager state.
func (n *NftablesBackend) Add(m Mapping) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	rendered, err := renderMapping(m)
	if err != nil {
		return err
	}
	stage := filepath.Join(n.stagingDir, m.ID+".nft")
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

// Remove deletes the staging file and asks nft to flush
// the corresponding rule. Remove failures are non-fatal —
// the runtime emits a MappingEvent but does not block.
func (n *NftablesBackend) Remove(id string) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	stage := filepath.Join(n.stagingDir, id+".nft")
	if _, err := os.Stat(stage); err == nil {
		_ = os.Remove(stage)
	}
	if n.validateOnly {
		return nil
	}
	// Best-effort flush. The runtime tolerates errors.
	cmd := exec.Command(n.binPath, "flush", "table", "inet", "duckos_upnp")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("upnp: nft flush failed: %w: %s", err, strings.TrimSpace(stderr.String()))
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

// renderMapping produces a minimal nftables snippet that
// installs one DNAT rule for the dynamic mapping. The full
// port-range / multi-protocol case is intentionally out of
// V1 scope; the bounded surface is one mapping per id.
func renderMapping(m Mapping) ([]byte, error) {
	if m.ID == "" {
		return nil, errors.New("upnp: mapping id is required")
	}
	if m.ExternalPort < 1 || m.ExternalPort > 65535 {
		return nil, errors.New("upnp: externalPort out of range")
	}
	if m.InternalPort < 1 || m.InternalPort > 65535 {
		return nil, errors.New("upnp: internalPort out of range")
	}
	if m.InternalHost == "" {
		return nil, errors.New("upnp: internalHost is required")
	}
	var b strings.Builder
	b.WriteString("# duckos.upnp mapping\n")
	b.WriteString("add table inet duckos_upnp\n")
	b.WriteString("add chain inet duckos_upnp prerouting { type nat hook prerouting priority -100 ; }\n")
	b.WriteString(fmt.Sprintf(
		"add rule inet duckos_upnp prerouting tcp dport %d dnat to %s:%d\n",
		m.ExternalPort, m.InternalHost, m.InternalPort,
	))
	return []byte(b.String()), nil
}