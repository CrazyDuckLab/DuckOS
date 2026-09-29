// duck diagjobs command: render the G19 contextual
// diagnostic job list. Read-only.
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

const diagjobHelp = `duck diagjobs — render the contextual diagnostic job list

Usage:
  duck diagjobs [--id <id>] [--output json|table|text]

Examples:
  duck diagjobs
  duck diagjobs --id dns-1
`

// diagJobsResponse mirrors the IPC envelope.
type diagJobsResponse struct {
	Version   int            `json:"version"`
	UpdatedAt time.Time      `json:"updatedAt"`
	Jobs      []diagJobEntry `json:"jobs"`
}

type diagJobEntry struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	Status    string    `json:"status"`
	Subject   string    `json:"subject,omitempty"`
	StartedAt time.Time `json:"startedAt,omitempty"`
	EndedAt   time.Time `json:"endedAt,omitempty"`
	Truncated bool      `json:"truncated,omitempty"`
	Error     string    `json:"error,omitempty"`
	Lines     []string  `json:"outputLines,omitempty"`
}

func diagjobCmd(args []string) error {
	fs := flag.NewFlagSet("diagjobs", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	cf := addCommonFlags(fs)
	var id string
	fs.StringVar(&id, "id", "", "Optional job id")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Print(diagjobHelp)
			return nil
		}
		return err
	}
	state, err := newState(cf)
	if err != nil {
		return err
	}
	path := "/agent/v1/diagjobs"
	if id != "" {
		path += "?id=" + id
	}
	var out diagJobsResponse
	code, err := state.call(context.Background(), apiCall{method: http.MethodGet, path: path}, &out)
	if err != nil {
		return err
	}
	if code != 200 {
		return fmt.Errorf("diagjobs: HTTP %d", code)
	}
	return renderDiagJobs(cf.output, &out)
}

func renderDiagJobs(format string, r *diagJobsResponse) error {
	switch format {
	case "json", "":
		b, err := json.MarshalIndent(r, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(b))
		return nil
	case "text", "table":
		return renderDiagJobsText(r)
	}
	return nil
}

func renderDiagJobsText(r *diagJobsResponse) error {
	fmt.Printf("diagjobs version=%d updated=%s count=%d\n", r.Version, r.UpdatedAt.Format(time.RFC3339), len(r.Jobs))
	for _, j := range r.Jobs {
		duration := "-"
		if !j.StartedAt.IsZero() && !j.EndedAt.IsZero() {
			duration = j.EndedAt.Sub(j.StartedAt).String()
		}
		fmt.Printf("  - %s kind=%s status=%s subject=%s duration=%s truncated=%v error=%s\n",
			j.ID, j.Kind, j.Status, orDash(j.Subject), duration, j.Truncated, orDash(j.Error))
	}
	return nil
}
