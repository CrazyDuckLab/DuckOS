// upnp/trusted_lan_registry.go: B-008 (BETA.2) UPnP trusted-
// LAN model.
//
// The BETA.2 product model for UPnP / NAT-PMP / PCP trust
// is intent-driven. The user selects a DuckOS network
// object (e.g. network.lan.inside) and asks DuckOS to trust
// it; the runtime scans the live enrollment, resolves the
// stable object ID to the underlying Linux interface, and
// admits only that interface to the bounded mapping
// surface. WAN, management, and unknown objects are
// structurally rejected — the runtime does not accept a
// raw kernel iface name from a public request.
//
// This file owns:
//   - the persistent JSON store (/var/lib/duckos/upnp/
//     trusted-networks.json);
//   - the per-ref resolution against the live enrollment;
//   - the atomic write / load contract so a half-written
//     file cannot surface partial trust;
//   - the predicate NetworkBackend.Ready takes to gate
//   AllowListen.
//
// Default OFF: an empty registry admits no interface.
// An attempt to EnableNetwork on a non-trusted ref
// returns ErrNotTrusted at the runtime layer before any
// backend work is done.

package upnp

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// TrustedLANState is the on-disk shape of the registry.
// The file is keyed by stable DuckOS object IDs (matching
// network.Interface.ID). The Kernel/Linux
// interface name is NEVER stored: a kernel rename must
// not invalidate trust, and an operator must not be
// able to spoof a kernel name through the persistent
// store.
type TrustedLANState struct {
	// Version is the schema version of the file. The
	// loader accepts v1 only; future bumps must
	// migrate, never silently accept an older shape.
	Version int `json:"version"`
	// Networks is the canonical list of trusted DuckOS
	// object IDs. Sorted on the writer side so a
	// round-trip produces a deterministic byte stream.
	Networks []string `json:"networks"`
}

// EnrollmentSnapshot is the struct of the enrollment
// resolver the registry consumes. The production
// wiring passes the network.Config (parsed from the
// /var/lib/duckos/<host>/network.json enrollment file).
// We accept a struct so the registry stays free of the
// network.Config import cycle (upnp is a leaf package).
type EnrollmentSnapshot struct {
	Interfaces []EnrollmentInterface `json:"interfaces"`
}

// EnrollmentInterface is the bounded view the registry
// needs from each Interface in the enrollment. Only
// fields relevant to the trust decision are exposed so a
// caller cannot smuggle unrelated metadata through the
// resolver.
type EnrollmentInterface struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Role       string `json:"role"`
	Management bool   `json:"management"`
}

// ErrNotTrusted is returned when a network ref is not in
// the registry. Callers must surface this as
// "upnp_network_not_trusted" so the operator-facing UI
// can render an actionable message.
var ErrNotTrusted = errors.New("upnp: network is not in the trusted LAN registry")

// ErrWANRejected is returned when a network ref resolves
// to a WAN interface. The error is structurally distinct
// from ErrNotTrusted so the audit trail can distinguish
// an accidental omission (operator never trusted) from a
// structural block (operator tried to trust a WAN — a
// configuration error).
var ErrWANRejected = errors.New("upnp: WAN networks cannot be trusted for UPnP")

// ErrMgmtRejected is the same structural block for the
// management interface. The management interface is the
// DuckOS control plane; exposing it to UPnP would mean
// any device on that LAN can mutate /var/lib/duckos via
// PCP / NAT-PMP / UPnP-IGD.
var ErrMgmtRejected = errors.New("upnp: the management interface cannot be trusted for UPnP")

// TrustedLANRegistry owns the persistent trust list and
// resolves refs to kernel names against the supplied
// EnrollmentSnapshot. Constructed once at Agent startup;
// mutations are goroutine-safe.
type TrustedLANRegistry struct {
	mu    sync.RWMutex
	state TrustedLANState
	path  string
	now   func() time.Time
}

