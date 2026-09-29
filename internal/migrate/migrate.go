// Package migrate owns the bounded migration and
// import surface the DuckOS V1 contract requires.
//
// The package is a parser and classifier. It
// never executes the source commands and never
// mutates host state. The user supplies a
// textual source (a MikroTik .rsc export, an
// OpenWrt UCI config, a Linux /etc/network/interfaces
// file or a Kerio-style .conf), the package
// auto-detects the format, classifies every
// significant line as recognized, convertible,
// attention or unsupported, and returns a
// deterministic, bounded plan the operator can
// review before any DuckOS object is created.
//
// The plan never modifies the source. The
// operator decides what to apply. The plan is
// a Contract, not a transaction: the Engine +
// Agent + Duck CLI are still the only side
// that creates DuckOS objects through the
// normal Safe Apply path.
package migrate

import (
	"bytes"
	"encoding/json"
	"errors"
	"regexp"
	"sort"
	"strings"
	"time"
)

// jsonMarshalStable is the single JSON encoder
// for the package.
func jsonMarshalStable(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func jsonNewDecoder(data []byte) *json.Decoder {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	return dec
}

// SchemaVersion is the wire/JSON schema version
// this model serialises.
const SchemaVersion = "1"

// Hard caps. The plan is bounded; the source
// is bounded; the line count is bounded. A V1
// install never needs to migrate a 100 MiB
// router config.
const (
	MaxSourceBytes = 1 << 20 // 1 MiB
	MaxLineLength  = 4096
	MaxItems       = 8192
	MaxNotes       = 256
	MaxPlanBytes   = 64 << 10
)

// FormatKind enumerates the supported source
// formats. The package refuses anything that
// does not match one of the canonical kinds.
type FormatKind string

const (
	FormatUnknown         FormatKind = "unknown"
	FormatMikroTik        FormatKind = "mikrotik"
	FormatOpenWrtUCI      FormatKind = "openwrt-uci"
	FormatLinuxInterfaces FormatKind = "linux-interfaces"
	FormatKerio           FormatKind = "kerio"
	FormatVyOS            FormatKind = "vyos"
)

func isValidFormat(k FormatKind) bool {
	switch k {
	case FormatUnknown, FormatMikroTik, FormatOpenWrtUCI, FormatLinuxInterfaces, FormatKerio, FormatVyOS:
		return true
	}
	return false
}

// Status enumerates the per-item classification.
// Recognized means the line maps directly to a
// canonical DuckOS object. Convertible means
// the line maps with caveats the operator must
// confirm. Attention means the line is
// important but requires a manual decision
// (e.g. an OpenWrt iptables rule with
// extension matches). Unsupported means the
// line cannot be translated.
type Status string

const (
	StatusRecognized  Status = "recognized"
	StatusConvertible Status = "convertible"
	StatusAttention   Status = "attention"
	StatusUnsupported Status = "unsupported"
)

func isValidStatus(s Status) bool {
	switch s {
	case StatusRecognized, StatusConvertible, StatusAttention, StatusUnsupported:
		return true
	}
	return false
}

// IsSuccess returns true for statuses that mean
// the line was successfully classified. The CLI
// uses the predicate to colour the row.
func IsSuccess(s Status) bool {
	return s == StatusRecognized || s == StatusConvertible
}

// Confidence is the auto-detection confidence
// band. The detector is conservative: an
// ambiguous source is reported as low.
type Confidence string

const (
	ConfidenceHigh   Confidence = "high"
	ConfidenceMedium Confidence = "medium"
	ConfidenceLow    Confidence = "low"
)

func isValidConfidence(c Confidence) bool {
	switch c {
	case ConfidenceHigh, ConfidenceMedium, ConfidenceLow:
		return true
	}
	return false
}

// Source is the bounded input the package
// accepts. The bytes are kept verbatim for the
// preview; the line count is bounded; the
// format hint is optional.
type Source struct {
	Filename   string     `json:"filename,omitempty"`
	Format     FormatKind `json:"format,omitempty"`
	Bytes      int        `json:"bytes"`
	Lines      int        `json:"lines"`
	Provenance string     `json:"provenance,omitempty"`
	// CapturedAt is the timestamp the source was
	// imported; the renderer uses it for the
	// preview footer.
	CapturedAt int64 `json:"capturedAt"`
}

// Item is the bounded record for a single
// significant line in the source. The Line
// field is the verbatim source line; Section
// is the optional logical block the line
// belongs to (UCI config, interface section,
// MikroTik menu). Note is a short human-
// readable explanation the operator sees in the
// preview.
type Item struct {
	Line    int    `json:"line"`
	Section string `json:"section,omitempty"`
	Raw     string `json:"raw"`
	Intent  string `json:"intent"`
	Status  Status `json:"status"`
	Target  string `json:"target,omitempty"`
	Note    string `json:"note,omitempty"`
}

// Valid enforces the per-item contract. The
// renderer refuses to emit an invalid item.
func (i Item) Valid() error {
	if i.Line < 1 {
		return errors.New("migrate: item line is not positive")
	}
	if i.Raw == "" {
		return errors.New("migrate: item raw is empty")
	}
	if len(i.Raw) > MaxLineLength {
		return errors.New("migrate: item raw is oversized")
	}
	if strings.ContainsAny(i.Raw, "\x00") {
		return errors.New("migrate: item raw contains NUL")
	}
	if i.Intent == "" {
		return errors.New("migrate: item intent is empty")
	}
	if !isValidStatus(i.Status) {
		return errors.New("migrate: item status is not canonical")
	}
	for _, f := range []string{i.Section, i.Intent, i.Target, i.Note} {
		if strings.ContainsAny(f, "\x00\n\r") {
			return errors.New("migrate: item field contains newline")
		}
	}
	return nil
}

// Plan is the bounded migration contract. The
// plan is what the API returns to the GUI/CLI;
// it is never a live mutation. The Plan carries
// the source metadata, the detected format, the
// per-item classification and a small notes
// list the operator should read before applying.
type Plan struct {
	SchemaVersion string     `json:"schemaVersion"`
	Source        Source     `json:"source"`
	Format        FormatKind `json:"format"`
	Confidence    Confidence `json:"confidence"`
	Items         []Item     `json:"items"`
	Notes         []string   `json:"notes,omitempty"`
	GeneratedAt   int64      `json:"generated"`
}

// Valid enforces the plan contract. The Plan
// is rejected at the model layer; the renderer
// never produces a Plan that does not validate.
func (p Plan) Valid() error {
	if p.SchemaVersion != SchemaVersion {
		return errors.New("migrate: plan schema version mismatch")
	}
	if !isValidFormat(p.Format) {
		return errors.New("migrate: plan format is not canonical")
	}
	if !isValidConfidence(p.Confidence) {
		return errors.New("migrate: plan confidence is not canonical")
	}
	if len(p.Items) > MaxItems {
		return errors.New("migrate: too many items")
	}
	for i, it := range p.Items {
		if err := it.Valid(); err != nil {
			return err
		}
		_ = i
	}
	if len(p.Notes) > MaxNotes {
		return errors.New("migrate: too many notes")
	}
	for _, n := range p.Notes {
		if n == "" || strings.ContainsAny(n, "\x00") {
			return errors.New("migrate: note is empty or contains NUL")
		}
	}
	return nil
}

// Encode serialises a Plan using the stable
// encoder. The output is bounded by MaxPlanBytes.
func (p Plan) Encode() ([]byte, error) {
	if err := p.Valid(); err != nil {
		return nil, err
	}
	data, err := jsonMarshalStable(p)
	if err != nil {
		return nil, err
	}
	if len(data) > MaxPlanBytes {
		return nil, errors.New("migrate: plan exceeds maximum serialised size")
	}
	return data, nil
}

// ParsePlan decodes a JSON payload produced by
// Encode. The decoder refuses unknown fields.
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

// Summary returns a deterministic, per-status
// summary. The summary is what `duck migrate
// status` prints by default.
type Summary struct {
	Format      FormatKind `json:"format"`
	Confidence  Confidence `json:"confidence"`
	Recognized  int        `json:"recognized"`
	Convertible int        `json:"convertible"`
	Attention   int        `json:"attention"`
	Unsupported int        `json:"unsupported"`
	Total       int        `json:"total"`
}

// Summarise returns the per-status counts of a
// Plan. The summary is sorted by line number so
// the CLI can render a deterministic table.
func Summarise(p Plan) Summary {
	s := Summary{Format: p.Format, Confidence: p.Confidence, Total: len(p.Items)}
	for _, it := range p.Items {
		switch it.Status {
		case StatusRecognized:
			s.Recognized++
		case StatusConvertible:
			s.Convertible++
		case StatusAttention:
			s.Attention++
		case StatusUnsupported:
			s.Unsupported++
		}
	}
	return s
}

// SortItemsByLine returns a deterministic, line-
// sorted copy of the items. The Plan keeps the
// original order; the CLI uses the sorted copy
// for the per-item table.
func SortItemsByLine(items []Item) []Item {
	out := make([]Item, len(items))
	copy(out, items)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Line != out[j].Line {
			return out[i].Line < out[j].Line
		}
		return out[i].Raw < out[j].Raw
	})
	return out
}

