// Package nftlaunch defines the bounded IPC protocol shared by
// duck-agent (client) and duck-nft-launcher (privileged server).
//
// BETA.3 privilege-boundary remediation: duck-agent runs with
// CapabilityBoundingSet= empty and NoNewPrivileges=yes, so it cannot
// perform the CAP_NET_ADMIN netlink operations nftables requires.
// Instead of exec'ing nft directly, the agent sends a framed request
// over /run/duckos-agent/nft-launcher.sock; the launcher (its own
// systemd unit, CAP_NET_ADMIN only) validates the request against
// the DuckOS ownership allowlist below and, only then, execs nft.
//
// SECURITY MODEL (post-audit, F1/F2):
//
// The validator is the only authorization boundary between a (possibly
// compromised) agent and the kernel's nftables state. Two batch
// formats exist and both are validated with parsers whose semantics
// deliberately match the consuming parser:
//
//   - Text batches are lexed with a complete nft-compatible lexer
//     (quoted strings with escapes, comments, braces, semicolons)
//     BEFORE any grammar decision, so the authorization decision can
//     never disagree with what nft actually executes. Anything the
//     lexer cannot tokenize unambiguously is rejected, not guessed.
//   - JSON batches are decoded with a duplicate-key-rejecting strict
//     decoder and validated structurally: exactly one allowlisted
//     verb, exactly one allowlisted object kind, an explicit
//     per-kind field allowlist, and ownership checks against the
//     nested object nft will actually consume — never the wrapper.
//
// Both sides also enforce size bounds on every frame, batch and
// output so a flooded or compromised agent cannot exhaust launcher
// resources.
package nftlaunch

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

// Canonical launcher socket path. duck-agent owns /run/duckos-agent
// (RuntimeDirectory) exactly like the WG and FRR launchers.
const DefaultSocket = "/run/duckos-agent/nft-launcher.sock"

// Op is a launcher operation name. The set is closed: any other
// value is rejected before any nft execution.
type Op string

const (
	// OpCheck validates a batch with `nft --check` without mutating
	// the kernel.
	OpCheck Op = "check"
	// OpApply validates a batch with `nft --check` and then loads it
	// atomically with `nft -f <file>`.
	OpApply Op = "apply"
	// OpListTable runs `nft --json list table <family> <name>` for an
	// owned table only (read-only).
	OpListTable Op = "list_table"
	// OpListRuleset runs `nft --json list ruleset` and filters the
	// output SERVER-SIDE down to the DuckOS-owned tables (F7): the
	// agent can never observe foreign nftables objects.
	OpListRuleset Op = "list_ruleset"
	// OpPing is an availability probe; it never execs nft.
	OpPing Op = "ping"
)

// OwnedTables is the complete set of nftables tables DuckOS owns.
// The launcher refuses every batch and every read that touches a
// table outside this set, and every family other than inet.
var OwnedTables = map[string]struct{}{
	"duckos":      {},
	"duckos_fw":   {},
	"duckos_upnp": {},
}

// OwnedFamilies is the family allowlist. Every DuckOS table is
// declared in inet; ip/ip6/... families are refused.
var OwnedFamilies = map[string]struct{}{
	"inet": {},
}

// Limits bound every request before the launcher touches nft.
const (
	// MaxBatchBytes caps the nft script carried in one request. The
	// network safeapply path bounds its snapshot at 1 MiB
	// (maxNFTSnapshotBytes); the launcher uses the same ceiling.
	MaxBatchBytes = 1 << 20
	// MaxStatements caps the number of nft statements in one batch.
	MaxStatements = 4096
	// MaxFrameBytes bounds one protocol frame (request or response).
	// The owned-tables listing of a lab router is far below this; a
	// larger body is rejected at the door.
	MaxFrameBytes = 4 << 20
	// MaxOutputBytes bounds captured nft stdout for read-only ops.
	MaxOutputBytes = 4 << 20
	// MaxStringBytes bounds one quoted string inside a text batch.
	MaxStringBytes = 4096
)

