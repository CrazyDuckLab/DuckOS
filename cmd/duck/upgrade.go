// duck upgrade command: render the G16 upgrade activation
// status from the privileged Agent. Read-only; activate /
// materialize remain Agent-privileged.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const upgradeHelp = `duck upgrade — render the upgrade activation status

Usage:
  duck upgrade [--output json|table|text]

Examples:
  duck upgrade
  duck upgrade --output table
`

// upgradeStatus mirrors the IPC envelope so the CLI can
// decode without importing internal/agent.
type upgradeStatus struct {
	Version          int       `json:"version"`
	UpdatedAt        time.Time `json:"updatedAt"`
	CurrentRelease   string    `json:"currentRelease,omitempty"`
	CurrentVersion   string    `json:"currentVersion,omitempty"`
	CandidateRelease string    `json:"candidateRelease,omitempty"`
	CandidateVersion string    `json:"candidateVersion,omitempty"`
	PreviousRelease  string    `json:"previousRelease,omitempty"`
	PreviousVersion  string    `json:"previousVersion,omitempty"`
	Healthcheck      string    `json:"healthcheck,omitempty"`
	Healthy          bool      `json:"healthy"`
	Notes            []string  `json:"notes,omitempty"`
}

func upgradeCmd(args []string) error {
	fs := flag.NewFlagSet("upgrade", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	cf := addCommonFlags(fs)
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Print(upgradeHelp)
			return nil
		}
		return err
	}
	state, err := newState(cf)
	if err != nil {
		return err
	}
	var out upgradeStatus
	code, err := state.call(context.Background(), apiCall{method: http.MethodGet, path: "/api/v1/upgrade"}, &out)
	if err != nil {
		return err
	}
	if code != 200 {
		return fmt.Errorf("upgrade: HTTP %d", code)
	}
	return renderUpgrade(cf.output, &out)
}

func renderUpgrade(format string, r *upgradeStatus) error {
	switch format {
	case "json", "":
		b, err := json.MarshalIndent(r, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(b))
		return nil
	case "text", "table":
		return renderUpgradeText(r)
	}
	return nil
}

func renderUpgradeText(r *upgradeStatus) error {
	fmt.Printf("upgrade version=%d updated=%s healthy=%v\n", r.Version, r.UpdatedAt.Format(time.RFC3339), r.Healthy)
	fmt.Printf("  current:   %s (%s)\n", orDash(r.CurrentRelease), orDash(r.CurrentVersion))
	fmt.Printf("  candidate: %s (%s)\n", orDash(r.CandidateRelease), orDash(r.CandidateVersion))
	fmt.Printf("  previous:  %s (%s)\n", orDash(r.PreviousRelease), orDash(r.PreviousVersion))
	if r.Healthcheck != "" {
		fmt.Printf("  health:    %s\n", r.Healthcheck)
	}
	for _, note := range r.Notes {
		fmt.Printf("  note:      %s\n", note)
	}
	return nil
}

// silence unused-import warning for strings.
var _ = strings.TrimSpace
