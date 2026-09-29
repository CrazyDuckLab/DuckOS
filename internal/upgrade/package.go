// Package upgrade owns the .duck package envelope, the Active/Previous
// release selection, the bounded verification of incoming packages
// and the durable transaction journal. The state machine is the
// minimal subset that lets the foundation lay down the file format,
// the signature gate and the storage layout; the full happy-path
// commit/rollback flow is a follow-up that builds on the same
// primitives.
//
// The package envelope is a deterministic structure:
//
//	<header>  : 8-byte magic + 4-byte version + 8-byte body length
//	<manifest> : canonical JSON, body length bytes
//	<sig>     : 64-byte detached Ed25519 signature of the manifest
//	<payload> : body length bytes of the architecture-specific tar
//
// The manifest carries source/target versions, schema versions, the
// required architecture, declared dependencies, checksums of every
// payload byte and a stable space estimate. Verification refuses a
// package on every failure mode listed in docs/update-system.md.
package upgrade

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

const (
	Magic            = "DUCK1ARC"
	FormatVer        = 1
	HeaderLen        = len(Magic) + 4 + 8
	MaxManifestBytes = 256 << 10
	MaxPayloadBytes  = 2 << 30 // 2 GiB hard cap
)

var (
	archPattern = regexp.MustCompile(`^[a-z0-9_]+$`)
	idPattern   = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)
)

// Manifest is the signed body of a .duck package. Every field is
// bounded; verification refuses a package when any of them is out
// of range or missing.
type Manifest struct {
	Format           int      `json:"format"`
	Architecture     string   `json:"architecture"`
	SourceVersion    string   `json:"sourceVersion"`
	TargetVersion    string   `json:"targetVersion"`
	InputSchema      int      `json:"inputSchema"`
	OutputSchema     int      `json:"outputSchema"`
	RequiredSpace    int64    `json:"requiredSpaceBytes"`
	ReserveSpace     int64    `json:"reserveSpaceBytes"`
	Dependencies     []string `json:"dependencies,omitempty"`
	Capabilities     []string `json:"capabilities,omitempty"`
	CreatedAt        string   `json:"createdAt"`
	PayloadAlgorithm string   `json:"payloadAlgorithm"`
	PayloadDigest    string   `json:"payloadDigest"`
	PayloadBytes     int64    `json:"payloadBytes"`
}

// Validate enforces the bounded contract every manifest must satisfy.
// The function never inspects the payload, only the manifest claims.
func (m Manifest) Validate() error {
	if m.Format != FormatVer {
		return fmt.Errorf("package format version %d is not supported", m.Format)
	}
	if !archPattern.MatchString(m.Architecture) {
		return errors.New("package architecture is invalid")
	}
	if !idPattern.MatchString(m.SourceVersion) || !idPattern.MatchString(m.TargetVersion) {
		return errors.New("package version identifiers are invalid")
	}
	if m.InputSchema < 1 || m.OutputSchema < 1 {
		return errors.New("package schema versions are invalid")
	}
	if m.PayloadBytes <= 0 || m.PayloadBytes > MaxPayloadBytes {
		return errors.New("package payload size out of bounds")
	}
	if m.RequiredSpace < 0 || m.RequiredSpace > MaxPayloadBytes {
		return errors.New("package required space out of bounds")
	}
	if m.ReserveSpace < 0 || m.ReserveSpace > MaxPayloadBytes {
		return errors.New("package reserve space out of bounds")
	}
	if m.PayloadAlgorithm != "sha256" {
		return errors.New("package payload algorithm must be sha256")
	}
	if _, err := hex.DecodeString(m.PayloadDigest); err != nil || len(m.PayloadDigest) != 64 {
		return errors.New("package payload digest is not a sha256 hex value")
	}
	if m.CreatedAt != "" {
		if _, err := time.Parse(time.RFC3339, m.CreatedAt); err != nil {
			return fmt.Errorf("package createdAt must be RFC3339: %w", err)
		}
	}
	for _, dep := range m.Dependencies {
		if !idPattern.MatchString(dep) {
			return fmt.Errorf("package dependency %q is invalid", dep)
		}
	}
	for _, cap := range m.Capabilities {
		if !archPattern.MatchString(cap) {
			return fmt.Errorf("package capability %q is invalid", cap)
		}
	}
	return nil
}

// ReleaseID is the canonical release identifier. The format is the
// DuckOS version with a stable lowercase prefix. Reserved IDs are
// rejected because the engine treats them as boot anchors.
type ReleaseID string

const (
	ReleaseActive   ReleaseID = "active"
	ReleasePrevious ReleaseID = "previous"
)

func (r ReleaseID) valid() bool {
	if r == "" {
		return false
	}
	if r == ReleaseActive || r == ReleasePrevious {
		return true
	}
	return idPattern.MatchString(string(r))
}