// Request is one launcher operation.
type Request struct {
	Op     Op     `json:"op"`
	Family string `json:"family,omitempty"`
	Table  string `json:"table,omitempty"`
	// Batch is the base64-encoded nft script (mutation ops only).
	Batch []byte `json:"batch,omitempty"`
	// JSONInput marks Batch as nft's JSON input format (the recovery
	// path emits `{"nftables":[...]}` documents and nft parses them
	// only when invoked with --json). The launcher validates either
	// format against the same ownership allowlist.
	JSONInput bool `json:"json,omitempty"`
}

// Response is one launcher reply.
type Response struct {
	Status int    `json:"status"`
	Code   string `json:"code"`
	// Output is the base64-encoded nft stdout for read-only ops;
	// empty otherwise. nft stderr is never relayed verbatim when it
	// can carry rule payloads; the launcher truncates it into
	// Message.
	Output []byte `json:"output,omitempty"`
	// Message is a short, structured error string (already redacted:
	// the launcher never echoes the offending batch line).
	Message string `json:"message,omitempty"`
}

// EncodeFrame writes one length-prefixed JSON frame.
func EncodeFrame(w io.Writer, v any) error {
	body, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if len(body) > MaxFrameBytes {
		return fmt.Errorf("frame of %d bytes exceeds limit %d", len(body), MaxFrameBytes)
	}
	head := [4]byte{
		byte(len(body) >> 24), byte(len(body) >> 16), byte(len(body) >> 8), byte(len(body)),
	}
	if _, err := w.Write(head[:]); err != nil {
		return err
	}
	_, err = w.Write(body)
	return err
}

// DecodeFrame reads exactly one length-prefixed JSON frame.
func DecodeFrame(r io.Reader, v any) error {
	head := make([]byte, 4)
	if _, err := io.ReadFull(r, head); err != nil {
		return err
	}
	n := int(head[0])<<24 | int(head[1])<<16 | int(head[2])<<8 | int(head[3])
	if n <= 0 || n > MaxFrameBytes {
		return fmt.Errorf("frame length %d is out of range", n)
	}
	body := make([]byte, n)
	if _, err := io.ReadFull(r, body); err != nil {
		return err
	}
	return json.Unmarshal(body, v)
}

// ValidateReadTarget enforces the table/family allowlist for
// read-only operations. Unknown tables are an authorization failure,
// not a lookup miss.
func ValidateReadTarget(family, table string) error {
	if _, ok := OwnedFamilies[family]; !ok {
		return fmt.Errorf("family %q is not DuckOS-owned", family)
	}
	if _, ok := OwnedTables[table]; !ok {
		return fmt.Errorf("table %q is not DuckOS-owned", table)
	}
	return nil
}

// ValidateBatch performs the ownership allowlist pass over an nft
// batch. The format is detected (text vs nft JSON) and dispatched to
// the matching strict validator; a batch that cannot be parsed
// unambiguously in either format is rejected.
func ValidateBatch(batch []byte) error {
	trimmed := bytes.TrimSpace(batch)
	if len(trimmed) > 0 && trimmed[0] == '{' {
		return ValidateJSONBatch(batch)
	}
	return ValidateTextBatch(batch)
}

// ============================================================
// F1: text batches — complete nft-compatible lexer
// ============================================================
//
// The previous implementation split statements on raw characters and
// therefore interpreted quotes, comments and braces differently from
// nft's own scanner; the independent audit demonstrated a
// foreign-table deletion by exploiting that differential. The fix
// below lexes the ENTIRE batch into tokens first — strings, comments
// and brace nesting are resolved at the lexical layer, exactly like
// nft's scanner resolves them — and only then applies the statement
// grammar. There is no character-level fast path left.

// tokKind classifies one lexical token.
type tokKind int

const (
	tokWord   tokKind = iota // bare identifier / command text
	tokString                // double-quoted string (decoded value kept)
	tokLBrace
	tokRBrace
	tokSemi
	tokNL // newline: nft's statement separator outside strings/comments
)

type token struct {
	kind tokKind
	text string
	// line is the 1-based source line, used only for redacted errors.
	line int
}