// NewTrustedLANRegistry constructs an empty registry
// bound to a path on disk. The file is created lazily on
// the first Save so a freshly-installed Agent does not
// produce an empty JSON file at boot. now is the
// timestamp source (production: time.Now).
func NewTrustedLANRegistry(path string, now func() time.Time) *TrustedLANRegistry {
	if now == nil {
		now = time.Now
	}
	return &TrustedLANRegistry{state: TrustedLANState{Version: 1, Networks: []string{}}, path: path, now: now}
}

// LoadTrustedLANRegistry loads the persistent trust file
// from path. A missing file yields an empty registry
// (default OFF) without an error. A malformed file
// returns a typed error so the Agent can refuse to
// start rather than silently dropping trust.
func LoadTrustedLANRegistry(path string, now func() time.Time) (*TrustedLANRegistry, error) {
	r := NewTrustedLANRegistry(path, now)
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return r, nil
		}
		return nil, fmt.Errorf("upnp: cannot read trust file: %w", err)
	}
	var s TrustedLANState
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("upnp: trust file is malformed: %w", err)
	}
	if s.Version != 1 {
		return nil, fmt.Errorf("upnp: trust file is unsupported version %d", s.Version)
	}
	s.Networks = canonicaliseStrings(s.Networks)
	r.state = s
	return r, nil
}

// Trust adds ref to the registry after resolving it
// against enrollment. The trust is NOT applied if the
// ref is unknown, WAN, or management — those are
// structural blocks surfaced as typed errors. Save
// persists the registry atomically on success.
func (r *TrustedLANRegistry) Trust(ref string, enrollment EnrollmentSnapshot) error {
	if ref == "" {
		return errors.New("upnp: ref is required")
	}
	if err := validateRef(ref); err != nil {
		return err
	}
	if err := assertLANRole(ref, enrollment); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, n := range r.state.Networks {
		if n == ref {
			return r.saveLocked()
		}
	}
	r.state.Networks = append(r.state.Networks, ref)
	r.state.Networks = canonicaliseStrings(r.state.Networks)
	return r.saveLocked()
}

// Revoke removes ref from the registry. Removing a
// ref the runtime has not been asked to trust is a
// no-op (so the operator can re-issue a revoke without
// first checking current state).
func (r *TrustedLANRegistry) Revoke(ref string) error {
	if ref == "" {
		return errors.New("upnp: ref is required")
	}
	if err := validateRef(ref); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, n := range r.state.Networks {
		if n == ref {
			r.state.Networks = append(r.state.Networks[:i], r.state.Networks[i+1:]...)
			return r.saveLocked()
		}
	}
	return nil
}

// IsTrusted reports whether ref is in the trust list
// AND resolves to a non-WAN, non-management
// enrollment entry. The enrollment is consulted at every
// call so a rename / rebind that changes the role is
// reflected immediately.
func (r *TrustedLANRegistry) IsTrusted(ref string, enrollment EnrollmentSnapshot) bool {
	if ref == "" {
		return false
	}
	r.mu.RLock()
	ok := false
	for _, n := range r.state.Networks {
		if n == ref {
			ok = true
			break
		}
	}
	r.mu.RUnlock()
	if !ok {
		return false
	}
	// The trust list itself is the canonical source;
	// the structural rejection (WAN / management) is
	// enforced at Trust time so a stale entry that
	// later becomes WAN is rejected at the next call
	// to IsTrusted for that ref. This is the
	// "rename/rebind handled through stable object
	// identity" guarantee the BETA.2 review asked for.
	if err := assertLANRole(ref, enrollment); err != nil {
		return false
	}
	return true
}

// Trusted returns the snapshot of the trust list in
// canonical order. Used by the public API state surface.
func (r *TrustedLANRegistry) Trusted() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, len(r.state.Networks))
	copy(out, r.state.Networks)
	return out
}

