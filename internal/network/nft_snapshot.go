package network

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
)

const maxNFTSnapshotBytes = 512 << 10

// NFTSnapshot is kernel readback of the exclusively enrolled inet/duckos table.
// It belongs in the private Agent recovery journal, never in mutation input.
// Empty Objects means the table was absent, not that observation failed.
type NFTSnapshot struct {
	Objects []json.RawMessage `json:"objects"`
}

func DecodeNFTSnapshot(raw []byte) (NFTSnapshot, error) {
	var wire struct {
		NFTables *[]json.RawMessage `json:"nftables"`
	}
	if len(raw) > maxNFTSnapshotBytes {
		return NFTSnapshot{}, errors.New("nft snapshot exceeds limit")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&wire) != nil || d.Decode(new(any)) != io.EOF || wire.NFTables == nil {
		return NFTSnapshot{}, errors.New("invalid nft snapshot")
	}
	s := NFTSnapshot{Objects: []json.RawMessage{}}
	for _, rawObject := range *wire.NFTables {
		var object map[string]json.RawMessage
		if json.Unmarshal(rawObject, &object) != nil || len(object) != 1 {
			return NFTSnapshot{}, errors.New("invalid nft object")
		}
		if _, ok := object["metainfo"]; ok {
			continue
		}
		s.Objects = append(s.Objects, append(json.RawMessage(nil), rawObject...))
	}
	return s, s.Validate()
}

func (s NFTSnapshot) Validate() error {
	if len(s.Objects) > 4096 {
		return errors.New("too many nft objects")
	}
	tables := 0
	total := 0
	for _, raw := range s.Objects {
		total += len(raw)
		if total > maxNFTSnapshotBytes {
			return errors.New("nft snapshot exceeds limit")
		}
		var object map[string]json.RawMessage
		if json.Unmarshal(raw, &object) != nil || len(object) != 1 {
			return errors.New("invalid nft object")
		}
		for kind, body := range object {
			if kind != "table" && kind != "chain" && kind != "rule" {
				return errors.New("unsupported nft recovery object")
			}
			var scope struct {
				Family string `json:"family"`
				Name   string `json:"name"`
				Table  string `json:"table"`
			}
			if json.Unmarshal(body, &scope) != nil || scope.Family != "inet" {
				return errors.New("nft object outside owned family")
			}
			if kind == "table" {
				if scope.Name != "duckos" {
					return errors.New("nft table outside owned scope")
				}
				tables++
			} else if scope.Table != "duckos" {
				return errors.New("nft object outside owned table")
			}
		}
	}
	if (len(s.Objects) > 0 && tables != 1) || tables > 1 {
		return errors.New("ambiguous nft table snapshot")
	}
	return nil
}

// Fingerprint ignores kernel-assigned handles and changing packet counters,
// while retaining rule order and every other observed expression/attribute.
func (s NFTSnapshot) Fingerprint() (string, error) {
	objects, err := s.stableObjects()
	if err != nil {
		return "", err
	}
	b, err := json.Marshal(objects)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:]), nil
}

func (s NFTSnapshot) stableObjects() ([]any, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}
	objects := make([]any, 0, len(s.Objects))
	for _, raw := range s.Objects {
		var object map[string]map[string]any
		d := json.NewDecoder(bytes.NewReader(raw))
		d.UseNumber()
		if err := d.Decode(&object); err != nil {
			return nil, err
		}
		for kind, body := range object {
			delete(body, "handle")
			if kind == "rule" {
				if expressions, ok := body["expr"].([]any); ok {
					for _, expression := range expressions {
						if expr, ok := expression.(map[string]any); ok {
							if counter, ok := expr["counter"].(map[string]any); ok {
								counter["packets"] = json.Number("0")
								counter["bytes"] = json.Number("0")
							}
						}
					}
				}
			}
		}
		objects = append(objects, object)
	}
	return objects, nil
}

// RestoreBatch is accepted only from captured private recovery evidence. It
// cannot address any other table or emit command wrappers from a client.
func (s NFTSnapshot) RestoreBatch() ([]byte, error) {
	objects, err := s.stableObjects()
	if err != nil {
		return nil, err
	}
	commands := []any{map[string]any{"destroy": map[string]any{"table": map[string]string{"family": "inet", "name": "duckos"}}}}
	for _, object := range objects {
		commands = append(commands, map[string]any{"add": object})
	}
	return json.Marshal(map[string]any{"nftables": commands})
}

// DeleteBatch emits nft delete commands for every object in the snapshot
// without destroying the surrounding table. It is the recovery surface
// the Safe Apply rollback path uses to remove the duckos_apply_guard
// chain once the host kernel has been restored — destroying the whole
// table would also wipe any filter rules the operator added out-of-band
// between the failed apply and the rollback, so a targeted delete is the
// narrowest production mechanism available.
func (s NFTSnapshot) DeleteBatch() ([]byte, error) {
	objects, err := s.stableObjects()
	if err != nil {
		return nil, err
	}
	commands := []any{}
	for _, object := range objects {
		commands = append(commands, map[string]any{"delete": object})
	}
	return json.Marshal(map[string]any{"nftables": commands})
}