// lexText tokenizes a batch with nft-compatible lexical rules:
//
//   - `#` outside a string starts a comment running to end of line;
//   - `"..."` outside a comment is a string; inside, `\"` and `\\`
//     are the only accepted escapes — any other backslash sequence
//     is REJECTED (nft's exact escape set is broader, but DuckOS
//     renderers never emit one; on ambiguity we fail closed);
//   - `{`, `}` and `;` are structural tokens;
//   - a backslash outside a string (line continuation) is rejected;
//   - unterminated strings and unbalanced braces are rejected.
func lexText(batch []byte) ([]token, error) {
	var toks []token
	var cur []byte
	curLine := 1
	tokLine := 1
	flush := func() {
		if len(cur) > 0 {
			toks = append(toks, token{kind: tokWord, text: string(cur), line: tokLine})
			cur = cur[:0]
		}
	}
	i := 0
	for i < len(batch) {
		c := batch[i]
		switch {
		case c == '\n':
			flush()
			toks = append(toks, token{kind: tokNL, line: curLine})
			curLine++
			i++
		case c == ' ' || c == '\t' || c == '\r':
			flush()
			i++
		case c == '#':
			// Comment to end of line; not tokenized.
			for i < len(batch) && batch[i] != '\n' {
				i++
			}
		case c == '"':
			flush()
			tokLine = curLine
			s, next, err := lexString(batch, i, curLine)
			if err != nil {
				return nil, err
			}
			if len(s) > MaxStringBytes {
				return nil, errors.New("quoted string exceeds the size limit")
			}
			toks = append(toks, token{kind: tokString, text: s, line: curLine})
			i = next
		case c == '{':
			flush()
			toks = append(toks, token{kind: tokLBrace, line: curLine})
			i++
		case c == '}':
			flush()
			toks = append(toks, token{kind: tokRBrace, line: curLine})
			i++
		case c == ';':
			flush()
			toks = append(toks, token{kind: tokSemi, line: curLine})
			i++
		case c == '\\':
			return nil, fmt.Errorf("line %d: a backslash outside a quoted string is not accepted", curLine)
		default:
			if len(cur) == 0 {
				tokLine = curLine
			}
			cur = append(cur, c)
			i++
		}
	}
	flush()
	return toks, nil
}

// lexString decodes a double-quoted string starting at batch[i]=='"'.
// It returns the decoded value and the index just past the closing
// quote. Only `\"` and `\\` escapes are accepted; everything else
// fails closed.
func lexString(batch []byte, i, line int) (string, int, error) {
	i++ // skip opening quote
	var out []byte
	for i < len(batch) {
		c := batch[i]
		switch c {
		case '"':
			return string(out), i + 1, nil
		case '\n':
			return "", 0, fmt.Errorf("line %d: unterminated quoted string", line)
		case '\\':
			if i+1 >= len(batch) {
				return "", 0, fmt.Errorf("line %d: dangling escape at end of batch", line)
			}
			switch batch[i+1] {
			case '"', '\\':
				out = append(out, batch[i+1])
				i += 2
			default:
				return "", 0, fmt.Errorf("line %d: unsupported escape \\%c in quoted string", line, batch[i+1])
			}
		default:
			out = append(out, c)
			i++
		}
	}
	return "", 0, fmt.Errorf("line %d: unterminated quoted string", line)
}

// stmtNode is one parsed statement: its word/string tokens plus any
// brace groups it carries (each group is itself a list of statement
// nodes, so `table inet duckos { chain input { ... } }` becomes a
// two-level tree; deeper nesting is rejected).
type stmtNode struct {
	toks     []token
	children []stmtNode
}

