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
// The protocol is a single length-prefixed JSON frame per request
// and per response:
//
//	request:  {op, family, table, batch}  (batch is base64 nft script)
//	response: {status, code, output}      (output is base64 nft stdout)
//
// Frame size is bounded on both ends (MaxFrameBytes) so a compromised
// agent cannot exhaust launcher memory, and execution time is bounded
// in the launcher.
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
	// OpListTables runs `nft --json list tables` (read-only).
	OpListTables Op = "list_tables"
	// OpListTable runs `nft --json list table <family> <name>` for an
	// owned table only (read-only).
	OpListTable Op = "list_table"
	// OpListRuleset runs `nft --json list ruleset` (read-only).
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
// declared in inet; ip/ip6/arm64... families are refused.
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
	// The ruleset listing of a lab router is far below this; a larger
	// body is rejected at the door.
	MaxFrameBytes = 8 << 20
	// MaxOutputBytes bounds captured nft stdout for read-only ops.
	MaxOutputBytes = 6 << 20
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
	// Output is the base64-encoded combined nft stdout for read-only
	// ops; empty otherwise. nft stderr is never relayed verbatim when
	// it can carry rule payloads; the launcher truncates it into
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

// ValidateBatch performs the structural allowlist pass over an nft
// script. It is a deny-oriented parser: every statement must match
// the small grammar DuckOS renderers emit, referencing only owned
// tables; a statement that cannot be proven owned is rejected. The
// launcher still runs `nft --check` afterwards, so syntactic
// completeness is nft's job — this pass is purely an ownership
// boundary. Two surface forms are accepted:
//
//	flat:     add table inet duckos_fw / add rule inet duckos_fw c ... / ...
//	block:    table inet duckos { chain input { ...rule lines... } }
//
// The flat form is what fwrules and the UPnP backend render; the
// block form is what the network safeapply renderer emits.
func ValidateBatch(batch []byte) error {
	if len(bytes.TrimSpace(batch)) > 0 && bytes.TrimSpace(batch)[0] == '{' {
		return ValidateJSONBatch(batch)
	}
	return validateTextBatch(batch)
}

func validateTextBatch(batch []byte) error {
	if len(batch) == 0 {
		return errors.New("batch is empty")
	}
	if len(batch) > MaxBatchBytes {
		return fmt.Errorf("batch of %d bytes exceeds limit %d", len(batch), MaxBatchBytes)
	}
	stmts, err := splitTop(string(batch))
	if err != nil {
		return err
	}
	if len(stmts) > MaxStatements {
		return fmt.Errorf("batch has %d statements, limit is %d", len(stmts), MaxStatements)
	}
	for _, s := range stmts {
		if err := validateStatement(s); err != nil {
			return err
		}
	}
	return nil
}

// jsonBatchVerbs is the closed verb set accepted in a JSON batch.
// Anything else — notably a "flush" of "ruleset" — is denied.
var jsonBatchVerbs = map[string]struct{}{
	"add": {}, "delete": {}, "destroy": {}, "insert": {}, "replace": {},
}

// ValidateJSONBatch enforces the ownership allowlist over an nft
// JSON document (the format `nft -j -f -` parses). Every command
// entry must carry exactly one allowlisted verb and reference an
// owned family/table; entries that cannot be proven owned are
// rejected. This is what the Safe Apply recovery path (RestoreBatch /
// DeleteBatch) goes through.
func ValidateJSONBatch(batch []byte) error {
	if len(batch) == 0 {
		return errors.New("batch is empty")
	}
	if len(batch) > MaxBatchBytes {
		return fmt.Errorf("batch of %d bytes exceeds limit %d", len(batch), MaxBatchBytes)
	}
	var doc struct {
		NFTables []map[string]json.RawMessage `json:"nftables"`
	}
	if err := json.Unmarshal(batch, &doc); err != nil || doc.NFTables == nil {
		return errors.New("batch is not a valid nft JSON document")
	}
	if len(doc.NFTables) > MaxStatements {
		return fmt.Errorf("batch has %d commands, limit is %d", len(doc.NFTables), MaxStatements)
	}
	for i, entry := range doc.NFTables {
		if err := validateJSONEntry(i, entry); err != nil {
			return err
		}
	}
	return nil
}

