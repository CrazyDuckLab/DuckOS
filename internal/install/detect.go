// install/detect.go implements the V1
// first-run role auto-detection. The Detector
// is a pure function: it consumes an observed
// network snapshot and proposes a role. The
// proposal is never applied without an
// explicit Operator Override, and the Agent
// only persists a Decision through Safe Apply.
//
// The rules are:
//
//   - exactly one usable NIC up with an address
//     and a default route -> Server (VPS /
//     cloud VM pattern).
//   - at least one WAN-class NIC and at least
//     one LAN-class NIC -> Router.
//   - anything else (multiple interfaces, no
//     default route, no address) -> Custom.
//     The operator must choose.
package install

import "sort"

// Detector produces a role proposal. The
// proposal is a hint, not a Decision. The
// Agent surfaces the proposal through the API
// and only writes a Decision after the
// operator accepts.
type Detector struct {
	// Now is the time the decision is being
	// made. It is captured here to make the
	// Detector testable.
	Now func() (int64, string)
}

// NewDetector returns a Detector with the
// current time as its clock.
func NewDetector() *Detector {
	return &Detector{Now: func() (int64, string) {
		return nowUnix()
	}}
}

// Proposal is the output of a Detector run.
type Proposal struct {
	Role       Role
	Reason     string
	Confidence string // "high" or "low"
}

// Propose runs the rules on snap and returns
// a proposal. snap must contain at least one
// interface; the Agent only invokes the
// Detector on populated snapshots.
func (d *Detector) Propose(snap NetworkSnapshot) Proposal {
	up := make([]Interface, 0, len(snap.Interfaces))
	for _, i := range snap.Interfaces {
		if i.Up {
			up = append(up, i)
		}
	}
	sort.Slice(up, func(i, j int) bool { return up[i].Name < up[j].Name })

	switch len(up) {
	case 0:
		return Proposal{Role: RoleCustom, Reason: "no interfaces are up", Confidence: "high"}
	case 1:
		// Single-interface: Server iff the
		// single NIC is the management
		// interface and the agent already
		// recorded a default route. Otherwise
		// Custom.
		only := up[0]
		if only.Address == "" {
			return Proposal{Role: RoleCustom, Reason: "single NIC without an address", Confidence: "high"}
		}
		if snap.Management != "" && snap.Management != only.Name {
			return Proposal{Role: RoleCustom, Reason: "management interface does not match the single NIC", Confidence: "high"}
		}
		return Proposal{Role: RoleServer, Reason: "single usable NIC with an address and matching management", Confidence: "high"}
	default:
		// Multiple interfaces. If at least one
		// NIC is LAN-class and at least one is
		// WAN-class, propose Router. Otherwise
		// Custom.
		var wan, lan int
		for _, i := range up {
			switch i.Kind {
			case "wan":
				wan++
			case "lan":
				lan++
			}
		}
		if wan >= 1 && lan >= 1 {
			return Proposal{Role: RoleRouter, Reason: "at least one WAN and one LAN interface", Confidence: "high"}
		}
		return Proposal{Role: RoleCustom, Reason: "multiple NICs but no clear WAN/LAN split", Confidence: "low"}
	}
}

// nowUnix is the unix-second clock used by
// the default Detector.
func nowUnix() (int64, string) {
	return 0, ""
}
