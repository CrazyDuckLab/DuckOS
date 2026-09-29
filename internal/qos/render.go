// render.go owns the deterministic conversion from a
// Topology to a Plan. The Plan is the contract the Linux
// adapter executes. The renderer is the only place that
// knows tc's textual syntax; the model layer is the only
// place that knows the canonical DuckOS object model.
//
// A Plan is a list of Steps. Each Step has a kind
// (qdisc/class/filter), a target (the linux interface
// name), an ordered list of arguments and a textual
// comment the GUI/CLI can show as a preview. The Plan is
// sorted so a serialised form is byte-for-byte stable
// across runs.
//
// The renderer refuses to emit tc invocations for an
// invalid topology; callers must Validate first.

package qos

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// StepKind enumerates the tc verbs the plan can emit.
// The set is closed: anything new must be added here so
// the linux adapter has a complete switch.
type StepKind string

const (
	StepQdisc  StepKind = "qdisc"
	StepClass  StepKind = "class"
	StepFilter StepKind = "filter"
)

// Step is a single tc invocation. Args is the ordered
// list of arguments that follow the verb; the linux
// adapter joins them with single spaces. Comment is a
// human-readable description of what the step does.
type Step struct {
	Kind    StepKind `json:"kind"`
	Link    string   `json:"link"`
	Parent  string   `json:"parent,omitempty"`
	Handle  string   `json:"handle,omitempty"`
	Args    []string `json:"args"`
	Comment string   `json:"comment,omitempty"`
}

// Plan is the deterministic tc plan the linux adapter
// executes. The total number of steps is bounded to keep
// the preview small.
type Plan struct {
	SchemaVersion string `json:"schemaVersion"`
	Steps         []Step `json:"steps"`
	// Warnings are non-fatal notes the operator should
	// see before applying. A Plan with warnings is still
	// runnable; the warnings do not change the steps.
	Warnings []string `json:"warnings,omitempty"`
	// Generated is the unix timestamp of the render.
	Generated int64 `json:"generated"`
}

// MaxSteps is the hard cap on the number of steps. A
// topology of 16 shapers with 8 classes and 32 filters
// each yields 16*41 = 656 steps; the cap gives a wide
// margin.
const MaxSteps = 4096

// Render produces a deterministic Plan from a Topology.
// The returned Plan is sorted by (link, kind, parent,
// handle) so two identical topologies produce identical
// byte output.
func Render(t Topology) (Plan, error) {
	if err := t.Valid(); err != nil {
		return Plan{}, err
	}
	shaperByID := map[string]Shaper{}
	for _, s := range t.Shapers {
		shaperByID[s.ID] = s
	}
	classByID := map[string]Class{}
	for _, c := range t.Classes {
		classByID[c.ID] = c
	}
	steps := make([]Step, 0, len(t.Shapers)+len(t.Classes)+len(t.Filters))
	warnings := []string{}
	for _, s := range t.Shapers {
		step, warn := renderShaperQdisc(s)
		if warn != "" {
			warnings = append(warnings, warn)
		}
		steps = append(steps, step)
		// Classes are bound to a shaper; render them as
		// children of the root. Each class becomes a
		// filter target on ingress.
		for _, c := range t.Classes {
			if c.ShaperID != s.ID {
				continue
			}
			cstep := renderClass(s, c)
			steps = append(steps, cstep)
		}
	}
	for _, f := range t.Filters {
		sh := shaperByID[f.ShaperID]
		cl := classByID[f.ClassID]
		steps = append(steps, renderFilter(sh, cl, f))
	}
	sort.SliceStable(steps, func(i, j int) bool {
		if steps[i].Link != steps[j].Link {
			return steps[i].Link < steps[j].Link
		}
		if steps[i].Kind != steps[j].Kind {
			return steps[i].Kind < steps[j].Kind
		}
		if steps[i].Parent != steps[j].Parent {
			return steps[i].Parent < steps[j].Parent
		}
		return steps[i].Handle < steps[j].Handle
	})
	if len(steps) > MaxSteps {
		return Plan{}, fmt.Errorf("qos: rendered plan has %d steps, exceeds %d", len(steps), MaxSteps)
	}
	plan := Plan{SchemaVersion: SchemaVersion, Steps: steps, Warnings: warnings, Generated: t.GeneratedAt}
	if err := plan.Valid(); err != nil {
		return Plan{}, err
	}
	return plan, nil
}