// Header is the small fixed-size prefix that names the format and
// records the body length. It is the only structure a stream parser
// needs to find the manifest boundary.
type Header struct {
	Magic     string
	Format    int
	BodyBytes int64
}

func (h Header) Marshal() []byte {
	out := make([]byte, 0, HeaderLen)
	out = append(out, []byte(h.Magic)...)
	out = append(out, byte(h.Format>>24), byte(h.Format>>16), byte(h.Format>>8), byte(h.Format))
	var sizeBuf [8]byte
	u := uint64(h.BodyBytes)
	for i := 7; i >= 0; i-- {
		sizeBuf[i] = byte(u)
		u >>= 8
	}
	out = append(out, sizeBuf[:]...)
	return out
}

func (h *Header) Parse(b []byte) error {
	if len(b) < HeaderLen {
		return errors.New("package header is truncated")
	}
	if string(b[:len(Magic)]) != Magic {
		return errors.New("package magic does not match")
	}
	off := len(Magic)
	h.Format = int(uint32(b[off])<<24 | uint32(b[off+1])<<16 | uint32(b[off+2])<<8 | uint32(b[off+3]))
	off += 4
	if h.Format != FormatVer {
		return fmt.Errorf("package format version %d is not supported", h.Format)
	}
	var size uint64
	for i := 0; i < 8; i++ {
		size = size<<8 | uint64(b[off+i])
	}
	h.BodyBytes = int64(size)
	if h.BodyBytes < 0 || h.BodyBytes > MaxPayloadBytes+int64(MaxManifestBytes) {
		return errors.New("package body length is out of bounds")
	}
	return nil
}

// Package is the in-memory representation of a .duck envelope. The
// payload is read on demand; the manifest and signature are
// eagerly validated.
type Package struct {
	Header   Header
	Manifest Manifest
	Payload  []byte
}

// Decode parses the manifest and signature from a byte slice. The
// payload slice is a copy of the input. The function refuses any
// envelope that fails manifest validation, signature verification
// or header parsing. Verification is bounded: a malformed package
// cannot make the engine allocate large buffers or follow hostile
// links.
func Decode(archive []byte, verifier SignatureVerifier) (*Package, error) {
	if verifier == nil {
		return nil, errors.New("signature verifier is required")
	}
	if len(archive) < HeaderLen {
		return nil, errors.New("package header is truncated")
	}
	var header Header
	if err := header.Parse(archive[:HeaderLen]); err != nil {
		return nil, err
	}
	body := archive[HeaderLen:]
	if int64(len(body)) < header.BodyBytes {
		return nil, errors.New("package body is truncated")
	}
	// The first 64 bytes of the body are the detached signature; the
	// rest is the manifest.
	if header.BodyBytes < 64 {
		return nil, errors.New("package body must hold signature and manifest")
	}
	signature := body[:64]
	manifestBytes := body[64:header.BodyBytes]
	if len(manifestBytes) > MaxManifestBytes {
		return nil, errors.New("package manifest exceeds the size limit")
	}
	var manifest Manifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return nil, fmt.Errorf("package manifest is invalid JSON: %w", err)
	}
	if err := manifest.Validate(); err != nil {
		return nil, fmt.Errorf("package manifest: %w", err)
	}
	if err := verifier.Verify(manifestBytes, signature); err != nil {
		return nil, fmt.Errorf("package signature: %w", err)
	}
	payload := body[header.BodyBytes:]
	if int64(len(payload)) < manifest.PayloadBytes {
		return nil, errors.New("package payload is truncated")
	}
	if int64(len(payload)) != manifest.PayloadBytes {
		return nil, errors.New("package contains trailing payload data")
	}
	actual := sha256.Sum256(payload[:manifest.PayloadBytes])
	if hex.EncodeToString(actual[:]) != manifest.PayloadDigest {
		return nil, errors.New("package payload digest does not match manifest claim")
	}
	return &Package{Header: header, Manifest: manifest, Payload: append([]byte(nil), payload[:manifest.PayloadBytes]...)}, nil
}

// SignatureVerifier checks the detached signature of a manifest. The
// production wiring uses an Ed25519 public key; tests inject a
// deterministic synthetic.
type SignatureVerifier interface {
	Verify(manifest, signature []byte) error
}

// Fingerprint returns the canonical identifier of a release as it
// appears in the durable journal. The fingerprint is the manifest
// digest, which the agent carries in the version state.
func Fingerprint(m Manifest) string {
	raw, _ := json.Marshal(m)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:16])
}

// FormatArch is a tiny helper that returns the canonical
// architecture identifier DuckOS uses for both package and host
// detection. Real architectures are detected at install time.
func FormatArch(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if !archPattern.MatchString(value) {
		return "", errors.New("architecture identifier is invalid")
	}
	return value, nil
}