// parseStatements builds the statement tree. At every level, `;`
// terminates a statement, `{` opens a nested group and `}` closes the
// current one. Empty statements are rejected except for the trailing
// `;` immediately before a closing brace (the canonical nft chain
// definition form ends with `; }`).
func parseStatements(toks []token, depth int) ([]stmtNode, int, error) {
	var nodes []stmtNode
	cur := stmtNode{}
	nonEmpty := func() bool { return len(cur.toks) > 0 || len(cur.children) > 0 }
	for i := 0; i < len(toks); i++ {
		t := toks[i]
		switch t.kind {
		case tokLBrace:
			if depth >= 3 {
				return nil, 0, fmt.Errorf("line %d: brace nesting deeper than table>chain>inline-set is not accepted", t.line)
			}
			children, next, err := parseStatements(toks[i+1:], depth+1)
			if err != nil {
				return nil, 0, err
			}
			// The brace group is one opaque child whose members are
			// the group's statements.
			cur.children = append(cur.children, stmtNode{children: children})
			i = i + 1 + next
		case tokRBrace:
			if depth == 0 {
				return nil, 0, fmt.Errorf("line %d: unbalanced '}'", t.line)
			}
			if nonEmpty() {
				nodes = append(nodes, cur)
			}
			return nodes, i, nil
		case tokSemi, tokNL:
			if nonEmpty() {
				nodes = append(nodes, cur)
				cur = stmtNode{}
				continue
			}
			if t.kind == tokNL {
				// Blank lines are not statements.
				continue
			}
			// An empty statement is rejected as ambiguous — the one
			// exception is the canonical trailing `; }` of a chain
			// definition.
			if i+1 >= len(toks) || toks[i+1].kind != tokRBrace {
				return nil, 0, fmt.Errorf("line %d: empty statement", t.line)
			}
		default:
			cur.toks = append(cur.toks, t)
		}
	}
	if depth != 0 {
		return nil, 0, errors.New("unbalanced '{' at end of batch")
	}
	if nonEmpty() {
		nodes = append(nodes, cur)
	}
	return nodes, len(toks), nil
}

// ValidateTextBatch enforces the ownership allowlist over a text nft
// script. Grammar (after strict lexing):
//
//	flat:  add|delete|destroy|flush table inet <owned>
//	       add|delete|flush chain inet <owned> <chain> [ { ... } ]
//	       add rule inet <owned> <chain> <payload...>
//	block: table inet <owned> { chain <name> { ... } }
//
// Quoted strings in the fixed verb/family/table/chain prefix are
// rejected (DuckOS renderers never emit them; accepting them would
// reopen representation differentials). Rule payloads are free-form
// nft expressions but may never begin with an object-class verb, and
// `define`/`include`/`$var` are refused everywhere.
func ValidateTextBatch(batch []byte) error {
	if len(bytes.TrimSpace(batch)) == 0 {
		return errors.New("batch is empty")
	}
	if len(batch) > MaxBatchBytes {
		return fmt.Errorf("batch of %d bytes exceeds limit %d", len(batch), MaxBatchBytes)
	}
	toks, err := lexText(batch)
	if err != nil {
		return err
	}
	stmts, _, err := parseStatements(toks, 0)
	if err != nil {
		return err
	}
	if len(stmts) > MaxStatements {
		return fmt.Errorf("batch has %d statements, limit is %d", len(stmts), MaxStatements)
	}
	for i := range stmts {
		if err := validateStatement(&stmts[i]); err != nil {
			return err
		}
	}
	return nil
}

func validateStatement(s *stmtNode) error {
	if len(s.toks) == 0 {
		return errors.New("empty statement")
	}
	words := wordToks(s.toks)
	// Block-carrying statement: `table <fam> <owned> { ... }` — the
	// only block form at top level. `add chain ... { ... }` and
	// inline-set groups in rule payloads are validated flat below.
	if words[0].kind == tokWord && words[0].text == "table" && len(s.children) > 0 {
		if len(words) != 3 {
			return fmt.Errorf("line %d: block statement is not an owned table definition", words[0].line)
		}
		if words[1].kind != tokWord || words[2].kind != tokWord {
			return fmt.Errorf("line %d: quoted strings are not accepted in the command prefix", words[0].line)
		}
		if err := ValidateReadTarget(words[1].text, words[2].text); err != nil {
			return err
		}
		if len(s.children) != 1 {
			return fmt.Errorf("line %d: table block must be exactly one brace group", words[0].line)
		}
		if len(s.children[0].children) == 0 {
			return fmt.Errorf("line %d: empty table block", words[0].line)
		}
		for i := range s.children[0].children {
			if err := validateTableBodyStatement(&s.children[0].children[i]); err != nil {
				return err
			}
		}
		return nil
	}
	return validateFlatStatement(s)
}

// wordToks returns only word/string tokens with structural tokens
// removed; grammar-position callers additionally check kind.
func wordToks(toks []token) []token {
	out := make([]token, 0, len(toks))
	for _, t := range toks {
		if t.kind == tokWord || t.kind == tokString {
			out = append(out, t)
		}
	}
	return out
}