// renderShaperQdisc renders the root qdisc step. CAKE
// and fq_codel have different argument sets. The
// arguments are tc-style: keyword followed by a
// space and the value (e.g. "bandwidth 10000kbit",
// not "bandwidth=10000kbit"). The Linux tc(8)
// toolchain accepts both styles for some keywords
// but CAKE and fq_codel expect the spaced form.
// CAKE additionally requires the diffserv mode
// (besteffort, diffserv8, ...) to appear BEFORE
// the bandwidth keyword; we honour that order.
func renderShaperQdisc(s Shaper) (Step, string) {
	args := []string{}
	warn := ""
	switch s.Algorithm {
	case AlgorithmCAKE:
		args = append(args, "cake")
		args = append(args, cakeDiffserv(s.Priority))
		args = append(args, "bandwidth", fmt.Sprintf("%dkbit", s.RateKbps))
		if s.Overhead > 0 {
			args = append(args, "overhead", fmt.Sprintf("%d", s.Overhead))
		}
		if s.MPU > 0 {
			args = append(args, "mpu", fmt.Sprintf("%d", s.MPU))
		}
	case AlgorithmFQCoDel:
		if s.Direction == DirectionIngress {
			// Ingress policing on linux: fq_codel
			// applied as an ingress qdisc. fq_codel
			// does not accept a rate argument; rate
			// shaping on ingress is best-effort. The
			// Shaper's RateKbps is reported in the
			// comment for the operator.
			args = append(args, "fq_codel", "limit", "1000", "flows", "1024")
			warn = fmt.Sprintf("qos: ingress shaping on %s is best-effort; fq_codel does not accept rate; kernel policing is not a substitute for upstream shaper", s.Link)
		} else {
			// Egress fq_codel: no rate at the qdisc
			// level. Rate shaping requires a tbf or
			// htb parent which the foundation
			// explicitly out of scope. The
			// Shaper's RateKbps is reported in the
			// comment for the operator.
			args = append(args, "fq_codel", "limit", "10000", "flows", "1024")
		}
	}
	// The root qdisc must use a major:minor handle
	// in the form "1:0" or "1:"; "root" is a valid
	// parent but not a valid handle. The renderer
	// emits "1:" which tc accepts and treats as
	// the root qdisc handle.
	handle := "1:"
	comment := fmt.Sprintf("%s %s shaper on %s at %dkbit", s.Algorithm, s.Direction, s.Link, s.RateKbps)
	return Step{Kind: StepQdisc, Link: s.Link, Parent: "root", Handle: handle, Args: args, Comment: comment}, warn
}

// renderClass renders a CAKE sub-queue or fq_codel
// filter target. The class becomes a leaf of the
// shaper's root; a Mark-based filter then routes
// traffic to it.
func renderClass(s Shaper, c Class) Step {
	args := []string{}
	switch s.Algorithm {
	case AlgorithmCAKE:
		// CAKE exposes "diffserv" sub-queues, not
		// user-defined classes. A Class becomes a
		// iptables/nftables mark to one of the
		// built-in tiers. The renderer emits a CAKE
		// filter on the chosen mark.
		args = append(args, "cake")
		args = append(args, "diffserv"+string(s.Priority))
	case AlgorithmFQCoDel:
		// fq_codel has no sub-queues. A Class becomes a
		// hash on a fwmark. The renderer emits a
		// filter on the fwmark.
		args = append(args, "fq_codel")
	}
	handle := fmt.Sprintf("1:%d", hashHandle(c.ID))
	comment := fmt.Sprintf("class %s on shaper %s", c.Name, s.ID)
	return Step{Kind: StepClass, Link: s.Link, Parent: "root", Handle: handle, Args: args, Comment: comment}
}

// renderFilter renders the tc filter that maps a fwmark
// to a class. The filter is the bridge between the
// firewall's mark work and the queueing discipline.
func renderFilter(s Shaper, c Class, f Filter) Step {
	proto := f.Protocol
	if proto == "" {
		proto = "ip"
	}
	args := []string{"protocol", proto, "fwmark", fmt.Sprintf("0x%x/0xff", f.Mark), "flowid", fmt.Sprintf("1:%d", hashHandle(c.ID))}
	comment := fmt.Sprintf("filter mark=0x%x -> class %s (shaper %s, %s %s)", f.Mark, c.Name, s.ID, s.Algorithm, s.Direction)
	return Step{Kind: StepFilter, Link: s.Link, Parent: "", Handle: "", Args: args, Comment: comment}
}

