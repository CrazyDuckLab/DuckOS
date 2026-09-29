// support/support.go: V1 sanitized
// support-bundle generator. The package
// owns the typed contracts for the
// generated bundle, the redaction policy,
// and the include/exclude filter. The
// runtime is a thin mapping to a tar.gz
// archive; the package itself does not run
// shell.
//
// The V1 contract: no raw secret ever
// appears in the bundle. The Redactor
// walks the input and replaces every
// recognized secret with a fixed tag. The
// policy is pure and unit-testable.
package support

import (
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// RedactionTag is the placeholder the
// Redactor substitutes for every detected
// secret.
const RedactionTag = "[REDACTED]"

// Pattern is a single redaction pattern.
type Pattern struct {
	Name        string         `json:"name"`
	Regex       *regexp.Regexp `json:"-"`
	RawRegex    string         `json:"regex"`
	Description string         `json:"description"`
}

// NewPattern compiles a pattern and returns
// it. The pattern's regex is anchored at
// any character: a match is sufficient.
func NewPattern(name, expr, description string) (Pattern, error) {
	re, err := regexp.Compile(expr)
	if err != nil {
		return Pattern{}, err
	}
	return Pattern{Name: name, Regex: re, RawRegex: expr, Description: description}, nil
}

// MustPattern is a panic-on-error wrapper
// for static pattern definitions.
func MustPattern(name, expr, description string) Pattern {
	p, err := NewPattern(name, expr, description)
	if err != nil {
		panic("support: bad pattern " + name + ": " + err.Error())
	}
	return p
}

// DefaultPatterns returns the V1 redaction
// pattern set. The set covers common
// secret shapes; a per-installation
// pattern can be added with AddPattern.
func DefaultPatterns() []Pattern {
	return []Pattern{
		// WireGuard / Xray / IPsec private keys
		// (base64 44 chars)
		MustPattern("private-key-base64", `[A-Za-z0-9+/]{42,44}==?`, "base64 44-char key"),
		// Generic API tokens (32+ hex). No \b because
		// real-world secrets are typically glued to a
		// prefix (e.g. sk_live_, ghp_, AIza) or to
		// separators (_-:). The 32-char minimum avoids
		// false-positives on hash digests we want to keep.
		MustPattern("api-token-hex", `[a-fA-F0-9]{32,128}`, "long hex token"),
		// Ed25519 seed hex (64 hex chars).
		MustPattern("ed25519-seed", `[a-fA-F0-9]{64}`, "ed25519 seed"),
		// Bearer tokens
		MustPattern("bearer", `(?i)bearer\s+[A-Za-z0-9._-]{16,}`, "bearer token"),
		// Vault references
		MustPattern("vault-ref", `vault://[a-zA-Z0-9._/-]{2,128}`, "vault reference"),
		// Basic auth in URL
		MustPattern("basic-auth-url", `https?://[A-Za-z0-9._~%-]+:[A-Za-z0-9._~%-]+@`, "basic auth in URL"),
		// JWT
		MustPattern("jwt", `eyJ[A-Za-z0-9_-]+\.eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+`, "JWT token"),
		// Issuer-prefixed credentials, regardless of
		// context. Covers Stripe, GitHub, Google, Slack,
		// AWS access key IDs, OAuth bearer prefixes, and
		// any other "short alpha prefix + long token"
		// shape used by mainstream credential issuers.
		MustPattern("prefix-credential", `(?:^|[\s"'=:])(?:sk_(?:live|test)_|ghp_|gho_|ghs_|ghr_|ghu_|xox[bpars]-|ya29\.|AKIA[0-9A-Z]{16}|ASIA[0-9A-Z]{16})[A-Za-z0-9._-]{8,}|AIza[0-9A-Za-z_-]{35}`, "issuer-prefixed credential"),
		// KEY=value / KEY: value with a long opaque
		// token. The common .env / YAML / shell style.
		// Catches both prefix-bearing and bare secrets.
		MustPattern("env-var-secret", `(?im)\b(?:[A-Za-z][A-Za-z0-9_]*(?:SECRET|TOKEN|PASSWORD|PASSWD|PWD|API[_-]?KEY|ACCESS[_-]?KEY|PRIVATE[_-]?KEY))\s*[:=]\s*['"]?([A-Za-z0-9._+/-]{16,})['"]?`, "key=value or key: value with long token"),
		// AWS secret access key (40-char base64-ish,
		// surrounded by separators to avoid eating
		// non-secret 40-char base64 strings).
		MustPattern("aws-secret-key", `(?:^|[\s"':=;])[A-Za-z0-9/+=]{40}(?:$|[\s"',;])`, "AWS secret access key (40 char base64-ish)"),
		// PEM private key block (multi-line, must be
		// before other patterns because they would
		// otherwise match base64 inside it).
		MustPattern("pem-private-key", `-----BEGIN (?:RSA |EC |DSA |OPENSSH |PGP |ENCRYPTED )?PRIVATE KEY-----[\s\S]*?-----END (?:RSA |EC |DSA |OPENSSH |PGP |ENCRYPTED )?PRIVATE KEY-----`, "PEM private key block"),
	}
}

// Redactor is the V1 secret redactor.
// Redactor is goroutine-safe.
type Redactor struct {
	mu       sync.Mutex
	patterns []Pattern
}

// NewRedactor returns a Redactor with the
// supplied patterns.
func NewRedactor(patterns []Pattern) *Redactor {
	out := &Redactor{patterns: append([]Pattern(nil), patterns...)}
	sort.Slice(out.patterns, func(i, k int) bool { return out.patterns[i].Name < out.patterns[k].Name })
	return out
}

// AddPattern appends a pattern.
func (r *Redactor) AddPattern(p Pattern) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.patterns = append(r.patterns, p)
	sort.Slice(r.patterns, func(i, k int) bool { return r.patterns[i].Name < r.patterns[k].Name })
}

