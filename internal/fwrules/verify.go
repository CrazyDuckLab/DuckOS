// verify.go: exact semantic verification of the kernel readback
// of the owned inet duckos_fw table against the expected rule
// set. Verification compares full semantic state — table
// ownership, chain existence/policy/hook/priority, per-rule
// action, protocol, CIDRs, interfaces, ports, enablement,
// ordering and ownership identifiers — never object counts.
package fwrules

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// observedChain is the semantic subset of an nft chain object
// this module verifies.
type observedChain struct {
	Family string `json:"family"`
	Table  string `json:"table"`
	Name   string `json:"name"`
	Type   string `json:"type"`
	Hook   string `json:"hook"`
	Prio   *int   `json:"prio"`
	Policy string `json:"policy"`
}

// observedRule is the semantic subset of an nft rule object:
// one match tuple per meaningful expression plus the verdict
// and the ownership comment.
type observedRule struct {
	Family string `json:"family"`
	Table  string `json:"table"`
	Chain  string `json:"chain"`

	srcPrefix string
	dstPrefix string
	inIface   string
	outIface  string
	protocol  string // l4 protocol when matched explicitly
	dstPort   int
	action    string
	comment   string
}

// VerifySnapshot compares an `nft -j list table inet duckos_fw`
// readback against the expected enabled rule set. It fails on:
// missing or extra chain, wrong policy/hook/priority, missing,
// extra, reordered, or semantically diverging rules, and any
// object that does not belong to the owned table.
func VerifySnapshot(rules []Rule, objects []json.RawMessage) error {
	enabled := EnabledRules(rules)
	var (
		haveTable bool
		chains    []*observedChain
		rulesOut  []*observedRule
	)
	for _, raw := range objects {
		var object map[string]json.RawMessage
		if err := json.Unmarshal(raw, &object); err != nil || len(object) != 1 {
			return fmt.Errorf("fwrules: verify: invalid observed object")
		}
		switch kind, body := onlyKey(object); kind {
		case "table":
			var t struct {
				Family string `json:"family"`
				Name   string `json:"name"`
			}
			if err := json.Unmarshal(body, &t); err != nil {
				return fmt.Errorf("fwrules: verify: invalid observed table")
			}
			if t.Family != "inet" || t.Name != TableName {
				return fmt.Errorf("fwrules: verify: observed object outside owned table")
			}
			haveTable = true
		case "chain":
			var c observedChain
			if err := json.Unmarshal(body, &c); err != nil {
				return fmt.Errorf("fwrules: verify: invalid observed chain")
			}
			if c.Family != "inet" || c.Table != TableName {
				return fmt.Errorf("fwrules: verify: observed chain outside owned table")
			}
			chains = append(chains, &c)
		case "rule":
			r, err := decodeObservedRule(body)
			if err != nil {
				return err
			}
			rulesOut = append(rulesOut, r)
		default:
			return fmt.Errorf("fwrules: verify: unsupported observed object %q", kind)
		}
	}
	if !haveTable {
		return fmt.Errorf("fwrules: verify: owned table missing from readback")
	}
	if len(enabled) == 0 {
		// Canonical empty state: Render removes the whole table,
		// so any surviving owned object is a divergence.
		return fmt.Errorf("fwrules: verify: owned objects present after empty apply (%d objects)", len(objects))
	}
	if len(chains) != 1 {
		return fmt.Errorf("fwrules: verify: expected exactly one owned chain, observed %d", len(chains))
	}
	c := chains[0]
	if c.Name != "forward" || c.Type != "filter" || c.Hook != "forward" || c.Policy != "accept" || c.Prio == nil || *c.Prio != 0 {
		return fmt.Errorf("fwrules: verify: owned forward chain properties diverge from the rendered contract")
	}
	if len(rulesOut) != len(enabled) {
		return fmt.Errorf("fwrules: verify: expected %d kernel rules, observed %d", len(enabled), len(rulesOut))
	}
	for i, want := range enabled {
		if err := ruleMatches(want, rulesOut[i]); err != nil {
			return fmt.Errorf("fwrules: verify: rule %d: %w", i+1, err)
		}
	}
	return nil
}

