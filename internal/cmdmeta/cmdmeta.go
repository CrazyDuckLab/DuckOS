// cmdmeta/cmdmeta.go: V1 "Show CLI / Show
// API" metadata registry. The package owns
// the typed contract that lets the future
// UI surface equivalent CLI/API operations
// for every first-class object.
//
// The runtime is a thin read-only
// metadata store; the package itself does
// not execute commands. Every entry binds
// a logical operation to its CLI snippet
// and its API request method/path.
package cmdmeta

import (
	"errors"
	"regexp"
	"sort"
	"sync"
)

// Operation is the logical name of an
// operation the UI/API exposes.
type Operation string

// OpKind is the operation class.
type OpKind string

const (
	OpShow     OpKind = "show"
	OpSet      OpKind = "set"
	OpDelete   OpKind = "delete"
	OpTest     OpKind = "test"
	OpApply    OpKind = "apply"
	OpRollback OpKind = "rollback"
	OpDiagnose OpKind = "diagnose"
)

// Valid reports whether k is known.
func (k OpKind) Valid() bool {
	switch k {
	case OpShow, OpSet, OpDelete, OpTest, OpApply, OpRollback, OpDiagnose:
		return true
	}
	return false
}

// Entry binds a logical Operation to its
// CLI snippet and API request. Both
// representations are deliberately
// template strings; the runtime is
// responsible for the actual argument
// substitution.
type Entry struct {
	Operation   Operation `json:"operation"`
	Object      string    `json:"object"`
	Kind        OpKind    `json:"kind"`
	Summary     string    `json:"summary"`
	CLITemplate string    `json:"cliTemplate"`
	APIMethod   string    `json:"apiMethod"`
	APIPath     string    `json:"apiPath"`
	// DryRunSupported indicates the operation
	// supports --dry-run semantics.
	DryRunSupported bool `json:"dryRunSupported"`
	// PreviewSupported indicates the operation
	// supports a preview/diff.
	PreviewSupported bool `json:"previewSupported"`
	// CommitConfirmed indicates the operation
	// requires commit-confirmed semantics.
	CommitConfirmed bool `json:"commitConfirmed"`
}

var opPattern = regexp.MustCompile(`^[a-z][a-z0-9._-]{1,63}$`)
var objPattern = regexp.MustCompile(`^[a-z][a-z0-9._-]{1,63}$`)

// Valid reports whether e is well-formed.
func (e Entry) Valid() error {
	if !opPattern.MatchString(string(e.Operation)) {
		return errors.New("cmdmeta: operation is not canonical")
	}
	if !objPattern.MatchString(e.Object) {
		return errors.New("cmdmeta: object is not canonical")
	}
	if !e.Kind.Valid() {
		return errors.New("cmdmeta: kind is unknown")
	}
	if e.Summary == "" {
		return errors.New("cmdmeta: summary is required")
	}
	if e.CLITemplate == "" && e.APIPath == "" {
		return errors.New("cmdmeta: cliTemplate or apiPath is required")
	}
	if e.APIPath != "" && e.APIMethod == "" {
		return errors.New("cmdmeta: apiMethod required when apiPath set")
	}
	return nil
}

// Registry is the typed metadata registry.
// Registry is goroutine-safe.
type Registry struct {
	mu      sync.Mutex
	entries map[Operation]Entry
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry { return &Registry{entries: map[Operation]Entry{}} }

// Register inserts or replaces an entry.
func (r *Registry) Register(e Entry) error {
	if err := e.Valid(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries[e.Operation] = e
	return nil
}

// Get returns the entry for an operation.
func (r *Registry) Get(op Operation) (Entry, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.entries[op]
	return e, ok
}

// ByObject returns every entry for the
// supplied object, sorted by operation.
func (r *Registry) ByObject(obj string) []Entry {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []Entry{}
	for _, e := range r.entries {
		if e.Object == obj {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, k int) bool { return out[i].Operation < out[k].Operation })
	return out
}

// List returns every entry sorted by
// operation.
func (r *Registry) List() []Entry {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Entry, 0, len(r.entries))
	for _, e := range r.entries {
		out = append(out, e)
	}
	sort.Slice(out, func(i, k int) bool { return out[i].Operation < out[k].Operation })
	return out
}

// RenderCLI substitutes the positional
// arguments in the entry's CLI template.
// The runtime is expected to validate the
// arguments before substitution.
func RenderCLI(e Entry, args map[string]string) (string, error) {
	out := e.CLITemplate
	for k, v := range args {
		out = replaceToken(out, k, v)
	}
	return out, nil
}

func replaceToken(s, key, value string) string {
	for {
		i := indexOf(s, "{{"+key+"}}")
		if i < 0 {
			return s
		}
		s = s[:i] + value + s[i+len("{{"+key+"}}"):]
	}
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