// hashHandle turns a canonical id into a stable 16-bit
// handle the linux kernel accepts. The hash is not
// cryptographic; it just spreads classes across the
// kernel's id space deterministically.
func hashHandle(id string) int {
	var h uint32 = 2166136261
	for i := 0; i < len(id); i++ {
		h ^= uint32(id[i])
		h *= 16777619
	}
	return int(h & 0xffff)
}

// Valid enforces the plan contract. The renderer calls
// Valid before returning; callers can re-validate on
// the wire.
func (p Plan) Valid() error {
	if p.SchemaVersion != SchemaVersion {
		return errors.New("qos: plan schema version mismatch")
	}
	if len(p.Steps) > MaxSteps {
		return errors.New("qos: plan exceeds max steps")
	}
	for i, s := range p.Steps {
		if !linkPattern.MatchString(s.Link) {
			return fmt.Errorf("qos: plan step %d has invalid link", i)
		}
		switch s.Kind {
		case StepQdisc, StepClass, StepFilter:
		default:
			return fmt.Errorf("qos: plan step %d has unknown kind", i)
		}
		for _, a := range s.Args {
			if a == "" {
				return fmt.Errorf("qos: plan step %d has empty argument", i)
			}
			if strings.ContainsAny(a, " \t\n\r\"';\\") {
				return fmt.Errorf("qos: plan step %d argument contains shell metacharacter", i)
			}
		}
	}
	return nil
}

// Encode serialises a Plan using the stable encoder.
// The output is bounded by MaxPlanBytes.
func (p Plan) Encode() ([]byte, error) {
	if err := p.Valid(); err != nil {
		return nil, err
	}
	data, err := jsonMarshalStable(p)
	if err != nil {
		return nil, err
	}
	if len(data) > MaxPlanBytes {
		return nil, errors.New("qos: plan exceeds maximum serialised size")
	}
	return data, nil
}

// PlanJSON is a thin wrapper over Encode that
// returns the encoded Plan as []byte. Tests and the
// Agent IPC layer use it to ship a Plan over the
// wire; production callers go through Render +
// Encode.
func PlanJSON(p Plan) ([]byte, error) { return p.Encode() }

// ParsePlan decodes a JSON Plan payload. The
// decoder refuses unknown fields so a malformed
// document is rejected before it reaches the
// runner.
func ParsePlan(data []byte) (Plan, error) {
	var p Plan
	dec := jsonNewDecoder(data)
	if err := dec.Decode(&p); err != nil {
		return Plan{}, err
	}
	if err := p.Valid(); err != nil {
		return Plan{}, err
	}
	return p, nil
}

// ToText renders the plan as a deterministic, human-
// readable text. The text is what the operator pastes
// in a support ticket or pastes into a manual tc
// session for verification. It is never executed by the
// agent.
func (p Plan) ToText() string {
	var b strings.Builder
	for i, s := range p.Steps {
		fmt.Fprintf(&b, "# step %d: %s\n", i, s.Comment)
		fmt.Fprintf(&b, "tc %s add dev %s", s.Kind, s.Link)
		if s.Parent != "" {
			fmt.Fprintf(&b, " parent %s", s.Parent)
		}
		if s.Handle != "" {
			fmt.Fprintf(&b, " handle %s", s.Handle)
		}
		for _, a := range s.Args {
			fmt.Fprintf(&b, " %s", a)
		}
		b.WriteString("\n")
	}
	return b.String()
}

// cakeDiffserv translates the DuckOS Priority
// enum to the corresponding CAKE diffserv keyword.
// CAKE accepts only besteffort, diffserv8,
// diffserv4 or diffserv3; the DuckOS enum is
// more granular so the renderer does the
// translation. The mapping is conservative:
// voice maps to diffserv8 (8 classes), video
// to diffserv4 (4 classes), network to
// diffserv3 (3 classes) and the rest to
// besteffort.
func cakeDiffserv(p Priority) string {
	switch p {
	case PriorityVoice:
		return "diffserv8"
	case PriorityVideo:
		return "diffserv4"
	case PriorityNetwork:
		return "diffserv3"
	case PriorityBestEffort, PriorityBulk, "":
		return "besteffort"
	}
	return "besteffort"
}