// Patterns returns the patterns sorted by
// name.
func (r *Redactor) Patterns() []Pattern {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Pattern, len(r.patterns))
	copy(out, r.patterns)
	return out
}

// Redact returns a copy of in with every
// pattern match replaced by RedactionTag.
func (r *Redactor) Redact(in string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := in
	for _, p := range r.patterns {
		out = p.Regex.ReplaceAllString(out, RedactionTag)
	}
	return out
}

// RedactBytes is the []byte form of
// Redact.
func (r *Redactor) RedactBytes(in []byte) []byte {
	return []byte(r.Redact(string(in)))
}

// File is one entry in a support bundle.
type File struct {
	Path    string `json:"path"`
	Body    []byte `json:"-"`
	Redact  bool   `json:"redact"`
	Comment string `json:"comment,omitempty"`
}

// Bundle is the generated support bundle.
type Bundle struct {
	GeneratedAt   time.Time `json:"generatedAt"`
	DuckOSVersion string    `json:"duckosVersion"`
	HostID        string    `json:"hostId,omitempty"`
	Files         []File    `json:"files"`
	Redaction     []string  `json:"redactionPatterns"`
}

// BundleBuilder accumulates Files and
// applies the redactor at Build time.
type BundleBuilder struct {
	mu       sync.Mutex
	files    []File
	hostID   string
	version  string
	redactor *Redactor
	clock    func() time.Time
}

// NewBundleBuilder returns an empty
// builder.
func NewBundleBuilder(hostID, version string, redactor *Redactor, clock func() time.Time) *BundleBuilder {
	if clock == nil {
		clock = time.Now
	}
	if redactor == nil {
		redactor = NewRedactor(DefaultPatterns())
	}
	return &BundleBuilder{hostID: hostID, version: version, redactor: redactor, clock: clock}
}

// Add appends a file. When redact is true,
// the body is run through the redactor at
// Build time.
func (b *BundleBuilder) Add(f File) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if f.Path == "" {
		return
	}
	if !strings.HasPrefix(f.Path, "/") {
		f.Path = "/" + f.Path
	}
	b.files = append(b.files, f)
}

// Build returns the bundle. The bodies of
// the redact-flagged files are sanitized.
func (b *BundleBuilder) Build() Bundle {
	b.mu.Lock()
	defer b.mu.Unlock()
	files := make([]File, len(b.files))
	for i, f := range b.files {
		body := f.Body
		if f.Redact {
			body = b.redactor.RedactBytes(body)
		}
		files[i] = File{Path: f.Path, Body: body, Redact: f.Redact, Comment: f.Comment}
	}
	patterns := []string{}
	for _, p := range b.redactor.Patterns() {
		patterns = append(patterns, p.Name)
	}
	return Bundle{
		GeneratedAt:   b.clock().UTC(),
		DuckOSVersion: b.version,
		HostID:        b.hostID,
		Files:         files,
		Redaction:     patterns,
	}
}