// ResolvedInterfaces maps each trusted ref to the
// current Linux interface name from enrollment. A ref
// whose enrollment entry has gone (interface was deleted
// from the live enrollment) is omitted; the operator
// sees it as "missing" in the public surface and can
// revoke it.
func (r *TrustedLANRegistry) ResolvedInterfaces(enrollment EnrollmentSnapshot) map[string]string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[string]string, len(r.state.Networks))
	for _, ref := range r.state.Networks {
		if err := assertLANRole(ref, enrollment); err != nil {
			continue
		}
		for _, i := range enrollment.Interfaces {
			if i.ID == ref {
				out[ref] = i.Name
				break
			}
		}
	}
	return out
}

// AssertLANRoleOrRevoke is the boot-restore helper. It
// re-runs assertLANRole against the supplied enrollment
// for a ref that is already in the trust list. If the
// role check fails (the ref was renamed / reclassified
// since the last Trust), the ref is removed from the
// trust list so a stale WAN / management entry does not
// survive a reboot into the trusted set. Returns nil if
// the ref still passes the role check; returns the
// assertion error (after Revoke) so the caller can log
// the structural flip.
func (r *TrustedLANRegistry) AssertLANRoleOrRevoke(ref string, enrollment EnrollmentSnapshot) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := assertLANRole(ref, enrollment); err != nil {
		for i, n := range r.state.Networks {
			if n == ref {
				r.state.Networks = append(r.state.Networks[:i], r.state.Networks[i+1:]...)
				_ = r.saveLocked()
				return err
			}
		}
		return err
	}
	return nil
}

// saveLocked persists the trust file atomically. The
// caller MUST hold r.mu (write lock).
func (r *TrustedLANRegistry) saveLocked() error {
	if r.path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(r.path), 0o700); err != nil {
		return fmt.Errorf("upnp: cannot create trust dir: %w", err)
	}
	b, err := json.MarshalIndent(r.state, "", "  ")
	if err != nil {
		return fmt.Errorf("upnp: cannot encode trust file: %w", err)
	}
	tmp := r.path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o600); err != nil {
		return fmt.Errorf("upnp: cannot stage trust file: %w", err)
	}
	if err := os.Rename(tmp, r.path); err != nil {
		return fmt.Errorf("upnp: cannot commit trust file: %w", err)
	}
	return nil
}

// canonicaliseStrings returns a sorted, deduplicated
// copy of in. Empty entries are dropped. The order is
// canonical so two operators with the same trust list
// produce byte-identical files.
func canonicaliseStrings(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

// validateRef ensures the ref is a canonical DuckOS
// object ID before it reaches the resolver. The DuckOS
// idPattern is shared with the identity package but is
// duplicated here (not imported) because the upnp
// package must remain a leaf: importing internal/identity
// would create a cycle (identity -> nothing, but upnp
// already imports many things, and the cross-package
// contract is documented at the boundary).
func validateRef(ref string) error {
	if ref == "" {
		return errors.New("upnp: ref is required")
	}
	if !idPattern.MatchString(ref) {
		return errors.New("upnp: ref is not a canonical DuckOS object id")
	}
	return nil
}

// assertLANRole enforces the structural blocks. It is
// the single point that rejects WAN / management entries
// so Trust, IsTrusted and ResolvedInterfaces all share
// the same predicate.
func assertLANRole(ref string, enrollment EnrollmentSnapshot) error {
	for _, i := range enrollment.Interfaces {
		if i.ID != ref {
			continue
		}
		if i.Management {
			return ErrMgmtRejected
		}
		if i.Role == "wan" {
			return ErrWANRejected
		}
		// VLANs, bridges, and physicals with role "lan"
		// are all admitted; nothing else exists today but
		// future kinds must opt-in here.
		if i.Role != "lan" {
			return fmt.Errorf("upnp: role %q cannot be trusted for LPnP", i.Role)
		}
		return nil
	}
	return ErrNotTrusted
}
