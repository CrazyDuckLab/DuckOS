// Package backup owns the portable archive envelope the V1
// contract requires for backup and restore. The envelope is a
// tamper-evident, signature-gated, schema-versioned container
// that carries a manifest of every file it contains and a
// gzip-compressed tar payload.
//
// The package is split into:
//   - backup.go: the model, validation and bound contract.
//   - builder.go: the streaming builder that produces an envelope.
//   - inspector.go: the read-only inspector that lists an envelope.
//   - restorer.go: the dry-run-aware restore that extracts entries.
//
// The package never executes external commands. The whole
// pipeline is implemented in pure Go so the only attack surface
// is the manifest, the signature and the tar format itself.
package backup

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	// SchemaVersion is the manifest schema. The package rejects
	// any envelope whose schema version does not match.
	SchemaVersion = "1"

	// MagicHeader is the eight-byte preamble that identifies a
	// DuckOS backup envelope. The same magic is used for the
	// upgrade envelope to keep tooling simple.
	MagicHeader = "DUCKBK01"

	// MaxManifestBytes is the largest manifest the package will
	// accept. The bound is deliberately small: a backup is a
	// metadata-heavy object and the API cannot serve a manifest
	// that exceeds the response cap.
	MaxManifestBytes = 1 << 20 // 1 MiB

	// MaxEntries is the per-envelope entry cap. A backup with
	// more than this many entries is almost certainly a leak:
	// a real DuckOS install has well under ten thousand files
	// of operational state.
	MaxEntries = 16384

	// MaxPathBytes is the longest path the package will accept
	// in a manifest. The bound is the same as the upgrade
	// envelope: 256 bytes.
	MaxPathBytes = 256

	// MaxTotalBytes is the cumulative uncompressed byte cap. A
	// single backup that would expand beyond this limit is a
	// clear sign of a malformed or malicious archive.
	MaxTotalBytes = 8 << 30 // 8 GiB

	// MaxSingleFileBytes is the largest single file the package
	// will emit or accept.
	MaxSingleFileBytes = 256 << 20 // 256 MiB
)

// Kind enumerates the canonical entry kinds. The kind is part
// of the manifest and is part of the signature; an inspector
// will refuse an envelope whose entries have unknown kinds.
type Kind string

const (
	KindConfig   Kind = "config"   // duckos-owned configuration
	KindVault    Kind = "vault"    // encrypted vault records
	KindIdentity Kind = "identity" // identity / users / sessions
	KindNetwork  Kind = "network"  // network transaction journal
	KindState    Kind = "state"    // generic internal state
	KindAudit    Kind = "audit"    // bounded audit log
	KindLog      Kind = "log"      // bounded diagnostic log
	KindMeta     Kind = "meta"     // manifest-only entry, no payload
)

// isValidKind returns true for canonical kinds.
func isValidKind(k Kind) bool {
	switch k {
	case KindConfig, KindVault, KindIdentity, KindNetwork,
		KindState, KindAudit, KindLog, KindMeta:
		return true
	}
	return false
}

// Entry is one file in the manifest. The Path is relative to
// the archive root; the Size and Digest are computed at build
// time and verified at restore time.
type Entry struct {
	Path   string `json:"path"`
	Kind   Kind   `json:"kind"`
	Size   int64  `json:"size"`
	Digest string `json:"digest"` // hex sha256
	Mode   uint32 `json:"mode"`
	MTime  int64  `json:"mtime"`
	Owner  string `json:"owner,omitempty"`
}

// Valid enforces the per-entry contract.
func (e Entry) Valid() error {
	if !pathPattern.MatchString(e.Path) {
		return errors.New("backup: entry path is not canonical")
	}
	if hasPathTraversal(e.Path) {
		return errors.New("backup: entry path escapes the archive root")
	}
	if len(e.Path) > MaxPathBytes {
		return errors.New("backup: entry path exceeds size cap")
	}
	if !isValidKind(e.Kind) {
		return errors.New("backup: entry kind is not canonical")
	}
	if e.Size < 0 || e.Size > MaxSingleFileBytes {
		return errors.New("backup: entry size out of bounds")
	}
	if len(e.Digest) != 64 || !hexPattern.MatchString(e.Digest) {
		return errors.New("backup: entry digest is not a sha256 hex")
	}
	if e.Mode > 0o7777 {
		return errors.New("backup: entry mode out of bounds")
	}
	return nil
}

// hasPathTraversal rejects any path segment that would let the
// archive escape its root: "..", empty segments, absolute
// paths or doubled separators.
func hasPathTraversal(p string) bool {
	if p == "" {
		return true
	}
	if strings.HasPrefix(p, "/") {
		return true
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return true
		}
	}
	return false
}