func validateTableBodyStatement(s *stmtNode) error {
	if len(s.toks) == 0 {
		return errors.New("empty statement")
	}
	words := wordToks(s.toks)
	if words[0].kind != tokWord || words[0].text != "chain" {
		return fmt.Errorf("line %d: only chain definitions are accepted inside a table block", words[0].line)
	}
	if len(words) != 2 || words[1].kind != tokWord || !validChainName(words[1].text) {
		return fmt.Errorf("line %d: chain definition inside a table block must be `chain <name> { ... }`", words[0].line)
	}
	if len(s.children) != 1 {
		return fmt.Errorf("line %d: chain definition requires exactly one brace block", words[0].line)
	}
	// The chain body is a statement list: optional hook clauses and
	// rule lines. Each rule line may itself carry inline-set groups
	// (opaque at this level; nft --check is the syntactic authority).
	body := s.children[0].children
	for i := range body {
		rule := &body[i]
		if len(rule.toks) == 0 {
			return errors.New("empty statement in chain body")
		}
		for j := range rule.children {
			for m := range rule.children[j].children {
				if len(rule.children[j].children[m].children) > 0 {
					return fmt.Errorf("line %d: nesting inside an inline set is not accepted", rule.toks[0].line)
				}
			}
		}
		if err := validateRuleTokens(rule.toks); err != nil {
			return err
		}
	}
	return nil
}

// validateFlatStatement allowlists one flat statement. A trailing
// brace group is accepted only for `add chain ... { ... }`.
func validateFlatStatement(s *stmtNode) error {
	words := wordToks(s.toks)
	if len(words) < 4 {
		return fmt.Errorf("line %d: statement is too short to be owned", s.toks[0].line)
	}
	head := words[:4]
	for _, t := range head {
		if t.kind != tokWord {
			return fmt.Errorf("line %d: quoted strings are not accepted in the command prefix", t.line)
		}
	}
	verb, sub := head[0].text, head[1].text
	family, table := head[2].text, head[3].text
	switch verb + " " + sub {
	case "add table", "delete table", "destroy table", "flush table":
		if len(words) != 4 || len(s.children) > 0 {
			return fmt.Errorf("line %d: %s statement has an unexpected tail", head[0].line, verb+" "+sub)
		}
	case "add chain":
		if len(words) < 5 {
			return fmt.Errorf("line %d: chain statement has no chain name", head[0].line)
		}
		if words[4].kind != tokWord || !validChainName(words[4].text) {
			return fmt.Errorf("line %d: chain name is not canonical", words[4].line)
		}
		switch {
		case len(words) == 5 && len(s.children) == 0: // bare declaration
		case len(words) == 5 && len(s.children) == 1: // `{ ... }` definition
			if err := validateChainBody(flattenChildren(&s.children[0])); err != nil {
				return err
			}
		default:
			return fmt.Errorf("line %d: chain definition is not the canonical one-clause form", head[0].line)
		}
	case "delete chain", "flush chain":
		if len(words) != 5 || len(s.children) > 0 {
			return fmt.Errorf("line %d: %s statement has an unexpected tail", head[0].line, verb+" "+sub)
		}
		if words[4].kind != tokWord || !validChainName(words[4].text) {
			return fmt.Errorf("line %d: chain name is not canonical", words[4].line)
		}
	case "add rule":
		if len(words) < 6 {
			return fmt.Errorf("line %d: rule statement has no payload", head[0].line)
		}
		if words[4].kind != tokWord || !validChainName(words[4].text) {
			return fmt.Errorf("line %d: chain name is not canonical", words[4].line)
		}
		// Payload: free-form rule expression with opaque inline-set
		// groups. A new object-class command can never hide inside a
		// rule expression for nft (commands only start after a `;`/
		// newline, which our tokenizer already split on), but the
		// reserved-verb and variable bans still apply to the payload
		// words.
		for g := range s.children {
			for m := range s.children[g].children {
				if len(s.children[g].children[m].children) > 0 {
					return fmt.Errorf("line %d: nesting inside an inline set is not accepted", s.toks[0].line)
				}
			}
		}
		payload := append(append([]token{}, s.toks[5:]...), flattenChildren(s)...)
		if err := validateRuleTokens(payload); err != nil {
			return err
		}
	default:
		return fmt.Errorf("line %d: verb %q is not on the DuckOS launcher allowlist", head[0].line, verb+" "+sub)
	}
	if err := ValidateReadTarget(family, table); err != nil {
		return err
	}
	return nil
}

