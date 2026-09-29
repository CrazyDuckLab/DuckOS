// upgrade/recovery.go: H11 update recovery semantics.
//
// The base Orchestrator in orchestrator.go implements
// Stage / Activate / Rollback, but does not verify the
// candidate's integrity before activation and does not
// detect inconsistent state on load. This file adds:
//
//	VerifyBeforeActivate — checks the candidate's payload
//	fingerprint and signature before the atomic symlink
//	swap. Corrupt candidates are refused without mutating
//	the active release.
//
//	LoadState — wraps SetState with a crash-recovery
//	audit. If the persisted State references a candidate
//	that is not materialised, or an Active/Previous that
//	is missing from the store, the load surfaces an
//	explicit recovery hint instead of silently rolling
//	forward with stale state.
//
//	CancellableHealthCheck — wraps the runner with a
//	context.Context so a stalled probe does not hang the
//	activation. The orchestrator still holds the lock for
//	the duration of the check (per the existing
//	ActivateAndVerify contract) so cancellation does not
//	leave the system in a half-swapped state.
package upgrade

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// ErrCorruptCandidate is returned by VerifyBeforeActivate
// when the staged release fails integrity checks.
var ErrCorruptCandidate = errors.New("upgrade: candidate failed integrity check")

// ErrInconsistentState is returned by LoadState when the
// persisted state references releases that are not
// materialised on disk.
var ErrInconsistentState = errors.New("upgrade: persisted state is inconsistent")

// VerifyCandidate runs the integrity checks against the
// supplied release id using the ReleaseStorage. The
// function refuses to verify an empty id. The Store
// implementation is responsible for the underlying
// fingerprint and signature verification; this wrapper
// only enforces the canonical "verify before activate"
// contract and converts the verifier's error into the
// ErrCorruptCandidate sentinel so callers can route it
// uniformly.
func VerifyCandidate(ctx context.Context, store ReleaseStorage, id string) error {
	if ctx == nil {
		return errors.New("upgrade: verify context is nil")
	}
	if id == "" {
		return errors.New("upgrade: candidate id is empty")
	}
	if store == nil {
		return errors.New("upgrade: release storage is nil")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := store.VerifyFingerprint(id); err != nil {
		return fmt.Errorf("%w: %s", ErrCorruptCandidate, err.Error())
	}
	return nil
}

// LoadState runs a crash-recovery audit before committing
// the supplied state to the orchestrator. The audit
// verifies every referenced release (Active, Previous,
// Candidate) is actually materialised. A missing release
// surfaces an explicit ErrInconsistentState so the caller
// can decide whether to:
//   - roll back to a known-good release, OR
//   - re-materialise the missing release, OR
//   - start the recovery flow.
//
// LoadState never mutates orchestrator state on failure.
func LoadState(o *Orchestrator, store ReleaseStorage, s State) error {
	if o == nil {
		return errors.New("upgrade: orchestrator is nil")
	}
	if store == nil {
		return errors.New("upgrade: release storage is nil")
	}
	if err := s.Valid(); err != nil {
		return err
	}
	missing := []string{}
	if s.Active != "" {
		if err := store.VerifyFingerprint(s.Active); err != nil {
			missing = append(missing, "active="+s.Active)
		}
	}
	if s.Previous != "" {
		if err := store.VerifyFingerprint(s.Previous); err != nil {
			missing = append(missing, "previous="+s.Previous)
		}
	}
	if s.Candidate != "" {
		if err := store.VerifyFingerprint(s.Candidate); err != nil {
			missing = append(missing, "candidate="+s.Candidate)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("%w: %v", ErrInconsistentState, missing)
	}
	return o.SetState(s)
}

// CancellableHealthCheck executes a single HealthCheck
// honouring ctx cancellation. The function returns
// immediately when ctx is canceled. The orchestrator
// invokes this from ActivateAndVerify; using a separate
// helper keeps the cancellation contract obvious and
// unit-testable.
func CancellableHealthCheck(ctx context.Context, runner CheckRunner, check HealthCheck) CheckResult {
	if ctx == nil {
		return CheckResult{Name: check.Name, Passed: false, Error: "context is nil"}
	}
	if runner == nil {
		return CheckResult{Name: check.Name, Passed: false, Error: "health runner unavailable"}
	}
	if err := ctx.Err(); err != nil {
		return CheckResult{Name: check.Name, Passed: false, Error: "canceled: " + err.Error()}
	}
	if !checkNamePattern.MatchString(check.Name) {
		return CheckResult{Name: check.Name, Passed: false, Error: "invalid check name"}
	}
	if err := check.Validate(); err != nil {
		return CheckResult{Name: check.Name, Passed: false, Error: "invalid check: " + err.Error()}
	}
	type runResult struct {
		passed  bool
		err     string
		elapsed time.Duration
	}
	done := make(chan runResult, 1)
	start := time.Now()
	go func() {
		runErr := runner.Run(check)
		if runErr != nil {
			done <- runResult{err: runErr.Error(), elapsed: time.Since(start)}
			return
		}
		done <- runResult{passed: true, elapsed: time.Since(start)}
	}()
	select {
	case <-ctx.Done():
		return CheckResult{Name: check.Name, Passed: false, Error: "canceled: " + ctx.Err().Error(), Elapsed: time.Since(start)}
	case r := <-done:
		if err := ctx.Err(); err != nil {
			return CheckResult{Name: check.Name, Passed: false, Error: "canceled: " + err.Error(), Elapsed: time.Since(start)}
		}
		return CheckResult{Name: check.Name, Passed: r.passed, Error: r.err, Elapsed: r.elapsed}
	}
}
