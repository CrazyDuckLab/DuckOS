//go:build linux

package qos

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"sync"
	"syscall"
	"time"
)

var ErrRecoveryRequired = errors.New("qos: recovery_required")

type TCObserver interface {
	Executor
	Observe(context.Context, string) (json.RawMessage, error)
}

// Observe retains configuration, not counters/refcounts that change while
// traffic flows. All qdiscs/classes/filters are included in the ownership proof.
func (e *ExecRunner) Observe(ctx context.Context, link string) (json.RawMessage, error) {
	if !linkPattern.MatchString(link) {
		return nil, errors.New("qos: invalid observation link")
	}
	state := map[string][]map[string]any{}
	for _, kind := range []string{"qdisc", "class", "filter"} {
		cmd := exec.CommandContext(ctx, e.Binary, "-j", kind, "show", "dev", link)
		var out bytes.Buffer
		cmd.Stdout = &out
		if err := cmd.Run(); err != nil {
			return nil, fmt.Errorf("qos: %s observation failed", kind)
		}
		if out.Len() > 1<<20 {
			return nil, errors.New("qos: observation exceeds limit")
		}
		var rows []map[string]any
		if err := json.Unmarshal(out.Bytes(), &rows); err != nil {
			return nil, errors.New("qos: invalid tc observation")
		}
		for _, row := range rows {
			delete(row, "refcnt")
			delete(row, "stats")
			delete(row, "stats2")
		}
		state[kind] = rows
	}
	return json.Marshal(state)
}

type ownedPending struct {
	Before   map[string]json.RawMessage `json:"before"`
	Observed map[string]json.RawMessage `json:"observed"`
	Touched  map[string]bool            `json:"touched"`
	Target   Plan                       `json:"target"`
	InFlight bool                       `json:"inFlight"`
}
type ownedJournal struct {
	Version          int                        `json:"version"`
	Current          Plan                       `json:"current"`
	Previous         Plan                       `json:"previous"`
	Live             map[string]json.RawMessage `json:"live"`
	Pending          *ownedPending              `json:"pending,omitempty"`
	RecoveryRequired bool                       `json:"recoveryRequired"`
}

// OwnedRunner refuses to replace an external qdisc. Every runtime operation
// has a durable before-image and an ownership witness. An interrupted command
// is deliberately recovery_required: restart must not guess who owns a qdisc.
type OwnedRunner struct {
	mu      sync.Mutex
	path    string
	lock    *os.File
	exec    TCObserver
	journal ownedJournal
}