// trimLine returns a bounded single-line view
// of a source line. Whitespace runs are
// collapsed to a single space and control
// characters are dropped.
func trimLine(s string) string {
	if len(s) > MaxLineLength {
		s = s[:MaxLineLength] + "..."
	}
	var b strings.Builder
	b.Grow(len(s))
	inSpace := true
	for _, r := range s {
		switch r {
		case '\n', '\r':
			continue
		case ' ', '\t':
			if !inSpace {
				b.WriteByte(' ')
				inSpace = true
			}
		default:
			if r < 32 {
				continue
			}
			b.WriteRune(r)
			inSpace = false
		}
	}
	return strings.TrimSpace(b.String())
}

// nowUnix returns the unix timestamp of the
// supplied time. The caller passes the current
// time at Plan construction; the helper exists
// so the tests can pin the value.
func nowUnix(t time.Time) int64 {
	if t.IsZero() {
		return time.Now().UTC().Unix()
	}
	return t.Unix()
}

// DetectFormat inspects the supplied bytes and
// returns the most likely format together with
// a confidence band. The detector is conservative:
// an unknown source returns FormatUnknown +
// ConfidenceLow so the operator is never misled.
func DetectFormat(data []byte) (FormatKind, Confidence) {
	if len(data) == 0 {
		return FormatUnknown, ConfidenceLow
	}
	text := string(data)
	// MikroTik .rsc: a sequence of /ip/...,
	// /interface/..., /queue/... lines. The
	// canonical export is line-based and uses
	// add name=... and set name=... keywords.
	if mikrotikPattern.MatchString(text) {
		return FormatMikroTik, ConfidenceHigh
	}
	// OpenWrt UCI: a sequence of
	// `config <type> '<name>'` blocks followed
	// by `option <key> <value>` lines.
	if openWrtPattern.MatchString(text) {
		return FormatOpenWrtUCI, ConfidenceHigh
	}
	// Linux /etc/network/interfaces: a sequence
	// of `iface <name> <family>` and
	// `auto <name>` lines.
	if linuxInterfacesPattern.MatchString(text) {
		return FormatLinuxInterfaces, ConfidenceHigh
	}
	// Kerio-style: a sequence of
	// `[Section]` headers with `Variable = Value`
	// lines.
	if kerioPattern.MatchString(text) {
		return FormatKerio, ConfidenceMedium
	}
	// VyOS-style: a sequence of section
	// headers `name { ... }` with indented
	// `key value` lines.
	if vyosPattern.MatchString(text) {
		return FormatVyOS, ConfidenceHigh
	}
	return FormatUnknown, ConfidenceLow
}

// Patterns used by the detector. The patterns
// are deliberately loose: the classifier
// (analyse.go) does the heavy lifting.
var (
	mikrotikPattern        = regexp.MustCompile(`(?m)^/(?:ip|interface|queue|system|routing|firewall|tool)\b`)
	openWrtPattern         = regexp.MustCompile(`(?m)^config\s+[a-zA-Z_][a-zA-Z0-9_-]*\s+'[^']+'`)
	linuxInterfacesPattern = regexp.MustCompile(`(?m)^(iface|auto|allow-|mapping)\s+[a-zA-Z0-9_:-]+`)
	kerioPattern           = regexp.MustCompile(`(?m)^\[[A-Za-z][A-Za-z0-9 _-]{0,32}\]\s*$`)
	vyosPattern            = regexp.MustCompile(`(?m)^[a-zA-Z][a-zA-Z0-9_-]*\s*\{`)
)
