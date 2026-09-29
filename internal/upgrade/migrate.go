// upgrade/migrate.go: V1 config-schema
// migration runner. The runtime applies a
// chain of Migrations in order, validates the
// post-migration configuration, and refuses
// to publish a broken migration.
//
// The runner is a thin layer over the typed
// Migration contract; it does not import any
// concrete schema. Callers (the R11 state
// store) supply the validators and the
// migration chain.
package upgrade

import (
	"errors"
	"regexp"
	"sort"
	"sync"
	"time"
)

// MigrationStep is a single named migration
// from a source schema version to a target
// schema version. The Apply function must be
// deterministic and idempotent: a re-run
// after a crash must yield the same
// configuration.
type MigrationStep struct {
	Name       string                                        `json:"name"`
	FromSchema int                                           `json:"fromSchema"`
	ToSchema   int                                           `json:"toSchema"`
	Apply      func(current []byte) (next []byte, err error) `json:"-"`
}

var migNamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:-]{0,63}$`)

// Valid reports whether m is well-formed.
func (m MigrationStep) Valid() error {
	if !migNamePattern.MatchString(m.Name) {
		return errors.New("upgrade: migration name is not canonical")
	}
	if m.FromSchema < 0 || m.ToSchema < 0 {
		return errors.New("upgrade: schema versions must be non-negative")
	}
	if m.ToSchema <= m.FromSchema {
		return errors.New("upgrade: ToSchema must exceed FromSchema")
	}
	if m.Apply == nil {
		return errors.New("upgrade: Apply function is required")
	}
	return nil
}

// MigrationChain is the ordered list of
// migrations. The chain is sorted by
// FromSchema ascending.
type MigrationChain struct {
	mu    sync.Mutex
	steps []MigrationStep
}

// NewMigrationChain returns an empty chain.
func NewMigrationChain() *MigrationChain {
	return &MigrationChain{}
}

// Add appends a migration. The chain rejects
// duplicate names and overlapping
// FromSchema..ToSchema ranges.
func (c *MigrationChain) Add(m MigrationStep) error {
	if err := m.Valid(); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, s := range c.steps {
		if s.Name == m.Name {
			return errors.New("upgrade: migration name already present")
		}
		if s.FromSchema < m.ToSchema && m.FromSchema < s.ToSchema {
			return errors.New("upgrade: migration range overlaps")
		}
	}
	c.steps = append(c.steps, m)
	return nil
}

// Plan describes the migration the runner
// must perform for a given source schema.
type MigrationPlan struct {
	FromSchema int
	ToSchema   int
	Steps      []MigrationStep
	Reason     string
}

// Plan computes the migration plan from
// current schema currentSchema to target
// targetSchema using the registered chain.
func (c *MigrationChain) Plan(currentSchema int, targetSchema int) (MigrationPlan, error) {
	if currentSchema < 0 || targetSchema < 0 {
		return MigrationPlan{}, errors.New("upgrade: schema versions must be non-negative")
	}
	if targetSchema == currentSchema {
		return MigrationPlan{FromSchema: currentSchema, ToSchema: targetSchema}, nil
	}
	if targetSchema < currentSchema {
		return MigrationPlan{}, errors.New("upgrade: down-migration is not supported")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	steps := append([]MigrationStep(nil), c.steps...)
	sort.Slice(steps, func(i, k int) bool { return steps[i].FromSchema < steps[k].FromSchema })
	out := []MigrationStep{}
	cursor := currentSchema
	for _, s := range steps {
		if s.FromSchema != cursor {
			continue
		}
		out = append(out, s)
		cursor = s.ToSchema
		if cursor == targetSchema {
			return MigrationPlan{FromSchema: currentSchema, ToSchema: targetSchema, Steps: out, Reason: "chain"}, nil
		}
	}
	return MigrationPlan{}, errors.New("upgrade: no migration path to target schema")
}

// RunResult is the outcome of a migration.
type RunResult struct {
	FromSchema int
	ToSchema   int
	Config     []byte
	Steps      []string
	Duration   time.Duration
}

// Run applies the plan to the supplied
// configuration. Each step's output is the
// next step's input. The validator is run
// after every step; the run is aborted on
// the first failure.
func Run(plan MigrationPlan, in []byte, validate func([]byte) error) (RunResult, error) {
	if plan.FromSchema == plan.ToSchema {
		return RunResult{FromSchema: plan.FromSchema, ToSchema: plan.ToSchema, Config: in, Steps: []string{}}, nil
	}
	started := time.Now()
	cur := append([]byte(nil), in...)
	steps := []string{}
	for _, s := range plan.Steps {
		next, err := s.Apply(cur)
		if err != nil {
			return RunResult{FromSchema: plan.FromSchema, ToSchema: plan.ToSchema, Config: cur, Steps: steps, Duration: time.Since(started)}, errors.New("upgrade: migration " + s.Name + " failed: " + err.Error())
		}
		if err := validate(next); err != nil {
			return RunResult{FromSchema: plan.FromSchema, ToSchema: plan.ToSchema, Config: cur, Steps: steps, Duration: time.Since(started)}, errors.New("upgrade: migration " + s.Name + " produced invalid config: " + err.Error())
		}
		cur = next
		steps = append(steps, s.Name)
	}
	return RunResult{FromSchema: plan.FromSchema, ToSchema: plan.ToSchema, Config: cur, Steps: steps, Duration: time.Since(started)}, nil
}
