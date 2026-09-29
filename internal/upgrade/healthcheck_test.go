// upgrade/healthcheck_test.go pins the V1
// health-check executor.
package upgrade

import (
	"errors"
	"testing"
	"time"
)

type stubRunner struct {
	errs map[string]error
}

func (s stubRunner) Run(c HealthCheck) error {
	if e, ok := s.errs[c.Name]; ok {
		return e
	}
	return nil
}

func TestExecutorAllPass(t *testing.T) {
	results, ok := NewExecutor(stubRunner{errs: map[string]error{}}, time.Second).RunAll([]HealthCheck{
		{Name: "b", Command: "true"},
		{Name: "a", URL: "http://localhost/health"},
	})
	if !ok {
		t.Fatalf("expected all pass, results=%+v", results)
	}
	if len(results) != 2 {
		t.Fatalf("results: %d", len(results))
	}
	if results[0].Name != "a" || results[1].Name != "b" {
		t.Fatalf("sort order: %+v", results)
	}
}

func TestExecutorFails(t *testing.T) {
	results, ok := NewExecutor(stubRunner{errs: map[string]error{"b": errors.New("boom")}}, time.Second).RunAll([]HealthCheck{
		{Name: "a", Command: "true"},
		{Name: "b", Command: "true"},
		{Name: "c", Command: "true"},
	})
	if ok {
		t.Fatal("expected not all pass")
	}
	if results[1].Passed {
		t.Fatal("b must fail")
	}
	if results[1].Error == "" {
		t.Fatal("error string must be set")
	}
}

func TestExecutorInvalidName(t *testing.T) {
	_, ok := NewExecutor(stubRunner{}, time.Second).RunAll([]HealthCheck{{Name: "!bad", Command: "true"}})
	if ok {
		t.Fatal("invalid name must fail")
	}
}

func TestExecutorInvalidCheck(t *testing.T) {
	_, ok := NewExecutor(stubRunner{}, time.Second).RunAll([]HealthCheck{{Name: "x"}})
	if ok {
		t.Fatal("missing command/url must fail")
	}
}

func TestExecutorTimeout(t *testing.T) {
	slow := func(c HealthCheck) error { time.Sleep(50 * time.Millisecond); return nil }
	results, ok := NewExecutor(checkRunnerFunc(slow), 5*time.Millisecond).RunAll([]HealthCheck{{Name: "x", Command: "true"}})
	if ok {
		t.Fatal("timeout must fail")
	}
	if results[0].Error == "" {
		t.Fatal("error must be set on timeout")
	}
}

type checkRunnerFunc func(HealthCheck) error

func (f checkRunnerFunc) Run(c HealthCheck) error { return f(c) }