// flattenChildren joins a statement's brace-group tokens back into
// one token stream (inline-set payload content).
func flattenChildren(s *stmtNode) []token {
	var out []token
	for i := range s.children {
		for j := range s.children[i].children {
			out = append(out, flattenChildren(&s.children[i].children[j])...)
		}
		out = append(out, s.children[i].toks...)
	}
	return out
}

// validateChainBody checks the `{ type nat hook prerouting priority -100 ; policy accept ; }`
// definition clauses. Only the hook-definition keywords nft defines
// for chain declarations are accepted.
func validateChainBody(toks []token) error {
	for _, w := range toks {
		if w.kind != tokWord {
			return fmt.Errorf("line %d: quoted strings are not accepted in a chain definition", w.line)
		}
		switch w.text {
		case "type", "hook", "priority", "prio", "policy", "device", "devices",
			"filter", "nat", "route", "prerouting", "input", "forward", "output", "postrouting",
			"ingress", "accept", "drop", "dstnat", "srcnat":
		default:
			if isNumericWord(w.text) {
				continue
			}
			return fmt.Errorf("line %d: unexpected token %q in a chain definition", w.line, w.text)
		}
	}
	return nil
}

// validateRuleTokens checks one rule line inside a chain body. Rules
// are free-form nft expressions (nft --check is the syntactic
// authority) but a rule line must never open a new object class —
// those are the ownership-relevant verbs and are denied outright.
func validateRuleTokens(toks []token) error {
	if len(toks) == 0 {
		return errors.New("empty statement")
	}
	if toks[0].kind == tokString {
		return errors.New("a rule line may not begin with a quoted string")
	}
	switch toks[0].text {
	case "table", "chain", "add", "delete", "destroy", "flush", "include", "define", "reset", "monitor", "element", "map", "set", "ruleset":
		return fmt.Errorf("rule line may not begin with the reserved verb %q", toks[0].text)
	}
	for _, t := range toks {
		if t.kind == tokWord && strings.HasPrefix(t.text, "$") {
			return errors.New("rule line contains a variable reference; define is not permitted")
		}
	}
	return nil
}

func isNumericWord(s string) bool {
	if s == "" {
		return false
	}
	i := 0
	if s[0] == '-' || s[0] == '+' {
		i = 1
	}
	if i >= len(s) {
		return false
	}
	for ; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// validChainName enforces the canonical chain-name form used by every
// DuckOS renderer: letters, digits, dash and underscore, 1-31 chars,
// no quoting, no wildcards.
func validChainName(s string) bool {
	if len(s) == 0 || len(s) > 31 {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
		default:
			return false
		}
	}
	return true
}

// ============================================================
// F2: JSON batches — strict structural validation
// ============================================================
//
// The wrapper/kind/value separation of nft's JSON input previously
// allowed a request to be authorized on one representation while the
// kernel executed another. The validator below decodes with a
// duplicate-key-rejecting scanner and then enforces, per command:
// exactly one allowlisted verb, exactly one allowlisted object kind,
// a per-kind field allowlist, and ownership taken from the nested
// object nft will actually consume. Unknown fields, unknown kinds,
// conflicting values and ruleset-wide commands are rejected.

// jsonBatchVerbs is the closed verb set accepted in a JSON batch.
var jsonBatchVerbs = map[string]struct{}{
	"add": {}, "delete": {}, "destroy": {}, "flush": {}, "insert": {}, "replace": {},
}

// jsonKindFields is the per-kind field allowlist for the nested
// objects nft consumes. `handle` is allowed only with the delete
// verb (targeted removal of a captured object). Field sets mirror
// what DuckOS captures from `nft --json list table`; anything else
// is rejected instead of guessed.
var jsonKindFields = map[string]map[string]struct{}{
	"table": {"family": {}, "name": {}, "flags": {}},
	"chain": {"family": {}, "table": {}, "name": {}, "type": {}, "hook": {}, "prio": {}, "priority": {}, "policy": {}, "devices": {}},
	"rule":  {"family": {}, "table": {}, "chain": {}, "expr": {}, "comment": {}},
}