func ruleMatches(want Rule, got *observedRule) error {
	if got.Family != "inet" || got.Table != TableName || got.Chain != "forward" {
		return fmt.Errorf("rule outside owned table/chain")
	}
	if got.srcPrefix != want.SrcPrefix {
		return fmt.Errorf("srcPrefix %q != expected %q", got.srcPrefix, want.SrcPrefix)
	}
	if got.dstPrefix != want.DstPrefix {
		return fmt.Errorf("dstPrefix %q != expected %q", got.dstPrefix, want.DstPrefix)
	}
	if got.inIface != want.InIface {
		return fmt.Errorf("inIface %q != expected %q", got.inIface, want.InIface)
	}
	if got.outIface != want.OutIface {
		return fmt.Errorf("outIface %q != expected %q", got.outIface, want.OutIface)
	}
	// A dport match implies its protocol; a protocol-only rule
	// matches meta l4proto. "any" rules render no protocol
	// expression at all.
	proto := got.protocol
	if proto == "" {
		proto = "any"
	}
	if proto != want.Protocol {
		return fmt.Errorf("protocol %q != expected %q", proto, want.Protocol)
	}
	if got.dstPort != want.DstPort {
		return fmt.Errorf("dstPort %d != expected %d", got.dstPort, want.DstPort)
	}
	if got.action != want.Action {
		return fmt.Errorf("action %q != expected %q", got.action, want.Action)
	}
	expectedComment := want.Comment
	if expectedComment == "" {
		expectedComment = want.ID
	}
	if got.comment != "duckos_fw:"+sanitizeComment(expectedComment) {
		return fmt.Errorf("ownership comment %q does not match rule %q", got.comment, want.ID)
	}
	return nil
}

// decodeObservedRule walks the nft JSON expression list and
// extracts the semantic match/verdict/comment tuples. Unknown
// expressions (e.g. counter) are tolerated; expressions that
// would change match semantics but cannot be classified fail
// verification closed.
func decodeObservedRule(body []byte) (*observedRule, error) {
	var wire struct {
		Family  string `json:"family"`
		Table   string `json:"table"`
		Chain   string `json:"chain"`
		Comment string `json:"comment"`
		Expr    []map[string]json.RawMessage
	}
	// The envelope is decoded leniently because the kernel adds
	// bookkeeping fields (e.g. handles); every semantic element
	// (expressions, verdicts) is validated strictly below.
	if err := json.Unmarshal(body, &wire); err != nil {
		return nil, fmt.Errorf("fwrules: verify: invalid observed rule")
	}
	r := &observedRule{Family: wire.Family, Table: wire.Table, Chain: wire.Chain, comment: wire.Comment}
	for _, expr := range wire.Expr {
		kind, payload := onlyKey(expr)
		switch kind {
		case "match":
			var m struct {
				Left  map[string]json.RawMessage `json:"left"`
				Op    string                     `json:"op"`
				Right json.RawMessage            `json:"right"`
			}
			if err := strictUnmarshal(payload, &m); err != nil || m.Op != "==" {
				return nil, fmt.Errorf("fwrules: verify: unclassifiable match expression")
			}
			if err := r.applyMatch(m.Left, m.Right); err != nil {
				return nil, err
			}
		case "verdict":
			var v struct {
				Verdict string `json:"verdict"`
			}
			if err := strictUnmarshal(payload, &v); err != nil {
				return nil, fmt.Errorf("fwrules: verify: invalid verdict expression")
			}
			if err := r.setVerdict(v.Verdict); err != nil {
				return nil, err
			}
		case "accept", "drop":
			// nft 1.x emits terminal verdicts as bare {"drop":null}.
			if err := r.setVerdict(kind); err != nil {
				return nil, err
			}
		case "reject":
			var rej struct {
				Type string `json:"type"`
			}
			_ = json.Unmarshal(payload, &rej)
			if err := r.setVerdict("reject"); err != nil {
				return nil, err
			}
		case "comment":
			// Legacy shape: comment carried as an expression.
			var s string
			if err := json.Unmarshal(payload, &s); err != nil {
				return nil, fmt.Errorf("fwrules: verify: invalid comment expression")
			}
			r.comment = s
		case "counter":
			// Kernel bookkeeping; ignored by design.
		default:
			return nil, fmt.Errorf("fwrules: verify: unsupported expression %q", kind)
		}
	}
	if r.action == "" {
		return nil, fmt.Errorf("fwrules: verify: observed rule without a terminal verdict")
	}
	return r, nil
}