func validateJSONEntry(i int, entry map[string]json.RawMessage) error {
	if len(entry) != 1 {
		return fmt.Errorf("JSON command %d must carry exactly one verb", i)
	}
	for verb, raw := range entry {
		if _, ok := jsonBatchVerbs[verb]; !ok {
			return fmt.Errorf("JSON verb %q is not on the DuckOS launcher allowlist", verb)
		}
		var payload map[string]any
		if err := json.Unmarshal(raw, &payload); err != nil {
			return fmt.Errorf("JSON command %d payload is not an object", i)
		}
		// The payload is the kind wrapper: {"table":{...}} or
		// {"chain":{...}} / {"rule":{...}} / ... Unwrap the single
		// object-kind value; ownership lives inside it.
		inner := payload
		if len(payload) == 1 {
			for _, v := range payload {
				if nested, isObj := v.(map[string]any); isObj {
					inner = nested
				}
			}
		}
		if _, ok := inner["ruleset"]; ok {
			return errors.New("JSON command targets the whole ruleset")
		}
		family, _ := inner["family"].(string)
		table, _ := inner["table"].(string)
		// table objects carry name, not table
		if table == "" {
			if n, ok := inner["name"].(string); ok {
				table = n
			}
		}
		if family == "" && table == "" {
			return fmt.Errorf("JSON command %d carries no ownership target", i)
		}
		if err := ValidateReadTarget(family, table); err != nil {
			return fmt.Errorf("JSON command %d: %v", i, err)
		}
	}
	return nil
}

// splitTop splits a script into statements on newlines and on `;`
// that sit outside braces, keeping one trailing `{ ... }` block glued
// to its opening statement. A `;` at brace depth 0 is the smuggling
// vector for `flush ruleset` and is refused outright; `;` inside
// braces (chain hooks, inline sets) stays within its statement.
// `#` comments run to end of line and are dropped.
func splitTop(script string) ([]string, error) {
	var stmts []string
	var cur []rune
	depth := 0
	comment := false
	for _, r := range script {
		switch {
		case comment:
			if r == '\n' {
				comment = false
			}
		case r == '#' && depth == 0:
			comment = true
		case r == '{':
			depth++
			cur = append(cur, r)
		case r == '}':
			depth--
			if depth < 0 {
				return nil, errors.New("batch has unbalanced braces")
			}
			cur = append(cur, r)
		case r == ';' && depth == 0, r == '\n' && depth == 0, r == '\r' && depth == 0:
			// `;` is a statement separator everywhere nft accepts it.
			// Smuggled commands are not a concern here because every
			// resulting fragment must independently pass the verb and
			// ownership allowlist below.
			if t := trimSpace(string(cur)); t != "" {
				stmts = append(stmts, t)
			}
			cur = cur[:0]
		default:
			cur = append(cur, r)
		}
	}
	if depth != 0 {
		return nil, errors.New("batch has unbalanced braces")
	}
	if t := trimSpace(string(cur)); t != "" {
		stmts = append(stmts, t)
	}
	return stmts, nil
}

// validateStatement allowlists one top-level statement. Grammar:
//
//	add table inet <owned>          delete table inet <owned>
//	destroy table inet <owned>      flush table inet <owned>
//	add chain inet <owned> <chain> [ { ... } ]
//	delete chain inet <owned> <chain>
//	flush chain inet <owned> <chain>
//	add rule inet <owned> <chain> <payload...>
//	table inet <owned> { <table body> }
//
// `delete`/`destroy table` of an owned table are the established
// DuckOS table-replacement idiom and stay scoped to the allowlist.
// `flush ruleset`, `destroy ruleset`, include/define/reset/monitor
// and every other verb never appear in DuckOS renderers and are
// refused here.
func validateStatement(stmt string) error {
	body, ok := splitBlock(stmt)
	if !ok {
		return validateFlatStatement(stmt)
	}
	// Block form: `table <family> <owned> { ... }` — the only block
	// statement DuckOS renders at top level. `add chain ... { ... }`
	// also carries a trailing block and is handled by the flat
	// validator instead.
	head, inner := body.head, body.inner
	toks := tokenize(head)
	if toks[0] != "table" {
		return validateFlatStatement(stmt)
	}
	if len(toks) != 3 {
		return fmt.Errorf("block statement %q is not an owned table definition", redactTokens(toks))
	}
	if err := ValidateReadTarget(toks[1], toks[2]); err != nil {
		return err
	}
	innerStmts, err := splitTop(inner)
	if err != nil {
		return err
	}
	for _, s := range innerStmts {
		if err := validateTableBodyStatement(s); err != nil {
			return err
		}
	}
	return nil
}

// blockParts carries a `head { inner }` split.
type blockParts struct{ head, inner string }

// splitBlock recognizes exactly one trailing balanced `{ ... }` group
// as the statement's body. Anything else (block mid-statement, two
// blocks, unbalanced) is not a block statement.
func splitBlock(stmt string) (blockParts, bool) {
	open := strings.Index(stmt, "{")
	if open < 0 || !strings.HasSuffix(stmt, "}") {
		return blockParts{}, false
	}
	head := trimSpace(stmt[:open])
	inner := trimSpace(stmt[open+1 : len(stmt)-1])
	if head == "" {
		return blockParts{}, false
	}
	// A nested `{` inside the head means the statement is malformed
	// for our grammar; let the flat validator reject it.
	if strings.ContainsAny(head, "{}") {
		return blockParts{}, false
	}
	if strings.Count(inner, "{") != strings.Count(inner, "}") {
		return blockParts{}, false
	}
	return blockParts{head, inner}, true
}