// ValidateJSONBatch enforces the ownership allowlist over an nft
// JSON document (the format `nft -j -f -` parses).
func ValidateJSONBatch(batch []byte) error {
	if len(bytes.TrimSpace(batch)) == 0 {
		return errors.New("batch is empty")
	}
	if len(batch) > MaxBatchBytes {
		return fmt.Errorf("batch of %d bytes exceeds limit %d", len(batch), MaxBatchBytes)
	}
	commands, err := decodeStrictJSON(batch)
	if err != nil {
		return err
	}
	if len(commands) > MaxStatements {
		return fmt.Errorf("batch has %d commands, limit is %d", len(commands), MaxStatements)
	}
	for i, cmd := range commands {
		if err := validateJSONCommand(i, cmd); err != nil {
			return err
		}
	}
	return nil
}

// decodeStrictJSON parses {"nftables":[...]} while rejecting
// duplicate object keys anywhere in the document (Go's standard
// decoder silently keeps the last occurrence, which is exactly the
// wrapper/object differential F2 exploits).
func decodeStrictJSON(batch []byte) ([]map[string]strictValue, error) {
	dec := json.NewDecoder(bytes.NewReader(batch))
	dec.UseNumber()
	tok, err := dec.Token()
	if err != nil {
		return nil, errors.New("batch is not a valid nft JSON document")
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return nil, errors.New("batch is not a valid nft JSON document")
	}
	var commands []map[string]strictValue
	sawNftables := false
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return nil, errors.New("batch is not a valid nft JSON document")
		}
		key, ok := keyTok.(string)
		if !ok {
			return nil, errors.New("batch is not a valid nft JSON document")
		}
		if key != "nftables" {
			return nil, fmt.Errorf("unknown document key %q", key)
		}
		if sawNftables {
			return nil, errors.New("duplicate \"nftables\" key")
		}
		sawNftables = true
		if commands, err = decodeStrictArray(dec); err != nil {
			return nil, err
		}
	}
	if !sawNftables {
		return nil, errors.New("batch has no \"nftables\" array")
	}
	if t, err := dec.Token(); err != nil {
		return nil, errors.New("batch is not a valid nft JSON document")
	} else if d, ok := t.(json.Delim); !ok || d != '}' {
		return nil, errors.New("batch is not a valid nft JSON document")
	}
	if dec.More() {
		return nil, errors.New("trailing content after the JSON document")
	}
	return commands, nil
}

// strictValue is one decoded JSON value with duplicate-key
// protection already applied to any nested object.
type strictValue struct {
	kind  byte // 'o' object, 'a' array, 's' string, 'n' number, 'b' bool, 'z' null
	obj   map[string]strictValue
	arr   []strictValue
	str   string
	extra any
}

func (v strictValue) strVal() (string, bool) {
	if v.kind == 's' {
		return v.str, true
	}
	return "", false
}

func decodeStrictArray(dec *json.Decoder) ([]map[string]strictValue, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, errors.New("batch is not a valid nft JSON document")
	}
	if d, ok := tok.(json.Delim); !ok || d != '[' {
		return nil, errors.New("\"nftables\" is not an array")
	}
	var out []map[string]strictValue
	for dec.More() {
		v, err := decodeStrictValue(dec)
		if err != nil {
			return nil, err
		}
		if v.kind != 'o' {
			return nil, errors.New("\"nftables\" entries must be objects")
		}
		out = append(out, v.obj)
	}
	if t, err := dec.Token(); err != nil {
		return nil, errors.New("batch is not a valid nft JSON document")
	} else if d, ok := t.(json.Delim); !ok || d != ']' {
		return nil, errors.New("batch is not a valid nft JSON document")
	}
	return out, nil
}

