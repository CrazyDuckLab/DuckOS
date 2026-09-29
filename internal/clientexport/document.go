// Package clientexport owns bounded secret-bearing client export documents.
// Renderers produce one payload; QR, clipboard and download cannot independently
// regenerate divergent configurations. This package never writes public assets.
package clientexport

import (
	"errors"
	"log/slog"
	"regexp"
	"unicode/utf8"
)

// Byte-mode QR at medium error correction is bounded to version 40 capacity.
// Oversized configurations are refused rather than truncated or linked to a
// different payload. The UI must use the same correction level and byte mode.
const MaxPayloadBytes = 2331

var objectID = regexp.MustCompile(`^[0-9a-f]{32}$`)
var filename = regexp.MustCompile(`^[a-z][a-z0-9-]{0,47}\.(conf|txt)$`)

type Document struct {
	id       string
	revision string
	kind     string
	file     string
	payload  string
}

func NewDocument(id, revision, kind, file, payload string) (*Document, error) {
	if !objectID.MatchString(id) || !objectID.MatchString(revision) ||
		(kind != "wireguard" && kind != "subscription") || !filename.MatchString(file) ||
		len(payload) == 0 || len(payload) > MaxPayloadBytes || !utf8.ValidString(payload) {
		return nil, errors.New("invalid or oversized client export")
	}
	return &Document{id: id, revision: revision, kind: kind, file: file, payload: payload}, nil
}

// Reveal is an explicit secret boundary for an already authorized transport.
// Returned values must not enter logs, caches, URLs or ordinary configuration.
func (d *Document) Reveal() string      { return d.payload }
func (d *Document) Filename() string    { return d.file }
func (d *Document) ID() string          { return d.id }
func (d *Document) Revision() string    { return d.revision }
func (d *Document) Kind() string        { return d.kind }
func (d *Document) Download() []byte    { return []byte(d.payload) }
func (d Document) String() string       { return "[secret client export]" }
func (d Document) GoString() string     { return d.String() }
func (d Document) LogValue() slog.Value { return slog.StringValue(d.String()) }
func (d Document) MarshalJSON() ([]byte, error) {
	return nil, errors.New("secret export requires explicit authorized serialization")
}
