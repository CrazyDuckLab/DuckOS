//go:build linux

package qos

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
)

type ownedFake struct {
	mu         sync.Mutex
	root       map[string]any
	failDelete bool
	cancel     context.CancelFunc
}

func (f *ownedFake) Observe(context.Context, string) (json.RawMessage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	root := f.root
	if root == nil {
		root = map[string]any{"kind": "noqueue", "handle": "0:"}
	}
	return json.Marshal(map[string]any{"qdisc": []any{root}, "class": []any{}, "filter": []any{}})
}
func (f *ownedFake) Run(ctx context.Context, args []string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if args[1] == "class" {
		if f.cancel != nil {
			f.cancel()
			return context.Canceled
		}
		return errors.New("class rejected")
	}
	if args[2] == "del" {
		if f.failDelete {
			return errors.New("delete rejected")
		}
		f.root = nil
		return nil
	}
	f.root = map[string]any{"kind": args[8], "handle": args[7], "options": append([]string(nil), args[9:]...)}
	return nil
}

func ownedPlan() Plan {
	return Plan{SchemaVersion: SchemaVersion, Steps: []Step{{Kind: StepQdisc, Link: "test0", Parent: "root", Handle: "1:", Args: []string{"fq_codel", "limit", "1000"}}}}
}
func partialPlan() Plan {
	p := ownedPlan()
	p.Steps = append(p.Steps, Step{Kind: StepClass, Link: "test0", Parent: "1:", Handle: "1:1", Args: []string{"invalid"}})
	return p
}

func TestOwnedRollbackRestoresPreviousAndSurvivesCancellation(t *testing.T) {
	for _, cancelled := range []bool{false, true} {
		f := &ownedFake{}
		r, err := OpenOwnedRunner(t.TempDir(), f)
		if err != nil {
			t.Fatal(err)
		}
		defer r.Close()
		c, _ := NewController(r)
		if _, err := c.Apply(context.Background(), ownedPlan()); err != nil {
			t.Fatal(err)
		}
		before, _ := f.Observe(context.Background(), "test0")
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		if cancelled {
			f.cancel = cancel
		}
		p := partialPlan()
		p.Steps[0].Args = []string{"fq_codel", "limit", "2000"}
		if _, err := c.Apply(ctx, p); err == nil {
			t.Fatal("partial apply reported success")
		}
		after, _ := f.Observe(context.Background(), "test0")
		if string(before) != string(after) {
			t.Fatal("previous owned state not restored")
		}
		if _, err := c.PreviewState(context.Background()); err != nil {
			t.Fatal(err)
		}
		if len(c.Current().Steps) != 1 || c.Current().Steps[0].Args[2] != "1000" {
			t.Fatal("failed candidate became current")
		}
	}
}

func TestOwnedRollbackFailureIsDurableRecoveryRequired(t *testing.T) {
	dir := t.TempDir()
	f := &ownedFake{failDelete: true}
	r, err := OpenOwnedRunner(dir, f)
	if err != nil {
		t.Fatal(err)
	}
	c, _ := NewController(r)
	if _, err := c.Apply(context.Background(), partialPlan()); !errors.Is(err, ErrRecoveryRequired) {
		t.Fatalf("rollback failure not typed: %v", err)
	}
	if len(c.Current().Steps) != 0 {
		t.Fatal("failed runtime became current")
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	restarted, err := OpenOwnedRunner(dir, f)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	if s, err := restarted.State(context.Background()); !errors.Is(err, ErrRecoveryRequired) || !strings.Contains(s.Error, "recovery_required") {
		t.Fatal("restart forgot recovery")
	}
	if _, err := restarted.Apply(context.Background(), ownedPlan()); !errors.Is(err, ErrRecoveryRequired) {
		t.Fatal("mutation accepted during recovery")
	}
}

func TestOwnedRefusesExternalStateWithoutRollbackMutation(t *testing.T) {
	f := &ownedFake{root: map[string]any{"kind": "pfifo", "handle": "77:", "options": map[string]any{"limit": 2345}}}
	r, err := OpenOwnedRunner(t.TempDir(), f)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	c, _ := NewController(r)
	before, _ := f.Observe(context.Background(), "test0")
	if _, err := c.Apply(context.Background(), ownedPlan()); err == nil {
		t.Fatal("external state accepted")
	}
	after, _ := f.Observe(context.Background(), "test0")
	if string(before) != string(after) {
		t.Fatal("external qdisc changed")
	}
}