var (
	pathPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._/\-]{0,254}$`)
	hexPattern  = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// Manifest is the signed, ordered list of every file in the
// envelope. The order is significant: the restore path walks
// the manifest in order and never sorts.
type Manifest struct {
	SchemaVersion string    `json:"schemaVersion"`
	CreatedAt     time.Time `json:"createdAt"`
	HostID        string    `json:"hostId,omitempty"`
	DuckOSVersion string    `json:"duckosVersion"`
	Reason        string    `json:"reason,omitempty"`
	Operator      string    `json:"operator,omitempty"`
	Entries       []Entry   `json:"entries"`
}

// Valid enforces the manifest contract.
func (m Manifest) Valid() error {
	if m.SchemaVersion != SchemaVersion {
		return errors.New("backup: schema version mismatch")
	}
	if m.CreatedAt.IsZero() {
		return errors.New("backup: createdAt is required")
	}
	if len(m.Entries) == 0 {
		return errors.New("backup: manifest is empty")
	}
	if len(m.Entries) > MaxEntries {
		return errors.New("backup: too many entries")
	}
	if m.Operator != "" && (len(m.Operator) > 64 || !userPattern.MatchString(m.Operator)) {
		return errors.New("backup: operator is not canonical")
	}
	if m.Reason != "" && len(m.Reason) > 256 {
		return errors.New("backup: reason exceeds size cap")
	}
	var total int64
	seen := make(map[string]struct{}, len(m.Entries))
	for _, e := range m.Entries {
		if err := e.Valid(); err != nil {
			return err
		}
		if _, dup := seen[e.Path]; dup {
			return errors.New("backup: duplicate entry path")
		}
		seen[e.Path] = struct{}{}
		total += e.Size
		if total > MaxTotalBytes {
			return errors.New("backup: total bytes exceed cap")
		}
	}
	return nil
}

var userPattern = regexp.MustCompile(`^[a-z][a-z0-9._@-]{0,63}$`)

// Envelope is the on-disk wire format. The Header is the
// eight-byte magic. The Signature is a detached Ed25519
// signature over the canonical manifest. The ManifestJSON is
// the canonical JSON encoding of Manifest. The Payload is the
// gzip-compressed tar stream that carries the file contents
// in manifest order.
type Envelope struct {
	Header       []byte   `json:"-"`
	Signature    []byte   `json:"signature"`
	ManifestJSON []byte   `json:"manifest"`
	Payload      []byte   `json:"payload"`
	Manifest     Manifest `json:"-"`
	PayloadSize  int64    `json:"-"`
}

// MaxEnvelopeBytes is the largest envelope the package will
// load into memory. The bound is well above MaxTotalBytes so a
// compressed payload is honoured, but a malicious archive that
// would explode into multi-gigabytes is refused at read time.
const MaxEnvelopeBytes = 16 << 30 // 16 GiB

// Build assembles a Manifest from a list of entries. The
// function is a pure-data constructor: it does not read or
// write files. The streaming builder in builder.go consumes
// the manifest and produces the payload.
func Build(entries []Entry, hostID, duckosVersion, reason, operator string, now time.Time) (Manifest, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	m := Manifest{
		SchemaVersion: SchemaVersion,
		CreatedAt:     now,
		HostID:        hostID,
		DuckOSVersion: duckosVersion,
		Reason:        reason,
		Operator:      operator,
		Entries:       entries,
	}
	if err := m.Valid(); err != nil {
		return Manifest{}, err
	}
	return m, nil
}

// EncodeManifest produces the canonical JSON encoding of the
// manifest. The encoding is sorted, compact and stable so a
// signature produced by a third party can be verified by the
// Agent without ambiguity.
func EncodeManifest(m Manifest) ([]byte, error) {
	if err := m.Valid(); err != nil {
		return nil, err
	}
	data, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	if len(data) > MaxManifestBytes {
		return nil, errors.New("backup: manifest exceeds size cap")
	}
	return data, nil
}

// DecodeManifest parses a JSON manifest. The function refuses
// partial or future-schema payloads before they can reach the
// verifier.
func DecodeManifest(data []byte) (Manifest, error) {
	if len(data) == 0 {
		return Manifest{}, errors.New("backup: empty manifest")
	}
	if len(data) > MaxManifestBytes {
		return Manifest{}, errors.New("backup: manifest exceeds size cap")
	}
	var m Manifest
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&m); err != nil {
		return Manifest{}, err
	}
	if err := m.Valid(); err != nil {
		return Manifest{}, err
	}
	return m, nil
}

// Digest computes the canonical hex sha256 of the supplied
// bytes. The function is exposed so callers can build entries
// without pulling in the crypto package.
func Digest(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
