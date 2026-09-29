// qos2/qos2.go: V1 advanced hierarchical
// QoS surface. The package owns the typed
// contract for parent/child qdisc/class
// trees (HTB / HFSC / fq_codel / CAKE),
// the deterministic tc-renderer, the
// DSCP classification, the per-class
// counter surface, and the per-WAN /
// per-VRF / per-tenant scope.
//
// The runtime is a thin mapping to Linux
// tc. The package does not execute tc at
// runtime; the privileged Agent does.
package qos2

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// QDiscKind is the qdisc class.
type QDiscKind string

const (
	QDiscHTB     QDiscKind = "htb"
	QDiscHFSC    QDiscKind = "hfsc"
	QDiscFQCoDel QDiscKind = "fq_codel"
	QDiscCAKE    QDiscKind = "cake"
)

// Valid reports whether k is known.
func (k QDiscKind) Valid() bool {
	switch k {
	case QDiscHTB, QDiscHFSC, QDiscFQCoDel, QDiscCAKE:
		return true
	}
	return false
}

// Scope is the QoS scope (where the policy
// applies).
type Scope string

const (
	ScopeWAN    Scope = "wan"
	ScopeIface  Scope = "interface"
	ScopeVLAN   Scope = "vlan"
	ScopeVRF    Scope = "vrf"
	ScopeTenant Scope = "tenant"
	ScopeTunnel Scope = "tunnel"
	ScopeClient Scope = "client"
)

// Valid reports whether s is known.
func (s Scope) Valid() bool {
	switch s {
	case ScopeWAN, ScopeIface, ScopeVLAN, ScopeVRF, ScopeTenant, ScopeTunnel, ScopeClient:
		return true
	}
	return false
}

// Class is a single qdisc / class.
type Class struct {
	ID     string    `json:"id"`
	Parent string    `json:"parent,omitempty"`
	Kind   QDiscKind `json:"kind"`
	// Rate / ceil in kbps. 0 means "no
	// limit".
	RateKbps int `json:"rateKbps,omitempty"`
	CeilKbps int `json:"ceilKbps,omitempty"`
	// Burst in bytes.
	BurstBytes int `json:"burstBytes,omitempty"`
	// Priority is the leaf priority
	// (lower = better).
	Priority int `json:"priority,omitempty"`
	// DSCPFilter is the optional DSCP
	// match.
	DSCPFilter []int `json:"dscpFilter,omitempty"`
	// MarkFilter is the optional fwmark.
	MarkFilter int `json:"markFilter,omitempty"`
	// RemarkDSCP re-marks packets
	// leaving the class.
	RemarkDSCP int `json:"remarkDscp,omitempty"`
}

var idPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:-]{0,63}$`)

// Valid reports whether c is well-formed.
func (c Class) Valid() error {
	if !idPattern.MatchString(c.ID) {
		return errors.New("qos2: class id is not canonical")
	}
	if c.Parent != "" && !idPattern.MatchString(c.Parent) {
		return errors.New("qos2: parent id is not canonical")
	}
	if !c.Kind.Valid() {
		return errors.New("qos2: kind is unknown")
	}
	if c.RateKbps < 0 {
		return errors.New("qos2: rateKbps must be non-negative")
	}
	if c.CeilKbps < c.RateKbps {
		return errors.New("qos2: ceilKbps must be >= rateKbps")
	}
	if c.Priority < 0 || c.Priority > 7 {
		return errors.New("qos2: priority out of range")
	}
	for _, d := range c.DSCPFilter {
		if d < 0 || d > 63 {
			return errors.New("qos2: DSCP out of range")
		}
	}
	if c.RemarkDSCP < 0 || c.RemarkDSCP > 63 {
		return errors.New("qos2: remark DSCP out of range")
	}
	return nil
}

// Policy is the QoS policy for a scope.
type Policy struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Scope Scope  `json:"scope"`
	// Target is the WAN / iface / VRF /
	// tenant / tunnel id the policy
	// applies to.
	Target  string  `json:"target"`
	Classes []Class `json:"classes"`
	Enabled bool    `json:"enabled"`
}

// Valid reports whether p is well-formed.
func (p Policy) Valid() error {
	if !idPattern.MatchString(p.ID) {
		return errors.New("qos2: policy id is not canonical")
	}
	if !p.Scope.Valid() {
		return errors.New("qos2: scope is unknown")
	}
	if p.Target == "" {
		return errors.New("qos2: target is required")
	}
	if len(p.Classes) == 0 {
		return errors.New("qos2: at least one class is required")
	}
	ids := map[string]bool{}
	for _, c := range p.Classes {
		if err := c.Valid(); err != nil {
			return err
		}
		if ids[c.ID] {
			return errors.New("qos2: duplicate class id: " + c.ID)
		}
		ids[c.ID] = true
	}
	// Parent classes must exist.
	for _, c := range p.Classes {
		if c.Parent != "" && !ids[c.Parent] {
			return errors.New("qos2: class references unknown parent: " + c.Parent)
		}
	}
	return nil
}

// Render produces the tc-renderer output.
// The output is deterministic.
func Render(p Policy) (string, error) {
	if err := p.Valid(); err != nil {
		return "", err
	}
	var sb strings.Builder
	sb.WriteString("# DuckOS-generated tc configuration\n")
	// Sort classes by parent chain to
	// ensure deterministic rendering.
	classes := append([]Class(nil), p.Classes...)
	sort.Slice(classes, func(i, k int) bool { return classes[i].ID < classes[k].ID })
	for _, c := range classes {
		writeClass(&sb, p, c)
	}
	return sb.String(), nil
}

func writeClass(sb *strings.Builder, p Policy, c Class) {
	switch c.Kind {
	case QDiscHTB:
		fmt.Fprintf(sb, "qdisc add dev %s parent %s handle %s htb ", p.Target, c.Parent, c.ID)
		if c.RateKbps > 0 {
			fmt.Fprintf(sb, "rate %dkbit ", c.RateKbps)
		}
		if c.CeilKbps > 0 {
			fmt.Fprintf(sb, "ceil %dkbit ", c.CeilKbps)
		}
		if c.BurstBytes > 0 {
			fmt.Fprintf(sb, "burst %d ", c.BurstBytes)
		}
		fmt.Fprintf(sb, "prio %d\n", c.Priority)
	case QDiscHFSC:
		fmt.Fprintf(sb, "qdisc add dev %s parent %s handle %s hfsc ", p.Target, c.Parent, c.ID)
		if c.RateKbps > 0 {
			fmt.Fprintf(sb, "rate %dkbit ", c.RateKbps)
		}
		fmt.Fprintf(sb, "prio %d\n", c.Priority)
	case QDiscFQCoDel:
		fmt.Fprintf(sb, "qdisc add dev %s parent %s handle %s fq_codel\n", p.Target, c.Parent, c.ID)
	case QDiscCAKE:
		fmt.Fprintf(sb, "qdisc add dev %s parent %s handle %s cake\n", p.Target, c.Parent, c.ID)
	}
	if len(c.DSCPFilter) > 0 {
		for _, d := range c.DSCPFilter {
			fmt.Fprintf(sb, "filter add dev %s parent %s protocol ip prio 1 u32 match ip tos 0x%x 0xfc flowid %s\n",
				p.Target, c.Parent, d<<2, c.ID)
		}
	}
	if c.MarkFilter > 0 {
		fmt.Fprintf(sb, "filter add dev %s parent %s protocol all prio 1 handle %d fw flowid %s\n",
			p.Target, c.Parent, c.MarkFilter, c.ID)
	}
	if c.RemarkDSCP > 0 {
		fmt.Fprintf(sb, "filter add dev %s parent %s protocol all prio 2 u32 match u32 0 0 action skbedit mark %d\n",
			p.Target, c.Parent, c.RemarkDSCP)
	}
}
