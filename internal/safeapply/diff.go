// safeapply/diff.go: V1 unified diff between
// two canonical configurations. The diff is
// what the operator sees before Confirm:
// which objects are added, removed, modified,
// and which dependencies each change touches.
//
// The diff is a small, JSON-serialisable
// structure; the human-readable rendering is
// the API/CLI's job.
package safeapply

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
)

// ChangeKind is the kind of object delta.
type ChangeKind string

const (
	ChangeAdded    ChangeKind = "added"
	ChangeRemoved  ChangeKind = "removed"
	ChangeModified ChangeKind = "modified"
)

// DiffObject is a single object delta.
type DiffObject struct {
	Kind string     `json:"kind"`
	ID   string     `json:"id"`
	Op   ChangeKind `json:"op"`
	// Before / After are the canonical JSON
	// for the object as it was / will be. The
	// CLI renders the diff in a stable
	// representation; the structure here is
	// machine-readable.
	Before json.RawMessage `json:"before,omitempty"`
	After  json.RawMessage `json:"after,omitempty"`
}

// Impact is a high-level summary of what the
// change affects: which services restart, which
// sessions are torn down, and the management
// risk assessment.
type Impact struct {
	RestartServices   []string `json:"restartServices"`
	TearsSessions     bool     `json:"tearsSessions"`
	ManagementRisk    string   `json:"managementRisk"`
	AffectsCheckpoint bool     `json:"affectsCheckpoint"`
	Notes             []string `json:"notes,omitempty"`
}

// Diff is the full preview of a candidate.
type Diff struct {
	Objects []DiffObject `json:"objects"`
	Impact  Impact       `json:"impact"`
	// Fingerprint is a stable 32-char hex of
	// (before, after); the API surface uses
	// it to dedupe identical previews.
	Fingerprint string `json:"fingerprint"`
}

// Diff computes the object-level diff between
// before and after. The inputs are canonical
// JSON objects; the comparison is at the
// (kind,id) key level.
//
// before / after are maps of "kind/id" ->
// canonical JSON. Missing keys are
// added/removed; equal keys are skipped;
// differing keys are modified.
func ComputeDiff(before, after map[string]json.RawMessage, restartServices []string, tearsSessions bool, managementRisk string) Diff {
	keys := map[string]struct{}{}
	for k := range before {
		keys[k] = struct{}{}
	}
	for k := range after {
		keys[k] = struct{}{}
	}
	sorted := make([]string, 0, len(keys))
	for k := range keys {
		sorted = append(sorted, k)
	}
	sort.Strings(sorted)
	out := Diff{Objects: []DiffObject{}}
	for _, k := range sorted {
		b, bOK := before[k]
		a, aOK := after[k]
		switch {
		case !bOK && aOK:
			out.Objects = append(out.Objects, DiffObject{Kind: splitKind(k), ID: splitID(k), Op: ChangeAdded, After: a})
		case bOK && !aOK:
			out.Objects = append(out.Objects, DiffObject{Kind: splitKind(k), ID: splitID(k), Op: ChangeRemoved, Before: b})
		case !bytesEqual(b, a):
			out.Objects = append(out.Objects, DiffObject{Kind: splitKind(k), ID: splitID(k), Op: ChangeModified, Before: b, After: a})
		}
	}
	// Roll-forward fingerprint.
	sum := sha256.New()
	for _, o := range out.Objects {
		sum.Write([]byte(o.Kind))
		sum.Write([]byte{'/'})
		sum.Write([]byte(o.ID))
		sum.Write([]byte{byte(0)})
		sum.Write([]byte(o.Op))
		sum.Write(o.Before)
		sum.Write(o.After)
		sum.Write([]byte{byte(0)})
	}
	out.Fingerprint = hex.EncodeToString(sum.Sum(nil)[:16])
	out.Impact = Impact{
		RestartServices: append([]string(nil), restartServices...),
		TearsSessions:   tearsSessions,
		ManagementRisk:  managementRisk,
	}
	return out
}

func splitKind(k string) string {
	for i := 0; i < len(k); i++ {
		if k[i] == '/' {
			return k[:i]
		}
	}
	return k
}

func splitID(k string) string {
	for i := 0; i < len(k); i++ {
		if k[i] == '/' {
			return k[i+1:]
		}
	}
	return ""
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
