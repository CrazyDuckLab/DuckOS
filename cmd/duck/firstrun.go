// duck firstrun command: render the G18 first-run
// persistence status from the privileged Agent. Read-only;
// writes remain privileged on the Agent.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"time"
)

const firstrunHelp = `duck firstrun — render the first-run persistence status

Usage:
  duck firstrun [--output json|table|text]

Examples:
  duck firstrun
  duck firstrun --output table
`

// firstRunResponse mirrors the IPC envelope so the CLI
// can decode without importing internal/agent.
type firstRunResponse struct {
	Version   int            `json:"version"`
	UpdatedAt time.Time      `json:"updatedAt"`
	Pending   bool           `json:"pending,omitempty"`
	Completed bool           `json:"completed,omitempty"`
	State     *firstRunState `json:"state,omitempty"`
}

type firstRunState struct {
	Version        int       `json:"version"`
	CompletedAt    time.Time `json:"completedAt,omitempty"`
	Mode           string    `json:"mode,omitempty"`
	Locale         string    `json:"locale,omitempty"`
	Theme          string    `json:"theme,omitempty"`
	Steps          []string  `json:"steps,omitempty"`
	AdminTokenHash string    `json:"adminTokenHash,omitempty"`
	Hostname       string    `json:"hostname,omitempty"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

func firstrunCmd(args []string) error {
	fs := flag.NewFlagSet("firstrun", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	cf := addCommonFlags(fs)
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Print(firstrunHelp)
			return nil
		}
		return err
	}
	state, err := newState(cf)
	if err != nil {
		return err
	}
	var out firstRunResponse
	code, err := state.call(context.Background(), apiCall{method: http.MethodGet, path: "/agent/v1/firstrun"}, &out)
	if err != nil {
		return err
	}
	if code != 200 {
		return fmt.Errorf("firstrun: HTTP %d", code)
	}
	return renderFirstRun(cf.output, &out)
}

func renderFirstRun(format string, r *firstRunResponse) error {
	switch format {
	case "json", "":
		b, err := json.MarshalIndent(r, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(b))
		return nil
	case "text", "table":
		return renderFirstRunText(r)
	}
	return nil
}

func renderFirstRunText(r *firstRunResponse) error {
	fmt.Printf("firstrun version=%d updated=%s\n", r.Version, r.UpdatedAt.Format(time.RFC3339))
	if r.Pending {
		fmt.Println("status: pending (no first-run document on disk)")
		return nil
	}
	if !r.Completed {
		fmt.Println("status: not completed")
		return nil
	}
	if r.State == nil {
		fmt.Println("status: completed (state omitted)")
		return nil
	}
	st := r.State
	fmt.Println("status: completed")
	fmt.Printf("  completedAt: %s\n", st.CompletedAt.Format(time.RFC3339))
	fmt.Printf("  mode:        %s\n", orDash(st.Mode))
	fmt.Printf("  locale:      %s\n", orDash(st.Locale))
	fmt.Printf("  theme:       %s\n", orDash(st.Theme))
	fmt.Printf("  hostname:    %s\n", orDash(st.Hostname))
	if len(st.Steps) > 0 {
		fmt.Printf("  steps:       %s\n", joinStrings(st.Steps, ","))
	}
	fmt.Printf("  tokenHash:   %s\n", shortHash(st.AdminTokenHash))
	return nil
}

func joinStrings(s []string, sep string) string {
	out := ""
	for i, v := range s {
		if i > 0 {
			out += sep
		}
		out += v
	}
	return out
}

func shortHash(h string) string {
	if len(h) <= 12 {
		return orDash(h)
	}
	return h[:12] + "…"
}
