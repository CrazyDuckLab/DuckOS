// Package install defines the V1 installation
// role model. A role is the operator-facing
// answer to "what kind of box is this?". It is
// the basis for the first-run topology
// auto-detection, the management interface
// guard, and the dependency-aware deletion
// preflight. The role is durable: a Router
// stays a Router until the operator explicitly
// changes it through the API.
//
// Three roles are defined by V1:
//
//   - Router: a traditional router/gateway with
//     at least one WAN and one LAN. NAT, firewall
//     and policy routing are mandatory.
//   - Server: a single-interface appliance such
//     as a VPS, cloud VM, or hypervisor VM with
//     exactly one usable NIC. The NIC is
//     simultaneously uplink and management; no
//     LAN interface exists.
//   - Custom: a deployment the operator is
//     steering explicitly. DuckOS never
//     auto-detects Custom.
//
// The role is decided once during first-run and
// durably persisted. Re-detection is allowed
// only when the operator explicitly asks for
// it; an automated re-run that changes the role
// is treated as a Safe Apply event.
package install

import (
	"encoding/json"
	"errors"
	"regexp"
	"time"
)

// Role is the canonical installation role.
type Role string

const (
	// RoleUnset is the zero value; the Agent
	// refuses every operation that depends on
	// the role until it is decided.
	RoleUnset Role = ""
	// RoleRouter is the gateway/router role.
	RoleRouter Role = "router"
	// RoleServer is the single-interface
	// appliance role.
	RoleServer Role = "server"
	// RoleCustom is the operator-steerable role.
	RoleCustom Role = "custom"
)

// Valid reports whether r is a known role.
func (r Role) Valid() bool {
	switch r {
	case RoleRouter, RoleServer, RoleCustom:
		return true
	}
	return false
}

// Decision is a durable record of when and how
// the role was decided. The Detector that
// produced the decision is recorded so an
// operator can audit the auto-detection.
type Decision struct {
	Role       Role      `json:"role"`
	DecidedAt  time.Time `json:"decidedAt"`
	DecidedBy  string    `json:"decidedBy"`
	DetectorID string    `json:"detectorId,omitempty"`
	Reason     string    `json:"reason,omitempty"`
	// NetworkSnapshot records the interfaces
	// observed at decision time. The Agent
	// refuses a re-detection that would change
	// the role without an explicit override.
	NetworkSnapshot NetworkSnapshot `json:"networkSnapshot"`
}

// NetworkSnapshot is the minimal evidence
// captured at decision time.
type NetworkSnapshot struct {
	Interfaces []Interface `json:"interfaces"`
	Management string      `json:"management"`
}

// Interface describes a single NIC observed
// at decision time. The Agent never reads the
// kernel directly; the snapshot is built from
// the topology graph.
type Interface struct {
	Name     string `json:"name"`
	Address  string `json:"address,omitempty"`
	Up       bool   `json:"up"`
	Kind     string `json:"kind,omitempty"`
	Role     string `json:"role,omitempty"`
	Selected bool   `json:"selected,omitempty"`
}

var namePattern = regexp.MustCompile(`^[a-z][a-z0-9._-]{0,15}$`)

// Validate returns an error if the Decision
// is malformed. Decisions are persisted through
// the State store; invalid Decisions never
// reach the durable snapshot.
func (d Decision) Validate() error {
	if !d.Role.Valid() {
		return errors.New("install: role is unset or unknown")
	}
	if d.DecidedAt.IsZero() {
		return errors.New("install: decidedAt is required")
	}
	if d.DecidedBy == "" {
		return errors.New("install: decidedBy is required")
	}
	for _, i := range d.NetworkSnapshot.Interfaces {
		if !namePattern.MatchString(i.Name) {
			return errors.New("install: interface name is not canonical")
		}
	}
	if d.NetworkSnapshot.Management != "" && !namePattern.MatchString(d.NetworkSnapshot.Management) {
		return errors.New("install: management interface is not canonical")
	}
	return nil
}

// JSON encodes d for durable storage. The
// inverse of JSON is Go's stock json.Unmarshal.
func (d Decision) JSON() ([]byte, error) {
	if err := d.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(d)
}

// DecodeDecision parses a JSON-encoded
// Decision. The same validation rules apply.
func DecodeDecision(raw []byte) (Decision, error) {
	var d Decision
	if err := json.Unmarshal(raw, &d); err != nil {
		return d, err
	}
	if err := d.Validate(); err != nil {
		return d, err
	}
	return d, nil
}
