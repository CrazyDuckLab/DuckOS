// Package main — Duck CLI config transaction subcommands
// (G07 CLI/API parity). CLI and API go through the same
// Engine-mediated public endpoints; there is no separate
// hidden configuration path.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
)

// configCmd is the parent dispatch for the `duck config`
// subcommand.
func configCmd(args []string) error {
	if len(args) == 0 {
		return errors.New("config: missing operation")
	}
	op, rest := args[0], args[1:]
	switch op {
	case "status":
		return configStatus(rest)
	case "diff":
		return configDiff(rest)
	case "apply":
		return configApply(rest)
	case "confirm":
		return configConfirm(rest)
	case "rollback":
		return configRollback(rest)
	default:
		return fmt.Errorf("config: unknown operation %q", op)
	}
}

// configStatus returns the canonical config journal status
// (current revision, pending transaction, recent outcomes).
func configStatus(args []string) error {
	fs := flag.NewFlagSet("config status", flag.ContinueOnError)
	cf := addCommonFlags(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	state, err := newState(cf)
	if err != nil {
		return err
	}
	var out map[string]any
	code, err := state.call(context.Background(), apiCall{method: http.MethodGet, path: "/api/v1/config/status"}, &out)
	if err != nil {
		return err
	}
	if code != 200 {
		return fmt.Errorf("config status returned %d: %s", code, errorMessage(out))
	}
	return render(cf.output, out)
}

// configDiff computes a structured diff between the current
// revision and a supplied candidate.
func configDiff(args []string) error {
	fs := flag.NewFlagSet("config diff", flag.ContinueOnError)
	cf := addCommonFlags(fs)
	file := fs.String("candidate", "", "Path to a candidate config JSON file")
	expected := fs.String("expected-revision", "", "Expected current revision id")
	requestID := fs.String("request-id", "", "Stable request id")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *file == "" {
		return errors.New("config diff: --candidate is required")
	}
	raw, err := os.ReadFile(*file)
	if err != nil {
		return fmt.Errorf("config diff: read candidate: %w", err)
	}
	body, err := json.Marshal(map[string]any{
		"requestId": *requestID,
		"expected":  *expected,
		"candidate": json.RawMessage(raw),
	})
	if err != nil {
		return err
	}
	state, err := newState(cf)
	if err != nil {
		return err
	}
	var out map[string]any
	code, err := state.call(context.Background(), apiCall{method: http.MethodPost, path: "/api/v1/config/diff", body: bytes.NewReader(body)}, &out)
	if err != nil {
		return err
	}
	if code != 200 {
		return fmt.Errorf("config diff returned %d: %s", code, errorMessage(out))
	}
	return render(cf.output, out)
}

// configApply stages a candidate transaction in the durable
// config journal. Window is clamped to [120, 900] seconds.
func configApply(args []string) error {
	fs := flag.NewFlagSet("config apply", flag.ContinueOnError)
	cf := addCommonFlags(fs)
	file := fs.String("candidate", "", "Path to a candidate config JSON file")
	expected := fs.String("expected-revision", "", "Expected current revision id")
	requestID := fs.String("request-id", "", "Stable request id (32 hex chars)")
	actor := fs.String("actor", "duck-cli", "Operator identity")
	reason := fs.String("reason", "duck-cli apply", "Human-readable reason")
	window := fs.Int("confirm-seconds", 300, "Confirmation window in seconds (120..900)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *file == "" || *requestID == "" || *expected == "" {
		return errors.New("config apply: --candidate, --request-id, and --expected-revision are required")
	}
	if len(*requestID) != 32 {
		return errors.New("config apply: --request-id must be exactly 32 hex characters")
	}
	if *window < 120 || *window > 900 {
		return errors.New("config apply: --confirm-seconds must be in [120, 900]")
	}
	raw, err := os.ReadFile(*file)
	if err != nil {
		return fmt.Errorf("config apply: read candidate: %w", err)
	}
	body, err := json.Marshal(map[string]any{
		"requestId":     *requestID,
		"expected":      *expected,
		"candidate":     json.RawMessage(raw),
		"actor":         *actor,
		"reason":        *reason,
		"windowSeconds": *window,
	})
	if err != nil {
		return err
	}
	state, err := newState(cf)
	if err != nil {
		return err
	}
	var out map[string]any
	code, err := state.call(context.Background(), apiCall{method: http.MethodPost, path: "/api/v1/config/apply", body: bytes.NewReader(body)}, &out)
	if err != nil {
		return err
	}
	if code != 202 && code != 200 {
		return fmt.Errorf("config apply returned %d: %s", code, errorMessage(out))
	}
	return render(cf.output, out)
}

// configConfirm commits the pending transaction whose id
// is supplied as the first argument; remaining arguments
// are flag arguments (--endpoint, --allow-insecure).
func configConfirm(args []string) error {
	if len(args) == 0 {
		return errors.New("config confirm: requires a transaction id argument")
	}
	txID := args[0]
	fs := flag.NewFlagSet("config confirm", flag.ContinueOnError)
	cf := addCommonFlags(fs)
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	state, err := newState(cf)
	if err != nil {
		return err
	}
	var out map[string]any
	code, err := state.call(context.Background(), apiCall{method: http.MethodPost, path: "/api/v1/config/confirm/" + txID}, &out)
	if err != nil {
		return err
	}
	if code != 200 {
		return fmt.Errorf("config confirm returned %d: %s", code, errorMessage(out))
	}
	return render(state.cf.output, out)
}

// configRollback rolls back the pending transaction whose
// id is supplied as the first argument; remaining arguments
// are flag arguments.
func configRollback(args []string) error {
	if len(args) == 0 {
		return errors.New("config rollback: requires a transaction id argument")
	}
	txID := args[0]
	fs := flag.NewFlagSet("config rollback", flag.ContinueOnError)
	cf := addCommonFlags(fs)
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	state, err := newState(cf)
	if err != nil {
		return err
	}
	var out map[string]any
	code, err := state.call(context.Background(), apiCall{method: http.MethodPost, path: "/api/v1/config/rollback/" + txID}, &out)
	if err != nil {
		return err
	}
	if code != 200 {
		return fmt.Errorf("config rollback returned %d: %s", code, errorMessage(out))
	}
	return render(state.cf.output, out)
}
