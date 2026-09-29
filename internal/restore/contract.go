package restore

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"path"
	"strings"
)

// Request contains logical identities only. Roots are never part of the wire contract.
type Request struct {
	ID       string   `json:"requestId"`
	Expected string   `json:"expectedRevision"`
	Backup   string   `json:"backupId"`
	Keys     []string `json:"selectedKeys"`
}
type Result struct {
	ID      string `json:"requestId"`
	Outcome string `json:"outcome"`
}

func (r Request) Validate() error {
	id, err := hex.DecodeString(r.ID)
	if err != nil || len(id) != 16 || len(r.Expected) != 64 || len(r.Keys) == 0 || len(r.Keys) > 64 {
		return errors.New("invalid restore identity or selection")
	}
	if _, err := hex.DecodeString(r.Expected); err != nil {
		return errors.New("invalid expected revision")
	}
	if r.Backup == "" || strings.ContainsAny(r.Backup, "/\\") || r.Backup == "." || r.Backup == ".." || len(r.Backup) > 64 {
		return errors.New("invalid backup identity")
	}
	seen := map[string]bool{}
	for _, k := range r.Keys {
		if k == "" || k == "." || path.IsAbs(k) || path.Clean(k) != k || strings.Contains(k, "\\") || k == ".." || strings.HasPrefix(k, "../") || seen[k] || len(k) > 240 {
			return errors.New("invalid restore target")
		}
		seen[k] = true
	}
	return nil
}
func (r Request) Digest() string {
	b, _ := json.Marshal(r)
	d := sha256.Sum256(b)
	return hex.EncodeToString(d[:])
}