func decodeStrictValue(dec *json.Decoder) (strictValue, error) {
	tok, err := dec.Token()
	if err != nil {
		return strictValue{}, errors.New("batch is not a valid nft JSON document")
	}
	switch t := tok.(type) {
	case json.Delim:
		switch t {
		case '{':
			obj := map[string]strictValue{}
			for dec.More() {
				kt, err := dec.Token()
				if err != nil {
					return strictValue{}, errors.New("batch is not a valid nft JSON document")
				}
				key, ok := kt.(string)
				if !ok {
					return strictValue{}, errors.New("batch is not a valid nft JSON document")
				}
				if _, dup := obj[key]; dup {
					return strictValue{}, fmt.Errorf("duplicate JSON key %q", key)
				}
				val, err := decodeStrictValue(dec)
				if err != nil {
					return strictValue{}, err
				}
				obj[key] = val
			}
			if closeTok, err := dec.Token(); err != nil {
				return strictValue{}, errors.New("batch is not a valid nft JSON document")
			} else if d, ok := closeTok.(json.Delim); !ok || d != '}' {
				return strictValue{}, errors.New("batch is not a valid nft JSON document")
			}
			return strictValue{kind: 'o', obj: obj}, nil
		case '[':
			arr := []strictValue{}
			for dec.More() {
				v, err := decodeStrictValue(dec)
				if err != nil {
					return strictValue{}, err
				}
				arr = append(arr, v)
			}
			if closeTok, err := dec.Token(); err != nil {
				return strictValue{}, errors.New("batch is not a valid nft JSON document")
			} else if d, ok := closeTok.(json.Delim); !ok || d != ']' {
				return strictValue{}, errors.New("batch is not a valid nft JSON document")
			}
			return strictValue{kind: 'a', arr: arr}, nil
		default:
			return strictValue{}, errors.New("batch is not a valid nft JSON document")
		}
	case string:
		return strictValue{kind: 's', str: t}, nil
	case json.Number:
		return strictValue{kind: 'n', extra: t}, nil
	case bool:
		return strictValue{kind: 'b', extra: t}, nil
	case nil:
		return strictValue{kind: 'z'}, nil
	default:
		return strictValue{}, errors.New("batch is not a valid nft JSON document")
	}
}

// validateJSONCommand authorizes one {"verb": {kind: object}} entry
// against the nested object nft will consume.
func validateJSONCommand(i int, cmd map[string]strictValue) error {
	if len(cmd) != 1 {
		return fmt.Errorf("JSON command %d must carry exactly one verb", i)
	}
	var verb string
	var payload strictValue
	for k, v := range cmd {
		if _, ok := jsonBatchVerbs[k]; !ok {
			return fmt.Errorf("JSON verb %q is not on the DuckOS launcher allowlist", k)
		}
		verb, payload = k, v
	}
	if payload.kind != 'o' || len(payload.obj) != 1 {
		return fmt.Errorf("JSON command %d must carry exactly one object kind", i)
	}
	var kindName string
	var obj strictValue
	for k, v := range payload.obj {
		if _, ok := jsonKindFields[k]; !ok {
			return fmt.Errorf("JSON command %d uses unknown object kind %q", i, k)
		}
		kindName, obj = k, v
	}
	if obj.kind != 'o' {
		return fmt.Errorf("JSON command %d object %q is not an object", i, kindName)
	}
	for f := range obj.obj {
		if _, ok := jsonKindFields[kindName][f]; !ok {
			return fmt.Errorf("JSON command %d: field %q is not accepted on object kind %q", i, f, kindName)
		}
	}
	if _, ok := obj.obj["handle"]; ok && verb != "delete" {
		return fmt.Errorf("JSON command %d: handle is only accepted with the delete verb", i)
	}
	// Ownership comes from the object nft consumes, never from the
	// wrapper. Table objects carry name; chain/rule objects carry the
	// parent table reference.
	family, _ := obj.obj["family"].strVal()
	table := ""
	if kindName == "table" {
		table, _ = obj.obj["name"].strVal()
	} else {
		table, _ = obj.obj["table"].strVal()
	}
	if family == "" || table == "" {
		return fmt.Errorf("JSON command %d carries no complete ownership target", i)
	}
	if err := ValidateReadTarget(family, table); err != nil {
		return fmt.Errorf("JSON command %d: %v", i, err)
	}
	// A `flush` may only target a whole owned table, never the
	// ruleset: kind "table" only.
	if verb == "flush" && kindName != "table" {
		return fmt.Errorf("JSON command %d: flush is only accepted for an owned table", i)
	}
	return nil
}

// B64 is the single base64 codec for the protocol; both sides must
// agree on strict std encoding.
func B64(src []byte) string { return base64.StdEncoding.EncodeToString(src) }

// UnB64 decodes a strictly-encoded base64 field.
func UnB64(s string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(s)
}