func (r *observedRule) setVerdict(v string) error {
	if r.action != "" || (v != "accept" && v != "drop" && v != "reject") {
		return fmt.Errorf("fwrules: verify: unexpected verdict %q", v)
	}
	r.action = v
	return nil
}

func (r *observedRule) applyMatch(left map[string]json.RawMessage, right json.RawMessage) error {
	switch kind, payload := onlyKey(left); kind {
	case "payload":
		var p struct {
			Protocol string `json:"protocol"`
			Field    string `json:"field"`
		}
		if err := strictUnmarshal(payload, &p); err != nil {
			return fmt.Errorf("fwrules: verify: unclassifiable payload match")
		}
		switch {
		case p.Protocol == "ip" && p.Field == "saddr":
			return decodeString(right, &r.srcPrefix)
		case p.Protocol == "ip" && p.Field == "daddr":
			return decodeString(right, &r.dstPrefix)
		case (p.Protocol == "tcp" || p.Protocol == "udp") && p.Field == "dport":
			r.protocol = p.Protocol
			return decodeInt(right, &r.dstPort)
		default:
			return fmt.Errorf("fwrules: verify: unclassifiable payload match %s/%s", p.Protocol, p.Field)
		}
	case "meta":
		var m struct {
			Key string `json:"key"`
		}
		if err := strictUnmarshal(payload, &m); err != nil {
			return fmt.Errorf("fwrules: verify: unclassifiable meta match")
		}
		switch m.Key {
		case "iifname":
			return decodeString(right, &r.inIface)
		case "oifname":
			return decodeString(right, &r.outIface)
		case "l4proto":
			return decodeString(right, &r.protocol)
		default:
			return fmt.Errorf("fwrules: verify: unclassifiable meta match %q", m.Key)
		}
	default:
		return fmt.Errorf("fwrules: verify: unclassifiable match expression")
	}
}

func decodeString(raw json.RawMessage, dst *string) error {
	if *dst != "" {
		return fmt.Errorf("fwrules: verify: duplicate match on the same key")
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		*dst = s
		return nil
	}
	// nft 1.x emits prefixes as {"prefix":{"addr":..,"len":..}}.
	var p struct {
		Prefix struct {
			Addr string `json:"addr"`
			Len  int    `json:"len"`
		} `json:"prefix"`
	}
	if err := json.Unmarshal(raw, &p); err == nil && p.Prefix.Addr != "" {
		*dst = p.Prefix.Addr + "/" + fmt.Sprint(p.Prefix.Len)
		return nil
	}
	return fmt.Errorf("fwrules: verify: match value is neither a string nor a prefix")
}

func decodeInt(raw json.RawMessage, dst *int) error {
	if *dst != 0 {
		return fmt.Errorf("fwrules: verify: duplicate match on the same key")
	}
	return json.Unmarshal(raw, dst)
}

func strictUnmarshal(raw []byte, v any) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	return d.Decode(v)
}

func onlyKey(object map[string]json.RawMessage) (string, json.RawMessage) {
	for k, v := range object {
		return k, v
	}
	return "", nil
}
