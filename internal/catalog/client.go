// Package catalog owns indexed canonical client metadata, not runtime activation
// or secret bytes. The Engine resolves profiles and coordinates service changes.
package catalog

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/CrazyDuckLab/DuckOS/internal/vault"
)

var ErrUnavailable = errors.New("client catalog is unavailable")
var ErrInput = errors.New("invalid client metadata")
var ErrConflict = errors.New("client revision conflict")
var ErrNotFound = errors.New("client not found")
var ErrCapacity = errors.New("client catalog capacity exceeded")
var identifier = regexp.MustCompile(`^[0-9a-f]{32}$`)

const MaxClients = 1024
const maxDocumentBytes = 8192

type Client struct {
	ID         string           `json:"id"`
	Name       string           `json:"name"`
	Kind       string           `json:"kind"`
	ProfileID  string           `json:"profileId,omitempty"`
	Enabled    bool             `json:"enabled"`
	ExpiresAt  string           `json:"expiresAt,omitempty"`
	Credential *vault.Reference `json:"credential,omitempty"`
}

type Entry struct {
	Revision int64  `json:"revision"`
	Client   Client `json:"client"`
}

func (c Client) Validate() error {
	if !identifier.MatchString(c.ID) || !utf8.ValidString(c.Name) || utf8.RuneCountInString(c.Name) < 1 || utf8.RuneCountInString(c.Name) > 80 || strings.TrimSpace(c.Name) != c.Name || (c.Kind != "wireguard" && c.Kind != "subscription") {
		return ErrInput
	}
	for _, r := range c.Name {
		if unicode.IsControl(r) {
			return ErrInput
		}
	}
	if c.ProfileID != "" && !identifier.MatchString(c.ProfileID) {
		return ErrInput
	}
	if c.ExpiresAt != "" {
		t, err := time.Parse(time.RFC3339, c.ExpiresAt)
		if err != nil || t.UTC().Format(time.RFC3339) != c.ExpiresAt || t.Year() < 2020 || t.Year() > 9999 {
			return ErrInput
		}
	}
	if c.Credential != nil {
		r := c.Credential
		purpose := "wireguard-private"
		if c.Kind == "subscription" {
			purpose = "subscription-token"
		}
		if r.ID != c.ID || !identifier.MatchString(r.Revision) || !identifier.MatchString(r.KeyID) || r.Purpose != purpose {
			return ErrInput
		}
	}
	if c.Enabled && (c.ProfileID == "" || c.Credential == nil) {
		return ErrInput
	}
	return nil
}

// ExportEligible concerns desired metadata only; a provider must additionally
// establish verified service activation and resolve the referenced profile/key.
func (c Client) ExportEligible(now time.Time) bool {
	if c.Validate() != nil || !c.Enabled {
		return false
	}
	if c.ExpiresAt == "" {
		return true
	}
	expires, err := time.Parse(time.RFC3339, c.ExpiresAt)
	return err == nil && now.Before(expires)
}

func decodeClient(raw string) (Client, error) {
	var c Client
	if len(raw) == 0 || len(raw) > maxDocumentBytes {
		return c, ErrInput
	}
	d := json.NewDecoder(bytes.NewBufferString(raw))
	d.DisallowUnknownFields()
	if d.Decode(&c) != nil || d.Decode(new(any)) != io.EOF || c.Validate() != nil {
		return Client{}, ErrInput
	}
	canonical, err := json.Marshal(c)
	if err != nil || string(canonical) != raw {
		return Client{}, ErrInput
	}
	return c, nil
}