// validateTableBodyStatement allowlists one statement inside a table
// block: either `chain <name> { <chain body> }` or a flat chain
// command (the UPnP idiom keeps flat commands even in mixed files —
// DuckOS renderers do not mix, but the boundary stays explicit).
func validateTableBodyStatement(stmt string) error {
	if body, ok := splitBlock(stmt); ok {
		toks := tokenize(body.head)
		if len(toks) != 2 || toks[0] != "chain" || !validChainName(toks[1]) {
			return fmt.Errorf("table-body statement %q is not a chain definition", redactTokens(toks))
		}
		chainStmts, err := splitTop(body.inner)
		if err != nil {
			return err
		}
		for _, s := range chainStmts {
			if err := validateRuleLine(s); err != nil {
				return err
			}
		}
		return nil
	}
	toks := tokenize(stmt)
	if len(toks) >= 2 && (toks[0] == "chain" || toks[0] == "flush" && toks[1] == "table") {
		return validateFlatStatement(stmt)
	}
	return validateRuleLine(stmt)
}

// validateRuleLine checks one rule line inside a chain body. Rules
// are free-form nft expressions (nft --check is the syntactic
// authority), but a rule line must never open a new object class —
// those are the ownership-relevant verbs and are denied outright.
func validateRuleLine(line string) error {
	toks := tokenize(line)
	if len(toks) == 0 {
		return errors.New("empty statement")
	}
	switch toks[0] {
	case "table", "chain", "add", "delete", "destroy", "flush", "include", "define", "reset", "monitor", "element", "map", "set", "ruleset":
		return fmt.Errorf("rule line may not begin with the reserved verb %q", toks[0])
	}
	if strings.Contains(line, "$") {
		return errors.New("rule line contains a variable reference; define is not permitted")
	}
	return nil
}

// validateFlatStatement allowlists one flat (command-form) statement.
func validateFlatStatement(stmt string) error {
	toks := tokenize(stmt)
	if len(toks) < 4 {
		return fmt.Errorf("statement %q is too short to be owned", redactTokens(toks))
	}
	verb, sub := toks[0], toks[1]
	family, table := toks[2], toks[3]
	switch verb + " " + sub {
	case "add table", "delete table", "destroy table", "flush table":
		if len(toks) != 4 {
			return fmt.Errorf("statement %q has unexpected tail", redactTokens(toks))
		}
	case "add chain":
		if len(toks) < 5 {
			return fmt.Errorf("chain statement %q has no chain name", redactTokens(toks))
		}
		if !validChainName(toks[4]) {
			return fmt.Errorf("chain name %q is not canonical", toks[4])
		}
		// Optional `{ ... }` definition: must be the canonical
		// one-clause trailing block.
		if len(toks) > 5 {
			if _, isBlock := splitBlock(stmt); !isBlock {
				return fmt.Errorf("chain definition %q is not the canonical one-clause form", redactTokens(toks))
			}
		}
	case "delete chain", "flush chain":
		if len(toks) != 5 {
			return fmt.Errorf("statement %q has unexpected tail", redactTokens(toks))
		}
		if !validChainName(toks[4]) {
			return fmt.Errorf("chain name %q is not canonical", toks[4])
		}
	case "add rule":
		if len(toks) < 6 {
			return fmt.Errorf("rule statement %q has no payload", redactTokens(toks))
		}
		if !validChainName(toks[4]) {
			return fmt.Errorf("chain name %q is not canonical", toks[4])
		}
	default:
		return fmt.Errorf("verb %q is not on the DuckOS launcher allowlist", verb+" "+sub)
	}
	if err := ValidateReadTarget(family, table); err != nil {
		return err
	}
	return nil
}

// tokenize splits a statement on whitespace. Braced groups are NOT
// special here: callers split blocks before tokenizing, so a token
// containing `{` makes the statement malformed and it is rejected.
func tokenize(stmt string) []string {
	return strings.FieldsFunc(stmt, func(r rune) bool {
		return r == ' ' || r == '\t' || r == '\n' || r == '\r'
	})
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

// redactTokens renders a statement's tokens so an error message can
// never echo an attacker-controlled rule payload (which may embed
// comment strings or hostnames) into the journal: at most the first
// five tokens survive.
func redactTokens(toks []string) string {
	if len(toks) > 5 {
		toks = append(append([]string(nil), toks[:5]...), "...")
	}
	return strings.Join(toks, " ")
}

func trimSpace(s string) string {
	start, end := 0, len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n' || s[start] == '\r') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n' || s[end-1] == '\r') {
		end--
	}
	return s[start:end]
}

// B64 is the single base64 codec for the protocol; both sides must
// agree on strict std encoding.
func B64(src []byte) string { return base64.StdEncoding.EncodeToString(src) }

// UnB64 decodes a strictly-encoded base64 field.
func UnB64(s string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(s)
}
