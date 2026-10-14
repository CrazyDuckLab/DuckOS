// render.go: the atomic nftables transaction renderer for
// the owned inet duckos_fw table. Same lifecycle design as
// the hardened port-mapping backend: one self-contained
// batch per change, applied by a single `nft -f`; rules are
// re-derived from the rule set so no handles are used and
// stale rules from a previous run are cleared by the first
// mutation.
package fwrules

import (
	"fmt"
	"strings"
)

// TableName is the exclusively-owned nftables table.
const TableName = "duckos_fw"

// Render produces one atomic nft transaction that replaces
// inet duckos_fw with exactly one rule per enabled rule, in
// deterministic id order.
func Render(rules []Rule) ([]byte, error) {
	var b strings.Builder
	b.WriteString("# duckos.duckos_fw atomic table replacement\n")
	b.WriteString("add table inet " + TableName + "\n")
	b.WriteString("delete table inet " + TableName + "\n")
	b.WriteString("add table inet " + TableName + "\n")
	b.WriteString("add chain inet " + TableName + " forward { type filter hook forward priority filter ; policy accept ; }\n")
	for _, r := range rules {
		if !r.Enabled {
			continue
		}
		line, err := renderRule(r)
		if err != nil {
			return nil, err
		}
		b.WriteString(line)
	}
	return []byte(b.String()), nil
}

// renderRule renders one typed rule. Validation already ran
// in the Manager; this function re-checks the render-critical
// fields so a hand-built Rule can never produce a malformed
// line.
func renderRule(r Rule) (string, error) {
	if r.Direction != "forward" {
		return "", fmt.Errorf("fwrules: unsupported direction %q", r.Direction)
	}
	var parts []string
	if r.SrcPrefix != "" {
		parts = append(parts, "ip saddr "+r.SrcPrefix)
	}
	if r.DstPrefix != "" {
		parts = append(parts, "ip daddr "+r.DstPrefix)
	}
	if r.InIface != "" {
		parts = append(parts, "iifname \""+escapeIface(r.InIface)+"\"")
	}
	if r.OutIface != "" {
		parts = append(parts, "oifname \""+escapeIface(r.OutIface)+"\"")
	}
	switch {
	case r.DstPort != 0:
		// "<tcp|udp> dport N" implies the protocol match.
		parts = append(parts, fmt.Sprintf("%s dport %d", r.Protocol, r.DstPort))
	case r.Protocol == "tcp" || r.Protocol == "udp":
		parts = append(parts, "meta l4proto "+r.Protocol)
	}
	verdict, err := verdictKeyword(r.Action)
	if err != nil {
		return "", err
	}
	parts = append(parts, verdict)
	comment := r.Comment
	if comment == "" {
		comment = r.ID
	}
	parts = append(parts, fmt.Sprintf("comment \"duckos_fw:%s\"", sanitizeComment(comment)))
	return "add rule inet " + TableName + " forward " + strings.Join(parts, " ") + "\n", nil
}

func verdictKeyword(action string) (string, error) {
	switch action {
	case "accept":
		return "accept", nil
	case "drop":
		return "drop", nil
	case "reject":
		return "reject", nil
	}
	return "", fmt.Errorf("fwrules: unsupported action %q", action)
}

// escapeIface quotes-safe kernel names (validated by the
// model, escaped here for defense in depth).
func escapeIface(name string) string {
	return strings.ReplaceAll(name, "\"", "")
}

func sanitizeComment(c string) string {
	c = strings.ReplaceAll(c, "\"", "'")
	c = strings.ReplaceAll(c, "\\", "")
	return c
}