func OpenOwnedRunner(dir string, executor TCObserver) (*OwnedRunner, error) {
	if dir == "" {
		return nil, errors.New("qos: journal directory required")
	}
	if executor == nil {
		executor = NewExecRunner("/sbin/tc")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	lock, err := os.OpenFile(filepath.Join(dir, "writer.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		lock.Close()
		return nil, err
	}
	r := &OwnedRunner{path: filepath.Join(dir, "transaction.json"), lock: lock, exec: executor, journal: ownedJournal{Version: 1, Current: Plan{SchemaVersion: SchemaVersion}, Previous: Plan{SchemaVersion: SchemaVersion}, Live: map[string]json.RawMessage{}}}
	ok := false
	defer func() {
		if !ok {
			r.Close()
		}
	}()
	f, err := os.Open(r.path)
	if err == nil {
		raw, readErr := io.ReadAll(io.LimitReader(f, (4<<20)+1))
		f.Close()
		if readErr != nil || len(raw) > 4<<20 || json.Unmarshal(raw, &r.journal) != nil || r.journal.Version != 1 || r.journal.Current.Valid() != nil || r.journal.Previous.Valid() != nil {
			return nil, errors.New("qos: corrupt durable journal")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if r.journal.Pending != nil {
		// Observe actual state even when rollback cannot be safely inferred.
		for link := range r.journal.Pending.Before {
			if _, err := executor.Observe(ctx, link); err != nil {
				r.journal.RecoveryRequired = true
			}
		}
		r.journal.RecoveryRequired = true
	} else {
		for link, want := range r.journal.Live {
			got, err := executor.Observe(ctx, link)
			if err != nil || !bytes.Equal(got, want) {
				r.journal.RecoveryRequired = true
			}
		}
	}
	if err := r.save(); err != nil {
		return nil, err
	}
	ok = true
	return r, nil
}

func (r *OwnedRunner) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.lock == nil {
		return nil
	}
	err := r.lock.Close()
	r.lock = nil
	return err
}
func (r *OwnedRunner) Plans() (Plan, Plan) {
	r.mu.Lock()
	defer r.mu.Unlock()
	raw, _ := json.Marshal(r.journal)
	var copy ownedJournal
	_ = json.Unmarshal(raw, &copy)
	return copy.Current, copy.Previous
}
func (r *OwnedRunner) save() error {
	raw, err := json.Marshal(r.journal)
	if err != nil || len(raw) > 4<<20 {
		return errors.New("qos: journal exceeds limit")
	}
	f, err := os.CreateTemp(filepath.Dir(r.path), ".qos-")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if _, err = f.Write(raw); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = os.Rename(f.Name(), r.path); err != nil {
		return err
	}
	d, err := os.Open(filepath.Dir(r.path))
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}
func (r *OwnedRunner) recovery() error {
	r.journal.RecoveryRequired = true
	_ = r.save()
	return ErrRecoveryRequired
}

func baseline(raw json.RawMessage) bool {
	var state map[string][]map[string]any
	if json.Unmarshal(raw, &state) != nil || len(state["class"]) != 0 || len(state["filter"]) != 0 {
		return false
	}
	for _, q := range state["qdisc"] {
		if q["kind"] != "noqueue" || q["handle"] != "0:" {
			return false
		}
	}
	return true
}
func roots(plan Plan) map[string]Step {
	out := map[string]Step{}
	for _, s := range plan.Steps {
		if s.Kind == StepQdisc && s.Parent == "root" {
			out[s.Link] = s
		}
	}
	return out
}

func (r *OwnedRunner) Apply(ctx context.Context, p Plan) (ApplyOutcome, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.apply(ctx, p)
}
func (r *OwnedRunner) apply(ctx context.Context, p Plan) (out ApplyOutcome, err error) {
	out.StartedAt = time.Now().UTC()
	defer func() {
		out.EndedAt = time.Now().UTC()
		if err != nil {
			out.Error = err.Error()
		}
	}()
	if r.lock == nil || r.journal.RecoveryRequired || r.journal.Pending != nil {
		return out, ErrRecoveryRequired
	}
	if err = p.Valid(); err != nil {
		return out, err
	}
	wanted := roots(p)
	owned := roots(r.journal.Current)
	for _, s := range p.Steps {
		if _, ok := wanted[s.Link]; !ok {
			return out, errors.New("qos: each changed link requires an owned root")
		}
	}
	links := map[string]bool{}
	for link := range wanted {
		links[link] = true
	}
	for link := range owned {
		links[link] = true
	}
	if len(links) > 128 {
		return out, errors.New("qos: too many links")
	}
	before := map[string]json.RawMessage{}
	observed := map[string]json.RawMessage{}
	for link := range links {
		raw, e := r.exec.Observe(ctx, link)
		if e != nil {
			return out, e
		}
		if prior, ok := r.journal.Live[link]; ok {
			if !bytes.Equal(raw, prior) {
				return out, r.recovery()
			}
		} else if !baseline(raw) {
			return out, errors.New("qos: external tc state is not owned by DuckOS")
		}
		before[link] = raw
		observed[link] = raw
	}
	r.journal.Pending = &ownedPending{Before: before, Observed: observed, Touched: map[string]bool{}, Target: p}
	if err = r.save(); err != nil {
		return out, r.recovery()
	}
	for link := range owned {
		if err = r.operation(ctx, link, []string{"tc", "qdisc", "del", "dev", link, "root"}); err != nil {
			return out, err
		}
	}
	for i, s := range p.Steps {
		if err = r.operation(ctx, s.Link, (&LinuxRunner{}).argvFor(s)); err != nil {
			out.Failed = len(p.Steps) - i
			return out, err
		}
		out.Applied++
	}
	// Runtime identity must match the requested roots, not merely exit zero.
	for link, root := range wanted {
		var rows map[string][]map[string]any
		_ = json.Unmarshal(r.journal.Pending.Observed[link], &rows)
		matched := false
		for _, row := range rows["qdisc"] {
			if len(root.Args) > 0 && row["kind"] == root.Args[0] && row["handle"] == root.Handle {
				matched = true
			}
		}
		if !matched {
			return out, r.recovery()
		}
	}
	live := map[string]json.RawMessage{}
	for link := range wanted {
		live[link] = r.journal.Pending.Observed[link]
	}
	r.journal.Previous = r.journal.Current
	r.journal.Current = p
	r.journal.Live = live
	r.journal.Pending = nil
	if err = r.save(); err != nil {
		return out, r.recovery()
	}
	return out, nil
}

func (r *OwnedRunner) operation(ctx context.Context, link string, args []string) error {
	p := r.journal.Pending
	actual, err := r.exec.Observe(ctx, link)
	if err != nil || !bytes.Equal(actual, p.Observed[link]) {
		return r.recovery()
	}
	p.InFlight = true
	if err = r.save(); err != nil {
		return r.recovery()
	}
	err = r.exec.Run(ctx, args)
	observeCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	after, observeErr := r.exec.Observe(observeCtx, link)
	if observeErr != nil {
		return r.recovery()
	}
	if err != nil {
		// A failed/cancelled command with changed state has uncertain effects.
		if !bytes.Equal(after, actual) {
			return r.recovery()
		}
		p.InFlight = false
		if saveErr := r.save(); saveErr != nil {
			return r.recovery()
		}
		return err
	}
	p.Observed[link] = after
	p.Touched[link] = true
	p.InFlight = false
	if err = r.save(); err != nil {
		return r.recovery()
	}
	return nil
}

func (r *OwnedRunner) Rollback(ctx context.Context, current, previous Plan) (ApplyOutcome, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.journal.RecoveryRequired || r.lock == nil {
		return ApplyOutcome{Error: ErrRecoveryRequired.Error()}, ErrRecoveryRequired
	}
	if r.journal.Pending == nil {
		// Failed admission has no effects to undo. An explicit rollback is
		// admitted only against the actual committed plan, never a failed plan.
		a, _ := json.Marshal(current)
		b, _ := json.Marshal(r.journal.Current)
		if !bytes.Equal(a, b) {
			return ApplyOutcome{}, nil
		}
		out, err := r.apply(ctx, r.journal.Previous)
		if err != nil && r.journal.Pending != nil {
			_, _ = r.undo(context.WithoutCancel(ctx))
		}
		return out, err
	}
	return r.undo(ctx)
}
func (r *OwnedRunner) undo(ctx context.Context) (out ApplyOutcome, err error) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 16*time.Second)
	defer cancel()
	p := r.journal.Pending
	if p.InFlight {
		return out, r.recovery()
	}
	links := []string{}
	for link := range p.Touched {
		links = append(links, link)
	}
	sort.Strings(links)
	for _, link := range links {
		actual, e := r.exec.Observe(ctx, link)
		if e != nil || !bytes.Equal(actual, p.Observed[link]) {
			return out, r.recovery()
		}
		if !baseline(actual) {
			if e = r.operation(ctx, link, []string{"tc", "qdisc", "del", "dev", link, "root"}); e != nil {
				return out, r.recovery()
			}
		}
	}
	for _, s := range r.journal.Current.Steps {
		if p.Touched[s.Link] {
			if e := r.operation(ctx, s.Link, (&LinuxRunner{}).argvFor(s)); e != nil {
				return out, r.recovery()
			}
			out.Applied++
		}
	}
	for link, want := range p.Before {
		actual, e := r.exec.Observe(ctx, link)
		if e != nil || !bytes.Equal(actual, want) {
			return out, r.recovery()
		}
	}
	r.journal.Pending = nil
	if err = r.save(); err != nil {
		return out, r.recovery()
	}
	return out, nil
}
func (r *OwnedRunner) State(ctx context.Context) (RunnerState, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.journal.RecoveryRequired {
		return RunnerState{Error: ErrRecoveryRequired.Error()}, ErrRecoveryRequired
	}
	for link, want := range r.journal.Live {
		got, err := r.exec.Observe(ctx, link)
		if err != nil || !bytes.Equal(got, want) {
			return RunnerState{Error: ErrRecoveryRequired.Error()}, r.recovery()
		}
	}
	return RunnerState{HasShapers: len(r.journal.Live)}, nil
}
